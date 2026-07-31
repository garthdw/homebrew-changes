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
// heading is found, it returns up to MaxContentLines from there. If neither
// heading can be found, it returns the first MaxContentLines of content.
func TrimToRange(content, newVersion, installedVersion string) string {
	lines := strings.Split(content, "\n")

	startLine := findHeading(lines, newVersion)
	if startLine == -1 {
		end := MaxContentLines
		if end > len(lines) {
			end = len(lines)
		}
		result := strings.Join(lines[:end], "\n")
		return result + fmt.Sprintf("\n\n... (couldn't locate version headings, showing first %d lines)", MaxContentLines)
	}

	endLine := findHeading(lines, installedVersion)
	if endLine != -1 && endLine > startLine {
		return strings.Join(lines[startLine:endLine], "\n")
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
	return result
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
