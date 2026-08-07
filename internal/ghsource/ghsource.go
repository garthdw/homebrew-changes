// Package ghsource resolves a package's GitHub repository and fetches its
// changelog file or release notes over the GitHub REST API.
package ghsource

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"sync"
)

var changelogFilenames = []string{"CHANGELOG.md", "CHANGES.md", "HISTORY.md", "NEWS.md", "CHANGELOG"}

// apiBaseURL is the GitHub REST API base, overridable in tests.
var apiBaseURL = "https://api.github.com"

var githubURLPattern = regexp.MustCompile(`github\.com[:/]([^/]+)/([^/.]+)(\.git)?(/.*)?$`)

// ResolveRepo extracts "owner" and "repo" from a URL pointing at
// github.com, trying homepage first and falling back to url. ok is false if
// neither points at GitHub.
func ResolveRepo(homepage, url string) (owner, repo string, ok bool) {
	if owner, repo, ok = repoFromURL(homepage); ok {
		return owner, repo, true
	}
	return repoFromURL(url)
}

func repoFromURL(url string) (owner, repo string, ok bool) {
	m := githubURLPattern.FindStringSubmatch(url)
	if m == nil {
		return "", "", false
	}
	return m[1], m[2], true
}

var (
	tokenOnce  sync.Once
	tokenValue string
)

// token returns a GitHub API token, preferring GITHUB_TOKEN/GH_TOKEN, then
// falling back to `gh auth token` if the gh CLI happens to be available.
// Returns "" if no token could be found (requests proceed unauthenticated).
// The result is resolved once per process and cached, since it can require
// spawning the gh CLI and doesn't change during a run.
func token() string {
	tokenOnce.Do(func() {
		for _, env := range []string{"GITHUB_TOKEN", "GH_TOKEN"} {
			if v := os.Getenv(env); v != "" {
				tokenValue = v
				return
			}
		}
		if _, err := exec.LookPath("gh"); err == nil {
			if out, err := exec.Command("gh", "auth", "token").Output(); err == nil {
				tokenValue = strings.TrimSpace(string(out))
			}
		}
	})
	return tokenValue
}

func get(url string) (*http.Response, error) {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	if t := token(); t != "" {
		req.Header.Set("Authorization", "Bearer "+t)
	}
	return http.DefaultClient.Do(req)
}

type contentsResponse struct {
	Content string `json:"content"`
}

// FetchChangelogFileAt tries fetching a single candidate changelog filename
// from the repo's default branch.
func FetchChangelogFileAt(owner, repo, filename string) (content string, ok bool) {
	url := fmt.Sprintf("%s/repos/%s/%s/contents/%s", apiBaseURL, owner, repo, filename)
	resp, err := get(url)
	if err != nil {
		return "", false
	}
	body, err := io.ReadAll(resp.Body)
	resp.Body.Close()
	if err != nil || resp.StatusCode != http.StatusOK {
		return "", false
	}

	var parsed contentsResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		return "", false
	}
	decoded, err := base64.StdEncoding.DecodeString(strings.ReplaceAll(parsed.Content, "\n", ""))
	if err != nil || len(decoded) == 0 {
		return "", false
	}
	return string(decoded), true
}

// FetchChangelogFile tries each well-known changelog filename in turn on
// the repo's default branch, returning the first one found.
func FetchChangelogFile(owner, repo string) (filename, content string, ok bool) {
	p := NewChangelogProbe(owner, repo)
	for name := p.Filename(); name != ""; name = p.Filename() {
		if content, ok := p.Try(); ok {
			return name, content, true
		}
	}
	return "", "", false
}

// ChangelogProbe steps through the well-known changelog filenames for a
// repo one at a time, so a caller (e.g. the TUI) can report which filename
// is currently being checked without knowing the candidate list itself.
type ChangelogProbe struct {
	owner, repo string
	idx         int
}

// NewChangelogProbe creates a probe positioned at the first candidate
// changelog filename for owner/repo.
func NewChangelogProbe(owner, repo string) *ChangelogProbe {
	return &ChangelogProbe{owner: owner, repo: repo}
}

// Filename returns the candidate filename the next call to Try will check,
// or "" once every candidate has been tried.
func (p *ChangelogProbe) Filename() string {
	if p.idx >= len(changelogFilenames) {
		return ""
	}
	return changelogFilenames[p.idx]
}

// Try fetches the current candidate filename and advances the probe to the
// next one. ok reports whether the file was found.
func (p *ChangelogProbe) Try() (content string, ok bool) {
	name := p.Filename()
	if name == "" {
		return "", false
	}
	content, ok = FetchChangelogFileAt(p.owner, p.repo, name)
	p.idx++
	return content, ok
}

// Release is a single GitHub release.
type Release struct {
	TagName     string `json:"tag_name"`
	PublishedAt string `json:"published_at"`
	Body        string `json:"body"`
}

const maxReleasePages = 3

// FetchReleases returns releases newer than installedVersion, newest first,
// capped at a reasonable count. Tag names are compared with a leading "v"
// stripped so "v1.2.3" and "1.2.3" match.
func FetchReleases(owner, repo, installedVersion string) ([]Release, error) {
	normalizedInstalled := strings.TrimPrefix(installedVersion, "v")
	var filtered []Release
	url := fmt.Sprintf("%s/repos/%s/%s/releases?per_page=100", apiBaseURL, owner, repo)

	for page := 0; page < maxReleasePages && url != "" && len(filtered) < 20; page++ {
		resp, err := get(url)
		if err != nil {
			return nil, fmt.Errorf("fetching releases for %s/%s: %w", owner, repo, err)
		}
		body, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			return nil, err
		}
		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("fetching releases for %s/%s: status %d", owner, repo, resp.StatusCode)
		}

		var pageReleases []Release
		if err := json.Unmarshal(body, &pageReleases); err != nil {
			return nil, fmt.Errorf("parsing releases for %s/%s: %w", owner, repo, err)
		}
		for _, r := range pageReleases {
			if strings.TrimPrefix(r.TagName, "v") == normalizedInstalled {
				continue
			}
			filtered = append(filtered, r)
			if len(filtered) >= 20 {
				break
			}
		}

		url = nextPageURL(resp.Header.Get("Link"))
	}

	return filtered, nil
}

var linkNextPattern = regexp.MustCompile(`<([^>]+)>;\s*rel="next"`)

func nextPageURL(linkHeader string) string {
	m := linkNextPattern.FindStringSubmatch(linkHeader)
	if m == nil {
		return ""
	}
	return m[1]
}
