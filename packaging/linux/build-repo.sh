#!/usr/bin/env bash
# Adds release packages to the apt and rpm repository and signs the metadata.
#
# Usage: build-repo.sh <repo-dir> <packages-dir>
#
# <repo-dir> is a checkout of the hosting repository (GitHub Pages). It keeps
# every released package, so the metadata is rebuilt from the files and needs
# no database. <packages-dir> holds the new *.deb and *.rpm files.
#
# Needs apt-ftparchive (apt-utils), createrepo_c, gzip and gpg on the PATH. The
# signing key must already be in the keyring of GNUPGHOME, and
# GPG_SIGNING_KEY names it (the fingerprint of the signing subkey).
#
# Layout:
#   pool/main/t/taiga-tui/*.deb          packages
#   dists/stable/...                     apt metadata, InRelease is signed
#   rpm/*.rpm, rpm/repodata/...          rpm packages and metadata,
#                                        repomd.xml.asc is signed
#   taiga-tui.sources, taiga-tui.repo    ready-made config for apt and dnf, so
#                                        users download one file each
#
# REPO_URL is the public URL of the repository, default GitHub Pages.
set -euo pipefail

if [[ $# -ne 2 ]]; then
  echo "usage: $0 <repo-dir> <packages-dir>" >&2
  exit 2
fi
: "${GPG_SIGNING_KEY:?set GPG_SIGNING_KEY to the fingerprint of the signing subkey}"

repo=$(cd "$1" && pwd)
pkgs=$(cd "$2" && pwd)
archs=(amd64 arm64)
base=${REPO_URL:-https://zeckitoz.github.io/linux-repo}
suite=stable

# The trailing ! makes gpg use exactly this subkey.
sign=(gpg --batch --yes --pinentry-mode loopback --local-user "${GPG_SIGNING_KEY}!")

mkdir -p "$repo/pool/main/t/taiga-tui" "$repo/rpm"
shopt -s nullglob
debs=("$pkgs"/*.deb)
rpms=("$pkgs"/*.rpm)
if ((${#debs[@]} == 0 || ${#rpms[@]} == 0)); then
  echo "no .deb or no .rpm in $pkgs" >&2
  exit 1
fi
cp -f "${debs[@]}" "$repo/pool/main/t/taiga-tui/"
cp -f "${rpms[@]}" "$repo/rpm/"

cd "$repo"

# apt: Packages per architecture, then a signed Release.
rm -rf "dists/$suite"
for arch in "${archs[@]}"; do
  dir="dists/$suite/main/binary-$arch"
  mkdir -p "$dir"
  apt-ftparchive --arch "$arch" packages pool >"$dir/Packages"
  gzip -9nk "$dir/Packages"
done
apt-ftparchive \
  -o "APT::FTPArchive::Release::Origin=taiga-tui" \
  -o "APT::FTPArchive::Release::Label=taiga-tui" \
  -o "APT::FTPArchive::Release::Suite=$suite" \
  -o "APT::FTPArchive::Release::Codename=$suite" \
  -o "APT::FTPArchive::Release::Architectures=${archs[*]}" \
  -o "APT::FTPArchive::Release::Components=main" \
  release "dists/$suite" >"dists/$suite/Release"
"${sign[@]}" --clearsign --output "dists/$suite/InRelease" "dists/$suite/Release"
"${sign[@]}" --armor --detach-sign --output "dists/$suite/Release.gpg" "dists/$suite/Release"

# rpm: repodata for all packages, then a signed repomd.xml.
rm -rf rpm/repodata
createrepo_c --quiet rpm
"${sign[@]}" --armor --detach-sign --output rpm/repodata/repomd.xml.asc rpm/repodata/repomd.xml

# Ready-made config. The apt file carries the public key inline in Signed-By
# (deb822 format): an empty line of the armor becomes " .".
key="$repo/taiga-tui.asc"
[[ -f "$key" ]] || { echo "missing $key" >&2; exit 1; }
{
  cat <<EOF
Types: deb
URIs: $base
Suites: $suite
Components: main
Signed-By:
EOF
  sed -e 's/^$/./' -e 's/^/ /' "$key"
} >taiga-tui.sources
cat >taiga-tui.repo <<EOF
[taiga-tui]
name=taiga-tui
baseurl=$base/rpm
enabled=1
gpgcheck=1
repo_gpgcheck=1
gpgkey=$base/taiga-tui.asc
EOF
