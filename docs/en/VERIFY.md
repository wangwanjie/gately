# Verifying release artifacts

Every official Gately release ships with signatures and provenance, so you can confirm that the files you downloaded were built by this repository's GitHub Actions and were not tampered with. `checksums.txt` carries a minisign signature whose public key is built into Gately, and online upgrades use it to verify downloads; the cosign signatures and build provenance use [Sigstore](https://www.sigstore.dev/) keyless signing.

## Binary archives

The release page provides `checksums.txt` (SHA-256 of each archive), `checksums.txt.minisig` (minisign signature of the checksum file), `checksums.txt.sigstore.json` (cosign signature of the checksum file) and an SBOM. Releases published before signing was introduced have no `checksums.txt.minisig`.

1. Verify the signature of `checksums.txt` with [minisign](https://jedisct1.github.io/minisign/). The Gately release public key (key ID `512082B4037581F0`):

```text
RWTwgXUDtIIgUVNxL3scLhMObpyS0vKQbkCepGauqNA0/u0nQA86M50/
```

```bash
minisign -Vm checksums.txt -P RWTwgXUDtIIgUVNxL3scLhMObpyS0vKQbkCepGauqNA0/u0nQA86M50/
```

The output `Signature and comment signature verified` means the signature is valid; the `Trusted comment` line shows the version and signing time.

2. Verify the archive's SHA-256:

```bash
sha256sum -c checksums.txt --ignore-missing
```

3. Verify the signature of `checksums.txt` with [cosign](https://docs.sigstore.dev/cosign/installation/). The signing identity must be this repository's release workflow:

```bash
cosign verify-blob checksums.txt \
  --bundle checksums.txt.sigstore.json \
  --certificate-identity-regexp '^https://github.com/wangwanjie/gately/\.github/workflows/release\.yml@refs/tags/v' \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com
```

4. You can also verify the build provenance of a single archive with the GitHub CLI:

```bash
gh attestation verify gately_v1.0.0_linux_amd64.tar.gz --repo wangwanjie/gately
```

## Signature check during online upgrades

Online upgrades of Linux binary deployments ("Upgrade now") first download `checksums.txt` and `checksums.txt.minisig`, verify the signature with the public key built into the program, and only then compare the SHA-256 of the archive:

- Invalid signature, unknown key or modified `checksums.txt`: the upgrade is refused, the upgrade dialog and the system log show why, and the installed program stays as it is;
- No `checksums.txt.minisig` in the release (published before signing was introduced): the upgrade goes ahead, and the upgrade dialog and the system log warn that the release is unsigned and only its SHA-256 was checked.
- A release of v0.6.8 or later without `checksums.txt.minisig`: every release from v0.6.8 on is signed, so a missing signature file means the release page may have been tampered with, and the online upgrade refuses it. To install it anyway, download it from the release page and use "Upload upgrade package".

The program has a second public key slot for key rotation: a new public key is first built into a release and published on this page, and only later releases are signed with it, so deployed instances keep upgrading. "Upload upgrade package" does not check signatures; verify the archive with the steps above before uploading. Docker image upgrades rely on the cosign image signatures below.

## macOS code signing

The `gately` binary in the macOS archives is signed with an Apple Developer ID certificate (team ID `X6B6C6U6QV`) and notarized by Apple, so it runs right after download and extraction. To inspect the signature:

```bash
codesign -dv --verbose=2 gately
spctl -a -vv -t install gately
```

The output should include `TeamIdentifier=X6B6C6U6QV` and `source=Notarized Developer ID`. On first run, macOS checks the notarization record online.

## Docker image

```bash
cosign verify ghcr.io/wangwanjie/gately:latest \
  --certificate-identity-regexp '^https://github.com/wangwanjie/gately/\.github/workflows/release\.yml@refs/tags/v' \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com
```

`vanjayak/gately` on Docker Hub is the same image with the same signing identity:

```bash
cosign verify docker.io/vanjayak/gately:latest \
  --certificate-identity-regexp '^https://github.com/wangwanjie/gately/\.github/workflows/release\.yml@refs/tags/v' \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com
```

The registry also stores an SBOM and a provenance attestation for the image. Inspect the SBOM with:

```bash
docker buildx imagetools inspect ghcr.io/wangwanjie/gately:latest --format '{{ json .SBOM }}'
```

## SBOM

`gately_<version>_sbom.spdx.json` in the release is in SPDX format. It lists every Go module and frontend dependency, and can be fed to `grype`, `trivy` and similar scanners.
