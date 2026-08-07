package changelog

import (
	"strconv"
	"strings"
	"testing"
)

func TestTrimToRange_BetweenHeadings(t *testing.T) {
	content := strings.Join([]string{
		"# Changelog",
		"",
		"## 2.0.0",
		"line a",
		"line b",
		"## 1.0.0",
		"line c",
	}, "\n")

	got, found := TrimToRange(content, "2.0.0", "1.0.0")
	want := strings.Join([]string{
		"## 2.0.0",
		"line a",
		"line b",
	}, "\n")

	if !found {
		t.Error("expected found = true")
	}
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestTrimToRange_InstalledHeadingMissing(t *testing.T) {
	lines := make([]string, 0, 5)
	lines = append(lines, "## 2.0.0")
	for i := 0; i < 150; i++ {
		lines = append(lines, "line "+strconv.Itoa(i))
	}
	content := strings.Join(lines, "\n")

	got, found := TrimToRange(content, "2.0.0", "1.0.0")

	if !found {
		t.Error("expected found = true")
	}
	if !strings.HasPrefix(got, "## 2.0.0\nline 0") {
		t.Errorf("expected result to start from the new-version heading, got prefix %q", got[:30])
	}
	if !strings.Contains(got, "truncated, showing 100 lines from the 2.0.0 heading") {
		t.Errorf("expected truncation notice, got %q", got)
	}
	gotLines := strings.Split(got, "\n")
	// MaxContentLines (100) content lines + 2 blank/notice lines appended.
	if len(gotLines) != MaxContentLines+2 {
		t.Errorf("got %d lines, want %d", len(gotLines), MaxContentLines+2)
	}
}

func TestTrimToRange_NewVersionHeadingMissing(t *testing.T) {
	lines := make([]string, 0, 150)
	for i := 0; i < 150; i++ {
		lines = append(lines, "line "+strconv.Itoa(i))
	}
	content := strings.Join(lines, "\n")

	got, found := TrimToRange(content, "9.9.9", "1.0.0")

	if found {
		t.Error("expected found = false when the new-version heading can't be located")
	}
	if !strings.Contains(got, "couldn't locate version headings, showing first 100 lines") {
		t.Errorf("expected fallback notice, got %q", got)
	}
	if !strings.HasPrefix(got, "line 0\n") {
		t.Errorf("expected result to start from the beginning, got prefix %q", got[:20])
	}
}

func TestTrimToRange_InstalledHeadingBeforeNewVersion(t *testing.T) {
	// installedVersion's heading appears before newVersion's — should be
	// ignored, falling back to the truncation-from-start behavior.
	content := strings.Join([]string{
		"## 1.0.0",
		"old stuff",
		"## 2.0.0",
		"new stuff",
	}, "\n")

	got, found := TrimToRange(content, "2.0.0", "1.0.0")
	want := "## 2.0.0\nnew stuff"

	if !found {
		t.Error("expected found = true")
	}
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestTrimToRange_EmptyInstalledVersion(t *testing.T) {
	content := strings.Join([]string{
		"## 2.0.0",
		"line a",
	}, "\n")

	got, found := TrimToRange(content, "2.0.0", "")
	want := "## 2.0.0\nline a"

	if !found {
		t.Error("expected found = true")
	}
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestFindHeading_IgnoresNonHeadingLines(t *testing.T) {
	lines := []string{
		"See 2.0.0 in the body text, not a heading",
		"## 2.0.0 actual heading",
	}

	got := findHeading(lines, "2.0.0")
	if got != 1 {
		t.Errorf("got %d, want 1", got)
	}
}

func TestFindHeading_NotFound(t *testing.T) {
	lines := []string{"## 1.0.0", "## 3.0.0"}
	if got := findHeading(lines, "2.0.0"); got != -1 {
		t.Errorf("got %d, want -1", got)
	}
}
