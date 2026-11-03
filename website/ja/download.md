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

### カスタムビルド {#custom-build}

オプションのモジュールと画面の言語はコンパイル時に取り除けます（説明は [モジュール一覧 · カスタムビルド](/ja/docs/modules/#custom-build) を参照）。いちばん手軽なのは [build.gately.cn](https://build.gately.cn) で選んでオンラインで生成する方法です。自分でコンパイルする場合：

- Go はビルドタグでモジュールと言語を取り除きます。オプションのモジュールごとに `no_<モジュール>`（例：`no_waf`、`no_smb`）があり、言語のレベルは `lang_zh`（簡体字中国語のみ）または `lang_cjken`（簡体字中国語・繁体字中国語・英語）です。タグを付けなければ完全版になります
- フロントエンドは環境変数 `GATELY_LOCALES` で選んだ言語だけを同梱します（例：`zh-CN,zh-TW,en`）。空なら全言語です
- リポジトリのルートにある `modules.json` にオプションのモジュール、依存関係、サイズの見積もりがすべて載っています。`go run ./tools/custombuild resolve` は選択内容をタグ、言語、Docker のビルド引数に換算し、`build` サブコマンドは tar.gz や fnOS パッケージを直接生成します

```bash
# DDNS と frp だけを残し、画面は簡体字中国語のみ
go run ./tools/custombuild resolve -modules ddns,frp -lang zh
go run ./tools/custombuild build -version v0.6.4 -modules ddns,frp -lang zh -targets linux-arm64 -out dist
```

Docker イメージは同じ Dockerfile を使います。ビルド引数 `GATELY_TAGS`、`GATELY_LOCALES` は上記のタグと言語に対応し、`WITH_DOCKER_CLI=0` は docker-cli と compose をインストールせず（Docker モジュールを含まない場合）、`WITH_NFTABLES=0` は nftables をインストールしません（侵入防御と STUN を含まない場合）。`resolve -targets docker` で対応する引数が出力されます。

## 検証

各バージョンにはチェックサムファイルが付属しています。ダウンロード後に SHA-256 を照合してから実行してください。

v0.5.0 以降、チェックサムファイルと Docker イメージには cosign のキーレス署名が付き、アーカイブにはビルド来歴の証明と SBOM が用意されています。検証方法は[リリースの検証](/ja/docs/guide/verify)（英語）を参照してください。
