# homebrew-changes

A Homebrew tap that adds `brew changes`: before you upgrade, see what actually
changed.

`brew outdated` tells you *that* packages are outdated. `brew changes` tells
you *what's in* the update — an interactive list of every outdated formula
and cask, where you can expand any of them in place to see its
`CHANGELOG.md` (or `CHANGES.md`, `HISTORY.md`, `NEWS.md`), falling back to
GitHub Releases, then upgrade everything or just the one you're looking at.

## Install

```sh
brew install garthdw/changes/brew-changes
```

This installs a prebuilt binary — no Go toolchain required.

## Usage

```sh
brew changes
```

This will:

1. Find every outdated formula and cask (`brew outdated`).
2. For each one, try to resolve its GitHub repository from its homepage or
   source URL.
3. Open an interactive list — nothing is fetched yet, so this is instant even
   with many outdated packages.
4. Navigate and review: expanding a package fetches and renders its
   changelog in place, so you only pay the GitHub API cost for what you
   actually look at.
5. Upgrade everything, or just the package you're currently looking at.

### Keybindings

| Key         | Action                                      |
| ----------- | -------------------------------------------- |
| `↑`/`↓`, `j`/`k` | Move the cursor                        |
| `Enter`     | Expand/collapse the highlighted package's changelog |
| `a`         | Upgrade all outdated packages and quit      |
| `u`         | Upgrade just the highlighted package and quit |
| `q`, `Ctrl+C` | Quit without upgrading                    |

Packages whose source isn't hosted on GitHub are listed with a note that no
changelog could be found, rather than being skipped silently.

## GitHub API rate limits

`brew changes` talks to the GitHub API directly. Unauthenticated requests are
limited to 60/hour, which can be tight if you review a lot of changelogs. Set
a token to raise that limit:

```sh
export GITHUB_TOKEN=ghp_...   # or GH_TOKEN
```

If you have the [`gh`](https://cli.github.com) CLI installed and authenticated
(`gh auth login`), `brew changes` will use its token automatically as a
fallback when neither environment variable is set.

## Releasing

Publishing a GitHub Release (with a `vX.Y.Z` tag) triggers
[`.github/workflows/release.yml`](.github/workflows/release.yml), which runs
[GoReleaser](https://goreleaser.com) to build macOS binaries (arm64 + amd64)
and attach them to the release, then auto-commits the regenerated
`Formula/brew-changes.rb` to `main` with the new version/url/sha256.
