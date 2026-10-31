# Gately 管理 API

Gately 管理 API 用於指令碼與自動化批次管理。

驗證：在管理後台的「API 權杖」頁建立權杖，請求標頭攜帶 `Authorization: Bearer gtl_xxxx`。

權限：每個權杖只擁有建立時勾選的權限，格式為 `<模組>:read` 或 `<模組>:write`，寫入權限包含讀取權限。帳號、兩步驟驗證、權杖管理以及備份、還原、重新啟動、系統設定介面只能使用登入工作階段。

請求本文使用 `Content-Type: application/json`，錯誤統一回傳 `{"error": "說明"}`。

在「設定 → 功能模組」中關閉的模組不提供介面，請求其介面回傳 403，執行中執行個體的介面文件也不再列出這些介面。

官網：https://gately.cn

機器可讀的描述見同目錄的 `openapi.json`（OpenAPI 3.0）。

## 快速開始

```bash
# 檢視系統狀態（需要 status:read）
curl -H "Authorization: Bearer gtl_xxxx" http://localhost:16680/api/status

# 列出全部 DDNS 任務（需要 ddns:read）
curl -H "Authorization: Bearer gtl_xxxx" http://localhost:16680/api/ddns/tasks

# 立即同步一個 DDNS 任務（需要 ddns:write，key 取自上一步回傳的清單）
curl -X POST -H "Authorization: Bearer gtl_xxxx" http://localhost:16680/api/ddns/tasks/<key>/sync
```

## 權限

| 權限域 | 名稱 | 可用權限 |
| --- | --- | --- |
| `portforward` | 連接埠轉發 | `portforward:read`、`portforward:write` |
| `protoroute` | 協定分流 | `protoroute:read`、`protoroute:write` |
| `ddns` | 動態網域 | `ddns:read`、`ddns:write` |
| `stun` | STUN 穿透 | `stun:read`、`stun:write` |
| `tailscale` | Tailscale 組網 | `tailscale:read`、`tailscale:write` |
| `frp` | frp 內網穿透 | `frp:read`、`frp:write` |
| `cloudflared` | Cloudflare 通道 | `cloudflared:read`、`cloudflared:write` |
| `ssl` | SSL 憑證 | `ssl:read`、`ssl:write` |
| `waf` | WAF 防火牆 | `waf:read`、`waf:write` |
| `webservice` | Web 服務 | `webservice:read`、`webservice:write` |
| `guard` | 入侵防護 | `guard:read`、`guard:write` |
| `wol` | 網路喚醒 | `wol:read`、`wol:write` |
| `webterminal` | Web 終端機 | `webterminal:read`、`webterminal:write` |
| `cron` | 排程任務 | `cron:read`、`cron:write` |
| `docker` | Docker 管理 | `docker:read`、`docker:write` |
| `webdav` | WebDAV 服務 | `webdav:read`、`webdav:write` |
| `ftp` | FTP 服務 | `ftp:read`、`ftp:write` |
| `smb` | SMB 服務 | `smb:read`、`smb:write` |
| `filebrowser` | File Browser | `filebrowser:read`、`filebrowser:write` |
| `storage` | 儲存掛載 | `storage:read`、`storage:write` |
| `notify` | 通知中心 | `notify:read`、`notify:write` |
| `nettools` | 網路診斷 | `nettools:read`、`nettools:write` |
| `backup` | 設定備份 | `backup:read`、`backup:write` |
| `status` | 系統狀態 | `status:read` |
| `logs` | 系統日誌 | `logs:read` |
| `metrics` | 監控指標（Prometheus） | `metrics:read` |
| `import` | Lucky 設定匯入 | `import:write` |
| `fileusers` | 檔案服務共用使用者 | `fileusers:read`、`fileusers:write` |

萬用寫法：`*:read` 表示全部模組唯讀，`*:write` 或 `*` 表示全部模組讀寫。

## 連接埠轉發

| 方法 | 路徑 | 所需權限 | 說明 |
| --- | --- | --- | --- |
| GET | `/api/portforward/logs` | `portforward:read` | 檢視模組執行日誌 |
| DELETE | `/api/portforward/logs` | `portforward:write` | 清空模組執行日誌 |
| GET | `/api/portforward/rules` | `portforward:read` | 取得連接埠轉發規則清單 |
| POST | `/api/portforward/rules` | `portforward:write` | 新增連接埠轉發規則 |
| PUT | `/api/portforward/rules/{key}` | `portforward:write` | 修改連接埠轉發規則 |
| DELETE | `/api/portforward/rules/{key}` | `portforward:write` | 刪除連接埠轉發規則 |
| PUT | `/api/portforward/rules/{key}/enable` | `portforward:write` | 啟用或停用連接埠轉發規則 |
| PUT | `/api/portforward/rules-order` | `portforward:write` | 調整連接埠轉發規則排序 |
| PUT | `/api/portforward/rules-group` | `portforward:write` | 批次設定連接埠轉發規則分組 |
| GET | `/api/portforward/settings` | `portforward:read` | 讀取連接埠轉發全域設定 |
| PUT | `/api/portforward/settings` | `portforward:write` | 儲存連接埠轉發全域設定 |

## 協定分流

| 方法 | 路徑 | 所需權限 | 說明 |
| --- | --- | --- | --- |
| GET | `/api/protoroute/logs` | `protoroute:read` | 檢視模組執行日誌 |
| DELETE | `/api/protoroute/logs` | `protoroute:write` | 清空模組執行日誌 |
| GET | `/api/protoroute/rules` | `protoroute:read` | 取得協定分流規則清單 |
| POST | `/api/protoroute/rules` | `protoroute:write` | 新增協定分流規則 |
| PUT | `/api/protoroute/rules/{key}` | `protoroute:write` | 修改協定分流規則 |
| DELETE | `/api/protoroute/rules/{key}` | `protoroute:write` | 刪除協定分流規則 |
| PUT | `/api/protoroute/rules/{key}/enable` | `protoroute:write` | 啟用或停用協定分流規則 |
| PUT | `/api/protoroute/rules-order` | `protoroute:write` | 調整協定分流規則排序 |
| PUT | `/api/protoroute/rules-group` | `protoroute:write` | 批次設定協定分流規則分組 |
| GET | `/api/protoroute/settings` | `protoroute:read` | 讀取協定分流全域設定 |
| PUT | `/api/protoroute/settings` | `protoroute:write` | 儲存協定分流全域設定 |

## 動態網域

| 方法 | 路徑 | 所需權限 | 說明 |
| --- | --- | --- | --- |
| GET | `/api/ddns/logs` | `ddns:read` | 檢視模組執行日誌 |
| DELETE | `/api/ddns/logs` | `ddns:write` | 清空模組執行日誌 |
| GET | `/api/ddns/tasks` | `ddns:read` | 取得DDNS 任務清單 |
| POST | `/api/ddns/tasks` | `ddns:write` | 新增DDNS 任務 |
| PUT | `/api/ddns/tasks/{key}` | `ddns:write` | 修改DDNS 任務 |
| DELETE | `/api/ddns/tasks/{key}` | `ddns:write` | 刪除DDNS 任務 |
| PUT | `/api/ddns/tasks/{key}/enable` | `ddns:write` | 啟用或停用DDNS 任務 |
| PUT | `/api/ddns/tasks-order` | `ddns:write` | 調整DDNS 任務排序 |
| PUT | `/api/ddns/tasks-group` | `ddns:write` | 批次設定DDNS 任務分組 |
| GET | `/api/ddns/tasks/{key}/secrets` | `ddns:write` | 查看金鑰明文 |
| POST | `/api/ddns/tasks/{key}/sync` | `ddns:write` | 立即同步一個 DDNS 任務 |
| DELETE | `/api/ddns/tasks/{key}/records/{rec}` | `ddns:write` | 刪除 DDNS 任務中的一條記錄 |
| PUT | `/api/ddns/tasks/{key}/records-layout` | `ddns:write` | 調整任務內解析記錄的順序與分組 |
| GET | `/api/ddns/providers` | `ddns:read` | 列出支援的 DNS 服務商及其所需參數 |
| GET | `/api/ddns/netinterfaces` | `ddns:read` | 列出本機網卡及位址 |
| POST | `/api/ddns/iptest` | `ddns:write` | 測試取得公網 IP |
| GET | `/api/ddns/defaults` | `ddns:read` | 讀取 DDNS 預設值 |
| GET | `/api/ddns/settings` | `ddns:read` | 讀取 DDNS 全域設定 |
| PUT | `/api/ddns/settings` | `ddns:write` | 儲存 DDNS 全域設定 |

## STUN 穿透

| 方法 | 路徑 | 所需權限 | 說明 |
| --- | --- | --- | --- |
| GET | `/api/stun/logs` | `stun:read` | 檢視模組執行日誌 |
| DELETE | `/api/stun/logs` | `stun:write` | 清空模組執行日誌 |
| GET | `/api/stun/rules` | `stun:read` | 取得STUN 規則清單 |
| POST | `/api/stun/rules` | `stun:write` | 新增STUN 規則 |
| PUT | `/api/stun/rules/{key}` | `stun:write` | 修改STUN 規則 |
| DELETE | `/api/stun/rules/{key}` | `stun:write` | 刪除STUN 規則 |
| PUT | `/api/stun/rules/{key}/enable` | `stun:write` | 啟用或停用STUN 規則 |
| PUT | `/api/stun/rules-order` | `stun:write` | 調整STUN 規則排序 |
| PUT | `/api/stun/rules-group` | `stun:write` | 批次設定STUN 規則分組 |
| POST | `/api/stun/rules/{key}/restart` | `stun:write` | 重新啟動一條 STUN 規則 |
| GET | `/api/stun/netinterfaces` | `stun:read` | 列出本機網卡及位址 |
| GET | `/api/stun/settings` | `stun:read` | 讀取 STUN 全域設定 |
| PUT | `/api/stun/settings` | `stun:write` | 儲存 STUN 全域設定 |

## Tailscale 組網

