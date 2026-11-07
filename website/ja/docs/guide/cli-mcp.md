# コマンドラインと MCP

`gately` の実行ファイルには 2 つのサブコマンドが組み込まれています。`gately ctl` はターミナルから稼働中のインスタンスの管理 API を呼び出し、`gately mcp` は同じ API を MCP ツールとして AI クライアントに提供します。どちらも HTTP と [API トークン](/ja/docs/reference/api) でインスタンスにアクセスします。エンドポイント一覧はインスタンスの `/api/openapi.json` から取得され、権限はトークンによって決まります。

このほか `gately reset-password` と `gately reset-totp` は API を経由せずローカルの設定ディレクトリを直接操作し、パスワードを忘れたときや認証アプリを紛失したときに管理者アカウントを復旧します。[パスワードと 2 段階認証のリセット](/ja/docs/guide/install#reset) を参照してください。

`gately service install|uninstall|start|stop|restart|status` で Gately を起動時に自動起動するシステムサービスとして登録できます。[自動起動](/ja/docs/guide/install#service) を参照してください。

## 接続設定

接続情報は次の順序で決まり、後のものが前のものを上書きします。

1. デフォルトのアドレス `http://127.0.0.1:16680`
2. 設定ファイル `ctl.json`
3. 環境変数 `GATELY_URL`、`GATELY_TOKEN`
4. コマンドラインオプション `-url`、`-token`

設定ファイルはシステムのユーザー設定ディレクトリ配下の `gately/ctl.json` にあります（Linux では `~/.config/gately/ctl.json`、macOS では `~/Library/Application Support/gately/ctl.json`）。内容の例：

```json
{
  "url": "http://127.0.0.1:16680",
  "token": "gtl_xxxx",
  "insecure": false
}
```

このファイルは `gately ctl login` で生成されます。

## gately ctl

### サブコマンド

| コマンド | 説明 |
| --- | --- |
| `gately ctl ls [キーワード]` | トークンでアクセスできるエンドポイントを一覧表示します。メソッド、パス、概要を出力し、キーワードで絞り込めます |
| `gately ctl get <パス>` | GET リクエストを送信します |
| `gately ctl post <パス> [JSON または @ファイル]` | POST リクエストを送信します |
| `gately ctl put <パス> [JSON または @ファイル]` | PUT リクエストを送信します |
| `gately ctl delete <パス> [JSON または @ファイル]` | DELETE リクエストを送信します |
| `gately ctl login <アドレス> <トークン>` | 接続を検証して設定ファイルに保存します |

パスは `/api/` で始まる必要があります。リクエストボディには、JSON リテラル、`@ファイル名`（ファイルから読み込み）、`-`（標準入力から読み込み）のいずれかを指定でき、内容は有効な JSON である必要があります。`patch` メソッドも利用でき、`ls` は `list` とも書けます。

### 接続オプション

オプションはサブコマンドの前に書きます。

| オプション | 説明 |
| --- | --- |
| `-url <アドレス>` | 管理アドレス。http または https で始まります |
| `-token <トークン>` | API トークン |
| `-insecure` | HTTPS 証明書の検証をスキップします。自己署名証明書を使う LAN 内のインスタンス向けです |
| `-raw` | レスポンスをそのまま出力します。デフォルトでは JSON を整形します |

コマンドが成功すると終了コード 0 を返します。API が 2xx 以外を返した場合は `エラー: HTTP ステータスコード: 説明` を出力して 1 で終了し、引数の誤りは 2 で終了します。

### 例

```bash
# 接続設定を保存（先に /api/session へリクエストしてトークンを検証します）
gately ctl login http://192.168.1.2:16680 gtl_xxxx

# トークンでアクセスできるエンドポイントを表示し、ddns を含む行に絞り込む
gately ctl ls ddns

# システムの状態を取得
gately ctl get /api/status

# デバイスを起動（Wake-on-LAN）
gately ctl post /api/wol/devices/abc123/wake

# DDNS タスクを無効化
gately ctl put /api/ddns/tasks/abc/enable '{"enable":false}'

# ファイルからリクエストボディを読み込む
gately ctl post /api/ddns/tasks @task.json

# 環境変数を一時的に使用
GATELY_URL=https://gw.example.com GATELY_TOKEN=gtl_xxxx gately ctl get /api/status
```

::: tip ヒント
`ls` が表示するのはトークンでアクセスできるエンドポイントです。ログインセッションでしか使えないエンドポイント（アカウント、トークン管理、バックアップと復元など）は表示されません。書き込み操作が許可されるかどうかはトークンの権限で決まり、権限が足りない場合は 403 が返ります。
:::

## gately mcp

`gately mcp` は標準入出力で MCP サービス（JSON-RPC、1 行に 1 メッセージ）を提供し、AI クライアントがサブプロセスとして起動します。

### オプション

| オプション | 説明 |
| --- | --- |
| `--url <アドレス>` | 管理アドレス |
| `--token <トークン>` | API トークン |
| `--write` | 書き込み系のツールも公開します |
| `--insecure` | HTTPS 証明書の検証をスキップします |

接続情報の取得元は `gately ctl` と同じです（設定ファイル、`GATELY_URL`、`GATELY_TOKEN`、オプション）。

### ツールの生成ルール

- サービス起動後、クライアントが最初にツール一覧を要求した時点でインスタンスの OpenAPI ドキュメントを読み込み、トークンでアクセスできるエンドポイントごとに 1 つのツールを生成します。
- ツール名はメソッドとパスから `get_ddns_tasks`、`put_ddns_tasks_key_enable` のように組み立てられ、最大 64 文字で、重複する場合は連番が付きます。
- パスパラメーターは必須の文字列パラメーターになります。GET ツールにはオプションの `query` パラメーター（例：`since=10`）が付き、リクエストボディを持つエンドポイントには `body` パラメーターが付きます。構造は OpenAPI から取得します。
- ツールの説明には、エンドポイントの概要、メソッドとパス、必要な権限が含まれます。ツールには `readOnlyHint` が付き、DELETE ツールには `destructiveHint` が付きます。
- 返却内容が 200000 文字を超える場合は切り詰められます。

### デフォルトは読み取り専用

`--write` を付けない場合は、GET と HEAD のツールだけを公開し、`.../secrets` のように平文のパスワードを返す API も含めません。これらは `--write` を付けたときに提供されます。AI に設定を変更させたいときは `--write` を付けてください。書き込みを有効にしても、トークン自体の権限は引き続き適用されます。そのため、MCP 専用のトークンを作り、必要な権限だけを付与する運用が安全です。

::: warning 注意
`--write` を付けると、AI クライアントはトークンの権限範囲内のすべての書き込みエンドポイントを呼び出せます。削除も含まれます。まず読み取り専用のトークンで流れを確認してから、必要に応じて書き込み権限を付与してください。
:::

### クライアントの設定

Claude Desktop や Cursor など、`mcpServers` 形式の設定を使うクライアントの場合：

```json
{
  "mcpServers": {
    "gately": {
      "command": "gately",
      "args": ["mcp"],
      "env": {
        "GATELY_URL": "http://127.0.0.1:16680",
        "GATELY_TOKEN": "gtl_xxxx"
      }
    }
  }
}
```

書き込みを有効にする場合は、`args` を `["mcp", "--write"]` に変更します。`command` には `gately` の絶対パスも指定できます。

Claude Code ではコマンドラインで追加します。

```bash
claude mcp add gately -e GATELY_URL=http://127.0.0.1:16680 -e GATELY_TOKEN=gtl_xxxx -- gately mcp
```
