# 動態網域（DDNS）

家用寬頻的公網 IP 會變動，DDNS 在 IP 變動後自動更新 DNS 解析，讓網域始終指向目前的位址。

## 支援的服務商

阿里雲（Alibaba Cloud）、Cloudflare、騰訊雲、DNSPod、華為雲、GoDaddy、Porkbun、dynv6、DuckDNS、Amazon Route 53、OVHcloud、Gandi、NS1、Bunny DNS、Namecheap、DynDNS2，以及自訂回呼（用於未內建的服務商或自建介面）。

| 服務商 | 憑據 | 可用於憑證 DNS 驗證 |
| --- | --- | --- |
| Amazon Route 53 | Access Key ID / Secret Access Key，可選託管區域 ID | 是 |
| OVHcloud | Application Key / Secret 與 Consumer Key，接入點 ovh-eu / ovh-ca / ovh-us | 是 |
| Gandi | Personal Access Token | 是 |
| NS1 | API Key | 是 |
| Bunny DNS | Account API Key | 是 |
| Namecheap | 網域的 Dynamic DNS 密碼（僅 A 記錄） | 否 |
| DynDNS2 | 使用者名稱、密碼與更新網址，適用於 No-IP、Dynu 等相容 dyndns2 協定的服務 | 否 |

## 取得 IP 的方式

- **URL**：請求公網 IP 查詢位址（可自行替換為任意服務，或自建）；
- **網卡**：直接讀取指定網卡的位址，適合擁有公網 IP 的裝置；
- **命令**：執行命令並取其輸出，適合特殊網路環境。

支援 IPv4 與 IPv6。

## 記錄值變數 {#record-vars}

記錄值可以使用以下變數，每輪同步時替換為目前的值：

- `{ipv4Addr}`、`{ipv6Addr}`：本任務取得的 IPv4、IPv6 位址（A、AAAA 記錄留空時預設使用）；
- `{STUN_規則名_PORT}`、`{STUN_規則名_IP}`、`{STUN_規則名_ADDR}`：STUN 穿透規則目前的公網連接埠、IP、IP:連接埠，規則名中可以含底線。例如 TXT 記錄寫 `port={STUN_ssh_PORT}`，A 記錄寫 `{STUN_ssh_IP}`。

STUN 規則的公網位址變化後，引用它的任務會立即同步，不必等到下個檢查週期；規則未啟用或尚未穿透成功時，該記錄本輪略過，記錄狀態中會顯示原因。STUN 變數同樣可以在本任務的 Webhook 中使用。只在 Webhook 中引用時，位址變化後也會呼叫一次 Webhook。規則名稱也可以換成規則的 key。

## 通知

IP 變動或同步失敗時可呼叫 Webhook。

## 使用建議

- 為 DNS 服務商建立僅具備解析編輯權限的子帳號金鑰，不要使用主帳號金鑰；
- 新增工作後點擊「強制同步」確認記錄已更新；
- 不再需要的記錄可以點擊列表中該記錄右側的刪除圖示直接移除，無需進入編輯頁；只會從工作中移除，服務商處已有的解析保留，工作至少保留一筆記錄；
- 公網 IP 查詢位址不再依賴任何專案方服務，可以在工作中隨時更換。

## 分組與排序

任務可以填寫「分組」（最長 64 個字元）。清單按分組顯示可摺疊的區塊：分組按首次出現的順序排列，未分組的任務排在最後，摺疊狀態儲存在瀏覽器中；頁首的搜尋框可按名稱、分組、網域或服務商篩選。

點選「排序」進入排序模式：用箭頭上移、下移任務或整組移動；勾選多個任務後拖動其中任一項會一起移動，拖到其他分組的任務或分組標題上即加入該分組；工具列可以把選中的任務批次移到指定分組，留空則移出分組。順序和分組只影響清單顯示，修改後不會重新啟動任務。

滑鼠移到分組標題上，點鉛筆按鈕可以重新命名分組：組內條目一起改為新名稱，位置和順序不變；改成已有分組的名稱時，確認後兩個分組合併。
