# リリースの検証

Gately の正式リリースにはすべて署名と来歴証明が付いており、ダウンロードしたファイルがこのリポジトリの GitHub Actions でビルドされ、改ざんされていないことを確認できます。`checksums.txt` には minisign 署名が付いており、その公開鍵は Gately に組み込まれていて、オンラインアップグレードはこれでダウンロード内容を検証します。cosign 署名とビルド来歴証明は [Sigstore](https://www.sigstore.dev/) のキーレス方式で生成しています。

## バイナリのアーカイブ

リリースページには `checksums.txt`（各アーカイブの SHA-256）、`checksums.txt.minisig`（チェックサムファイルの minisign 署名）、`checksums.txt.sigstore.json`（チェックサムファイルの cosign 署名）と SBOM ファイルがあります。署名導入前に公開されたバージョンには `checksums.txt.minisig` がありません。

1. [minisign](https://jedisct1.github.io/minisign/) で `checksums.txt` の署名を検証します。Gately のリリース公開鍵（鍵 ID `512082B4037581F0`）は次のとおりです。

```text
RWTwgXUDtIIgUVNxL3scLhMObpyS0vKQbkCepGauqNA0/u0nQA86M50/
```

```bash
minisign -Vm checksums.txt -P RWTwgXUDtIIgUVNxL3scLhMObpyS0vKQbkCepGauqNA0/u0nQA86M50/
```

`Signature and comment signature verified` と表示されれば署名は有効です。`Trusted comment` の行にはバージョンと署名日時が記載されています。

2. アーカイブの SHA-256 を検証します。

```bash
sha256sum -c checksums.txt --ignore-missing
```

3. [cosign](https://docs.sigstore.dev/cosign/installation/) で `checksums.txt` の署名を検証します。署名者の ID は、このリポジトリのリリースワークフローでなければなりません。

```bash
cosign verify-blob checksums.txt \
  --bundle checksums.txt.sigstore.json \
  --certificate-identity-regexp '^https://github.com/wangwanjie/gately/\.github/workflows/release\.yml@refs/tags/v' \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com
```

4. GitHub CLI で個別のアーカイブのビルド来歴証明を検証することもできます。

```bash
gh attestation verify gately_v1.0.0_linux_amd64.tar.gz --repo wangwanjie/gately
```

## オンラインアップグレード時の署名検証

Linux バイナリ配置のオンラインアップグレード（「今すぐアップグレード」）は、まず `checksums.txt` と `checksums.txt.minisig` をダウンロードし、プログラムに組み込まれた公開鍵で署名を検証してから、アップグレードパッケージの SHA-256 を照合します。

- 署名が無効、未知の鍵による署名、または `checksums.txt` が改変されている場合: アップグレードを拒否し、アップグレードダイアログとシステムログに理由を表示します。現在のプログラムはそのまま残ります。
- そのバージョンに `checksums.txt.minisig` がない場合（署名導入前の公開）: アップグレードは続行し、アップグレードダイアログとシステムログで、未署名のため SHA-256 のみ確認したことを警告します。

鍵の切り替えに備えて、プログラムには 2 つ目の公開鍵の枠があります。新しい公開鍵はまずあるバージョンに組み込まれてこのページで公開され、その後のバージョンから新しい鍵で署名されるため、配置済みのプログラムはそのままアップグレードできます。「アップグレードパッケージをアップロード」は署名を検証しないため、アップロード前に上記の手順で確認してください。Docker イメージのアップグレードは下記の cosign によるイメージ署名で確認します。

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
