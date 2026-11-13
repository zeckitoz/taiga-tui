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

## Signature of checksums.txt

`checksums.txt` is also signed with [cosign](https://github.com/sigstore/cosign) keyless signing.
The release has the signature bundle `checksums.txt.sigstore.json` next to it.
The signing identity is the release workflow of this repository.

```sh
cosign verify-blob checksums.txt \
  --bundle checksums.txt.sigstore.json \
  --certificate-identity-regexp '^https://github.com/zeckitoz/taiga-tui/\.github/workflows/release\.yml@refs/tags/v' \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com
```

Together with `sha256sum --check` above, this ties every artefact to the workflow run that built it.

## SBOM

Every archive has an SPDX SBOM next to it, named `<archive>.sbom.json`.
It lists the Go modules compiled into the binary.
The SBOM files are listed in `checksums.txt`, so the checks above cover them too.
