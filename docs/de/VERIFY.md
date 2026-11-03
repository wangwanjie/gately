# Releases verifizieren

Jedes offizielle Gately-Release wird mit Signaturen und Provenienz ausgeliefert. So können Sie prüfen, dass die heruntergeladenen Dateien von den GitHub Actions dieses Repositorys gebaut und nicht manipuliert wurden. Alle Signaturen verwenden schlüsselloses Signieren mit [Sigstore](https://www.sigstore.dev/), es gibt also keinen privaten Schlüssel, der geschützt werden müsste.

## Binärarchive

Die Release-Seite stellt `checksums.txt` (SHA-256 jedes Archivs), `checksums.txt.sigstore.json` (die Signatur der Prüfsummendatei) und eine SBOM bereit.

1. Die SHA-256 des Archivs prüfen:

```bash
sha256sum -c checksums.txt --ignore-missing
```

2. Die Signatur von `checksums.txt` mit [cosign](https://docs.sigstore.dev/cosign/installation/) prüfen. Die signierende Identität muss der Release-Workflow dieses Repositorys sein:

```bash
cosign verify-blob checksums.txt \
  --bundle checksums.txt.sigstore.json \
  --certificate-identity-regexp '^https://github.com/wangwanjie/gately/\.github/workflows/release\.yml@refs/tags/v' \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com
```

3. Mit der GitHub CLI können Sie außerdem die Build-Provenienz eines einzelnen Archivs prüfen:

```bash
gh attestation verify gately_v1.0.0_linux_amd64.tar.gz --repo wangwanjie/gately
```

## Docker-Image

```bash
cosign verify ghcr.io/wangwanjie/gately:latest \
  --certificate-identity-regexp '^https://github.com/wangwanjie/gately/\.github/workflows/release\.yml@refs/tags/v' \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com
```

`vanjayak/gately` auf Docker Hub ist dasselbe Image mit derselben Signaturidentität:

```bash
cosign verify docker.io/vanjayak/gately:latest \
  --certificate-identity-regexp '^https://github.com/wangwanjie/gately/\.github/workflows/release\.yml@refs/tags/v' \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com
```

Die Registry speichert für das Image außerdem eine SBOM und eine Provenienz-Attestierung. Die SBOM zeigen Sie so an:

```bash
docker buildx imagetools inspect ghcr.io/wangwanjie/gately:latest --format '{{ json .SBOM }}'
```

## SBOM

`gately_<version>_sbom.spdx.json` im Release liegt im SPDX-Format vor. Sie listet alle Go-Module und Frontend-Abhängigkeiten und kann an `grype`, `trivy` und ähnliche Scanner übergeben werden.
