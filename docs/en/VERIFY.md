# Verifying release artifacts

Every official Gately release ships with signatures and provenance, so you can confirm that the files you downloaded were built by this repository's GitHub Actions and were not tampered with. All signatures use [Sigstore](https://www.sigstore.dev/) keyless signing, so there is no private key to protect.

## Binary archives

The release page provides `checksums.txt` (SHA-256 of each archive), `checksums.txt.sigstore.json` (the signature of the checksum file) and an SBOM.

1. Verify the archive's SHA-256:

```bash
sha256sum -c checksums.txt --ignore-missing
```

2. Verify the signature of `checksums.txt` with [cosign](https://docs.sigstore.dev/cosign/installation/). The signing identity must be this repository's release workflow:

```bash
cosign verify-blob checksums.txt \
  --bundle checksums.txt.sigstore.json \
  --certificate-identity-regexp '^https://github.com/wangwanjie/gately/\.github/workflows/release\.yml@refs/tags/v' \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com
```

3. You can also verify the build provenance of a single archive with the GitHub CLI:

```bash
gh attestation verify gately_v1.0.0_linux_amd64.tar.gz --repo wangwanjie/gately
```

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