| 方法 | 路徑 | 所需權限 | 說明 |
| --- | --- | --- | --- |
| GET | `/api/tailscale/logs` | `tailscale:read` | 檢視模組執行日誌 |
| DELETE | `/api/tailscale/logs` | `tailscale:write` | 清空模組執行日誌 |
| GET | `/api/tailscale/settings` | `tailscale:read` | 讀取 Tailscale 設定 |
| PUT | `/api/tailscale/settings` | `tailscale:write` | 儲存 Tailscale 設定 |
| GET | `/api/tailscale/secrets` | `tailscale:write` | 查看金鑰明文 |
| GET | `/api/tailscale/status` | `tailscale:read` | 檢視 Tailscale 連線狀態 |
| POST | `/api/tailscale/restart` | `tailscale:write` | 重新啟動 Tailscale |
| POST | `/api/tailscale/logout` | `tailscale:write` | 登出 Tailscale |
| GET | `/api/tailscale/install` | `tailscale:read` | 檢視託管程式的安裝資訊與進度 |
| POST | `/api/tailscale/install` | `tailscale:write` | 線上安裝或更新託管程式 |
| POST | `/api/tailscale/install/rollback` | `tailscale:write` | 切換到保存的舊版本 |
| DELETE | `/api/tailscale/install` | `tailscale:write` | 解除安裝託管程式 |
| DELETE | `/api/tailscale/install/backups/{id}` | `tailscale:write` | 刪除保存的舊版本 |
| POST | `/api/tailscale/reset` | `tailscale:write` | 恢復模組預設設定 |

## frp 內網穿透

| 方法 | 路徑 | 所需權限 | 說明 |
| --- | --- | --- | --- |
| GET | `/api/frp/logs` | `frp:read` | 檢視模組執行日誌 |
| DELETE | `/api/frp/logs` | `frp:write` | 清空模組執行日誌 |
| GET | `/api/frp/clients` | `frp:read` | 取得frp 用戶端清單 |
| POST | `/api/frp/clients` | `frp:write` | 新增frp 用戶端 |
| PUT | `/api/frp/clients/{key}` | `frp:write` | 修改frp 用戶端 |
| DELETE | `/api/frp/clients/{key}` | `frp:write` | 刪除frp 用戶端 |
| PUT | `/api/frp/clients/{key}/enable` | `frp:write` | 啟用或停用frp 用戶端 |
| PUT | `/api/frp/clients-order` | `frp:write` | 調整frp 用戶端排序 |
| PUT | `/api/frp/clients-group` | `frp:write` | 批次設定frp 用戶端分組 |
| GET | `/api/frp/clients/{key}/secrets` | `frp:write` | 查看金鑰明文 |
| GET | `/api/frp/servers` | `frp:read` | 取得frp 伺服器端清單 |
| POST | `/api/frp/servers` | `frp:write` | 新增frp 伺服器端 |
| PUT | `/api/frp/servers/{key}` | `frp:write` | 修改frp 伺服器端 |
| DELETE | `/api/frp/servers/{key}` | `frp:write` | 刪除frp 伺服器端 |
| PUT | `/api/frp/servers/{key}/enable` | `frp:write` | 啟用或停用frp 伺服器端 |
| PUT | `/api/frp/servers-order` | `frp:write` | 調整frp 伺服器端排序 |
| PUT | `/api/frp/servers-group` | `frp:write` | 批次設定frp 伺服器端分組 |
| GET | `/api/frp/servers/{key}/secrets` | `frp:write` | 查看金鑰明文 |
| GET | `/api/frp/proxies` | `frp:read` | 取得frp 代理清單 |
| POST | `/api/frp/proxies` | `frp:write` | 新增frp 代理 |
| PUT | `/api/frp/proxies/{key}` | `frp:write` | 修改frp 代理 |
| DELETE | `/api/frp/proxies/{key}` | `frp:write` | 刪除frp 代理 |
| PUT | `/api/frp/proxies/{key}/enable` | `frp:write` | 啟用或停用frp 代理 |
| PUT | `/api/frp/proxies-order` | `frp:write` | 調整frp 代理排序 |
| PUT | `/api/frp/proxies-group` | `frp:write` | 批次設定frp 代理分組 |
| GET | `/api/frp/proxies/{key}/secrets` | `frp:write` | 查看金鑰明文 |
| POST | `/api/frp/import` | `frp:write` | 匯入既有的 frpc / frps 設定（支援舊版 ini 與新版 toml）；apply 為 false 時僅預覽 |
| GET | `/api/frp/install` | `frp:read` | 檢視託管程式的安裝資訊與進度 |
| POST | `/api/frp/install` | `frp:write` | 線上安裝或更新託管程式 |
| POST | `/api/frp/install/rollback` | `frp:write` | 切換到保存的舊版本 |
| DELETE | `/api/frp/install` | `frp:write` | 解除安裝託管程式 |
| DELETE | `/api/frp/install/backups/{id}` | `frp:write` | 刪除保存的舊版本 |
| POST | `/api/frp/clients/{key}/restart` | `frp:write` | 重新啟動 frpc 實例 |
| POST | `/api/frp/servers/{key}/restart` | `frp:write` | 重新啟動 frps 實例 |
| GET | `/api/frp/settings` | `frp:read` | 讀取程式路徑與版本 |
| PUT | `/api/frp/settings` | `frp:write` | 儲存程式路徑 |
| POST | `/api/frp/reset` | `frp:write` | 恢復模組預設設定 |

## Cloudflare 通道

| 方法 | 路徑 | 所需權限 | 說明 |
| --- | --- | --- | --- |
| GET | `/api/cloudflared/logs` | `cloudflared:read` | 檢視模組執行日誌 |
| DELETE | `/api/cloudflared/logs` | `cloudflared:write` | 清空模組執行日誌 |
| GET | `/api/cloudflared/tunnels` | `cloudflared:read` | 取得Cloudflare 通道清單 |
| POST | `/api/cloudflared/tunnels` | `cloudflared:write` | 新增Cloudflare 通道 |
| PUT | `/api/cloudflared/tunnels/{key}` | `cloudflared:write` | 修改Cloudflare 通道 |
| DELETE | `/api/cloudflared/tunnels/{key}` | `cloudflared:write` | 刪除Cloudflare 通道 |
| PUT | `/api/cloudflared/tunnels/{key}/enable` | `cloudflared:write` | 啟用或停用Cloudflare 通道 |
| PUT | `/api/cloudflared/tunnels-order` | `cloudflared:write` | 調整Cloudflare 通道排序 |
| PUT | `/api/cloudflared/tunnels-group` | `cloudflared:write` | 批次設定Cloudflare 通道分組 |
| GET | `/api/cloudflared/tunnels/{key}/secrets` | `cloudflared:write` | 查看金鑰明文 |
| POST | `/api/cloudflared/tunnels/{key}/restart` | `cloudflared:write` | 重新啟動一條 Cloudflare 通道 |
| GET | `/api/cloudflared/tunnels/{key}/ingress` | `cloudflared:read` | 讀取權杖通道的路由規則 |
| PUT | `/api/cloudflared/tunnels/{key}/ingress` | `cloudflared:write` | 儲存權杖通道的路由規則 |
| GET | `/api/cloudflared/settings` | `cloudflared:read` | 讀取 Cloudflare 通道設定 |
| PUT | `/api/cloudflared/settings` | `cloudflared:write` | 儲存 Cloudflare 通道設定 |
| GET | `/api/cloudflared/install` | `cloudflared:read` | 檢視託管程式的安裝資訊與進度 |
| POST | `/api/cloudflared/install` | `cloudflared:write` | 線上安裝或更新託管程式 |
| POST | `/api/cloudflared/install/rollback` | `cloudflared:write` | 切換到保存的舊版本 |
| DELETE | `/api/cloudflared/install` | `cloudflared:write` | 解除安裝託管程式 |
| DELETE | `/api/cloudflared/install/backups/{id}` | `cloudflared:write` | 刪除保存的舊版本 |
| POST | `/api/cloudflared/reset` | `cloudflared:write` | 恢復模組預設設定 |

## SSL 憑證

| 方法 | 路徑 | 所需權限 | 說明 |
| --- | --- | --- | --- |
| GET | `/api/ssl/logs` | `ssl:read` | 檢視模組執行日誌 |
| DELETE | `/api/ssl/logs` | `ssl:write` | 清空模組執行日誌 |
| GET | `/api/ssl/certs` | `ssl:read` | 取得憑證清單 |
| POST | `/api/ssl/certs` | `ssl:write` | 新增憑證 |
| PUT | `/api/ssl/certs/{key}` | `ssl:write` | 修改憑證 |
| DELETE | `/api/ssl/certs/{key}` | `ssl:write` | 刪除憑證 |
| PUT | `/api/ssl/certs/{key}/enable` | `ssl:write` | 啟用或停用憑證 |
| PUT | `/api/ssl/certs-order` | `ssl:write` | 調整憑證排序 |
| PUT | `/api/ssl/certs-group` | `ssl:write` | 批次設定憑證分組 |
| GET | `/api/ssl/certs/{key}/secrets` | `ssl:write` | 查看金鑰明文 |
| POST | `/api/ssl/certs/{key}/renew` | `ssl:write` | 立即簽發或續期憑證 |
| GET | `/api/ssl/certs/{key}/download` | `ssl:write` | 下載憑證與私鑰 |
| POST | `/api/ssl/certs/{key}/deploy` | `ssl:write` | 立即部署憑證 |
| GET | `/api/ssl/deploy-targets` | `ssl:read` | 列出可選的部署目標 |
| GET | `/api/ssl/providers` | `ssl:read` | 列出支援的 DNS 驗證服務商 |
| GET | `/api/ssl/settings` | `ssl:read` | 讀取憑證全域設定 |
| PUT | `/api/ssl/settings` | `ssl:write` | 儲存憑證全域設定 |

## WAF 防火牆

| 方法 | 路徑 | 所需權限 | 說明 |
| --- | --- | --- | --- |
| GET | `/api/waf/logs` | `waf:read` | 檢視模組執行日誌 |
| DELETE | `/api/waf/logs` | `waf:write` | 清空模組執行日誌 |
| GET | `/api/waf/instances` | `waf:read` | 取得WAF 執行個體清單 |
| POST | `/api/waf/instances` | `waf:write` | 新增WAF 執行個體 |
| PUT | `/api/waf/instances/{key}` | `waf:write` | 修改WAF 執行個體 |
| DELETE | `/api/waf/instances/{key}` | `waf:write` | 刪除WAF 執行個體 |
| PUT | `/api/waf/instances/{key}/enable` | `waf:write` | 啟用或停用WAF 執行個體 |
| PUT | `/api/waf/instances-order` | `waf:write` | 調整WAF 執行個體排序 |
| PUT | `/api/waf/instances-group` | `waf:write` | 批次設定WAF 執行個體分組 |
| GET | `/api/waf/crs` | `waf:read` | 列出 OWASP CRS 規則檔案 |
| GET | `/api/waf/crs/{file}` | `waf:read` | 讀取一個 CRS 規則檔案 |

