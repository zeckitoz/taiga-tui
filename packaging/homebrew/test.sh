#!/usr/bin/env bash
# Checks a rendered formula the way a user would meet it: style, strict audit,
# a build from source and the formula's own test block.
#
# Usage: test.sh <formula.rb>
#
# Needs `brew` on the PATH. It installs into the Homebrew of the machine, so
# run it in CI or a container, not next to a taiga-tui you use.
set -euo pipefail

if [[ $# -ne 1 ]]; then
  echo "usage: $0 <formula.rb>" >&2
  exit 2
fi
formula=$(cd "$(dirname "$1")" && pwd)/$(basename "$1")

tap=local/formula-test
brew tap-new --no-git "$tap" >/dev/null
trap 'brew uninstall --force taiga-tui >/dev/null 2>&1 || true; brew untap "$tap" >/dev/null 2>&1 || true' EXIT
cp "$formula" "$(brew --repository "$tap")/Formula/taiga-tui.rb"

brew style "$tap/taiga-tui"
brew audit --strict --formula "$tap/taiga-tui"
brew install --build-from-source "$tap/taiga-tui"
brew test "$tap/taiga-tui"
