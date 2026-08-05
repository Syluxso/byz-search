package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// Solr field names expected from Ingest (see README).
const (
	fieldID     = "id"
	fieldTitle  = "title"
	fieldBody   = "content"
	fieldOrg    = "organization_id"
	fieldTenant = "tenant_id"
	fieldUser   = "user_id"
	fieldSource = "source"
	fieldPath   = "path"
	fieldTags   = "tags"
)

type SolrClient struct {
	base       string
	collection string
	http       *http.Client
}

func NewSolrClient(baseURL, collection string) *SolrClient {
	baseURL = strings.TrimRight(baseURL, "/")
	return &SolrClient{
		base:       baseURL,
		collection: collection,
		http: &http.Client{
			Timeout: 15 * time.Second,
		},
	}
}

func (s *SolrClient) Ping(ctx context.Context) error {
	u := fmt.Sprintf("%s/solr/%s/admin/ping?wt=json", s.base, url.PathEscape(s.collection))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return err
	}
	resp, err := s.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)
	if resp.StatusCode >= 300 {
		return fmt.Errorf("solr ping status %d", resp.StatusCode)
	}
	return nil
}

type solrSelectResponse struct {
	Response struct {
		NumFound int64            `json:"numFound"`
		Docs     []map[string]any `json:"docs"`
	} `json:"response"`
	Highlighting map[string]map[string][]string `json:"highlighting"`
}

// GetByID returns one document's stored fields within org (+ optional user visibility).
// Returns (nil, nil) when no matching doc (caller maps to 404).
func (s *SolrClient) GetByID(ctx context.Context, id, orgID, userID string) (map[string]any, error) {
	id = strings.TrimSpace(id)
	orgID = strings.TrimSpace(orgID)
	if id == "" || orgID == "" {
		return nil, fmt.Errorf("id and organizationId required")
	}
	params := url.Values{}
	params.Set("wt", "json")
	params.Set("q", fieldID+":"+solrEscape(id))
	params.Set("rows", "1")
	params.Set("fl", "id,title,content,organization_id,tenant_id,user_id,source,path,tags")
	params.Add("fq", fieldOrg+":"+solrEscape(orgID))
	if userID != "" {
		params.Add("fq", fmt.Sprintf("(*:* -%s:[* TO *]) OR %s:%s", fieldUser, fieldUser, solrEscape(userID)))
	}

	u := fmt.Sprintf("%s/solr/%s/select?%s", s.base, url.PathEscape(s.collection), params.Encode())
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	resp, err := s.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 16<<20))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 300 {
		return nil, fmt.Errorf("solr select status %d: %s", resp.StatusCode, truncate(string(body), 600))
	}
	var parsed solrSelectResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, fmt.Errorf("solr decode: %w", err)
	}
	if len(parsed.Response.Docs) == 0 {
		return nil, nil
	}
	return parsed.Response.Docs[0], nil
}

func (s *SolrClient) Search(ctx context.Context, q string, orgID, tenantID, userID string, page, size int) (total int64, hits []SearchHit, err error) {
	// Prefer code_tokens for path-like queries; if Solr schema lacks the field (400),
	// retry without it so normal search keeps working.
	luceneQ := buildContainsQuery(q, true)
	total, hits, err = s.searchWithQuery(ctx, luceneQ, q, orgID, tenantID, userID, page, size)
	if err != nil && strings.Contains(err.Error(), "status 400") && strings.Contains(luceneQ, fieldCodeTokens) {
		fallback := buildContainsQuery(q, false)
		if fallback != luceneQ {
			return s.searchWithQuery(ctx, fallback, q, orgID, tenantID, userID, page, size)
		}
	}
	return total, hits, err
}

