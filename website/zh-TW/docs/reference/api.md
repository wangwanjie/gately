# API 權杖與 OpenAPI

Gately 管理介面使用的全部介面都可以透過 HTTP 呼叫。指令碼與自動化使用 API 權杖驗證，介面說明由程式依已登錄的路由自動產生。

## API 權杖

管理介面路徑：側邊欄「系統」分組 → 「API 權杖」。

### 建立權杖

點擊「新增權杖」，填寫：

| 項目 | 說明 |
| --- | --- |
| 名稱 | 必填，最長 64 個字元，用於標識用途，如「監控指令碼」 |
| 到期時間 | 可選「永不到期」、7 天、30 天、90 天、1 年或自訂日期；自訂時間必須晚於目前時間 |
| 權限 | 必填，至少選擇一項。新增時預設不授予任何權限 |

單一實例最多儲存 100 個權杖。

建立成功後彈窗只顯示一次權杖明文，格式為 `gtl_` 加 48 位十六進位字元。磁碟上只儲存 SHA-256 雜湊，遺失後只能「重新產生」，舊值立即失效。清單中以 `gtl_xxxx…yyyy` 形式的提示辨認權杖，並顯示最近使用時間與來源 IP（寫入磁碟最多延遲一分鐘）。

編輯權杖可以修改名稱、權限、到期時間和啟用開關，修改立即生效，權杖明文保持不變。已到期的權杖可以保留原到期時間並只修改其他欄位。

### 驗證方式

請求標頭攜帶 `Authorization: Bearer <權杖>`：

```bash
curl -H "Authorization: Bearer gtl_xxxx" http://localhost:16680/api/status
```

請求本文使用 `Content-Type: application/json`，錯誤統一回傳 `{"error": "說明"}`。權杖被停用、已到期或不存在時，請求會繼續依登入工作階段驗證，最終回傳未登入錯誤。

### 權限範圍

權限格式為 `<權限域>:read` 或 `<權限域>:write`，寫入權限包含讀取權限。權限域是功能模組的 ID（如 `ddns`、`portforward`、`notify`、`backup`）加上五個內建域：

| 權限域 | 名稱 | 可用權限 | 對應介面 |
| --- | --- | --- | --- |
| `status` | 系統狀態 | `status:read` | `/api/status`、`/api/system/info`、`/api/system/update` |
| `logs` | 系統日誌 | `logs:read` | `/api/logs` |
| `import` | Lucky 設定匯入 | `import:write` | `/api/import/lucky` |
| `fileusers` | 檔案服務共用使用者 | `fileusers:read`、`fileusers:write` | `/api/fileusers` |
| `metrics` | 全域指標（Prometheus） | `metrics:read` | `/api/metrics` |

萬用寫法：

| 寫法 | 含義 |
| --- | --- |
| `*:read` | 全部模組唯讀 |
| `*:write` 或 `*` | 全部模組讀寫 |
| `ddns:read` | 僅 DDNS 唯讀 |

介面所需權限由路徑和方法決定：

- 路徑 `/api/<模組>/...` 需要 `<模組>` 域的權限，GET 與 HEAD 要求 `read`，其餘方法要求 `write`。
- 模組日誌 `GET /api/<模組>/logs` 屬於唯讀，清空日誌（DELETE）屬於寫入操作。
- 下列 GET 介面會回傳敏感內容或建立互動通道，按 `write` 處理：路徑中含有 `ws`（WebSocket 終端機）、`exec`、`download`、`export`、`read`、`config`、`preview-archive`、`files`、`sftp`、`secrets` 段的介面，例如憑證下載、容器內執行命令與檔案讀取。被遮蔽的密碼、金鑰可透過對應資源的 `.../secrets` 介面取回明文，需要寫入權限，唯讀帳號和唯讀權杖呼叫時回傳 403。容器詳情 `/api/docker/containers/{id}` 只需 `docker:read`，其中的環境變數值與啟動指令在沒有 `docker:write` 時遮蔽，明文透過 `/api/docker/containers/{id}/secrets` 取得。
- 舊介面 `PUT /api/webdav/shared-users`、`PUT /api/ftp/shared-users`、`PUT /api/smb/shared-users` 修改的是三個服務共用的同一份共用使用者，需要同時具備 `webdav:write`、`ftp:write`、`smb:write`；新介面 `/api/fileusers` 使用 `fileusers` 域的權限。
- `GET /api/session` 與 `GET /api/openapi.json` 接受任意有效權杖。
- 以下模組的寫入權限可以在伺服器上執行命令，等同於 Gately 程序使用者的命令執行權限：`ddns`（命令取 IP）、`cron`（排程任務）、`guard`（命令日誌來源）、`stun`（規則指令碼）、`ssl`（對應指令碼與本機部署命令）、`webterminal`、`docker`（容器內執行與 Compose）、`frp`、`filebrowser`、`cloudflared`（可指定程式路徑）。新增令牌時這些模組帶有「可執行命令」標記，只授予完全信任的指令碼。令牌呼叫 Lucky 匯入（`import:write`）時項目一律匯入為停用狀態，`enable=1` 只對登入工作階段生效。

