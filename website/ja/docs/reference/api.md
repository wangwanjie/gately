# API トークンと OpenAPI

Gately の管理画面が使うすべての API は、HTTP で呼び出せます。スクリプトや自動化には API トークンで認証し、API の説明は登録済みのルートからプログラムが自動生成します。

## API トークン

管理画面での場所：サイドバーの「システム」グループ → 「API トークン」。

### トークンの作成

「トークンを作成」を押して、次の項目を入力します。

| 項目 | 説明 |
| --- | --- |
| 名前 | 必須。最大 64 文字で、用途の識別に使います。例：「監視スクリプト」 |
| 有効期限 | 「無期限」、7 日、30 日、90 日、1 年、または日付の指定から選べます。日付を指定する場合は、現在より後である必要があります |
| 権限 | 必須で、少なくとも 1 つ選びます。新規作成時はデフォルトでは何も付与されません |

1 つのインスタンスに保存できるトークンは最大 100 個です。

作成に成功すると、ダイアログにトークンの平文が 1 度だけ表示されます。形式は `gtl_` に 16 進数 48 文字が続くものです。ディスクには SHA-256 ハッシュのみが保存されるため、紛失した場合は「再生成」するしかなく、古い値はすぐに無効になります。一覧では `gtl_xxxx…yyyy` の形式のヒントでトークンを見分けられ、最終使用日時と接続元 IP も表示されます（ディスクへの反映は最大 1 分遅れます）。

トークンを編集すると、名前、権限、有効期限、有効化スイッチを変更できます。変更はすぐに反映され、トークンの平文は変わりません。期限切れのトークンでは、元の有効期限のまま、それ以外のフィールドだけを変更できます。

### 認証方法

リクエストヘッダーに `Authorization: Bearer <トークン>` を付けます。

```bash
curl -H "Authorization: Bearer gtl_xxxx" http://localhost:16680/api/status
```

リクエストボディには `Content-Type: application/json` を使い、エラーは一律に `{"error": "説明"}` で返されます。トークンが無効化されている、期限切れ、または存在しない場合は、リクエストはそのままログインセッションとして検証され、最終的に未ログインのエラーが返されます。

### 権限のスコープ

権限の形式は `<スコープ>:read` または `<スコープ>:write` で、write には read が含まれます。スコープは機能モジュールの ID（`ddns`、`portforward`、`notify`、`backup` など）に、組み込みの 5 つのスコープを加えたものです。

| スコープ | 名前 | 利用できる権限 | 対応する API |
| --- | --- | --- | --- |
| `status` | システム状態 | `status:read` | `/api/status`、`/api/system/info`、`/api/system/update` |
| `logs` | システムログ | `logs:read` | `/api/logs` |
| `import` | Lucky 設定のインポート | `import:write` | `/api/import/lucky` |
| `fileusers` | ファイルサービスの共有ユーザー | `fileusers:read`、`fileusers:write` | `/api/fileusers` |
| `metrics` | グローバルメトリクス（Prometheus） | `metrics:read` | `/api/metrics` |

ワイルドカードの書き方：

| 書き方 | 意味 |
| --- | --- |
| `*:read` | すべてのモジュールを読み取り専用 |
| `*:write` または `*` | すべてのモジュールを読み書き可能 |
| `ddns:read` | DDNS のみ読み取り専用 |

API に必要な権限は、パスとメソッドで決まります。

- パス `/api/<モジュール>/...` には `<モジュール>` スコープの権限が必要で、GET と HEAD は `read`、それ以外のメソッドは `write` が必要です。
- モジュールログの `GET /api/<モジュール>/logs` は読み取り専用で、ログの消去（DELETE）は書き込み操作です。
- 次の GET API は、機密性の高い内容を返すか、対話的なチャネルを確立するため、`write` として扱われます。パスに `ws`（WebSocket ターミナル）、`exec`、`download`、`export`、`read`、`config`、`preview-archive`、`files`、`sftp`、`secrets` のセグメントを含む API で、たとえば証明書のダウンロード、コンテナ内でのコマンド実行、ファイルの読み取りが該当します。マスクされたパスワードやキーは、対応するリソースの `.../secrets` API で平文を取得できます。これには書き込み権限が必要で、読み取り専用アカウントと読み取り専用トークンでは 403 が返ります。コンテナ詳細 `/api/docker/containers/{id}` は `docker:read` だけで取得できますが、`docker:write` がない場合は環境変数の値と起動コマンドがマスクされ、平文は `/api/docker/containers/{id}/secrets` で取得します。
- 旧 API の `PUT /api/webdav/shared-users`、`PUT /api/ftp/shared-users`、`PUT /api/smb/shared-users` は 3 つのサービスで共通の共有ユーザーを変更するため、`webdav:write`、`ftp:write`、`smb:write` をすべて必要とします。新しい `/api/fileusers` は `fileusers` スコープの権限を使います。
- `GET /api/session` と `GET /api/openapi.json` は、有効なトークンであれば何でも受け付けます。
- 次のモジュールの書き込み権限ではサーバー上でコマンドを実行でき、Gately プロセスのユーザーとしてのコマンド実行権限と同等です：`ddns`（コマンドによる IP 取得）、`cron`（定期タスク）、`guard`（コマンドのログソース）、`stun`（ルールのスクリプト）、`ssl`（マッピングスクリプトとローカルデプロイコマンド）、`webterminal`、`docker`（コンテナ内実行と Compose）、`frp`、`filebrowser`、`cloudflared`（プログラムのパスを指定可能）。トークン作成時にこれらのモジュールには「コマンド実行可」の印が付きます。完全に信頼できるスクリプトにだけ付与してください。トークンで Lucky インポート（`import:write`）を呼び出した場合、項目は常に無効の状態でインポートされ、`enable=1` はログインセッションでのみ有効です。

