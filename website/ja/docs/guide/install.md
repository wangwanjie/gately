# インストールとデプロイ

## Docker

イメージのアドレスは `ghcr.io/wangwanjie/gately` で、Docker Hub にも `vanjayak/gately` として公開しています（同じイメージで、タグも共通です）。amd64、arm64、arm/v7 に対応しています。GHCR にアクセスしにくい場合は、以下のイメージ名を `vanjayak/gately:latest` に置き換えてください。Docker のレジストリミラーとも組み合わせられます。

```bash
docker run -d --name gately --network host --cap-add NET_ADMIN \
  -v ./gately-data:/etc/gately \
  -v /var/run/docker.sock:/var/run/docker.sock \
  ghcr.io/wangwanjie/gately:latest
```

### Docker Compose

```yaml
services:
  gately:
    image: ghcr.io/wangwanjie/gately:latest
    container_name: gately
    restart: unless-stopped
    # ポート転送、STUN 穿孔、Web サービスはいずれもホストのポートを直接待ち受ける必要があります
    network_mode: host
    # 侵入防御がブロックをホストのファイアウォールに書き込みます（不要なら削除）
    cap_add:
      - NET_ADMIN
    volumes:
      # 設定ディレクトリ（平文の JSON。git でのバックアップも可能。ssl.json には証明書の秘密鍵が含まれます）
      - ./data:/etc/gately
      # Docker 管理（不要なら削除してください）
      - /var/run/docker.sock:/var/run/docker.sock
    environment:
      - TZ=Asia/Shanghai
```

`--cap-add NET_ADMIN` と `cap_add` は、[侵入防御](/ja/docs/modules/guard)がブロックをホストの nftables ファイアウォールに書き込むためのものです。不要なら外してもかまいません。その場合もブロックは Gately 自身のサービスに適用されます。

イメージには `sh`、`bash`、`curl` が含まれているため、一般的な通知スクリプトはそのまま実行できます。

## バイナリ

1. [ダウンロードページ](/ja/download) から使用するプラットフォーム向けのアーカイブを取得します。対応は Linux（amd64 / arm64 / arm / mips / mipsle / riscv64）、macOS、Windows、FreeBSD です。
2. 展開して次のコマンドを実行します。

```bash
./gately -c /etc/gately
```

### systemd

