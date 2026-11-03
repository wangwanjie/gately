# Gately 技術方案與路線圖

## 目標

打造一個開源、可稽核、社群驅動的網路閘道工具：涵蓋家用網路、NAS 與伺服器使用者最常用的 DDNS、STUN 穿透、連接埠轉發、反向代理、憑證管理，並支援匯入既有的 Lucky 設定。

設計原則：

1. **透明**：預設不連線任何專案方伺服器；所有對外位址皆可檢視、可修改、可關閉。
2. **可遷移**：提供 Lucky `.lkcf` 設定匯入。
3. **可稽核的設定**：設定為明文 JSON（檔案權限 0600），可直接納入 git 做 diff 與回溯。
4. **單一檔案部署**：一個靜態二進位檔內嵌前端，支援 Docker、systemd、OpenWrt；可選模組與介面語言可在編譯時去掉，產生更小的自訂建置。

## 技術堆疊

| 層 | 選型 | 理由 |
| --- | --- | --- |
| 後端 | Go 1.26+ | 可直接重用 lego、pion/stun、frp、cloudflared、coraza、rclone 等成熟函式庫；靜態編譯，交叉編譯到 arm/arm64/mips/riscv 路由器與 NAS |
| HTTP | 標準函式庫 `net/http`（Go 1.22 路由語法） | 相依最少，便於稽核 |
| 儲存 | 每個模組一個明文 JSON 檔案，原子寫入 | 便於匯入、備份與 git diff |
| 驗證 | bcrypt 密碼 + HMAC 簽章工作階段權杖，登入失敗限流，後續支援 TOTP | 無外部相依 |
| 日誌 | 每個模組記憶體環形緩衝 + JSON Lines 批次落盤（依大小輪替，重新啟動後還原）+ 增量拉取介面 | 日誌輪詢介面簡單，前端增量重新整理 |
| 前端 | Vue 3 + TypeScript + Vite + Element Plus，`go:embed` 打包 | 元件豐富，開發效率高 |
| 發布 | GoReleaser 多架構二進位檔 + 多架構 Docker 映像檔 | — |

## 目錄結構

```
cmd/gately/            程式進入點
internal/app/          組裝：載入設定、啟動模組、HTTP 服務
internal/config/       模組化 JSON 設定儲存
internal/auth/         管理員驗證與工作階段
internal/logbuf/       模組日誌環形緩衝
internal/httpx/        HTTP 工具與中介軟體
internal/module/       模組介面與註冊表
internal/modules/*/    各功能模組
internal/modules/registry/  模組註冊：可選模組各自一個 mod_<模組>.go（build tag no_<模組>），跨模組能力經注入提供
tools/custombuild/     自訂建置：依 modules.json 換算 build tag、語言與 Docker 建置參數並打包
modules.json           可選模組、相依、體積估算、語言檔位、建置目標與預設組合（build.gately.cn 讀取）
internal/luckyimport/  Lucky .lkcf 解密與欄位對應
web/                   前端專案，建置產物由 web/embed.go 嵌入
```

## 里程碑

依使用者實際使用規模排序（STUN 18、連接埠轉發 25、反向代理 27、憑證 5、DDNS 75 筆記錄）。