## Web 服務

| 方法 | 路徑 | 所需權限 | 說明 |
| --- | --- | --- | --- |
| GET | `/api/webservice/logs` | `webservice:read` | 檢視模組執行日誌 |
| DELETE | `/api/webservice/logs` | `webservice:write` | 清空模組執行日誌 |
| GET | `/api/webservice/listeners` | `webservice:read` | 取得Web 服務監聽清單 |
| POST | `/api/webservice/listeners` | `webservice:write` | 新增Web 服務監聽 |
| PUT | `/api/webservice/listeners/{key}` | `webservice:write` | 修改Web 服務監聽 |
| DELETE | `/api/webservice/listeners/{key}` | `webservice:write` | 刪除Web 服務監聽 |
| PUT | `/api/webservice/listeners/{key}/enable` | `webservice:write` | 啟用或停用Web 服務監聽 |
| PUT | `/api/webservice/listeners-order` | `webservice:write` | 調整Web 服務監聽排序 |
| GET | `/api/webservice/iplists` | `webservice:read` | 取得訂閱 IP 清單 |
| POST | `/api/webservice/iplists` | `webservice:write` | 新增訂閱 IP 清單 |
| PUT | `/api/webservice/iplists/{key}` | `webservice:write` | 修改訂閱 IP 清單 |
| DELETE | `/api/webservice/iplists/{key}` | `webservice:write` | 刪除訂閱 IP 清單 |
| PUT | `/api/webservice/iplists/{key}/enable` | `webservice:write` | 啟用或停用訂閱 IP 清單 |
| PUT | `/api/webservice/iplists-order` | `webservice:write` | 調整訂閱 IP 清單排序 |
| POST | `/api/webservice/iplists/{key}/refresh` | `webservice:write` | 立即重新整理訂閱 IP 清單 |
| GET | `/api/webservice/users` | `webservice:read` | 讀取共用使用者與 OIDC 身分提供者 |
| PUT | `/api/webservice/users` | `webservice:write` | 儲存共用使用者與 OIDC 身分提供者，批次設定站點 |
| POST | `/api/webservice/import/nginx` | `webservice:write` | 匯入 nginx 設定 |
| GET | `/api/webservice/settings` | `webservice:read` | 讀取 Web 服務全域設定 |
| GET | `/api/webservice/settings/secrets/totp` | `webservice:write` | 查看共用使用者的 TOTP 金鑰明文（依使用者名稱索引） |
| GET | `/api/webservice/settings/secrets/oauth` | `webservice:write` | 查看第三方登入的 Client Secret 明文（依登入方式識別索引） |
| POST | `/api/webservice/totp-secret` | `webservice:write` | 為共用使用者產生新的 TOTP 金鑰（不儲存，儲存設定後生效） |
| PUT | `/api/webservice/settings` | `webservice:write` | 儲存 Web 服務全域設定 |
| GET | `/api/webservice/stats` | `webservice:read` | 站點分鐘級統計（?listener=&site=&minutes=） |
| GET | `/api/webservice/metrics` | `webservice:read` | Prometheus 文字格式指標 |
| GET | `/api/webservice/bans` | `webservice:read` | 目前被封鎖的 IP |
| POST | `/api/webservice/bans` | `webservice:write` | 手動封鎖 IP |
| DELETE | `/api/webservice/bans` | `webservice:write` | 解除封鎖（body 為 {"key": 位址}） |
| GET | `/api/webservice/geoip` | `webservice:read` | GeoIP 資料庫狀態 |
| POST | `/api/webservice/geoip/upload` | `webservice:write` | 上傳 GeoIP 資料庫（multipart 欄位 file，.mmdb） |
| POST | `/api/webservice/geoip/download` | `webservice:write` | 從位址下載 GeoIP 資料庫（.mmdb） |
| DELETE | `/api/webservice/geoip` | `webservice:write` | 移除 GeoIP 資料庫 |

## 入侵防護

| 方法 | 路徑 | 所需權限 | 說明 |
| --- | --- | --- | --- |
| GET | `/api/guard/logs` | `guard:read` | 檢視模組執行日誌 |
| DELETE | `/api/guard/logs` | `guard:write` | 清空模組執行日誌 |
| GET | `/api/guard/jails` | `guard:read` | 取得入侵防護規則清單 |
| POST | `/api/guard/jails` | `guard:write` | 新增入侵防護規則 |
| PUT | `/api/guard/jails/{key}` | `guard:write` | 修改入侵防護規則 |
| DELETE | `/api/guard/jails/{key}` | `guard:write` | 刪除入侵防護規則 |
| PUT | `/api/guard/jails/{key}/enable` | `guard:write` | 啟用或停用入侵防護規則 |
| PUT | `/api/guard/jails-order` | `guard:write` | 調整入侵防護規則排序 |
| PUT | `/api/guard/jails-group` | `guard:write` | 批次設定入侵防護規則分組 |
| GET | `/api/guard/presets` | `guard:read` | 常見服務的規則範本 |
| POST | `/api/guard/jails/test` | `guard:write` | 用日誌樣例測試過濾規則 |
| GET | `/api/guard/status` | `guard:read` | 防火牆同步狀態與執行環境 |
| GET | `/api/guard/settings` | `guard:read` | 全域設定 |
| PUT | `/api/guard/settings` | `guard:write` | 修改全域設定 |
| GET | `/api/guard/bans` | `guard:read` | 目前封鎖的 IP |
| POST | `/api/guard/bans` | `guard:write` | 手動封鎖 IP 或網段 |
| DELETE | `/api/guard/bans` | `guard:write` | 解除封鎖（body 為 {"key": 位址}） |
| GET | `/api/guard/firewall/rules` | `guard:read` | Gately 寫入 nftables 的規則（文字） |

## 網路喚醒

| 方法 | 路徑 | 所需權限 | 說明 |
| --- | --- | --- | --- |
| GET | `/api/wol/logs` | `wol:read` | 檢視模組執行日誌 |
| DELETE | `/api/wol/logs` | `wol:write` | 清空模組執行日誌 |
| GET | `/api/wol/devices` | `wol:read` | 取得喚醒裝置清單 |
| POST | `/api/wol/devices` | `wol:write` | 新增喚醒裝置 |
| PUT | `/api/wol/devices/{key}` | `wol:write` | 修改喚醒裝置 |
| DELETE | `/api/wol/devices/{key}` | `wol:write` | 刪除喚醒裝置 |
| PUT | `/api/wol/devices/{key}/enable` | `wol:write` | 啟用或停用喚醒裝置 |
| PUT | `/api/wol/devices-order` | `wol:write` | 調整喚醒裝置排序 |
| PUT | `/api/wol/devices-group` | `wol:write` | 批次設定喚醒裝置分組 |
| POST | `/api/wol/devices/{key}/wake` | `wol:write` | 向裝置傳送喚醒魔術封包 |
| GET | `/api/wol/status` | `wol:read` | 檢視裝置線上狀態 |

## Web 終端機

| 方法 | 路徑 | 所需權限 | 說明 |
| --- | --- | --- | --- |
| GET | `/api/webterminal/logs` | `webterminal:read` | 檢視模組執行日誌 |
| DELETE | `/api/webterminal/logs` | `webterminal:write` | 清空模組執行日誌 |
| GET | `/api/webterminal/connections` | `webterminal:read` | 取得終端機連線清單 |
| POST | `/api/webterminal/connections` | `webterminal:write` | 新增終端機連線 |
| PUT | `/api/webterminal/connections/{key}` | `webterminal:write` | 修改終端機連線 |
| DELETE | `/api/webterminal/connections/{key}` | `webterminal:write` | 刪除終端機連線 |
| PUT | `/api/webterminal/connections/{key}/enable` | `webterminal:write` | 啟用或停用終端機連線 |
| PUT | `/api/webterminal/connections-order` | `webterminal:write` | 調整終端機連線排序 |
| PUT | `/api/webterminal/connections-group` | `webterminal:write` | 批次設定終端機連線分組 |
| GET | `/api/webterminal/connections/{key}/secrets` | `webterminal:write` | 查看金鑰明文 |
| DELETE | `/api/webterminal/connections/{key}/hostkey` | `webterminal:write` | 清除已記錄的主機金鑰 |
| POST | `/api/webterminal/connections/{key}/test` | `webterminal:write` | 測試已儲存連線 |
| POST | `/api/webterminal/connections/test` | `webterminal:write` | 測試暫時連線參數 |
| PUT | `/api/webterminal/connections/{key}/quickaccess` | `webterminal:write` | 設定連線的快捷入口 |
| GET | `/api/webterminal/credentials` | `webterminal:read` | 取得項目清單 |
| POST | `/api/webterminal/credentials` | `webterminal:write` | 新增項目 |
| PUT | `/api/webterminal/credentials/{key}` | `webterminal:write` | 修改項目 |
| DELETE | `/api/webterminal/credentials/{key}` | `webterminal:write` | 刪除項目 |
| PUT | `/api/webterminal/credentials/{key}/enable` | `webterminal:write` | 啟用或停用項目 |
| PUT | `/api/webterminal/credentials-order` | `webterminal:write` | 調整項目排序 |
| GET | `/api/webterminal/credentials/{key}/secrets` | `webterminal:write` | 查看金鑰明文 |
| GET | `/api/webterminal/connections/{key}/ws` | `webterminal:write` | 開啟終端機工作階段（WebSocket） |
| GET | `/api/webterminal/sessions/{id}/ws` | `webterminal:write` | 接入既有終端機工作階段（WebSocket） |
| GET | `/api/webterminal/sessions` | `webterminal:read` | 列出終端機工作階段 |
| GET | `/api/webterminal/sessions/{id}` | `webterminal:read` | 檢視終端機工作階段詳情 |
| PUT | `/api/webterminal/sessions/{id}/remark` | `webterminal:write` | 修改終端機工作階段備註 |
| GET | `/api/webterminal/sessions/{id}/stats` | `webterminal:read` | 檢視終端機工作階段統計 |
| DELETE | `/api/webterminal/sessions/{id}` | `webterminal:write` | 關閉終端機工作階段 |
| GET | `/api/webterminal/security` | 僅登入工作階段 | 檢視終端機安全設定狀態 |
| POST | `/api/webterminal/security/agreement` | 僅登入工作階段 | 確認終端機使用協議 |
| POST | `/api/webterminal/security/2fa/setup` | 僅登入工作階段 | 產生終端機兩步驟驗證金鑰 |
| POST | `/api/webterminal/security/2fa/enable` | 僅登入工作階段 | 啟用終端機兩步驟驗證 |
| POST | `/api/webterminal/security/2fa/disable` | 僅登入工作階段 | 關閉終端機兩步驟驗證 |
| POST | `/api/webterminal/security/verify2fa` | 僅登入工作階段 | 驗證終端機兩步驟驗證碼 |
| GET | `/api/webterminal/security/check2fa` | 僅登入工作階段 | 檢查終端機兩步驟驗證是否已通過 |
| GET | `/api/webterminal/sftp/{key}/list` | `webterminal:write` | 列出遠端目錄 |
| GET | `/api/webterminal/sftp/{key}/read` | `webterminal:write` | 讀取遠端檔案 |
| POST | `/api/webterminal/sftp/{key}/write` | `webterminal:write` | 寫入遠端檔案 |
| POST | `/api/webterminal/sftp/{key}/upload` | `webterminal:write` | 上傳檔案到遠端主機 |
| POST | `/api/webterminal/sftp/{key}/mkdir` | `webterminal:write` | 建立遠端目錄 |
| POST | `/api/webterminal/sftp/{key}/touch` | `webterminal:write` | 建立遠端空檔案 |
| POST | `/api/webterminal/sftp/{key}/rename` | `webterminal:write` | 重新命名或移動遠端檔案 |
| POST | `/api/webterminal/sftp/{key}/copy` | `webterminal:write` | 複製遠端檔案 |
| POST | `/api/webterminal/sftp/{key}/chmod` | `webterminal:write` | 修改遠端檔案權限 |
| DELETE | `/api/webterminal/sftp/{key}/remove` | `webterminal:write` | 刪除遠端檔案 |
| POST | `/api/webterminal/sftp/{key}/compress` | `webterminal:write` | 在遠端主機壓縮檔案 |
| POST | `/api/webterminal/sftp/{key}/decompress` | `webterminal:write` | 在遠端主機解壓縮檔案 |
| GET | `/api/webterminal/sftp/{key}/preview-archive` | `webterminal:write` | 預覽遠端壓縮檔內容 |
| GET | `/api/webterminal/settings` | `webterminal:read` | 讀取 Web 終端機設定 |
| PUT | `/api/webterminal/settings` | `webterminal:write` | 儲存 Web 終端機設定 |
| GET | `/api/webterminal/shells` | `webterminal:read` | 列出本機可用 Shell |
| GET | `/api/webterminal/globalshortcuts` | `webterminal:read` | 讀取全域快速指令 |
| PUT | `/api/webterminal/globalshortcuts` | `webterminal:write` | 儲存全域快速指令 |
| GET | `/api/webterminal/splitlayout` | `webterminal:read` | 讀取分割畫面配置 |
| PUT | `/api/webterminal/splitlayout` | `webterminal:write` | 儲存分割畫面配置 |
| DELETE | `/api/webterminal/splitlayout` | `webterminal:write` | 清除分割畫面配置 |

