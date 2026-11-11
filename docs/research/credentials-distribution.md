# Credentials, configuration and distribution of a Go CLI

Research for issue #6 (part of the MVP map #1).
Question: How does a Go CLI store tokens securely on macOS, Linux and Windows, where does the config file live, and how is the tool distributed to the team when the GitHub repo is private?

Sources were read on 2026-11-11.
Source code was read at these commits: zalando/go-keyring `66b55cc` (tag v0.2.8), 99designs/keyring `master` (last commit 2022-12-19), taigaio/taiga-back `8ab6a4e`.

## Recommendation (short)

- **Secrets:** `github.com/zalando/go-keyring`, wrapped with a timeout, plus a plain-text fallback file (mode 0600) in the config dir when no credential store is available.
  This is exactly what the GitHub CLI does.
- **What to store:** only the Taiga refresh token (and optionally the current auth token), never the university password.
- **Config:** `os.UserConfigDir()/taiga-tui/config.toml` (non-secret settings: instance URL, username, last project).
- **Build:** pure Go, `CGO_ENABLED=0`, cross-compiled by GoReleaser in GitHub Actions, published as GitHub Release assets on the private repo.
- **Install for the team:** `gh release download -R zeckitoz/taiga-tui -p '<os>_<arch>' ...` as the primary path, `go install` with `GOPRIVATE` for team members who have Go installed.
  Skip Homebrew tap and Scoop while the repo is private.

## 1. Storing tokens

### zalando/go-keyring (recommended)

- Supports macOS, Linux/BSD (D-Bus Secret Service) and Windows; the README states it "aims to simplify using statically linked binaries, which is cumbersome when relying on C bindings (as other keyring libraries do)".
  Source: https://github.com/zalando/go-keyring/blob/master/README.md
- No CGo on any platform:
  - macOS shells out to `/usr/bin/security` (`keyring_darwin.go`, `execPathKeychain = "/usr/bin/security"`).
    https://github.com/zalando/go-keyring/blob/master/keyring_darwin.go
  - Linux talks to the Secret Service over D-Bus via the pure-Go `github.com/godbus/dbus/v5` (`keyring_unix.go`, `secret_service/secret_service.go`).
  - Windows uses Credential Manager via `github.com/danieljoos/wincred`, which contains no `import "C"` (checked in its source).
  - Therefore GoReleaser can cross-compile all targets from one Linux runner with `CGO_ENABLED=0`.
