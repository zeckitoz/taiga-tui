---
name: Release checklist
about: One issue per release, covering the RC flow and the manual verification
title: "Release vX.Y.Z"
labels: release
---

Always work through the full checklist, also for a patch release.

## Before tagging

- [ ] `main` is green on Linux, macOS and Windows
- [ ] `TAP_GITHUB_TOKEN` has not expired: check the expiry of the fine-grained PAT (1 year) and renew it if it ends within the next 30 days
- [ ] Repos `zeckitoz/homebrew-tap`, `zeckitoz/scoop-bucket` and `zeckitoz/linux-repo` exist and the token has `contents: write` on all three
- [ ] The signing subkey in `GPG_PRIVATE_KEY` has not expired: check the expiry in `docs/linux-repository.md` and extend it if it ends within the next 30 days (apt refuses signatures of an expired key)
- [ ] The variable `GPG_SIGNING_KEY` still holds the fingerprint of that subkey

## Release candidate

- [ ] Tag `vX.Y.Z-rc.N` on the commit to release and push it
- [ ] The Release workflow is green and the pre-release has all artefacts and `checksums.txt`
- [ ] The tap, the bucket and `zeckitoz/linux-repo` were **not** updated (tags with `-` are pre-releases)
- [ ] The rpm files of the pre-release are signed: `rpm -K` on a downloaded file reports the signature (after importing `taiga-tui.asc`)
- [ ] `gh attestation verify` passes for an artefact and for `checksums.txt`
- [ ] Live smoke test against the real Taiga instance: `go test -tags live ./internal/taiga -run TestLiveSmoke -v` (see `docs/test-project.md`), and it leaves the test project as it was

## Manual run with the RC artefacts in real terminals

Run on every system, with the artefacts of the RC:

| Check | macOS | Linux (`.deb` file) | Windows Terminal |
|---|---|---|---|
| Install from the artefact (macOS: direct download with `xattr -d com.apple.quarantine`; Windows: SmartScreen "Run anyway") | [ ] | [ ] | [ ] |
| `taiga-tui --version` shows the RC version | [ ] | [ ] | [ ] |
| Login | [ ] | [ ] | [ ] |
| Kanban Board | [ ] | [ ] | [ ] |
| Taskboard | [ ] | [ ] | [ ] |
| Status Change | [ ] | [ ] | [ ] |
| Take Over | [ ] | [ ] | [ ] |
| Conflict from two windows | [ ] | [ ] | [ ] |
| Too small window | [ ] | [ ] | [ ] |
| Update hint (needs an older build, a newer release exists) | [ ] | [ ] | [ ] |

- [ ] Problems found: fix, tag `-rc.N+1` and repeat the whole section

## Release

- [ ] Tag `vX.Y.Z` on the **last RC commit** and push it
- [ ] The Release workflow is green
- [ ] The GitHub release has generated notes and all artefacts
- [ ] `gh attestation verify` passes for the final artefacts
- [ ] The formula jobs of the Release workflow are green and the tap and the bucket were updated: `brew install zeckitoz/tap/taiga-tui` and `scoop install taiga-tui` work
- [ ] The `linux-repo` job of the Release workflow is green and `zeckitoz/linux-repo` has a commit for the release (GitHub Pages deploy finished)
- [ ] The fingerprint of `curl -fsSL https://zeckitoz.github.io/linux-repo/taiga-tui.asc | gpg --show-keys` matches the one in `docs/linux-repository.md`
- [ ] In a clean Debian or Ubuntu container the apt install from the README works and `taiga-tui --version` shows the release
- [ ] In a clean Fedora container the dnf install from the README works (signature check on) and `taiga-tui --version` shows the release
- [ ] For a release after the first one: `apt upgrade` and `dnf upgrade` pick up the new version from the previous one
- [ ] README installation instructions still match the release
- [ ] Close this issue
