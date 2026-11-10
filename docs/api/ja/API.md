# Gately 管理 API

Gately 管理 API は、スクリプトや自動化による一括管理に使用します。

認証：管理コンソールの「API トークン」ページでトークンを作成し、リクエストヘッダーに `Authorization: Bearer gtl_xxxx` を付与してください。

権限：各トークンには作成時に選択した権限のみが付与されます。形式は `<モジュール>:read` または `<モジュール>:write` で、書き込み権限は読み取り権限を含みます。アカウント、2 要素認証、トークン管理、およびバックアップ、復元、再起動、システム設定のエンドポイントはログインセッションでのみ使用できます。

リクエストボディには `Content-Type: application/json` を使用します。エラーは一律 `{"error": "説明"}` の形式で返されます。

「設定 → 機能モジュール」で無効にしたモジュールは API を提供せず、そのエンドポイントは 403 を返します。実行中のインスタンスの API ドキュメントにも表示されなくなります。カスタムビルド（build.gately.cn）に含まれていないモジュールのエンドポイントは 404 を返し、そのモジュールはこのビルドに含まれていないと説明します。実行中のインスタンスの API ドキュメントにも同様に表示されません。

公式サイト：https://gately.cn

機械可読な定義は同じディレクトリの `openapi.json`（OpenAPI 3.0）を参照してください。

## クイックスタート

```bash
# システムステータスを表示（status:read が必要）
curl -H "Authorization: Bearer gtl_xxxx" http://localhost:16680/api/status

# すべての DDNS タスクを一覧表示（ddns:read が必要）
curl -H "Authorization: Bearer gtl_xxxx" http://localhost:16680/api/ddns/tasks

# DDNS タスクを即時同期（ddns:write が必要。key は前の手順で返された一覧から取得）
curl -X POST -H "Authorization: Bearer gtl_xxxx" http://localhost:16680/api/ddns/tasks/<key>/sync
```

## 権限

| 権限ドメイン | 名称 | 利用可能な権限 |
| --- | --- | --- |
| `portforward` | ポートフォワード | `portforward:read`、`portforward:write` |
| `protoroute` | プロトコル振り分け | `protoroute:read`、`protoroute:write` |
| `ddns` | ダイナミック DNS | `ddns:read`、`ddns:write` |
| `stun` | STUN 穿透 | `stun:read`、`stun:write` |
| `tailscale` | Tailscale ネットワーク | `tailscale:read`、`tailscale:write` |
| `netbird` | NetBird ネットワーク | `netbird:read`、`netbird:write` |
| `frp` | frp イントラネット穿透 | `frp:read`、`frp:write` |
| `cloudflared` | Cloudflare トンネル | `cloudflared:read`、`cloudflared:write` |
| `ssl` | SSL 証明書 | `ssl:read`、`ssl:write` |
| `waf` | WAF ファイアウォール | `waf:read`、`waf:write` |
| `webservice` | Web サービス | `webservice:read`、`webservice:write` |
| `guard` | 侵入防御 | `guard:read`、`guard:write` |
| `wol` | Wake-on-LAN | `wol:read`、`wol:write` |
| `webterminal` | Web ターミナル | `webterminal:read`、`webterminal:write` |
| `cron` | スケジュールタスク | `cron:read`、`cron:write` |
| `docker` | Docker 管理 | `docker:read`、`docker:write` |
| `webdav` | WebDAV サービス | `webdav:read`、`webdav:write` |
| `ftp` | FTP サービス | `ftp:read`、`ftp:write` |
| `smb` | SMB サービス | `smb:read`、`smb:write` |
| `filebrowser` | File Browser | `filebrowser:read`、`filebrowser:write` |
| `storage` | ストレージマウント | `storage:read`、`storage:write` |
| `notify` | 通知センター | `notify:read`、`notify:write` |
| `perf` | パフォーマンス監視 | `perf:read`、`perf:write` |
| `nettools` | ネットワーク診断 | `nettools:read`、`nettools:write` |
| `backup` | 設定バックアップ | `backup:read`、`backup:write` |
| `status` | システムステータス | `status:read` |
| `logs` | システムログ | `logs:read` |
| `metrics` | 監視メトリクス（Prometheus） | `metrics:read` |
| `import` | Lucky 設定のインポート | `import:write` |
| `fileusers` | ファイルサービス共有ユーザー | `fileusers:read`、`fileusers:write` |

ワイルドカード：`*:read` は全モジュールの読み取り専用、`*:write` または `*` は全モジュールの読み書きを表します。

## ポートフォワード

| メソッド | パス | 必要な権限 | 説明 |
| --- | --- | --- | --- |
| GET | `/api/portforward/logs` | `portforward:read` | モジュールの実行ログを表示 |
| GET | `/api/portforward/logs/sources` | `portforward:read` | モジュールログのすべてのソースを一覧表示 |
| GET | `/api/portforward/logs/export` | `portforward:read` | モジュールログをエクスポート |
| DELETE | `/api/portforward/logs` | `portforward:write` | モジュールの実行ログを消去 |
| GET | `/api/portforward/rules` | `portforward:read` | ポートフォワードルール の一覧を取得 |
| POST | `/api/portforward/rules` | `portforward:write` | ポートフォワードルール を追加 |
| POST | `/api/portforward/rules/{key}/copy` | `portforward:write` | ポートフォワードルール を複製 |
| PUT | `/api/portforward/rules/{key}` | `portforward:write` | ポートフォワードルール を更新 |
| DELETE | `/api/portforward/rules/{key}` | `portforward:write` | ポートフォワードルール を削除 |
| PUT | `/api/portforward/rules/{key}/enable` | `portforward:write` | ポートフォワードルール の有効/無効を切り替え |
| PUT | `/api/portforward/rules-order` | `portforward:write` | ポートフォワードルール の並び順を変更 |
| PUT | `/api/portforward/rules-enable` | `portforward:write` | ポートフォワードルール の一括有効化・無効化 |
| POST | `/api/portforward/rules-delete` | `portforward:write` | ポートフォワードルール の一括削除 |
| PUT | `/api/portforward/rules-group` | `portforward:write` | ポートフォワードルール のグループを一括設定 |
| GET | `/api/portforward/settings` | `portforward:read` | ポートフォワードのグローバル設定を読み取り |
| PUT | `/api/portforward/settings` | `portforward:write` | ポートフォワードのグローバル設定を保存 |

## プロトコル振り分け

| メソッド | パス | 必要な権限 | 説明 |
| --- | --- | --- | --- |
| GET | `/api/protoroute/logs` | `protoroute:read` | モジュールの実行ログを表示 |
| GET | `/api/protoroute/logs/sources` | `protoroute:read` | モジュールログのすべてのソースを一覧表示 |
| GET | `/api/protoroute/logs/export` | `protoroute:read` | モジュールログをエクスポート |
| DELETE | `/api/protoroute/logs` | `protoroute:write` | モジュールの実行ログを消去 |
| GET | `/api/protoroute/rules` | `protoroute:read` | プロトコル振り分けルール の一覧を取得 |
| POST | `/api/protoroute/rules` | `protoroute:write` | プロトコル振り分けルール を追加 |
| POST | `/api/protoroute/rules/{key}/copy` | `protoroute:write` | プロトコル振り分けルール を複製 |
| PUT | `/api/protoroute/rules/{key}` | `protoroute:write` | プロトコル振り分けルール を更新 |
| DELETE | `/api/protoroute/rules/{key}` | `protoroute:write` | プロトコル振り分けルール を削除 |
| PUT | `/api/protoroute/rules/{key}/enable` | `protoroute:write` | プロトコル振り分けルール の有効/無効を切り替え |
| PUT | `/api/protoroute/rules-order` | `protoroute:write` | プロトコル振り分けルール の並び順を変更 |
| PUT | `/api/protoroute/rules-enable` | `protoroute:write` | プロトコル振り分けルール の一括有効化・無効化 |
| POST | `/api/protoroute/rules-delete` | `protoroute:write` | プロトコル振り分けルール の一括削除 |
| PUT | `/api/protoroute/rules-group` | `protoroute:write` | プロトコル振り分けルール のグループを一括設定 |
| GET | `/api/protoroute/settings` | `protoroute:read` | プロトコル振り分けの全体設定を取得 |
| PUT | `/api/protoroute/settings` | `protoroute:write` | プロトコル振り分けの全体設定を保存 |

## ダイナミック DNS

| メソッド | パス | 必要な権限 | 説明 |
| --- | --- | --- | --- |
| GET | `/api/ddns/logs` | `ddns:read` | モジュールの実行ログを表示 |
| GET | `/api/ddns/logs/sources` | `ddns:read` | モジュールログのすべてのソースを一覧表示 |
| GET | `/api/ddns/logs/export` | `ddns:read` | モジュールログをエクスポート |
| DELETE | `/api/ddns/logs` | `ddns:write` | モジュールの実行ログを消去 |
| GET | `/api/ddns/tasks` | `ddns:read` | DDNS タスク の一覧を取得 |
| POST | `/api/ddns/tasks` | `ddns:write` | DDNS タスク を追加 |
| POST | `/api/ddns/tasks/{key}/copy` | `ddns:write` | DDNS タスク を複製 |
| PUT | `/api/ddns/tasks/{key}` | `ddns:write` | DDNS タスク を更新 |
| DELETE | `/api/ddns/tasks/{key}` | `ddns:write` | DDNS タスク を削除 |
| PUT | `/api/ddns/tasks/{key}/enable` | `ddns:write` | DDNS タスク の有効/無効を切り替え |
| PUT | `/api/ddns/tasks-order` | `ddns:write` | DDNS タスク の並び順を変更 |
| PUT | `/api/ddns/tasks-enable` | `ddns:write` | DDNS タスク の一括有効化・無効化 |
| POST | `/api/ddns/tasks-delete` | `ddns:write` | DDNS タスク の一括削除 |
| PUT | `/api/ddns/tasks-group` | `ddns:write` | DDNS タスク のグループを一括設定 |
| GET | `/api/ddns/tasks/{key}/secrets` | `ddns:write` | シークレットを平文で表示 |
| POST | `/api/ddns/tasks/{key}/sync` | `ddns:write` | DDNS タスクを即時同期 |
| DELETE | `/api/ddns/tasks/{key}/records/{rec}` | `ddns:write` | DDNS タスクのレコードを 1 件削除 |
| POST | `/api/ddns/tasks/{key}/remote-delete` | `ddns:write` | DDNS タスクの DNS レコードを DNS プロバイダーから削除 |
| PUT | `/api/ddns/tasks/{key}/records-layout` | `ddns:write` | タスク内の DNS レコードの順序とグループを調整 |
| GET | `/api/ddns/providers` | `ddns:read` | 対応する DNS プロバイダーと必要なパラメーターを一覧表示 |
| GET | `/api/ddns/netinterfaces` | `ddns:read` | ローカルのネットワークインターフェースとアドレスを一覧表示 |
| POST | `/api/ddns/iptest` | `ddns:write` | パブリック IP の取得をテスト |
| GET | `/api/ddns/defaults` | `ddns:read` | DDNS の既定値を読み取り |
| GET | `/api/ddns/settings` | `ddns:read` | DDNS のグローバル設定を読み取り |
| PUT | `/api/ddns/settings` | `ddns:write` | DDNS のグローバル設定を保存 |

## STUN 穿透

| メソッド | パス | 必要な権限 | 説明 |
| --- | --- | --- | --- |
| GET | `/api/stun/logs` | `stun:read` | モジュールの実行ログを表示 |
| GET | `/api/stun/logs/sources` | `stun:read` | モジュールログのすべてのソースを一覧表示 |
| GET | `/api/stun/logs/export` | `stun:read` | モジュールログをエクスポート |
| DELETE | `/api/stun/logs` | `stun:write` | モジュールの実行ログを消去 |
| GET | `/api/stun/rules` | `stun:read` | STUN ルール の一覧を取得 |
| POST | `/api/stun/rules` | `stun:write` | STUN ルール を追加 |
| POST | `/api/stun/rules/{key}/copy` | `stun:write` | STUN ルール を複製 |
| PUT | `/api/stun/rules/{key}` | `stun:write` | STUN ルール を更新 |
| DELETE | `/api/stun/rules/{key}` | `stun:write` | STUN ルール を削除 |
| PUT | `/api/stun/rules/{key}/enable` | `stun:write` | STUN ルール の有効/無効を切り替え |
| PUT | `/api/stun/rules-order` | `stun:write` | STUN ルール の並び順を変更 |
| PUT | `/api/stun/rules-enable` | `stun:write` | STUN ルール の一括有効化・無効化 |
| POST | `/api/stun/rules-delete` | `stun:write` | STUN ルール の一括削除 |
| PUT | `/api/stun/rules-group` | `stun:write` | STUN ルール のグループを一括設定 |
| POST | `/api/stun/rules/{key}/restart` | `stun:write` | STUN ルールを 1 つ再起動 |
| GET | `/api/stun/netinterfaces` | `stun:read` | ローカルのネットワークインターフェースとアドレスを一覧表示 |
| GET | `/api/stun/settings` | `stun:read` | STUN のグローバル設定を読み取り |
| PUT | `/api/stun/settings` | `stun:write` | STUN のグローバル設定を保存 |

## Tailscale ネットワーク

| メソッド | パス | 必要な権限 | 説明 |
| --- | --- | --- | --- |
| GET | `/api/tailscale/logs` | `tailscale:read` | モジュールの実行ログを表示 |
| GET | `/api/tailscale/logs/sources` | `tailscale:read` | モジュールログのすべてのソースを一覧表示 |
| GET | `/api/tailscale/logs/export` | `tailscale:read` | モジュールログをエクスポート |
| DELETE | `/api/tailscale/logs` | `tailscale:write` | モジュールの実行ログを消去 |
| GET | `/api/tailscale/instances` | `tailscale:read` | Tailscale インスタンス の一覧を取得 |
| POST | `/api/tailscale/instances` | `tailscale:write` | Tailscale インスタンスを追加 |
| POST | `/api/tailscale/instances/{key}/copy` | `tailscale:write` | Tailscale インスタンス を複製 |
| PUT | `/api/tailscale/instances/{key}` | `tailscale:write` | Tailscale インスタンス を更新 |
| DELETE | `/api/tailscale/instances/{key}` | `tailscale:write` | Tailscale インスタンス を削除 |
| PUT | `/api/tailscale/instances/{key}/enable` | `tailscale:write` | Tailscale インスタンス の有効/無効を切り替え |
| PUT | `/api/tailscale/instances-order` | `tailscale:write` | Tailscale インスタンス の並び順を変更 |
| PUT | `/api/tailscale/instances-enable` | `tailscale:write` | Tailscale インスタンス の一括有効化・無効化 |
| POST | `/api/tailscale/instances-delete` | `tailscale:write` | Tailscale インスタンス の一括削除 |
| PUT | `/api/tailscale/instances-group` | `tailscale:write` | Tailscale インスタンス のグループを一括設定 |
| GET | `/api/tailscale/instances/{key}/secrets` | `tailscale:write` | シークレットを平文で表示 |
| GET | `/api/tailscale/status` | `tailscale:read` | すべての Tailscale インスタンスの接続状態を表示 |
| GET | `/api/tailscale/instances/{key}/status` | `tailscale:read` | Tailscale インスタンスの接続状態を表示 |
| POST | `/api/tailscale/instances/{key}/restart` | `tailscale:write` | Tailscale インスタンスを再起動 |
| POST | `/api/tailscale/instances/{key}/logout` | `tailscale:write` | Tailscale インスタンスをログアウト |
| GET | `/api/tailscale/settings` | `tailscale:read` | Tailscale のプログラム設定を取得 |
| PUT | `/api/tailscale/settings` | `tailscale:write` | Tailscale のプログラム設定を保存 |
| GET | `/api/tailscale/install` | `tailscale:read` | 管理プログラムのインストール情報と進捗を表示 |
| POST | `/api/tailscale/install` | `tailscale:write` | 管理プログラムをオンラインでインストールまたは更新 |
| POST | `/api/tailscale/install/check` | `tailscale:write` | 管理プログラムの更新を確認 |
| POST | `/api/tailscale/install/rollback` | `tailscale:write` | 保存された旧バージョンに切り替え |
| DELETE | `/api/tailscale/install` | `tailscale:write` | 管理プログラムをアンインストール |
| DELETE | `/api/tailscale/install/backups/{id}` | `tailscale:write` | 保存された旧バージョンを削除 |
| POST | `/api/tailscale/reset` | `tailscale:write` | モジュールのデフォルト設定に戻す |

## NetBird ネットワーク

