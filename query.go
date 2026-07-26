package main

import (
	"strings"
	"unicode"
)

const fieldCodeTokens = "code_tokens"

// buildContainsQuery turns a plain user query into a Lucene query.
// When useCodeTokens is false, path/API matching falls back to word tokens only
// (needed when Solr has not defined the code_tokens field yet).
func buildContainsQuery(raw string, useCodeTokens bool) string {
	q := strings.TrimSpace(raw)
	if q == "" {
		return "*:*"
	}
	if looksLikeSolrSyntax(q) {
		return q
	}

	lower := strings.ToLower(q)
	pathLike := isPathLikeQuery(lower)
	parts := make([]string, 0, 8)

	if useCodeTokens && pathLike {
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

	for _, tok := range splitSearchTokens(lower) {
		esc := escapeLuceneTerm(tok)
		if esc == "" {
			continue
		}
		clause := "(title:*" + esc + "* OR content:*" + esc + "* OR path:*" + esc + "*"
		if useCodeTokens && pathLike {
			clause += " OR " + fieldCodeTokens + ":*" + esc + "*"
		}
		clause += ")"
		parts = append(parts, clause)
	}

	if len(parts) == 0 {
		esc := escapeLuceneTerm(lower)
		if esc == "" {
			return "*:*"
		}
		return "(title:*" + esc + "* OR content:*" + esc + "* OR path:*" + esc + "*)"
	}

	if pathLike {
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
