# 協定分流

多個服務共用同一個公網連接埠（例如網站、VPN、SSH 和遠端桌面都使用 443）。Gately 讀取用戶端連線後最先送出的資料辨識協定，TLS 再看交握裡的網域（SNI）與 ALPN，HTTP 再看 Host，然後把整條連線透傳給對應的後端。這裡不解密、不終止任何協定，TLS 憑證仍放在後端。作用等同於 haproxy 的 `req_ssl_sni` 規則加上 sslh。

依網域分流（SNI 分流）是其中的一種情況：新增 TLS 路由並填寫網域即可。

每條規則包含：

- 監聽：協定（僅 TCP，tcp 為雙堆疊，tcp4 / tcp6 為單一位址族）、監聽位址與連接埠（支援 `443`、`443,8443` 或連接埠範圍）；
- 路由：每條路由有協定、名稱、目標位址（多個位址依序嘗試，具備容錯移轉）、選填的目標連接埠（留空與監聽連接埠相同），以及 PROXY 協定設定（不傳送、v1、v2）。PROXY 設定屬於路由，因為有的後端要求它，有的後端會拒絕它；
- 預設目標：規則自身的目標位址、目標連接埠與 PROXY 協定設定，未命中路由或無法辨識協定時使用；
- 辨識逾時：等待用戶端送來可辨識資料的秒數，0 為預設 5 秒。

## 可辨識的協定

| 路由協定 | 辨識依據 | 進一步比對 |
| --- | --- | --- |
| TLS | TLS 交握記錄（ClientHello） | 網域（SNI）、ALPN |
| HTTP | 請求行的方法名稱與 `HTTP/` 版本號，包括 CONNECT、WebDAV 方法與 HTTP/2 明文直連（h2c） | 網域（Host 標頭） |
| SSH | 用戶端版本識別字串 `SSH-` | — |
| RDP 遠端桌面 | TPKT 標頭與 X.224 連線請求 | — |
| SOCKS4 / SOCKS5 | SOCKS4、SOCKS4a 請求與 SOCKS5 方法協商 | — |
| OpenVPN（TCP） | 用戶端硬重設封包，包括 tls-auth、tls-crypt 與 tls-crypt-v2 | — |
| XMPP | XML 宣告或 `<stream:stream` | — |
| MQTT | CONNECT 封包（3.1、3.1.1、5.0） | — |
| PostgreSQL | 啟動訊息、SSLRequest 與 GSSENCRequest | — |
| Minecraft（Java 版） | 交握封包；1.6 及更早版本的伺服器清單查詢 | 伺服器位址 |
| RTSP | 請求行版本為 `RTSP/1.0` 或 `RTSP/2.0`（網路攝影機、NVR） | — |
| SIP | 請求行版本為 `SIP/2.0`（TCP） | — |
| Redis | 以 RESP 陣列送出的指令 | — |
| AMQP | 協定標頭 `AMQP`（0-9-1 與 1.0，RabbitMQ 等） | — |
| SMB | NetBIOS 工作階段訊息中的 SMB1、SMB2、SMB3 協商請求 | — |
| Git | `git://` 協定的第一個 pkt-line（git daemon） | — |
| IRC | 用戶端先送出的 CAP、PASS、NICK 或 USER 指令 | — |
| ADB | Android 偵錯橋的 CNXN 訊息（adb connect） | — |
| BitTorrent | 未加密的對等端交握 | — |
| 等待逾時 | 辨識逾時內用戶端沒有送出任何資料 | — |

## 比對規則

TLS、HTTP 與 Minecraft 路由依網域比對（Minecraft 使用交握封包中用戶端填寫的伺服器位址，Forge、BungeeCord、TCPShield 附加的內容會被忽略）：精確網域（`vpn.example.com`）優先，其次最長的萬用字元（`*.example.com`，符合任意層級的子網域），再次 `*`（符合任何帶有網域的連線），最後是沒有填寫網域的路由（符合該協定的全部連線，包括沒有 SNI 或 Host 的）。網域不區分大小寫。

TLS 路由還可以填寫 ALPN：用戶端宣告的 ALPN 含其中任一項時才符合。同一個網域下填寫了 ALPN 的路由優先，ALPN 不符合時繼續依更寬泛的網域查找。例如把 `acme-tls/1` 交給負責 TLS-ALPN-01 憑證驗證的程式，把 `postgresql` 交給 PostgreSQL 17 的直接 TLS 連線。同一協定下，同一個網域（及同一個 ALPN）只能屬於一條路由；其他協定每種只能有一條路由。

「等待逾時」用於 VNC、MySQL、FTP、SMTP、POP3、IMAP 這類由伺服器先送出資料的協定：用戶端連上後一直等伺服器發話，Gately 收不到任何資料，也就分辨不出是哪一種。所以每個連接埠只能把其中一種交給「等待逾時」路由，這類連線要等到辨識逾時才轉發。辨識逾時可以帶一位小數：TLS、SSH、HTTP 等用戶端在交握後幾毫秒內就會送出首包，有「等待逾時」路由時設為 0.3～1 秒即可，VNC 等連線也就只等這麼久。