| メソッド | パス | 必要な権限 | 説明 |
| --- | --- | --- | --- |
| GET | `/api/netbird/logs` | `netbird:read` | モジュールの実行ログを表示 |
| GET | `/api/netbird/logs/sources` | `netbird:read` | モジュールログのすべてのソースを一覧表示 |
| GET | `/api/netbird/logs/export` | `netbird:read` | モジュールログをエクスポート |
| DELETE | `/api/netbird/logs` | `netbird:write` | モジュールの実行ログを消去 |
| GET | `/api/netbird/instances` | `netbird:read` | NetBird インスタンス の一覧を取得 |
| POST | `/api/netbird/instances` | `netbird:write` | NetBird インスタンスを追加 |
| POST | `/api/netbird/instances/{key}/copy` | `netbird:write` | NetBird インスタンス を複製 |
| PUT | `/api/netbird/instances/{key}` | `netbird:write` | NetBird インスタンス を更新 |
| DELETE | `/api/netbird/instances/{key}` | `netbird:write` | NetBird インスタンス を削除 |
| PUT | `/api/netbird/instances/{key}/enable` | `netbird:write` | NetBird インスタンス の有効/無効を切り替え |
| PUT | `/api/netbird/instances-order` | `netbird:write` | NetBird インスタンス の並び順を変更 |
| PUT | `/api/netbird/instances-enable` | `netbird:write` | NetBird インスタンス の一括有効化・無効化 |
| POST | `/api/netbird/instances-delete` | `netbird:write` | NetBird インスタンス の一括削除 |
| PUT | `/api/netbird/instances-group` | `netbird:write` | NetBird インスタンス のグループを一括設定 |
| GET | `/api/netbird/instances/{key}/secrets` | `netbird:write` | シークレットを平文で表示 |
| GET | `/api/netbird/status` | `netbird:read` | すべての NetBird インスタンスの接続状態を表示 |
| GET | `/api/netbird/instances/{key}/status` | `netbird:read` | NetBird インスタンスの接続状態を表示 |
| POST | `/api/netbird/instances/{key}/restart` | `netbird:write` | NetBird インスタンスを再起動 |
| POST | `/api/netbird/instances/{key}/logout` | `netbird:write` | NetBird インスタンスからログアウト |
| GET | `/api/netbird/settings` | `netbird:read` | NetBird のプログラム設定を読み取る |
| PUT | `/api/netbird/settings` | `netbird:write` | NetBird のプログラム設定を保存 |
| GET | `/api/netbird/install` | `netbird:read` | 管理プログラムのインストール情報と進捗を表示 |
| POST | `/api/netbird/install` | `netbird:write` | 管理プログラムをオンラインでインストールまたは更新 |
| POST | `/api/netbird/install/check` | `netbird:write` | 管理プログラムの更新を確認 |
| POST | `/api/netbird/install/rollback` | `netbird:write` | 保存された旧バージョンに切り替え |
| DELETE | `/api/netbird/install` | `netbird:write` | 管理プログラムをアンインストール |
| DELETE | `/api/netbird/install/backups/{id}` | `netbird:write` | 保存された旧バージョンを削除 |
| POST | `/api/netbird/reset` | `netbird:write` | モジュールのデフォルト設定に戻す |

## frp イントラネット穿透

| メソッド | パス | 必要な権限 | 説明 |
| --- | --- | --- | --- |
| GET | `/api/frp/logs` | `frp:read` | モジュールの実行ログを表示 |
| GET | `/api/frp/logs/sources` | `frp:read` | モジュールログのすべてのソースを一覧表示 |
| GET | `/api/frp/logs/export` | `frp:read` | モジュールログをエクスポート |
| DELETE | `/api/frp/logs` | `frp:write` | モジュールの実行ログを消去 |
| GET | `/api/frp/clients` | `frp:read` | frp クライアント の一覧を取得 |
| POST | `/api/frp/clients` | `frp:write` | frp クライアント を追加 |
| POST | `/api/frp/clients/{key}/copy` | `frp:write` | frp クライアント を複製 |
| PUT | `/api/frp/clients/{key}` | `frp:write` | frp クライアント を更新 |
| DELETE | `/api/frp/clients/{key}` | `frp:write` | frp クライアント を削除 |
| PUT | `/api/frp/clients/{key}/enable` | `frp:write` | frp クライアント の有効/無効を切り替え |
| PUT | `/api/frp/clients-order` | `frp:write` | frp クライアント の並び順を変更 |
| PUT | `/api/frp/clients-enable` | `frp:write` | frp クライアント の一括有効化・無効化 |
| POST | `/api/frp/clients-delete` | `frp:write` | frp クライアント の一括削除 |
| PUT | `/api/frp/clients-group` | `frp:write` | frp クライアント のグループを一括設定 |
| GET | `/api/frp/clients/{key}/secrets` | `frp:write` | シークレットを平文で表示 |
| GET | `/api/frp/servers` | `frp:read` | frp サーバー の一覧を取得 |
| POST | `/api/frp/servers` | `frp:write` | frp サーバー を追加 |
| POST | `/api/frp/servers/{key}/copy` | `frp:write` | frp サーバー を複製 |
| PUT | `/api/frp/servers/{key}` | `frp:write` | frp サーバー を更新 |
| DELETE | `/api/frp/servers/{key}` | `frp:write` | frp サーバー を削除 |
| PUT | `/api/frp/servers/{key}/enable` | `frp:write` | frp サーバー の有効/無効を切り替え |
| PUT | `/api/frp/servers-order` | `frp:write` | frp サーバー の並び順を変更 |
| PUT | `/api/frp/servers-enable` | `frp:write` | frp サーバー の一括有効化・無効化 |
| POST | `/api/frp/servers-delete` | `frp:write` | frp サーバー の一括削除 |
| PUT | `/api/frp/servers-group` | `frp:write` | frp サーバー のグループを一括設定 |
| GET | `/api/frp/servers/{key}/secrets` | `frp:write` | シークレットを平文で表示 |
| GET | `/api/frp/proxies` | `frp:read` | frp プロキシ の一覧を取得 |
| POST | `/api/frp/proxies` | `frp:write` | frp プロキシ を追加 |
| POST | `/api/frp/proxies/{key}/copy` | `frp:write` | frp プロキシ を複製 |
| PUT | `/api/frp/proxies/{key}` | `frp:write` | frp プロキシ を更新 |
| DELETE | `/api/frp/proxies/{key}` | `frp:write` | frp プロキシ を削除 |
| PUT | `/api/frp/proxies/{key}/enable` | `frp:write` | frp プロキシ の有効/無効を切り替え |
| PUT | `/api/frp/proxies-order` | `frp:write` | frp プロキシ の並び順を変更 |
| PUT | `/api/frp/proxies-enable` | `frp:write` | frp プロキシ の一括有効化・無効化 |
| POST | `/api/frp/proxies-delete` | `frp:write` | frp プロキシ の一括削除 |
| PUT | `/api/frp/proxies-group` | `frp:write` | frp プロキシ のグループを一括設定 |
| GET | `/api/frp/proxies/{key}/secrets` | `frp:write` | シークレットを平文で表示 |
| POST | `/api/frp/import` | `frp:write` | 既存の frpc / frps 設定をインポート（旧形式 ini と新形式 toml に対応）。apply が false の場合はプレビューのみ |
| GET | `/api/frp/install` | `frp:read` | 管理プログラムのインストール情報と進捗を表示 |
| POST | `/api/frp/install` | `frp:write` | 管理プログラムをオンラインでインストールまたは更新 |
| POST | `/api/frp/install/check` | `frp:write` | 管理プログラムの更新を確認 |
| POST | `/api/frp/install/rollback` | `frp:write` | 保存された旧バージョンに切り替え |
| DELETE | `/api/frp/install` | `frp:write` | 管理プログラムをアンインストール |
| DELETE | `/api/frp/install/backups/{id}` | `frp:write` | 保存された旧バージョンを削除 |
| POST | `/api/frp/clients/{key}/restart` | `frp:write` | frpc インスタンスを再起動 |
| POST | `/api/frp/servers/{key}/restart` | `frp:write` | frps インスタンスを再起動 |
| GET | `/api/frp/settings` | `frp:read` | プログラムのパスとバージョンを取得 |
| PUT | `/api/frp/settings` | `frp:write` | プログラムのパスを保存 |
| POST | `/api/frp/reset` | `frp:write` | モジュールのデフォルト設定に戻す |

## Cloudflare トンネル

| メソッド | パス | 必要な権限 | 説明 |
| --- | --- | --- | --- |
| GET | `/api/cloudflared/logs` | `cloudflared:read` | モジュールの実行ログを表示 |
| GET | `/api/cloudflared/logs/sources` | `cloudflared:read` | モジュールログのすべてのソースを一覧表示 |
| GET | `/api/cloudflared/logs/export` | `cloudflared:read` | モジュールログをエクスポート |
| DELETE | `/api/cloudflared/logs` | `cloudflared:write` | モジュールの実行ログを消去 |
| GET | `/api/cloudflared/tunnels` | `cloudflared:read` | Cloudflare トンネル の一覧を取得 |
| POST | `/api/cloudflared/tunnels` | `cloudflared:write` | Cloudflare トンネル を追加 |
| POST | `/api/cloudflared/tunnels/{key}/copy` | `cloudflared:write` | Cloudflare トンネル を複製 |
| PUT | `/api/cloudflared/tunnels/{key}` | `cloudflared:write` | Cloudflare トンネル を更新 |
| DELETE | `/api/cloudflared/tunnels/{key}` | `cloudflared:write` | Cloudflare トンネル を削除 |
| PUT | `/api/cloudflared/tunnels/{key}/enable` | `cloudflared:write` | Cloudflare トンネル の有効/無効を切り替え |
| PUT | `/api/cloudflared/tunnels-order` | `cloudflared:write` | Cloudflare トンネル の並び順を変更 |
| PUT | `/api/cloudflared/tunnels-enable` | `cloudflared:write` | Cloudflare トンネル の一括有効化・無効化 |
| POST | `/api/cloudflared/tunnels-delete` | `cloudflared:write` | Cloudflare トンネル の一括削除 |
| PUT | `/api/cloudflared/tunnels-group` | `cloudflared:write` | Cloudflare トンネル のグループを一括設定 |
| GET | `/api/cloudflared/tunnels/{key}/secrets` | `cloudflared:write` | シークレットを平文で表示 |
| POST | `/api/cloudflared/tunnels/{key}/restart` | `cloudflared:write` | Cloudflare トンネルを 1 つ再起動 |
| GET | `/api/cloudflared/tunnels/{key}/ingress` | `cloudflared:read` | トークントンネルのルートを取得 |
| PUT | `/api/cloudflared/tunnels/{key}/ingress` | `cloudflared:write` | トークントンネルのルートを保存 |
| GET | `/api/cloudflared/settings` | `cloudflared:read` | Cloudflare トンネル設定を読み取り |
| PUT | `/api/cloudflared/settings` | `cloudflared:write` | Cloudflare トンネルの設定を保存 |
| GET | `/api/cloudflared/install` | `cloudflared:read` | 管理プログラムのインストール情報と進捗を表示 |
| POST | `/api/cloudflared/install` | `cloudflared:write` | 管理プログラムをオンラインでインストールまたは更新 |
| POST | `/api/cloudflared/install/check` | `cloudflared:write` | 管理プログラムの更新を確認 |
| POST | `/api/cloudflared/install/rollback` | `cloudflared:write` | 保存された旧バージョンに切り替え |
| DELETE | `/api/cloudflared/install` | `cloudflared:write` | 管理プログラムをアンインストール |
| DELETE | `/api/cloudflared/install/backups/{id}` | `cloudflared:write` | 保存された旧バージョンを削除 |
| POST | `/api/cloudflared/reset` | `cloudflared:write` | モジュールのデフォルト設定に戻す |

## SSL 証明書

| メソッド | パス | 必要な権限 | 説明 |
| --- | --- | --- | --- |
| GET | `/api/ssl/logs` | `ssl:read` | モジュールの実行ログを表示 |
| GET | `/api/ssl/logs/sources` | `ssl:read` | モジュールログのすべてのソースを一覧表示 |
| GET | `/api/ssl/logs/export` | `ssl:read` | モジュールログをエクスポート |
| DELETE | `/api/ssl/logs` | `ssl:write` | モジュールの実行ログを消去 |
| GET | `/api/ssl/certs` | `ssl:read` | 証明書 の一覧を取得 |
| POST | `/api/ssl/certs` | `ssl:write` | 証明書 を追加 |
| POST | `/api/ssl/certs/{key}/copy` | `ssl:write` | 証明書 を複製 |
| PUT | `/api/ssl/certs/{key}` | `ssl:write` | 証明書 を更新 |
| DELETE | `/api/ssl/certs/{key}` | `ssl:write` | 証明書 を削除 |
| PUT | `/api/ssl/certs/{key}/enable` | `ssl:write` | 証明書 の有効/無効を切り替え |
| PUT | `/api/ssl/certs-order` | `ssl:write` | 証明書 の並び順を変更 |
| PUT | `/api/ssl/certs-enable` | `ssl:write` | 証明書 の一括有効化・無効化 |
| POST | `/api/ssl/certs-delete` | `ssl:write` | 証明書 の一括削除 |
| PUT | `/api/ssl/certs-group` | `ssl:write` | 証明書 のグループを一括設定 |
| GET | `/api/ssl/certs/{key}/secrets` | `ssl:write` | シークレットを平文で表示 |
| POST | `/api/ssl/certs/{key}/renew` | `ssl:write` | 証明書を即時発行または更新 |
| GET | `/api/ssl/certs/{key}/download` | `ssl:write` | 証明書と秘密鍵をダウンロード |
| POST | `/api/ssl/certs/{key}/deploy` | `ssl:write` | 証明書を今すぐデプロイ |
| GET | `/api/ssl/deploy-targets` | `ssl:read` | 選択できるデプロイ先を一覧表示 |
| GET | `/api/ssl/providers` | `ssl:read` | 対応する DNS 検証プロバイダーを一覧表示 |
| GET | `/api/ssl/settings` | `ssl:read` | 証明書のグローバル設定を読み取り |
| PUT | `/api/ssl/settings` | `ssl:write` | 証明書のグローバル設定を保存 |

## WAF ファイアウォール

| メソッド | パス | 必要な権限 | 説明 |
| --- | --- | --- | --- |
| GET | `/api/waf/logs` | `waf:read` | モジュールの実行ログを表示 |
| GET | `/api/waf/logs/sources` | `waf:read` | モジュールログのすべてのソースを一覧表示 |
| GET | `/api/waf/logs/export` | `waf:read` | モジュールログをエクスポート |
| DELETE | `/api/waf/logs` | `waf:write` | モジュールの実行ログを消去 |
| GET | `/api/waf/instances` | `waf:read` | WAF インスタンス の一覧を取得 |
| POST | `/api/waf/instances` | `waf:write` | WAF インスタンス を追加 |
| POST | `/api/waf/instances/{key}/copy` | `waf:write` | WAF インスタンス を複製 |
| PUT | `/api/waf/instances/{key}` | `waf:write` | WAF インスタンス を更新 |
| DELETE | `/api/waf/instances/{key}` | `waf:write` | WAF インスタンス を削除 |
| PUT | `/api/waf/instances/{key}/enable` | `waf:write` | WAF インスタンス の有効/無効を切り替え |
| PUT | `/api/waf/instances-order` | `waf:write` | WAF インスタンス の並び順を変更 |
| PUT | `/api/waf/instances-enable` | `waf:write` | WAF インスタンス の一括有効化・無効化 |
| POST | `/api/waf/instances-delete` | `waf:write` | WAF インスタンス の一括削除 |
| PUT | `/api/waf/instances-group` | `waf:write` | WAF インスタンス のグループを一括設定 |
| POST | `/api/waf/safeline/test` | `waf:write` | SafeLine 検知エンジンをテスト |
| GET | `/api/waf/crs` | `waf:read` | OWASP CRS ルールファイルを一覧表示 |
| GET | `/api/waf/crs/{file}` | `waf:read` | CRS ルールファイルを 1 つ読み取り |

## Web サービス

