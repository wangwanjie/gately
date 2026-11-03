# Verificar versiones

Cada versión oficial de Gately incluye firmas y procedencia, para que puedas confirmar que los archivos que descargaste los compiló GitHub Actions de este repositorio y que nadie los ha manipulado. Todas las firmas usan la firma sin claves (keyless) de [Sigstore](https://www.sigstore.dev/), así que no hay ninguna clave privada que proteger.

## Archivos de binarios

La página de la versión ofrece `checksums.txt` (SHA-256 de cada archivo), `checksums.txt.sigstore.json` (la firma del archivo de sumas de comprobación) y un SBOM.

1. Verifica el SHA-256 del archivo:

```bash
sha256sum -c checksums.txt --ignore-missing
```

2. Verifica la firma de `checksums.txt` con [cosign](https://docs.sigstore.dev/cosign/installation/). La identidad de firma debe ser el workflow de publicación de este repositorio:

```bash
cosign verify-blob checksums.txt \
  --bundle checksums.txt.sigstore.json \
  --certificate-identity-regexp '^https://github.com/wangwanjie/gately/\.github/workflows/release\.yml@refs/tags/v' \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com
```

3. También puedes verificar la procedencia de compilación de un archivo concreto con la CLI de GitHub:

```bash
gh attestation verify gately_v1.0.0_linux_amd64.tar.gz --repo wangwanjie/gately
```

## Imagen de Docker

```bash
cosign verify ghcr.io/wangwanjie/gately:latest \
  --certificate-identity-regexp '^https://github.com/wangwanjie/gately/\.github/workflows/release\.yml@refs/tags/v' \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com
```

`vanjayak/gately` en Docker Hub es la misma imagen, con la misma identidad de firma:

```bash
cosign verify docker.io/vanjayak/gately:latest \
  --certificate-identity-regexp '^https://github.com/wangwanjie/gately/\.github/workflows/release\.yml@refs/tags/v' \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com
```

El registro también guarda un SBOM y una atestación de procedencia para la imagen. Inspecciona el SBOM con:

```bash
docker buildx imagetools inspect ghcr.io/wangwanjie/gately:latest --format '{{ json .SBOM }}'
```

## SBOM

`gately_<version>_sbom.spdx.json` de la versión está en formato SPDX. Enumera todos los módulos de Go y las dependencias del frontend, y se puede pasar a `grype`, `trivy` y escáneres similares.
