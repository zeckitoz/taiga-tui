# Linux repository

The apt and rpm repository lives in [zeckitoz/linux-repo](https://github.com/zeckitoz/linux-repo) and is served by GitHub Pages at `https://zeckitoz.github.io/linux-repo/`.
Nobody edits it by hand.
The Release workflow rebuilds it for every stable tag, after the GitHub release exists.
Tags with a `-` (pre-releases) skip it.

> [!IMPORTANT]
> Both repositories are signed with the primary key `15C97F1D1B65AFBEC60B8E302DD359181160D21C`.
> Check the key before you trust it: `curl -fsSL https://zeckitoz.github.io/linux-repo/taiga-tui.asc | gpg --show-keys`.
> You may be asked to confirm the import of the key during installation.

## How it works

The job `linux-repo` in `.github/workflows/release.yml` downloads the `.deb` and `.rpm` files of the release and runs `packaging/linux/build-repo.sh`.
The script copies them into the repository checkout, which keeps every released package, and rebuilds the metadata from the files.
It needs no database.

| Path | Content |
| --- | --- |
| `pool/main/t/taiga-tui/*.deb` | packages |
| `dists/stable/` | apt metadata, `InRelease` and `Release.gpg` are signed |
| `rpm/*.rpm`, `rpm/repodata/` | rpm packages and metadata, `repomd.xml.asc` is signed |
| `taiga-tui.asc` | the public key |
| `taiga-tui.sources`, `taiga-tui.repo` | ready-made config for apt and dnf, written by the script |

apt trusts the signed `Release` file, which names the hash of every metadata file and, through them, every package.
dnf checks the signature of the metadata (`repo_gpgcheck=1`) and of every package (`gpgcheck=1`).
The rpm files are signed by GoReleaser (nfpm) inside the release run, so `checksums.txt`, the cosign signature and the attestations cover the signed files.
The deb files stay unsigned, as apt does not need it.

## Config files

The README tells users to download one file each: `taiga-tui.sources` for apt and `taiga-tui.repo` for dnf.
The script writes both on every run, so the key in the apt file always matches `taiga-tui.asc`.
The apt file has the key inline in `Signed-By`, which apt supports from 2.4 (Debian 12, Ubuntu 22.04).
Older apt, for example Ubuntu 20.04, rejects it with `Invalid value set for option Signed-By`.
Use the long form there:

```sh
sudo install -d -m 0755 /etc/apt/keyrings
sudo curl -fsSL https://zeckitoz.github.io/linux-repo/taiga-tui.asc -o /etc/apt/keyrings/taiga-tui.asc
echo "deb [signed-by=/etc/apt/keyrings/taiga-tui.asc] https://zeckitoz.github.io/linux-repo stable main" | sudo tee /etc/apt/sources.list.d/taiga-tui.list
sudo apt update && sudo apt install taiga-tui
```

## Keys

| Key | Where | Expires |
| --- | --- | --- |
| Primary key `15C97F1D1B65AFBEC60B8E302DD359181160D21C`, certify only | offline, in an encrypted disk image (iCloud Drive and a USB stick) | 2028-11-15 |
| Signing subkey `F11CFE312288CC6BDB9694F3EE39A795452C0B4A` | GitHub secret `GPG_PRIVATE_KEY` | 2027-11-16 |

Both are RSA 4096.
RSA, because RHEL 8 and older tools do not read Ed25519 reliably.

The secret `GPG_PRIVATE_KEY` holds the signing subkey only, without the primary key and without a passphrase.
nfpm cannot unlock a passphrase-protected subkey, and a passphrase stored next to the key in the same secret store protects little.
The variable `GPG_SIGNING_KEY` holds the fingerprint of the subkey.
The public key in the repository is the primary key with its subkey.
The signatures name the primary key as the signer.

## Rotation

Put a reminder in the calendar one month before each expiry.
Do the work in a throwaway keyring with a short path, for example `GNUPGHOME=~/gpg-ci/home`, and delete it afterwards.
Import `primary-backup.asc` from the disk image first.

### Extend the signing subkey (yearly)

```sh
gpg --quick-set-expire <primary-fingerprint> 1y <subkey-fingerprint>
gpg --armor --export <primary-fingerprint> > taiga-tui.asc
gpg --change-passphrase <primary-fingerprint>   # enter the old passphrase, then an empty new one
gpg --armor --export-secret-subkeys <subkey-fingerprint>! > ci-subkey.asc
```

Check with `gpg --list-packets ci-subkey.asc | grep iter` that the subkey has no protection left: nothing may match.
Commit the new `taiga-tui.asc` to `linux-repo` and update the secret with `gh secret set GPG_PRIVATE_KEY --repo zeckitoz/taiga-tui < ci-subkey.asc`.
Do this before the expiry, because apt refuses signatures of an expired key.
Users refresh `/etc/apt/keyrings/taiga-tui.asc` once.
dnf users re-import the key if dnf complains.

### Replace a leaked signing subkey

1. Revoke the subkey with the primary key: `gpg --edit-key <primary-fingerprint>`, `key 1`, `revkey`.
2. Add a new subkey with `gpg --quick-add-key <primary-fingerprint> rsa4096 sign 1y`.
3. Export the public key and the new unprotected subkey as above.
4. Update the secret and the variable `GPG_SIGNING_KEY`, commit `taiga-tui.asc`, and tag a release so that the metadata is signed again.
5. Say in the release notes that users should refresh the key file.

### Replace the primary key

Only if the primary key leaks or is lost.
Create a new key pair as in the first setup, publish the new fingerprint in the README and in this file, and tell users to replace the key file.
Every user has to act once, so protect the backup of the primary key.

## Size

Every release adds about 16 MB.
GitHub Pages recommends at most 1 GB per repository.
If the repository approaches this, delete the oldest files from `pool/` and `rpm/`, let the next release rebuild the metadata, and push.

## Try it locally

`packaging/linux/build-repo.sh` needs `apt-ftparchive`, `createrepo_c` and `gpg`.
Run it in a container, so that it does not touch your keyring, with a throwaway key and a snapshot build (`goreleaser release --snapshot --skip=sbom,publish` with `RPM_SIGNING_KEY_FILE` set, and without `--skip=sign`, which also skips the rpm signature).
Then add the result as a `file://` repository in clean Debian and Fedora containers.