| メソッド | パス | 必要な権限 | 説明 |
| --- | --- | --- | --- |
| GET | `/api/webservice/logs` | `webservice:read` | モジュールの実行ログを表示 |
| GET | `/api/webservice/logs/sources` | `webservice:read` | モジュールログのすべてのソースを一覧表示 |
| GET | `/api/webservice/logs/export` | `webservice:read` | モジュールログをエクスポート |
| DELETE | `/api/webservice/logs` | `webservice:write` | モジュールの実行ログを消去 |
| GET | `/api/webservice/listeners` | `webservice:read` | Web サービスのリッスン の一覧を取得 |
| POST | `/api/webservice/listeners` | `webservice:write` | Web サービスのリッスン を追加 |
| POST | `/api/webservice/listeners/{key}/copy` | `webservice:write` | Web サービスのリッスン を複製 |
| PUT | `/api/webservice/listeners/{key}` | `webservice:write` | Web サービスのリッスン を更新 |
| DELETE | `/api/webservice/listeners/{key}` | `webservice:write` | Web サービスのリッスン を削除 |
| PUT | `/api/webservice/listeners/{key}/enable` | `webservice:write` | Web サービスのリッスン の有効/無効を切り替え |
| PUT | `/api/webservice/listeners-order` | `webservice:write` | Web サービスのリッスン の並び順を変更 |
| PUT | `/api/webservice/listeners-enable` | `webservice:write` | Web サービスのリッスン の一括有効化・無効化 |
| POST | `/api/webservice/listeners-delete` | `webservice:write` | Web サービスのリッスン の一括削除 |
| GET | `/api/webservice/iplists` | `webservice:read` | 購読 IP リストの一覧を取得 |
| POST | `/api/webservice/iplists` | `webservice:write` | 購読 IP リスト を追加 |
| POST | `/api/webservice/iplists/{key}/copy` | `webservice:write` | 購読 IP リスト を複製 |
| PUT | `/api/webservice/iplists/{key}` | `webservice:write` | 購読 IP リスト を更新 |
| DELETE | `/api/webservice/iplists/{key}` | `webservice:write` | 購読 IP リスト を削除 |
| PUT | `/api/webservice/iplists/{key}/enable` | `webservice:write` | 購読 IP リスト の有効/無効を切り替え |
| PUT | `/api/webservice/iplists-order` | `webservice:write` | 購読 IP リスト の並び順を変更 |
| PUT | `/api/webservice/iplists-enable` | `webservice:write` | 購読 IP リスト の一括有効化・無効化 |
| POST | `/api/webservice/iplists-delete` | `webservice:write` | 購読 IP リスト の一括削除 |
| POST | `/api/webservice/iplists/{key}/refresh` | `webservice:write` | 購読 IP リストを今すぐ更新 |
| GET | `/api/webservice/errorpages` | `webservice:read` | カスタムエラーページの一覧（ページ内容は含まない） |
| POST | `/api/webservice/errorpages` | `webservice:write` | カスタムエラーページを追加：アップロードされたページはサニタイズして保存し、ファイルページはファイルが利用可能か確認します |
| GET | `/api/webservice/errorpages/{key}` | `webservice:read` | カスタムエラーページを取得（ページ内容を含む） |
| PUT | `/api/webservice/errorpages/{key}` | `webservice:write` | カスタムエラーページを変更 |
| DELETE | `/api/webservice/errorpages/{key}` | `webservice:write` | カスタムエラーページを削除（サイトから参照されている場合は拒否） |
| POST | `/api/webservice/errorpages/check` | `webservice:write` | ページの安全性と可用性を確認してプレビューを生成します（保存はしません） |
| GET | `/api/webservice/errorpages/styles` | `webservice:read` | デフォルトスタイルとすべての組み込みスタイルのプレビュー。?code= でステータスコードを指定（デフォルト 502） |
| GET | `/api/webservice/errorpages/{key}/preview` | `webservice:read` | カスタムエラーページのプレビュー。?code= でステータスコードを指定（デフォルト 502） |
| GET | `/api/webservice/panels` | `webservice:read` | ナビゲーションパネル の一覧を取得 |
| POST | `/api/webservice/panels` | `webservice:write` | ナビゲーションパネル を追加 |
| POST | `/api/webservice/panels/{key}/copy` | `webservice:write` | ナビゲーションパネル を複製 |
| PUT | `/api/webservice/panels/{key}` | `webservice:write` | ナビゲーションパネル を更新 |
| DELETE | `/api/webservice/panels/{key}` | `webservice:write` | ナビゲーションパネル を削除 |
| PUT | `/api/webservice/panels/{key}/enable` | `webservice:write` | ナビゲーションパネル の有効/無効を切り替え |
| PUT | `/api/webservice/panels-order` | `webservice:write` | ナビゲーションパネル の並び順を変更 |
| PUT | `/api/webservice/panels-enable` | `webservice:write` | ナビゲーションパネル の一括有効化・無効化 |
| POST | `/api/webservice/panels-delete` | `webservice:write` | ナビゲーションパネル の一括削除 |
| POST | `/api/webservice/panels/preview` | `webservice:write` | ナビゲーションパネルのプレビュー |
| GET | `/api/webservice/panels/icons` | `webservice:read` | ナビゲーションパネルの組み込みアイコン一覧 |
| GET | `/api/webservice/users` | `webservice:read` | 共有ユーザーと OIDC ID プロバイダーを取得 |
| PUT | `/api/webservice/users` | `webservice:write` | 共有ユーザーと OIDC ID プロバイダーを保存し、サイトを一括設定 |
| POST | `/api/webservice/sites/batch` | `webservice:write` | サイトの一括操作：有効化・無効化、削除、ドメインの追加・削除・置換、一部オプションの変更、別のリスナーへの移動。すべてのサイトが検証を通過した場合のみ保存し、1 件でも失敗すると何も変更しません |
| GET | `/api/webservice/cache` | `webservice:read` | サイトのレスポンスキャッシュのエントリ数と使用量（?listener=&site=） |
| POST | `/api/webservice/cache/purge` | `webservice:write` | サイトのレスポンスキャッシュを消去します。path が空なら全件、指定時はパスが一致するエントリのみ（キャッシュルールのパスと同じ書式、* 対応） |
| POST | `/api/webservice/import/nginx` | `webservice:write` | nginx 設定をインポート |
| GET | `/api/webservice/settings` | `webservice:read` | Web サービスのグローバル設定を読み取り |
| GET | `/api/webservice/settings/secrets/totp` | `webservice:write` | 共有ユーザーの TOTP シークレットを平文で表示（ユーザー名で索引） |
| GET | `/api/webservice/settings/secrets/oauth` | `webservice:write` | サードパーティログインの Client Secret を平文で表示（ログイン方式 ID で索引） |
| POST | `/api/webservice/totp-secret` | `webservice:write` | 共有ユーザーの新しい TOTP 鍵を生成（保存はせず、設定保存後に有効） |
| PUT | `/api/webservice/settings` | `webservice:write` | Web サービスのグローバル設定を保存 |
| GET | `/api/webservice/stats` | `webservice:read` | サイトの分単位統計（?listener=&site=&minutes=） |
| GET | `/api/webservice/metrics` | `webservice:read` | Prometheus テキスト形式のメトリクス |
| GET | `/api/webservice/topology` | `webservice:read` | Web サービスのトポロジースナップショット |
| GET | `/api/webservice/visitors` | `webservice:read` | 訪問者監査の一覧 |
| GET | `/api/webservice/visitors/detail` | `webservice:read` | 訪問者 1 件の監査記録 |
| POST | `/api/webservice/visitors/delete` | `webservice:write` | 訪問者監査の記録を削除 |
| GET | `/api/webservice/visitors/settings` | `webservice:read` | 訪問者監査の設定とストレージ状態を取得 |
| PUT | `/api/webservice/visitors/settings` | `webservice:write` | 訪問者監査の設定を保存 |
| GET | `/api/webservice/topology/replay` | `webservice:read` | トポロジー再生の概要 |
| GET | `/api/webservice/topology/replay/frame` | `webservice:read` | ある時点のトポロジー再生スナップショット |
| PUT | `/api/webservice/topology/replay/settings` | `webservice:write` | トポロジー再生の設定を変更 |
| GET | `/api/webservice/connections` | `webservice:read` | サイトの進行中リクエストと最近終了した長時間リクエスト（?listener=&site=&q=） |
| POST | `/api/webservice/connections/close` | `webservice:write` | サイトで進行中のリクエストを切断し、送信元アドレスを同時にブロックできます（banSeconds が -1 で無期限） |
| GET | `/api/webservice/bans` | `webservice:read` | 現在ブロックされている IP |
| POST | `/api/webservice/bans` | `webservice:write` | IP を手動でブロック（seconds が 0 で 24 時間、-1 で無期限） |
| DELETE | `/api/webservice/bans` | `webservice:write` | ブロックを解除（body は {"key": アドレス}） |
| GET | `/api/webservice/geoip` | `webservice:read` | GeoIP データベースの状態 |
| POST | `/api/webservice/geoip/upload` | `webservice:write` | GeoIP データベースをアップロード（multipart フィールド file、.mmdb） |
| POST | `/api/webservice/geoip/download` | `webservice:write` | URL から GeoIP データベース（.mmdb）をダウンロード |
| DELETE | `/api/webservice/geoip` | `webservice:write` | GeoIP データベースを削除 |

## 侵入防御

| メソッド | パス | 必要な権限 | 説明 |
| --- | --- | --- | --- |
| GET | `/api/guard/logs` | `guard:read` | モジュールの実行ログを表示 |
| GET | `/api/guard/logs/sources` | `guard:read` | モジュールログのすべてのソースを一覧表示 |
| GET | `/api/guard/logs/export` | `guard:read` | モジュールログをエクスポート |
| DELETE | `/api/guard/logs` | `guard:write` | モジュールの実行ログを消去 |
| GET | `/api/guard/jails` | `guard:read` | 侵入防御ルール の一覧を取得 |
| POST | `/api/guard/jails` | `guard:write` | 侵入防御ルール を追加 |
| POST | `/api/guard/jails/{key}/copy` | `guard:write` | 侵入防御ルール を複製 |
| PUT | `/api/guard/jails/{key}` | `guard:write` | 侵入防御ルール を更新 |
| DELETE | `/api/guard/jails/{key}` | `guard:write` | 侵入防御ルール を削除 |
| PUT | `/api/guard/jails/{key}/enable` | `guard:write` | 侵入防御ルール の有効/無効を切り替え |
| PUT | `/api/guard/jails-order` | `guard:write` | 侵入防御ルール の並び順を変更 |
| PUT | `/api/guard/jails-enable` | `guard:write` | 侵入防御ルール の一括有効化・無効化 |
| POST | `/api/guard/jails-delete` | `guard:write` | 侵入防御ルール の一括削除 |
| PUT | `/api/guard/jails-group` | `guard:write` | 侵入防御ルール のグループを一括設定 |
| GET | `/api/guard/presets` | `guard:read` | よく使われるサービスのルールテンプレート |
| POST | `/api/guard/jails/test` | `guard:write` | ログのサンプルでフィルターをテスト |
| GET | `/api/guard/status` | `guard:read` | ファイアウォール同期の状態と実行環境 |
| GET | `/api/guard/settings` | `guard:read` | グローバル設定 |
| PUT | `/api/guard/settings` | `guard:write` | 全体設定を変更 |
| GET | `/api/guard/bans` | `guard:read` | 現在ブロック中の IP |
| POST | `/api/guard/bans` | `guard:write` | IP またはネットワークを手動でブロック（seconds が 0 で既定の期間、-1 で無期限） |
| DELETE | `/api/guard/bans` | `guard:write` | ブロックを解除（body は {"key": アドレス}） |
| GET | `/api/guard/firewall/rules` | `guard:read` | Gately がシステムファイアウォールに書き込んだルール（テキスト） |

## Wake-on-LAN

| メソッド | パス | 必要な権限 | 説明 |
| --- | --- | --- | --- |
| GET | `/api/wol/logs` | `wol:read` | モジュールの実行ログを表示 |
| GET | `/api/wol/logs/sources` | `wol:read` | モジュールログのすべてのソースを一覧表示 |
| GET | `/api/wol/logs/export` | `wol:read` | モジュールログをエクスポート |
| DELETE | `/api/wol/logs` | `wol:write` | モジュールの実行ログを消去 |
| GET | `/api/wol/devices` | `wol:read` | デバイスをウェイクアップ の一覧を取得 |
| POST | `/api/wol/devices` | `wol:write,webterminal:write` | デバイスをウェイクアップ を追加 |
| POST | `/api/wol/devices/{key}/copy` | `wol:write,webterminal:write` | デバイスをウェイクアップ を複製 |
| PUT | `/api/wol/devices/{key}` | `wol:write,webterminal:write` | デバイスをウェイクアップ を更新 |
| DELETE | `/api/wol/devices/{key}` | `wol:write` | デバイスをウェイクアップ を削除 |
| PUT | `/api/wol/devices/{key}/enable` | `wol:write` | デバイスをウェイクアップ の有効/無効を切り替え |
| PUT | `/api/wol/devices-order` | `wol:write` | デバイスをウェイクアップ の並び順を変更 |
| PUT | `/api/wol/devices-enable` | `wol:write` | デバイスをウェイクアップ の一括有効化・無効化 |
| POST | `/api/wol/devices-delete` | `wol:write` | デバイスをウェイクアップ の一括削除 |
| PUT | `/api/wol/devices-group` | `wol:write` | デバイスをウェイクアップ のグループを一括設定 |
| POST | `/api/wol/devices/{key}/wake` | `wol:write` | デバイスにウェイクアップのマジックパケットを送信 |
| POST | `/api/wol/devices/{key}/shutdown` | `wol:write,webterminal:write` | リモートシャットダウン |
| GET | `/api/wol/status` | `wol:read` | デバイスのオンライン状態を表示 |
| GET | `/api/wol/ssh-targets` | `wol:read,webterminal:read` | リモートシャットダウンに使える SSH 接続の一覧 |
| GET | `/api/wol/bemfa` | `wol:read` | Bemfa 連携設定と接続状態 |
| GET | `/api/wol/bemfa/secrets` | `wol:write` | Bemfa 秘密鍵を表示 |
| PUT | `/api/wol/bemfa` | `wol:write` | Bemfa 連携設定を変更 |

## Web ターミナル

| メソッド | パス | 必要な権限 | 説明 |
| --- | --- | --- | --- |
| GET | `/api/webterminal/logs` | `webterminal:read` | モジュールの実行ログを表示 |
| GET | `/api/webterminal/logs/sources` | `webterminal:read` | モジュールログのすべてのソースを一覧表示 |
| GET | `/api/webterminal/logs/export` | `webterminal:read` | モジュールログをエクスポート |
| DELETE | `/api/webterminal/logs` | `webterminal:write` | モジュールの実行ログを消去 |
| GET | `/api/webterminal/connections` | `webterminal:read` | ターミナル接続 の一覧を取得 |
| POST | `/api/webterminal/connections` | `webterminal:write` | ターミナル接続 を追加 |
| POST | `/api/webterminal/connections/{key}/copy` | `webterminal:write` | ターミナル接続 を複製 |
| PUT | `/api/webterminal/connections/{key}` | `webterminal:write` | ターミナル接続 を更新 |
| DELETE | `/api/webterminal/connections/{key}` | `webterminal:write` | ターミナル接続 を削除 |
| PUT | `/api/webterminal/connections/{key}/enable` | `webterminal:write` | ターミナル接続 の有効/無効を切り替え |
| PUT | `/api/webterminal/connections-order` | `webterminal:write` | ターミナル接続 の並び順を変更 |
| PUT | `/api/webterminal/connections-enable` | `webterminal:write` | ターミナル接続 の一括有効化・無効化 |
| POST | `/api/webterminal/connections-delete` | `webterminal:write` | ターミナル接続 の一括削除 |
| PUT | `/api/webterminal/connections-group` | `webterminal:write` | ターミナル接続 のグループを一括設定 |
| GET | `/api/webterminal/connections/{key}/secrets` | `webterminal:write` | シークレットを平文で表示 |
| DELETE | `/api/webterminal/connections/{key}/hostkey` | `webterminal:write` | 記録済みのホストキーをクリア |
| POST | `/api/webterminal/connections/{key}/test` | `webterminal:write` | 保存済みの接続をテスト |
| POST | `/api/webterminal/connections/test` | `webterminal:write` | 一時的な接続パラメーターをテスト |
| PUT | `/api/webterminal/connections/{key}/quickaccess` | `webterminal:write` | 接続のクイックアクセスを設定 |
| GET | `/api/webterminal/credentials` | `webterminal:read` | ターミナル認証プリセット の一覧を取得 |
| POST | `/api/webterminal/credentials` | `webterminal:write` | ターミナル認証プリセット を追加 |
| POST | `/api/webterminal/credentials/{key}/copy` | `webterminal:write` | ターミナル認証プリセット を複製 |
| PUT | `/api/webterminal/credentials/{key}` | `webterminal:write` | ターミナル認証プリセット を更新 |
| DELETE | `/api/webterminal/credentials/{key}` | `webterminal:write` | ターミナル認証プリセット を削除 |
| PUT | `/api/webterminal/credentials/{key}/enable` | `webterminal:write` | ターミナル認証プリセット の有効/無効を切り替え |
| PUT | `/api/webterminal/credentials-order` | `webterminal:write` | ターミナル認証プリセット の並び順を変更 |
| PUT | `/api/webterminal/credentials-enable` | `webterminal:write` | ターミナル認証プリセット の一括有効化・無効化 |
| POST | `/api/webterminal/credentials-delete` | `webterminal:write` | ターミナル認証プリセット の一括削除 |
| GET | `/api/webterminal/credentials/{key}/secrets` | `webterminal:write` | シークレットを平文で表示 |
| GET | `/api/webterminal/connections/{key}/ws` | `webterminal:write` | ターミナルセッションを開く（WebSocket） |
| GET | `/api/webterminal/sessions/{id}/ws` | `webterminal:write` | 既存のターミナルセッションに接続（WebSocket） |
| GET | `/api/webterminal/sessions` | `webterminal:read` | ターミナルセッションを一覧表示 |
| GET | `/api/webterminal/sessions/{id}` | `webterminal:read` | ターミナルセッションの詳細を表示 |
| PUT | `/api/webterminal/sessions/{id}/remark` | `webterminal:write` | ターミナルセッションのメモを変更 |
| GET | `/api/webterminal/sessions/{id}/stats` | `webterminal:read` | ターミナルセッションの統計を表示 |
| DELETE | `/api/webterminal/sessions/{id}` | `webterminal:write` | ターミナルセッションを閉じる |
| GET | `/api/webterminal/security` | ログインセッションのみ | ターミナルのセキュリティ設定の状態を表示 |
| POST | `/api/webterminal/security/agreement` | ログインセッションのみ | ターミナル利用規約に同意 |
| POST | `/api/webterminal/security/2fa/setup` | ログインセッションのみ | ターミナルの 2 要素認証シークレットを生成 |
| POST | `/api/webterminal/security/2fa/enable` | ログインセッションのみ | ターミナルの 2 要素認証を有効化 |
| POST | `/api/webterminal/security/2fa/disable` | ログインセッションのみ | ターミナルの 2 要素認証を無効化 |
| POST | `/api/webterminal/security/verify2fa` | ログインセッションのみ | ターミナルの 2 要素認証コードを検証 |
| GET | `/api/webterminal/security/check2fa` | ログインセッションのみ | ターミナルの 2 要素認証が完了しているか確認 |
| GET | `/api/webterminal/sftp/{key}/list` | `webterminal:write` | リモートディレクトリを一覧表示 |
| GET | `/api/webterminal/sftp/{key}/read` | `webterminal:write` | リモートファイルを読み取り |
| POST | `/api/webterminal/sftp/{key}/write` | `webterminal:write` | リモートファイルに書き込み |
| POST | `/api/webterminal/sftp/{key}/upload` | `webterminal:write` | リモートホストにファイルをアップロード |
| POST | `/api/webterminal/sftp/{key}/mkdir` | `webterminal:write` | リモートディレクトリを作成 |
| POST | `/api/webterminal/sftp/{key}/touch` | `webterminal:write` | リモートに空ファイルを作成 |
| POST | `/api/webterminal/sftp/{key}/rename` | `webterminal:write` | リモートファイルの名前変更または移動 |
| POST | `/api/webterminal/sftp/{key}/copy` | `webterminal:write` | リモートファイルをコピー |
| POST | `/api/webterminal/sftp/{key}/chmod` | `webterminal:write` | リモートファイルの権限を変更 |
| DELETE | `/api/webterminal/sftp/{key}/remove` | `webterminal:write` | リモートファイルを削除 |
| POST | `/api/webterminal/sftp/{key}/compress` | `webterminal:write` | リモートホスト上でファイルを圧縮 |
| POST | `/api/webterminal/sftp/{key}/decompress` | `webterminal:write` | リモートホスト上でファイルを展開 |
| GET | `/api/webterminal/sftp/{key}/preview-archive` | `webterminal:write` | リモートアーカイブの内容をプレビュー |
| GET | `/api/webterminal/settings` | `webterminal:read` | Web ターミナル設定を読み取り |
| PUT | `/api/webterminal/settings` | `webterminal:write` | Web ターミナル設定を保存 |
| GET | `/api/webterminal/shells` | `webterminal:read` | ローカルで利用可能なシェルを一覧表示 |
| GET | `/api/webterminal/globalshortcuts` | `webterminal:read` | グローバルクイックコマンドを読み取り |
| PUT | `/api/webterminal/globalshortcuts` | `webterminal:write` | グローバルクイックコマンドを保存 |
| GET | `/api/webterminal/splitlayout` | `webterminal:read` | 分割レイアウトを読み取り |
| PUT | `/api/webterminal/splitlayout` | `webterminal:write` | 分割レイアウトを保存 |
| DELETE | `/api/webterminal/splitlayout` | `webterminal:write` | 分割レイアウトをクリア |