リポジトリには完全なサービスファイル [deploy/systemd/gately.service](https://github.com/wangwanjie/gately/blob/main/deploy/systemd/gately.service) が用意されており、低い番号のポートで待ち受けるために必要なケーパビリティも付与済みです。

```bash
sudo install -m 0755 gately /usr/local/bin/gately
sudo install -m 0644 gately.service /etc/systemd/system/gately.service
sudo systemctl daemon-reload && sudo systemctl enable --now gately
```

### OpenWrt {#openwrt}

各リリースには OpenWrt 向けパッケージが含まれます。バイナリ、procd 用の起動スクリプト、ファームウェア更新時に設定を保持するルールが入っています。

- OpenWrt 24.10 以前（iStoreOS など 24.10 ベースのファームウェアを含む）は `.ipk` を使い、`opkg install` でインストールします。
- OpenWrt 25.12 以降は `.apk` を使い、`apk add --allow-untrusted` でインストールします（パッケージは署名されていないため、`--allow-untrusted` が必要です）。

`grep DISTRIB_ARCH /etc/openwrt_release` でデバイスのアーキテクチャを確認し、次の表に従って [ダウンロードページ](/ja/download) の OpenWrt グループからパッケージを選びます。

| DISTRIB_ARCH | パッケージ |
| --- | --- |
| `x86_64` | `gately_<version>_openwrt_amd64` |
| `aarch64_*` | `gately_<version>_openwrt_arm64` |
| `arm_cortex-a*_*`（VFP / NEON 対応の ARMv7） | `gately_<version>_openwrt_arm` |
| `mipsel_*` | `gately_<version>_openwrt_mipsle` |
| `mips_*` | `gately_<version>_openwrt_mips` |
| `riscv64_*` | `gately_<version>_openwrt_riscv64` |

```bash
cd /tmp
# OpenWrt 24.10 / iStoreOS（x86_64 の例）
opkg install ./gately_<version>_openwrt_amd64.ipk
# OpenWrt 25.12
apk add --allow-untrusted ./gately_<version>_openwrt_amd64.apk
```

- インストール後に自動起動が有効になり、サービスがすぐに起動します。管理画面は `http://<ルーターのアドレス>:16680` です。設定は `/etc/gately` に保存され、ファームウェアの更新（sysupgrade）後も保持されます。
- アップグレード：新しいパッケージをダウンロードし、もう一度 `opkg install` または `apk add --allow-untrusted` を実行します。設定は保持されます。自動起動を無効にしていた場合は、アップグレード後も無効のままです。
- アンインストール：`opkg remove gately` または `apk del gately` を実行します。設定ディレクトリは残るため、完全に削除するには `rm -rf /etc/gately` も実行してください。
- インストール前にデバイスのアーキテクチャを確認します。アーキテクチャが合わない場合、opkg はインストールを中止します。OpenWrt 25.12 の apk はパッケージ自体はインストールしますがサービスを起動しないため、`apk del gately` を実行してから正しいパッケージを入れ直してください。
- バイナリは約 32 MB で、同程度の空き容量が必要です（`df -h /overlay` で確認できます）。x86 のソフトルーターや ARM64 ボックスなどストレージに余裕のあるデバイス向けで、フラッシュが 16 MB / 32 MB のルーターには収まりません。
- arm パッケージにはハードウェア浮動小数点に対応した ARMv7 が必要です。ARMv5 / ARMv6 のデバイスや、`arm_cortex-a9` のような FPU を持たないコアには現在対応していません。

手動でデプロイすることもできます。Linux 用のアーカイブをダウンロードし、procd 用の起動スクリプト [deploy/openwrt/gately.init](https://github.com/wangwanjie/gately/blob/main/deploy/openwrt/gately.init) を使います。

```bash
cp gately /usr/bin/gately && chmod +x /usr/bin/gately
cp gately.init /etc/init.d/gately && chmod +x /etc/init.d/gately
/etc/init.d/gately enable && /etc/init.d/gately start
```

#### LuCI アプリ {#openwrt-luci}

各リリースにはアーキテクチャに依存しない LuCI アプリ `luci-app-gately_<バージョン>_openwrt_all.ipk` / `.apk` も含まれます。Gately をインストールしたあとに導入します：

```bash
# OpenWrt 24.10 / iStoreOS
opkg install ./luci-app-gately_<バージョン>_openwrt_all.ipk
# OpenWrt 25.12
apk add --allow-untrusted ./luci-app-gately_<バージョン>_openwrt_all.apk
```

インストール後、LuCI の「サービス → Gately」で次の操作ができます：

- 実行状態、バージョン、メモリ使用量、自動起動の状態を確認する；
- サービスの開始・停止・再起動、自動起動の有効化・無効化；
- Gately の管理画面をワンクリックで開く（グローバル設定のセキュリティ入口もリンクに含まれます）；
- 管理画面のポートを設定する。ポートは `/etc/config/gately` に保存され、入力するとグローバル設定の待ち受けアドレスのポートを置き換えます（ループバックや LAN アドレスなどのホスト部分はそのまま）。ポートの競合や設定ミスで管理画面が開けないときにここで戻せます。空欄ならグローバル設定に従います。

その他の機能は引き続き Gately の管理画面で設定します。アプリは LuCI の言語設定に従い、簡体字中国語、繁体字中国語、英語、日本語、韓国語、ドイツ語、フランス語、スペイン語、ロシア語、ポルトガル語（ブラジル）に対応しています。アンインストールするときは先に `opkg remove luci-app-gately` または `apk del luci-app-gately` を実行し、そのあと gately を削除します。

## fnOS ネイティブアプリ {#fnos}

各リリースには fnOS 向けのオフラインパッケージ `gately_<version>_fnos_x86.fpk`（Intel / AMD）と `gately_<version>_fnos_arm.fpk`（ARM）が含まれます。公式のアプリセンターには掲載されていないため、手動でインストールします。

1. [ダウンロードページ](/ja/download#fnos) から、デバイスのアーキテクチャに合った `.fpk` を取得します。
2. fnOS で「アプリセンター → 手動インストール」（左下のボタン）を開き、ファイルを選んでウィザードを完了します。
3. fnOS デスクトップの Gately アイコンをクリックすると、管理画面が開きます。

### fnOS 統一ゲートウェイに対応

デスクトップのアイコンは、fnOS へのアクセスに使っているアドレスの `/app/gately/` で、fnOS 統一ゲートウェイ経由で管理画面を開き、同じアクセス入口を共用します。LAN のアドレスでも fnOS のリモートドメインでもそのまま使え、16680 ポートを開放・転送する必要はありません。

ゲートウェイ経由のアクセスは fnOS が制御し、fnOS 管理者だけが開けます。セキュアエントリーと「LAN 内からのみ許可」は 16680 ポートへの直接アクセスにのみ適用されます。LAN 内では `http://<NAS のアドレス>:16680` も引き続き使えます。

### fnOS 管理者のログイン状態でログイン

「設定 → アクセス制御」で「fnOS のログイン状態で自動ログイン」を有効にすると、fnOS にログイン済みの管理者はデスクトップのアイコンからパスワードや 2 段階認証コードなしでそのまま Gately に入れます。

- 自動ログインの対象は fnOS 管理者だけで、Gately の管理者としてログインします。一般の fnOS ユーザーは引き続きユーザー名とパスワードを入力します。
- 統一ゲートウェイ経由のアクセスにのみ適用されます。16680 ポートへの直接アクセスでは通常のログインが必要です。
- ログアウトした後は自動で再ログインせず、ログイン画面に「fnOS アカウント &lt;ユーザー名&gt; でログイン」ボタンが表示されます。
- この設定は既定で無効で、fnOS パッケージでのみ表示されます。

### 注意事項

- アプリは `root` で動作するため、80/443 の待ち受け、`docker.sock` を通じた Docker の管理、実際の Web ターミナルが利用できます。自分で管理している NAS にのみインストールしてください。
- 設定はアプリの `var` ディレクトリに保存され、アンインストール時も保持されます。アンインストールウィザードで「すべて削除」を選んだ場合のみ消去されます。
- パッケージには `/usr/local/bin/gately` コマンドも含まれ、`gately ctl`、`gately mcp`、`gately reset-password` を使えます。
- 16680 ポートは Docker イメージと共通です。ネイティブアプリを起動する前に `gately` コンテナを停止してください。

## 初回ログイン

管理画面はデフォルトで `16680` ポートを待ち受けます。デフォルトのアカウントは `admin`、パスワードは `gately.cn` です。自動デプロイでは環境変数 `GATELY_INITIAL_PASSWORD` で、デフォルトのパスワードの代わりに使う初期パスワードを指定できます。

デフォルトのパスワードは公開されているため、使用している間は管理画面の上部に変更を促す警告が表示され続けます。初回ログイン時には初期設定ガイドが表示され、パスワードの変更、2 段階認証または Passkey の有効化、ネットワーク自己診断の実行、Lucky からのインポート、通知の設定を順に案内します。

## パスワードと 2 段階認証のリセット {#reset}

管理者パスワードを忘れたときや、2 段階認証（TOTP）のアプリを紛失したときは、Gately が稼働しているサーバー上でコマンドを実行してリセットします。コマンドは設定ディレクトリの読み書きが必要で、サーバーの権限を持つことと同等なため、ネットワーク経由では使えません。

- `gately reset-password`：管理者パスワードをデフォルトの `gately.cn` にリセットします。`-p <新しいパスワード>` で新しいパスワードを直接指定することもできます。
- `gately reset-totp`：管理者の 2 段階認証を無効にします。ログイン後に再び有効にできます。

実行前に管理者のユーザー名と実行する操作が表示され、誤操作を防ぐため `yes` と入力して確認する必要があります。スクリプトなどの非対話環境では `-y` を付けると確認を省略できます。稼働中の Gately には再起動なしで数秒以内に反映され、稼働していない場合は次回起動時に反映されます。リセット後はログイン中のすべてのセッションが無効になり、ログイン失敗によるロックも解除されます。ユーザー名、Passkey、読み取り専用アカウント、その他の設定は変わりません。

| インストール方法 | コマンド |
| --- | --- |
| Docker | `docker exec -it gately gately reset-password` |
| systemd | `sudo gately reset-password` |
| OpenWrt / iStoreOS | `gately reset-password` |
| fnOS | `sudo gately reset-password` |
| その他 | `./gately reset-password -c <設定ディレクトリ>` |

`-c` を指定しない場合は、`GATELY_CONFIG_DIR`、カレントディレクトリの `data`、`/etc/gately`、fnOS パッケージの設定ディレクトリ `/var/apps/gately/var/config` の順に探し、実際に使ったディレクトリを表示します。リセット操作はシステムログと監査ログに記録されます。

## アップグレード

### 自動更新チェックとセキュリティ通知 {#update-check}

Gately はデフォルトで 12 時間ごとに GitHub にアクセスし、Gately のリリース一覧と[公開済みのセキュリティアドバイザリー](https://github.com/wangwanjie/gately/security/advisories)を読み込み、オンラインインストール済みの管理プログラム（frp、Cloudflare トンネル、Tailscale、NetBird、File Browser）の最新バージョンも照会します。リクエストはすべて匿名の GET で、端末・設定・利用状況などの情報は一切含みません。

- **新しいバージョン**：管理画面の上部に表示され、更新内容を確認したり、そのバージョンをスキップしたりできます；
- **セキュリティ更新**：現在のバージョンがアドバイザリーの影響範囲に含まれる場合や、現在より新しいリリースのノートにセキュリティ修正の章がある場合、上部に赤い通知で該当するアドバイザリーとバージョンを表示します。セキュリティ通知は 24 時間までしか延期できず、新しいセキュリティ問題が出るとすぐに再表示されます；
- **通知**：新しいバージョンや新しいセキュリティ問題が見つかると `system.update` / `system.security` イベントを、管理プログラムの新しいバージョンでは `program.update` を発行します。[通知センター](/ja/docs/modules/notify)で「アップデート」テンプレートからルールを追加すると受け取れます。同じバージョン・同じセキュリティ問題の通知は 1 回だけで、再起動しても繰り返しません。

結果は設定ディレクトリの `update-check.json` に保存され、アップグレード後は再チェックなしで新しいバージョンに基づいて判定し直します。最初のチェックは起動の約 3 分後に行い（前回の成功から 12 時間未満の場合は 12 時間経つまで待ちます）、失敗した場合は 1 時間後に再試行します。GitHub へのアクセスには後述の更新プロキシを使います。自動チェックは「About」ページまたは「設定 → ネットワークと実行」で無効にでき、無効にすると「更新を確認」をクリックしたときだけ GitHub にアクセスします。自動チェックは通知するだけで、ダウンロードやインストールは一切行いません。「アップデートを確認」ボタンは管理者にのみ表示されます。読み取り専用アカウントと読み取り権限のみの API トークンがチェック API を呼ぶと、GitHub にアクセスせず前回の結果を返します。

### オンラインアップグレード {#online-upgrade}

Linux バイナリ導入（systemd、PVE の LXC コンテナ、手動で導入した OpenWrt など）では、管理画面の「バージョン情報」ページからオンラインでアップグレードできます。「アップデートを確認」をクリックし、新しいバージョンが見つかったら「今すぐアップグレード」をクリックします。PVE で OCI イメージから作成した LXC コンテナも対象で、削除して作り直す必要はありません。Gately は次の順に処理します。

1. このプラットフォーム用の `gately_<バージョン>_linux_<アーキテクチャ>.tar.gz` を GitHub からダウンロードします。
2. 同じリリースの `checksums.txt` と SHA-256 を照合します。
3. 新しいバイナリを `-version` で試験実行し、このマシンで動くことを確認します。
4. 実行ファイルを置き換えて同じ引数で再起動します。旧バージョンは同じディレクトリに `gately.old` として残ります。

すべてダイアログ内で進みます。まずそのバージョンの更新内容が表示され、開始後は現在のステップ、ダウンロードの進捗と速度が表示されます。プログラムを置き換える前（バージョン取得・ダウンロード・検証の段階）なら、いつでも「アップグレードを取消」をクリックでき、現在のプログラムは変更されません。

GitHub への接続が不安定な場合は、「バージョン情報」ページまたは「設定 → ネットワークと実行」で**更新用プロキシ**を設定できます。`http://`、`https://`、`socks5://` に対応し、更新確認とダウンロードの両方がこのプロキシを経由します。空欄の場合は直接接続し、`HTTPS_PROXY` 環境変数があればそれに従います。

インターネットに接続できない環境では、[ダウンロードページ](/ja/download) から tar.gz を入手し、「バージョン情報」ページの「アップグレードパッケージをアップロード」でアップグレードできます。アップロードはバージョンの新旧を問わないため、旧バージョンへのロールバックにも使えます。

[カスタムビルド](/ja/docs/modules/#custom-build)ではオンラインアップグレードを利用できません。オンラインアップグレードがダウンロードするのは完全版で、取り除いたモジュールが戻ってしまうためです。新しいバージョンが出たら [build.gately.cn](https://build.gately.cn) で同じオプション（「バージョン情報」ページのリンクに含まれています）のまま生成し直し、「アップグレードパッケージをアップロード」でアップグレードしてください。Docker のカスタムイメージは新しいタグを pull するだけです。

オンラインアップグレードは管理者が操作したときだけ実行され、Gately が自動でダウンロードやインストールを行うことはありません。新しいバージョンが起動しない場合は、残しておいた旧バージョンで手動でロールバックできます（systemd の例）。

```bash
mv /usr/local/bin/gately.old /usr/local/bin/gately
systemctl restart gately
```

`setcap` でバイナリに低番号ポートの権限を付与している場合は、アップグレード後にもう一度 `setcap` を実行してください。

### その他のデプロイ方式

- Docker：新しいイメージを `docker pull` してからコンテナを再作成します（設定ディレクトリをマウントしたままにしておけば問題ありません）。
- fnOS：アプリセンターで新しい `.fpk` を手動インストールします。設定は保持されます。
- OpenWrt パッケージ：新しい `.ipk` / `.apk` をダウンロードして `opkg install` / `apk add --allow-untrusted` を実行します。設定は保持されます（[OpenWrt](#openwrt) を参照）。
- Windows、macOS、FreeBSD：実行ファイルを置き換えて、サービスを再起動します。

アップグレード前に設定のバックアップをエクスポートするか、設定ディレクトリごと git に入れておくことをおすすめします。ダウンロードしたアーカイブやイメージは、[リリースの検証](/ja/docs/guide/verify) に従って署名を確認できます。
