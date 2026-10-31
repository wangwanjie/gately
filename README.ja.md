<p align="center">
  <a href="https://gately.cn/ja/"><img src="docs/images/banner.svg" alt="Gately — オールインワンのネットワークゲートウェイ: オープンソースで、すぐに使える" width="100%"></a>
</p>

<p align="center">
  <a href="https://github.com/wangwanjie/gately/releases/latest"><img src="https://img.shields.io/github/v/release/wangwanjie/gately?style=flat-square&color=0e7c66&label=release" alt="Release"></a>
  <a href="https://github.com/wangwanjie/gately/pkgs/container/gately"><img src="https://img.shields.io/badge/docker-ghcr.io%2Fwangwanjie%2Fgately-0e7c66?style=flat-square&logo=docker&logoColor=white" alt="Docker"></a>
  <a href="LICENSE"><img src="https://img.shields.io/github/license/wangwanjie/gately?style=flat-square&color=0e7c66" alt="License"></a>
  <a href="https://github.com/wangwanjie/gately/stargazers"><img src="https://img.shields.io/github/stars/wangwanjie/gately?style=flat-square&color=0e7c66" alt="Stars"></a>
  <a href="https://gately.cn/ja/"><img src="https://img.shields.io/badge/%E5%85%AC%E5%BC%8F%E3%82%B5%E3%82%A4%E3%83%88-gately.cn-0e7c66?style=flat-square&logo=googlechrome&logoColor=white" alt="gately.cn"></a>
  <a href="https://t.me/gately_chat"><img src="https://img.shields.io/badge/Telegram-gately__chat-26A5E4?style=flat-square&logo=telegram&logoColor=white" alt="Telegram"></a>
</p>

<p align="center">
  <a href="https://gately.cn/ja/"><b>公式サイト</b></a> &nbsp;·&nbsp; <a href="https://gately.cn/ja/docs/guide/quick-start"><b>クイックスタート</b></a> &nbsp;·&nbsp; <a href="https://gately.cn/ja/docs/guide/install"><b>インストールガイド</b></a> &nbsp;·&nbsp; <a href="https://gately.cn/ja/docs/guide/introduction"><b>ドキュメント</b></a> &nbsp;·&nbsp; <a href="https://github.com/wangwanjie/gately/releases/latest"><b>ダウンロード</b></a> &nbsp;·&nbsp; <a href="https://gately.cn/ja/news/"><b>更新履歴</b></a>
</p>

<p align="center">
  <a href="https://gately.cn/ja/sponsor"><img src="https://img.shields.io/badge/%E3%82%B3%E3%83%BC%E3%83%92%E3%83%BC%E3%82%92%E3%81%8A%E3%81%94%E3%82%8B-Gately-FFDD00?style=for-the-badge&logo=buymeacoffee&logoColor=black" alt="コーヒーをおごる" height="34"></a>
</p>

<p align="center">
  <a href="README.md">English</a> | <a href="README.zh-CN.md">简体中文</a> | <a href="README.zh-TW.md">繁體中文</a> | <b>日本語</b> | <a href="README.ko.md">한국어</a> | <a href="README.es.md">Español</a> | <a href="README.fr.md">Français</a> | <a href="README.de.md">Deutsch</a> | <a href="README.ru.md">Русский</a> | <a href="README.pt-BR.md">Português (Brasil)</a>
</p>

ドメイン解決や NAT 越えから、リバースプロキシ、HTTPS 証明書、WAF、侵入防御、シングルサインオン、ファイル共有まで。自宅ネットワークとサーバーの入口に必要な機能を、Gately はひとつの管理画面にまとめました。NAS、ソフトルーター、クラウドサーバーに入れれば、設定も運用もブラウザーで完結します。ソースコードは Apache-2.0 で公開しており、設定とデータはあなたのデバイスにだけ残り、収集も送信もしません。