## スケジュールタスク

| メソッド | パス | 必要な権限 | 説明 |
| --- | --- | --- | --- |
| GET | `/api/cron/logs` | `cron:read` | モジュールの実行ログを表示 |
| GET | `/api/cron/logs/sources` | `cron:read` | モジュールログのすべてのソースを一覧表示 |
| GET | `/api/cron/logs/export` | `cron:read` | モジュールログをエクスポート |
| DELETE | `/api/cron/logs` | `cron:write` | モジュールの実行ログを消去 |
| GET | `/api/cron/tasks` | `cron:read` | スケジュールタスク の一覧を取得 |
| POST | `/api/cron/tasks` | `cron:write` | スケジュールタスク を追加 |
| POST | `/api/cron/tasks/{key}/copy` | `cron:write` | スケジュールタスク を複製 |
| PUT | `/api/cron/tasks/{key}` | `cron:write` | スケジュールタスク を更新 |
| DELETE | `/api/cron/tasks/{key}` | `cron:write` | スケジュールタスク を削除 |
| PUT | `/api/cron/tasks/{key}/enable` | `cron:write` | スケジュールタスク の有効/無効を切り替え |
| PUT | `/api/cron/tasks-order` | `cron:write` | スケジュールタスク の並び順を変更 |
| PUT | `/api/cron/tasks-enable` | `cron:write` | スケジュールタスク の一括有効化・無効化 |
| POST | `/api/cron/tasks-delete` | `cron:write` | スケジュールタスク の一括削除 |
| PUT | `/api/cron/tasks-group` | `cron:write` | スケジュールタスク のグループを一括設定 |
| POST | `/api/cron/tasks/{key}/run` | `cron:write` | スケジュールタスクを即時実行 |
| POST | `/api/cron/parse` | `cron:write` | Cron 式を解析してプレビュー |

## Docker 管理

| メソッド | パス | 必要な権限 | 説明 |
| --- | --- | --- | --- |
| GET | `/api/docker/logs` | `docker:read` | モジュールの実行ログを表示 |
| GET | `/api/docker/logs/sources` | `docker:read` | モジュールログのすべてのソースを一覧表示 |
| GET | `/api/docker/logs/export` | `docker:read` | モジュールログをエクスポート |
| DELETE | `/api/docker/logs` | `docker:write` | モジュールの実行ログを消去 |
| GET | `/api/docker/tasks` | `docker:read` | バックグラウンドタスクを一覧表示 |
| GET | `/api/docker/tasks/{id}` | `docker:read` | バックグラウンドタスクの進行状況と出力を表示 |
| POST | `/api/docker/tasks/{id}/cancel` | `docker:write` | 実行中のバックグラウンドタスクを中断 |
| DELETE | `/api/docker/tasks` | `docker:write` | 終了済みのバックグラウンドタスクをクリーンアップ |
| GET | `/api/docker/images` | `docker:read` | イメージを一覧表示 |
| POST | `/api/docker/images/pull` | `docker:write` | イメージを取得 |
| POST | `/api/docker/images/push` | `docker:write` | イメージをプッシュ |
| POST | `/api/docker/images/search` | `docker:write` | イメージを検索 |
| POST | `/api/docker/images/load` | `docker:write` | イメージファイルをインポート |
| POST | `/api/docker/images/build` | `docker:write` | イメージをビルド |
| GET | `/api/docker/images/{id}` | `docker:read` | イメージの詳細を表示 |
| POST | `/api/docker/images/{id}/tag` | `docker:write` | イメージにタグを付与 |
| DELETE | `/api/docker/images/{id}` | `docker:write` | イメージを削除 |
| GET | `/api/docker/images/{id}/export` | `docker:write` | イメージをエクスポート |
| GET | `/api/docker/networks` | `docker:read` | ネットワークを一覧表示 |
| POST | `/api/docker/networks` | `docker:write` | ネットワークを作成 |
| DELETE | `/api/docker/networks/{id}` | `docker:write` | ネットワークを削除 |
| POST | `/api/docker/networks/{id}/connect` | `docker:write` | コンテナをネットワークに接続 |
| POST | `/api/docker/networks/{id}/disconnect` | `docker:write` | コンテナをネットワークから切断 |
| GET | `/api/docker/volumes` | `docker:read` | ボリュームを一覧表示 |
| POST | `/api/docker/volumes` | `docker:write` | データボリュームを作成 |
| DELETE | `/api/docker/volumes/{name}` | `docker:write` | データボリュームを削除 |
| GET | `/api/docker/volumes/{name}/export` | `docker:write` | データボリュームをエクスポート |
| POST | `/api/docker/volumes/{name}/import` | `docker:write` | データボリュームにデータをインポート |
| GET | `/api/docker/compose/projects` | `docker:read` | Compose プロジェクトを一覧表示 |
| POST | `/api/docker/compose/projects` | `docker:write` | Compose プロジェクトを作成 |
| POST | `/api/docker/compose/{project}/up` | `docker:write` | Compose プロジェクトを起動 |
| POST | `/api/docker/compose/{project}/pull` | `docker:write` | Compose プロジェクトのイメージを取得 |
| POST | `/api/docker/compose/{project}/{action}` | `docker:write` | Compose プロジェクトに対して操作を実行 |
| GET | `/api/docker/compose/{project}/config` | `docker:write` | Compose 設定ファイルを読み取り |
| PUT | `/api/docker/compose/{project}/config` | `docker:write` | Compose 設定ファイルを保存 |
| GET | `/api/docker/compose/{project}/logs` | `docker:read` | Compose プロジェクトのログを表示 |
| DELETE | `/api/docker/compose/{project}` | `docker:write` | Compose プロジェクトを削除 |
| GET | `/api/docker/info` | `docker:read` | Docker エンジン情報を表示 |
| GET | `/api/docker/disk-usage` | `docker:read` | Docker のディスク使用量を表示 |
| POST | `/api/docker/prune` | `docker:write` | 不要な Docker リソースをクリーンアップ |
| GET | `/api/docker/settings` | `docker:read` | Docker モジュール設定を読み取り |
| PUT | `/api/docker/settings` | `docker:write` | Docker モジュールの設定を保存 |
| GET | `/api/docker/settings/secrets` | `docker:write` | レジストリのパスワードを平文で表示（キーは `registry:<レジストリアドレス>`） |
| GET | `/api/docker/containers` | `docker:read` | コンテナを一覧表示 |
| POST | `/api/docker/containers` | `docker:write` | コンテナを作成 |
| POST | `/api/docker/containers/check-all` | `docker:write` | すべてのコンテナイメージの更新を確認 |
| POST | `/api/docker/containers/upgrade-batch` | `docker:write` | コンテナを一括アップグレード |
| POST | `/api/docker/images/upgrade-check` | `docker:write` | イメージに新しいバージョンがあるか確認 |
| GET | `/api/docker/containers/{id}` | `docker:read` | コンテナの詳細を表示 |
| GET | `/api/docker/containers/{id}/secrets` | `docker:write` | コンテナの環境変数と起動コマンドを平文で表示 |
| DELETE | `/api/docker/containers/{id}` | `docker:write` | コンテナを削除 |
| POST | `/api/docker/containers/{id}/edit` | `docker:write` | コンテナ設定を変更して再作成 |
| POST | `/api/docker/containers/{id}/rename` | `docker:write` | コンテナの名前を変更 |
| POST | `/api/docker/containers/{id}/commit` | `docker:write` | コンテナをイメージとしてコミット |
| POST | `/api/docker/containers/{id}/check` | `docker:write` | コンテナイメージに更新があるか確認 |
| POST | `/api/docker/containers/{id}/upgrade` | `docker:write` | コンテナイメージをアップグレードして再作成 |
| POST | `/api/docker/containers/{id}/{action}` | `docker:write` | コンテナに対して操作を実行 |
| GET | `/api/docker/containers/{id}/logs` | `docker:read` | コンテナのログを表示 |
| GET | `/api/docker/containers/{id}/stats` | `docker:read` | コンテナのリソース使用量を表示 |
| GET | `/api/docker/containers/{id}/processes` | `docker:read` | コンテナ内のプロセスを表示 |
| GET | `/api/docker/containers/{id}/files/list` | `docker:write` | コンテナ内のディレクトリを一覧表示 |
| GET | `/api/docker/containers/{id}/files/read` | `docker:write` | コンテナ内のファイルを読み取り |
| GET | `/api/docker/containers/{id}/files/download` | `docker:write` | コンテナ内のファイルをダウンロード |
| POST | `/api/docker/containers/{id}/files/write` | `docker:write` | コンテナ内のファイルに書き込み |
| POST | `/api/docker/containers/{id}/files/mkdir` | `docker:write` | コンテナ内にディレクトリを作成 |
| POST | `/api/docker/containers/{id}/files/rename` | `docker:write` | コンテナ内のファイルの名前を変更 |
| POST | `/api/docker/containers/{id}/files/upload` | `docker:write` | コンテナにファイルをアップロード |
| DELETE | `/api/docker/containers/{id}/files` | `docker:write` | コンテナ内のファイルを削除 |
| GET | `/api/docker/containers/{id}/exec` | `docker:write` | コンテナ内でターミナルを開く（WebSocket） |

## WebDAV サービス

| メソッド | パス | 必要な権限 | 説明 |
| --- | --- | --- | --- |
| GET | `/api/webdav/logs` | `webdav:read` | モジュールの実行ログを表示 |
| GET | `/api/webdav/logs/sources` | `webdav:read` | モジュールログのすべてのソースを一覧表示 |
| GET | `/api/webdav/logs/export` | `webdav:read` | モジュールログをエクスポート |
| DELETE | `/api/webdav/logs` | `webdav:write` | モジュールの実行ログを消去 |
| GET | `/api/webdav/settings` | `webdav:read` | WebDAV 設定を読み取り |
| GET | `/api/webdav/settings/secrets` | `webdav:write` | ローカルユーザーのパスワードを平文で表示 |
| GET | `/api/webdav/shared-users` | `webdav:read` | ファイルサービスの共有ユーザーを取得（旧エンドポイント） |
| PUT | `/api/webdav/shared-users` | `webdav:write,ftp:write,smb:write` | ファイルサービスの共有ユーザーを保存（旧エンドポイント） |
| PUT | `/api/webdav/settings` | `webdav:write` | WebDAV 設定を保存 |

## FTP サービス

| メソッド | パス | 必要な権限 | 説明 |
| --- | --- | --- | --- |
| GET | `/api/ftp/logs` | `ftp:read` | モジュールの実行ログを表示 |
| GET | `/api/ftp/logs/sources` | `ftp:read` | モジュールログのすべてのソースを一覧表示 |
| GET | `/api/ftp/logs/export` | `ftp:read` | モジュールログをエクスポート |
| DELETE | `/api/ftp/logs` | `ftp:write` | モジュールの実行ログを消去 |
| GET | `/api/ftp/settings` | `ftp:read` | FTP 設定を読み取り |
| GET | `/api/ftp/settings/secrets` | `ftp:write` | ローカルユーザーのパスワードを平文で表示 |
| GET | `/api/ftp/shared-users` | `ftp:read` | ファイルサービスの共有ユーザーを取得（旧エンドポイント） |
| PUT | `/api/ftp/shared-users` | `webdav:write,ftp:write,smb:write` | ファイルサービスの共有ユーザーを保存（旧エンドポイント） |
| PUT | `/api/ftp/settings` | `ftp:write` | FTP 設定を保存 |

## SMB サービス

| メソッド | パス | 必要な権限 | 説明 |
| --- | --- | --- | --- |
| GET | `/api/smb/logs` | `smb:read` | モジュールの実行ログを表示 |
| GET | `/api/smb/logs/sources` | `smb:read` | モジュールログのすべてのソースを一覧表示 |
| GET | `/api/smb/logs/export` | `smb:read` | モジュールログをエクスポート |
| DELETE | `/api/smb/logs` | `smb:write` | モジュールの実行ログを消去 |
| GET | `/api/smb/settings` | `smb:read` | SMB 設定を取得 |
| GET | `/api/smb/settings/secrets` | `smb:write` | ローカルユーザーのパスワードを平文で表示 |
| GET | `/api/smb/shared-users` | `smb:read` | ファイルサービスの共有ユーザーを取得（旧エンドポイント） |
| PUT | `/api/smb/shared-users` | `webdav:write,ftp:write,smb:write` | ファイルサービスの共有ユーザーを保存（旧エンドポイント） |
| PUT | `/api/smb/settings` | `smb:write` | SMB 設定を保存 |

## File Browser

| メソッド | パス | 必要な権限 | 説明 |
| --- | --- | --- | --- |
| GET | `/api/filebrowser/logs` | `filebrowser:read` | モジュールの実行ログを表示 |
| GET | `/api/filebrowser/logs/sources` | `filebrowser:read` | モジュールログのすべてのソースを一覧表示 |
| GET | `/api/filebrowser/logs/export` | `filebrowser:read` | モジュールログをエクスポート |
| DELETE | `/api/filebrowser/logs` | `filebrowser:write` | モジュールの実行ログを消去 |
| GET | `/api/filebrowser/instances` | `filebrowser:read` | File Browser インスタンス の一覧を取得 |
| POST | `/api/filebrowser/instances` | `filebrowser:write` | File Browser インスタンス を追加 |
| POST | `/api/filebrowser/instances/{key}/copy` | `filebrowser:write` | File Browser インスタンス を複製 |
| PUT | `/api/filebrowser/instances/{key}` | `filebrowser:write` | File Browser インスタンス を更新 |
| DELETE | `/api/filebrowser/instances/{key}` | `filebrowser:write` | File Browser インスタンス を削除 |
| PUT | `/api/filebrowser/instances/{key}/enable` | `filebrowser:write` | File Browser インスタンス の有効/無効を切り替え |
| PUT | `/api/filebrowser/instances-order` | `filebrowser:write` | File Browser インスタンス の並び順を変更 |
| PUT | `/api/filebrowser/instances-enable` | `filebrowser:write` | File Browser インスタンス の一括有効化・無効化 |
| POST | `/api/filebrowser/instances-delete` | `filebrowser:write` | File Browser インスタンス の一括削除 |
| PUT | `/api/filebrowser/instances-group` | `filebrowser:write` | File Browser インスタンス のグループを一括設定 |
| GET | `/api/filebrowser/instances/{key}/secrets` | `filebrowser:write` | シークレットを平文で表示 |
| POST | `/api/filebrowser/instances/{key}/restart` | `filebrowser:write` | File Browser インスタンスを再起動 |
| GET | `/api/filebrowser/settings` | `filebrowser:read` | File Browser のプログラム設定を取得 |
| PUT | `/api/filebrowser/settings` | `filebrowser:write` | File Browser のプログラム設定を保存 |
| GET | `/api/filebrowser/install` | `filebrowser:read` | 管理プログラムのインストール情報と進捗を表示 |
| POST | `/api/filebrowser/install` | `filebrowser:write` | 管理プログラムをオンラインでインストールまたは更新 |
| POST | `/api/filebrowser/install/check` | `filebrowser:write` | 管理プログラムの更新を確認 |
| POST | `/api/filebrowser/install/rollback` | `filebrowser:write` | 保存された旧バージョンに切り替え |
| DELETE | `/api/filebrowser/install` | `filebrowser:write` | 管理プログラムをアンインストール |
| DELETE | `/api/filebrowser/install/backups/{id}` | `filebrowser:write` | 保存された旧バージョンを削除 |
| POST | `/api/filebrowser/reset` | `filebrowser:write` | モジュールのデフォルト設定に戻す |

