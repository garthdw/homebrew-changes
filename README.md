# homebrew-changes

A Homebrew tap that adds `brew changes`: before you upgrade, see what actually
changed.

`brew outdated` tells you *that* packages are outdated. `brew changes` tells
you *what's in* the update — pulling each package's `CHANGELOG.md` (or
`CHANGES.md`, `HISTORY.md`, `NEWS.md`) from GitHub, or falling back to GitHub
Releases, before asking whether you want to upgrade.

## Requirements

- [`gh`](https://cli.github.com) (GitHub CLI), authenticated: `gh auth login`
- `jq`

Install both with:

```sh
brew install gh jq
```

## Install

```sh
brew tap <your-github-user>/changes
```

`brew changes` is then available automatically — Homebrew discovers external
commands in a tap's `cmd/` directory.

## Usage

```sh
brew changes
```

This will:

1. Find every outdated formula and cask (`brew outdated`).
2. For each one, try to resolve its GitHub repository from its homepage or
   source URL.
3. Fetch and print the relevant changelog section (or recent GitHub releases
   if no changelog file exists).
4. Prompt once, at the end, to upgrade everything via `brew upgrade` /
   `brew upgrade --cask` (default: yes — press Enter to proceed).

Packages whose source isn't hosted on GitHub are listed with a note that no
changelog could be found, rather than being skipped silently.