## 排程任務

| 方法 | 路徑 | 所需權限 | 說明 |
| --- | --- | --- | --- |
| GET | `/api/cron/logs` | `cron:read` | 檢視模組執行日誌 |
| DELETE | `/api/cron/logs` | `cron:write` | 清空模組執行日誌 |
| GET | `/api/cron/tasks` | `cron:read` | 取得排程任務清單 |
| POST | `/api/cron/tasks` | `cron:write` | 新增排程任務 |
| PUT | `/api/cron/tasks/{key}` | `cron:write` | 修改排程任務 |
| DELETE | `/api/cron/tasks/{key}` | `cron:write` | 刪除排程任務 |
| PUT | `/api/cron/tasks/{key}/enable` | `cron:write` | 啟用或停用排程任務 |
| PUT | `/api/cron/tasks-order` | `cron:write` | 調整排程任務排序 |
| PUT | `/api/cron/tasks-group` | `cron:write` | 批次設定排程任務分組 |
| POST | `/api/cron/tasks/{key}/run` | `cron:write` | 立即執行一個排程任務 |
| POST | `/api/cron/parse` | `cron:write` | 解析並預覽 Cron 運算式 |

## Docker 管理

| 方法 | 路徑 | 所需權限 | 說明 |
| --- | --- | --- | --- |
| GET | `/api/docker/logs` | `docker:read` | 檢視模組執行日誌 |
| DELETE | `/api/docker/logs` | `docker:write` | 清空模組執行日誌 |
| GET | `/api/docker/tasks` | `docker:read` | 列出背景任務 |
| GET | `/api/docker/tasks/{id}` | `docker:read` | 檢視背景任務進度與輸出 |
| DELETE | `/api/docker/tasks` | `docker:write` | 清理已結束的背景任務 |
| GET | `/api/docker/images` | `docker:read` | 列出映像檔 |
| POST | `/api/docker/images/pull` | `docker:write` | 拉取映像檔 |
| POST | `/api/docker/images/push` | `docker:write` | 推送映像檔 |
| POST | `/api/docker/images/search` | `docker:write` | 搜尋映像檔 |
| POST | `/api/docker/images/load` | `docker:write` | 匯入映像檔檔案 |
| POST | `/api/docker/images/build` | `docker:write` | 建置映像檔 |
| GET | `/api/docker/images/{id}` | `docker:read` | 檢視映像檔詳情 |
| POST | `/api/docker/images/{id}/tag` | `docker:write` | 為映像檔加上標籤 |
| DELETE | `/api/docker/images/{id}` | `docker:write` | 刪除映像檔 |
| GET | `/api/docker/images/{id}/export` | `docker:write` | 匯出映像檔 |
| GET | `/api/docker/networks` | `docker:read` | 列出網路 |
| POST | `/api/docker/networks` | `docker:write` | 建立網路 |
| DELETE | `/api/docker/networks/{id}` | `docker:write` | 刪除網路 |
| POST | `/api/docker/networks/{id}/connect` | `docker:write` | 將容器接入網路 |
| POST | `/api/docker/networks/{id}/disconnect` | `docker:write` | 將容器從網路中斷開 |
| GET | `/api/docker/volumes` | `docker:read` | 列出資料卷 |
| POST | `/api/docker/volumes` | `docker:write` | 建立資料卷 |
| DELETE | `/api/docker/volumes/{name}` | `docker:write` | 刪除資料卷 |
| GET | `/api/docker/volumes/{name}/export` | `docker:write` | 匯出資料卷 |
| POST | `/api/docker/volumes/{name}/import` | `docker:write` | 向資料卷匯入資料 |
| GET | `/api/docker/compose/projects` | `docker:read` | 列出 Compose 專案 |
| POST | `/api/docker/compose/projects` | `docker:write` | 建立 Compose 專案 |
| POST | `/api/docker/compose/{project}/up` | `docker:write` | 啟動 Compose 專案 |
| POST | `/api/docker/compose/{project}/pull` | `docker:write` | 拉取 Compose 專案映像檔 |
| POST | `/api/docker/compose/{project}/{action}` | `docker:write` | 對 Compose 專案執行操作 |
| GET | `/api/docker/compose/{project}/config` | `docker:write` | 讀取 Compose 設定檔 |
| PUT | `/api/docker/compose/{project}/config` | `docker:write` | 儲存 Compose 設定檔 |
| GET | `/api/docker/compose/{project}/logs` | `docker:read` | 檢視 Compose 專案日誌 |
| DELETE | `/api/docker/compose/{project}` | `docker:write` | 刪除 Compose 專案 |
| GET | `/api/docker/info` | `docker:read` | 檢視 Docker 引擎資訊 |
| GET | `/api/docker/disk-usage` | `docker:read` | 檢視 Docker 磁碟佔用 |
| POST | `/api/docker/prune` | `docker:write` | 清理無用的 Docker 資源 |
| GET | `/api/docker/settings` | `docker:read` | 讀取 Docker 模組設定 |
| PUT | `/api/docker/settings` | `docker:write` | 儲存 Docker 模組設定 |
| GET | `/api/docker/settings/secrets` | `docker:write` | 查看倉庫密碼明文（鍵為 `registry:<倉庫位址>`） |
| GET | `/api/docker/containers` | `docker:read` | 列出容器 |
| POST | `/api/docker/containers` | `docker:write` | 建立容器 |
| POST | `/api/docker/containers/check-all` | `docker:write` | 檢查全部容器映像檔更新 |
| POST | `/api/docker/containers/upgrade-batch` | `docker:write` | 批次升級容器 |
| POST | `/api/docker/images/upgrade-check` | `docker:write` | 檢查映像檔是否有新版本 |
| GET | `/api/docker/containers/{id}` | `docker:read` | 檢視容器詳情 |
| GET | `/api/docker/containers/{id}/secrets` | `docker:write` | 檢視容器環境變數與啟動指令明文 |
| DELETE | `/api/docker/containers/{id}` | `docker:write` | 刪除容器 |
| POST | `/api/docker/containers/{id}/edit` | `docker:write` | 修改容器設定並重建 |
| POST | `/api/docker/containers/{id}/rename` | `docker:write` | 重新命名容器 |
| POST | `/api/docker/containers/{id}/commit` | `docker:write` | 將容器提交為映像檔 |
| POST | `/api/docker/containers/{id}/check` | `docker:write` | 檢查容器映像檔是否有更新 |
| POST | `/api/docker/containers/{id}/upgrade` | `docker:write` | 升級容器映像檔並重建 |
| POST | `/api/docker/containers/{id}/{action}` | `docker:write` | 對容器執行操作 |
| GET | `/api/docker/containers/{id}/logs` | `docker:read` | 檢視容器日誌 |
| GET | `/api/docker/containers/{id}/stats` | `docker:read` | 檢視容器資源佔用 |
| GET | `/api/docker/containers/{id}/processes` | `docker:read` | 檢視容器內程序 |
| GET | `/api/docker/containers/{id}/files/list` | `docker:write` | 列出容器內目錄 |
| GET | `/api/docker/containers/{id}/files/read` | `docker:write` | 讀取容器內檔案 |
| GET | `/api/docker/containers/{id}/files/download` | `docker:write` | 下載容器內檔案 |
| POST | `/api/docker/containers/{id}/files/write` | `docker:write` | 寫入容器內檔案 |
| POST | `/api/docker/containers/{id}/files/mkdir` | `docker:write` | 在容器內建立目錄 |
| POST | `/api/docker/containers/{id}/files/rename` | `docker:write` | 重新命名容器內檔案 |
| POST | `/api/docker/containers/{id}/files/upload` | `docker:write` | 上傳檔案到容器 |
| DELETE | `/api/docker/containers/{id}/files` | `docker:write` | 刪除容器內檔案 |
| GET | `/api/docker/containers/{id}/exec` | `docker:write` | 在容器內開啟終端機（WebSocket） |