## ストレージマウント

| メソッド | パス | 必要な権限 | 説明 |
| --- | --- | --- | --- |
| GET | `/api/storage/logs` | `storage:read` | モジュールの実行ログを表示 |
| GET | `/api/storage/logs/sources` | `storage:read` | モジュールログのすべてのソースを一覧表示 |
| GET | `/api/storage/logs/export` | `storage:read` | モジュールログをエクスポート |
| DELETE | `/api/storage/logs` | `storage:write` | モジュールの実行ログを消去 |
| GET | `/api/storage/mounts` | `storage:read` | ストレージマウント の一覧を取得 |
| POST | `/api/storage/mounts` | `storage:write` | ストレージマウント を追加 |
| POST | `/api/storage/mounts/{key}/copy` | `storage:write` | ストレージマウント を複製 |
| PUT | `/api/storage/mounts/{key}` | `storage:write` | ストレージマウント を更新 |
| DELETE | `/api/storage/mounts/{key}` | `storage:write` | ストレージマウント を削除 |
| PUT | `/api/storage/mounts/{key}/enable` | `storage:write` | ストレージマウント の有効/無効を切り替え |
| PUT | `/api/storage/mounts-order` | `storage:write` | ストレージマウント の並び順を変更 |
| PUT | `/api/storage/mounts-enable` | `storage:write` | ストレージマウント の一括有効化・無効化 |
| POST | `/api/storage/mounts-delete` | `storage:write` | ストレージマウント の一括削除 |
| PUT | `/api/storage/mounts-group` | `storage:write` | ストレージマウント のグループを一括設定 |
| GET | `/api/storage/mounts/{key}/secrets` | `storage:write` | シークレットを平文で表示 |
| GET | `/api/storage/files/{key}/list` | `storage:write` | ストレージのディレクトリを一覧表示 |
| GET | `/api/storage/files/{key}/download` | `storage:write` | ストレージのファイルをダウンロード |
| POST | `/api/storage/files/{key}/upload` | `storage:write` | ストレージにファイルをアップロード |
| POST | `/api/storage/files/{key}/mkdir` | `storage:write` | ストレージディレクトリを作成 |
| POST | `/api/storage/files/{key}/rename` | `storage:write` | ストレージのファイルの名前変更または移動 |
| DELETE | `/api/storage/files/{key}` | `storage:write` | ストレージのファイルを削除 |

## 通知センター

| メソッド | パス | 必要な権限 | 説明 |
| --- | --- | --- | --- |
| GET | `/api/notify/logs` | `notify:read` | モジュールの実行ログを表示 |
| GET | `/api/notify/logs/sources` | `notify:read` | モジュールログのすべてのソースを一覧表示 |
| GET | `/api/notify/logs/export` | `notify:read` | モジュールログをエクスポート |
| DELETE | `/api/notify/logs` | `notify:write` | モジュールの実行ログを消去 |
| GET | `/api/notify/channels` | `notify:read` | 通知チャネル の一覧を取得 |
| POST | `/api/notify/channels` | `notify:write` | 通知チャネル を追加 |
| POST | `/api/notify/channels/{key}/copy` | `notify:write` | 通知チャネル を複製 |
| PUT | `/api/notify/channels/{key}` | `notify:write` | 通知チャネル を更新 |
| DELETE | `/api/notify/channels/{key}` | `notify:write` | 通知チャネル を削除 |
| PUT | `/api/notify/channels/{key}/enable` | `notify:write` | 通知チャネル の有効/無効を切り替え |
| PUT | `/api/notify/channels-order` | `notify:write` | 通知チャネル の並び順を変更 |
| PUT | `/api/notify/channels-enable` | `notify:write` | 通知チャネル の一括有効化・無効化 |
| POST | `/api/notify/channels-delete` | `notify:write` | 通知チャネル の一括削除 |
| PUT | `/api/notify/channels-group` | `notify:write` | 通知チャネル のグループを一括設定 |
| GET | `/api/notify/channels/{key}/secrets` | `notify:write` | シークレットを平文で表示 |
| GET | `/api/notify/rules` | `notify:read` | 通知ルール の一覧を取得 |
| POST | `/api/notify/rules` | `notify:write` | 通知ルール を追加 |
| POST | `/api/notify/rules/{key}/copy` | `notify:write` | 通知ルール を複製 |
| PUT | `/api/notify/rules/{key}` | `notify:write` | 通知ルール を更新 |
| DELETE | `/api/notify/rules/{key}` | `notify:write` | 通知ルール を削除 |
| PUT | `/api/notify/rules/{key}/enable` | `notify:write` | 通知ルール の有効/無効を切り替え |
| PUT | `/api/notify/rules-order` | `notify:write` | 通知ルール の並び順を変更 |
| PUT | `/api/notify/rules-enable` | `notify:write` | 通知ルール の一括有効化・無効化 |
| POST | `/api/notify/rules-delete` | `notify:write` | 通知ルール の一括削除 |
| PUT | `/api/notify/rules-group` | `notify:write` | 通知ルール のグループを一括設定 |
| GET | `/api/notify/settings` | `notify:read` | 通知設定を取得 |
| PUT | `/api/notify/settings` | `notify:write` | 通知設定を保存 |
| GET | `/api/notify/event-types` | `notify:read` | 購読可能なイベントタイプ |
| GET | `/api/notify/history` | `notify:read` | 最近の配信履歴 |
| POST | `/api/notify/channels/{key}/test` | `notify:write` | チャネルにテスト通知を送信 |

## パフォーマンス監視

| メソッド | パス | 必要な権限 | 説明 |
| --- | --- | --- | --- |
| GET | `/api/perf/logs` | `perf:read` | モジュールの実行ログを表示 |
| GET | `/api/perf/logs/sources` | `perf:read` | モジュールログのすべてのソースを一覧表示 |
| GET | `/api/perf/logs/export` | `perf:read` | モジュールログをエクスポート |
| DELETE | `/api/perf/logs` | `perf:write` | モジュールの実行ログを消去 |
| GET | `/api/perf/rules` | `perf:read` | パフォーマンスアラートルール の一覧を取得 |
| POST | `/api/perf/rules` | `perf:write` | パフォーマンスアラートルール を追加 |
| POST | `/api/perf/rules/{key}/copy` | `perf:write` | パフォーマンスアラートルール を複製 |
| PUT | `/api/perf/rules/{key}` | `perf:write` | パフォーマンスアラートルール を更新 |
| DELETE | `/api/perf/rules/{key}` | `perf:write` | パフォーマンスアラートルール を削除 |
| PUT | `/api/perf/rules/{key}/enable` | `perf:write` | パフォーマンスアラートルール の有効/無効を切り替え |
| PUT | `/api/perf/rules-order` | `perf:write` | パフォーマンスアラートルール の並び順を変更 |
| PUT | `/api/perf/rules-enable` | `perf:write` | パフォーマンスアラートルール の一括有効化・無効化 |
| POST | `/api/perf/rules-delete` | `perf:write` | パフォーマンスアラートルール の一括削除 |
| PUT | `/api/perf/rules-group` | `perf:write` | パフォーマンスアラートルール のグループを一括設定 |
| GET | `/api/perf/metrics` | `perf:read` | アラートに使える指標 |
| GET | `/api/perf/overview` | `perf:read` | ホストとプロセスの履歴（?range=60s|1h|24h） |
| GET | `/api/perf/sites` | `perf:read` | 各サイトの直近約 60 秒の指標 |
| GET | `/api/perf/alerts` | `perf:read` | 進行中のアラート（計測中と発報中） |
| GET | `/api/perf/processes` | `perf:read` | 管理対象プロセス（frpc、tailscaled など）の現在と直近約 60 秒のリソース使用量 |

## ネットワーク診断

| メソッド | パス | 必要な権限 | 説明 |
| --- | --- | --- | --- |
| GET | `/api/nettools/logs` | `nettools:read` | モジュールの実行ログを表示 |
| GET | `/api/nettools/logs/sources` | `nettools:read` | モジュールログのすべてのソースを一覧表示 |
| GET | `/api/nettools/logs/export` | `nettools:read` | モジュールログをエクスポート |
| DELETE | `/api/nettools/logs` | `nettools:write` | モジュールの実行ログを消去 |
| POST | `/api/nettools/run` | `nettools:write` | 診断ツールを実行してタスクを返す（tool: ping / traceroute / tcp / dns / speed / scan） |
| GET | `/api/nettools/tasks` | `nettools:read` | 最近の診断タスク |
| GET | `/api/nettools/tasks/{id}` | `nettools:read` | タスクの詳細と出力 |
| DELETE | `/api/nettools/tasks/{id}` | `nettools:write` | 実行中のタスクをキャンセル、または終了済みのタスクを削除 |
| GET | `/api/nettools/settings` | `nettools:read` | 診断設定 |
| PUT | `/api/nettools/settings` | `nettools:write` | スキャン禁止リストを変更 |
| GET | `/api/nettools/networks` | `nettools:read` | LAN スキャンに使用できるローカルのネットワークセグメント |

## 設定バックアップ

| メソッド | パス | 必要な権限 | 説明 |
| --- | --- | --- | --- |
| GET | `/api/backup/logs` | `backup:read` | モジュールの実行ログを表示 |
| GET | `/api/backup/logs/sources` | `backup:read` | モジュールログのすべてのソースを一覧表示 |
| GET | `/api/backup/logs/export` | `backup:read` | モジュールログをエクスポート |
| DELETE | `/api/backup/logs` | `backup:write` | モジュールの実行ログを消去 |
| GET | `/api/backup/targets` | `backup:read` | バックアップ先 の一覧を取得 |
| POST | `/api/backup/targets` | `backup:write` | バックアップ先 を追加 |
| POST | `/api/backup/targets/{key}/copy` | `backup:write` | バックアップ先 を複製 |
| PUT | `/api/backup/targets/{key}` | `backup:write` | バックアップ先 を更新 |
| DELETE | `/api/backup/targets/{key}` | `backup:write` | バックアップ先 を削除 |
| PUT | `/api/backup/targets/{key}/enable` | `backup:write` | バックアップ先 の有効/無効を切り替え |
| PUT | `/api/backup/targets-order` | `backup:write` | バックアップ先 の並び順を変更 |
| PUT | `/api/backup/targets-enable` | `backup:write` | バックアップ先 の一括有効化・無効化 |
| POST | `/api/backup/targets-delete` | `backup:write` | バックアップ先 の一括削除 |
| GET | `/api/backup/settings` | `backup:read` | バックアップ設定 |
| PUT | `/api/backup/settings` | `backup:write` | バックアップ設定を変更（パスフレーズにプレースホルダーを渡すと変更なし） |
| GET | `/api/backup/secrets` | `backup:write` | シークレットを平文で表示 |
| POST | `/api/backup/run` | `backup:write` | 今すぐバックアップを 1 回実行 |
| GET | `/api/backup/status` | `backup:read` | 直近のバックアップ結果と次回実行時刻 |
| GET | `/api/backup/mounts` | `backup:read` | バックアップ先として使用できるストレージのマウントポイント |
| GET | `/api/backup/local` | ログインセッションのみ | ローカルバックアップファイル一覧 |
| GET | `/api/backup/local/{name}` | ログインセッションのみ | ローカルバックアップファイルをダウンロード |

## ファイルサービス共有ユーザー

| メソッド | パス | 必要な権限 | 説明 |
| --- | --- | --- | --- |
| GET | `/api/fileusers` | `fileusers:read` | ファイルサービスの共有ユーザーを取得 |
| PUT | `/api/fileusers` | `fileusers:write` | ファイルサービスの共有ユーザーを保存 |
| GET | `/api/fileusers/secrets` | `fileusers:write` | 共有ユーザーのパスワードを平文で表示 |

## システム

| メソッド | パス | 必要な権限 | 説明 |
| --- | --- | --- | --- |
| GET | `/api/session` | 任意の有効なトークン | 現在の認証 ID を表示 |
| GET | `/api/status` | `status:read` | システムと各モジュールの状態を表示 |
| GET | `/api/logs` | `logs:read` | システムログを表示 |
| GET | `/api/logs/sources` | `logs:read` | システムログのすべてのソースを一覧表示 |
| GET | `/api/logs/export` | `logs:read` | システムログをエクスポート |
| GET | `/api/system/settings` | ログインセッションのみ | グローバル設定を読み取り |
| PUT | `/api/system/settings` | ログインセッションのみ | グローバル設定を保存 |
| GET | `/api/system/settings/secrets` | ログインセッションのみ | サードパーティログインの Client Secret を平文で表示 |
| GET | `/api/system/info` | `status:read` | システム情報を表示 |
| GET | `/api/system/update` | `status:read` | 新しいバージョンとセキュリティアドバイザリーを確認 |
| GET | `/api/system/update/state` | `status:read` | 前回の更新チェックの結果を表示 |
| POST | `/api/system/upgrade` | ログインセッションのみ | 最新バージョンへオンラインアップグレード |
| GET | `/api/system/upgrade/status` | ログインセッションのみ | オンラインアップグレードの進捗を確認 |
| POST | `/api/system/upgrade/cancel` | ログインセッションのみ | オンラインアップグレードを取消 |
| POST | `/api/system/upgrade/upload` | ログインセッションのみ | アップロードしたパッケージでアップグレード |
| GET | `/api/system/backup` | ログインセッションのみ | 設定バックアップをダウンロード |
| GET | `/api/system/diagnostics` | ログインセッションのみ | マスク済み診断パッケージをエクスポート |
| GET | `/api/audit` | ログインセッションのみ | 操作監査ログを検索 |
| GET | `/api/metrics` | `metrics:read` | Prometheus グローバルメトリクス |
| GET | `/api/history` | ログインセッションのみ | 設定の履歴バージョンを一覧表示 |
| GET | `/api/history/{name}/{id}/diff` | ログインセッションのみ | 履歴バージョンと現在の設定を比較 |
| POST | `/api/history/{name}/{id}/restore` | ログインセッションのみ | 設定を履歴バージョンにロールバック |
| POST | `/api/system/restore` | ログインセッションのみ | バックアップから設定を復元 |
| POST | `/api/system/restart` | ログインセッションのみ | Gately を再起動 |
| POST | `/api/system/selfcheck` | ログインセッションのみ | ネットワークと設定のセルフチェック |
| GET | `/api/system/onboarding` | ログインセッションのみ | 初回起動セットアップが必要かどうか |
| POST | `/api/system/onboarding/dismiss` | ログインセッションのみ | 初回起動セットアップを完了またはスキップ |
| GET | `/api/openapi.json` | 任意の有効なトークン | この API の OpenAPI 定義を取得 |

## 設定のインポート

| メソッド | パス | 必要な権限 | 説明 |
| --- | --- | --- | --- |
| POST | `/api/import/lucky` | `import:write` | Lucky 設定をインポート |

## API トークン

| メソッド | パス | 必要な権限 | 説明 |
| --- | --- | --- | --- |
| GET | `/api/tokens` | ログインセッションのみ | API トークンを一覧表示 |
| GET | `/api/tokens/scopes` | ログインセッションのみ | 付与可能な権限ドメインを一覧表示 |
| POST | `/api/tokens` | ログインセッションのみ | API トークンを作成 |
| PUT | `/api/tokens/order` | ログインセッションのみ | API トークンの並べ替え |
| PUT | `/api/tokens/group` | ログインセッションのみ | 複数の API トークンのグループを一括設定 |
| PUT | `/api/tokens/{id}` | ログインセッションのみ | API トークンの名前・グループ・権限・有効期限・有効/無効を変更 |
| POST | `/api/tokens/{id}/regenerate` | ログインセッションのみ | トークンの平文を再生成 |
| DELETE | `/api/tokens/{id}` | ログインセッションのみ | API トークンを削除 |

## アカウント

