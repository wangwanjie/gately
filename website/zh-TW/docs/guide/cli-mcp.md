# 命令列與 MCP

`gately` 執行檔內建兩個子命令：`gately ctl` 在終端機中呼叫執行中實例的管理 API，`gately mcp` 把同一批介面以 MCP 工具的形式提供給 AI 用戶端。兩者都透過 HTTP 與 [API 權杖](/zh-TW/docs/reference/api) 存取實例，介面清單來自實例的 `/api/openapi.json`，權限由權杖決定。

另有 `gately reset-password` 與 `gately reset-totp` 直接讀寫本機設定目錄，不經過 API，用於忘記密碼或遺失兩步驟驗證器時找回管理員帳號，見 [重設密碼與兩步驟驗證](/zh-TW/docs/guide/install#reset)。

`gately service install|uninstall|start|stop|restart|status` 把 Gately 註冊為開機自啟的系統服務，見 [開機自啟](/zh-TW/docs/guide/install#service)。

## 連線設定

連線資訊依以下順序決定，後面的會覆蓋前面的：

1. 預設位址 `http://127.0.0.1:16680`
2. 設定檔 `ctl.json`
3. 環境變數 `GATELY_URL`、`GATELY_TOKEN`
4. 命令列選項 `-url`、`-token`

設定檔位於系統使用者設定目錄下的 `gately/ctl.json`（Linux 為 `~/.config/gately/ctl.json`，macOS 為 `~/Library/Application Support/gately/ctl.json`）。內容範例：

```json
{
  "url": "http://127.0.0.1:16680",
  "token": "gtl_xxxx",
  "insecure": false
}
```

`gately ctl login` 會產生這個檔案。

## gately ctl

### 子命令

| 命令 | 說明 |
| --- | --- |
| `gately ctl ls [關鍵字]` | 列出權杖可存取的介面，輸出方法、路徑與摘要，可依關鍵字過濾 |
| `gately ctl get <路徑>` | 送出 GET 請求 |
| `gately ctl post <路徑> [JSON或@檔案]` | 送出 POST 請求 |
| `gately ctl put <路徑> [JSON或@檔案]` | 送出 PUT 請求 |
| `gately ctl delete <路徑> [JSON或@檔案]` | 送出 DELETE 請求 |
| `gately ctl login <位址> <權杖>` | 驗證連線並儲存到設定檔 |

路徑必須以 `/api/` 開頭。請求本文可以是 JSON 字面值、`@檔名`（讀取檔案）或 `-`（讀取標準輸入），內容必須是合法 JSON。`patch` 方法同樣可用，`ls` 也可寫作 `list`。

### 連線選項

選項寫在子命令之前：

| 選項 | 說明 |
| --- | --- |
| `-url <位址>` | 管理位址，以 http 或 https 開頭 |
| `-token <權杖>` | API 權杖 |
| `-insecure` | 略過 HTTPS 憑證驗證，用於使用自簽憑證的內網實例 |
| `-raw` | 輸出回應原文，預設會格式化 JSON |

命令成功時結束碼為 0，介面回傳非 2xx 時輸出 `錯誤: HTTP 狀態碼: 說明` 並以 1 結束，參數問題以 2 結束。

### 範例

```bash
# 儲存連線設定（會先請求 /api/session 驗證權杖）
gately ctl login http://192.168.1.2:16680 gtl_xxxx

# 檢視權杖可存取的介面，過濾包含 ddns 的行
gately ctl ls ddns

# 讀取系統狀態
gately ctl get /api/status

# 喚醒裝置
gately ctl post /api/wol/devices/abc123/wake

# 停用一個 DDNS 工作
gately ctl put /api/ddns/tasks/abc/enable '{"enable":false}'

# 從檔案讀取請求本文
gately ctl post /api/ddns/tasks @task.json

# 臨時使用環境變數
GATELY_URL=https://gw.example.com GATELY_TOKEN=gtl_xxxx gately ctl get /api/status
```

::: tip 提示
`ls` 列出的是權杖可以存取的介面，僅登入工作階段可用的介面（帳號、權杖管理、備份還原等）不會出現。寫入操作是否放行取決於權杖權限，權限不足時回傳 403。
:::

## gately mcp

`gately mcp` 透過標準輸入輸出提供 MCP 服務（JSON-RPC，每行一則訊息），由 AI 用戶端作為子行程啟動。

### 選項

| 選項 | 說明 |
| --- | --- |
| `--url <位址>` | 管理位址 |
| `--token <權杖>` | API 權杖 |
| `--write` | 同時公開寫入操作工具 |
| `--insecure` | 略過 HTTPS 憑證驗證 |

連線資訊的來源與 `gately ctl` 一致（設定檔、`GATELY_URL`、`GATELY_TOKEN`、選項）。

### 工具產生規則

- 服務啟動後，在用戶端第一次請求工具清單時讀取實例的 OpenAPI 文件，每個權杖可存取的介面產生一個工具。
- 工具名稱由方法與路徑組成，如 `get_ddns_tasks`、`put_ddns_tasks_key_enable`，最長 64 個字元，重名時附加序號。
- 路徑參數成為必填的字串參數；GET 工具帶有選填的 `query` 參數（如 `since=10`）；有請求本文的介面帶有 `body` 參數，結構取自 OpenAPI。
- 工具描述包含介面摘要、方法路徑和所需權限；工具標註 `readOnlyHint`，DELETE 工具標註 `destructiveHint`。
- 回傳內容超過 200000 個字元時截斷。

### 預設唯讀

不帶 `--write` 時只公開 GET 與 HEAD 工具，也不提供 `.../secrets` 這類取回明文密碼的介面，加上 `--write` 後才會提供。需要讓 AI 修改設定時加上 `--write`。即使開啟寫入，權杖自身的權限依然生效，因此更穩妥的做法是為 MCP 建立專用權杖，只授予需要的權限。

::: warning 注意
`--write` 允許 AI 用戶端呼叫權杖權限範圍內的全部寫入介面，包括刪除。請先用唯讀權杖驗證流程，再視需要授予寫入權限。
:::

### 用戶端設定

Claude Desktop、Cursor 等使用 `mcpServers` 設定格式的用戶端：

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

開啟寫入時把 `args` 改為 `["mcp", "--write"]`。`command` 可以寫 `gately` 的絕對路徑。

Claude Code 使用命令列新增：

```bash
claude mcp add gately -e GATELY_URL=http://127.0.0.1:16680 -e GATELY_TOKEN=gtl_xxxx -- gately mcp
```