## WebDAV 服務

| 方法 | 路徑 | 所需權限 | 說明 |
| --- | --- | --- | --- |
| GET | `/api/webdav/logs` | `webdav:read` | 檢視模組執行日誌 |
| DELETE | `/api/webdav/logs` | `webdav:write` | 清空模組執行日誌 |
| GET | `/api/webdav/settings` | `webdav:read` | 讀取 WebDAV 設定 |
| GET | `/api/webdav/settings/secrets` | `webdav:write` | 查看本機使用者密碼明文 |
| GET | `/api/webdav/shared-users` | `webdav:read` | 讀取檔案服務共用使用者（舊介面） |
| PUT | `/api/webdav/shared-users` | `webdav:write,ftp:write,smb:write` | 儲存檔案服務共用使用者（舊介面） |
| PUT | `/api/webdav/settings` | `webdav:write` | 儲存 WebDAV 設定 |

## FTP 服務

| 方法 | 路徑 | 所需權限 | 說明 |
| --- | --- | --- | --- |
| GET | `/api/ftp/logs` | `ftp:read` | 檢視模組執行日誌 |
| DELETE | `/api/ftp/logs` | `ftp:write` | 清空模組執行日誌 |
| GET | `/api/ftp/settings` | `ftp:read` | 讀取 FTP 設定 |
| GET | `/api/ftp/settings/secrets` | `ftp:write` | 查看本機使用者密碼明文 |
| GET | `/api/ftp/shared-users` | `ftp:read` | 讀取檔案服務共用使用者（舊介面） |
| PUT | `/api/ftp/shared-users` | `webdav:write,ftp:write,smb:write` | 儲存檔案服務共用使用者（舊介面） |
| PUT | `/api/ftp/settings` | `ftp:write` | 儲存 FTP 設定 |

## SMB 服務

| 方法 | 路徑 | 所需權限 | 說明 |
| --- | --- | --- | --- |
| GET | `/api/smb/logs` | `smb:read` | 檢視模組執行日誌 |
| DELETE | `/api/smb/logs` | `smb:write` | 清空模組執行日誌 |
| GET | `/api/smb/settings` | `smb:read` | 讀取 SMB 設定 |
| GET | `/api/smb/settings/secrets` | `smb:write` | 查看本機使用者密碼明文 |
| GET | `/api/smb/shared-users` | `smb:read` | 讀取檔案服務共用使用者（舊介面） |
| PUT | `/api/smb/shared-users` | `webdav:write,ftp:write,smb:write` | 儲存檔案服務共用使用者（舊介面） |
| PUT | `/api/smb/settings` | `smb:write` | 儲存 SMB 設定 |

## File Browser

| 方法 | 路徑 | 所需權限 | 說明 |
| --- | --- | --- | --- |
| GET | `/api/filebrowser/logs` | `filebrowser:read` | 檢視模組執行日誌 |
| DELETE | `/api/filebrowser/logs` | `filebrowser:write` | 清空模組執行日誌 |
| GET | `/api/filebrowser/instances` | `filebrowser:read` | 取得File Browser 實例清單 |
| POST | `/api/filebrowser/instances` | `filebrowser:write` | 新增File Browser 實例 |
| PUT | `/api/filebrowser/instances/{key}` | `filebrowser:write` | 修改File Browser 實例 |
| DELETE | `/api/filebrowser/instances/{key}` | `filebrowser:write` | 刪除File Browser 實例 |
| PUT | `/api/filebrowser/instances/{key}/enable` | `filebrowser:write` | 啟用或停用File Browser 實例 |
| PUT | `/api/filebrowser/instances-order` | `filebrowser:write` | 調整File Browser 實例排序 |
| PUT | `/api/filebrowser/instances-group` | `filebrowser:write` | 批次設定File Browser 實例分組 |
| GET | `/api/filebrowser/instances/{key}/secrets` | `filebrowser:write` | 查看金鑰明文 |
| POST | `/api/filebrowser/instances/{key}/restart` | `filebrowser:write` | 重新啟動一個 File Browser 實例 |
| GET | `/api/filebrowser/settings` | `filebrowser:read` | 讀取 File Browser 程式設定 |
| PUT | `/api/filebrowser/settings` | `filebrowser:write` | 儲存 File Browser 程式設定 |
| GET | `/api/filebrowser/install` | `filebrowser:read` | 檢視託管程式的安裝資訊與進度 |
| POST | `/api/filebrowser/install` | `filebrowser:write` | 線上安裝或更新託管程式 |
| POST | `/api/filebrowser/install/rollback` | `filebrowser:write` | 切換到保存的舊版本 |
| DELETE | `/api/filebrowser/install` | `filebrowser:write` | 解除安裝託管程式 |
| DELETE | `/api/filebrowser/install/backups/{id}` | `filebrowser:write` | 刪除保存的舊版本 |
| POST | `/api/filebrowser/reset` | `filebrowser:write` | 恢復模組預設設定 |

## 儲存掛載

| 方法 | 路徑 | 所需權限 | 說明 |
| --- | --- | --- | --- |
| GET | `/api/storage/logs` | `storage:read` | 檢視模組執行日誌 |
| DELETE | `/api/storage/logs` | `storage:write` | 清空模組執行日誌 |
| GET | `/api/storage/mounts` | `storage:read` | 取得儲存掛載清單 |
| POST | `/api/storage/mounts` | `storage:write` | 新增儲存掛載 |
| PUT | `/api/storage/mounts/{key}` | `storage:write` | 修改儲存掛載 |
| DELETE | `/api/storage/mounts/{key}` | `storage:write` | 刪除儲存掛載 |
| PUT | `/api/storage/mounts/{key}/enable` | `storage:write` | 啟用或停用儲存掛載 |
| PUT | `/api/storage/mounts-order` | `storage:write` | 調整儲存掛載排序 |
| PUT | `/api/storage/mounts-group` | `storage:write` | 批次設定儲存掛載分組 |
| GET | `/api/storage/mounts/{key}/secrets` | `storage:write` | 查看金鑰明文 |
| GET | `/api/storage/files/{key}/list` | `storage:write` | 列出儲存目錄 |
| GET | `/api/storage/files/{key}/download` | `storage:write` | 下載儲存檔案 |
| POST | `/api/storage/files/{key}/upload` | `storage:write` | 上傳檔案到儲存 |
| POST | `/api/storage/files/{key}/mkdir` | `storage:write` | 建立儲存目錄 |
| POST | `/api/storage/files/{key}/rename` | `storage:write` | 重新命名或移動儲存檔案 |
| DELETE | `/api/storage/files/{key}` | `storage:write` | 刪除儲存檔案 |

## 通知中心

| 方法 | 路徑 | 所需權限 | 說明 |
| --- | --- | --- | --- |
| GET | `/api/notify/logs` | `notify:read` | 檢視模組執行日誌 |
| DELETE | `/api/notify/logs` | `notify:write` | 清空模組執行日誌 |
| GET | `/api/notify/channels` | `notify:read` | 取得通知管道清單 |
| POST | `/api/notify/channels` | `notify:write` | 新增通知管道 |
| PUT | `/api/notify/channels/{key}` | `notify:write` | 修改通知管道 |
| DELETE | `/api/notify/channels/{key}` | `notify:write` | 刪除通知管道 |
| PUT | `/api/notify/channels/{key}/enable` | `notify:write` | 啟用或停用通知管道 |
| PUT | `/api/notify/channels-order` | `notify:write` | 調整通知管道排序 |
| PUT | `/api/notify/channels-group` | `notify:write` | 批次設定通知管道分組 |
| GET | `/api/notify/channels/{key}/secrets` | `notify:write` | 查看金鑰明文 |
| GET | `/api/notify/rules` | `notify:read` | 取得通知規則清單 |
| POST | `/api/notify/rules` | `notify:write` | 新增通知規則 |
| PUT | `/api/notify/rules/{key}` | `notify:write` | 修改通知規則 |
| DELETE | `/api/notify/rules/{key}` | `notify:write` | 刪除通知規則 |
| PUT | `/api/notify/rules/{key}/enable` | `notify:write` | 啟用或停用通知規則 |
| PUT | `/api/notify/rules-order` | `notify:write` | 調整通知規則排序 |
| PUT | `/api/notify/rules-group` | `notify:write` | 批次設定通知規則分組 |
| GET | `/api/notify/event-types` | `notify:read` | 可訂閱的事件類型 |
| GET | `/api/notify/history` | `notify:read` | 最近的投遞記錄 |
| POST | `/api/notify/channels/{key}/test` | `notify:write` | 向管道傳送測試通知 |

## 網路診斷

| 方法 | 路徑 | 所需權限 | 說明 |
| --- | --- | --- | --- |
| GET | `/api/nettools/logs` | `nettools:read` | 檢視模組執行日誌 |
| DELETE | `/api/nettools/logs` | `nettools:write` | 清空模組執行日誌 |
| POST | `/api/nettools/run` | `nettools:write` | 執行診斷工具，回傳任務（tool: ping / traceroute / tcp / dns / speed / scan） |
| GET | `/api/nettools/tasks` | `nettools:read` | 最近的診斷任務 |
| GET | `/api/nettools/tasks/{id}` | `nettools:read` | 任務詳情與輸出 |
| DELETE | `/api/nettools/tasks/{id}` | `nettools:write` | 取消執行中的任務，或刪除已結束的任務 |
| GET | `/api/nettools/settings` | `nettools:read` | 診斷設定 |
| PUT | `/api/nettools/settings` | `nettools:write` | 修改禁止探測清單 |
| GET | `/api/nettools/networks` | `nettools:read` | 本機可用於區域網路掃描的網段 |

## 設定備份

