# Проверка релизов

Каждый официальный выпуск Gately снабжён подписями и подтверждением происхождения, поэтому можно убедиться, что скачанные файлы собраны GitHub Actions этого репозитория и не изменены. `checksums.txt` подписан minisign, открытый ключ встроен в Gately, и онлайн-обновление проверяет им загрузки; подписи cosign и подтверждение происхождения сборки создаются бесключевым способом [Sigstore](https://www.sigstore.dev/).

## Бинарные архивы

На странице выпуска есть `checksums.txt` (SHA-256 каждого архива), `checksums.txt.minisig` (подпись minisign файла контрольных сумм), `checksums.txt.sigstore.json` (подпись cosign файла контрольных сумм) и SBOM. У выпусков, опубликованных до появления подписи, нет `checksums.txt.minisig`.

1. Проверьте подпись `checksums.txt` с помощью [minisign](https://jedisct1.github.io/minisign/). Открытый ключ выпусков Gately (ID ключа `512082B4037581F0`):

```text
RWTwgXUDtIIgUVNxL3scLhMObpyS0vKQbkCepGauqNA0/u0nQA86M50/
```

```bash
minisign -Vm checksums.txt -P RWTwgXUDtIIgUVNxL3scLhMObpyS0vKQbkCepGauqNA0/u0nQA86M50/
```

Вывод `Signature and comment signature verified` означает, что подпись действительна; строка `Trusted comment` содержит версию и время подписи.

2. Проверьте SHA-256 архива:

```bash
sha256sum -c checksums.txt --ignore-missing
```

3. Проверьте подпись `checksums.txt` с помощью [cosign](https://docs.sigstore.dev/cosign/installation/). Подписывающей стороной должен быть workflow релизов этого репозитория:

```bash
cosign verify-blob checksums.txt \
  --bundle checksums.txt.sigstore.json \
  --certificate-identity-regexp '^https://github.com/wangwanjie/gately/\.github/workflows/release\.yml@refs/tags/v' \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com
```

4. Происхождение сборки отдельного архива можно проверить и через GitHub CLI:

```bash
gh attestation verify gately_v1.0.0_linux_amd64.tar.gz --repo wangwanjie/gately
```

## Проверка подписи при онлайн-обновлении

Онлайн-обновление бинарных установок в Linux («Обновить») сначала скачивает `checksums.txt` и `checksums.txt.minisig`, проверяет подпись встроенным в программу открытым ключом и только потом сверяет SHA-256 пакета:

- подпись недействительна, сделана неизвестным ключом или `checksums.txt` изменён: обновление отклоняется, окно обновления и системный журнал показывают причину, установленная программа остаётся прежней;
- в выпуске нет `checksums.txt.minisig` (опубликован до появления подписи): обновление продолжается, а окно обновления и системный журнал предупреждают, что выпуск не подписан и проверен только его SHA-256.

Для смены ключа в программе предусмотрено второе место под открытый ключ: новый ключ сначала встраивается в один из выпусков и публикуется на этой странице, и только последующие выпуски подписываются им, поэтому установленные экземпляры продолжают обновляться. «Загрузить пакет обновления» подписи не проверяет — перед загрузкой проверьте архив по шагам выше. Обновления образа Docker опираются на описанные ниже подписи cosign образа.

## Подпись кода в macOS

Бинарный файл `gately` в архивах для macOS подписан сертификатом Apple Developer ID (Team ID `X6B6C6U6QV`) и нотаризован Apple, поэтому запускается сразу после скачивания и распаковки. Проверить подпись:

```bash
codesign -dv --verbose=2 gately
spctl -a -vv -t install gately
```

В выводе должны быть `TeamIdentifier=X6B6C6U6QV` и `source=Notarized Developer ID`. При первом запуске macOS проверяет запись о нотаризации через интернет.

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
