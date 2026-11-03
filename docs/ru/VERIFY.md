# Проверка релизов

Каждый официальный релиз Gately поставляется с подписями и сведениями о происхождении сборки, поэтому можно убедиться, что скачанные файлы собраны GitHub Actions этого репозитория и не были подменены. Все подписи создаются бесключевым способом [Sigstore](https://www.sigstore.dev/), поэтому хранить закрытый ключ не нужно.

## Бинарные архивы

На странице релиза есть `checksums.txt` (SHA-256 каждого архива), `checksums.txt.sigstore.json` (подпись файла контрольных сумм) и SBOM.

1. Проверьте SHA-256 архива:

```bash
sha256sum -c checksums.txt --ignore-missing
```

2. Проверьте подпись `checksums.txt` с помощью [cosign](https://docs.sigstore.dev/cosign/installation/). Подписывающей стороной должен быть workflow релизов этого репозитория:

```bash
cosign verify-blob checksums.txt \
  --bundle checksums.txt.sigstore.json \
  --certificate-identity-regexp '^https://github.com/wangwanjie/gately/\.github/workflows/release\.yml@refs/tags/v' \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com
```

3. Происхождение сборки отдельного архива можно проверить и через GitHub CLI:

```bash
gh attestation verify gately_v1.0.0_linux_amd64.tar.gz --repo wangwanjie/gately
```

## Образ Docker

```bash
cosign verify ghcr.io/wangwanjie/gately:latest \
  --certificate-identity-regexp '^https://github.com/wangwanjie/gately/\.github/workflows/release\.yml@refs/tags/v' \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com
```

`vanjayak/gately` на Docker Hub — тот же образ, подписанный тем же рабочим процессом выпуска:

```bash
cosign verify docker.io/vanjayak/gately:latest \
  --certificate-identity-regexp '^https://github.com/wangwanjie/gately/\.github/workflows/release\.yml@refs/tags/v' \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com
```

В реестре образов также хранятся SBOM и аттестация происхождения образа. Посмотреть SBOM можно так:

```bash
docker buildx imagetools inspect ghcr.io/wangwanjie/gately:latest --format '{{ json .SBOM }}'
```

## SBOM

`gately_<version>_sbom.spdx.json` в релизе имеет формат SPDX. В нём перечислены все модули Go и зависимости фронтенда, и его можно передать сканерам `grype`, `trivy` и подобным.