| 方法 | 路徑 | 所需權限 | 說明 |
| --- | --- | --- | --- |
| GET | `/api/backup/logs` | `backup:read` | 檢視模組執行日誌 |
| DELETE | `/api/backup/logs` | `backup:write` | 清空模組執行日誌 |
| GET | `/api/backup/targets` | `backup:read` | 取得備份目的地清單 |
| POST | `/api/backup/targets` | `backup:write` | 新增備份目的地 |
| PUT | `/api/backup/targets/{key}` | `backup:write` | 修改備份目的地 |
| DELETE | `/api/backup/targets/{key}` | `backup:write` | 刪除備份目的地 |
| PUT | `/api/backup/targets/{key}/enable` | `backup:write` | 啟用或停用備份目的地 |
| PUT | `/api/backup/targets-order` | `backup:write` | 調整備份目的地排序 |
| GET | `/api/backup/settings` | `backup:read` | 備份設定 |
| PUT | `/api/backup/settings` | `backup:write` | 修改備份設定（口令傳佔位符表示保持不變） |
| GET | `/api/backup/secrets` | `backup:write` | 查看金鑰明文 |
| POST | `/api/backup/run` | `backup:write` | 立即備份一次 |
| GET | `/api/backup/status` | `backup:read` | 最近一次備份結果與下次執行時間 |
| GET | `/api/backup/mounts` | `backup:read` | 可作為備份目的地的儲存掛載點 |
| GET | `/api/backup/local` | 僅登入工作階段 | 本機備份檔案清單 |
| GET | `/api/backup/local/{name}` | 僅登入工作階段 | 下載本機備份檔案 |

## 檔案服務共用使用者

| 方法 | 路徑 | 所需權限 | 說明 |
| --- | --- | --- | --- |
| GET | `/api/fileusers` | `fileusers:read` | 讀取檔案服務共用使用者 |
| PUT | `/api/fileusers` | `fileusers:write` | 儲存檔案服務共用使用者 |
| GET | `/api/fileusers/secrets` | `fileusers:write` | 查看共用使用者密碼明文 |

## 系統

| 方法 | 路徑 | 所需權限 | 說明 |
| --- | --- | --- | --- |
| GET | `/api/session` | 任意有效權杖 | 檢視目前驗證身分 |
| GET | `/api/status` | `status:read` | 檢視系統與各模組狀態 |
| GET | `/api/logs` | `logs:read` | 檢視系統日誌 |
| GET | `/api/system/settings` | 僅登入工作階段 | 讀取全域設定 |
| PUT | `/api/system/settings` | 僅登入工作階段 | 儲存全域設定 |
| GET | `/api/system/settings/secrets` | 僅登入工作階段 | 查看第三方登入 Client Secret 明文 |
| GET | `/api/system/info` | `status:read` | 檢視系統資訊 |
| GET | `/api/system/update` | `status:read` | 檢查新版本 |
| POST | `/api/system/upgrade` | 僅登入工作階段 | 線上升級到最新版本 |
| GET | `/api/system/upgrade/status` | 僅登入工作階段 | 查看線上升級進度 |
| POST | `/api/system/upgrade/cancel` | 僅登入工作階段 | 取消線上升級 |
| POST | `/api/system/upgrade/upload` | 僅登入工作階段 | 上傳升級包升級 |
| GET | `/api/system/backup` | 僅登入工作階段 | 下載設定備份 |
| GET | `/api/system/diagnostics` | 僅登入工作階段 | 匯出已去識別化的診斷包 |
| GET | `/api/audit` | 僅登入工作階段 | 查詢操作稽核日誌 |
| GET | `/api/metrics` | `metrics:read` | Prometheus 全域指標 |
| GET | `/api/history` | 僅登入工作階段 | 列出設定歷史版本 |
| GET | `/api/history/{name}/{id}/diff` | 僅登入工作階段 | 比較歷史版本與目前設定 |
| POST | `/api/history/{name}/{id}/restore` | 僅登入工作階段 | 將設定還原到歷史版本 |
| POST | `/api/system/restore` | 僅登入工作階段 | 從備份還原設定 |
| POST | `/api/system/restart` | 僅登入工作階段 | 重新啟動 Gately |
| POST | `/api/system/selfcheck` | 僅登入工作階段 | 網路與設定自我檢查 |
| GET | `/api/system/onboarding` | 僅登入工作階段 | 是否需要首次啟動引導 |
| POST | `/api/system/onboarding/dismiss` | 僅登入工作階段 | 完成或略過首次啟動引導 |
| GET | `/api/openapi.json` | 任意有效權杖 | 取得本 API 的 OpenAPI 描述 |

## 設定匯入

| 方法 | 路徑 | 所需權限 | 說明 |
| --- | --- | --- | --- |
| POST | `/api/import/lucky` | `import:write` | 匯入 Lucky 設定 |

## API 權杖

| 方法 | 路徑 | 所需權限 | 說明 |
| --- | --- | --- | --- |
| GET | `/api/tokens` | 僅登入工作階段 | 列出 API 權杖 |
| GET | `/api/tokens/scopes` | 僅登入工作階段 | 列出可授權的權限域 |
| POST | `/api/tokens` | 僅登入工作階段 | 建立 API 權杖 |
| PUT | `/api/tokens/order` | 僅登入工作階段 | 調整 API 權杖的排序 |
| PUT | `/api/tokens/group` | 僅登入工作階段 | 批次設定 API 權杖的分組 |
| PUT | `/api/tokens/{id}` | 僅登入工作階段 | 修改 API 權杖的名稱、分組、權限、到期時間與啟停 |
| POST | `/api/tokens/{id}/regenerate` | 僅登入工作階段 | 重新產生權杖明文 |
| DELETE | `/api/tokens/{id}` | 僅登入工作階段 | 刪除 API 權杖 |

## 帳號

| 方法 | 路徑 | 所需權限 | 說明 |
| --- | --- | --- | --- |
| PUT | `/api/account` | 僅登入工作階段 | 修改管理員使用者名稱與密碼 |
| PUT | `/api/account/ui-language` | 僅登入工作階段 | 記錄管理介面語言 |
| GET | `/api/account/passkeys` | 僅登入工作階段 | 列出已註冊的 Passkey |
| POST | `/api/account/passkeys/begin` | 僅登入工作階段 | 開始註冊 Passkey |
| POST | `/api/account/passkeys` | 僅登入工作階段 | 完成註冊 Passkey |
| DELETE | `/api/account/passkeys/{id}` | 僅登入工作階段 | 刪除 Passkey |
| GET | `/api/account/users` | 僅登入工作階段 | 列出唯讀帳號 |
| POST | `/api/account/users` | 僅登入工作階段 | 建立唯讀帳號 |
| PUT | `/api/account/users/{name}` | 僅登入工作階段 | 重設唯讀帳號密碼 |
| DELETE | `/api/account/users/{name}` | 僅登入工作階段 | 刪除唯讀帳號 |
| GET | `/api/account/totp` | 僅登入工作階段 | 檢視兩步驟驗證狀態 |
| POST | `/api/account/totp/setup` | 僅登入工作階段 | 產生兩步驟驗證金鑰 |
| POST | `/api/account/totp/enable` | 僅登入工作階段 | 啟用兩步驟驗證 |
| POST | `/api/account/totp/disable` | 僅登入工作階段 | 關閉兩步驟驗證 |

## 資料結構

### backup.LocalFile

| 欄位 | 類型 |
| --- | --- |
| `modTime` | 時間 (RFC 3339) |
| `name` | string |
| `size` | integer |

### backup.Result

| 欄位 | 類型 |
| --- | --- |
| `error` | string |
| `file` | string |
| `ok` | boolean |
| `pruned` | integer |
| `target` | string |

### backup.Settings

| 欄位 | 類型 |
| --- | --- |
| `enable` | boolean |
| `keep` | integer |
| `localEnable` | boolean |
| `password` | string |
| `schedule` | string |

### backup.Status

| 欄位 | 類型 |
| --- | --- |
| `lastRun` | 時間 (RFC 3339) |
| `manual` | boolean |
| `nextRun` | 時間 (RFC 3339) |
| `results` | 陣列 of `backup.Result` |
| `running` | boolean |

### backup.Target

| 欄位 | 類型 |
| --- | --- |
| `dir` | string |
| `enable` | boolean |
| `key` | string |
| `mount` | string |
| `name` | string |

### backup.settingsView

| 欄位 | 類型 |
| --- | --- |
| `enable` | boolean |
| `hasPassword` | boolean |
| `keep` | integer |
| `localEnable` | boolean |
| `password` | string |
| `schedule` | string |

### banlist.Entry

| 欄位 | 類型 |
| --- | --- |
| `count` | integer |
| `created` | 時間 (RFC 3339) |
| `key` | string |
| `reason` | string |
| `source` | string |
| `until` | 時間 (RFC 3339) |

### cloudflared.Tunnel

| 欄位 | 類型 |
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

| 欄位 | 類型 |
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

### crud.enableRequest

| 欄位 | 類型 |
| --- | --- |
| `enable` | boolean |

### crud.groupRequest

| 欄位 | 類型 |
| --- | --- |
| `group` | string |
| `keys` | 陣列 of string |

### crud.orderRequest

| 欄位 | 類型 |
| --- | --- |
| `keys` | 陣列 of string |

### ddns.Record

| 欄位 | 類型 |
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

### ddns.Task

| 欄位 | 類型 |
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
| `records` | 陣列 of `ddns.Record` |
| `remark` | string |
| `webhook` | `webhook.Config` |

### ddns.recordLayout

| 欄位 | 類型 |
| --- | --- |
| `group` | string |
| `key` | string |

### ddns.recordsLayoutRequest

| 欄位 | 類型 |
| --- | --- |
| `records` | 陣列 of `ddns.recordLayout` |

### dnsprovider.Credentials

| 欄位 | 類型 |
| --- | --- |
| `extra` | 對應 of string |
| `id` | string |
| `secret` | string |

### events.TypeInfo

| 欄位 | 類型 |
| --- | --- |
| `level` | string |
| `name` | string |
| `type` | string |

### filebrowser.Instance

| 欄位 | 類型 |
| --- | --- |
| `accessUrls` | 陣列 of string |
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
| `sources` | 陣列 of `filebrowser.Source` |
| `tokenHours` | integer |
| `trustProxyHeaders` | boolean |

### filebrowser.Source

| 欄位 | 類型 |
| --- | --- |
| `name` | string |
| `path` | string |
| `readOnly` | boolean |

### frp.BinaryView

| 欄位 | 類型 |
| --- | --- |
| `binary` | string |
| `error` | string |
| `version` | string |

### frp.Client

| 欄位 | 類型 |
| --- | --- |
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

