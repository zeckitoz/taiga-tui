# taiga-tui

A keyboard-driven terminal UI for Taiga Taskboards and Kanban Boards.

## Installation

### Homebrew (macOS, Linux)

```sh
brew install --cask zeckitoz/tap/taiga-tui
```

The cask removes the macOS quarantine flag after the install, so the binary starts without a warning.

### Scoop (Windows)

```powershell
scoop bucket add zeckitoz https://github.com/zeckitoz/scoop-bucket
scoop install taiga-tui
```

### Debian, Ubuntu and Fedora packages

Download the `.deb` or `.rpm` for your architecture from the [latest release](https://github.com/zeckitoz/taiga-tui/releases/latest).

```sh
sudo dpkg -i taiga-tui_*_linux_amd64.deb   # Debian, Ubuntu
sudo rpm -i taiga-tui_*_linux_amd64.rpm    # Fedora, RHEL
```

### Direct download

Download the archive for your system from the [latest release](https://github.com/zeckitoz/taiga-tui/releases/latest), unpack it and put `taiga-tui` on your `PATH`.
The binaries are not signed, so the systems warn about them:

- **macOS**: remove the quarantine flag once with `xattr -d com.apple.quarantine taiga-tui`.
- **Windows**: SmartScreen shows "Windows protected your PC".
  Choose "More info", then "Run anyway".

Prefer Homebrew or Scoop, which avoid both warnings.

### From source

```sh
go install github.com/zeckitoz/taiga-tui/cmd/taiga-tui@latest
```

## Verifying a download

Every release has a `checksums.txt` with the SHA-256 sums of all artefacts.

```sh
sha256sum --check --ignore-missing checksums.txt
```

The artefacts and `checksums.txt` carry GitHub Artifact Attestations, which prove they were built from this repository by the release workflow.
Verify them with the [GitHub CLI](https://cli.github.com):

```sh
gh attestation verify taiga-tui_*_linux_amd64.tar.gz --repo zeckitoz/taiga-tui
gh attestation verify checksums.txt --repo zeckitoz/taiga-tui
```

## Usage

Run `taiga-tui --help` for the flags and the location of the config directory.

## Update hint

At most once a day the app asks GitHub for the latest release in the background.
If a newer version exists, the status line says so: `vX.Y.Z available - see github.com/zeckitoz/taiga-tui/releases`.
The check never blocks the start and stays silent when it fails.
Pre-releases and development builds are ignored.

To turn it off, set this in `config.toml` in the config directory:

```toml
check_updates = false
```

## Releasing

Tag `vX.Y.Z` to publish.
Tags containing `-`, like `v0.4.0-rc.1`, make a pre-release that the Homebrew tap and the Scoop bucket ignore.
Follow the "Release checklist" issue template for every release.

## License

MIT, see [LICENSE](LICENSE).
