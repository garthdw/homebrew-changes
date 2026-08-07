// Package changelog trims a full changelog file down to the section
// relevant to a specific version upgrade.
package changelog

import (
	"fmt"
	"strings"
)

// MaxContentLines caps how much of a changelog is shown when the exact
// version range can't be located.
const MaxContentLines = 100

// TrimToRange returns the section of content between the newVersion and
// installedVersion headings, if both can be located. If only newVersion's
// heading is found, it returns up to MaxContentLines from there. found is
// false only when newVersion's heading can't be located at all — e.g. an
// index-style changelog (like Node.js's, which just links out to
// per-major-version files) that doesn't actually document the version in
// question. In that case, the first MaxContentLines of content is returned
// as a last resort, but callers may prefer to treat found=false as a
// signal to fall back to another source (release notes, say).
func TrimToRange(content, newVersion, installedVersion string) (string, bool) {
	lines := strings.Split(content, "\n")

	startLine := findHeading(lines, newVersion)
	if startLine == -1 {
		end := MaxContentLines
		if end > len(lines) {
			end = len(lines)
		}
		result := strings.Join(lines[:end], "\n")
		return result + fmt.Sprintf("\n\n... (couldn't locate version headings, showing first %d lines)", MaxContentLines), false
	}

	endLine := findHeading(lines, installedVersion)
	if endLine != -1 && endLine > startLine {
		return strings.Join(lines[startLine:endLine], "\n"), true
	}

	end := startLine + MaxContentLines
	truncated := end < len(lines)
	if end > len(lines) {
		end = len(lines)
	}
	result := strings.Join(lines[startLine:end], "\n")
	if truncated {
		result += fmt.Sprintf("\n\n... (truncated, showing %d lines from the %s heading)", MaxContentLines, newVersion)
	}
	return result, true
}

// findHeading returns the index of the first markdown heading line
// containing version, or -1 if none is found.
func findHeading(lines []string, version string) int {
	if version == "" {
		return -1
	}
	for i, line := range lines {
		trimmed := strings.TrimLeft(line, "#")
		if trimmed == line {
			continue // no leading '#', not a heading
		}
		if strings.Contains(line, version) {
			return i
		}
	}
	return -1
}
