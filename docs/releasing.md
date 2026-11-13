# Releasing

Tag `vX.Y.Z` to publish.
Tags containing `-`, like `v0.4.0-rc.1`, make a pre-release that the Homebrew tap and the Scoop bucket ignore.
Follow the "Release checklist" issue template for every release.

## Homebrew formula

The tap holds a formula that builds from the source archive of the tag.
GoReleaser does not write it.
The Release workflow renders `packaging/homebrew/taiga-tui.rb.in`, installs the result with `brew` on Linux and macOS, and only then commits it to `zeckitoz/homebrew-tap`.
A failing formula test stops the publish, the GitHub release stays.
Pre-releases skip all of it.
Pull requests that touch `packaging/homebrew` run the same test against the latest release (`formula.yml`).

To run the test yourself, use a container, because it installs `taiga-tui` into the Homebrew it finds:

```sh
docker run --rm -v "$PWD":/src:ro homebrew/brew bash -c '/src/packaging/homebrew/test.sh /path/to/rendered/taiga-tui.rb'
```
