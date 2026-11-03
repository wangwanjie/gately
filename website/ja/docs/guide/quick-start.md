# クイックスタート

5 分以内にデプロイ、ログイン、最初のルール作成まで完了できます。

## 1. 起動

```bash
docker run -d --name gately --network host \
  -v ./gately-data:/etc/gately \
  -v /var/run/docker.sock:/var/run/docker.sock \
  ghcr.io/wangwanjie/gately:latest
```

Docker 管理機能が不要な場合は、`docker.sock` のマウントを外せます。ポート転送、STUN、Web サービスはホストのポートを待ち受ける必要があるため、`--network host` を推奨します。

GHCR へのアクセスが遅い場合は、Docker Hub の `vanjayak/gately:latest` を使えます。中身は同じイメージです。

::: tip fnOS ユーザーの方へ
代わりにネイティブアプリをインストールしてください。fnOS 統一ゲートウェイ経由でデスクトップのアイコンから開け、fnOS 管理者は fnOS のログイン状態でそのままログインできます。詳しくは [fnOS ネイティブアプリ](/ja/docs/guide/install#fnos) を参照してください。
:::

## 2. ログイン

ブラウザで `http://<デバイスのIP>:16680` を開き、ユーザー名 `admin`、デフォルトのパスワード `gately.cn` でログインします。デフォルトのパスワードは公開されているため、変更するまで管理画面の上部に警告が表示され続けます。

## 3. 基本的なハードニング

ログイン後すぐに、「設定」で次の操作を行ってください。

1. 管理者パスワードを変更する
2. 2 段階認証（TOTP）を有効にする
3. 必要に応じて、セキュアエントリー、LAN 内からのみ許可、管理画面の HTTPS を設定する

詳しくは [ハードニングの推奨事項](/ja/docs/guide/hardening) を参照してください。

## 4. 最初のルールを追加

目的に合ったモジュールから始めましょう。

| 目的 | モジュール |
| --- | --- |
| 自宅の公開 IP が頻繁に変わるので、固定のドメイン名でアクセスしたい | [ダイナミック DNS](/ja/docs/modules/ddns) |
| 回線事業者から固定の公開ポートが割り当てられず、外出先から自宅のサービスにアクセスしたい | [STUN 穿孔](/ja/docs/modules/stun) |
| 1 つのポートを LAN 内の別のデバイスに転送したい | [ポート転送](/ja/docs/modules/portforward) |
| 1 つのドメイン名と HTTPS で複数の LAN 内サービスにアクセスしたい | [SSL 証明書](/ja/docs/modules/ssl) + [Web サービス](/ja/docs/modules/web) |
| 拠点間をつなぐネットワークを構築したい | [Tailscale / frp](/ja/docs/modules/tunnels) |
