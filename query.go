package main

import (
	"strings"
	"unicode"
)

const fieldCodeTokens = "code_tokens"

// buildContainsQuery turns a plain user query into a Lucene query that:
//   - matches word tokens on title/content/path (punctuation-split)
//   - matches path/API-shaped strings on code_tokens (keeps / { } etc.)
//
// Explicit Solr syntax is passed through unchanged.
func buildContainsQuery(raw string) string {
	q := strings.TrimSpace(raw)
	if q == "" {
		return "*:*"
	}
	if looksLikeSolrSyntax(q) {
		return q
	}

	lower := strings.ToLower(q)
	parts := make([]string, 0, 8)

	// Path / code shaped: keep punctuation, search multi-valued string field.
	if isPathLikeQuery(lower) {
		esc := escapeLuceneTerm(lower)
		if esc != "" {
			parts = append(parts, fieldCodeTokens+":*"+esc+"*")
			if trimmed := strings.TrimPrefix(lower, "/"); trimmed != lower {
				if e2 := escapeLuceneTerm(trimmed); e2 != "" {
					parts = append(parts, fieldCodeTokens+":*"+e2+"*")
				}
			}
		}
	}

	// Word tokens from punctuation-split (so "/notifications/{id}" also yields notifications + id).
	for _, tok := range splitSearchTokens(lower) {
		esc := escapeLuceneTerm(tok)
		if esc == "" {
			continue
		}
		parts = append(parts,
			"(title:*"+esc+"* OR content:*"+esc+"* OR path:*"+esc+"* OR "+fieldCodeTokens+":*"+esc+"*)")
	}

	if len(parts) == 0 {
		// Fallback: whole string as contains on analyzed fields.
		esc := escapeLuceneTerm(lower)
		if esc == "" {
			return "*:*"
		}
		return "(title:*" + esc + "* OR content:*" + esc + "* OR path:*" + esc + "*)"
	}

	// Path-like queries: prefer code_tokens match OR all word parts (OR, not AND),
	// so "/notifications/{id}" hits the path token without requiring every segment.
	if isPathLikeQuery(lower) {
		return "(" + strings.Join(parts, " OR ") + ")"
	}
	return strings.Join(parts, " AND ")
}

func isPathLikeQuery(q string) bool {
	return strings.ContainsAny(q, "/{}")
}

func splitSearchTokens(q string) []string {
	var b strings.Builder
	var out []string
	flush := func() {
		t := strings.TrimSpace(b.String())
		b.Reset()
		if len(t) < 2 {
			return
		}
		out = append(out, t)
	}
	for _, r := range q {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' || r == '.' || r == '-' {
			b.WriteRune(unicode.ToLower(r))
			continue
		}
		flush()
	}
	flush()
	return out
}

func looksLikeSolrSyntax(q string) bool {
	if strings.HasPrefix(q, "{!") {
		return true
	}
	// Path-like queries with { } are NOT Solr syntax — we handle those ourselves.
	if isPathLikeQuery(q) {
		return false
	}
	for _, r := range q {
		switch r {
		case '*', '?', ':', '"', '(', ')', '[', ']', '~', '^':
			return true
		}
	}
	upper := strings.ToUpper(q)
	if strings.Contains(upper, " AND ") || strings.Contains(upper, " OR ") || strings.Contains(upper, " NOT ") {
		return true
	}
	return false
}

// escapeLuceneTerm escapes Lucene special chars for a single wildcard term body.
func escapeLuceneTerm(s string) string {
	var b strings.Builder
	b.Grow(len(s) + 8)
	for _, r := range s {
		switch r {
		case '\\', '+', '-', '&', '|', '!', '(', ')', '{', '}', '[', ']', '^', '"', '~', '*', '?', ':', '/':
			b.WriteByte('\\')
			b.WriteRune(r)
		default:
			if unicode.IsControl(r) {
				continue
			}
			b.WriteRune(r)
		}
	}
	return b.String()
}