### トークンでは許可されない操作

次の API はログインセッションのみを受け付け、トークンでアクセスすると常に 403 が返ります。

- アカウントと 2 段階認証：`/api/account`
- トークン管理：`/api/tokens`
- 監査ログと設定履歴：`/api/audit`、`/api/history`
- システム設定、システムトークン、再起動、オンラインアップグレード：`/api/system/settings`、`/api/system/token`、`/api/system/restart`、`/api/system/upgrade`
- 診断パッケージ、セルフチェック、初期セットアップ：`/api/system/diagnostics`、`/api/system/selfcheck`、`/api/system/onboarding`
- 設定バックアップのダウンロードと復元：`/api/system/backup`、`/api/system/restore`
- ローカルのバックアップファイルのダウンロード：`/api/backup/local`
- Web ターミナルのセキュリティ設定：`/api/webterminal/security`
- ログインとログアウト：`/api/login`、`/api/logout`

権限が足りないリクエストには `トークンに権限 <権限> がありません` が返され、システムログにトークン名、接続元 IP、拒否された API を含む警告が 1 件記録されます。

### レガシートークン

旧バージョンの単一のグローバルトークンは、アップグレード時に「レガシートークン（旧バージョンから移行）」という名前のトークンに自動で移行され、権限は `*` で、一覧に「旧式」のマークが付きます。必要な権限だけを付与した新しいトークンを作成してから、これを削除することをおすすめします。レガシートークンを「再生成」すると、レガシーのマークも同時に外れます。

::: tip おすすめ
スクリプトごとに別のトークンを作成し、必要な権限だけにチェックを入れ、有効期限を設定してください。トークンが漏えいした場合は、該当する 1 つを削除するか再生成するだけで済みます。
:::

## OpenAPI と API ドキュメント

### 管理画面のドキュメントページ

サイドバーの「システム」グループ → 「API ドキュメント」。ページには、Base URL と認証の説明が表示され、すべての API が機能別にグループ化されて一覧表示され、パスまたは概要で検索できます。API を展開すると、必要な権限、パラメーター、リクエストボディのフィールド、コピーできる curl の例を確認できます。ログインセッションが必要な API には「ログインセッション専用」と表示され、curl の例は表示されません。

### OpenAPI ドキュメントの取得

インスタンスの実行中は、`GET /api/openapi.json` が OpenAPI 3.0 ドキュメントを返し、有効なトークンまたはログインセッションであれば読み取れます。

```bash
curl -H "Authorization: Bearer gtl_xxxx" http://localhost:16680/api/openapi.json -o openapi.json
```

ドキュメントは `bearerAuth` セキュリティスキームを使い、各 API に次の拡張フィールドが追加されます。

| 拡張フィールド | 意味 |
| --- | --- |
| `x-required-scope` | その API へのアクセスに必要なトークンの権限 |
| `x-token-allowed` | トークンでのアクセスを許可するか。`false` はログインセッション専用を意味します |
| `x-scope-domains`（ドキュメントのルート） | すべての権限スコープの一覧 |

[コマンドラインと MCP](/ja/docs/guide/cli-mcp) のエンドポイント一覧は、このドキュメントから読み込まれます。

### コマンドラインからのエクスポート

`-api-docs` パラメーターは、一時的な設定ディレクトリでアプリケーションを構築し（ポートは待ち受けず、既存の設定にも影響しません）、ドキュメントを指定ディレクトリに書き出して終了します。

```bash
gately -api-docs ./api-docs
```

2 つのファイルが出力されます。

- `openapi.json`：OpenAPI 3.0 の記述
- `API.md`：Markdown 版の API ドキュメント。クイックスタート、権限スコープの表、グループ別の API 一覧表を含みます

## 呼び出しの例

```bash
# システム状態を確認（status:read が必要）
curl -H "Authorization: Bearer gtl_xxxx" http://localhost:16680/api/status

# すべての DDNS タスクを一覧表示（ddns:read が必要）
curl -H "Authorization: Bearer gtl_xxxx" http://localhost:16680/api/ddns/tasks

# DDNS タスクをすぐに同期（ddns:write が必要。key は前の手順で返された一覧から取得）
curl -X POST -H "Authorization: Bearer gtl_xxxx" http://localhost:16680/api/ddns/tasks/KEY/sync

# シーケンス番号が 10 より大きいモジュールログだけを取得（ddns:read が必要）
curl -H "Authorization: Bearer gtl_xxxx" "http://localhost:16680/api/ddns/logs?since=10"

# 最新の 200 件だけを取得し、その最小の番号を before に渡すとさらに前のページを取得
curl -H "Authorization: Bearer gtl_xxxx" "http://localhost:16680/api/ddns/logs?limit=200"
curl -H "Authorization: Bearer gtl_xxxx" "http://localhost:16680/api/ddns/logs?before=1200&limit=200"
```

## レスポンスの言語

レスポンスはリクエストヘッダー `Accept-Language` の言語で返り、ヘッダーがない場合はサーバーの言語（システム設定の表示言語）を使います。翻訳されるのは Gately が生成したテキストだけです：エラーレスポンス、状態とログ、プロバイダーと権限の説明、設定データ内の `error`、`lastError`、`details`、`warnings`、`message`、`title` などの説明フィールド。名前、備考、グループ、コマンド、ドメイン、レコード値など入力した内容は常に保存されたまま返るため、読み出してそのまま保存しても書き換わりません。スクリプトで中国語のメッセージが必要なら `-H "Accept-Language: zh-CN"` を付けてください。

## 完全な API 一覧

すべての API、パラメーター、必要な権限は [API エンドポイント一覧](/en/docs/reference/api-reference) を参照してください。