| メソッド | パス | 必要な権限 | 説明 |
| --- | --- | --- | --- |
| PUT | `/api/account` | ログインセッションのみ | 管理者のユーザー名とパスワードを変更 |
| PUT | `/api/account/ui-language` | ログインセッションのみ | 管理画面の言語を記録 |
| GET | `/api/account/passkeys` | ログインセッションのみ | 登録済みの Passkey を一覧表示 |
| POST | `/api/account/passkeys/begin` | ログインセッションのみ | Passkey の登録を開始 |
| POST | `/api/account/passkeys` | ログインセッションのみ | Passkey の登録を完了 |
| DELETE | `/api/account/passkeys/{id}` | ログインセッションのみ | Passkey を削除 |
| GET | `/api/account/users` | ログインセッションのみ | 読み取り専用アカウントを一覧表示 |
| POST | `/api/account/users` | ログインセッションのみ | 読み取り専用アカウントを作成 |
| PUT | `/api/account/users/{name}` | ログインセッションのみ | 読み取り専用アカウントのパスワードをリセット |
| DELETE | `/api/account/users/{name}` | ログインセッションのみ | 読み取り専用アカウントを削除 |
| GET | `/api/account/totp` | ログインセッションのみ | 2 要素認証の状態を表示 |
| POST | `/api/account/totp/setup` | ログインセッションのみ | 2 要素認証のシークレットを生成 |
| POST | `/api/account/totp/enable` | ログインセッションのみ | 2 要素認証を有効化 |
| POST | `/api/account/totp/disable` | ログインセッションのみ | 2 要素認証を無効化 |

## データ構造

### backup.LocalFile

| フィールド | 型 |
| --- | --- |
| `modTime` | 時刻 (RFC 3339) |
| `name` | string |
| `size` | integer |

### backup.Result

| フィールド | 型 |
| --- | --- |
| `error` | string |
| `file` | string |
| `ok` | boolean |
| `pruned` | integer |
| `target` | string |

### backup.Settings

| フィールド | 型 |
| --- | --- |
| `enable` | boolean |
| `keep` | integer |
| `localEnable` | boolean |
| `password` | string |
| `schedule` | string |

### backup.Status

| フィールド | 型 |
| --- | --- |
| `lastRun` | 時刻 (RFC 3339) |
| `manual` | boolean |
| `nextRun` | 時刻 (RFC 3339) |
| `results` | 配列 of `backup.Result` |
| `running` | boolean |

### backup.Target

| フィールド | 型 |
| --- | --- |
| `dir` | string |
| `enable` | boolean |
| `key` | string |
| `mount` | string |
| `name` | string |

### backup.settingsView

| フィールド | 型 |
| --- | --- |
| `enable` | boolean |
| `hasPassword` | boolean |
| `keep` | integer |
| `localEnable` | boolean |
| `password` | string |
| `schedule` | string |

### banlist.Entry

| フィールド | 型 |
| --- | --- |
| `count` | integer |
| `created` | 時刻 (RFC 3339) |
| `key` | string |
| `reason` | string |
| `source` | string |
| `until` | 時刻 (RFC 3339) |

### cloudflared.Tunnel

| フィールド | 型 |
| --- | --- |
| `apiToken` | string |
| `edgeIpVersion` | string |
| `enable` | boolean |
| `group` | string |
| `key` | string |
| `mode` | string |
| `name` | string |
| `protocol` | string |
| `remark` | string |
| `service` | string |
| `token` | string |

### cron.Task

| フィールド | 型 |
| --- | --- |
| `command` | string |
| `enable` | boolean |
| `group` | string |
| `http` | `webhook.Config` |
| `key` | string |
| `name` | string |
| `remark` | string |
| `spec` | string |
| `type` | string |

### crud.batchDeleteRequest

| フィールド | 型 |
| --- | --- |
| `keys` | 配列 of string |

### crud.batchEnableRequest

| フィールド | 型 |
| --- | --- |
| `enable` | boolean |
| `keys` | 配列 of string |

### crud.enableRequest

| フィールド | 型 |
| --- | --- |
| `enable` | boolean |

### crud.groupRequest

| フィールド | 型 |
| --- | --- |
| `group` | string |
| `keys` | 配列 of string |

### crud.orderRequest

| フィールド | 型 |
| --- | --- |
| `keys` | 配列 of string |

### ddns.Record

| フィールド | 型 |
| --- | --- |
| `enable` | boolean |
| `group` | string |
| `key` | string |
| `line` | string |
| `remark` | string |
| `sub` | string |
| `ttl` | integer |
| `type` | string |
| `value` | string |
| `zone` | string |

### ddns.RemoteDeleteResult

| フィールド | 型 |
| --- | --- |
| `deleted` | integer |
| `domain` | string |
| `error` | string |
| `key` | string |
| `type` | string |

### ddns.Task

| フィールド | 型 |
| --- | --- |
| `credentials` | `dnsprovider.Credentials` |
| `enable` | boolean |
| `forceInterval` | integer |
| `group` | string |
| `httpTimeout` | integer |
| `interval` | integer |
| `ipv4` | `ipsource.Source` |
| `ipv6` | `ipsource.Source` |
| `key` | string |
| `name` | string |
| `provider` | string |
| `records` | 配列 of `ddns.Record` |
| `remark` | string |
| `webhook` | `webhook.Config` |

### ddns.recordLayout

| フィールド | 型 |
| --- | --- |
| `group` | string |
| `key` | string |

### ddns.recordsLayoutRequest

| フィールド | 型 |
| --- | --- |
| `records` | 配列 of `ddns.recordLayout` |

### ddns.remoteDeleteRequest

| フィールド | 型 |
| --- | --- |
| `records` | 配列 of string |
| `task` | `ddns.Task` |

### dnsprovider.Credentials

| フィールド | 型 |
| --- | --- |
| `extra` | マップ of string |
| `id` | string |
| `secret` | string |

### events.TypeInfo

| フィールド | 型 |
| --- | --- |
| `level` | string |
| `name` | string |
| `type` | string |

### filebrowser.Instance

| フィールド | 型 |
| --- | --- |
| `accessUrls` | 配列 of string |
| `adminPassword` | string |
| `adminUser` | string |
| `authMethod` | string |
| `baseUrl` | string |
| `disableWebDav` | boolean |
| `enable` | boolean |
| `group` | string |
| `key` | string |
| `listenIp` | string |
| `locale` | string |
| `name` | string |
| `port` | integer |
| `proxyHeader` | string |
| `remark` | string |
| `sources` | 配列 of `filebrowser.Source` |
| `tokenHours` | integer |
| `trustProxyHeaders` | boolean |

### filebrowser.Source

| フィールド | 型 |
| --- | --- |
| `name` | string |
| `path` | string |
| `readOnly` | boolean |

### frp.BinaryView

| フィールド | 型 |
| --- | --- |
| `binary` | string |
| `error` | string |
| `version` | string |

### frp.Client

| フィールド | 型 |
| --- | --- |
| `access` | `frp.ClientAccess` |
| `enable` | boolean |
| `group` | string |
| `key` | string |
| `name` | string |
| `protocol` | string |
| `remark` | string |
| `serverAddr` | string |
| `serverPort` | integer |
| `token` | string |
| `user` | string |

### frp.ClientAccess

| フィールド | 型 |
| --- | --- |
| `httpPort` | integer |
| `httpScheme` | string |
| `httpsPort` | integer |
| `subdomainHost` | string |

### frp.ClientSettings

| フィールド | 型 |
| --- | --- |
| `protocol` | string |
| `serverAddr` | string |
| `serverPort` | integer |
| `token` | string |
| `user` | string |

### frp.ImportRequest

| フィールド | 型 |
| --- | --- |
| `apply` | boolean |
| `content` | string |
| `name` | string |
| `proxyMode` | string |
| `role` | string |
| `target` | string |

### frp.ImportResponse

| フィールド | 型 |
| --- | --- |
| `added` | integer |
| `applied` | boolean |
| `client` | `frp.ClientSettings` |
| `key` | string |
| `proxies` | 配列 of `frp.Proxy` |
| `role` | string |
| `server` | `frp.ServerSettings` |
| `warnings` | 配列 of string |

### frp.Proxy

| フィールド | 型 |
| --- | --- |
| `client` | string |
| `customDomains` | 配列 of string |
| `enable` | boolean |
| `group` | string |
| `key` | string |
| `localIp` | string |
| `localPort` | integer |
| `name` | string |
| `remark` | string |
| `remotePort` | integer |
| `secretKey` | string |
| `subdomain` | string |
| `type` | string |

### frp.Server

| フィールド | 型 |
| --- | --- |
| `allowPorts` | string |
| `bindAddr` | string |
| `bindPort` | integer |
| `dashboardAddr` | string |
| `dashboardPort` | integer |
| `dashboardPwd` | string |
| `dashboardUser` | string |
| `enable` | boolean |
| `group` | string |
| `ipList` | 配列 of string |
| `ipMode` | string |
| `ipSets` | 配列 of string |
| `key` | string |
| `name` | string |
| `proxyBindAddr` | string |
| `remark` | string |
| `subdomainHost` | string |
| `token` | string |
| `vhostHttpPort` | integer |
| `vhostHttpsPort` | integer |

### frp.ServerSettings

| フィールド | 型 |
| --- | --- |
| `allowPorts` | string |
| `bindAddr` | string |
| `bindPort` | integer |
| `dashboardAddr` | string |
| `dashboardPort` | integer |
| `dashboardPwd` | string |
| `dashboardUser` | string |
| `ipList` | 配列 of string |
| `ipMode` | string |
| `ipSets` | 配列 of string |
| `proxyBindAddr` | string |
| `subdomainHost` | string |
| `token` | string |
| `vhostHttpPort` | integer |
| `vhostHttpsPort` | integer |

### frp.Settings

| フィールド | 型 |
| --- | --- |
| `frpcPath` | string |
| `frpsPath` | string |

### frp.SettingsView

| フィールド | 型 |
| --- | --- |
| `frpc` | `frp.BinaryView` |
| `frps` | `frp.BinaryView` |
| `settings` | `frp.Settings` |

### geoip.Info

| フィールド | 型 |
| --- | --- |
| `buildTime` | 時刻 (RFC 3339) |
| `loaded` | boolean |
| `size` | integer |
| `type` | string |

### guard.BanView

| フィールド | 型 |
| --- | --- |
| `count` | integer |
| `created` | 時刻 (RFC 3339) |
| `jail` | string |
| `key` | string |
| `reason` | string |
| `source` | string |
| `until` | 時刻 (RFC 3339) |

### guard.FirewallStatus

| フィールド | 型 |
| --- | --- |
| `active` | boolean |
| `backend` | string |
| `checkedAt` | 時刻 (RFC 3339) |
| `dropped` | マップ of integer |
| `enabled` | boolean |
| `error` | string |
| `hits` | マップ of integer |
| `openPorts` | integer |
| `reason` | string |
| `trapPorts` | string |
| `trapSkipped` | 配列 of integer |
| `version` | string |

### guard.Jail

| フィールド | 型 |
| --- | --- |
| `anyStatus` | boolean |
| `banTime` | integer |
| `enable` | boolean |
| `filter` | string |
| `findTime` | integer |
| `group` | string |
| `ignore` | string |
| `key` | string |
| `maxRetry` | integer |
| `name` | string |
| `preset` | string |
| `remark` | string |
| `source` | string |
| `target` | string |
| `unknownHost` | boolean |

### guard.Platform

| フィールド | 型 |
| --- | --- |
| `docker` | boolean |
| `inDocker` | boolean |
| `journal` | boolean |
| `os` | string |

### guard.ScanSettings

| フィールド | 型 |
| --- | --- |
| `banSeconds` | integer |
| `enable` | boolean |
| `excludePorts` | string |
| `ipv6Strict` | boolean |
| `threshold` | integer |

### guard.Settings

| フィールド | 型 |
| --- | --- |
| `banSeconds` | integer |
| `firewall` | boolean |
| `ignoreList` | 配列 of string |
| `ignorePrivate` | boolean |
| `increment` | boolean |
| `maxBanSeconds` | integer |
| `scan` | `guard.ScanSettings` |
| `trap` | `guard.TrapSettings` |

### guard.Status

| フィールド | 型 |
| --- | --- |
| `bans` | integer |
| `firewall` | `guard.FirewallStatus` |
| `platform` | `guard.Platform` |
| `userTrapPorts` | 配列 of integer |

### guard.TestResult

| フィールド | 型 |
| --- | --- |
| `host` | string |
| `ignored` | boolean |
| `line` | string |
| `matched` | boolean |

### guard.TrapSettings

| フィールド | 型 |
| --- | --- |
| `banSeconds` | integer |
| `enable` | boolean |
| `ports` | string |

### guard.banRequest

| フィールド | 型 |
| --- | --- |
| `key` | string |
| `reason` | string |
| `seconds` | integer |

### guard.testRequest

| フィールド | 型 |
| --- | --- |
| `filter` | string |
| `ignore` | string |
| `lines` | string |
| `source` | string |

### ipsource.Source

| フィールド | 型 |
| --- | --- |
| `command` | string |
| `enable` | boolean |
| `interface` | string |
| `regex` | string |
| `type` | string |
| `urls` | 配列 of string |

### netbird.ConnView

| フィールド | 型 |
| --- | --- |
| `connected` | boolean |
| `error` | string |
| `url` | string |

### netbird.Instance

| フィールド | 型 |
| --- | --- |
| `acceptDns` | boolean |
| `acceptRoutes` | boolean |
| `blockInbound` | boolean |
| `daemonAddr` | string |
| `daemonEnv` | string |
| `daemonExtraArgs` | string |
| `enable` | boolean |
| `extraUpArgs` | string |
| `group` | string |
| `hostname` | string |
| `key` | string |
| `legacyStateDir` | boolean |
| `localForwarding` | boolean |
| `manageDaemon` | boolean |
| `managementUrl` | string |
| `name` | string |
| `port` | integer |
| `remark` | string |
| `serverRoutes` | boolean |
| `setupKey` | string |
| `socks5` | string |
| `stateDir` | string |
| `userspace` | boolean |

### netbird.PeerView

| フィールド | 型 |
| --- | --- |
| `connType` | string |
| `fqdn` | string |
| `ips` | 配列 of string |
| `lastSeen` | 時刻 (RFC 3339) |
| `latencyMs` | number |
| `name` | string |
| `online` | boolean |
| `relay` | string |
| `routes` | 配列 of string |
| `rxBytes` | integer |
| `status` | string |
| `txBytes` | integer |

### netbird.SelfView

| フィールド | 型 |
| --- | --- |
| `fqdn` | string |
| `ips` | 配列 of string |
| `kernel` | boolean |
| `name` | string |
| `wgPort` | integer |

### netbird.Settings

| フィールド | 型 |
| --- | --- |
| `installProxy` | string |
| `netbirdPath` | string |

### netbird.SettingsView

| フィールド | 型 |
| --- | --- |
| `cli` | string |
| `defaultManagementUrl` | string |
| `settings` | `netbird.Settings` |

### netbird.StatusView

| フィールド | 型 |
| --- | --- |
| `authCode` | string |
| `authUrl` | string |
| `cli` | string |
| `daemon` | `proc.State` |
| `daemonStatus` | string |
| `enable` | boolean |
| `error` | string |
| `key` | string |
| `management` | `netbird.ConnView` |
| `name` | string |
| `peers` | 配列 of `netbird.PeerView` |
| `peersConnected` | integer |
| `relaysTotal` | integer |
| `relaysUp` | integer |
| `self` | `netbird.SelfView` |
| `signal` | `netbird.ConnView` |
| `version` | string |

### nettools.Request

| フィールド | 型 |
| --- | --- |
| `cidr` | string |
| `count` | integer |
| `ports` | string |
| `seconds` | integer |
| `server` | string |
| `target` | string |
| `tool` | string |
| `type` | string |
| `url` | string |

### nettools.Settings

| フィールド | 型 |
| --- | --- |
| `deny` | 配列 of string |

### nettools.Task

| フィールド | 型 |
| --- | --- |
| `error` | string |
| `finished` | 時刻 (RFC 3339) |
| `id` | string |
| `lines` | 配列 of string |
| `result` | 任意 |
| `started` | 時刻 (RFC 3339) |
| `status` | string |
| `title` | string |
| `tool` | string |

### notify.Channel

| フィールド | 型 |
| --- | --- |
| `enable` | boolean |
| `from` | string |
| `group` | string |
| `host` | string |
| `key` | string |
| `kind` | string |
| `name` | string |
| `port` | integer |
| `proxy` | string |
| `remark` | string |
| `secret` | string |
| `security` | string |
| `target` | string |
| `url` | string |
| `user` | string |
| `webhook` | `webhook.Config` |

### notify.Record

| フィールド | 型 |
| --- | --- |
| `channel` | string |
| `error` | string |
| `ok` | boolean |
| `rule` | string |
| `time` | 時刻 (RFC 3339) |
| `title` | string |
| `type` | string |

### notify.Rule

| フィールド | 型 |
| --- | --- |
| `channels` | 配列 of string |
| `enable` | boolean |
| `events` | 配列 of string |
| `group` | string |
| `key` | string |
| `minLevel` | string |
| `name` | string |
| `silence` | integer |

### notify.Settings

| フィールド | 型 |
| --- | --- |
| `proxy` | string |
| `serverName` | string |