### frp.ClientSettings

| 欄位 | 類型 |
| --- | --- |
| `protocol` | string |
| `serverAddr` | string |
| `serverPort` | integer |
| `token` | string |
| `user` | string |

### frp.ImportRequest

| 欄位 | 類型 |
| --- | --- |
| `apply` | boolean |
| `content` | string |
| `name` | string |
| `proxyMode` | string |
| `role` | string |
| `target` | string |

### frp.ImportResponse

| 欄位 | 類型 |
| --- | --- |
| `added` | integer |
| `applied` | boolean |
| `client` | `frp.ClientSettings` |
| `key` | string |
| `proxies` | 陣列 of `frp.Proxy` |
| `role` | string |
| `server` | `frp.ServerSettings` |
| `warnings` | 陣列 of string |

### frp.Proxy

| 欄位 | 類型 |
| --- | --- |
| `client` | string |
| `customDomains` | 陣列 of string |
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

| 欄位 | 類型 |
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
| `key` | string |
| `name` | string |
| `proxyBindAddr` | string |
| `remark` | string |
| `subdomainHost` | string |
| `token` | string |
| `vhostHttpPort` | integer |
| `vhostHttpsPort` | integer |

### frp.ServerSettings

| 欄位 | 類型 |
| --- | --- |
| `allowPorts` | string |
| `bindAddr` | string |
| `bindPort` | integer |
| `dashboardAddr` | string |
| `dashboardPort` | integer |
| `dashboardPwd` | string |
| `dashboardUser` | string |
| `proxyBindAddr` | string |
| `subdomainHost` | string |
| `token` | string |
| `vhostHttpPort` | integer |
| `vhostHttpsPort` | integer |

### frp.Settings

| 欄位 | 類型 |
| --- | --- |
| `frpcPath` | string |
| `frpsPath` | string |

### frp.SettingsView

| 欄位 | 類型 |
| --- | --- |
| `frpc` | `frp.BinaryView` |
| `frps` | `frp.BinaryView` |
| `settings` | `frp.Settings` |

### geoip.Info

| 欄位 | 類型 |
| --- | --- |
| `buildTime` | 時間 (RFC 3339) |
| `loaded` | boolean |
| `size` | integer |
| `type` | string |

### guard.BanView

| 欄位 | 類型 |
| --- | --- |
| `count` | integer |
| `created` | 時間 (RFC 3339) |
| `jail` | string |
| `key` | string |
| `reason` | string |
| `source` | string |
| `until` | 時間 (RFC 3339) |

### guard.FirewallStatus

| 欄位 | 類型 |
| --- | --- |
| `active` | boolean |
| `checkedAt` | 時間 (RFC 3339) |
| `dropped` | 對應 of integer |
| `enabled` | boolean |
| `error` | string |
| `hits` | 對應 of integer |
| `openPorts` | integer |
| `reason` | string |
| `trapPorts` | string |
| `trapSkipped` | 陣列 of integer |
| `version` | string |

### guard.Jail

| 欄位 | 類型 |
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

| 欄位 | 類型 |
| --- | --- |
| `docker` | boolean |
| `inDocker` | boolean |
| `journal` | boolean |
| `os` | string |

### guard.ScanSettings

| 欄位 | 類型 |
| --- | --- |
| `banSeconds` | integer |
| `enable` | boolean |
| `excludePorts` | string |
| `threshold` | integer |

### guard.Settings

| 欄位 | 類型 |
| --- | --- |
| `banSeconds` | integer |
| `firewall` | boolean |
| `ignoreList` | 陣列 of string |
| `ignorePrivate` | boolean |
| `increment` | boolean |
| `maxBanSeconds` | integer |
| `scan` | `guard.ScanSettings` |
| `trap` | `guard.TrapSettings` |

### guard.Status

| 欄位 | 類型 |
| --- | --- |
| `bans` | integer |
| `firewall` | `guard.FirewallStatus` |
| `platform` | `guard.Platform` |
| `userTrapPorts` | 陣列 of integer |

### guard.TestResult

| 欄位 | 類型 |
| --- | --- |
| `host` | string |
| `ignored` | boolean |
| `line` | string |
| `matched` | boolean |

### guard.TrapSettings

| 欄位 | 類型 |
| --- | --- |
| `banSeconds` | integer |
| `enable` | boolean |
| `ports` | string |

### guard.banRequest

| 欄位 | 類型 |
| --- | --- |
| `key` | string |
| `reason` | string |
| `seconds` | integer |

### guard.testRequest

| 欄位 | 類型 |
| --- | --- |
| `filter` | string |
| `ignore` | string |
| `lines` | string |
| `source` | string |

### ipsource.Source

| 欄位 | 類型 |
| --- | --- |
| `command` | string |
| `enable` | boolean |
| `interface` | string |
| `regex` | string |
| `type` | string |
| `urls` | 陣列 of string |

### nettools.Request

| 欄位 | 類型 |
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

| 欄位 | 類型 |
| --- | --- |
| `deny` | 陣列 of string |

### nettools.Task

| 欄位 | 類型 |
| --- | --- |
| `error` | string |
| `finished` | 時間 (RFC 3339) |
| `id` | string |
| `lines` | 陣列 of string |
| `result` | 任意 |
| `started` | 時間 (RFC 3339) |
| `status` | string |
| `title` | string |
| `tool` | string |

### notify.Channel

| 欄位 | 類型 |
| --- | --- |
| `enable` | boolean |
| `from` | string |
| `group` | string |
| `host` | string |
| `key` | string |
| `kind` | string |
| `name` | string |
| `port` | integer |
| `remark` | string |
| `secret` | string |
| `security` | string |
| `target` | string |
| `url` | string |
| `user` | string |
| `webhook` | `webhook.Config` |

### notify.Record

| 欄位 | 類型 |
| --- | --- |
| `channel` | string |
| `error` | string |
| `ok` | boolean |
| `rule` | string |
| `time` | 時間 (RFC 3339) |
| `title` | string |
| `type` | string |

### notify.Rule

| 欄位 | 類型 |
| --- | --- |
| `channels` | 陣列 of string |
| `enable` | boolean |
| `events` | 陣列 of string |
| `group` | string |
| `key` | string |
| `minLevel` | string |
| `name` | string |
| `silence` | integer |

### portforward.Rule

| 欄位 | 類型 |
| --- | --- |
| `enable` | boolean |
| `group` | string |
| `key` | string |
| `listenAddress` | string |
| `listenPorts` | string |
| `maxConnections` | integer |
| `mode` | string |
| `name` | string |
| `protocols` | 陣列 of string |
| `proxyProtocol` | string |
| `remark` | string |
| `targetAddresses` | 陣列 of string |
| `targetPorts` | string |
| `udpSessionTimeout` | integer |

### protoroute.Route

| 欄位 | 類型 |
| --- | --- |
| `alpn` | 陣列 of string |
| `domains` | 陣列 of string |
| `name` | string |
| `protocol` | string |
| `proxyProtocol` | string |
| `targetAddresses` | 陣列 of string |
| `targetPort` | integer |

### protoroute.Rule

| 欄位 | 類型 |
| --- | --- |
| `enable` | boolean |
| `group` | string |
| `key` | string |
| `listenAddress` | string |
| `listenPorts` | string |
| `maxConnections` | integer |
| `name` | string |
| `protocols` | 陣列 of string |
| `proxyProtocol` | string |
| `remark` | string |
| `routes` | 陣列 of `protoroute.Route` |
| `sniffTimeout` | number |
| `targetAddresses` | 陣列 of string |
| `targetPorts` | string |

### ssl.ACMEConfig

| 欄位 | 類型 |
| --- | --- |
| `ca` | string |
| `challenge` | string |
| `credentials` | `dnsprovider.Credentials` |
| `directoryUrl` | string |
| `disableAuthoritativeCheck` | boolean |
| `domains` | 陣列 of string |
| `eabHmac` | string |
| `eabKid` | string |
| `email` | string |
| `httpPort` | integer |
| `keyType` | string |
| `network` | string |
| `profile` | string |
| `propagationTimeout` | integer |
| `provider` | string |
| `resolvers` | 陣列 of string |
| `skipPropagationCheck` | boolean |
| `zone` | string |

### ssl.Cert

| 欄位 | 類型 |
| --- | --- |
| `acme` | `ssl.ACMEConfig` |
| `certFile` | string |
| `certPem` | string |
| `deploys` | 陣列 of `ssl.Deploy` |
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
| `updatedAt` | 時間 (RFC 3339) |

### ssl.Deploy

| 欄位 | 類型 |
| --- | --- |
| `certName` | string |
| `command` | string |
| `connection` | string |
| `containers` | 陣列 of string |
| `deployedAt` | 時間 (RFC 3339) |
| `deployedHash` | string |
| `dir` | string |
| `enable` | boolean |
| `keyName` | string |
| `remark` | string |
| `type` | string |

### ssl.Mapping

| 欄位 | 類型 |
| --- | --- |
| `dir` | string |
| `enable` | boolean |
| `script` | string |

### storage.Mount

| 欄位 | 類型 |
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

| 欄位 | 類型 |
| --- | --- |
| `enable` | boolean |
| `key` | string |
| `name` | string |
| `readOnly` | boolean |
| `type` | string |

### stun.Bind

| 欄位 | 類型 |
| --- | --- |
| `interface` | string |
| `ip` | string |
| `mode` | string |
| `regex` | string |

### stun.Rule

| 欄位 | 類型 |
| --- | --- |
| `autoRetry` | boolean |
| `bind` | `stun.Bind` |
| `disableCheck` | boolean |
| `disableForward` | boolean |
| `enable` | boolean |
| `group` | string |
| `heartbeatInterval` | integer |
| `keepAliveServers` | 陣列 of string |
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
| `stunServers` | 陣列 of string |
| `stunTimeout` | integer |
| `targetAddresses` | 陣列 of string |
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

### thirdauth.Provider

| 欄位 | 類型 |
| --- | --- |
| `clientId` | string |
| `clientSecret` | string |
| `issuer` | string |
| `key` | string |
| `name` | string |
| `proxy` | string |
| `type` | string |

### waf.Instance

| 欄位 | 類型 |
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
| `ruleSets` | 陣列 of `waf.RuleSet` |
| `skipBodyPaths` | string |
| `webdav` | boolean |

### waf.RuleSet

| 欄位 | 類型 |
| --- | --- |
| `enable` | boolean |
| `param` | string |
| `remark` | string |
| `type` | string |

