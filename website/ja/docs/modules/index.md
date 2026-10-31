# モジュール一覧

Gately の各機能は独立したモジュールで、個別に有効化・無効化・設定でき、設定はそれぞれ 1 つの平文 JSON ファイルに保存されます。

「設定 → 機能モジュール」でモジュール全体を無効にできます。無効にしたモジュールは設定を読み込みますが、サービスを起動せず、管理 API も提供せず（エンドポイントは 403 を返します）、画面からも非表示になります。設定は保持され、再び有効にすると元に戻ります。他のモジュールが依存しているモジュールは無効にできず、変更は再起動後に反映されます。

| モジュール | 機能 | ドキュメント |
| --- | --- | --- |
| ダイナミック DNS | 11 の DNS プロバイダー、URL / NIC / コマンドで IP 取得、IPv4 / IPv6、Webhook | [開く](/ja/docs/modules/ddns) |
| STUN 穿孔 | TCP / UDP、キープアライブ、UPnP / NAT-PMP、内蔵フォワーディング、変化時の Webhook とスクリプト実行 | [開く](/ja/docs/modules/stun) |
| ポート転送 | TCP / UDP、IPv4 / IPv6、ポート範囲、複数ターゲットのフェイルオーバー、接続数制限、トラフィック統計 | [開く](/ja/docs/modules/portforward) |
| プロトコル振り分け | 複数のサービスで 1 つのポートを共用：TLS は SNI と ALPN、HTTP は Host、Minecraft はサーバーアドレスで振り分け、SSH、RDP、SOCKS、OpenVPN、RTSP、SIP、XMPP、MQTT、AMQP、PostgreSQL、Redis、SMB、Git、IRC、ADB、BitTorrent も判別、VNC などは待機タイムアウトのルートへ | [開く](/ja/docs/modules/protoroute) |
| SSL 証明書 | ACME DNS-01 による自動発行・更新、アップロード、ディレクトリへのマッピングとスクリプト実行 | [開く](/ja/docs/modules/ssl) |
| Web サービス | リバースプロキシ、リダイレクト、静的ファイル、HTTPS、認証、アクセス制御、負荷分散とヘルスチェック、キャッシュとレート制限、アクセス統計 | [開く](/ja/docs/modules/web) |
| OIDC シングルサインオン | 共有ユーザーを OpenID Connect で Gitea、OpenList、Grafana などのアプリに提供し、Authelia の OIDC の代わりになる | [開く](/ja/docs/modules/oidc) |
| WAF ファイアウォール | Coraza + OWASP CRS v4 | [開く](/ja/docs/modules/waf) |
| 侵入防御 | ログルール（fail2ban 風）、おとりポート、ポートスキャン検出。ブロックは nftables に同期 | [開く](/ja/docs/modules/guard) |
| Tailscale / frp / cloudflared | 公式バイナリを管理するネットワーク接続と NAT 越え | [開く](/ja/docs/modules/tunnels) |
| Docker 管理 | コンテナの作成と編集、ログとターミナル、Compose、イメージの更新とアップグレード、ネットワークとボリューム | [開く](/ja/docs/modules/docker) |
| Web ターミナル | SSH、Telnet、ローカルシェル、SFTP ファイル管理、永続セッション | [開く](/ja/docs/modules/terminal) |
| ネットワーク診断 | Ping、トレースルート、ポート検査、DNS 照会、速度測定、LAN スキャン | [開く](/ja/docs/modules/nettools) |
| 通知センター | 複数チャネルへのイベント通知とルール | [開く](/ja/docs/modules/notify) |
| 設定バックアップ | 暗号化バックアップ、保持ポリシー、定期バックアップと復元 | [開く](/ja/docs/modules/backup) |
| ファイルと運用ツール | WebDAV、FTP、SMB、File Browser、ストレージマウント、Wake-on-LAN、スケジュールタスク | [開く](/ja/docs/modules/tools) |
