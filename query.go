package main

import (
	"strings"
	"unicode"
)

// buildContainsQuery turns a plain user query into a leading/trailing wildcard
// Lucene query over title, content, and path. Explicit Solr syntax is passed through.
func buildContainsQuery(raw string) string {
	q := strings.TrimSpace(raw)
	if q == "" {
		return "*:*"
	}
	if looksLikeSolrSyntax(q) {
		return q
	}

	tokens := strings.Fields(q)
	parts := make([]string, 0, len(tokens))
	for _, tok := range tokens {
		tok = strings.ToLower(strings.TrimSpace(tok))
		if tok == "" {
			continue
		}
		esc := escapeLuceneTerm(tok)
		if esc == "" {
			continue
		}
		parts = append(parts,
			"(title:*"+esc+"* OR content:*"+esc+"* OR path:*"+esc+"*)")
	}
	if len(parts) == 0 {
		return "*:*"
	}
	return strings.Join(parts, " AND ")
}

func looksLikeSolrSyntax(q string) bool {
	if strings.HasPrefix(q, "{!") {
		return true
	}
	for _, r := range q {
		switch r {
		case '*', '?', ':', '"', '(', ')', '[', ']', '{', '}', '~', '^':
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
