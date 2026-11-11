# WAF 防火牆

基於 [Coraza](https://coraza.io/) 與 OWASP CRS v4 的 Web 應用程式防火牆，可掛接到 [Web 服務](/zh-TW/docs/modules/web) 的站台。

## 能力

- 內建 OWASP CRS v4 規則集，可依規則檔案啟用；
- 支援自訂 SecLang 規則；
- 規則組可調整順序；命中日誌記錄用戶端 IP、網域、路徑與規則 ID，可在日誌中按網域、IP 或關鍵字篩選；
- 攔截模式與僅偵測模式；
- 請求內容檢查上限可調，超過上限時拒絕請求或只檢查前段；可依路徑略過請求內容檢查；
- WebDAV 相容模式：放行網盤、WebDAV 用到的方法、請求標頭與任意上傳內容類型；
- 可接入自行部署的雷池（SafeLine）偵測引擎，單獨使用或與規則組同時使用；
- 多個實例，站台視需要引用。

## 使用建議

1. 新站台先使用「僅偵測」，觀察一段時間的日誌，確認沒有誤判；
2. 針對誤判調整規則檔案或新增自訂例外；
3. 再切換為「攔截」。

## 管理後台的請求

管理後台經掛了 WAF 的站點反向代理存取時，帶本實例有效管理員登入狀態的請求不經過 WAF（含雷池）。儲存設定的請求本文裡常有檔案路徑（如 nginx 匯入寫入的備註）、HTML（自訂錯誤頁）、指令碼（憑證部署）等內容，會被 CRS 的路徑穿越、XSS、命令注入規則誤攔成 403。登入狀態由伺服器簽章，只認管理員角色；唯讀帳號、訪客和其他實例的管理後台請求仍照常檢查。

## 已登入使用者

站點開啟「信任已登入使用者」（見 [Web 服務](./web)）後，已通過該站點 Gately 驗證的訪客（網頁驗證、允許清單中逐一列出的第三方登入身分、共享使用者單一登入、驗證過的基本驗證）不經過 WAF。未登入訪客、登入頁本身的請求、經 `github:*` 這類萬用字元允許的第三方登入身分、轉發驗證與分享連結仍照常檢查。

## 規則組順序

實例裡的規則組從上到下載入，可用上移、下移按鈕調整。同一階段內排在前面的規則先執行：

- 攔截規則（deny 等）放在哪裡都會生效；
- 執行期的放行或排除規則（`allow`、`ctl:ruleRemoveById`、`ctl:ruleRemoveTargetById` 等）要放在 OWASP CRS 規則組之前，否則請求可能已被 CRS 攔截；
- `SecRuleRemoveById`、`SecRuleUpdateTargetById` 這類設定期指令只作用於已經載入的規則，要放在 CRS 規則組之後，放在之前不起作用。

## 上傳大型檔案的站台

WAF 預設只檢查不超過 12.5 MB 的請求內容，更大的請求會被拒絕（413），網盤、WebDAV、檔案管理這類站台因此無法上傳大型檔案。在實例的「請求內容檢查」中可以調整：

- **檢查上限**：0 表示預設的 12.5 MB，最大 1024 MB；
- **超過上限時**：「拒絕請求」與先前一致；「只檢查前段」只檢查不超過上限的部分，其餘內容原樣轉發給後端，適合上傳大型檔案的站台；
- **相容 WebDAV 與上傳**：OWASP CRS 預設只允許 GET、HEAD、POST、OPTIONS，並攔截 `Lock-Token`、`If`、`Content-Range` 請求標頭與白名單以外的內容類型，WebDAV 用戶端因此無法列出目錄、上傳和加鎖。開啟後放行 PUT、DELETE、PROPFIND、PROPPATCH、MKCOL、COPY、MOVE、LOCK、UNLOCK 等方法與這些請求標頭，不再限制上傳的內容類型；PUT 與 `application/octet-stream` 的請求內容是檔案內容，不再按表單參數檢查。注入、路徑穿越等攻擊檢查照常生效；
- **略過請求內容的路徑**：每行一個路徑前綴（如 `/api/upload`、`/remote.php/dav/`），符合的請求不檢查請求內容，請求標頭、URL 與參數仍照常檢查。

建議為網盤、WebDAV 這類站台單獨建立一個實例：開啟「相容 WebDAV 與上傳」並選擇「只檢查前段」，必要時把上傳介面加入略過路徑，其他站台繼續使用預設設定。

curl、rclone 等用戶端上傳較大的內容時會帶 `Expect: 100-continue` 請求標頭，CRS 4 預設會攔截它。Gately 的所有實例都放行該請求標頭（由 Gately 自行處理 100-continue），不需要額外設定。

## 接入雷池（SafeLine）

[雷池](https://waf-ce.chaitin.cn/)是長亭科技的 Web 應用程式防火牆，靠語意分析引擎識別攻擊，誤判較少。Gately 可以把請求送到你自行部署的雷池偵測引擎（safeline-detector）偵測，依結果放行或攔截；入口、憑證、認證與封鎖仍由 Gately 負責，雷池不必再當一層反向代理。

1. 依雷池官方文件安裝社群版，預設安裝目錄為 `/data/safeline`。
2. 選擇連線偵測引擎的方式：
   - **Unix 通訊端**（Gately 與雷池在同一台機器上）：偵測引擎預設監聽 `/data/safeline/resources/detector/snserver.sock`，位址填 `unix:/data/safeline/resources/detector/snserver.sock`。Gately 執行在 Docker 中時，把 `/data/safeline/resources/detector` 目錄掛載進 Gately 容器，位址填容器內的路徑。
   - **TCP**：在 `/data/safeline/resources/detector/detector.yml` 中寫入下面兩行，再執行 `docker restart safeline-detector`。改為 TCP 後偵測引擎不再監聽 Unix 通訊端。

     ```yaml
     bind_addr: 0.0.0.0
     listen_port: 8000
     ```

     位址填 safeline-detector 容器的 IP 與連接埠，例如 `172.22.222.5:8000`，容器 IP 可以用 `docker inspect safeline-detector | grep IPAddress` 查看。Gately 執行在 Docker 的 bridge 網路中時，用 `docker network connect safeline-ce <Gately 容器名稱>` 把它接入雷池的網路。偵測引擎沒有認證，不要把 8000 連接埠暴露到公網。
3. 在 WAF 實例中開啟「接入雷池」，填寫位址後點「測試連線」。Gately 先傳送心跳，再送檢一個範例 SQL 注入請求，提示「範例攻擊已被識別」表示偵測引擎運作正常。這條範例攻擊會出現在雷池控制台的攻擊記錄中，來源為 127.0.0.1，網域為 gately-test.invalid。
4. 在 Web 服務的站點中引用這個實例。

說明：

- 接入雷池後規則組可以留空；同時保留規則組時，請求先經雷池偵測，再經規則組檢查。「僅偵測」模式下，雷池識別出的攻擊只寫日誌，請求照常放行。
- 被攔截的請求：瀏覽器看到帶事件 ID 的攔截頁，其他用戶端得到與雷池官方外掛相同的 JSON（含 `event_id`）。狀態碼採用偵測引擎給出的值，通常為 403。攔截計入存取日誌（`deny=waf`）、站點統計與 WAF 攔截事件，入侵防護的「WAF 攔截」預設同樣生效。
- 攻擊記錄也會寫入雷池控制台，可以依事件 ID 查到；這些記錄不歸屬雷池中的任何站點。
- 送檢的請求本文預設最多 1024 KB，超出部分不送檢、原樣轉發給後端。「略過請求內容的路徑」與「相容 WebDAV 與上傳」對雷池同樣生效；經 HTTP/2 傳輸、長度未知的 gRPC 請求本文不送檢。
- 偵測逾時預設 1 秒。偵測引擎無法使用（連線失敗、逾時）時預設放行請求，實例顯示「雷池無法使用」並寫入錯誤日誌；選擇「拒絕請求」後，這類請求回傳 503。偵測引擎剛啟動的幾秒內可能回傳無法識別的結果，同樣視為無法使用。
- 只偵測請求，不送檢回應。

## 驗證

WAF 使用真實攻擊載荷測試，涵蓋路徑穿越等攻擊的攔截。
