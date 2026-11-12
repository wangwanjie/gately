# モジュール一覧

Gately の各機能は独立したモジュールで、個別に有効化・無効化・設定でき、設定はそれぞれ 1 つの平文 JSON ファイルに保存されます。

「設定 → 機能モジュール」でモジュール全体を無効にできます。無効にしたモジュールは設定を読み込みますが、サービスを起動せず、管理 API も提供せず（エンドポイントは 403 を返します）、画面からも非表示になります。設定は保持され、再び有効にすると元に戻ります。他のモジュールが依存しているモジュールは無効にできず、変更は再起動後に反映されます。

## カスタムビルド {#custom-build}

ポート転送、Web サービス、SSL 証明書の 3 つのコアモジュール以外は、すべてコンパイル時に取り除けます。画面の言語も簡体字中国語のみ、または簡体字中国語・繁体字中国語・英語の 3 言語だけに絞れます。その分パッケージとメモリ使用量が小さくなり、ストレージの限られたルーターに向いています。[build.gately.cn](https://build.gately.cn) でモジュールと言語を選ぶと、バイナリ、OpenWrt パッケージ、fnOS パッケージ、マルチアーキテクチャの Docker イメージをオンラインで生成でき、ページにサイズの見積もりも表示されます。[ソースからビルド](/ja/download#custom-build) の手順で自分でコンパイルすることもできます。

カスタムビルドに含まれていないモジュールについて：設定ファイルはそのまま残り、設定のスイッチや API トークンの権限も引き続き有効です。API は 404 を返してこのビルドに含まれていないことを示し、画面では該当するページと関連オプション（サイトの WAF、証明書の SSH や Docker へのデプロイなど）が非表示になります。完全版に戻すと元に戻ります。「バージョン情報」ページにはこのビルドで取り除いたモジュールが一覧表示され、同じオプションでビルダーを開くリンクも付いています。カスタムビルドはオンラインアップグレードできません（オンラインアップグレードがダウンロードするのは完全版です）。新しいバージョンが出たらビルダーで同じオプションのまま生成し直し、「アップグレードパッケージをアップロード」でアップグレードしてください。

| モジュール | 機能 | ドキュメント |
| --- | --- | --- |
| ダイナミック DNS | 11 の DNS プロバイダー、URL / NIC / コマンドで IP 取得、IPv4 / IPv6、Webhook | [開く](/ja/docs/modules/ddns) |
| STUN 穿孔 | TCP / UDP、キープアライブ、UPnP / NAT-PMP、内蔵フォワーディング、変化時の Webhook とスクリプト実行 | [開く](/ja/docs/modules/stun) |
| ポート転送 | TCP / UDP、IPv4 / IPv6、ポート範囲、複数ターゲットのフェイルオーバー、接続数制限、トラフィック統計 | [開く](/ja/docs/modules/portforward) |
| プロトコル振り分け | 複数のサービスで 1 つのポートを共用：TLS は SNI と ALPN、HTTP は Host、Minecraft はサーバーアドレスで振り分け、SSH、RDP、SOCKS、OpenVPN、RTSP、SIP、XMPP、MQTT、AMQP、PostgreSQL、Redis、SMB、Git、IRC、ADB、BitTorrent も判別、VNC などは待機タイムアウトのルートへ | [開く](/ja/docs/modules/protoroute) |
| SSL 証明書 | ACME DNS-01 による自動発行・更新、アップロード、ディレクトリへのマッピングとスクリプト実行 | [開く](/ja/docs/modules/ssl) |
| Web サービス | リバースプロキシ、リダイレクト、静的ファイル、HTTPS、認証、アクセス制御、負荷分散とヘルスチェック、キャッシュとレート制限、アクセス統計、ナビゲーションパネル、訪問者監査、ネットワークトポロジー | [開く](/ja/docs/modules/web) |
| OIDC シングルサインオン | 共有ユーザーを OpenID Connect で Gitea、OpenList、Grafana などのアプリに提供し、Authelia の OIDC の代わりになる | [開く](/ja/docs/modules/oidc) |
| WAF ファイアウォール | Coraza + OWASP CRS v4 | [開く](/ja/docs/modules/waf) |
| 侵入防御 | ログルール（fail2ban 風）、おとりポート、ポートスキャン検出。ブロックは nftables に同期 | [開く](/ja/docs/modules/guard) |
| ポートノッキング | fwknop 互換のシングルパケット認可（SPA）。SSH などのポートをファイアウォールでブロックし、ポート転送・プロトコル振り分け・STUN もノックで許可 | [開く](/ja/docs/modules/knock) |
| Tailscale / NetBird / EasyTier / frp / cloudflared | 公式バイナリを管理するネットワーク接続と NAT 越え | [開く](/ja/docs/modules/tunnels) |
| Docker 管理 | コンテナの作成と編集、ログとターミナル、Compose、イメージの更新とアップグレード、ネットワークとボリューム | [開く](/ja/docs/modules/docker) |
| Web ターミナル | SSH、Telnet、ローカルシェル、SFTP ファイル管理、永続セッション | [開く](/ja/docs/modules/terminal) |
| ネットワーク診断 | Ping、トレースルート、ポート検査、DNS 照会、速度測定、LAN スキャン | [開く](/ja/docs/modules/nettools) |
| 通知センター | 複数チャネルへのイベント通知とルール | [開く](/ja/docs/modules/notify) |
| パフォーマンス監視 | ホスト・プロセス・サイトの推移（最初のバイトまでの時間を含む）、しきい値と継続時間によるアラート | [開く](/ja/docs/modules/perf) |
| 設定バックアップ | 暗号化バックアップ、保持ポリシー、定期バックアップと復元 | [開く](/ja/docs/modules/backup) |
| ファイルと運用ツール | WebDAV、FTP、SMB、File Browser、ストレージマウント、Wake-on-LAN、スケジュールタスク | [開く](/ja/docs/modules/tools) |
