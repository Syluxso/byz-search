package main

import (
	"strings"
	"testing"
)

func TestBuildContainsQueryPathLike(t *testing.T) {
	got := buildContainsQuery("/notifications/{id}", true)
	if !strings.Contains(got, "code_tokens:*") {
		t.Fatalf("expected code_tokens clause, got %q", got)
	}
	gotOff := buildContainsQuery("/notifications/{id}", false)
	if strings.Contains(gotOff, "code_tokens") {
		t.Fatalf("useCodeTokens=false must omit code_tokens: %q", gotOff)
	}
	if !strings.Contains(gotOff, "notifications") {
		t.Fatalf("expected word token notifications, got %q", gotOff)
	}
}

func TestBuildContainsQueryWords(t *testing.T) {
	got := buildContainsQuery("darryn resume", true)
	if !strings.Contains(got, " AND ") {
		t.Fatalf("expected AND for multi-word, got %q", got)
	}
	if strings.Contains(got, "code_tokens") {
		t.Fatalf("plain words must not query code_tokens: %q", got)
	}
}

func TestBuildContainsQueryPassthrough(t *testing.T) {
	raw := `title:foo*`
	if buildContainsQuery(raw, true) != raw {
		t.Fatalf("expected passthrough for solr syntax")
	}
}
