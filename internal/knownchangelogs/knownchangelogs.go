// Package knownchangelogs holds a small manual override list of real
// changelog/release-notes pages for Homebrew formulae whose changelog
// can't be found through ghsource's GitHub-based auto-detection. See
// known_changelogs.json for how entries were chosen and verified, and why
// each one needed a manual override rather than being auto-detectable.
package knownchangelogs

import (
	_ "embed"
	"encoding/json"
	"sync"
)

//go:embed known_changelogs.json
var data []byte

var (
	once sync.Once
	urls map[string]string
)

func load() map[string]string {
	once.Do(func() {
		var parsed map[string]string
		if err := json.Unmarshal(data, &parsed); err != nil {
			urls = map[string]string{}
			return
		}
		delete(parsed, "_comment")
		urls = parsed
	})
	return urls
}

// Lookup returns the known changelog URL for a Homebrew formula or cask
// name, if one is recorded.
func Lookup(name string) (url string, ok bool) {
	url, ok = load()[name]
	return url, ok
}
