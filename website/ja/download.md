---
title: "ダウンロード"
description: "Gately の最新バージョンのバイナリ、fnOS パッケージ、OpenWrt パッケージをダウンロードするか、Docker イメージでデプロイします。"
---

# Gately のダウンロード

## Docker イメージ（推奨）

イメージは amd64、arm64、arm/v7 の 3 アーキテクチャを提供しています。

```bash
docker pull ghcr.io/wangwanjie/gately:latest
```

同じイメージ（タグも共通）を Docker Hub からも取得できます。

```bash
docker pull vanjayak/gately:latest
```

デプロイ時のパラメーターの詳細は [インストールとデプロイ](/ja/docs/guide/install) を参照してください。

## バイナリ

使用するプラットフォーム向けのアーカイブをダウンロードして展開し、`./gately -c /etc/gately` を実行します。Linux（amd64 / arm64 / arm / mips / mipsle / riscv64）、macOS、Windows、FreeBSD に対応しています。

<DownloadList />

::: tip ダウンロードが遅い場合
ネットワーク環境によっては GitHub へのアクセスが遅くなることがあります。NAS やソフトルーターのユーザーは、先にプロキシ経由で取得するか、Docker イメージを利用してください。
:::

## fnOS ネイティブアプリ {#fnos}

fnOS ユーザーはネイティブアプリを直接インストールできます。上の一覧の fnOS グループから `.fpk` をダウンロードし（Intel / AMD は `x86`、ARM デバイスは `arm`）、fnOS の「アプリセンター → 手動インストール」で選択してください。

- fnOS 統一ゲートウェイに対応：デスクトップのアイコンから開け、fnOS のリモートドメインでも利用でき、ポートを追加で開放する必要はありません
- fnOS 管理者は fnOS のログイン状態でそのままログイン：有効にすると、fnOS にログイン済みの管理者は Gately のパスワードと 2 段階認証コードを省略できます

詳しくは [インストールとデプロイ · fnOS ネイティブアプリ](/ja/docs/guide/install#fnos) を参照してください。

## OpenWrt パッケージ {#openwrt}

OpenWrt ユーザーは、上の一覧の OpenWrt グループからパッケージをダウンロードできます。OpenWrt 24.10 以前（iStoreOS を含む）は `.ipk` を選んで `opkg install` で、OpenWrt 25.12 以降は `.apk` を選んで `apk add --allow-untrusted` でインストールします。アーキテクチャは `/etc/openwrt_release` の `DISTRIB_ARCH` に合わせて選んでください。インストール後すぐにサービスが起動し、設定は `/etc/gately` に保存されます。

同じグループの `luci-app-gately_<バージョン>_openwrt_all` は LuCI アプリです。アーキテクチャに依存せず、Gately のあとにインストールすると LuCI の「サービス → Gately」からサービスを管理できます。

詳しくは [インストールとデプロイ · OpenWrt](/ja/docs/guide/install#openwrt) を参照してください。

## ソースからビルド

Go 1.26+、Node.js 24+、pnpm が必要です。

```bash
git clone https://github.com/wangwanjie/gately.git
cd gately
make web      # フロントエンドをビルド
make build    # bin/gately を出力
```

## 検証

各バージョンにはチェックサムファイルが付属しています。ダウンロード後に SHA-256 を照合してから実行してください。

v0.5.0 以降、チェックサムファイルと Docker イメージには cosign のキーレス署名が付き、アーカイブにはビルド来歴の証明と SBOM が用意されています。検証方法は[リリースの検証](/ja/docs/guide/verify)（英語）を参照してください。
