#!/usr/bin/env bash
# Renders the Homebrew formula for a release tag and prints it to stdout.
#
# Usage: render.sh <tag> <commit> <date>
#   tag     the release tag, for example v0.2.0
#   commit  the full commit hash the tag points to
#   date    the commit date as RFC 3339 UTC, for example 2026-11-13T11:18:46Z
#
# The sha256 is taken from the GitHub source archive of the tag, so the tag
# must already be pushed.
set -euo pipefail

if [[ $# -ne 3 ]]; then
  echo "usage: $0 <tag> <commit> <date>" >&2
  exit 2
fi
tag=$1 commit=$2 date=$3
version=${tag#v}
if [[ ! $version =~ ^[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
  echo "render.sh: $tag is not a stable vX.Y.Z tag" >&2
  exit 2
fi

here=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
url="https://github.com/zeckitoz/taiga-tui/archive/refs/tags/${tag}.tar.gz"
if command -v sha256sum >/dev/null; then
  sha256=$(curl -fsSL "$url" | sha256sum | cut -d' ' -f1)
else
  sha256=$(curl -fsSL "$url" | shasum -a 256 | cut -d' ' -f1)
fi

sed \
  -e "s|@VERSION@|${version}|g" \
  -e "s|@SHA256@|${sha256}|g" \
  -e "s|@COMMIT@|${commit}|g" \
  -e "s|@DATE@|${date}|g" \
  "$here/taiga-tui.rb.in"
