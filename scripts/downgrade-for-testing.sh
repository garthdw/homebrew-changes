#!/usr/bin/env bash
# Downgrades a Homebrew formula to a known older version so `brew changes`
# has something real to show as outdated, without waiting for an actual
# upstream release. For local testing only.
#
# How it works: fetches the formula file as it existed at an older commit
# in Homebrew/homebrew-core, installs it from a scratch local tap (Homebrew
# refuses to install untapped formula files directly), then patches the
# resulting Cellar receipt to claim it came from homebrew/core instead of
# the scratch tap. That last step matters: brew's outdated check resolves
# a formula against the tap recorded in its receipt, so leaving it pointed
# at the scratch tap (which only knows the old version) would never report
# it as outdated.
#
# To restore afterward: brew upgrade <formula> (fetches the real current
# version from homebrew/core and overwrites the patched receipt).
#
# Usage: scripts/downgrade-for-testing.sh [formula...]
#   With no arguments, downgrades every formula known to downgrade_commit().

set -euo pipefail

# Commit in Homebrew/homebrew-core whose Formula/<letter>/<name>.rb still
# has the previous version's bottle. Found via:
#   curl -s "https://api.github.com/repos/Homebrew/homebrew-core/commits?path=Formula/<letter>/<name>.rb" \
#     | grep -E '"sha"|"message"'
# and picking the commit for the version before the one currently installed.
#
# Plain case statement rather than an associative array: macOS's default
# /bin/bash is 3.2, which predates bash 4's `declare -A`.
known_formulae() {
  echo "jq bat"
}

downgrade_commit() {
  case "$1" in
    jq) echo "76881a708efb7c3667f0b3a0c44237d4a466db86" ;;   # jq 1.8.1 (current: 1.8.2)
    bat) echo "1664b3e4774d707b76b0ed02ef0863f307ad5199" ;;  # bat 0.26.0 (current: 0.26.1)
    *) echo "" ;;
  esac
}

TAP_OWNER="$(whoami)"
TAP="${TAP_OWNER}/downgrade-test"
TAP_DIR="$(brew --repository)/Library/Taps/${TAP_OWNER}/homebrew-downgrade-test"

usage() {
  echo "Usage: $0 [formula...]" >&2
  echo "Known formulae: $(known_formulae)" >&2
  exit 1
}

if [ "$#" -eq 0 ]; then
  names="$(known_formulae)"
else
  names="$*"
fi

if ! brew tap | grep -qx "$TAP"; then
  echo "==> Creating scratch tap $TAP"
  brew tap-new "$TAP" --no-git >/dev/null
fi

for name in $names; do
  commit="$(downgrade_commit "$name")"
  if [ -z "$commit" ]; then
    echo "No known downgrade commit for '$name'." >&2
    usage
  fi

  letter="${name:0:1}"
  formula_path="Formula/${letter}/${name}.rb"
  dest="$TAP_DIR/Formula/${name}.rb"

  echo "==> Fetching ${name}.rb @ ${commit}"
  curl -fsSL "https://raw.githubusercontent.com/Homebrew/homebrew-core/${commit}/${formula_path}" -o "$dest"

  echo "==> Uninstalling current ${name} (if installed)"
  brew uninstall --ignore-dependencies "$name" >/dev/null 2>&1 || true

  echo "==> Installing downgraded ${name} from ${TAP}"
  brew install "${TAP}/${name}"

  installed_version="$(brew list --versions "$name" | awk '{print $2}')"
  receipt="$(brew --cellar "$name")/${installed_version}/INSTALL_RECEIPT.json"

  echo "==> Repointing receipt at homebrew/core so 'brew outdated' picks it up"
  python3 - "$receipt" "$formula_path" "$(brew --prefix)" <<'PYEOF'
import json, sys
receipt_path, formula_path = sys.argv[1], sys.argv[2]
with open(receipt_path) as f:
    data = json.load(f)
data["source"]["tap"] = "homebrew/core"
data["source"]["path"] = f"{sys.argv[3]}/Library/Taps/homebrew/homebrew-core/{formula_path}"
data["loaded_from_api"] = True
with open(receipt_path, "w") as f:
    json.dump(data, f, indent=2)
PYEOF

  echo "==> ${name} downgraded to ${installed_version}"
done

echo
echo "Done. 'brew outdated' should now list: ${names}"
echo "Restore with: brew upgrade ${names}"
