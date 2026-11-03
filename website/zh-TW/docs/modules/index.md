# 模組總覽

Gately 的每項能力都是獨立模組，可單獨啟停、單獨設定，設定各自儲存為一個純文字 JSON 檔案。

在「設定 → 功能模組」中可以關閉整個模組：關閉的模組只載入設定，不啟動服務、不提供管理介面（介面回傳 403），並從介面中隱藏；既有設定會保留，重新開啟後恢復。被其他模組依賴的模組不能關閉，修改後重新啟動生效。

## 自訂建置 {#custom-build}

除連接埠轉發、Web 服務與 SSL 憑證三個核心模組外，其餘模組都可以在編譯時去掉，介面語言也可以只保留簡體中文或簡繁英三種，安裝包與記憶體占用隨之變小，適合儲存空間有限的路由器。在 [build.gately.cn](https://build.gately.cn) 勾選模組與語言即可線上產生執行檔、OpenWrt、飛牛安裝包或多架構 Docker 映像檔，頁面會估算體積；也可以依 [從原始碼建置](/zh-TW/download#custom-build) 自行編譯。

自訂建置中沒有編譯進來的模組：設定檔原樣保留，設定中的開關與 API 權杖的權限照常有效；介面回傳 404 並說明未包含在此建置中，管理介面隱藏對應頁面與相關選項（如站台的 WAF、憑證部署到 SSH 或 Docker）；換回完整版後恢復。「關於」頁列出本建置去掉的模組，並附帶以相同選項開啟建置器的連結。自訂建置不能線上升級（線上升級下載的是完整版本），新版本發佈後到建置器以相同選項重新產生，再「上傳升級包」。

| 模組 | 能力 | 文件 |
| --- | --- | --- |
| 動態網域 | 11 家 DNS 服務商，URL / 網卡 / 命令取得 IP，IPv4 / IPv6，Webhook | [查看](/zh-TW/docs/modules/ddns) |
| STUN 穿透 | TCP / UDP、保活、UPnP / NAT-PMP、內建轉發、變更時觸發 Webhook 與指令碼 | [查看](/zh-TW/docs/modules/stun) |
| 連接埠轉發 | TCP / UDP、IPv4 / IPv6、連接埠範圍、多目標容錯移轉、連線數限制、流量統計 | [查看](/zh-TW/docs/modules/portforward) |
| 協定分流 | 多個服務共用一個連接埠：TLS 依 SNI 與 ALPN、HTTP 依 Host，Minecraft 依伺服器位址，另可辨識 SSH、RDP、SOCKS、OpenVPN、RTSP、SIP、XMPP、MQTT、AMQP、PostgreSQL、Redis、SMB、Git、IRC、ADB、BitTorrent，VNC 等走等待逾時路由 | [查看](/zh-TW/docs/modules/protoroute) |
| SSL 憑證 | ACME DNS-01 自動簽發與續期、上傳、對應到目錄並執行指令碼 | [查看](/zh-TW/docs/modules/ssl) |
| Web 服務 | 反向代理、重新導向、靜態檔案、HTTPS、驗證、存取控制、負載平衡與健康檢查、快取與限流、存取統計 | [查看](/zh-TW/docs/modules/web) |
| OIDC 單一登入 | 把共用使用者以 OpenID Connect 提供給 Gitea、OpenList、Grafana 等應用程式登入，可取代 Authelia 的 OIDC | [查看](/zh-TW/docs/modules/oidc) |
| WAF 防火牆 | Coraza + OWASP CRS v4 | [查看](/zh-TW/docs/modules/waf) |
| 入侵防護 | 日誌規則（類 fail2ban）、誘餌連接埠、連接埠掃描偵測，封鎖同步到 nftables | [查看](/zh-TW/docs/modules/guard) |
| Tailscale / NetBird / frp / cloudflared | 託管官方執行檔的組網與內網穿透 | [查看](/zh-TW/docs/modules/tunnels) |
| Docker 管理 | 容器建立與編輯、日誌與終端機、Compose、映像檔更新與升級、網路與磁碟區 | [查看](/zh-TW/docs/modules/docker) |
| Web 終端機 | SSH、Telnet、本機 Shell、SFTP 檔案管理、持久工作階段 | [查看](/zh-TW/docs/modules/terminal) |
| 網路診斷 | Ping、路由追蹤、連接埠探測、DNS 查詢、測速、區域網路掃描 | [查看](/zh-TW/docs/modules/nettools) |
| 通知中心 | 多管道事件推送與規則 | [查看](/zh-TW/docs/modules/notify) |
| 效能監控 | 主機、程序與站台的效能趨勢（含首位元組延遲），門檻加持續時長告警 | [查看](/zh-TW/docs/modules/perf) |
| 設定備份 | 加密備份、保留策略、排程備份與還原 | [查看](/zh-TW/docs/modules/backup) |
| 檔案與維運工具 | WebDAV、FTP、SMB、File Browser、儲存掛載、網路喚醒、排程工作 | [查看](/zh-TW/docs/modules/tools) |