### notify.settingsView

| フィールド | 型 |
| --- | --- |
| `hostname` | string |
| `proxy` | string |
| `serverName` | string |

### perf.AlertState

| フィールド | 型 |
| --- | --- |
| `firedAt` | integer |
| `level` | string |
| `name` | string |
| `rule` | string |
| `since` | integer |
| `state` | string |
| `subject` | string |
| `target` | string |
| `value` | number |

### perf.MetricDef

| フィールド | 型 |
| --- | --- |
| `id` | string |
| `label` | string |
| `max` | number |
| `scope` | string |
| `unit` | string |
| `window` | boolean |

### perf.Overview

| フィールド | 型 |
| --- | --- |
| `latest` | `perf.Point` |
| `range` | string |
| `series` | 配列 of `perf.Point` |
| `step` | integer |

### perf.Point

| フィールド | 型 |
| --- | --- |
| `childCpu` | number |
| `childCpuMax` | number |
| `childRss` | number |
| `cpu` | number |
| `cpuMax` | number |
| `disk` | number |
| `goroutines` | number |
| `load1` | number |
| `mem` | number |
| `procCpu` | number |
| `procCpuMax` | number |
| `procFds` | number |
| `procRss` | number |
| `t` | integer |

### perf.ProcSample

| フィールド | 型 |
| --- | --- |
| `cpu` | number |
| `rss` | number |
| `t` | integer |

### perf.ProcessView

| フィールド | 型 |
| --- | --- |
| `cpu` | number |
| `module` | string |
| `name` | string |
| `pid` | integer |
| `program` | string |
| `restarts` | integer |
| `rss` | integer |
| `samples` | 配列 of `perf.ProcSample` |
| `since` | 時刻 (RFC 3339) |
| `threads` | integer |

### perf.Rule

| フィールド | 型 |
| --- | --- |
| `duration` | integer |
| `enable` | boolean |
| `group` | string |
| `key` | string |
| `level` | string |
| `metric` | string |
| `minRequests` | integer |
| `name` | string |
| `op` | string |
| `remark` | string |
| `sites` | 配列 of string |
| `threshold` | number |

### perf.SiteView

| フィールド | 型 |
| --- | --- |
| `active` | integer |
| `err5xx` | number |
| `key` | string |
| `latencies` | integer |
| `name` | string |
| `p95` | number |
| `qps` | number |
| `requests` | integer |
| `upstreams` | 配列 of `perf.UpstreamView` |

### perf.UpstreamView

| フィールド | 型 |
| --- | --- |
| `conns` | integer |
| `healthy` | boolean |
| `key` | string |
| `route` | string |
| `url` | string |

### portforward.Rule

| フィールド | 型 |
| --- | --- |
| `autoFirewall` | boolean |
| `enable` | boolean |
| `group` | string |
| `ipList` | 配列 of string |
| `ipMode` | string |
| `ipSets` | 配列 of string |
| `key` | string |
| `listenAddress` | string |
| `listenPorts` | string |
| `maxConnections` | integer |
| `mode` | string |
| `name` | string |
| `protocols` | 配列 of string |
| `proxyProtocol` | string |
| `remark` | string |
| `targetAddresses` | 配列 of string |
| `targetPorts` | string |
| `udpSessionTimeout` | integer |

### proc.State

| フィールド | 型 |
| --- | --- |
| `error` | string |
| `lastExit` | string |
| `pid` | integer |
| `restarts` | integer |
| `running` | boolean |
| `since` | 時刻 (RFC 3339) |

### protoroute.Route

| フィールド | 型 |
| --- | --- |
| `alpn` | 配列 of string |
| `disabled` | boolean |
| `domains` | 配列 of string |
| `name` | string |
| `protocol` | string |
| `proxyProtocol` | string |
| `targetAddresses` | 配列 of string |
| `targetPort` | integer |

### protoroute.Rule

| フィールド | 型 |
| --- | --- |
| `autoFirewall` | boolean |
| `enable` | boolean |
| `group` | string |
| `ipList` | 配列 of string |
| `ipMode` | string |
| `ipSets` | 配列 of string |
| `key` | string |
| `listenAddress` | string |
| `listenPorts` | string |
| `maxConnections` | integer |
| `name` | string |
| `protocols` | 配列 of string |
| `proxyProtocol` | string |
| `remark` | string |
| `routes` | 配列 of `protoroute.Route` |
| `sniffTimeout` | number |
| `targetAddresses` | 配列 of string |
| `targetPorts` | string |

### ssl.ACMEConfig

| フィールド | 型 |
| --- | --- |
| `ca` | string |
| `challenge` | string |
| `credentials` | `dnsprovider.Credentials` |
| `directoryUrl` | string |
| `disableAuthoritativeCheck` | boolean |
| `disableCname` | boolean |
| `domains` | 配列 of string |
| `eabHmac` | string |
| `eabKid` | string |
| `email` | string |
| `httpPort` | integer |
| `keyType` | string |
| `network` | string |
| `profile` | string |
| `propagationTimeout` | integer |
| `provider` | string |
| `resolvers` | 配列 of string |
| `skipPropagationCheck` | boolean |
| `zone` | string |

### ssl.Cert

| フィールド | 型 |
| --- | --- |
| `acme` | `ssl.ACMEConfig` |
| `certFile` | string |
| `certPem` | string |
| `deploys` | 配列 of `ssl.Deploy` |
| `enable` | boolean |
| `group` | string |
| `issuedProfile` | string |
| `issuerPem` | string |
| `key` | string |
| `keyFile` | string |
| `keyPem` | string |
| `mapping` | `ssl.Mapping` |
| `remark` | string |
| `source` | string |
| `updatedAt` | 時刻 (RFC 3339) |

### ssl.Deploy

| フィールド | 型 |
| --- | --- |
| `certName` | string |
| `command` | string |
| `connection` | string |
| `containers` | 配列 of string |
| `deployedAt` | 時刻 (RFC 3339) |
| `deployedHash` | string |
| `dir` | string |
| `enable` | boolean |
| `keyName` | string |
| `remark` | string |
| `type` | string |

### ssl.Mapping

| フィールド | 型 |
| --- | --- |
| `dir` | string |
| `enable` | boolean |
| `script` | string |

### storage.Mount

| フィールド | 型 |
| --- | --- |
| `domain` | string |
| `enable` | boolean |
| `group` | string |
| `key` | string |
| `name` | string |
| `password` | string |
| `path` | string |
| `readOnly` | boolean |
| `remark` | string |
| `skipVerify` | boolean |
| `type` | string |
| `url` | string |
| `username` | string |

### storage.MountInfo

| フィールド | 型 |
| --- | --- |
| `enable` | boolean |
| `key` | string |
| `name` | string |
| `readOnly` | boolean |
| `type` | string |

### stun.Bind

| フィールド | 型 |
| --- | --- |
| `interface` | string |
| `ip` | string |
| `mode` | string |
| `regex` | string |

### stun.Rule

| フィールド | 型 |
| --- | --- |
| `autoFirewall` | boolean |
| `autoRetry` | boolean |
| `bind` | `stun.Bind` |
| `disableCheck` | boolean |
| `disableForward` | boolean |
| `enable` | boolean |
| `group` | string |
| `heartbeatInterval` | integer |
| `ipList` | 配列 of string |
| `ipMode` | string |
| `ipSets` | 配列 of string |
| `keepAliveServers` | 配列 of string |
| `key` | string |
| `localPort` | integer |
| `maxConnections` | integer |
| `name` | string |
| `natpmp` | boolean |
| `natpmpGateway` | string |
| `protocol` | string |
| `remark` | string |
| `retryInterval` | integer |
| `script` | string |
| `scriptEnable` | boolean |
| `stunServers` | 配列 of string |
| `stunTimeout` | integer |
| `targetAddresses` | 配列 of string |
| `targetPort` | integer |
| `udpSessionTimeout` | integer |
| `upnp` | boolean |
| `upnpControlUrl` | string |
| `upnpGateway` | string |
| `upnpInternalPort` | integer |
| `upnpLocalHost` | string |
| `useGlobalServers` | boolean |
| `useGlobalWebhook` | boolean |
| `webhook` | `webhook.Config` |
| `webhookOnlyOnChange` | boolean |

### tailscale.Instance

| フィールド | 型 |
| --- | --- |
| `acceptDns` | boolean |
| `acceptRoutes` | boolean |
| `advertiseExitNode` | boolean |
| `advertiseRoutes` | 配列 of string |
| `authKey` | string |
| `daemonEnv` | string |
| `daemonExtraArgs` | string |
| `enable` | boolean |
| `extraUpArgs` | string |
| `group` | string |
| `hostname` | string |
| `key` | string |
| `legacyStateDir` | boolean |
| `loginServer` | string |
| `manageDaemon` | boolean |
| `name` | string |
| `port` | integer |
| `remark` | string |
| `socket` | string |
| `socks5` | string |
| `stateDir` | string |
| `userspace` | boolean |

### tailscale.PeerView

| フィールド | 型 |
| --- | --- |
| `active` | boolean |
| `curAddr` | string |
| `dnsName` | string |
| `exitNode` | boolean |
| `exitNodeOption` | boolean |
| `ips` | 配列 of string |
| `lastSeen` | 時刻 (RFC 3339) |
| `name` | string |
| `online` | boolean |
| `os` | string |
| `relay` | string |
| `routes` | 配列 of string |
| `rxBytes` | integer |
| `txBytes` | integer |

### tailscale.Settings

| フィールド | 型 |
| --- | --- |
| `installProxy` | string |
| `tailscalePath` | string |
| `tailscaledPath` | string |

### tailscale.SettingsView

| フィールド | 型 |
| --- | --- |
| `cli` | string |
| `settings` | `tailscale.Settings` |

### tailscale.StatusView

| フィールド | 型 |
| --- | --- |
| `authUrl` | string |
| `backendState` | string |
| `cli` | string |
| `daemon` | `proc.State` |
| `enable` | boolean |
| `error` | string |
| `health` | 配列 of string |
| `key` | string |
| `magicDnsSuffix` | string |
| `name` | string |
| `peers` | 配列 of `tailscale.PeerView` |
| `self` | `tailscale.PeerView` |
| `tailnet` | string |
| `version` | string |

### thirdauth.Provider

| フィールド | 型 |
| --- | --- |
| `clientId` | string |
| `clientSecret` | string |
| `issuer` | string |
| `key` | string |
| `name` | string |
| `proxy` | string |
| `type` | string |

### waf.Instance

| フィールド | 型 |
| --- | --- |
| `bodyLimitAction` | string |
| `bodyLimitMB` | integer |
| `enable` | boolean |
| `group` | string |
| `inboundThreshold` | integer |
| `key` | string |
| `mode` | string |
| `name` | string |
| `outboundThreshold` | integer |
| `paranoia` | integer |
| `remark` | string |
| `ruleSets` | 配列 of `waf.RuleSet` |
| `safeline` | `waf.SafeLine` |
| `skipBodyPaths` | string |
| `webdav` | boolean |

### waf.RuleSet

| フィールド | 型 |
| --- | --- |
| `enable` | boolean |
| `param` | string |
| `remark` | string |
| `type` | string |

### waf.SafeLine

| フィールド | 型 |
| --- | --- |
| `addr` | string |
| `bodyLimitKB` | integer |
| `enable` | boolean |
| `failClosed` | boolean |
| `timeoutMs` | integer |

### webhook.Config

| フィールド | 型 |
| --- | --- |
| `body` | string |
| `enable` | boolean |
| `headers` | 配列 of string |
| `insecureSkipVerify` | boolean |
| `method` | string |
| `proxy` | string |
| `successContent` | 配列 of string |
| `timeout` | integer |
| `url` | string |

### webpage.CheckReport

| フィールド | 型 |
| --- | --- |
| `pageTitle` | string |
| `placeholders` | 配列 of string |
| `removed` | 配列 of string |
| `size` | integer |
| `warnings` | 配列 of string |

### webservice.AuthUser

| フィールド | 型 |
| --- | --- |
| `displayName` | string |
| `email` | string |
| `groups` | 配列 of string |
| `id` | string |
| `name` | string |
| `password` | string |
| `remark` | string |
| `totpSecret` | string |

### webservice.BackendTimeouts

| フィールド | 型 |
| --- | --- |
| `connect` | integer |
| `responseHeader` | integer |
| `tlsHandshake` | integer |

### webservice.BanInfo

| フィールド | 型 |
| --- | --- |
| `key` | string |
| `manual` | boolean |
| `permanent` | boolean |
| `reason` | string |
| `source` | string |
| `until` | 時刻 (RFC 3339) |

### webservice.CacheConfig

| フィールド | 型 |
| --- | --- |
| `enable` | boolean |
| `maxMb` | integer |
| `rules` | 配列 of `webservice.CacheRule` |
| `ttl` | integer |

### webservice.CacheRule

| フィールド | 型 |
| --- | --- |
| `noCache` | boolean |
| `override` | boolean |
| `path` | string |
| `ttl` | integer |

### webservice.CacheStatsView

| フィールド | 型 |
| --- | --- |
| `bytes` | integer |
| `enabled` | boolean |
| `entries` | integer |
| `maxBytes` | integer |

### webservice.ClientTimeouts

| フィールド | 型 |
| --- | --- |
| `readIdle` | integer |
| `writeIdle` | integer |

### webservice.ConnView

| フィールド | 型 |
| --- | --- |
| `down` | integer |
| `durationMs` | integer |
| `end` | 時刻 (RFC 3339) |
| `host` | string |
| `id` | string |
| `ip` | string |
| `method` | string |
| `path` | string |
| `peer` | string |
| `port` | integer |
| `proto` | string |
| `result` | string |
| `start` | 時刻 (RFC 3339) |
| `up` | integer |
| `websocket` | boolean |

### webservice.ConnsView

| フィールド | 型 |
| --- | --- |
| `active` | 配列 of `webservice.ConnView` |
| `closed` | integer |
| `history` | 配列 of `webservice.ConnView` |
| `matched` | integer |
| `total` | integer |

### webservice.ErrorPage

| フィールド | 型 |
| --- | --- |
| `file` | string |
| `html` | string |
| `key` | string |
| `name` | string |
| `remark` | string |
| `source` | string |
| `updatedAt` | 時刻 (RFC 3339) |

### webservice.ErrorPageCheckRequest

| フィールド | 型 |
| --- | --- |
| `code` | integer |
| `file` | string |
| `html` | string |
| `source` | string |

### webservice.ErrorPageCheckResult

| フィールド | 型 |
| --- | --- |
| `preview` | string |
| `report` | `webpage.CheckReport` |

### webservice.ErrorPageOptions

| フィールド | 型 |
| --- | --- |
| `codes` | string |
| `intercept` | boolean |
| `page` | string |
| `style` | string |

### webservice.ErrorPageSaved

| フィールド | 型 |
| --- | --- |
| `page` | `webservice.ErrorPageView` |
| `report` | `webpage.CheckReport` |

### webservice.ErrorPageStyle

| フィールド | 型 |
| --- | --- |
| `page` | string |
| `style` | string |

### webservice.ErrorPageView

| フィールド | 型 |
| --- | --- |
| `error` | string |
| `file` | string |
| `key` | string |
| `name` | string |
| `remark` | string |
| `size` | integer |
| `source` | string |
| `updatedAt` | 時刻 (RFC 3339) |
| `usedBy` | 配列 of string |

### webservice.ErrorStylePreview

| フィールド | 型 |
| --- | --- |
| `html` | string |
| `id` | string |

### webservice.ForwardAuth

| フィールド | 型 |
| --- | --- |
| `copyHeaders` | 配列 of string |
| `loginFromLocation` | boolean |
| `loginUrl` | string |
| `url` | string |

### webservice.HealthCheck

| フィールド | 型 |
| --- | --- |
| `enable` | boolean |
| `fails` | integer |
| `interval` | integer |
| `mode` | string |
| `passes` | integer |
| `path` | string |
| `timeout` | integer |

### webservice.IPList

| フィールド | 型 |
| --- | --- |
| `count` | integer |
| `enable` | boolean |
| `key` | string |
| `name` | string |
| `proxy` | string |
| `refreshHours` | integer |
| `remark` | string |
| `updatedAt` | 時刻 (RFC 3339) |
| `url` | string |

### webservice.IdentityHeader

| フィールド | 型 |
| --- | --- |
| `field` | string |
| `header` | string |

### webservice.Listener

| フィールド | 型 |
| --- | --- |
| `acceptProxyProtocol` | boolean |
| `accessPort` | integer |
| `accessScheme` | string |
| `autoFirewall` | boolean |
| `default` | `webservice.Site` |
| `enable` | boolean |
| `http3` | boolean |
| `httpPort` | integer |
| `httpPortMode` | string |
| `key` | string |
| `listenIp` | string |
| `minTls` | string |
| `name` | string |
| `network` | string |
| `plainHttp` | string |
| `port` | integer |
| `remark` | string |
| `sites` | 配列 of `webservice.Site` |
| `tls` | boolean |

### webservice.Location

| フィールド | 型 |
| --- | --- |
| `prefix` | string |
| `requestHeaders` | 配列 of string |
| `targets` | 配列 of string |

### webservice.OIDCClient