### webhook.Config

| 欄位 | 類型 |
| --- | --- |
| `body` | string |
| `enable` | boolean |
| `headers` | 陣列 of string |
| `insecureSkipVerify` | boolean |
| `method` | string |
| `successContent` | 陣列 of string |
| `timeout` | integer |
| `url` | string |

### webservice.AuthUser

| 欄位 | 類型 |
| --- | --- |
| `displayName` | string |
| `email` | string |
| `groups` | 陣列 of string |
| `id` | string |
| `name` | string |
| `password` | string |
| `remark` | string |
| `totpSecret` | string |

### webservice.BanInfo

| 欄位 | 類型 |
| --- | --- |
| `key` | string |
| `manual` | boolean |
| `permanent` | boolean |
| `reason` | string |
| `source` | string |
| `until` | 時間 (RFC 3339) |

### webservice.CacheConfig

| 欄位 | 類型 |
| --- | --- |
| `enable` | boolean |
| `maxMb` | integer |
| `ttl` | integer |

### webservice.ForwardAuth

| 欄位 | 類型 |
| --- | --- |
| `copyHeaders` | 陣列 of string |
| `loginUrl` | string |
| `url` | string |

### webservice.HealthCheck

| 欄位 | 類型 |
| --- | --- |
| `enable` | boolean |
| `fails` | integer |
| `interval` | integer |
| `mode` | string |
| `passes` | integer |
| `path` | string |
| `timeout` | integer |

### webservice.IPList

| 欄位 | 類型 |
| --- | --- |
| `count` | integer |
| `enable` | boolean |
| `key` | string |
| `name` | string |
| `refreshHours` | integer |
| `remark` | string |
| `updatedAt` | 時間 (RFC 3339) |
| `url` | string |

### webservice.Listener

| 欄位 | 類型 |
| --- | --- |
| `acceptProxyProtocol` | boolean |
| `default` | `webservice.Site` |
| `enable` | boolean |
| `http3` | boolean |
| `key` | string |
| `listenIp` | string |
| `minTls` | string |
| `name` | string |
| `network` | string |
| `plainHttp` | string |
| `port` | integer |
| `remark` | string |
| `sites` | 陣列 of `webservice.Site` |
| `tls` | boolean |

### webservice.Location

| 欄位 | 類型 |
| --- | --- |
| `prefix` | string |
| `requestHeaders` | 陣列 of string |
| `targets` | 陣列 of string |

### webservice.OIDCClient

| 欄位 | 類型 |
| --- | --- |
| `allow` | 陣列 of string |
| `id` | string |
| `name` | string |
| `public` | boolean |
| `redirectUris` | 陣列 of string |
| `remark` | string |
| `secret` | string |

### webservice.OIDCSettings

| 欄位 | 類型 |
| --- | --- |
| `clients` | 陣列 of `webservice.OIDCClient` |
| `enable` | boolean |
| `issuer` | string |
| `signingKey` | string |

### webservice.PathRule

| 欄位 | 類型 |
| --- | --- |
| `auth` | string |
| `ipList` | 陣列 of string |
| `ipMode` | string |
| `ipSets` | 陣列 of string |
| `prefix` | string |

### webservice.Point

| 欄位 | 類型 |
| --- | --- |
| `bytes` | integer |
| `requests` | integer |
| `s4xx` | integer |
| `s5xx` | integer |
| `t` | integer |

### webservice.RateLimit

| 欄位 | 類型 |
| --- | --- |
| `burst` | integer |
| `enable` | boolean |
| `maxConns` | integer |
| `rate` | number |

### webservice.Rewrite

| 欄位 | 類型 |
| --- | --- |
| `code` | integer |
| `pattern` | string |
| `redirect` | boolean |
| `to` | string |

### webservice.Settings

| 欄位 | 類型 |
| --- | --- |
| `authSecret` | string |
| `authUsers` | 陣列 of `webservice.AuthUser` |
| `autoBanSeconds` | integer |
| `enable` | boolean |
| `oauthCallbackBase` | string |
| `oauthProviders` | 陣列 of `thirdauth.Provider` |
| `oidc` | `webservice.OIDCSettings` |
| `selfServicePassword` | boolean |
| `ssoDomain` | string |
| `trustedProxies` | 陣列 of string |

### webservice.ShareLink

| 欄位 | 類型 |
| --- | --- |
| `expiresAt` | 時間 (RFC 3339) |
| `key` | string |
| `maxUses` | integer |
| `name` | string |
| `pathPrefix` | string |
| `token` | string |

### webservice.Site

| 欄位 | 類型 |
| --- | --- |
| `accessLog` | boolean |
| `authOptional` | boolean |
| `basicAuth` | boolean |
| `cache` | `webservice.CacheConfig` |
| `cleanUrls` | boolean |
| `compress` | boolean |
| `contentType` | string |
| `cors` | boolean |
| `countries` | 陣列 of string |
| `countryMode` | string |
| `denyClose` | boolean |
| `disableKeepAlives` | boolean |
| `domains` | 陣列 of string |
| `enable` | boolean |
| `forwardAuth` | `webservice.ForwardAuth` |
| `forwardedHeaders` | boolean |
| `group` | string |
| `health` | `webservice.HealthCheck` |
| `insecureSkipVerify` | boolean |
| `ipList` | 陣列 of string |
| `ipMode` | string |
| `ipSets` | 陣列 of string |
| `keepPath` | boolean |
| `key` | string |
| `lbPolicy` | string |
| `listDir` | boolean |
| `locations` | 陣列 of `webservice.Location` |
| `name` | string |
| `notFoundPage` | string |
| `oauthUsers` | 陣列 of string |
| `passAuthCookies` | boolean |
| `rateLimit` | `webservice.RateLimit` |
| `redirectCode` | integer |
| `redirectTo` | string |
| `remark` | string |
| `requestHeaders` | 陣列 of string |
| `responseHeaders` | 陣列 of string |
| `rewrites` | 陣列 of `webservice.Rewrite` |
| `root` | string |
| `rules` | 陣列 of `webservice.PathRule` |
| `sendProxyProtocol` | boolean |
| `shareLinks` | 陣列 of `webservice.ShareLink` |
| `sharedUsers` | boolean |
| `statusCode` | integer |
| `targets` | 陣列 of string |
| `text` | string |
| `type` | string |
| `upstreamSniFromHost` | boolean |
| `useTargetHost` | boolean |
| `userHeader` | string |
| `userRoutes` | 陣列 of `webservice.UserRoute` |
| `users` | 陣列 of string |
| `waf` | string |
| `webAuth` | boolean |

### webservice.SiteSharedUsers

| 欄位 | 類型 |
| --- | --- |
| `listener` | string |
| `sharedUsers` | boolean |
| `site` | string |

### webservice.SiteStatsView

| 欄位 | 類型 |
| --- | --- |
| `bytesOut` | integer |
| `series` | 陣列 of `webservice.Point` |
| `status` | 對應 of integer |
| `topCountries` | 陣列 of `webservice.TopItem` |
| `topIps` | 陣列 of `webservice.TopItem` |
| `upstreams` | 陣列 of `webservice.UpstreamStatus` |

### webservice.TopItem

| 欄位 | 類型 |
| --- | --- |
| `count` | integer |
| `key` | string |

### webservice.UpstreamStatus

| 欄位 | 類型 |
| --- | --- |
| `backup` | boolean |
| `conns` | integer |
| `healthy` | boolean |
| `url` | string |
| `weight` | integer |

### webservice.UserRoute

| 欄位 | 類型 |
| --- | --- |
| `targets` | 陣列 of string |
| `users` | 陣列 of string |

### webservice.UserSite

| 欄位 | 類型 |
| --- | --- |
| `basicAuth` | boolean |
| `default` | boolean |
| `domains` | 陣列 of string |
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

| 欄位 | 類型 |
| --- | --- |
| `authUsers` | 陣列 of `webservice.AuthUser` |
| `oidc` | `webservice.OIDCSettings` |
| `selfServicePassword` | boolean |
| `sites` | 陣列 of `webservice.SiteSharedUsers` |
| `ssoDomain` | string |

### webservice.UsersView

| 欄位 | 類型 |
| --- | --- |
| `authUsers` | 陣列 of `webservice.AuthUser` |
| `oidc` | `webservice.OIDCSettings` |
| `selfServicePassword` | boolean |
| `sites` | 陣列 of `webservice.UserSite` |
| `ssoDomain` | string |

### webservice.banRequest

| 欄位 | 類型 |
| --- | --- |
| `key` | string |
| `seconds` | integer |

### webservice.geoipDownload

| 欄位 | 類型 |
| --- | --- |
| `url` | string |

### webservice.totpRequest

| 欄位 | 類型 |
| --- | --- |
| `name` | string |

### webservice.totpResponse

| 欄位 | 類型 |
| --- | --- |
| `secret` | string |
| `url` | string |

### webterminal.Connection

| 欄位 | 類型 |
| --- | --- |
| `enable` | boolean |
| `group` | string |
| `key` | string |
| `local` | `webterminal.LocalConfig` |
| `name` | string |
| `quickAccessDirs` | 陣列 of string |
| `remark` | string |
| `ssh` | `webterminal.SSHConfig` |
| `telnet` | `webterminal.TelnetConfig` |
| `type` | string |

### webterminal.Credential

| 欄位 | 類型 |
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

| 欄位 | 類型 |
| --- | --- |
| `env` | 對應 of string |
| `shell` | string |
| `workDir` | string |

### webterminal.Proxy

| 欄位 | 類型 |
| --- | --- |
| `enabled` | boolean |
| `host` | string |
| `password` | string |
| `port` | integer |
| `type` | string |
| `username` | string |

### webterminal.SSHConfig

| 欄位 | 類型 |
| --- | --- |
| `authType` | string |
| `credentialKey` | string |
| `host` | string |
| `hostKey` | string |
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

| 欄位 | 類型 |
| --- | --- |
| `host` | string |
| `password` | string |
| `port` | integer |
| `proxy` | `webterminal.Proxy` |
| `timeout` | integer |
| `username` | string |

### wol.Device

| 欄位 | 類型 |
| --- | --- |
| `broadcastIps` | 陣列 of string |
| `checkAddr` | string |
| `enable` | boolean |
| `group` | string |
| `key` | string |
| `macs` | 陣列 of string |
| `name` | string |
| `port` | integer |
| `remark` | string |
| `repeat` | integer |

