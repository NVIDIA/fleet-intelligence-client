# Verify release artifacts

Fleet Intelligence releases include a SHA-256 checksum manifest, a detached
OpenPGP signature for that manifest, and the Fleet Intelligence public signing
key. Use these assets to authenticate a release before installing it.

The standard installation scripts verify each downloaded archive against
`checksums.txt`.

## Choose verification for your platform

### Linux

Linux does not provide a universal native code-signing and notarization
mechanism for downloaded executables. The Linux installer verifies the archive
checksum, but it does not authenticate the checksum manifest.

For publisher-authenticated verification, follow the
[signed checksum manifest procedure](#verify-the-signed-checksum-manifest)
before extracting or installing the archive.

### macOS

The macOS installer automatically verifies:

1. The downloaded archive against `checksums.txt`.
2. The extracted binary's Developer ID signature.
3. The binary's Apple notarization ticket.

No additional manual verification is required when using the macOS installer.

### Windows

The Windows PowerShell installer automatically verifies:

1. The downloaded archive against `checksums.txt`.
2. The extracted executable's Authenticode signature using the Windows
   certificate trust store.

No additional manual verification is required when using the Windows
installer. GPG is not required on Windows.

## Verify the signed checksum manifest

The following procedure is for Linux release archives.

### Prerequisites

Install GnuPG, which provides `gpg` and `gpgv`.

On Debian or Ubuntu:

```bash
sudo apt-get install gnupg
```

On Fedora or RHEL:

```bash
sudo dnf install gnupg2
```

You also need `curl` and `sha256sum`.

### Download a release

Set the release version and archive name for your Linux architecture. This
example uses AMD64:

```bash
VERSION=v1.0.0
ASSET=nvfleetint_1.0.0_linux_amd64.tar.gz
BASE_URL="https://github.com/NVIDIA/fleet-intelligence-client/releases/download/${VERSION}"

curl -fLO "${BASE_URL}/${ASSET}"
curl -fLO "${BASE_URL}/checksums.txt"
curl -fLO "${BASE_URL}/checksums.txt.asc"
curl -fLO "${BASE_URL}/fleet-intelligence.pub.asc"
```

Linux release archives use these architecture and extension values:

| Platform | Archive name suffix |
|---|---|
| Linux AMD64 | `linux_amd64.tar.gz` |
| Linux ARM64 | `linux_arm64.tar.gz` |

### Verify the signing key

The expected Fleet Intelligence signing-key fingerprint is:

```text
FE0C 8B74 CA66 357C 13BE 197D CCE3 C963 0871 99B3
```

Inspect the downloaded key in an isolated temporary GnuPG home and require it
to contain exactly one primary key with the expected fingerprint:

```bash
EXPECTED_FINGERPRINT="FE0C8B74CA66357C13BE197DCCE3C963087199B3"
VERIFY_HOME="$(mktemp -d)"
chmod 700 "$VERIFY_HOME"

KEY_FINGERPRINTS="$(
  gpg --batch \
    --homedir "$VERIFY_HOME" \
    --show-keys \
    --with-colons \
    fleet-intelligence.pub.asc |
    awk -F: '
      $1 == "pub" { expect_fingerprint = 1; next }
      expect_fingerprint && $1 == "fpr" {
        print $10
        expect_fingerprint = 0
      }
    '
)"

if [ "$KEY_FINGERPRINTS" != "$EXPECTED_FINGERPRINT" ]; then
  echo "Unexpected Fleet Intelligence signing key fingerprint:" >&2
  printf '%s\n' "${KEY_FINGERPRINTS:-<none>}" >&2
  exit 1
fi
```

Do not continue if the command reports an unexpected, missing, or additional
primary key.

### Verify the checksum manifest signature

After the exact single-key check succeeds, create a temporary keyring from the
verified public key and verify the detached signature:

```bash
gpg --batch \
  --homedir "$VERIFY_HOME" \
  --yes \
  --dearmor \
  --output "$VERIFY_HOME/fleet-intelligence.gpg" \
  fleet-intelligence.pub.asc

gpgv \
  --keyring "$VERIFY_HOME/fleet-intelligence.gpg" \
  checksums.txt.asc \
  checksums.txt
```

`gpgv` must report a good signature. Do not use the checksum manifest if
signature verification fails.

### Verify the release archive

On Linux:

```bash
awk -v asset="$ASSET" '$2 == asset || $2 == "*" asset' checksums.txt |
  sha256sum --check
```

The command must report that the archive is `OK`. You can then extract and
install the verified archive.

Remove the temporary keyring when verification is complete:

```bash
rm -rf "$VERIFY_HOME"
```

## What this verifies

The archive checksum detects changes to the downloaded archive. Verifying
`checksums.txt.asc` establishes that the checksum manifest was signed with the
Fleet Intelligence release-signing key whose fingerprint is pinned above.

The macOS and Windows installers use their native platform-signature checks
instead of this manual procedure.