- Actively maintained: v0.2.8 released 2026-03-23, last push 2026-10-16 (GitHub API).
- The GitHub CLI depends on `github.com/zalando/go-keyring v0.2.8` (https://github.com/cli/cli/blob/trunk/go.mod) and wraps every call in a 60 s timeout because keyring calls can block (https://github.com/cli/cli/blob/trunk/internal/keyring/keyring.go).
- Size limits (`ErrSetDataTooBig`, https://github.com/zalando/go-keyring/blob/master/keyring.go):
  - macOS: service + user + password should not exceed about 3000 bytes (the `security -i` command line is capped at 4096 bytes, and the secret is base64-encoded before writing).
  - Windows: password max 2560 bytes (`keyring_windows.go`).
  - Linux: no hard limit.
  - Taiga tokens are JWTs (taiga-back uses a simplejwt fork, `taiga/auth/tokens.py`), a few hundred bytes each, so store auth token and refresh token as separate entries rather than one JSON blob to stay far below the Windows limit.
- Tests: `keyring.MockInit()` swaps in an in-memory provider (`keyring_mock.go`), so tests never touch the real store.
- Errors: `ErrNotFound` when no entry exists; `ErrUnsupportedPlatform` on platforms without a provider (`keyring_fallback.go`).

### Linux without Secret Service (WSL, SSH, headless)

- On Linux, every call first opens `dbus.SessionBus()` (`secret_service.NewSecretService`), then uses the `login` collection, falling back to the `default` alias (`GetLoginCollection`), and calls `Unlock` on it.
  https://github.com/zalando/go-keyring/blob/master/secret_service/secret_service.go
- Consequences:
  - No D-Bus session bus (typical for WSL2 without a desktop, SSH sessions, containers): `Set`/`Get` return the D-Bus connection error. There is no built-in fallback.
  - Session bus present but no Secret Service provider (no gnome-keyring / KWallet running): the D-Bus call fails with an error.
  - Locked keyring: `Unlock` can trigger a GUI prompt and block; this is why the GitHub CLI adds a timeout.
- The README expects a `login` collection to exist and explains how to create it with Seahorse.
- The tool must treat any error other than `ErrNotFound` as "credential store unavailable" and fall back.

### Fallback precedent: GitHub CLI

- `gh auth login` stores the token "securely in the system credential store"; if no credential store is found or there is a problem using it, gh falls back to a plain-text file, and `--insecure-storage` forces plain text.
  https://cli.github.com/manual/gh_auth_login
- Recommended for taiga-tui: same behaviour.
  Fallback file `os.UserConfigDir()/taiga-tui/credentials` (or a section in a separate file, never in `config.toml`, so the config can be shared or committed), created with mode 0600 in a 0700 directory.
  Tell the user once that the plain-text fallback is in use, and offer a `--insecure-storage` flag / config option to choose it explicitly.

### 99designs/keyring (not recommended)

- Supports more backends (Keychain, Credential Manager, Secret Service, KWallet, pass, encrypted file, keyctl).
  https://github.com/99designs/keyring
- But: the macOS Keychain backend is behind `//go:build darwin && cgo` (`keychain.go`), so a `CGO_ENABLED=0` cross-compiled darwin binary silently loses Keychain support; GoReleaser would need macOS runners or a C cross toolchain.
- Last commit on `master` 2022-12-19, last push 2024-05-07, latest tag v1.2.2 (GitHub API): effectively unmaintained.
- Its encrypted file backend needs a passphrase prompt on every start, which is worse UX than the plain-text-0600 fallback for a team tool.

### Taiga token behaviour that affects storage

- taiga-back project settings: `ACCESS_TOKEN_LIFETIME = 24h`, `REFRESH_TOKEN_LIFETIME = 8 days`, and `ROTATE_REFRESH_TOKENS` is not overridden.
  https://github.com/taigaio/taiga-back/blob/main/settings/common.py
- The library default is `ROTATE_REFRESH_TOKENS: True`, `DENYLIST_AFTER_ROTATION: True` (`DEFAULTS` in https://github.com/taigaio/taiga-back/blob/main/taiga/auth/settings.py; the docstring example in the same file shows `False`, which is misleading).
- `refresh_token()` in https://github.com/taigaio/taiga-back/blob/main/taiga/auth/services.py therefore denylists the submitted refresh token and returns a new `refresh` with a renewed expiry.
- Consequences for taiga-tui (assuming the Ostfalia instance uses stock settings; ticket #2 verifies the actual auth flow):
  - After every refresh, the new refresh token must be persisted immediately; losing it means re-login.
  - Two concurrently running taiga-tui instances that refresh at the same time will invalidate each other's refresh token; use a lock file or accept a re-login in that case.
  - A session survives as long as the tool is used at least once every 8 days; storing the password is not needed.

## 2. Configuration file location

- `os.UserConfigDir()` returns `$XDG_CONFIG_HOME` or `$HOME/.config` on Unix, `$HOME/Library/Application Support` on macOS, and `%AppData%` on Windows; it errors if `$HOME` is undefined.
  Callers should create their own application-specific subdirectory.
  https://pkg.go.dev/os#UserConfigDir
- `os.UserCacheDir()` (for cached project lists etc.): `$XDG_CACHE_HOME` or `$HOME/.cache`, `$HOME/Library/Caches`, `%LocalAppData%`.
  https://pkg.go.dev/os#UserCacheDir
- Pitfall: on macOS `os.UserConfigDir()` ignores `XDG_CONFIG_HOME`, and many terminal users expect `~/.config/taiga-tui`.
  Options: accept the standard library behaviour (simplest, recommended), or check `$XDG_CONFIG_HOME` first on all platforms.
  Either way, print the resolved path in `--help` or a `config path` command.
- Allow overriding via a `--config` flag and an environment variable (e.g. `TAIGA_TUI_CONFIG`) for testing.

## 3. Distribution with a private repository

Repo visibility checked via GitHub API: `zeckitoz/taiga-tui` is private, user-owned.
Every team member who installs the tool therefore needs read access to the repo (collaborator) and a GitHub credential on their machine.

### GoReleaser plus GitHub Releases (recommended build pipeline)

- GoReleaser builds archives for darwin/linux/windows x amd64/arm64 and uploads them to a GitHub Release; in GitHub Actions the built-in `GITHUB_TOKEN` is enough for the same repo.
  https://goreleaser.com/customization/builds/go/ and https://goreleaser.com/ci/actions/
- Because the code is pure Go (see section 1), one `ubuntu-latest` job with `CGO_ENABLED=0` covers all targets.
- Private repo: Actions minutes count against the account's included quota (https://docs.github.com/en/billing/managing-billing-for-your-products/about-billing-for-github-actions); a release build on Linux takes a few minutes, so this is negligible.

### Installing from the private release: `gh release download`

- `gh release download [<tag>] -p <glob> -R OWNER/REPO -D <dir>` downloads matching assets of the latest (or given) release.
  https://cli.github.com/manual/gh_release_download
- `gh` uses the user's existing GitHub login, so private assets work without extra token handling.
- Example: `gh release download -R zeckitoz/taiga-tui -p '*darwin_arm64.tar.gz'`, then unpack and put the binary on `PATH`.
  A small documented one-liner per OS in the README is enough for a student team.

### `go install`

- Works for team members with a Go toolchain: `GOPRIVATE=github.com/zeckitoz/* go install github.com/zeckitoz/taiga-tui/cmd/taiga-tui@latest`.
- `GOPRIVATE` makes the go command skip `proxy.golang.org` and `sum.golang.org` for matching modules and fetch directly from Git.
  https://go.dev/ref/mod#private-modules
- Git needs credentials for `https://github.com`: either a `~/.netrc` line with a personal access token, or `url."ssh://git@github.com/".insteadOf = https://github.com/`.
  https://go.dev/doc/faq#git_https
- Running `gh auth setup-git` also configures Git to use the gh credential helper (https://cli.github.com/manual/gh_auth_setup-git).
- Pitfall: binaries built this way have no version info injected by `-ldflags`, unless the code reads `runtime/debug.ReadBuildInfo()` (which reports the module version for `go install pkg@version`).

### Homebrew tap (not recommended while private)

- GoReleaser now generates casks (`homebrew_casks`, since v2.10); the formula-based `brews` section is listed as deprecated.
  https://goreleaser.com/customization/homebrew_casks/
- For private repositories the docs require a custom `CurlDownloadStrategy` subclass in `custom_block`, and since Homebrew 5.1.14 the token can no longer be read in `url.template`/`url.headers`; users still have to export `HOMEBREW_GITHUB_API_TOKEN`.
- The tap repo itself would also have to be accessible to every user.
- Casks are expected to be signed and notarized; otherwise the docs suggest an `xattr` post-install hook to remove the quarantine flag, with the warning that this circumvents Gatekeeper.
- Too many moving parts for a handful of users; revisit if the repo becomes public.

### Scoop (not recommended while private)

- GoReleaser's `scoop` section is still supported in v2, but its docs only cover a token for publishing the manifest; `url_template` assumes a publicly downloadable URL and nothing covers authenticated downloads.
  https://goreleaser.com/customization/scoop/
- Scoop manifests therefore cannot fetch assets from a private release.

### If the repo were made public

All of the above becomes trivial: direct download links, `go install` without `GOPRIVATE`/credentials, a public Homebrew tap and Scoop bucket generated by GoReleaser.
This is a decision for the team, not a technical constraint.

## 4. Pitfalls checklist

- Do not use 99designs/keyring with `CGO_ENABLED=0`: macOS loses Keychain support silently.
- Wrap keyring calls in a timeout (Linux unlock prompts can block).
- Treat non-`ErrNotFound` keyring errors as "store unavailable" and fall back to a 0600 file, with a visible notice.
- Keep secrets out of `config.toml`.
- Persist the rotated refresh token after every refresh; guard against concurrent refreshes.
- Store auth and refresh token as separate keyring entries (Windows 2560-byte limit).
- macOS: `os.UserConfigDir()` is `~/Library/Application Support`, not `~/.config`.
- Unsigned binaries: macOS Gatekeeper blocks binaries that carry the quarantine attribute (typically set by browser downloads; remove with `xattr -d com.apple.quarantine <binary>`), and Windows SmartScreen may warn on downloaded `.exe` files.
  Installing via `gh release download` or `go install` avoids most of this in practice.
- Every installer needs read access to the private repo.
