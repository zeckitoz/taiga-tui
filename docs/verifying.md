# Verifying a download

Every release has a `checksums.txt` with the SHA-256 sums of all artefacts.

```sh
sha256sum --check --ignore-missing checksums.txt
```

On macOS use `shasum -a 256 --check --ignore-missing checksums.txt`.

The artefacts and `checksums.txt` carry GitHub Artifact Attestations, which prove they were built from this repository by the release workflow.
Verify them with the [GitHub CLI](https://cli.github.com):

```sh
gh attestation verify taiga-tui_*_linux_amd64.tar.gz --repo zeckitoz/taiga-tui
gh attestation verify checksums.txt --repo zeckitoz/taiga-tui
```