func (s *SolrClient) searchWithQuery(ctx context.Context, luceneQ, rawQ, orgID, tenantID, userID string, page, size int) (total int64, hits []SearchHit, err error) {
	params := url.Values{}
	params.Set("wt", "json")
	// Leading wildcards required for "contains" (*term*).
	params.Set("q", "{!lucene allowLeadingWildcard=true v=$byzq}")
	params.Set("byzq", luceneQ)
	params.Set("start", strconv.Itoa(page*size))
	params.Set("rows", strconv.Itoa(size))
	params.Set("fl", "id,title,content,organization_id,tenant_id,user_id,source,path,tags,score")
	params.Set("hl", "true")
	params.Set("hl.fl", fieldBody+","+fieldTitle)
	params.Set("hl.snippets", "2")
	params.Set("hl.fragsize", "280")
	params.Set("hl.method", "unified")
	params.Add("fq", fieldOrg+":"+solrEscape(orgID))
	// Tenant: matching tenant OR no tenant (many uploads leave tenant_id empty).
	if tenantID != "" {
		params.Add("fq", fmt.Sprintf("(*:* -%s:[* TO *]) OR %s:%s", fieldTenant, fieldTenant, solrEscape(tenantID)))
	}
	// Personal + shared: docs with no user_id (org/tenant-wide) OR owned by this user.
	if userID != "" {
		params.Add("fq", fmt.Sprintf("(*:* -%s:[* TO *]) OR %s:%s", fieldUser, fieldUser, solrEscape(userID)))
	}

	u := fmt.Sprintf("%s/solr/%s/select?%s", s.base, url.PathEscape(s.collection), params.Encode())
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return 0, nil, err
	}
	resp, err := s.http.Do(req)
	if err != nil {
		return 0, nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return 0, nil, err
	}
	if resp.StatusCode >= 300 {
		return 0, nil, fmt.Errorf("solr select status %d: %s", resp.StatusCode, truncate(string(body), 600))
	}

	var parsed solrSelectResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		return 0, nil, fmt.Errorf("solr decode: %w", err)
	}

	hits = make([]SearchHit, 0, len(parsed.Response.Docs))
	for _, doc := range parsed.Response.Docs {
		id := anyString(doc[fieldID])
		hit := SearchHit{
			ID:             id,
			Title:          anyString(doc[fieldTitle]),
			Score:          anyFloat(doc["score"]),
			Source:         anyString(doc[fieldSource]),
			Path:           anyString(doc[fieldPath]),
			OrganizationID: anyString(doc[fieldOrg]),
			TenantID:       anyString(doc[fieldTenant]),
			UserID:         anyString(doc[fieldUser]),
			Tags:           anyStringSlice(doc[fieldTags]),
		}
		body := anyString(doc[fieldBody])
		title := anyString(doc[fieldTitle])
		if hl := parsed.Highlighting[id]; hl != nil {
			// Prefer body highlights so Admin shows matching document text.
			if snips := hl[fieldBody]; len(snips) > 0 {
				hit.Snippet = snips[0]
			} else if snips := hl[fieldTitle]; len(snips) > 0 {
				hit.Snippet = snips[0]
			}
		}
		if hit.Snippet == "" {
			hit.Snippet = snippetAround(body, rawQ, 220)
		}
		if hit.Snippet == "" {
			hit.Snippet = truncate(firstNonEmpty(body, title), 220)
		}
		hits = append(hits, hit)
	}
	return parsed.Response.NumFound, hits, nil
}

func solrEscape(v string) string {
	// Quote values that need it for fq.
	if v == "" {
		return `""`
	}
	replacer := strings.NewReplacer(`\`, `\\`, `"`, `\"`)
	return `"` + replacer.Replace(v) + `"`
}

func anyString(v any) string {
	switch t := v.(type) {
	case nil:
		return ""
	case string:
		return t
	case []any:
		if len(t) == 0 {
			return ""
		}
		return anyString(t[0])
	case float64:
		return strconv.FormatFloat(t, 'f', -1, 64)
	default:
		return fmt.Sprint(t)
	}
}

func anyFloat(v any) float64 {
	switch t := v.(type) {
	case float64:
		return t
	case json.Number:
		f, _ := t.Float64()
		return f
	default:
		return 0
	}
}

func anyStringSlice(v any) []string {
	switch t := v.(type) {
	case nil:
		return nil
	case []any:
		out := make([]string, 0, len(t))
		for _, x := range t {
			if s := anyString(x); s != "" {
				out = append(out, s)
			}
		}
		return out
	case []string:
		return t
	case string:
		if t == "" {
			return nil
		}
		return []string{t}
	default:
		return nil
	}
}

func truncate(s string, n int) string {
	if n <= 0 || len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

// snippetAround returns a window of text around the first case-insensitive match
// of any query token, wrapping the match in <em> for Admin rendering.
func snippetAround(text, rawQ string, window int) string {
	text = strings.TrimSpace(text)
	if text == "" || window < 40 {
		return ""
	}
	lower := strings.ToLower(text)
	var match string
	var idx int = -1
	for _, tok := range strings.Fields(strings.ToLower(strings.TrimSpace(rawQ))) {
		tok = strings.Trim(tok, `*?"'`)
		if len(tok) < 2 {
			continue
		}
		if i := strings.Index(lower, tok); i >= 0 {
			match = text[i : i+len(tok)]
			idx = i
			break
		}
	}
	if idx < 0 {
		return ""
	}
	pad := window / 2
	start := idx - pad
	if start < 0 {
		start = 0
	}
	end := idx + len(match) + pad
	if end > len(text) {
		end = len(text)
	}
	prefix := ""
	if start > 0 {
		prefix = "…"
	}
	suffix := ""
	if end < len(text) {
		suffix = "…"
	}
	return prefix + text[start:idx] + "<em>" + match + "</em>" + text[idx+len(match):end] + suffix
}
