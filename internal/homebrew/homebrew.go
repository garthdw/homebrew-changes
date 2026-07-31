// Package homebrew wraps the `brew` CLI's JSON output for outdated-package
// discovery, per-package metadata lookup, and upgrading.
package homebrew

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// Package is an outdated formula or cask.
type Package struct {
	Name      string
	Installed string
	Current   string
	IsCask    bool
}

type outdatedFormula struct {
	Name              string   `json:"name"`
	InstalledVersions []string `json:"installed_versions"`
	CurrentVersion    string   `json:"current_version"`
}

type outdatedCask struct {
	Name              string   `json:"name"`
	InstalledVersions []string `json:"installed_versions"`
	CurrentVersion    string   `json:"current_version"`
}

type outdatedResult struct {
	Formulae []outdatedFormula `json:"formulae"`
	Casks    []outdatedCask    `json:"casks"`
}

// Outdated returns every outdated formula and cask.
func Outdated() ([]Package, error) {
	var result outdatedResult

	out, err := exec.Command("brew", "outdated", "--json=v2", "--formula").Output()
	if err != nil {
		return nil, fmt.Errorf("brew outdated --formula: %w", err)
	}
	var formulaResult outdatedResult
	if err := json.Unmarshal(out, &formulaResult); err != nil {
		return nil, fmt.Errorf("parsing brew outdated --formula output: %w", err)
	}
	result.Formulae = formulaResult.Formulae

	out, err = exec.Command("brew", "outdated", "--json=v2", "--cask").Output()
	if err != nil {
		return nil, fmt.Errorf("brew outdated --cask: %w", err)
	}
	var caskResult outdatedResult
	if err := json.Unmarshal(out, &caskResult); err != nil {
		return nil, fmt.Errorf("parsing brew outdated --cask output: %w", err)
	}
	result.Casks = caskResult.Casks

	packages := make([]Package, 0, len(result.Formulae)+len(result.Casks))
	for _, f := range result.Formulae {
		packages = append(packages, Package{
			Name:      f.Name,
			Installed: firstOrEmpty(f.InstalledVersions),
			Current:   f.CurrentVersion,
			IsCask:    false,
		})
	}
	for _, c := range result.Casks {
		packages = append(packages, Package{
			Name:      c.Name,
			Installed: firstOrEmpty(c.InstalledVersions),
			Current:   c.CurrentVersion,
			IsCask:    true,
		})
	}

	return packages, nil
}

func firstOrEmpty(versions []string) string {
	if len(versions) > 0 {
		return versions[0]
	}
	return ""
}

type formulaInfo struct {
	Homepage string `json:"homepage"`
	URLs     struct {
		Stable struct {
			URL string `json:"url"`
		} `json:"stable"`
		Head struct {
			URL string `json:"url"`
		} `json:"head"`
	} `json:"urls"`
}

type caskInfo struct {
	Homepage string `json:"homepage"`
	URL      string `json:"url"`
}

type formulaInfoResult struct {
	Formulae []formulaInfo `json:"formulae"`
}

type caskInfoResult struct {
	Casks []caskInfo `json:"casks"`
}

// ResolveInfo returns a package's homepage and source/download URL, either
// of which may point at its GitHub repository.
func ResolveInfo(name string, isCask bool) (homepage, url string, err error) {
	if isCask {
		out, err := exec.Command("brew", "info", "--json=v2", "--cask", name).Output()
		if err != nil {
			return "", "", fmt.Errorf("brew info --cask %s: %w", name, err)
		}
		var result caskInfoResult
		if err := json.Unmarshal(out, &result); err != nil {
			return "", "", fmt.Errorf("parsing brew info --cask %s output: %w", name, err)
		}
		if len(result.Casks) == 0 {
			return "", "", fmt.Errorf("no cask info returned for %s", name)
		}
		return result.Casks[0].Homepage, result.Casks[0].URL, nil
	}

	out, err := exec.Command("brew", "info", "--json=v2", "--formula", name).Output()
	if err != nil {
		return "", "", fmt.Errorf("brew info --formula %s: %w", name, err)
	}
	var result formulaInfoResult
	if err := json.Unmarshal(out, &result); err != nil {
		return "", "", fmt.Errorf("parsing brew info --formula %s output: %w", name, err)
	}
	if len(result.Formulae) == 0 {
		return "", "", fmt.Errorf("no formula info returned for %s", name)
	}
	f := result.Formulae[0]
	url = f.URLs.Stable.URL
	if url == "" {
		url = f.URLs.Head.URL
	}
	return f.Homepage, url, nil
}

// Upgrade runs `brew upgrade` for the named formulae and/or `brew upgrade
// --cask` for the named casks, streaming brew's output to the terminal.
// Only safe to call when nothing else owns the terminal (e.g. after a TUI
// has quit) — use UpgradeQuiet otherwise.
func Upgrade(formulaNames, caskNames []string) error {
	return upgrade(run, formulaNames, caskNames)
}

// UpgradeQuiet behaves like Upgrade but captures brew's output instead of
// streaming it to the terminal, for callers that don't own the terminal
// (e.g. a Bubble Tea UI still running in alt-screen mode).
func UpgradeQuiet(formulaNames, caskNames []string) error {
	return upgrade(runCaptured, formulaNames, caskNames)
}

func upgrade(runFn func(name string, args ...string) error, formulaNames, caskNames []string) error {
	if len(formulaNames) > 0 {
		if err := runFn("brew", append([]string{"upgrade"}, formulaNames...)...); err != nil {
			return err
		}
	}
	if len(caskNames) > 0 {
		if err := runFn("brew", append([]string{"upgrade", "--cask"}, caskNames...)...); err != nil {
			return err
		}
	}
	return nil
}

func run(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("%s %v: %w", name, args, err)
	}
	return nil
}

func runCaptured(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s %v: %w: %s", name, args, err, strings.TrimSpace(string(out)))
	}
	return nil
}
