package main

import "testing"

func TestSolrEscape(t *testing.T) {
	got := solrEscape(`acme"corp`)
	want := `"acme\"corp"`
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestTruncate(t *testing.T) {
	if truncate("abcdef", 3) != "abc…" {
		t.Fatal(truncate("abcdef", 3))
	}
}
