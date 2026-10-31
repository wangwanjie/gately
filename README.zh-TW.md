<p align="center">
  <a href="https://gately.cn/zh-TW/"><img src="docs/images/banner.zh-TW.svg" alt="Gately — 一站式網路閘道，開源透明，開箱即用" width="100%"></a>
</p>

<p align="center">
  <a href="https://github.com/wangwanjie/gately/releases/latest"><img src="https://img.shields.io/github/v/release/wangwanjie/gately?style=flat-square&color=0e7c66&label=release" alt="Release"></a>
  <a href="https://github.com/wangwanjie/gately/pkgs/container/gately"><img src="https://img.shields.io/badge/docker-ghcr.io%2Fwangwanjie%2Fgately-0e7c66?style=flat-square&logo=docker&logoColor=white" alt="Docker"></a>
  <a href="LICENSE"><img src="https://img.shields.io/github/license/wangwanjie/gately?style=flat-square&color=0e7c66" alt="License"></a>
  <a href="https://github.com/wangwanjie/gately/stargazers"><img src="https://img.shields.io/github/stars/wangwanjie/gately?style=flat-square&color=0e7c66" alt="Stars"></a>
  <a href="https://gately.cn/zh-TW/"><img src="https://img.shields.io/badge/%E5%AE%98%E7%B6%B2-gately.cn-0e7c66?style=flat-square&logo=googlechrome&logoColor=white" alt="gately.cn"></a>
  <a href="https://t.me/gately_chat"><img src="https://img.shields.io/badge/Telegram-gately__chat-26A5E4?style=flat-square&logo=telegram&logoColor=white" alt="Telegram"></a>
</p>

<p align="center">
  <a href="https://gately.cn/zh-TW/"><b>官網</b></a> &nbsp;·&nbsp; <a href="https://gately.cn/zh-TW/docs/guide/quick-start"><b>快速開始</b></a> &nbsp;·&nbsp; <a href="https://gately.cn/zh-TW/docs/guide/install"><b>安裝指南</b></a> &nbsp;·&nbsp; <a href="https://gately.cn/zh-TW/docs/guide/introduction"><b>文件</b></a> &nbsp;·&nbsp; <a href="https://github.com/wangwanjie/gately/releases/latest"><b>下載</b></a> &nbsp;·&nbsp; <a href="https://gately.cn/zh-TW/news/"><b>更新日誌</b></a>
</p>

<p align="center">
  <a href="https://gately.cn/zh-TW/sponsor"><img src="https://img.shields.io/badge/%E8%AB%8B%E6%88%91%E5%96%9D%E6%9D%AF%E5%92%96%E5%95%A1-Gately-FFDD00?style=for-the-badge&logo=buymeacoffee&logoColor=black" alt="請我喝杯咖啡" height="34"></a>
</p>

<p align="center">
  <a href="README.md">English</a> | <a href="README.zh-CN.md">简体中文</a> | <b>繁體中文</b> | <a href="README.ja.md">日本語</a> | <a href="README.ko.md">한국어</a> | <a href="README.es.md">Español</a> | <a href="README.fr.md">Français</a> | <a href="README.de.md">Deutsch</a> | <a href="README.ru.md">Русский</a> | <a href="README.pt-BR.md">Português (Brasil)</a>
</p>

從網域解析、內網穿透，到反向代理、HTTPS 憑證；從 WAF、入侵防護，到統一登入與檔案分享，家用網路和伺服器入口需要的能力，Gately 都整合在同一個後台。裝在 NAS、軟路由或雲端主機上，打開瀏覽器就能設定和維運。原始碼以 Apache-2.0 授權開源，設定和資料只留在你自己的裝置上，不蒐集、不回報。