| 階段 | 內容 | 驗收 |
| --- | --- | --- |
| ✅ **M0 骨架** | 設定儲存、管理員登入、模組框架、日誌緩衝、管理 UI 外殼、Dockerfile | 登入後能看到總覽與模組選單 |
| ✅ **M1 連接埠轉發** | TCP/UDP、v4/v6、多連接埠與連接埠範圍、多目標、連線數限制、啟停與日誌；匯入 Lucky 連接埠轉發規則 | 單元測試涵蓋 TCP/UDP 轉發；真實樣本 25/25 條匯入通過 |
| ✅ **M2 DDNS** | 取得 IP（URL/網路卡/指令）、IPv4/IPv6、阿里雲/Cloudflare/DNSPod(騰訊雲)/華為雲/自訂回呼、Webhook；匯入 | 11 家服務商（阿里雲簽章經官方範例校驗）；真實樣本 2/2 個任務、75 筆記錄匯入通過 |
| ✅ **M3 STUN 穿透** | STUN 取得公網對應、TCP 保活、UPnP/NAT-PMP、內建轉發、Webhook 與腳本變數；匯入 | 本機端對端測試 + 家用網路真實穿透；真實樣本 18/18 條匯入通過 |
| ✅ **M4 憑證** | ACME（lego，DNS-01 重用 M2 Provider）、自動續期、匯出到路徑、變更腳本 | Pebble 端對端（萬用字元 + 續簽）；真實樣本 5/5 張匯入且無需重新簽發 |
| ✅ **M5 Web 服務** | 反向代理、重新導向、靜態檔案、依網域路由、自動 TLS、BasicAuth、IP 過濾；匯入 | 涵蓋路由、4 種站台、驗證、WebSocket、HTTPS、PROXY 協定收發；真實樣本 27 個站台匯入通過 |
| ✅ M6 維運 | 網路喚醒、Web 終端機（SSH/本機 Shell）、排程工作、管理員兩步驟驗證 | WOL 3/3、終端機連線 14/14 匯入；TOTP 通過 RFC 6238 測試向量 |
| ✅ M7 生態（部分） | Coraza WAF、GitHub/Google/QQ/OIDC 第三方登入（站台驗證與管理後台登入，可遷移 Lucky 身分表）、Docker 管理、WebDAV、FTP 服務、儲存管理（本機/遠端 WebDAV 掛載與線上檔案管理） | WAF 以真實攻擊酬載測試；Docker 在真實環境唯讀驗證 |
| ✅ M7.5 組網穿透 | Tailscale、NetBird、frp（frpc、frps 皆可多實例）、cloudflared，以行程託管方式整合官方二進位檔 | 設定產生、行程守護與日誌收集有單元測試；不內建上游程式 |
| ✅ M8 可信與維運 | 操作稽核日誌、設定歷史與回溯、宣告式設定目錄；Passkey 登入與唯讀帳號；ACME HTTP-01；HTTP/3；全域 Prometheus 指標與 Grafana 面板；網路自我檢測與首次啟動引導；DDNS 新增 Route 53、OVH、Gandi、NS1、Bunny、Namecheap、DynDNS2；通知新增 ntfy、Gotify、Pushover、Discord、Slack、Server酱、PushPlus 與規則範本；發布產物 cosign 簽章、建置來源證明與 SBOM；systemd 與 OpenWrt 服務範本 | WebAuthn 以軟體驗證器端對端測試；Route 53 簽章經 AWS 官方 SigV4 測試向量校驗；HTTP/3 以 quic-go 用戶端實測 |
| ✅ M8.5 安裝與升級 | 飛牛 fnOS 原生應用（.fpk，接入統一閘道與飛牛登入狀態）；Linux 二進位檔線上升級；OpenWrt 安裝套件（24.10 及以前的 .ipk、25.12 起的 .apk） | fpk 在飛牛虛擬機實測；OpenWrt 安裝套件在 24.10 / 25.12 官方 rootfs 容器與 iStoreOS 實測，CI 冒煙測試涵蓋安裝與移除 |
| ✅ M8.6 遷移與維運 | 匯入 nginx 設定（`nginx -T`，含 include 展開、upstream、Authelia 的 `auth_request`）；憑證自動部署（經 SSH 推送並執行指令、重新啟動 Docker 容器）；WAF 請求內容檢查上限、超過上限只檢查前段與依路徑略過；訂閱 IP 清單（定時重新整理、站台與路徑規則引用）；自檢新增 HTTPS 站台憑證涵蓋與 frp 代理衝突；線上升級支援四段版本號 | nginx 匯入以真實正式環境設定（77 個站台、Authelia、upstream）驗證；SSH 推送以 SFTP 測試伺服器端對端測試；WAF 以 2 MB 請求內容驗證拒絕、只檢查前段與略過路徑 |
| ✅ M8.7 入侵防護 | 類 fail2ban 的日誌規則（日誌檔案、systemd 日誌、命令輸出、Docker 容器日誌、管理後台登入失敗、WAF 攔截）與常見服務範本；誘餌連接埠與連接埠掃描偵測；統一封鎖清單（合併 Web 服務的驗證失敗封鎖，持久保存、遞增時長、手動網段與永久封鎖、白名單）；連接埠轉發、Web 服務、STUN、WebDAV、FTP 與管理後台都拒絕被封鎖的位址；封鎖同步到 nftables（獨立的 `inet gately` 表）在核心層丟棄 | 在 fnOS（nftables 1.0.6）與 iStoreOS 24.10（nftables 1.1.1）的獨立網路命名空間中驗證誘餌連接埠、掃描偵測（IPv4/IPv6）、四種日誌來源、持久保存與規則自我修復；Docker 容器驗證權限偵測與使用者態誘餌連接埠回退 |
| M9 待定 | 多管理員與細粒度角色 | 視社群需求排程 |

## 設計取捨

- 線上升級只在 Linux 二進位檔部署中提供，且只由管理員手動觸發：下載後核對 checksums.txt 的 SHA-256 並試執行新程式，替換時保留舊版本；Docker、飛牛 fnOS 與 OpenWrt 安裝套件仍由映像檔、應用中心與套件管理器負責升級。自訂建置不提供線上升級（會裝回完整版本），在 build.gately.cn 以相同選項重新產生後上傳升級包。
- 不下發遠端設定，不回報裝置資訊。
- 設定以明文儲存；敏感欄位在 API 輸出中遮罩。
