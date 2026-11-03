# Vérifier les versions

Chaque version officielle de Gately est fournie avec des signatures et une attestation de provenance, pour vous permettre de confirmer que les fichiers téléchargés ont été construits par les GitHub Actions de ce dépôt et n'ont pas été altérés. Toutes les signatures utilisent la signature sans clé de [Sigstore](https://www.sigstore.dev/) : il n'y a donc aucune clé privée à protéger.

## Archives binaires

La page de la version fournit `checksums.txt` (SHA-256 de chaque archive), `checksums.txt.sigstore.json` (la signature du fichier de sommes) et un SBOM.

1. Vérifiez le SHA-256 de l'archive :

```bash
sha256sum -c checksums.txt --ignore-missing
```

2. Vérifiez la signature de `checksums.txt` avec [cosign](https://docs.sigstore.dev/cosign/installation/). L'identité de signature doit être le workflow de publication de ce dépôt :

```bash
cosign verify-blob checksums.txt \
  --bundle checksums.txt.sigstore.json \
  --certificate-identity-regexp '^https://github.com/wangwanjie/gately/\.github/workflows/release\.yml@refs/tags/v' \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com
```

3. Vous pouvez aussi vérifier l'attestation de provenance de build d'une archive avec la CLI GitHub :

```bash
gh attestation verify gately_v1.0.0_linux_amd64.tar.gz --repo wangwanjie/gately
```

## Image Docker

```bash
cosign verify ghcr.io/wangwanjie/gately:latest \
  --certificate-identity-regexp '^https://github.com/wangwanjie/gately/\.github/workflows/release\.yml@refs/tags/v' \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com
```

`vanjayak/gately` sur Docker Hub est la même image, avec la même identité de signature :

```bash
cosign verify docker.io/vanjayak/gately:latest \
  --certificate-identity-regexp '^https://github.com/wangwanjie/gately/\.github/workflows/release\.yml@refs/tags/v' \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com
```

Le registre stocke aussi un SBOM et une attestation de provenance pour l'image. Inspectez le SBOM avec :

```bash
docker buildx imagetools inspect ghcr.io/wangwanjie/gately:latest --format '{{ json .SBOM }}'
```

## SBOM

Le fichier `gately_<version>_sbom.spdx.json` de la version est au format SPDX. Il liste chaque module Go et chaque dépendance du frontend, et peut être fourni à `grype`, `trivy` et aux scanners similaires.
