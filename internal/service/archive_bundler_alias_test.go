package service

import (
	"strings"
	"testing"
)

func TestBuildRewriteAliases_IncludesAbsoluteRelativeAndOriginal(t *testing.T) {
	base := "https://example.com/ads/123"
	src := "https://example.com/lib/css/profile.css?v=4.07"
	aliases := buildRewriteAliases(base, src)

	want := []string{
		"https://example.com/lib/css/profile.css?v=4.07",
		"/lib/css/profile.css?v=4.07",
		"../lib/css/profile.css?v=4.07",
	}

	for _, expected := range want {
		found := false
		for _, a := range aliases {
			if a == expected {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("expected alias %q not found in %#v", expected, aliases)
		}
	}
}

func TestBuildRewriteAliases_CrossOriginDoesNotInventRelativeAliases(t *testing.T) {
	base := "https://example.com/ads/123"
	src := "https://cdn.example.net/lib/css/profile.css?v=4.07"
	aliases := buildRewriteAliases(base, src)

	containsAbs := false
	for _, a := range aliases {
		if a == src {
			containsAbs = true
		}
		if strings.HasPrefix(a, "../") || strings.HasPrefix(a, "/") {
			t.Fatalf("unexpected relative/root alias for cross-origin source: %q (all=%#v)", a, aliases)
		}
	}
	if !containsAbs {
		t.Fatalf("absolute source alias missing: %#v", aliases)
	}
}
