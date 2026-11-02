# STUN 穿透

當電信業者沒有提供固定公網連接埠時，STUN 穿透透過 NAT 打洞取得公網對應位址與連接埠，讓外網可以存取內網服務。

## 能力

- TCP / UDP 穿透與保活；
- UPnP / NAT-PMP 自動在路由器上建立連接埠對應；
- 內建轉發，把穿透取得的公網連接埠直接轉發到內網目標；
- 公網位址或連接埠變動時觸發 Webhook 與指令碼，可用於更新 DNS、通知用戶端等。
- 內建轉發的來源 IP 存取控制（白名單 / 黑名單）。

## 搭配使用

STUN 取得的公網連接埠通常會變動，建議搭配：

- [動態網域](/zh-TW/docs/modules/ddns) 或 Webhook / 指令碼，把新位址同步到解析；動態網域的記錄值也可以直接寫 `{STUN_規則名_PORT}` 等變數，規則位址變化後立即同步，見 [記錄值變數](/zh-TW/docs/modules/ddns#record-vars)；
- [Web 服務](/zh-TW/docs/modules/web) 提供 HTTPS 入口。
- Web 服務的重新導向站台可以用 `{STUN_規則名_PORT}` 等變數引用規則目前的公網位址，連接埠變化後自動跳到新連接埠，見 [重新導向變數](/zh-TW/docs/modules/web#redirect-vars)。

## 限制與多層 NAT

- **電信業者級 NAT（CGNAT）**：路由器 WAN 口取得 `100.64.0.0/10` 等內網位址時，能否穿透取決於電信業者那層 NAT 的類型。全錐型（外部任意位址都能存取對應連接埠）可以穿透；連接埠受限或對稱型無法穿透，此時可改用 [Tailscale、frp 或 Cloudflare 通道](/zh-TW/docs/modules/tunnels)。
- **多層 NAT 下的 UPnP**：開啟 UPnP / NAT-PMP 時，Gately 會在路由器上把通道連接埠對應到本機；如果路由器的外網位址與 STUN 取得的公網位址不同，表示外層還有一層 NAT，公網連接埠屬於外層，Gately 只保留通道連接埠的對應，不再依公網連接埠追加對應。
- **固定本機連接埠並放行防火牆**：本機連接埠留空時每次啟動隨機選擇。主機開啟了防火牆（例如飛牛 fnOS 的系統防火牆）時，建議為規則固定本機連接埠，並在防火牆中放行該連接埠（可預留一段連接埠給 STUN 規則使用），否則入站連線會被防火牆丟棄，表現為穿透位址無法存取。
- **TCP 與路由器的錐形 NAT**：部分路由器的「NAT1 / 全錐形」只對 UDP 生效，TCP 出站時會改寫來源連接埠。TCP 規則需要開啟 UPnP，或在路由器上把通道連接埠對應到本機。

## 自動放行防火牆 {#auto-firewall}

規則開啟「自動放行防火牆」後，Gately 會在本機防火牆放行通道埠（填寫了 UPnP / NAT-PMP 內部埠時一併放行），規則停止或刪除時移除。適合 Gately 執行在 OpenWrt 等路由器上，或執行在開啟了防火牆的 Linux 主機上；只支援 Linux。

- nftables：在所有掛在 input 鉤子上的過濾鏈（例如 OpenWrt fw4、firewalld）最前面插入放行規則；
- iptables：在 filter 表的 INPUT 鏈最前面插入放行規則（例如 OpenWrt fw3、ufw）；
- 放行規則帶 `gately-open:` 註解；防火牆重新載入會清掉這些規則，Gately 每 30 秒檢查一次並補回；
- Docker 部署需要 host 網路與 `cap_add: NET_ADMIN`，規則卡片會顯示放行結果或失敗原因；
- 飛牛 fnOS 系統防火牆等不經過 nftables / iptables 的防火牆需要在系統設定中手動放行。

## 來源 IP 存取控制

使用內建轉發時，每條規則可以設定白名單或黑名單，依來源 IP 決定是否轉發到目標，名單與訂閱 IP 清單的用法和[連接埠轉發](/zh-TW/docs/modules/portforward#來源-ip-存取控制)相同，入侵防護的封鎖同樣優先於名單。名單只作用於轉發，STUN 查詢與通道有效性檢測不受影響；修改名單不會重新打洞，公網連接埠保持不變。UDP 依封包判斷。不使用內建轉發時外部連線不經過 Gately，名單不起作用。

## 部署提示

STUN 需要直接監聽主機的連接埠，Docker 部署建議使用 `--network host`。
