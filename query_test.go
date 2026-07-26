package main

import "testing"

func TestBuildContainsQuery(t *testing.T) {
	got := buildContainsQuery("darr")
	want := "(title:*darr* OR content:*darr* OR path:*darr*)"
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}

	got = buildContainsQuery("darryn resume")
	if !containsAll(got, "title:*darryn*", "title:*resume*", " AND ") {
		t.Fatalf("unexpected multi-term query: %q", got)
	}

	raw := `title:foo*`
	if buildContainsQuery(raw) != raw {
		t.Fatalf("expected passthrough for solr syntax")
	}
}

func containsAll(s string, parts ...string) bool {
	for _, p := range parts {
		if !containsStr(s, p) {
			return false
		}
	}
	return true
}

func containsStr(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(sub) == 0 ||
		(func() bool {
			for i := 0; i+len(sub) <= len(s); i++ {
				if s[i:i+len(sub)] == sub {
					return true
				}
			}
			return false
		})())
}