> **fnOS ユーザーの方へ**：Gately は fnOS のネイティブアプリとして提供されています。**fnOS 統一ゲートウェイ**に統合されており、fnOS デスクトップのアイコンから開けます。リモートアクセス用のドメインを含め、fnOS と同じアドレスでそのまま使えるため、ポートを追加で開放する必要はありません。有効にすると、**fnOS 管理者は fnOS のログイン状態のままログイン**でき、Gately のパスワードや 2 段階認証コードの入力は不要です。詳しくは [fnOS ネイティブアプリ](https://gately.cn/ja/docs/guide/install#fnos) を参照してください。

## クイックスタート

使い始めるまで 3 ステップ。コンテナを起動し、`http://<デバイスのIP>:16680` を開いて `admin` / `gately.cn` でログインするだけです。ほかのプラットフォームは下にまとめてあり、すべての方法は[インストールガイド](https://gately.cn/ja/docs/guide/install)で解説しています。

### Docker

イメージは amd64、arm64、arm/v7 に対応しています。

```bash
docker run -d --name gately --network host --cap-add NET_ADMIN \
  -v ./gately-data:/etc/gately \
  -v /var/run/docker.sock:/var/run/docker.sock \
  ghcr.io/wangwanjie/gately:latest
```

`http://<デバイスのIP>:16680` を開き、デフォルトのアカウント `admin` とデフォルトのパスワード `gately.cn` でログインします。その後「設定」でパスワードを変更し、2 段階認証を有効にしてください。完全な Compose の例は [deploy/docker-compose.yml](deploy/docker-compose.yml) を参照してください。

パスワードを忘れたときや 2 段階認証アプリを紛失したときは、サーバー上で `gately reset-password` または `gately reset-totp` を実行します（Docker では `docker exec -it gately gately reset-password`）。確認するとすぐに反映されます。詳しくは[パスワードと 2 段階認証のリセット](https://gately.cn/ja/docs/guide/install#reset)を参照してください。

ポート転送、STUN、Web サービスはホストのポートで待ち受けるため、`--network host` を推奨します。Docker 管理が不要な場合は `docker.sock` のマウントを外せます。`--cap-add NET_ADMIN` は侵入防御がブロックをホストの nftables ファイアウォールに書き込むためのもので、付けない場合もブロックは Gately 自身のサービスに適用されます。

別の方法でインストールしたい場合は、下から選んでください。

<details>
<summary><b>バイナリ</b></summary>

[Releases](https://github.com/wangwanjie/gately/releases) からお使いのプラットフォーム向けのアーカイブ（Linux amd64/arm64/arm/mips/mipsle/riscv64、macOS、Windows、FreeBSD）をダウンロードし、展開して実行します。

```bash
./gately -c /etc/gately
```

オプション: `-c` は設定ディレクトリ（環境変数 `GATELY_CONFIG_DIR` でも指定可能）、`-listen` は管理画面の待ち受けアドレスの上書き、`-apply` は起動のたびにディレクトリ内の `<name>.json` を設定ディレクトリに適用（`GATELY_APPLY_DIR`、GitOps や Kubernetes に便利。アカウントとトークンのファイルは上書き不可）、`-version` はバージョン表示です。無人インストールでは `GATELY_INITIAL_PASSWORD` を設定すると、デフォルトのパスワード `gately.cn` の代わりに管理者の初期パスワードを指定できます。

サービステンプレート: [systemd](deploy/systemd/gately.service)、[OpenWrt procd](deploy/openwrt/gately.init)。ダウンロードしたアーカイブやイメージの検証（cosign 署名、ビルド来歴、SBOM）は[リリース成果物の検証](docs/ja/VERIFY.md)を参照してください。

</details>

<details>
<summary><b>fnOS ネイティブアプリ</b></summary>

各 Release には fnOS 向けのオフラインパッケージ `gately_<version>_fnos_x86.fpk`（Intel/AMD）と `gately_<version>_fnos_arm.fpk`（ARM）が含まれます。公式のアプリセンターには掲載されていないため、手動でインストールします。**アプリセンター → 手動インストール**（左下のボタン）を開き、ファイルを選んでウィザードを完了してください。

- **fnOS 統一ゲートウェイに対応**：fnOS デスクトップの Gately アイコンをクリックすると、管理画面が fnOS 統一ゲートウェイ経由で `/app/gately/` 配下に開き、fnOS と同じアクセス入口を共用します。LAN のアドレスでも fnOS のリモートドメインでもそのまま使え、16680 ポートを開放・転送する必要はありません。ゲートウェイ経由のアクセスは fnOS が制御する（fnOS 管理者だけが開けます）ため、セキュアエントリーと「LAN 内からのみ許可」は 16680 ポートへの直接アクセスにのみ適用されます。
- **fnOS 管理者は fnOS のログイン状態でそのままログイン**：**設定 → アクセス制御 → fnOS のログイン状態で自動ログイン** を有効にすると、fnOS にログイン済みの管理者はデスクトップのアイコンからパスワードや 2 段階認証コードなしで Gately に入れます。一般の fnOS ユーザーは引き続きログインが必要です。ログアウトした後は、自動で再ログインせず、ログイン画面に「fnOS アカウント &lt;ユーザー名&gt; でログイン」ボタンが表示されます。この設定は既定で無効で、fnOS パッケージでのみ表示されます。
- LAN 内では `http://<NAS のアドレス>:16680` でも管理画面にアクセスできます。デフォルトのアカウントは `admin`、パスワードは `gately.cn` です。
- アプリは `root` で動作するため、80/443 の待ち受け、`docker.sock` を通じた Docker の管理、実際の Web ターミナルが利用できます。自分で管理している NAS にのみインストールしてください。
- 設定はアプリの `var` ディレクトリに保存され、アンインストール時も保持されます。アンインストールウィザードで「すべて削除」を選んだ場合のみ消去されます。アップグレードは新しい `.fpk` を再度手動インストールするだけです。
- パッケージには `gately` コマンド（`/usr/local/bin/gately`）も含まれ、`gately ctl` と `gately mcp` を使えます。
- 16680 ポートは Docker イメージと共通です。ネイティブアプリを起動する前に `gately` コンテナを停止してください。
- ダウンロードしたファイルは [リリース成果物の検証](docs/ja/VERIFY.md) の手順で検証できます。`.fpk` ファイルも `checksums.txt` に記載されています。

</details>

<details>
<summary><b>OpenWrt</b></summary>

各 Release には OpenWrt 向けパッケージが含まれます。`gately_<version>_openwrt_<arch>.ipk` は OpenWrt 24.10 以前（iStoreOS を含む）向けで `opkg install` で、`.apk` は OpenWrt 25.12 以降向けで `apk add --allow-untrusted` でインストールします。アーキテクチャは amd64 / arm64 / arm / mipsle / mips / riscv64 で、`/etc/openwrt_release` の `DISTRIB_ARCH` に合わせて選びます（対応表は[インストールガイド](https://gately.cn/ja/docs/guide/install#openwrt)を参照）。

- インストール時に自動起動が有効になり、サービスが起動します。管理画面は `http://<ルーターのアドレス>:16680` で、デフォルトのアカウントは `admin`、パスワードは `gately.cn` です。
- 設定は `/etc/gately` に保存され、アップグレード、アンインストール、ファームウェアの更新（sysupgrade）でも保持されます。アップグレードは新しいパッケージをインストールするだけです。「バージョン情報」ページのオンラインアップグレードでは、パッケージマネージャーを使うよう案内されます。
- バイナリは約 32 MB のため、x86 のソフトルーターや ARM64 ボックスなどストレージに余裕のあるデバイス向けです。arm パッケージにはハードウェア浮動小数点に対応した ARMv7 が必要です。
- アーキテクチャに依存しない LuCI アプリ `luci-app-gately_<バージョン>_openwrt_all.ipk` / `.apk` も用意しています。LuCI の「サービス → Gately」で実行状態の確認、開始・停止、自動起動と管理画面ポートの設定、管理画面へのワンクリック移動ができます。

</details>

<details>
<summary><b>監視</b></summary>

`metrics:read` 権限だけを持つ API トークンを作成し、Prometheus から `/api/metrics` をスクレイプします。メトリクス、アラート例、Grafana ダッシュボードは[監視](docs/ja/MONITORING.md)で説明しています。

</details>

<details>
<summary><b>API の利用</b></summary>

管理コンソールの「API トークン」ページでトークンを作成します（複数作成可能で、モジュールごとに読み取り専用または読み書きを付与でき、有効期限も設定できます。平文は作成直後に一度だけ表示されます）。スクリプトから管理 API を一括で呼び出せます。

```bash
curl -H "Authorization: Bearer gtl_xxxx" http://localhost:16680/api/ddns/tasks
```

アカウント、2 段階認証、トークン管理、およびバックアップ、復元、再起動、システム設定の API はログインセッションでのみ利用できます。すべての API、必要な権限、データ構造は管理コンソールの「API ドキュメント」ページで確認できます。`make api-docs` で [docs/api](docs/api/ja)（`openapi.json` と `API.md`）にエクスポートすることもでき、公式サイト https://gately.cn の公開時にそのまま参照されます。

旧バージョンからアップグレードすると、従来のグローバル API トークンは自動的に「レガシートークン」になります（値は同じで、全モジュールの権限を持ちます）。API トークンページで権限を絞るか、新しいトークンに切り替えることを推奨します。

</details>

<details>
<summary><b>Tailscale、frp、cloudflared</b></summary>

この 3 つのモジュールは公式の外部プログラムを管理します（自動再起動、ログは画面に集約）。Gately 本体には同梱していません。各モジュールページの「オンラインインストール」カードは上流から最新の安定版または指定バージョンをダウンロードし、SHA-256 を検証して設定ディレクトリの `bin/` にインストールし、それを使うプロセスを再起動します。プログラムごとにダウンロードプロキシを設定でき、空欄の場合は「設定」の更新プロキシを使います。置き換えられたバージョンは直近 3 個まで保持され、新バージョンに問題があればワンクリックでロールバックできます。不要になったプログラムはアンインストールできます。ページヘッダーの「デフォルト設定に戻す」はアンインストールとは別の操作で、プロセスを停止し、モジュール設定をリセットし、Gately が生成したファイル（frp の設定ファイル、File Browser のインスタンスデータ、Tailscale のログイン状態）を削除します。以前の設定は設定履歴から復元できます。Tailscale がそのまま実行できるパッケージを公式に提供しているのは Linux のみのため、他のシステムでは公式インストーラーを使ってパスを入力してください。プログラムは次の順に探索されます: モジュール設定で指定したパス、設定ディレクトリ配下の `bin/`、`PATH`。バイナリを自分で設定ディレクトリに置くこともできます（Docker 環境の例: `./gately-data/bin/frpc`）。Tailscale には `tailscale` と `tailscaled` の両方が必要です。設定ファイル内のトークンとキーは 0600 のパーミッションで保存され、cloudflared のトークンは環境変数で渡されるため、プロセス一覧には現れません。

File Browser モジュールは [FileBrowser Quantum](https://github.com/gtsteffaniak/filebrowser) をホストします（元の filebrowser はアーカイブされ、セキュリティ修正が行われません）。探索順とオンラインでのインストール・更新・ロールバックは上記と同じで、インストール先は `bin/filebrowser` です。管理者パスワードは環境変数で渡され、設定ファイルには書き込まれません。

</details>

## 機能

まずは管理画面のスクリーンショットをいくつか。全機能の一覧はこのセクションの最後にあります。

### 全体をひと目で

概要ページには CPU、メモリー、ディスク、トラフィック、各ネットワークインターフェースとそのパブリック IPv4 / IPv6、全モジュールの稼働状態が並びます。対応が必要な箇所はすぐにわかります。

<img src="docs/images/overview.webp" alt="全体をひと目で" width="100%">

### nginx の代わりになる Web サービス

ドメインごとにリバースプロキシ、静的ファイル、リダイレクトを設定でき、HTTPS と HTTP/3 に対応。サイトごとに WAF、IP ルール、ログイン認証を有効にでき、設定を変えても既存の接続は切れません。

<img src="docs/images/web.webp" alt="nginx の代わりになる Web サービス" width="100%">

<p><b>サイト統計</b> — リクエスト数、エラー、バックエンドの状態、アクセス元 IP と国のランキングを分単位で集計します。</p>

<img src="docs/images/site-stats.webp" alt="サイト統計" width="100%">

### 443 番ポートひとつを全サービスで共有

ポート転送の SNI ルーティングで、複数のサービスをドメインごとに同じポートへ振り分けます。TLS はそのままパススルーし、PROXY プロトコルで実際のクライアントアドレスを渡します。

<img src="docs/images/portforward-sni.webp" alt="443 番ポートひとつを全サービスで共有" width="100%">

### 主要プロバイダーに対応したダイナミック DNS

Cloudflare、Alibaba Cloud、DNSPod、Route 53 などに対応。IPv4 と IPv6 の両方を扱い、IP は URL、ネットワークインターフェース、コマンドから取得できます。アドレスが変わればすぐに通知します。

<img src="docs/images/ddns.webp" alt="主要プロバイダーに対応したダイナミック DNS" width="100%">

### 侵入防御

fail2ban 風のルールがログファイル、journald、コンテナのログを読み取り、トラップポートとスキャン検知で探索行為を捕まえます。ブロックは nftables に同期され、カーネルでパケットを破棄します。

<img src="docs/images/guard.webp" alt="侵入防御" width="100%">

### frp、Tailscale、Cloudflare トンネル

複数の frpc・frps インスタンスを並行して動かし、Tailscale や cloudflared のトンネルと同じ画面で管理できます。これらのプログラムのインストール、更新、ロールバックは Gately が引き受けます。

<img src="docs/images/frp.webp" alt="frp、Tailscale、Cloudflare トンネル" width="100%">

### ほかにも

<table>
<tr>
<td width="50%" valign="top">

**Docker**<br>コンテナ、Compose、ログ、ターミナル。イメージはワンクリックで更新でき、失敗すれば自動でロールバックします。

<img src="docs/images/docker.webp" alt="Docker" width="100%">

</td>
<td width="50%" valign="top">

**Web ターミナル**<br>ブラウザーで SSH、Telnet、ローカルシェルを使えます。SFTP と画面分割にも対応。

<img src="docs/images/terminal.webp" alt="Web ターミナル" width="100%">

</td>
</tr>
</table>

<table>
<tr>
<td width="50%" valign="top">

**設定履歴**<br>変更のたびに以前のバージョンを保存。差分を確認して、ワンクリックで戻せます。

<img src="docs/images/config-history.webp" alt="設定履歴" width="100%">

</td>
<td width="50%" valign="top">

**API、CLI、MCP**<br>モジュール単位で権限を付けられる API トークン、組み込みの OpenAPI ドキュメント、`gately ctl`、AI アシスタントから使える MCP サーバー。

<img src="docs/images/api-docs.webp" alt="API、CLI、MCP" width="100%">

</td>
</tr>
</table>

<details>
<summary><b>全機能一覧</b></summary>

| モジュール | 機能 |
| --- | --- |
| ダイナミック DNS | Alibaba Cloud、Cloudflare、Tencent Cloud、DNSPod、Huawei Cloud、GoDaddy、Porkbun、dynv6、DuckDNS、Amazon Route 53、OVHcloud、Gandi、NS1、Bunny DNS、Namecheap、DynDNS2（No-IP、Dynu など）、カスタムコールバック。URL／ネットワークインターフェース／コマンドによる IP 取得、IPv4/IPv6、Webhook；グループ分け・並べ替え・検索 |
| STUN 穿孔 | TCP/UDP、キープアライブ、UPnP/NAT-PMP、内蔵フォワーディング、アドレス変更時の Webhook・スクリプト実行 |
| ポート転送 | TCP/UDP、IPv4/IPv6、ポート範囲、複数ターゲットのフェイルオーバー、接続数制限、トラフィック統計、PROXY プロトコル（v1/v2）、SNI 振り分けで 443 などの 1 つのポートを複数のサービスで共用（ドメインごとの TLS パススルー、デフォルトターゲット、ルートごとの PROXY プロトコル設定）；グループ分け・並べ替え・検索 |
| SSL 証明書 | ACME による自動発行・更新（DNS-01 はワイルドカード対応で、委任されたサブドメインの DNS ゾーンを SOA で自動判別、HTTP-01 は DNS プロバイダー不要）、短期証明書とグローバル IP アドレス証明書（Let's Encrypt の shortlived プロファイル）、EAB、アップロード、ファイル読み込み、ディレクトリへのマッピングとスクリプト実行、証明書の更新後に SSH で他のホストへ送ってコマンドを実行、または証明書を使う Docker コンテナを再起動 |
| Tailscale | tailscaled の管理（ユーザー空間ネットワーク、コンテナ対応）またはシステム既存のデーモンへの接続。認証キー／ログインリンク、Headscale、サブネットルート、出口ノード、ノード一覧と SOCKS5 プロキシ、固定ポート、tailscaled の追加引数と環境変数。Linux ではオンラインでインストール・更新・ロールバック・アンインストール。デフォルト設定に戻すことも可能 |
| frp | 複数の frpc / frps インスタンスを個別に実行。frpc クライアント（tcp/udp/http/https/stcp/xtcp プロキシ、追加・削除は即時反映）と frps サーバー（ポート許可リスト、サブドメイン、ダッシュボード）。旧形式の ini／新形式の toml 設定をワンクリックでインポート。オンラインでインストール・更新・ロールバック・アンインストール。デフォルト設定に戻すことも可能 |
| Cloudflare トンネル | cloudflared のトークントンネルと trycloudflare の一時トンネル、複数トンネル、自動再起動、パブリックアドレス表示。オンラインでインストール・更新・ロールバック・アンインストール。デフォルト設定に戻すことも可能。トークントンネルのルートを Cloudflare API で編集し DNS レコードを作成 |
| Web サービス | ドメインごとのリバースプロキシ／リダイレクト／静的ファイル／固定テキスト／接続を切断（nginx の return 444 相当）、HTTPS と HTTP/3（QUIC）、同一ポートでの HTTP と HTTPS の共存（リダイレクトまたはそのまま提供）、WebSocket、PROXY プロトコルの送受信、Basic 認証、Web 認証、GitHub/Google/QQ/OIDC ログイン、TOTP による 2 段階認証とサブドメイン間のシングルサインオンに対応した共有ユーザー、共有ユーザーで Gitea や Grafana などにログインできる内蔵 OIDC プロバイダー、Authelia などの外部サービスへのフォワード認証、公開側の Gately から LAN 内の Gately へのログイン Cookie の転送、IP 許可・拒否リストと定期更新される購読 IP リスト（中国の省・市の IP 範囲など）、パスルール、一時共有リンク、認証失敗時の自動ブロック、GeoIP による国別フィルタ、パスごとのバックエンド、ログインユーザーごとのバックエンド（nginx の map $remote_user 相当）、アクセスされたドメインを SNI として送る HTTPS バックエンド、バックエンドの重み／スタンバイ／負荷分散ポリシー、アクティブ／パッシブヘルスチェック、圧縮、レート制限、レスポンスキャッシュ、パス書き換え、拡張子なし URL とカスタム 404 ページに対応した静的サイト、サイトの変更時に既存の接続を切断しない、サイトのグループ化・並べ替え・検索、1 分単位のアクセス統計と Prometheus メトリクス。共有ユーザーによるパスワードの自己変更。アクセス拒否時に 403 を返さず接続を切断 |
| WAF ファイアウォール | Coraza + OWASP CRS v4、ファイル単位のルール有効化、カスタム SecLang、ブロック／検知のみ、リクエストボディの検査上限を調整でき、上限超過時は先頭部分のみ検査またはパス単位で省略できるため、クラウドドライブや WebDAV などのアップロード用サイトにも WAF を適用可能 |
| 侵入防御 | fail2ban 風のログルール（ログファイル、systemd ジャーナル、コマンド出力、Docker コンテナのログ、管理画面のログイン失敗、WAF ブロック）、SSH・Nginx・Vaultwarden などのテンプレート、おとりポートとポートスキャン検出、手動のネットワーク・永久ブロック、段階的な期間、ホワイトリストを備えた統一ブロック一覧。ブロックはシステムファイアウォール（nftables）に同期してカーネルで破棄し、nftables のない環境では Gately 自身のサービスに適用。Web パス探索のブロック：リッスンごとにおとりパス（/.env、/.git など）、未知ドメインのスキャン、連続する 404 を集計 |
| Docker 管理 | コンテナの作成と編集、ログとターミナル、Compose、イメージ更新チェックとワンクリックアップグレード（失敗時は自動ロールバック）、ネットワークとボリューム |
| Web ターミナル | SSH（ホストキーは初回接続時に信頼）、Telnet、ローカルシェル（既定は無効）。プロキシ、永続セッション、SFTP ファイル管理、2FA とプロトコル確認、クイックコマンド、画面分割 |
| ネットワーク診断 | Ping、トレースルート、ポート探査、DNS 照会、速度測定、LAN スキャン |
| 通知センター | Webhook、Telegram、Bark、ntfy、Gotify、Pushover、Discord、Slack、ServerChan、PushPlus、WeCom、DingTalk、Feishu、メール。レベル、ワイルドカード、サイレント・重複排除ルール。組み込みルールテンプレート（重大な障害、証明書の期限、アドレス変更、ログイン、設定変更） |
| 設定バックアップ | 暗号化バックアップ（scrypt + AES-GCM）、保持ポリシー、ローカルディレクトリやストレージのマウントポイントへの定期バックアップ |
| コマンドラインと MCP | `gately ctl` で稼働中のインスタンスを操作、`gately mcp` で stdio 経由の MCP サービスを提供（既定は読み取り専用） |
| Wake-on-LAN | マジックパケットのブロードキャスト、オンライン検出 |
| WebDAV | 複数ユーザー、ユーザーごとのディレクトリ、読み取り専用。WebDAV・FTP・SMB で共通の共有ユーザー |
| FTP | 複数ユーザー、ユーザーごとのディレクトリ、読み取り専用、FTPS とパッシブポート範囲に対応。WebDAV・FTP・SMB で共通の共有ユーザー |
| SMB | 複数ユーザー、ユーザーごとに複数ディレクトリを共有、読み取り専用、SMB 2/3・暗号化・Bonjour 検出に対応。WebDAV・FTP・SMB で共通の共有ユーザー |
| File Browser | Web ファイルマネージャー FileBrowser Quantum をホスト。複数インスタンス、複数の共有ディレクトリと読み取り専用、パスワードまたはリバースプロキシ認証。オンラインでインストール・更新・ロールバック・アンインストール。デフォルト設定に戻すことも可能 |
| ストレージマウント | ローカルディレクトリ、リモート WebDAV、SMB 共有のマウント、オンラインファイル管理 |
| スケジュールタスク | cron 式、コマンドまたは HTTP リクエスト、実行履歴 |
| セキュリティと設定 | パスキー（WebAuthn）ログイン、2 段階認証（TOTP）、読み取り専用アカウント、ログイン失敗のレート制限、セキュアエントリー、内部ネットワークのみのアクセス、管理画面の HTTPS、複数の API トークン（モジュール単位の読み書き権限）、保存済みのパスワードやキーの表示（書き込み権限が必要）、カスタム DNS（DoH 対応）、タイムゾーン、設定のバックアップと復元、ローカルのリバースプロキシから実際のクライアントアドレスを取得、機能モジュールの個別オン／オフ（無効にしたモジュールはサービスを起動せず、API も提供しません） |
| fnOS ネイティブアプリ | オフライン `.fpk` パッケージ（x86 / ARM）。fnOS 統一ゲートウェイに統合され、fnOS デスクトップのアイコンから開け、fnOS のリモートドメインでも利用可能。fnOS 管理者は fnOS のログイン状態でそのままログイン可能（既定は無効）。root で動作し、80/443 の待ち受けと Docker の管理が可能 |
| 監査と履歴 | ログイン、設定変更、機密情報へのアクセスを記録する監査ログ（リクエスト本文やシークレットは一切記録しません）。上書きされた設定ファイルは以前のバージョンを保持し、差分表示とワンクリックでのロールバックが可能。GitOps 向けの宣言的設定ディレクトリ（`-apply`） |
| 監視とセルフチェック | グローバルな Prometheus エンドポイント `/api/metrics`（証明書、DDNS、STUN、ポート転送、Web サービス）とすぐに使える Grafana ダッシュボード。ワンクリックのネットワークセルフチェック（グローバル IP、CGNAT、時刻、80/443 ポート、DDNS の名前解決、証明書の期限、HTTPS サイトの証明書不足、frp プロキシの競合） |
| 移行 | nginx 設定のインポート（`nginx -T` の出力に対応）: リバースプロキシと upstream、静的ディレクトリ、リダイレクト、パスごとのバックエンド、アクセス制御、`auth_request` はフォワード認証に変換 |

</details>

## なぜ Gately なのか

Gately は完全なオープンソースで、ユーザーとコミュニティがソースコードと設定を直接確認できます。ネットワークの境界で動作し、DNS の鍵や証明書の秘密鍵を扱うソフトウェアだからこそ、オープンソースであることで何をしているかを自分の目で確かめられます。Gately の約束:

- ソースコードは Apache-2.0 で公開し、コミュニティによるレビューを歓迎します
- 既定ではプロジェクト側のサーバーに一切接続せず、デバイス情報も送信しません。サードパーティログインにはご自身の OAuth アプリを使用し、中継は経由しません
- 外部へのアドレス（パブリック IP 取得、STUN サーバーなど）はすべて確認・変更・無効化できます
- 設定はプレーンテキストの JSON なので、そのまま git に入れて監査やロールバックができます

## パフォーマンス

ポート転送の SNI 振り分けと Web サービスを組み合わせると、よくある haproxy（SNI による TCP 振り分け）+ nginx（TLS とリバースプロキシ）の構成を置き換えられます。下の表は、実際のサーバーに近い設定で、同じマシン上の両者を比較したものです。90 個のドメインを SNI で振り分け、2 つのホップ間は PROXY プロトコル v2、証明書は ECDSA P-256、TLS 1.3 を使用しています。

| シナリオ | haproxy + nginx | Gately |
| --- | --- | --- |
| HTTPS リバースプロキシ、Keep-Alive、1 KB のレスポンス、64 接続 | 14,767 req/s（p99 35.9 ms） | **26,434 req/s**（p99 5.7 ms） |
| 同条件で HTTP/2 | 未テスト（使用した nginx の設定は HTTP/1.1） | 27,795 req/s |
| リクエストごとに新しい TLS 接続、32 並列 | 2,557 req/s | **3,448 req/s** |
| 1 MB のレスポンス、16 接続 | 1,053 MB/s | **1,585 MB/s** |
| TLS ストリームの SNI パススルー、4 接続 | 2,927 MB/s | **4,962 MB/s** |
| SNI パススルー、1 秒あたりの新規接続数、64 並列 | 6,334 /s | 6,266 /s（ほぼ同等） |
| 負荷時のメモリ | haproxy ≈ 21 MB + nginx ≈ 21 MB | ≈ 110 MB（全モジュールを含む単一プロセス） |

<details>
<summary><b>測定方法</b></summary>

- ホスト：Intel Core i5-13500T。小規模なクラウドサーバーに合わせて、各構成を CPU 2 コアに制限しました（haproxy と nginx は Docker の `--cpuset-cpus`、Gately は `taskset` で固定）。バックエンドと負荷生成ツールは別のコアで動かしました。
- haproxy は `mode tcp`、`req_ssl_sni` ルール、`send-proxy-v2` を使用しました。nginx は `worker_processes 2` と本番サイトのプロキシ設定を使用しており、この設定では `Connection: upgrade` を送るため、リクエストごとにバックエンドへの新しい接続が作られます。アップストリームの Keep-Alive を有効にしてチューニングした nginx であれば、小さなレスポンスでの差は縮まります。
- Gately の経路は、SNI ルール、PROXY v2、Web サービスのリスナーの順で、haproxy → nginx と同じホップ数です。
- 各ケースは 10 秒間実行し、繰り返したケースの結果は 3% 以内で一致しました。実際の 2 vCPU のクラウドホストでは、静的サイトは 5.5k req/s 対 3.4k req/s でした。クライアントが X25519 を使うと、新規 TLS 接続は同等（どちらも毎秒約 480〜550）でした。Go のクライアントや最近のブラウザが提供するポスト量子ハイブリッド鍵交換を使うと、この測定では Gately のハンドシェイク完了数は約 13% 少なくなりました。
- 結果はハードウェア、設定、トラフィックによって変わります。Gately が haproxy と nginx に引けを取らないことの目安として捉え、実際のワークロードで測定してください。

</details>

## 開発

必要環境: Go 1.26+、Node.js 24+、pnpm。

```bash
make web      # フロントエンドをビルド（web/dist に出力、go:embed で埋め込み）
make build    # bin/gately をコンパイル
make test     # go test -race ./...
make lint     # gofmt、go vet、フロントエンドの型チェック
```

フロントエンド開発時は `cd web && pnpm dev` を実行します。`/api` はローカルの `127.0.0.1:16680` にプロキシされます。

一部のテストは既定でスキップされ、追加の条件が必要です。

| テスト | 有効化方法 |
| --- | --- |
| ACME エンドツーエンド（Pebble） | pebble と pebble-challtestsrv をインストールし、`GATELY_PEBBLE_BIN`、`GATELY_PEBBLE_SRC` を設定（CI では自動実行） |
| 実際のパブリック STUN サーバー | `GATELY_NET_TEST=1` |

ディレクトリ構成と技術選定は [docs/ja/ROADMAP.md](docs/ja/ROADMAP.md) を参照してください。

公式サイト（[gately.cn](https://gately.cn)）のソースは `website/` にあり、VitePress で構築されています: `cd website && pnpm install && pnpm dev`。`pnpm deploy:site` でビルドしてサーバーに同期します。

## コントリビューションの方針

- 互換性のためのコードは、公開されている観察可能な挙動、インターフェース、設定形式のみに基づいてください。サードパーティ製ソフトウェアのバイナリやフロントエンド成果物から複製したコードは提出しないでください。
- 新しいモジュールには、設定構造とバリデーション、実行時の起動・停止、REST API、ユニットテストが必要です。概要ページにステータスを表示したいモジュールは `module.Summarizer` を実装してください。
- コードコメントは中国語で記述し、制御文には必ず波括弧を使用します。

## コミュニティ

使い方の質問やフィードバックは Telegram グループへどうぞ：[t.me/gately_chat](https://t.me/gately_chat)。スマートフォンで QR コードを読み取って参加することもできます。

<a href="https://t.me/gately_chat"><img src="docs/images/telegram-qr.svg" alt="QR コードを読み取って Gately の Telegram グループに参加" width="160"></a>

不具合は [GitHub Issues](https://github.com/wangwanjie/gately/issues) に報告し、[診断パッケージ](docs/ja/DIAGNOSTICS.md)（設定 → 診断）を添付してください。診断パッケージはローカルで生成され、パスワード、トークン、ドメイン、実際の IP は含まれません。

## スポンサー

Gately のすべての機能は無料です。役に立ったと感じたら、USDT、[愛発電](https://ifdian.net/a/gately)、WeChat Pay で開発を支援できます。アドレスと QR コードは[スポンサーページ](https://gately.cn/ja/sponsor)にあります。

## Star 履歴

[![Star History Chart](https://api.star-history.com/svg?repos=wangwanjie/gately&type=Date)](https://star-history.com/#wangwanjie/gately&Date)

## ライセンス

[Apache License 2.0](LICENSE)