> **飛牛 fnOS 使用者**：Gately 提供飛牛原生應用。它接入**飛牛統一閘道**，點擊飛牛桌面圖示即可開啟，遠端存取飛牛的網域同樣可用，無需額外開放連接埠；開啟後還支援**飛牛管理員登入狀態直接登入**，無需再輸入 Gately 密碼與兩步驟驗證碼。詳見[飛牛原生應用](https://gately.cn/zh-TW/docs/guide/install#fnos)。

## 快速開始

三步就能用：啟動容器，開啟 `http://<裝置IP>:16680`，用 `admin` / `gately.cn` 登入。其他平台的裝法見下方，完整說明見[安裝指南](https://gately.cn/zh-TW/docs/guide/install)。

### Docker

映像檔支援 amd64、arm64、arm/v7：

```bash
docker run -d --name gately --network host --cap-add NET_ADMIN \
  -v ./gately-data:/etc/gately \
  -v /var/run/docker.sock:/var/run/docker.sock \
  ghcr.io/wangwanjie/gately:latest
```

開啟 `http://<裝置IP>:16680`，使用預設帳號 `admin` 與預設密碼 `gately.cn` 登入，然後在「設定」中修改密碼並開啟兩步驟驗證。完整的 Compose 範例見 [deploy/docker-compose.yml](deploy/docker-compose.yml)。

忘記密碼或遺失兩步驟驗證器時，在伺服器上執行 `gately reset-password` 或 `gately reset-totp`（Docker 為 `docker exec -it gately gately reset-password`），確認後立即生效，詳見[重設密碼與兩步驟驗證](https://gately.cn/zh-TW/docs/guide/install#reset)。

連接埠轉發、STUN、Web 服務需要監聽主機連接埠，建議使用 `--network host`。不需要 Docker 管理時可拿掉 `docker.sock` 掛載。`--cap-add NET_ADMIN` 供入侵防護把封鎖寫入主機的 nftables 防火牆；不加時封鎖仍對 Gately 自身的服務生效。

想換個方式安裝？從下面挑一個。

<details>
<summary><b>二進位檔</b></summary>

從 [Releases](https://github.com/wangwanjie/gately/releases) 下載對應平台的壓縮檔（Linux amd64/arm64/arm/mips/mipsle/riscv64、macOS、Windows、FreeBSD），解壓縮後執行：

```bash
./gately -c /etc/gately
```

參數：`-c` 設定目錄（也可用環境變數 `GATELY_CONFIG_DIR`），`-listen` 覆寫管理介面監聽位址，`-apply` 每次啟動時將目錄中的 `<名稱>.json` 覆寫到設定目錄（環境變數 `GATELY_APPLY_DIR`，適合 GitOps 與 Kubernetes；帳號與權杖檔案不允許覆寫），`-version` 輸出版本。無人值守安裝時可設定環境變數 `GATELY_INITIAL_PASSWORD` 指定管理員初始密碼，取代預設密碼 `gately.cn`。

服務範本：[systemd](deploy/systemd/gately.service)、[OpenWrt procd](deploy/openwrt/gately.init)。驗證下載的壓縮檔與映像檔（cosign 簽章、建置來源證明、SBOM）請見[驗證發布產物](docs/zh-TW/VERIFY.md)。

</details>

<details>
<summary><b>飛牛原生應用</b></summary>

每個 Release 附帶飛牛 fnOS 的離線安裝包 `gately_<版本>_fnos_x86.fpk`（Intel/AMD）與 `gately_<版本>_fnos_arm.fpk`（ARM）。它們沒有上架官方應用中心，需要手動安裝：開啟 **應用中心 → 手動安裝**（左下角按鈕），選擇檔案並完成精靈。

- **支援飛牛統一閘道**：點擊飛牛桌面上的 Gately 圖示，管理介面經飛牛統一閘道在 `/app/gately/` 下開啟，與飛牛共用同一個存取入口。區域網路位址、遠端存取飛牛的網域都能直接使用，無需額外開放或對應 16680 連接埠。經閘道存取時由飛牛負責存取控制（只有飛牛管理員能開啟），安全入口與「僅允許內網存取」只約束直接存取 16680 連接埠。
- **支援飛牛管理員登入狀態直接登入**：在 **設定 → 存取控制 → 飛牛登入狀態自動登入** 開啟後，已登入飛牛的管理員點擊桌面圖示即可進入 Gately，無需密碼與兩步驟驗證碼；一般飛牛使用者仍需登入。主動登出後，登入頁會顯示「使用飛牛帳號 &lt;使用者名稱&gt; 登入」按鈕，不會靜默重新登入。該開關預設關閉，只在飛牛安裝包中顯示。
- 區域網路內也可直接存取 `http://<NAS 位址>:16680`。預設帳號 `admin`，預設密碼 `gately.cn`。
- 應用以 `root` 身分執行，因此可以監聽 80/443、透過 `docker.sock` 管理 Docker、使用真實的 Web 終端機。請只安裝在自己掌控的 NAS 上。
- 設定儲存在應用的 `var` 目錄，解除安裝時預設保留；在解除安裝精靈中選擇「全部刪除」才會清除。升級時手動安裝新版 `.fpk` 即可。
- 安裝包同時提供命令列入口 `gately`（`/usr/local/bin/gately`），可使用 `gately ctl` 與 `gately mcp`。
- 16680 連接埠與 Docker 映像檔相同，啟動原生應用前請先停止 `gately` 容器。
- 下載後可依[驗證發布產物](docs/zh-TW/VERIFY.md)驗證，`.fpk` 的 SHA-256 同樣列在 `checksums.txt`。

</details>

<details>
<summary><b>OpenWrt</b></summary>

每個 Release 附帶 OpenWrt 安裝包：`gately_<版本>_openwrt_<架構>.ipk` 用於 OpenWrt 24.10 及更早版本（含 iStoreOS），以 `opkg install` 安裝；`.apk` 用於 OpenWrt 25.12 起，以 `apk add --allow-untrusted` 安裝。架構有 amd64 / arm64 / arm / mipsle / mips / riscv64，依 `/etc/openwrt_release` 中的 `DISTRIB_ARCH` 選擇，對照表見[安裝文件](https://gately.cn/zh-TW/docs/guide/install#openwrt)。

- 安裝後自動啟用開機自動啟動並啟動服務，管理介面為 `http://<路由器位址>:16680`，預設帳號 `admin`，預設密碼 `gately.cn`。
- 設定位於 `/etc/gately`，升級、解除安裝與刷寫韌體（sysupgrade）時都會保留。升級時安裝新版安裝包即可，「關於」頁的線上升級會提示改用套件管理器。
- 程式約 32 MB，適合 x86 軟路由、ARM64 盒子等儲存空間較大的裝置；arm 安裝包需要 ARMv7 且具備硬體浮點運算。
- 另附 LuCI 外掛 `luci-app-gately_<版本>_openwrt_all.ipk` / `.apk`（與架構無關），安裝後在 LuCI「服務 → Gately」中查看執行狀態、啟停服務、設定開機自動啟動與管理介面連接埠，並一鍵開啟管理介面。

</details>

<details>
<summary><b>監控</b></summary>

建立一組僅具 `metrics:read` 權限的 API 權杖，讓 Prometheus 抓取 `/api/metrics`。指標清單、告警範例與 Grafana 儀表板請見[監控指標](docs/zh-TW/MONITORING.md)。

</details>

<details>
<summary><b>API 呼叫</b></summary>

在管理後台「API 權杖」頁建立權杖（可建立多組，依模組分別授予唯讀或讀寫，可設定到期時間，建立後明文只會顯示一次），腳本即可批次呼叫管理介面：

```bash
curl -H "Authorization: Bearer gtl_xxxx" http://localhost:16680/api/ddns/tasks
```

帳號、兩步驟驗證、權杖管理以及備份、還原、重啟、系統設定介面只能使用登入工作階段。全部介面、所需權限與資料結構見管理後台「API 文件」頁，也可以用 `make api-docs` 匯出到 [docs/api](docs/api/zh-TW)（`openapi.json` 與 `API.md`），官網 https://gately.cn 發布時直接引用。

從舊版升級時，原本的全域 API 權杖會自動變成一筆「舊版權杖」（值不變，擁有全部模組權限），建議在 API 權杖頁收緊權限或換成新權杖。

</details>

<details>
<summary><b>Tailscale、frp、cloudflared</b></summary>

這三個模組託管官方的外部程式（自動重啟、日誌彙整到介面），Gately 本體不內建它們。各模組頁面的「線上安裝」卡片從上游下載最新正式版或指定版本，校驗 SHA-256 後安裝到設定目錄的 `bin/`，並重啟使用它的處理程序；可以為每個程式單獨設定下載代理，留空時使用設定中的更新代理。替換下來的版本保留最近 3 個，新版本有問題時可一鍵回滾；不再需要時可以解除安裝程式。頁首的「恢復預設設定」與解除安裝分開，會停止處理程序、清空模組設定並刪除 Gately 為它產生的檔案（frp 設定檔、File Browser 實例資料、Tailscale 登入狀態），舊設定可在設定歷史中還原。Tailscale 官方只為 Linux 提供可直接執行的程式套件，其他系統請用官方安裝程式並填寫路徑。程式依下列順序尋找：模組設定中填寫的路徑、設定目錄下的 `bin/`、`PATH`。也可以自行把二進位檔放進設定目錄，Docker 部署時例如 `./gately-data/bin/frpc`；Tailscale 需要同時提供 `tailscale` 與 `tailscaled`。設定檔中的權杖與金鑰以 0600 權限儲存，cloudflared 的權杖透過環境變數傳遞，不會出現在處理程序清單中。

File Browser 模組託管 [FileBrowser Quantum](https://github.com/gtsteffaniak/filebrowser)（原版 filebrowser 已封存、不再修復安全問題），尋找順序與線上安裝、更新、回滾同上，安裝位置為 `bin/filebrowser`。管理員密碼透過環境變數傳遞，不寫入設定檔。

</details>

## 功能

先看幾張管理介面的截圖，完整功能清單在本節最後。

### 一眼看清全局

概覽頁集中顯示 CPU、記憶體、磁碟與流量，網路卡及其公網 IPv4 / IPv6，以及每個模組的執行狀態，哪裡需要處理一目了然。

<img src="docs/images/overview.webp" alt="一眼看清全局" width="100%">

### 能取代 nginx 的 Web 服務

依網域做反向代理、靜態檔案和重新導向，支援 HTTPS 與 HTTP/3。每個站點可以單獨啟用 WAF、IP 規則和登入驗證，改設定不會中斷現有連線。

<img src="docs/images/web.webp" alt="能取代 nginx 的 Web 服務" width="100%">

<p><b>站點統計</b> — 按分鐘統計請求數、錯誤、後端健康狀態，以及來源 IP 與國家排行。</p>

<img src="docs/images/site-stats.webp" alt="站點統計" width="100%">

### 一個 443 連接埠，所有服務共用

連接埠轉發的 SNI 分流依網域把多個服務放在同一個連接埠上，TLS 原樣透傳，再用 PROXY 協定保留真實用戶端位址。

<img src="docs/images/portforward-sni.webp" alt="一個 443 連接埠，所有服務共用" width="100%">

### 動態網域，主流服務商都支援

Cloudflare、阿里雲、DNSPod、Route 53 等都能用。支援 IPv4 與 IPv6，IP 可以從 URL、網路卡或指令取得，位址一變就發通知。

<img src="docs/images/ddns.webp" alt="動態網域，主流服務商都支援" width="100%">

### 入侵防護

類 fail2ban 的規則讀取日誌檔案、journald 和容器日誌，誘餌連接埠與掃描偵測揪出探測行為。封鎖同步到 nftables，直接在核心裡丟棄封包。

<img src="docs/images/guard.webp" alt="入侵防護" width="100%">

### frp、Tailscale 與 Cloudflare 通道

多個 frpc、frps 實例同時執行，和 Tailscale、cloudflared 通道放在一起管理。這些程式的安裝、更新與回滾都交給 Gately 處理。

<img src="docs/images/frp.webp" alt="frp、Tailscale 與 Cloudflare 通道" width="100%">

### 還有這些

<table>
<tr>
<td width="50%" valign="top">

**Docker**<br>容器、Compose、日誌與終端機，映像檔一鍵升級，失敗自動回滾。

<img src="docs/images/docker.webp" alt="Docker" width="100%">

</td>
<td width="50%" valign="top">

**Web 終端機**<br>在瀏覽器裡使用 SSH、Telnet 和本機 Shell，支援 SFTP 與分割視窗。

<img src="docs/images/terminal.webp" alt="Web 終端機" width="100%">

</td>
</tr>
</table>

<table>
<tr>
<td width="50%" valign="top">

**設定歷史**<br>每次修改都保留舊版本，可以比對差異、一鍵回滾。

<img src="docs/images/config-history.webp" alt="設定歷史" width="100%">

</td>
<td width="50%" valign="top">

**API、命令列與 MCP**<br>依模組授權的 API 權杖、內建 OpenAPI 文件、`gately ctl`，以及供 AI 助理接入的 MCP 服務。

<img src="docs/images/api-docs.webp" alt="API、命令列與 MCP" width="100%">

</td>
</tr>
</table>

<details>
<summary><b>完整功能清單</b></summary>

| 模組 | 功能 |
| --- | --- |
| 動態網域 | Alibaba Cloud、Cloudflare、Tencent Cloud、DNSPod、Huawei Cloud、GoDaddy、Porkbun、dynv6、DuckDNS、Amazon Route 53、OVHcloud、Gandi、NS1、Bunny DNS、Namecheap、DynDNS2（No-IP、Dynu 等）、自訂回呼；以 URL／網路卡／指令取得 IP；IPv4/IPv6；Webhook；分組、排序與搜尋 |
| STUN 穿透 | TCP/UDP、保活、UPnP/NAT-PMP、內建轉發、位址變更時觸發 Webhook 與腳本 |
| 連接埠轉發 | TCP/UDP、IPv4/IPv6、連接埠範圍、多目標故障轉移、連線數限制、流量統計、PROXY 協定（v1/v2）；SNI 分流，讓多個服務共用 443 等同一個連接埠（依網域 TLS 透傳、預設目標、每條路由可單獨設定 PROXY 協定）；分組、排序與搜尋 |
| SSL 憑證 | ACME 自動簽發與續期，支援 DNS-01（萬用字元；委派子網域依 SOA 自動識別 DNS 區域）與 HTTP-01（無需 DNS 服務商）、短期憑證與公網 IP 位址憑證（Let's Encrypt shortlived 設定檔）、EAB、上傳、讀取檔案、對應到目錄並執行腳本；憑證更新後經 SSH 推送到其他主機並執行指令，或重新啟動使用憑證的 Docker 容器 |
| Tailscale | 託管 tailscaled（使用者空間網路，容器友善）或接入系統既有的常駐程式；驗證金鑰／登入連結、Headscale、子網路由、出口節點、節點清單與 SOCKS5 代理、固定連接埠、tailscaled 額外參數與環境變數；Linux 上可線上安裝、更新、回滾與解除安裝，可恢復預設設定 |
| frp | 可同時執行多個 frpc 與 frps 實例；frpc 用戶端（tcp/udp/http/https/stcp/xtcp 代理，新增刪除即時生效）與 frps 伺服器端（連接埠白名單、子網域、主控台），可一鍵匯入舊版 ini／新版 toml 設定；線上安裝、更新、回滾與解除安裝，可恢復預設設定 |
| Cloudflare 通道 | cloudflared 權杖通道與 trycloudflare 臨時通道，多通道、自動重啟、公開位址顯示；線上安裝、更新、回滾與解除安裝，可恢復預設設定；權杖通道可透過 Cloudflare API 編輯路由規則並建立 DNS 記錄 |
| Web 服務 | 依網域反向代理／重新導向／靜態檔案／固定文字／中斷連線（相當於 nginx return 444）；HTTPS 與 HTTP/3（QUIC）、HTTP 與 HTTPS 共用同一埠（跳轉或直接服務）；WebSocket；PROXY 協定收發；基本驗證、網頁驗證、GitHub/Google/QQ/OIDC 登入、支援 TOTP 二次驗證並可跨子網域單一登入的共用使用者、內建 OIDC 身分提供者（讓 Gitea、Grafana 等應用程式以共用使用者登入）、轉發認證到 Authelia 等外部服務、公網 Gately 可把登入 Cookie 轉發給內網 Gately；IP 黑白名單與定時重新整理的訂閱 IP 清單（如省市 IP 段）、路徑規則、臨時分享連結、驗證失敗自動封鎖、GeoIP 國家過濾；依路徑分流後端、依登入使用者分流到不同後端（類似 nginx map $remote_user）、HTTPS 後端依造訪網域傳送 SNI、後端權重／備援／負載平衡策略、主動與被動健康檢查、壓縮、限流、回應快取、路徑改寫；靜態站台支援無副檔名網址與自訂 404 頁面；修改站台不會中斷既有連線；站台分組、排序與搜尋；分鐘級存取統計與 Prometheus 指標；共用使用者可自助修改密碼；存取被拒絕時站點可直接中斷連線，不回傳 403 |
| WAF 防火牆 | Coraza + OWASP CRS v4，可依檔案啟用規則、自訂 SecLang、攔截／僅偵測；請求內容檢查上限可調，超過上限時只檢查前段或依路徑略過，網盤、WebDAV 等上傳站台也能掛上 WAF |
| 入侵防護 | 類 fail2ban 的日誌規則（日誌檔案、systemd 日誌、命令輸出、Docker 容器日誌、管理後台登入失敗、WAF 攔截），內建 SSH、Nginx、Vaultwarden 等範本；誘餌連接埠與連接埠掃描偵測；統一封鎖清單，支援手動網段與永久封鎖、遞增時長與白名單；封鎖同步到系統防火牆（nftables）在核心層丟棄，沒有 nftables 的平台對 Gately 自身的服務生效；Web 路徑探測封鎖：按監聽統計誘餌路徑（/.env、/.git 等）、未知網域掃描與連續 404 |
| Docker 管理 | 容器建立與編輯、日誌與終端機、Compose、映像檔更新檢查與一鍵升級（失敗自動還原）、網路與磁碟區 |
| Web 終端機 | SSH（主機金鑰首次信任）、Telnet、本機 Shell（預設關閉）；代理、持久工作階段、SFTP 檔案管理、2FA 與協定確認、快捷指令、分割畫面 |
| 網路診斷 | Ping、路由追蹤、連接埠探測、DNS 查詢、測速、區域網路掃描 |
| 通知中心 | Webhook、Telegram、Bark、ntfy、Gotify、Pushover、Discord、Slack、Server酱、PushPlus、WeCom、DingTalk、Feishu、電子郵件；等級、萬用字元與靜音去重規則；內建規則範本（關鍵故障、憑證到期、位址變更、登入、設定變更） |
| 設定備份 | 加密備份（scrypt + AES-GCM）、保留策略、定時備份到本機目錄或儲存掛載點 |
| 命令列與 MCP | `gately ctl` 呼叫執行中的實例，`gately mcp` 以 stdio 提供 MCP 服務（預設唯讀） |
| 網路喚醒 | 魔術封包廣播、上線探測 |
| WebDAV | 多使用者、獨立目錄、唯讀；WebDAV、FTP、SMB 共用一份共享使用者 |
| FTP | 多使用者、獨立目錄、唯讀，支援 FTPS 與被動連接埠範圍；WebDAV、FTP、SMB 共用一份共享使用者 |
| SMB | 多使用者、依使用者分享多個目錄、唯讀，支援 SMB 2/3、加密與 Bonjour 探索；WebDAV、FTP、SMB 共用一份共享使用者 |
| File Browser | 託管 FileBrowser Quantum 網頁檔案管理器：多實例、多個共享目錄與唯讀、帳號密碼或反向代理驗證；線上安裝、更新、回滾與解除安裝，可恢復預設設定 |
| 儲存掛載 | 本機目錄、遠端 WebDAV 與 SMB 共用掛載，線上檔案管理 |
| 排程工作 | cron 運算式，指令或 HTTP 請求，執行記錄 |
| 安全與設定 | Passkey（WebAuthn）登入、兩步驟驗證（TOTP）、唯讀帳號、登入失敗限流、安全入口、僅內網存取、管理介面 HTTPS、多組 API 權杖（依模組讀寫授權）、已儲存的密碼與金鑰可依需求顯示（需寫入權限）、自訂 DNS（支援 DoH）、時區、設定備份與還原、信任本機反向代理傳來的真實用戶端位址、依需求開關功能模組（關閉的模組不啟動服務、不提供介面） |
| 飛牛原生應用 | 離線 `.fpk` 安裝包（x86 / ARM）；接入飛牛統一閘道，從飛牛桌面圖示開啟，遠端網域同樣可用；飛牛管理員可憑飛牛登入狀態直接登入（預設關閉）；以 root 執行，可監聽 80/443 並管理 Docker |
| 稽核與歷史 | 操作稽核日誌：登入、設定變更、存取敏感內容（不記錄請求內容與任何金鑰）；每次覆寫設定檔前保留舊版本，可比對差異並一鍵回滾；宣告式設定目錄（`-apply`），便於 GitOps |
| 監控與自檢 | 全域 Prometheus 指標 `/api/metrics`（憑證、DDNS、STUN、連接埠轉發、Web 服務），附 Grafana 儀表板；一鍵網路自檢（公網 IP、電信商 NAT（CGNAT）、系統時間、80/443 連接埠、DDNS 解析、憑證到期、HTTPS 站台缺少憑證、frp 代理衝突） |
| 遷移 | 匯入 nginx 設定（支援 `nginx -T` 輸出）：反向代理與 upstream、靜態目錄、跳轉、分路徑後端、存取控制，`auth_request` 轉換為轉發認證 |

</details>

## 為什麼做 Gately

Gately 完全開源，使用者和社群可以直接檢視原始碼與設定。對於運行在網路邊界、掌握 DNS 金鑰與憑證私鑰的軟體，開源讓使用者能夠看見它在做什麼。Gately 的承諾：

- 原始碼以 Apache-2.0 開源，接受社群審查
- 預設不連線任何專案方伺服器，不回報裝置資訊；第三方登入使用你自己的 OAuth 應用程式，不經過中轉
- 所有對外位址（公網 IP 查詢、STUN 伺服器等）皆可檢視、可修改、可關閉
- 設定為明文 JSON，可直接納入 git 進行稽核與還原

## 效能

連接埠轉發的 SNI 分流加上 Web 服務，可以取代常見的 haproxy（依 SNI 做 TCP 分流）+ nginx（TLS 與反向代理）組合。下表在同一台機器上，依一台真實伺服器的設定比較兩者：90 個網域依 SNI 分流，兩層之間使用 PROXY 協定 v2，ECDSA P-256 憑證，TLS 1.3。

| 情境 | haproxy + nginx | Gately |
| --- | --- | --- |
| HTTPS 反向代理，長連線，1 KB 回應，64 並行連線 | 14,767 req/s（p99 35.9 ms） | **26,434 req/s**（p99 5.7 ms） |
| 同上，HTTP/2 | 未測試（nginx 設定使用 HTTP/1.1） | 27,795 req/s |
| 每個請求建立新的 TLS 連線，32 並行連線 | 2,557 req/s | **3,448 req/s** |
| 1 MB 回應，16 並行連線 | 1,053 MB/s | **1,585 MB/s** |
| SNI 透傳 TLS 資料流，4 條連線 | 2,927 MB/s | **4,962 MB/s** |
| SNI 透傳，每秒新建連線數，64 並行連線 | 6,334 /s | 6,266 /s（持平） |
| 負載下的記憶體 | haproxy ≈ 21 MB + nginx ≈ 21 MB | ≈ 110 MB（包含所有模組的單一程序） |

<details>
<summary><b>測試方法</b></summary>

- 機器：Intel Core i5-13500T。為貼近小型雲端伺服器，每套方案限制在 2 個 CPU 核心（haproxy 與 nginx 在 Docker 中使用 `--cpuset-cpus`，Gately 用 `taskset` 綁定核心），後端與壓測程式執行在其他核心。
- haproxy 使用 `mode tcp`、`req_ssl_sni` 規則與 `send-proxy-v2`。nginx 使用 `worker_processes 2` 與正式站台的代理設定，其中 `Connection: upgrade` 會讓每個請求建立一條新的後端連線；啟用 upstream keep-alive 的 nginx 在小型回應上會縮小差距。
- Gately 的路徑是 SNI 規則、PROXY v2、再到 Web 服務監聽，層數與 haproxy → nginx 相同。
- 每個情境執行 10 秒，重複測試的情境差異在 3% 以內。在真實的 2 vCPU 雲端主機上，靜態站台為 5.5k req/s 對 3.4k req/s；用戶端使用 X25519 時，新建 TLS 連線持平（雙方都約每秒 480–550 條），使用 Go 用戶端與新版瀏覽器預設提供的後量子混合金鑰交換時，Gately 在該次測試中完成的握手少約 13%。
- 結果受硬體、設定與流量影響，僅用於確認 Gately 跟得上 haproxy 與 nginx，請用自己的實際負載實測。

</details>

## 開發

相依套件：Go 1.26+、Node.js 24+、pnpm。

```bash
make web      # 建置前端（輸出到 web/dist，由 go:embed 嵌入）
make build    # 編譯 bin/gately
make test     # go test -race ./...
make lint     # gofmt、go vet、前端型別檢查
```

前端開發時執行 `cd web && pnpm dev`，`/api` 會代理到本機 `127.0.0.1:16680`。

部分測試預設跳過，需要額外條件：

| 測試 | 啟用方式 |
| --- | --- |
| ACME 端對端（Pebble） | 安裝 pebble 與 pebble-challtestsrv，設定 `GATELY_PEBBLE_BIN`、`GATELY_PEBBLE_SRC`（CI 中自動執行） |
| 真實公共 STUN 伺服器 | `GATELY_NET_TEST=1` |

目錄結構與技術選型見 [docs/zh-TW/ROADMAP.md](docs/zh-TW/ROADMAP.md)。

官網（[gately.cn](https://gately.cn)）原始碼在 `website/`，基於 VitePress：`cd website && pnpm install && pnpm dev`；`pnpm deploy:site` 建置並同步到伺服器。

## 貢獻約定

- 相容性程式碼只能建立在公開可觀察的行為、介面和設定格式上，請勿提交從第三方執行檔或前端產物中複製的程式碼。
- 新模組需要：設定結構與驗證、執行階段啟停、REST 介面、單元測試；希望在總覽頁顯示狀態的模組實作 `module.Summarizer`。
- 程式碼註解使用中文；控制語句必須使用大括號。

## 交流群

歡迎加入 Telegram 群交流使用問題、回饋建議：[t.me/gately_chat](https://t.me/gately_chat)，也可以用手機掃描 QR Code 加入：

<a href="https://t.me/gately_chat"><img src="docs/images/telegram-qr.svg" alt="掃描加入 Gately Telegram 群組" width="160"></a>

Bug 請提交到 [GitHub Issues](https://github.com/wangwanjie/gately/issues)，並附上[診斷包](docs/zh-TW/DIAGNOSTICS.md)（設定 → 診斷）；診斷包在本機產生，不含密碼、權杖、網域名稱和真實 IP。

## 贊助

Gately 的全部功能免費開放。如果它幫到了你，歡迎透過 USDT、[愛發電](https://ifdian.net/a/gately)或微信贊助，收款碼與地址見[官網贊助頁](https://gately.cn/zh-TW/sponsor)。

## Star 趨勢

[![Star History Chart](https://api.star-history.com/svg?repos=wangwanjie/gately&type=Date)](https://star-history.com/#wangwanjie/gately&Date)

## 授權條款

[Apache License 2.0](LICENSE)
