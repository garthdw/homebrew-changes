package knownchangelogs

import "testing"

func TestLookup(t *testing.T) {
	url, ok := Lookup("rust")
	if !ok || url == "" {
		t.Errorf("Lookup(%q) = (%q, %v), want a known URL", "rust", url, ok)
	}

	if _, ok := Lookup("_comment"); ok {
		t.Error(`Lookup("_comment") should not expose the JSON file's own comment field`)
	}

	if _, ok := Lookup("definitely-not-a-real-package"); ok {
		t.Error("Lookup of an unknown package should return ok = false")
	}
}
