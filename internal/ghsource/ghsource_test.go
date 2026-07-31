package ghsource

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

func withTestServer(t *testing.T, handler http.HandlerFunc) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	orig := apiBaseURL
	apiBaseURL = srv.URL
	t.Cleanup(func() { apiBaseURL = orig })
	return srv
}

func TestResolveRepo(t *testing.T) {
	tests := []struct {
		name      string
		homepage  string
		url       string
		wantOwner string
		wantRepo  string
		wantOK    bool
	}{
		{
			name:      "homepage is github",
			homepage:  "https://github.com/owner/repo",
			url:       "https://example.com/tarball.tar.gz",
			wantOwner: "owner",
			wantRepo:  "repo",
			wantOK:    true,
		},
		{
			name:      "falls back to url when homepage isn't github",
			homepage:  "https://example.com",
			url:       "https://github.com/owner/repo.git",
			wantOwner: "owner",
			wantRepo:  "repo",
			wantOK:    true,
		},
		{
			name:      "homepage with trailing path",
			homepage:  "https://github.com/owner/repo/issues",
			wantOwner: "owner",
			wantRepo:  "repo",
			wantOK:    true,
		},
		{
			name:      "ssh-style url",
			url:       "git@github.com:owner/repo.git",
			wantOwner: "owner",
			wantRepo:  "repo",
			wantOK:    true,
		},
		{
			name:     "neither points at github",
			homepage: "https://example.com",
			url:      "https://gitlab.com/owner/repo",
			wantOK:   false,
		},
		{
			name:   "both empty",
			wantOK: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			owner, repo, ok := ResolveRepo(tt.homepage, tt.url)
			if ok != tt.wantOK {
				t.Fatalf("ok = %v, want %v", ok, tt.wantOK)
			}
			if !tt.wantOK {
				return
			}
			if owner != tt.wantOwner || repo != tt.wantRepo {
				t.Errorf("got (%q, %q), want (%q, %q)", owner, repo, tt.wantOwner, tt.wantRepo)
			}
		})
	}
}

func TestFetchChangelogFile_PicksFirstMatch(t *testing.T) {
	withTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/repos/o/r/contents/CHANGELOG.md" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		body, _ := json.Marshal(map[string]string{
			"content": base64.StdEncoding.EncodeToString([]byte("## 1.0.0\nnotes")),
		})
		w.Write(body)
	})

	filename, content, ok := FetchChangelogFile("o", "r")
	if !ok {
		t.Fatal("expected ok = true")
	}
	if filename != "CHANGELOG.md" {
		t.Errorf("got filename %q, want CHANGELOG.md", filename)
	}
	if content != "## 1.0.0\nnotes" {
		t.Errorf("got content %q", content)
	}
}

func TestFetchChangelogFile_FallsThroughToLaterName(t *testing.T) {
	withTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/repos/o/r/contents/HISTORY.md" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		body, _ := json.Marshal(map[string]string{
			"content": base64.StdEncoding.EncodeToString([]byte("history notes")),
		})
		w.Write(body)
	})

	filename, content, ok := FetchChangelogFile("o", "r")
	if !ok || filename != "HISTORY.md" || content != "history notes" {
		t.Errorf("got (%q, %q, %v), want (HISTORY.md, \"history notes\", true)", filename, content, ok)
	}
}

func TestFetchChangelogFile_NoneFound(t *testing.T) {
	withTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})

	_, _, ok := FetchChangelogFile("o", "r")
	if ok {
		t.Error("expected ok = false when no changelog file exists")
	}
}

func TestFetchReleases_FiltersInstalledAndPaginates(t *testing.T) {
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Query().Get("page") {
		case "", "1":
			w.Header().Set("Link", fmt.Sprintf(`<%s/repos/o/r/releases?per_page=100&page=2>; rel="next"`, srv.URL))
			body, _ := json.Marshal([]Release{
				{TagName: "v2.0.0", PublishedAt: "2024-02-01", Body: "second"},
				{TagName: "v1.0.0", PublishedAt: "2024-01-01", Body: "first (installed)"},
			})
			w.Write(body)
		case "2":
			// No Link header: this is the last page.
			body, _ := json.Marshal([]Release{{TagName: "v0.5.0", Body: "older"}})
			w.Write(body)
		}
	}))
	t.Cleanup(srv.Close)
	orig := apiBaseURL
	apiBaseURL = srv.URL
	t.Cleanup(func() { apiBaseURL = orig })

	releases, err := FetchReleases("o", "r", "1.0.0")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := []string{"v2.0.0", "v0.5.0"}
	if len(releases) != len(want) {
		t.Fatalf("got %+v, want tags %v", releases, want)
	}
	for i, tag := range want {
		if releases[i].TagName != tag {
			t.Errorf("release %d: got %q, want %q", i, releases[i].TagName, tag)
		}
	}
}

func TestFetchReleases_ErrorStatus(t *testing.T) {
	withTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	})

	_, err := FetchReleases("o", "r", "1.0.0")
	if err == nil {
		t.Error("expected error on non-200 status")
	}
}

func TestNextPageURL(t *testing.T) {
	tests := []struct {
		name       string
		linkHeader string
		want       string
	}{
		{
			name:       "no link header",
			linkHeader: "",
			want:       "",
		},
		{
			name:       "next only",
			linkHeader: `<https://api.github.com/repos/o/r/releases?page=2>; rel="next"`,
			want:       "https://api.github.com/repos/o/r/releases?page=2",
		},
		{
			name:       "next and last",
			linkHeader: `<https://api.github.com/repos/o/r/releases?page=2>; rel="next", <https://api.github.com/repos/o/r/releases?page=5>; rel="last"`,
			want:       "https://api.github.com/repos/o/r/releases?page=2",
		},
		{
			name:       "only last, no next (final page)",
			linkHeader: `<https://api.github.com/repos/o/r/releases?page=1>; rel="prev", <https://api.github.com/repos/o/r/releases?page=5>; rel="last"`,
			want:       "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := nextPageURL(tt.linkHeader); got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}