| フィールド | 型 |
| --- | --- |
| `allow` | 配列 of string |
| `id` | string |
| `name` | string |
| `public` | boolean |
| `redirectUris` | 配列 of string |
| `remark` | string |
| `secret` | string |

### webservice.OIDCSettings

| フィールド | 型 |
| --- | --- |
| `clients` | 配列 of `webservice.OIDCClient` |
| `enable` | boolean |
| `issuer` | string |
| `signingKey` | string |

### webservice.Panel

| フィールド | 型 |
| --- | --- |
| `enable` | boolean |
| `icon` | string |
| `key` | string |
| `lanCidrs` | 配列 of string |
| `links` | 配列 of `webservice.PanelLink` |
| `name` | string |
| `remark` | string |
| `subtitle` | string |
| `theme` | string |
| `title` | string |

### webservice.PanelAlt

| フィールド | 型 |
| --- | --- |
| `name` | string |
| `url` | string |

### webservice.PanelIconView

| フィールド | 型 |
| --- | --- |
| `cat` | string |
| `color` | string |
| `id` | string |
| `name` | string |
| `src` | string |
| `srcDark` | string |

### webservice.PanelLink

| フィールド | 型 |
| --- | --- |
| `alts` | 配列 of `webservice.PanelAlt` |
| `color` | string |
| `description` | string |
| `group` | string |
| `hide` | boolean |
| `icon` | string |
| `lanUrl` | string |
| `name` | string |
| `sameTab` | boolean |
| `url` | string |

### webservice.PanelPreview

| フィールド | 型 |
| --- | --- |
| `html` | string |

### webservice.PanelPreviewRequest

| フィールド | 型 |
| --- | --- |
| `lan` | boolean |
| `panel` | `webservice.Panel` |

### webservice.PathRule

| フィールド | 型 |
| --- | --- |
| `auth` | string |
| `ipList` | 配列 of string |
| `ipMode` | string |
| `ipSets` | 配列 of string |
| `prefix` | string |

### webservice.Point

| フィールド | 型 |
| --- | --- |
| `bytes` | integer |
| `p50` | number |
| `p95` | number |
| `requests` | integer |
| `s4xx` | integer |
| `s5xx` | integer |
| `t` | integer |

### webservice.Rate

| フィールド | 型 |
| --- | --- |
| `bps` | number |
| `qps` | number |
| `qps60` | number |

### webservice.RateLimit

| フィールド | 型 |
| --- | --- |
| `burst` | integer |
| `enable` | boolean |
| `maxConns` | integer |
| `rate` | number |

### webservice.RatePoint

| フィールド | 型 |
| --- | --- |
| `bytes` | integer |
| `requests` | integer |
| `t` | integer |

### webservice.ReplayBucket

| フィールド | 型 |
| --- | --- |
| `active` | integer |
| `bps` | number |
| `ips` | integer |
| `issue` | boolean |
| `qps` | number |
| `t` | integer |

### webservice.ReplayIndex

| フィールド | 型 |
| --- | --- |
| `bucket` | integer |
| `buckets` | 配列 of `webservice.ReplayBucket` |
| `end` | integer |
| `frames` | integer |
| `from` | integer |
| `limit` | integer |
| `samples` | integer |
| `settings` | `webservice.ReplaySettings` |
| `start` | integer |
| `step` | integer |
| `to` | integer |
| `used` | integer |

### webservice.ReplayMeta

| フィールド | 型 |
| --- | --- |
| `end` | integer |
| `frameEnd` | integer |
| `frameStart` | integer |
| `next` | integer |
| `prev` | integer |
| `start` | integer |
| `step` | integer |

### webservice.ReplaySettings

| フィールド | 型 |
| --- | --- |
| `enable` | boolean |
| `sizeMB` | integer |

### webservice.Rewrite

| フィールド | 型 |
| --- | --- |
| `code` | integer |
| `pattern` | string |
| `redirect` | boolean |
| `to` | string |

### webservice.Settings

| フィールド | 型 |
| --- | --- |
| `authSecret` | string |
| `authUsers` | 配列 of `webservice.AuthUser` |
| `autoBanSeconds` | integer |
| `enable` | boolean |
| `oauthCallbackBase` | string |
| `oauthProviders` | 配列 of `thirdauth.Provider` |
| `oidc` | `webservice.OIDCSettings` |
| `realIPFrom` | 配列 of string |
| `realIPHeaders` | 配列 of string |
| `selfServicePassword` | boolean |
| `ssoDomain` | string |
| `trustedProxies` | 配列 of string |

### webservice.ShareLink

| フィールド | 型 |
| --- | --- |
| `expiresAt` | 時刻 (RFC 3339) |
| `key` | string |
| `maxUses` | integer |
| `name` | string |
| `pathPrefix` | string |
| `token` | string |

### webservice.Site

| フィールド | 型 |
| --- | --- |
| `accessLog` | boolean |
| `authOptional` | boolean |
| `basicAuth` | boolean |
| `cache` | `webservice.CacheConfig` |
| `cleanUrls` | boolean |
| `clientTimeouts` | `webservice.ClientTimeouts` |
| `compress` | boolean |
| `contentType` | string |
| `cors` | boolean |
| `countries` | 配列 of string |
| `countryMode` | string |
| `denyClose` | boolean |
| `disableKeepAlives` | boolean |
| `domains` | 配列 of string |
| `enable` | boolean |
| `errorPage` | `webservice.ErrorPageOptions` |
| `forwardAuth` | `webservice.ForwardAuth` |
| `forwardedHeaders` | boolean |
| `group` | string |
| `health` | `webservice.HealthCheck` |
| `identityHeaders` | 配列 of `webservice.IdentityHeader` |
| `insecureSkipVerify` | boolean |
| `ipList` | 配列 of string |
| `ipMode` | string |
| `ipSets` | 配列 of string |
| `keepPath` | boolean |
| `key` | string |
| `lbPolicy` | string |
| `listDir` | boolean |
| `locations` | 配列 of `webservice.Location` |
| `name` | string |
| `notFoundPage` | string |
| `oauthUsers` | 配列 of string |
| `panel` | string |
| `passAuthCookies` | boolean |
| `rateLimit` | `webservice.RateLimit` |
| `redirectCode` | integer |
| `redirectTo` | string |
| `remark` | string |
| `requestHeaders` | 配列 of string |
| `responseHeaders` | 配列 of string |
| `rewrites` | 配列 of `webservice.Rewrite` |
| `root` | string |
| `rules` | 配列 of `webservice.PathRule` |
| `sendProxyProtocol` | boolean |
| `shareLinks` | 配列 of `webservice.ShareLink` |
| `sharedUsers` | boolean |
| `statusCode` | integer |
| `targets` | 配列 of string |
| `text` | string |
| `timeouts` | `webservice.BackendTimeouts` |
| `type` | string |
| `upstreamSniFromHost` | boolean |
| `useTargetHost` | boolean |
| `userHeader` | string |
| `userRoutes` | 配列 of `webservice.UserRoute` |
| `users` | 配列 of string |
| `waf` | string |
| `webAuth` | boolean |

### webservice.SiteBatchOptions

| フィールド | 型 |
| --- | --- |
| `accessLog` | boolean |
| `compress` | boolean |
| `cors` | boolean |
| `denyClose` | boolean |
| `errorPage` | `webservice.ErrorPageStyle` |
| `forwardedHeaders` | boolean |
| `waf` | string |

### webservice.SiteBatchRequest

| フィールド | 型 |
| --- | --- |
| `action` | string |
| `domains` | 配列 of string |
| `from` | string |
| `move` | `webservice.SiteMoveTarget` |
| `options` | `webservice.SiteBatchOptions` |
| `sites` | 配列 of `webservice.SiteRef` |
| `to` | string |

### webservice.SiteBatchResult

| フィールド | 型 |
| --- | --- |
| `changed` | integer |
| `moved` | 配列 of `webservice.SiteMoved` |
| `unchanged` | integer |

### webservice.SiteMoveTarget

| フィールド | 型 |
| --- | --- |
| `after` | string |
| `before` | string |
| `group` | string |
| `listener` | string |

### webservice.SiteMoved

| フィールド | 型 |
| --- | --- |
| `from` | `webservice.SiteRef` |
| `to` | `webservice.SiteRef` |

### webservice.SiteRef

| フィールド | 型 |
| --- | --- |
| `listener` | string |
| `site` | string |

### webservice.SiteSharedUsers

| フィールド | 型 |
| --- | --- |
| `disableAuth` | boolean |
| `listener` | string |
| `sharedUsers` | boolean |
| `site` | string |

### webservice.SiteStatsView

| フィールド | 型 |
| --- | --- |
| `blocked` | マップ of integer |
| `bytesOut` | integer |
| `series` | 配列 of `webservice.Point` |
| `status` | マップ of integer |
| `topCountries` | 配列 of `webservice.TopItem` |
| `topIps` | 配列 of `webservice.TopItem` |
| `upstreams` | 配列 of `webservice.UpstreamStatus` |

### webservice.TopItem

| フィールド | 型 |
| --- | --- |
| `count` | integer |
| `key` | string |

### webservice.TopoAction

| フィールド | 型 |
| --- | --- |
| `id` | string |
| `kind` | string |
| `rate` | `webservice.Rate` |
| `requests` | integer |

### webservice.TopoIP

| フィールド | 型 |
| --- | --- |
| `active` | integer |
| `activeAt` | マップ of integer |
| `ip` | string |
| `last` | integer |
| `nodes` | 配列 of string |

### webservice.TopoListener

| フィールド | 型 |
| --- | --- |
| `actions` | 配列 of `webservice.TopoAction` |
| `active` | integer |
| `enable` | boolean |
| `error` | string |
| `http3` | boolean |
| `httpPort` | integer |
| `httpPortMode` | string |
| `id` | string |
| `key` | string |
| `listenIp` | string |
| `name` | string |
| `network` | string |
| `plainHttp` | string |
| `port` | integer |
| `proxyProtocol` | boolean |
| `rate` | `webservice.Rate` |
| `requests` | integer |
| `running` | boolean |
| `sites` | 配列 of `webservice.TopoSite` |
| `tls` | boolean |

### webservice.TopoSite

| フィールド | 型 |
| --- | --- |
| `accessLog` | boolean |
| `active` | integer |
| `blocked` | マップ of integer |
| `closed` | integer |
| `default` | boolean |
| `domains` | 配列 of string |
| `enable` | boolean |
| `errors` | integer |
| `group` | string |
| `guards` | 配列 of string |
| `id` | string |
| `key` | string |
| `name` | string |
| `rate` | `webservice.Rate` |
| `requests` | integer |
| `running` | boolean |
| `status` | マップ of integer |
| `target` | string |
| `type` | string |
| `upstreams` | 配列 of `webservice.TopoUpstream` |

### webservice.TopoUpstream

| フィールド | 型 |
| --- | --- |
| `backup` | boolean |
| `bytes` | integer |
| `conns` | integer |
| `healthy` | boolean |
| `id` | string |
| `match` | string |
| `rate` | `webservice.Rate` |
| `requests` | integer |
| `scope` | string |
| `url` | string |
| `weight` | integer |

### webservice.TopologyView

| フィールド | 型 |
| --- | --- |
| `active` | integer |
| `enabled` | boolean |
| `focus` | string |
| `ipTotal` | integer |
| `ipTruncated` | boolean |
| `ips` | 配列 of `webservice.TopoIP` |
| `listeners` | 配列 of `webservice.TopoListener` |
| `rate` | `webservice.Rate` |
| `replay` | `webservice.ReplayMeta` |
| `requests` | integer |
| `series` | 配列 of `webservice.RatePoint` |
| `seriesStep` | integer |
| `time` | integer |
| `window` | integer |

### webservice.UpstreamStatus

| フィールド | 型 |
| --- | --- |
| `backup` | boolean |
| `bytes` | integer |
| `conns` | integer |
| `healthy` | boolean |
| `match` | string |
| `requests` | integer |
| `scope` | string |
| `url` | string |
| `weight` | integer |

### webservice.UserRoute

| フィールド | 型 |
| --- | --- |
| `targets` | 配列 of string |
| `users` | 配列 of string |

### webservice.UserSite

| フィールド | 型 |
| --- | --- |
| `basicAuth` | boolean |
| `default` | boolean |
| `domains` | 配列 of string |
| `enable` | boolean |
| `forwardAuth` | boolean |
| `group` | string |
| `listener` | string |
| `listenerEnable` | boolean |
| `listenerName` | string |
| `name` | string |
| `otherAuth` | boolean |
| `port` | integer |
| `sharedUsers` | boolean |
| `site` | string |
| `tls` | boolean |
| `type` | string |
| `webAuth` | boolean |

### webservice.UsersRequest

| フィールド | 型 |
| --- | --- |
| `authUsers` | 配列 of `webservice.AuthUser` |
| `oidc` | `webservice.OIDCSettings` |
| `selfServicePassword` | boolean |
| `sites` | 配列 of `webservice.SiteSharedUsers` |
| `ssoDomain` | string |

### webservice.UsersView

| フィールド | 型 |
| --- | --- |
| `authUsers` | 配列 of `webservice.AuthUser` |
| `oidc` | `webservice.OIDCSettings` |
| `selfServicePassword` | boolean |
| `sites` | 配列 of `webservice.UserSite` |
| `ssoDomain` | string |

### webservice.banRequest

| フィールド | 型 |
| --- | --- |
| `key` | string |
| `seconds` | integer |

### webservice.cachePurgeRequest

| フィールド | 型 |
| --- | --- |
| `listener` | string |
| `path` | string |
| `site` | string |

### webservice.cachePurgeResult

| フィールド | 型 |
| --- | --- |
| `bytes` | integer |
| `enabled` | boolean |
| `entries` | integer |
| `maxBytes` | integer |
| `removed` | integer |

### webservice.closeConnRequest

| フィールド | 型 |
| --- | --- |
| `banSeconds` | integer |
| `id` | string |
| `listener` | string |
| `site` | string |

### webservice.closeConnResult

| フィールド | 型 |
| --- | --- |
| `banKey` | string |
| `closed` | integer |

### webservice.geoipDownload

| フィールド | 型 |
| --- | --- |
| `proxy` | string |
| `url` | string |

### webservice.totpRequest

| フィールド | 型 |
| --- | --- |
| `name` | string |

### webservice.totpResponse

| フィールド | 型 |
| --- | --- |
| `secret` | string |
| `url` | string |

### webterminal.Connection

| フィールド | 型 |
| --- | --- |
| `enable` | boolean |
| `group` | string |
| `key` | string |
| `local` | `webterminal.LocalConfig` |
| `name` | string |
| `quickAccessDirs` | 配列 of string |
| `remark` | string |
| `ssh` | `webterminal.SSHConfig` |
| `telnet` | `webterminal.TelnetConfig` |
| `type` | string |

### webterminal.Credential

| フィールド | 型 |
| --- | --- |
| `authType` | string |
| `enable` | boolean |
| `key` | string |
| `keyFile` | string |
| `name` | string |
| `passphrase` | string |
| `password` | string |
| `privateKey` | string |
| `remark` | string |
| `username` | string |

### webterminal.LocalConfig

| フィールド | 型 |
| --- | --- |
| `env` | マップ of string |
| `shell` | string |
| `workDir` | string |

### webterminal.Proxy

| フィールド | 型 |
| --- | --- |
| `enabled` | boolean |
| `host` | string |
| `password` | string |
| `port` | integer |
| `type` | string |
| `username` | string |

### webterminal.SSHConfig

| フィールド | 型 |
| --- | --- |
| `authType` | string |
| `credentialKey` | string |
| `host` | string |
| `hostKey` | string |
| `hostKeyType` | string |
| `keepAlive` | integer |
| `keyFile` | string |
| `passphrase` | string |
| `password` | string |
| `port` | integer |
| `privateKey` | string |
| `proxy` | `webterminal.Proxy` |
| `timeout` | integer |
| `username` | string |
| `workDir` | string |

### webterminal.TelnetConfig

| フィールド | 型 |
| --- | --- |
| `host` | string |
| `password` | string |
| `port` | integer |
| `proxy` | `webterminal.Proxy` |
| `timeout` | integer |
| `username` | string |

### wol.BemfaSettings

| フィールド | 型 |
| --- | --- |
| `enable` | boolean |
| `uid` | string |

### wol.BemfaStatus

| フィールド | 型 |
| --- | --- |
| `connected` | boolean |
| `error` | string |
| `since` | integer |
| `topics` | integer |

### wol.Device

| フィールド | 型 |
| --- | --- |
| `bemfaTopic` | string |
| `broadcastIps` | 配列 of string |
| `checkAddr` | string |
| `enable` | boolean |
| `group` | string |
| `key` | string |
| `macs` | 配列 of string |
| `name` | string |
| `port` | integer |
| `remark` | string |
| `repeat` | integer |
| `shutdownCommand` | string |
| `shutdownConn` | string |

### wol.SSHTarget

| フィールド | 型 |
| --- | --- |
| `host` | string |
| `key` | string |
| `name` | string |
| `trusted` | boolean |

### wol.bemfaView

| フィールド | 型 |
| --- | --- |
| `settings` | `wol.BemfaSettings` |
| `status` | `wol.BemfaStatus` |

