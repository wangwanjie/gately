# 效能對比

本頁把 Gately 和它能取代的常見工具放在同一台機器、同樣的 CPU 限額下對比：反向代理、網頁驗證、WAF、入侵防護、連接埠敲門、動態網域，以及在 OpenWrt 軟路由上的表現。所有數字來自 2026 年 11 月的實測，測試腳本、設定和原始結果都在儲存庫的 [tools/benchmark](https://github.com/wangwanjie/gately/tree/main/tools/benchmark) 目錄，可以照著重現。

<script setup>
const H = '越大越好'
const L = '越小越好'
</script>

## 結論

- **反向代理**：C 語言寫的 nginx、HAProxy 吞吐量最高，HTTP 約是 Gately 的 3.5 倍、HTTPS 約 2.6 倍。Gately 在 Go 實作的閘道中屬於第一梯隊：HTTPS 吞吐量比 Caddy 高 43%、是 Lucky 的 2.3 倍，與 Traefik 相差 9%；TLS 新建連線速率在 7 個程式中最高。
- **站點多了不掉速**：1000 個站點時 Gately 的吞吐量只降 6%，超過 HAProxy、Traefik 和 Caddy，僅次於 nginx。
- **網頁驗證**：已登入使用者的請求，Gately 是 nginx + Authelia 的 1.9 倍、Caddy + Authelia 的 3.7 倍，記憶體只有它們的三分之一。
- **WAF**：同為 OWASP CRS 4.25，Gately 處理一般請求比 nginx + ModSecurity 快約 70%，攔截結果完全一致；檢查 POST 請求本文時 ModSecurity 更快、記憶體也低得多。
- **入侵防護**：1 萬個攻擊 IP 時 Gately 12 秒封完，fail2ban 要 51 秒；同樣日誌量下 CPU 只有 fail2ban 的一半以下。讀取日誌檔時單次封鎖延遲比 fail2ban 多約 0.2 秒。
- **連接埠敲門**：Gately 與 fwknop 敲門延遲都在 10 ms 內，放行後流量不經過敲門程式；飛牛敲門在一般 Linux 上經閘道轉發，短連線吞吐量只有直連的 3%。
- **資源占用**：只算單一功能時，Gately 的記憶體比 fail2ban、fwknop、ddns-go 這類小工具大；同時承擔反向代理、WAF、驗證、DDNS、入侵防護與連接埠敲門時，一個 Gately 約 91 MiB，換成各自獨立的工具合計約 204 MiB。詳見[一體化](#一體化)。

## 測試環境

| 項目 | 說明 |
| --- | --- |
| 主機 | Intel Core i5-13500T，32 GB 記憶體，飛牛 fnOS（KVM） |
| 壓測虛擬機 | Debian 13（核心 6.12），8 vCPU 綁定主機 4 個效能核心的 8 個執行緒，8 GB 記憶體，Docker 29 |
| CPU 劃分 | 受測程式 vCPU 0–1，來源站 nginx vCPU 2–3，壓測工具 vCPU 4–7，互不搶占 |
| 軟路由 | ImmortalWrt 25.12.1 虛擬機，2 vCPU（同一個效能核心的兩個執行緒），1 GB 記憶體 |
| 壓測工具 | wrk 4.1、oha 1.17、h2load 1.64（nghttp2） |
| 憑證 | 所有程式共用同一張 ECDSA P-256 憑證，TLS 1.3 |
| 來源站 | nginx 回傳 1 KB 文字；大檔案測試為 100 MB |

受測版本：Gately v0.6.8.1、nginx 1.29.8、HAProxy 3.2.26、Caddy 2.11.9、Traefik 3.7.19、Lucky 3.8.1、Nginx Proxy Manager 2.17.0（OpenResty 1.31.8）、Authelia 4.39.28、ModSecurity 3.0.16（OWASP CRS 4.25.0）、fail2ban 1.1.0、fwknop 2.6.11、飛牛敲門 fn-knock 2.5.11、ddns-go 6.17.9。除 fail2ban、fwknop 用 Debian 套件外，其餘都用官方 Docker 映像檔，網路為 host 模式。

**口徑**

- 吞吐量：每項 20 秒、測 3 輪取中位數。HTTP 與 HTTPS 用 wrk 128 條長連線；TLS 新建連線用 oha 64 並行、每個請求新開連線；HTTP/2 用 h2load 64 條連線、每條 10 路並行。
- 記憶體：受測容器內全部行程的 PSS 之和（多行程共用的記憶體按比例分攤，不會重複計算）。「閒置」是啟動並設定完成 10 秒後的值，「峰值」是壓測期間的最大值。
- CPU：受測容器 cgroup 的 CPU 時間，100% 等於一個 vCPU。
- 啟動：設定就緒後重新啟動程式，計時到第一個請求正確回傳，取 3 次中位數。
- 各程式都關閉存取日誌。nginx 開啟到來源站的長連線池並放寬每條連線的請求數上限（常見調校寫法）；其餘程式使用預設設定。

## 反向代理

網域 `bench.test` 反向代理到來源站，HTTP 監聽 8080，HTTPS 監聽 8443（Nginx Proxy Manager 只能用 80/443）。

<BenchBars title="HTTPS 請求/秒（長連線）" unit="QPS" :better-label="H" :items="[['nginx',85434],['HAProxy',83953],['Traefik',35812],['Nginx Proxy Manager',32816],['Gately',32764],['Caddy',22871],['Lucky',14376]]" />

<BenchBars title="TLS 新建連線/秒（每個請求一次完整握手）" unit="次/秒" :better-label="H" :items="[['Gately',4636],['Traefik',4482],['Nginx Proxy Manager',4015],['Caddy',3987],['nginx',3796],['Lucky',3621],['HAProxy',3293]]" />

<BenchBars title="閒置記憶體" unit="MiB" better="lower" :digits="1" :better-label="L" :items="[['nginx',15.7],['HAProxy',36.4],['Gately',47.6],['Caddy',48.8],['Traefik',89.0],['Lucky',124.6],['Nginx Proxy Manager',155.7]]" note="Gately 的 47.6 MiB 包含全部功能模組（DDNS、憑證、WAF、入侵防護等都在同一個行程裡）；Lucky 為 2 個行程，Nginx Proxy Manager 為 22 個行程（nginx、Node.js 後端等）。" />

| 程式 | HTTP | HTTPS | HTTP/2 | TLS 新建/秒 | 100 MB 大檔案 | 5000 QPS 時 CPU | 閒置記憶體 | 壓測峰值 | 每條閒置連線 | 重啟耗時 | 映像檔 |
| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |
| **Gately** | 35,364 | 32,764 | 21,591 | **4,636** | 4,157 MB/s | 67% | 47.6 MiB | 107 MiB | 21.2 KiB | 0.40 秒 | 229 MB |
| nginx | **123,343** | **85,434** | 78,110 | 3,796 | 2,161 MB/s | 55% | **15.7 MiB** | **25 MiB** | **0.6 KiB** | **0.32 秒** | 93 MB |
| HAProxy | 117,923 | 83,953 | **90,633** | 3,293 | 3,912 MB/s | **52%** | 36.4 MiB | 56 MiB | 1.0 KiB | 0.33 秒 | 58 MB |
| Caddy | 24,459 | 22,871 | 11,746 | 3,987 | 4,116 MB/s | 68% | 48.8 MiB | 132 MiB | 27.8 KiB | 0.45 秒 | 92 MB |
| Traefik | 39,112 | 35,812 | 19,606 | 4,482 | **4,311 MB/s** | 66% | 89.0 MiB | 253 MiB | 24.4 KiB | 0.53 秒 | 250 MB |
| Lucky | 15,150 | 14,376 | 10,261 | 3,621 | 3,963 MB/s | 80% | 124.6 MiB | 193 MiB | 26.2 KiB | 3.47 秒 | 58 MB |
| Nginx Proxy Manager | 45,257 | 32,816 | 34,710 | 4,015 | 2,150 MB/s | 58% | 155.7 MiB | 215 MiB | 預先配置 | 1.50 秒 | 1,959 MB |

- HTTP、HTTPS、HTTP/2 的單位為請求/秒。「5000 QPS 時 CPU」是以固定 5000 請求/秒壓測時受測程式占用的 CPU，越低越省電。
- nginx 預設每條連線處理 1000 個請求後關閉，而 h2load 不會重新建立連線，所以 HTTP/2 一欄 nginx 與 Nginx Proxy Manager 都放寬了這一上限（Nginx Proxy Manager 只在 HTTP/2 測試時透過自訂設定放寬，其餘測試保持預設）。
- 大檔案一欄 nginx 與 Nginx Proxy Manager 較低，是因為 nginx 預設把超出記憶體緩衝的回應先寫進磁碟暫存檔。
- 「每條閒置連線」是保持 1 萬條閒置長連線時增加的記憶體除以連線數。Go 程式每條連線有獨立的讀寫緩衝與協程，約 20–28 KiB；nginx 與 HAProxy 不到 1 KiB。Nginx Proxy Manager 按設定預先配置了連線表，測不出增量。
- Gately 映像檔較大是因為帶了 Docker CLI、nftables 和 iptables 工具；程式本身是 40 MB 的單一檔案，下載壓縮檔 18 MB。

## 1000 個站點

每個程式設定 1000 個網域不同的反向代理站點，壓測時每個請求隨機選一個網域。

<BenchBars title="1000 個站點時的 HTTP 請求/秒" unit="QPS" :better-label="H" :items="[['nginx',122958],['Gately',33211],['Traefik',24229],['HAProxy',24057],['Caddy',9819]]" />

| 程式 | 請求/秒 | 相比 1 個站點 | p99 延遲 | 重啟到最後一個站點可用 | 閒置記憶體 | 壓測峰值 |
| --- | --- | --- | --- | --- | --- | --- |
| **Gately** | 33,211 | -6% | 29.8 ms | **0.43 秒** | 94 MiB | 463 MiB |
| nginx | **122,958** | **0%** | **1.2 ms** | 0.54 秒 | 57 MiB | **58 MiB** |
| HAProxy | 24,057 | -80% | 30.0 ms | **0.37 秒** | **51 MiB** | **58 MiB** |
| Caddy | 9,819 | -60% | 52.7 ms | 0.80 秒 | 80 MiB | 361 MiB |
| Traefik | 24,229 | -38% | 18.8 ms | 0.74 秒 | 115 MiB | 157 MiB |

- HAProxy 用 `use_backend ... if { hdr(host) -i 網域 }` 逐條比對，站點越多越慢；改用 map 檔案可以避免，這裡保持「每個站點一條規則」的常見寫法。
- Gately 的記憶體峰值主要來自每個站點的存取統計：有流量的站點各自保存 24 小時的分鐘級統計、連線歷史與延遲分佈，1000 個站點同時有流量時常駐兩百多 MiB。

## 網頁驗證

站點需要登入才能存取，測試已登入使用者的請求和登入本身。Gately 使用站點內建的網頁驗證與基本驗證；對照組為 nginx 或 Caddy 前置 Authelia（`auth_request` / `forward_auth`），以及 nginx 內建的 `auth_basic`。全部走 HTTPS。

<BenchBars title="已登入使用者的請求/秒（攜帶工作階段 Cookie）" unit="QPS" :better-label="H" :items="[['Gately',26726],['nginx + Authelia',14291],['Lucky',10786],['Caddy + Authelia',7141]]" />

| 方案 | 已登入請求/秒 | 不加驗證 | 基本驗證（bcrypt） | 成功登入/秒 | 登入時記憶體峰值 | 閒置記憶體 |
| --- | --- | --- | --- | --- | --- | --- |
| **Gately** | **26,726** | 32,520 | **28,598** | **38.3** | **70 MiB** | **45 MiB** |
| nginx + Authelia | 14,291 | 83,474 | 40 | 13.4 | 1,082 MiB | 120 MiB |
| Caddy + Authelia | 7,141 | 22,703 | 21,213 | 13.4 | 1,271 MiB | 149 MiB |
| Lucky | 10,786 | 13,841 | 10,107 | 見說明 | 209 MiB | 127 MiB |

- 密碼雜湊：Gately 與 nginx/Caddy 的基本驗證為 bcrypt（cost 10），Authelia 為 argon2id（t=3、64 MiB、p=4），Lucky 以明文儲存站點使用者密碼。Authelia 每次登入要占用 64 MiB 記憶體，8 個並行登入時記憶體超過 1 GB。
- nginx 的 `auth_basic` 每個請求都重新計算 bcrypt，所以只有 40 請求/秒；Gately 與 Caddy 會快取驗證結果。換成 apr1（MD5）時 nginx 為 13,082 請求/秒。
- 錯誤密碼：Gately 同一來源失敗 10 次後直接拒絕（每秒可處理 1.6 萬個錯誤請求，CPU 不再做雜湊）；Authelia 每次失敗固定延遲約 1 秒。兩者在鎖定期間都會拒絕該來源的正確密碼，已登入的工作階段不受影響。
- Lucky 在 8 個並行登入時全部回傳 `create webauth session error`，未能測出登入吞吐量。

## WAF

三方都使用 OWASP CRS 4.25.0、偏執等級 1、攔截模式。Gately 與 Caddy 用同一個 Coraza 引擎，nginx 用 ModSecurity v3。

<BenchBars title="開啟 WAF 後的 GET 請求/秒" unit="QPS" :better-label="H" :items="[['Gately',2498],['Caddy + Coraza',2265],['nginx + ModSecurity',1491]]" />

| 程式 | GET | 帶 8 個參數的 GET | 4 KB 表單 POST | 攻擊流量 | 壓測峰值記憶體 | 閒置記憶體（開/關） |
| --- | --- | --- | --- | --- | --- | --- |
| **Gately** | **2,498** | **1,067** | 72 | **1,218** | 754 MiB | 88 / 48 MiB |
| nginx + ModSecurity | 1,491 | 591 | **102** | 1,014 | **73 MiB** | 62 / 24 MiB |
| Caddy + Coraza | 2,265 | 943 | 46 | 1,004 | 792 MiB | 96 / 55 MiB |

| 偵測效果 | Gately | nginx + ModSecurity | Caddy + Coraza |
| --- | --- | --- | --- |
| 攻擊樣本攔截（557 筆） | 77.2% | 77.2% | 77.2% |
| 正常樣本誤報（389 筆） | 20.1% | 20.1% | 20.1% |
| CRS 回歸案例命中（2049 筆） | 95.9% | 95.2% | 95.7% |

- CRS 規則本身很重：三方開啟 WAF 後吞吐量都只剩關閉時的 1%–10%，需要 WAF 的站點建議只對動態介面開啟。
- 檢查 POST 請求本文時，Coraza 引擎（Gately 與 Caddy 都是）的記憶體峰值超過 700 MiB，ModSecurity 保持在 73 MiB；小記憶體裝置上開啟請求本文檢查要留意。
- 樣本來自 gotestwaf（MIT）、CRS 內建回歸測試（Apache-2.0）與手寫的正常請求，誤報主要是含 SQL/Shell 關鍵字的技術文章和 PUT/DELETE 介面（CRS 預設不允許這些方法）。

## 入侵防護

情境：Web 服務被掃描，60 秒內 5 次 404 即封鎖 IP（nftables）。fail2ban 用 pyinotify 讀 nginx 存取日誌；Gately 用等價的日誌檔規則讀同一份日誌，另測一組 Gately Web 服務內建的路徑探測。

<BenchBars title="1 萬個攻擊 IP 全部封完所需時間" unit="秒" better="lower" :digits="1" :better-label="L" :items="[['Gately',12.3],['fail2ban',51.2]]" />

| 指標 | Gately（讀日誌檔） | Gately（Web 服務內建探測） | fail2ban |
| --- | --- | --- | --- |
| 單一攻擊者封鎖延遲 p50 / p99 | 478 / 996 ms | **10 / 13 ms** | 280 / 864 ms |
| 1000 行/秒日誌的 CPU | **2.9%** | — | 20.4% |
| 2 萬行/秒日誌的 CPU | **35.7%** | — | 83.2% |
| 1 萬個攻擊 IP：全部封完 | **12.3 秒** | — | 51.2 秒 |
| 1 萬個攻擊 IP：消耗 CPU | **10.1 CPU 秒** | — | 60.3 CPU 秒 |
| 閒置記憶體 | 44.1 MiB | 40.7 MiB | **17.6 MiB** |
| 漏封 / 誤封 | 0 / 0 | 0 / 0 | 0 / 0 |

- Gately 每秒讀取一次日誌檔，所以單次封鎖延遲在 0–1 秒之間，平均比 fail2ban 的 inotify 慢約 0.2 秒；Web 服務內建探測在請求處理時直接計數，沒有這段延遲。
- fail2ban 每次封鎖都要啟動一次 `nft` 行程，攻擊者一多就排隊：1 萬個 IP 時最後一個在注入結束 41 秒後才被封。
- fail2ban 本身 2 MB，但依賴約 24 MB 的 Python 執行環境。

## 連接埠敲門

對比三種做法：Gately 的連接埠敲門（相容 fwknop 的 SPA 單封包授權，nftables 攔截）、fwknop 2.6.11（同樣是 SPA，iptables 攔截），以及飛牛敲門 fn-knock 2.5.11（網頁登入後按來源 IP 放行，由它的閘道轉發流量）。Gately 也提供網頁登入後放行的入口，單獨作為一組。受保護的是 TCP 連接埠上的一個 nginx 服務，用戶端使用公網測試位址，不受「內網免敲門」影響。

<BenchBars title="放行後經受保護連接埠的短連線請求/秒" unit="QPS" :better-label="H" :items="[['fwknop',65046],['Gately',64763],['Gately 網頁放行',63556],['fn-knock',1886]]" note="不經任何敲門程式直連來源站為 67,034 請求/秒。" />

| 指標 | Gately（SPA） | Gately（網頁放行） | fwknop | fn-knock |
| --- | --- | --- | --- | --- |
| 敲門到連接埠可連 p50 / p99 | 8.4 / 13.4 ms | 58 / 73 ms | 9.0 / 18.1 ms | 84 / 143 ms |
| 未授權時受保護連接埠 | 丟棄（逾時） | 丟棄（逾時） | 丟棄（逾時） | 握手後重設（可被探測到） |
| 放行後長連線 / 短連線請求/秒 | 143,867 / 64,764 | 143,839 / 63,556 | 148,371 / 65,046 | 140,137 / 1,886 |
| 放行後轉發消耗的 CPU（短連線） | 2% | 1% | 0% | 134% |
| 單一來源 5 萬個無效封包/秒時的合法敲門 | **10/10**（CPU 26%） | — | 3/8（CPU 102%） | — |
| 偽造來源 1 萬個無效封包/秒時的合法敲門 | 1/7 | — | **10/10**（CPU 54%） | — |
| 閒置記憶體 | 48.7 MiB | 48.7 MiB | **2.2 MiB** | 63.2 MiB（3 個行程） |

- Gately 與 fwknop 放行後由防火牆直接放行，流量不經過敲門程式，所以吞吐量與直連相同；fn-knock 在一般 Linux 上透過閘道轉發，每個新連線還要向控制面查詢一次授權，短連線吞吐量只有直連的 3%。fn-knock 在飛牛 fpk 版本中另有防火牆直連模式，這裡測的是 Docker 版。
- fn-knock 的登入需要先完成 ALTCHA 工作量證明（本測試求解 p50 為 28 ms），不含求解時為 50 ms。
- fwknopd 單執行緒處理每個封包，單一來源 5 萬封包/秒時跑滿一個核心，合法敲門開始失敗。Gately 對每個來源限速，單一來源的洪泛幾乎不占 CPU。
- 已知問題：攻擊者偽造大量不同來源位址時，Gately 的來源限速表會被占滿，新來源共用的溢出配額被洪泛耗盡，合法敲門多數失敗（此時 Gately 的 CPU 只有約 19%）。fwknop 不限速，在 CPU 跑滿之前不受影響。

## 動態網域

每個程式設定 20 個 DDNS 工作、檢查週期 30 秒，公網 IP 從本機來源站查詢，更新透過「自訂回呼」發到本機來源站，不碰真實的 DNS 服務商。啟動並完成首次同步後，取樣 5 分鐘。

<BenchBars title="20 個 DDNS 工作時的平均記憶體" unit="MiB" better="lower" :digits="1" :better-label="L" :items="[['ddns-go',16.9],['Gately',51.6],['Lucky',131.3]]" />

| 程式 | 平均記憶體 | 峰值記憶體 | 平均 CPU | 映像檔 |
| --- | --- | --- | --- | --- |
| ddns-go 6.17.9 | **16.9 MiB** | **18.2 MiB** | **0.04%** | 43 MB |
| Gately | 51.6 MiB | 55.4 MiB | 0.16% | 229 MB |
| Lucky 3.8.1 | 131.3 MiB | 134.9 MiB | 0.48% | 58 MB |

- 三者都只在 IP 變化或到了強制同步週期時才請求服務商，常駐開銷都很小。只需要 DDNS 時 ddns-go 最省記憶體；Gately 的數字包含它的全部模組，同一個行程裡還可以同時跑憑證、反向代理與其他功能。

## OpenWrt 軟路由

在 ImmortalWrt 25.12.1 軟路由虛擬機（2 vCPU、1 GB 記憶體）上執行各程式，壓測機經區域網路橋接存取路由器，路由器再反向代理到壓測機上的來源站。nginx 1.26.3 與 HAProxy 3.2.21 來自 ImmortalWrt 軟體源，Gately 使用官方 OpenWrt apk 套件，Lucky 使用官方 Linux 二進位檔。

<BenchBars title="軟路由上的 HTTPS 請求/秒（長連線）" unit="QPS" :better-label="H" :items="[['nginx',40665],['HAProxy',36266],['Gately',15669],['Lucky',8113]]" />

| 程式 | HTTP | HTTPS | TLS 新建/秒 | 2000 QPS 時 CPU | 閒置記憶體 | 壓測峰值 | 程式體積 |
| --- | --- | --- | --- | --- | --- | --- | --- |
| **Gately** | 16,439 | 15,669 | 2,665 | 27% | 43 MiB | 65 MiB | 40.5 MB |
| nginx | **49,566** | **40,665** | **3,111** | **12%** | **15 MiB** | **22 MiB** | **0.8 MB** |
| HAProxy | 43,046 | 36,266 | 2,337 | 12% | 24 MiB | 29 MiB | 3.7 MB |
| Lucky | 8,642 | 8,113 | 2,021 | 44% | 64 MiB | 81 MiB | 17.9 MB |

- 路由器上的瓶頸主要在核心網路收發（軟中斷），各程式本身的 CPU 都沒有用滿兩個核心，所以差距比在壓測機上小。Gately 的吞吐量約是 Lucky 的 2 倍，記憶體少三分之一。
- OpenWrt 的核心沒有 smaps，這裡的記憶體按 `/proc/<pid>/statm` 估算（各行程私有常駐記憶體之和加一份共用部分）。測試期間暫時關閉了防火牆預設的 SYN 洪泛保護（每秒約 25 個新連線），否則 TLS 新建連線測試會被它限住。
- 只做反向代理的話，軟體源裡的 nginx 體積和記憶體都最小；Gately 的 40 MB 包含全部模組，可以在 [build.gately.cn](https://build.gately.cn) 只選需要的模組產生更小的安裝包。快閃記憶體較小的路由器建議先確認剩餘空間。

## 一體化

同一個 Gately 同時開啟：兩個反向代理站點（HTTP 與 HTTPS，均啟用 WAF）、一個網頁驗證站點、20 個 DDNS 工作、一條入侵防護日誌規則和一條連接埠敲門規則。對照組是用獨立工具實現同樣功能的組合，取上文各自實測的閒置記憶體相加。

<BenchBars title="同樣一組功能的閒置記憶體合計" unit="MiB" better="lower" :digits="1" :better-label="L" :items="[['Gately',90.8],['nginx + ModSecurity + Authelia + ddns-go + fail2ban + fwknop',203.5],['Nginx Proxy Manager + Authelia + ddns-go + fail2ban + fwknop',297.3]]" note="Nginx Proxy Manager 那一組沒有 WAF。" />

| 功能 | 獨立工具 | 閒置記憶體 | 安裝體積 |
| --- | --- | --- | --- |
| 反向代理 + WAF | nginx + ModSecurity（CRS 4.25） | 61.9 MiB | 156 MB 映像檔 |
| 反向代理（圖形介面） | Nginx Proxy Manager | 155.7 MiB | 1,959 MB 映像檔 |
| 網頁驗證 | Authelia | 104.4 MiB | 110 MB 映像檔 |
| 動態網域 | ddns-go | 17.4 MiB | 43 MB 映像檔 |
| 入侵防護 | fail2ban | 17.6 MiB | 24 MB（含 Python） |
| 連接埠敲門 | fwknop | 2.2 MiB | 0.3 MB |
| **全部** | **Gately** | **90.8 MiB** | **40 MB 程式（下載 18 MB）** |

- Gately 一體化實例在 1000 請求/秒經 WAF 的 HTTPS 流量下，記憶體峰值 114 MiB，CPU 71%；20 個 DDNS 工作全部同步成功。
- 憑證申請（certbot、acme.sh）只在續期時執行，不計入常駐記憶體。

## 開發版的優化

根據上面的結果，開發版（尚未發佈，提交 `3d579389` 之後）做了以下改動，並在同一環境複測。反向代理與 1000 個站點用提交 `3d579389` 的建置，入侵防護用包含後兩項改動的建置；對照列是同一天重新測的 v0.6.8.1（只有 HTTP 與 HTTPS 兩項當天重測，其餘對照取上文資料）。

- 同一監聽器內反向代理到同一後端、連線設定相同的站點共用一個連線池。
- 可用記憶體不少於 1 GiB 時，GC 百分比預設值由 100 改為 200（設定裡可改，小記憶體裝置保持 100）。
- 反向代理每個請求少複製一次請求、少建三個上下文值。
- 入侵防護讀取日誌檔案改用 inotify，寫入即讀取（保留每秒輪詢兜底）；封鎖寫入防火牆最多每 250 毫秒合併一次。

| 指標 | v0.6.8.1 | 開發版 | 變化 |
| --- | --- | --- | --- |
| HTTP 請求/秒（同日對照） | 35,968 | 38,358 | +6.6% |
| HTTPS 請求/秒（同日對照） | 34,072 | 36,079 | +5.9% |
| TLS 新建連線/秒 | 4,636 | 4,942 | +6.6% |
| HTTP/2 請求/秒 | 21,591 | 23,788 | +10% |
| 1000 個站點 請求/秒 | 33,211 | 38,839 | +17% |
| 1000 個站點 p99 延遲 | 29.8 ms | 11.7 ms | -61% |
| HTTP 壓測時記憶體峰值 | 64.5 MiB | 81.1 MiB | +26% |
| 入侵防護：單一攻擊者封鎖延遲 p50 / p99 | 478 / 996 ms | 9.8 / 129 ms | |
| 入侵防護：持續攻擊時封鎖延遲 p50 / p99（2 萬行/秒） | 509 / 1006 ms | 143 / 269 ms | |
| 入侵防護：1000 行/秒的 CPU | 2.9% | 8.0% | 升高 |
| 入侵防護：2 萬行/秒的 CPU | 35.7% | 26.5% | -26% |
| 入侵防護：1 萬個攻擊 IP 全部封完 | 12.3 秒，10.1 CPU 秒 | 10.1 秒，2.5 CPU 秒 | |

- 吞吐的提升主要來自 GC 設定，代價是壓測時記憶體峰值多出十幾 MiB。剖析顯示代理路徑上剩下的開銷幾乎都在 Go 標準庫的 HTTP 實作與 GC 中，與 nginx、HAProxy 的差距屬於語言與執行階段層面。
- 1000 個站點時吞吐提高、延遲下降，是因為共用連線池後到來源站的連線少了很多；記憶體峰值沒有下降，原因見上文。
- 入侵防護的單次封鎖延遲已低於 fail2ban（280 / 864 ms）。低速率日誌下 CPU 比舊版高，是因為防火牆每秒最多寫入 4 次，舊版每秒 1 次；仍只有 fail2ban（20.4%）的四成。

## 重現

```bash
git clone https://github.com/wangwanjie/gately && cd gately/tools/benchmark
# 在一台 Debian 13 虛擬機上以 root 執行，需要 Docker 與 wrk、oha、h2load
bash provision-debian.sh
python3 lib/run.py gately 3     # 反向代理，換成 nginx、haproxy、caddy、traefik、lucky、npm
bash auth/run.sh                # 網頁驗證
bash waf/run.sh                 # WAF
bash guard/run.sh               # 入侵防護
bash knock/run.sh               # 連接埠敲門
bash scale/run.sh 1000          # 1000 個站點
bash ddns/run.sh 20             # 動態網域
bash bundle/run.sh              # 一體化
bash router/run.sh              # 軟路由（另需一台 OpenWrt）
```

每個目錄的 README 寫明了設定、口徑和已知限制。結果受硬體、虛擬化和設定影響，建議用自己的業務負載重測。
