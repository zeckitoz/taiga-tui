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

# CI images ship a stale index and turn auto-update off. A user's brew
# updates before an install, and an old Go would fail the go.mod check.
brew update --quiet

tap=local/formula-test
brew tap-new --no-git "$tap" >/dev/null
trap 'brew uninstall --force taiga-tui >/dev/null 2>&1 || true; brew untap "$tap" >/dev/null 2>&1 || true' EXIT
# Homebrew 7 refuses formulae from taps it was not told to trust. Older
# versions have no such command.
if brew trust --help >/dev/null 2>&1; then
  brew trust "$tap"
fi
cp "$formula" "$(brew --repository "$tap")/Formula/taiga-tui.rb"

brew style "$tap/taiga-tui"
brew audit --strict --formula "$tap/taiga-tui"
brew install --build-from-source "$tap/taiga-tui"
brew test "$tap/taiga-tui"
