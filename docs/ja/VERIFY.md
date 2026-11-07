# リリースの検証

Gately の公式リリースにはすべて署名と来歴証明が付いており、ダウンロードしたファイルがこのリポジトリの GitHub Actions でビルドされ、改ざんされていないことを確認できます。署名はすべて [Sigstore](https://www.sigstore.dev/) のキーレス署名で作成されるため、管理すべき秘密鍵はありません。

## バイナリのアーカイブ

リリースページには `checksums.txt`（各アーカイブの SHA-256）、`checksums.txt.sigstore.json`（チェックサムファイルの署名）、SBOM があります。

1. アーカイブの SHA-256 を検証します。

```bash
sha256sum -c checksums.txt --ignore-missing
```

2. [cosign](https://docs.sigstore.dev/cosign/installation/) で `checksums.txt` の署名を検証します。署名者の ID は、このリポジトリのリリースワークフローでなければなりません。

```bash
cosign verify-blob checksums.txt \
  --bundle checksums.txt.sigstore.json \
  --certificate-identity-regexp '^https://github.com/wangwanjie/gately/\.github/workflows/release\.yml@refs/tags/v' \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com
```

3. GitHub CLI で個別のアーカイブのビルド来歴証明を検証することもできます。

```bash
gh attestation verify gately_v1.0.0_linux_amd64.tar.gz --repo wangwanjie/gately
```

## macOS のコード署名

macOS 向けアーカイブの `gately` は Apple Developer ID 証明書（チーム ID `X6B6C6U6QV`）で署名され、Apple の公証を受けています。ダウンロードして展開すればそのまま実行できます。署名の確認方法：

```bash
codesign -dv --verbose=2 gately
spctl -a -vv -t install gately
```

出力に `TeamIdentifier=X6B6C6U6QV` と `source=Notarized Developer ID` が含まれます。初回実行時に macOS がオンラインで公証記録を照会します。

## Docker イメージ

```bash
cosign verify ghcr.io/wangwanjie/gately:latest \
  --certificate-identity-regexp '^https://github.com/wangwanjie/gately/\.github/workflows/release\.yml@refs/tags/v' \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com
```

Docker Hub の `vanjayak/gately` は同じイメージで、署名の ID も同じです。

```bash
cosign verify docker.io/vanjayak/gately:latest \
  --certificate-identity-regexp '^https://github.com/wangwanjie/gately/\.github/workflows/release\.yml@refs/tags/v' \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com
```

レジストリにはイメージの SBOM と来歴証明も保存されています。SBOM は次のコマンドで確認できます。

```bash
docker buildx imagetools inspect ghcr.io/wangwanjie/gately:latest --format '{{ json .SBOM }}'
```

## SBOM

リリースに含まれる `gately_<version>_sbom.spdx.json` は SPDX 形式です。すべての Go モジュールとフロントエンドの依存関係が列挙されており、`grype` や `trivy` などのスキャナーに渡せます。
