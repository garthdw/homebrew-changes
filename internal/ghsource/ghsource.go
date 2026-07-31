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
)

var changelogFilenames = []string{"CHANGELOG.md", "CHANGES.md", "HISTORY.md", "NEWS.md", "CHANGELOG"}

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

// token returns a GitHub API token, preferring GITHUB_TOKEN/GH_TOKEN, then
// falling back to `gh auth token` if the gh CLI happens to be available.
// Returns "" if no token could be found (requests proceed unauthenticated).
func token() string {
	for _, env := range []string{"GITHUB_TOKEN", "GH_TOKEN"} {
		if v := os.Getenv(env); v != "" {
			return v
		}
	}
	if _, err := exec.LookPath("gh"); err == nil {
		out, err := exec.Command("gh", "auth", "token").Output()
		if err == nil {
			return strings.TrimSpace(string(out))
		}
	}
	return ""
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

// FetchChangelogFile tries each well-known changelog filename in turn on
// the repo's default branch, returning the first one found.
func FetchChangelogFile(owner, repo string) (filename, content string, ok bool) {
	for _, name := range changelogFilenames {
		url := fmt.Sprintf("https://api.github.com/repos/%s/%s/contents/%s", owner, repo, name)
		resp, err := get(url)
		if err != nil {
			continue
		}
		body, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil || resp.StatusCode != http.StatusOK {
			continue
		}

		var parsed contentsResponse
		if err := json.Unmarshal(body, &parsed); err != nil {
			continue
		}
		decoded, err := base64.StdEncoding.DecodeString(strings.ReplaceAll(parsed.Content, "\n", ""))
		if err != nil || len(decoded) == 0 {
			continue
		}
		return name, string(decoded), true
	}
	return "", "", false
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
	var all []Release
	url := fmt.Sprintf("https://api.github.com/repos/%s/%s/releases?per_page=100", owner, repo)

	for page := 0; page < maxReleasePages && url != ""; page++ {
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
		all = append(all, pageReleases...)

		url = nextPageURL(resp.Header.Get("Link"))
	}

	normalizedInstalled := strings.TrimPrefix(installedVersion, "v")
	var filtered []Release
	for _, r := range all {
		if strings.TrimPrefix(r.TagName, "v") == normalizedInstalled {
			continue
		}
		filtered = append(filtered, r)
		if len(filtered) >= 20 {
			break
		}
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
