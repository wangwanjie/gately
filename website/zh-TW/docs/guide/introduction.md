# 專案簡介

Gately 是開源、可稽核的網路閘道工具集，家用網路、NAS、雲端主機與各類伺服器都能使用。DDNS、STUN 穿透、連接埠轉發、反向代理、憑證、WAF 等常用功能整合在一個約 32 MB 的單一檔案中。

## 為什麼做 Gately

Gately 把 DDNS、STUN 穿透、反向代理等能力整合在一起，並選擇完全開源，讓使用者和社群可以直接檢視原始碼與設定。對於執行在網路邊界、掌握 DNS 金鑰和憑證私鑰的軟體，開源讓使用者能夠看到它在做什麼。Gately 的承諾：

- 原始碼以 Apache-2.0 開源，接受社群審查
- 預設不連線任何專案方伺服器，不回報裝置資訊；第三方登入使用你自己的 OAuth 應用程式，不經過中轉
- 所有對外位址（公網 IP 查詢、STUN 伺服器等）都看得見、改得了、關得掉
- 設定為純文字 JSON，可直接納入 git 做稽核和回復

## 設計原則

1. **透明**：預設不連線任何專案方伺服器，所有對外位址都看得見、改得了、關得掉。
2. **可遷移**：可匯入現有 nginx 網站等既有設定，匯入的規則預設停用，便於逐步切換。
3. **可稽核的設定**：設定為純文字 JSON（檔案權限 0600），可直接納入 git 做 diff 和回復。
4. **單一檔案部署**：一個靜態執行檔內嵌前端，支援 Docker、systemd、OpenWrt。

## 技術堆疊

| 層 | 選型 |
| --- | --- |
| 後端 | Go，靜態編譯，可交叉編譯到 arm / arm64 / mips / riscv 路由器與 NAS |
| HTTP | 標準函式庫 `net/http` |
| 儲存 | 每個模組一個純文字 JSON 檔案，原子寫入 |
| 驗證 | bcrypt 密碼 + HMAC 簽章工作階段權杖，登入失敗限流，TOTP 兩步驟驗證 |
| 前端 | Vue 3 + TypeScript + Vite + Element Plus，透過 `go:embed` 打包進執行檔 |

## 功能一覽

動態網域、STUN 穿透、連接埠轉發、協定分流、SSL 憑證、Web 服務（反向代理、存取控制、健康檢查、快取與限流、存取統計、OIDC 單一登入）、WAF 防火牆、入侵防護、連接埠敲門、Tailscale、frp、Cloudflare 通道、Docker 管理、Web 終端機、網路診斷、網路喚醒、WebDAV、FTP、SMB、File Browser（託管 FileBrowser Quantum 網頁檔案管理）、儲存掛載、排程工作、通知中心、設定備份，以及兩步驟驗證、安全入口、API 權杖、命令列與 MCP。詳見 [模組總覽](/zh-TW/docs/modules/)。

## 下一步

- 想馬上試用：[快速開始](/zh-TW/docs/guide/quick-start)
- 從現有工具遷移：[遷移指南](/zh-TW/docs/guide/migration)
- 關心隱私與安全：[安全模型與隱私承諾](/zh-TW/docs/guide/security)
