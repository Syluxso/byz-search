package main

import (
	"strings"
	"testing"
)

func TestBuildContainsQueryPathLike(t *testing.T) {
	got := buildContainsQuery("/notifications/{id}")
	if !strings.Contains(got, "code_tokens:*") {
		t.Fatalf("expected code_tokens clause, got %q", got)
	}
	if !strings.Contains(got, "notifications") {
		t.Fatalf("expected word token notifications, got %q", got)
	}
	// Must not be treated as raw Solr field syntax passthrough.
	if got == "/notifications/{id}" {
		t.Fatalf("should not passthrough path-like query")
	}
}

func TestBuildContainsQueryWords(t *testing.T) {
	got := buildContainsQuery("darryn resume")
	if !strings.Contains(got, " AND ") {
		t.Fatalf("expected AND for multi-word, got %q", got)
	}
	if strings.Contains(got, "code_tokens:*darryn resume*") {
		t.Fatalf("plain words should not use full path clause, got %q", got)
	}
}

func TestBuildContainsQueryPassthrough(t *testing.T) {
	raw := `title:foo*`
	if buildContainsQuery(raw) != raw {
		t.Fatalf("expected passthrough for solr syntax")
	}
}