以下連線交給預設目標：沒有符合的路由；資料不屬於上表任何協定（例如 Shadowsocks、VMess、啟用加密的 BitTorrent 這類沒有特徵的協定，以及 MongoDB、DNS over TCP 這類首包只有長度欄位、容易和其他協定混淆的協定）；ClientHello 無法解析；沒有「等待逾時」路由時用戶端一直不送資料。沒有設定預設目標時，這類連線會被關閉並計入「已斷開」。用戶端在辨識完成前斷線時直接丟棄。

## 說明

- 只處理 TCP。HTTP/3（QUIC）、WireGuard 等 UDP 流量不會被分流，請為它另外建立一條規則；
- SIP、Redis、AMQP、IRC 等協定的加密版本走 TLS，請用 TLS 路由依網域或 ALPN 分流；
- ClientHello 可以被拆成多個 TLS 記錄，最大支援 64 KB；HTTP、RTSP、SIP 的請求行最長 8 KB，HTTP 最多讀取 32 KB 請求標頭查找 Host；
- 每個連線只辨識一次，同一條 HTTP 長連線上的後續請求都送往第一次選中的後端；
- 規則卡片上會顯示每條路由與預設目標收到的連線數；執行日誌的除錯層級記錄每個連線辨識出的協定、SNI、ALPN 或 Host；
- 修改路由、預設目標與辨識逾時後立即生效，監聽不中斷，已建立的連線不受影響。

## 範例：取代 haproxy

這樣的 haproxy 設定：

```text
frontend https
    bind 192.0.2.10:443
    mode tcp
    tcp-request inspect-delay 5s
    tcp-request content accept if { req_ssl_hello_type 1 }
    use_backend ocserv if { req_ssl_sni -i tz.example.com }
    default_backend web
backend ocserv
    server ocserv 127.0.0.1:443 send-proxy-v2
backend web
    server web 127.0.0.1:8443 send-proxy-v2
```

對應一條 Gately 規則：

| 設定 | 值 |
| --- | --- |
| 協定 | TCP |
| 監聽位址 / 連接埠 | `192.0.2.10` / `443` |
| 路由 `ocserv` | 協定 TLS，網域 `tz.example.com`，目標 `127.0.0.1`，連接埠 `443`，PROXY v2 |
| 預設目標 | `127.0.0.1`，目標連接埠 `8443`，PROXY v2 |

監聽位址請填伺服器的公網位址，避免與監聽 `127.0.0.1:443` 的後端衝突。要用 Gately 自己提供網站，可以再新增一個監聽在 `127.0.0.1:8443` 的 [Web 服務](/zh-TW/docs/modules/web) 監聽，開啟「接收 PROXY 協定」並把 `127.0.0.0/8` 加入可信 PROXY 來源。啟用規則前請先停止 haproxy，同一個連接埠只能由一個程式監聽。

## 範例：取代 sslh

這樣的 sslh 啟動參數：

```text
sslh --listen 0.0.0.0:443 --timeout 2 \
     --ssh 127.0.0.1:22 --openvpn 127.0.0.1:1194 --tls 127.0.0.1:8443
```

對應一條 Gately 規則：

| 設定 | 值 |
| --- | --- |
| 協定 | TCP |
| 監聽位址 / 連接埠 | 留空 / `443` |
| 辨識逾時 | `2` |
| 路由 `ssh` | 協定 SSH，目標 `127.0.0.1`，連接埠 `22` |
| 路由 `vpn` | 協定 OpenVPN，目標 `127.0.0.1`，連接埠 `1194` |
| 路由 `web` | 協定 TLS，網域留空，目標 `127.0.0.1`，連接埠 `8443` |
| 路由 `vnc`（選用） | 協定「等待逾時」，目標 `127.0.0.1`，連接埠 `5900` |

sslh 在逾時後預設把連線交給 SSH；Gately 依 `SSH-` 版本識別字串立即辨識 SSH，不必等待，逾時的連線可以交給 VNC 這類伺服器先發話的服務。還可以繼續新增 RDP、SOCKS、HTTP 等路由，或依網域把 TLS 分給不同的後端。

## 從連接埠轉發升級

協定分流原先是連接埠轉發規則的「SNI 分流」模式。升級後首次啟動時，連接埠轉發中的 SNI 分流規則會自動移到協定分流模組，規則的 key、名稱、啟用狀態、分組和全部路由保持不變，原路由都成為 TLS 路由；連接埠轉發模組被關閉的，協定分流模組也一併關閉；擁有連接埠轉發權限的 API 權杖會同時取得協定分流的同級權限。

遷移後請改用 `/api/protoroute/rules` 管理這些規則：路由欄位由 `sniRoutes` 改名為 `routes`，不再使用 `mode` 欄位；向 `/api/portforward/rules` 提交分流規則會被拒絕。監控指標也改為 `gately_protoroute_*`，見[監控](/zh-TW/docs/reference/monitoring)。

## 部署提示

協定分流直接監聽主機的連接埠，Docker 部署建議使用 `--network host`。

## 分組與排序

規則可以填寫「分組」，清單的分組、排序與搜尋用法和[連接埠轉發](/zh-TW/docs/modules/portforward#分組與排序)相同；搜尋框可按名稱、分組、監聽連接埠、目標、網域或路由協定篩選。