### 權杖禁止的操作

以下介面只接受登入工作階段，權杖存取一律回傳 403：

- 帳號與兩步驟驗證：`/api/account`
- 權杖管理：`/api/tokens`
- 稽核日誌與設定歷史：`/api/audit`、`/api/history`
- 系統設定、系統權杖、重新啟動、線上升級：`/api/system/settings`、`/api/system/token`、`/api/system/restart`、`/api/system/upgrade`
- 診斷包、自我檢查與初始引導：`/api/system/diagnostics`、`/api/system/selfcheck`、`/api/system/onboarding`
- 設定備份下載與還原：`/api/system/backup`、`/api/system/restore`
- 本機備份檔案下載：`/api/backup/local`
- Web 終端機安全設定：`/api/webterminal/security`
- 登入與登出：`/api/login`、`/api/logout`

權限不足的請求會回傳 `權杖缺少權限 <權限>`，並在系統日誌中記錄一則警告，包含權杖名稱、來源 IP 與被拒絕的介面。

### 舊版權杖

舊版本的單一全域權杖在升級時自動遷移為名為「舊版權杖（由舊版本遷移）」的權杖，權限為 `*`，清單中帶有「舊版」標記。建議建立依需求授權的新權杖後刪除它；對舊版權杖「重新產生」會同時去掉舊版標記。

::: tip 建議
為每個指令碼建立單獨的權杖，只勾選它需要的權限，並設定到期時間。權杖外洩時刪除或重新產生對應的那一個即可。
:::

## OpenAPI 與介面文件

### 管理介面裡的文件頁

側邊欄「系統」分組 → 「API 文件」。頁面顯示 Base URL、驗證說明，依功能分組列出全部介面，支援依路徑或摘要搜尋。展開一個介面可以看到所需權限、參數、請求本文欄位和可複製的 curl 範例。需要登入工作階段的介面標記為「僅登入工作階段」，不提供 curl 範例。

### 取得 OpenAPI 文件

實例執行時，`GET /api/openapi.json` 回傳 OpenAPI 3.0 文件，任意有效權杖或登入工作階段均可讀取：

```bash
curl -H "Authorization: Bearer gtl_xxxx" http://localhost:16680/api/openapi.json -o openapi.json
```

文件使用 `bearerAuth` 安全方案，並為每個介面增加擴充欄位：

| 擴充欄位 | 含義 |
| --- | --- |
| `x-required-scope` | 存取該介面所需的權杖權限 |
| `x-token-allowed` | 權杖是否允許存取，`false` 表示僅登入工作階段 |
| `x-scope-domains`（文件根） | 全部權限域清單 |

[命令列與 MCP](/zh-TW/docs/guide/cli-mcp) 的介面清單讀取自這份文件。

### 命令列匯出

`-api-docs` 參數在臨時設定目錄中建置應用程式（不監聽連接埠、不影響現有設定），把文件寫入指定目錄後結束：

```bash
gately -api-docs ./api-docs
```

輸出兩個檔案：

- `openapi.json`：OpenAPI 3.0 描述
- `API.md`：Markdown 版介面文件，含快速開始、權限域表和依分組列出的介面表

## 呼叫範例

```bash
# 檢視系統狀態（需要 status:read）
curl -H "Authorization: Bearer gtl_xxxx" http://localhost:16680/api/status

# 列出全部 DDNS 工作（需要 ddns:read）
curl -H "Authorization: Bearer gtl_xxxx" http://localhost:16680/api/ddns/tasks

# 立即同步一個 DDNS 工作（需要 ddns:write，key 取自上一步回傳的清單）
curl -X POST -H "Authorization: Bearer gtl_xxxx" http://localhost:16680/api/ddns/tasks/KEY/sync

# 只拉取序號大於 10 的模組日誌（需要 ddns:read）
curl -H "Authorization: Bearer gtl_xxxx" "http://localhost:16680/api/ddns/logs?since=10"

# 只取最新 200 筆；再用其中最小的序號作為 before 往前翻頁
curl -H "Authorization: Bearer gtl_xxxx" "http://localhost:16680/api/ddns/logs?limit=200"
curl -H "Authorization: Bearer gtl_xxxx" "http://localhost:16680/api/ddns/logs?before=1200&limit=200"
```

## 回應語言

介面依請求標頭 `Accept-Language` 的語言回應，沒有該標頭時使用伺服器語言（系統設定中的介面語言）。翻譯只作用於程式產生的文字：錯誤回應、狀態與日誌、服務商與權限說明，以及設定資料中的 `error`、`lastError`、`details`、`warnings`、`message`、`title` 等說明欄位。名稱、備註、分組、命令、網域、記錄值等使用者填寫的內容一律原樣傳回，讀出後原樣儲存不會被改寫。腳本需要中文提示時可加 `-H "Accept-Language: zh-CN"`。

## 完整介面清單

全部介面、參數與所需權限請見 [API 介面清單](/en/docs/reference/api-reference)。
