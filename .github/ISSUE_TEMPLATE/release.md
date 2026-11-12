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
- [ ] Repos `zeckitoz/homebrew-tap` and `zeckitoz/scoop-bucket` exist and the token has `contents: write` on both

## Release candidate

- [ ] Tag `vX.Y.Z-rc.N` on the commit to release and push it
- [ ] The Release workflow is green and the pre-release has all artefacts and `checksums.txt`
- [ ] The tap and the bucket were **not** updated (tags with `-` are pre-releases)
- [ ] `gh attestation verify` passes for an artefact and for `checksums.txt`
- [ ] Live smoke test against the real Taiga instance

## Manual run with the RC artefacts in real terminals

Run on every system, with the artefacts of the RC:

| Check | macOS | Linux (`.deb`) | Windows Terminal |
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
- [ ] The tap and the bucket were updated: `brew install --cask zeckitoz/tap/taiga-tui` and `scoop install taiga-tui` work
- [ ] README installation instructions still match the release
- [ ] Close this issue
