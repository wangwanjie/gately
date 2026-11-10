# 檔案與維運工具

## WebDAV

在側邊欄「檔案服務」分組的「WebDAV 服務」中設定。

| 欄位 | 預設值 | 說明 |
| --- | --- | --- |
| `enable` | 關閉 | 啟用服務，啟用時至少需要一位使用者 |
| `listenIp` | 空 | 監聽位址，留空監聽所有位址 |
| `port` | 18800 | 監聽連接埠 |
| `tls` | 關閉 | 啟用 HTTPS，憑證由 [SSL 憑證](/zh-TW/docs/modules/ssl) 模組依 SNI 提供 |
| `users` | 無 | 多使用者，每位使用者包含 `name`、`password`、`root`（絕對路徑目錄）與 `readOnly`，或使用 `mounts` 掛載多個目錄 |
| `sharedUsers` | 關閉 | 允許[共用使用者](#shared-users)登入本服務 |

每位使用者可以對應到一個目錄，也可以掛載多個目錄：

- **單一目錄**：填寫 `root` 與 `readOnly`，登入後根目錄就是該目錄。
- **多個目錄**：在介面中點「掛載多個目錄」，或在 `mounts` 中填寫多項，每項包含 `name`（顯示名稱，不能含 `/`，同一使用者內不可重複）、`path`（絕對路徑）與 `readOnly`。登入後根目錄依名稱列出各目錄，根目錄本身無法寫入，檔案也無法在不同目錄之間移動。

## FTP

在「檔案服務」分組的「FTP 服務」中設定，支援 FTP 與 FTPS。

| 欄位 | 預設值 | 說明 |
| --- | --- | --- |
| `enable` | 關閉 | 啟用服務，啟用時至少需要一位使用者 |
| `listenIp` | 空 | 監聽位址，留空監聽所有位址 |
| `port` | 2121 | 監聽連接埠 |
| `passiveStart` / `passiveEnd` | 0 | 被動模式資料連接埠範圍，均為 0 時由系統隨機分配；填寫時需滿足 1 ≤ 起始 ≤ 結束 ≤ 65535，並在防火牆與連接埠對應中放行該範圍 |
| `publicHost` | 空 | 被動模式對外通告的 IP，位於 NAT 之後時填寫公網或轉發後的 IP，僅支援 IP 位址，留空自動取連線位址 |
| `tlsMode` | 0 | 0 為明文與 FTPS 均可，1 為必須使用顯式 FTPS（`AUTH TLS`），2 為隱式 FTPS（連線即加密）；非 0 時憑證由 [SSL 憑證](/zh-TW/docs/modules/ssl) 模組依 SNI 提供 |
| `idleTimeout` | 900 | 連線閒置多少秒後中斷 |
| `connectionTimeout` | 30 | 建立資料連線的逾時秒數 |
| `disableActiveMode` | 關閉 | 僅使用被動模式，更適合 NAT 環境 |
| `users` | 無 | 多使用者，欄位同 WebDAV：`name`、`password`、`root`、`readOnly` 或 `mounts`，使用者名稱不可重複 |
| `sharedUsers` | 關閉 | 允許[共用使用者](#shared-users)登入本服務 |

## SMB

在「檔案服務」分組的「SMB 服務」中設定，把本機目錄分享給 Windows 檔案總管、macOS Finder、Linux 以及手機、電視上的 SMB 用戶端。協定實作內建於 Gately（基於 [gosamba](https://github.com/ahmetozer/gosamba)），支援 SMB 2.0.2 ~ 3.1.1、NTLMv2 認證與簽章，可要求 SMB 3 加密，不支援 SMB 1。僅 Linux 與 macOS 上可用。

| 欄位 | 預設值 | 說明 |
| --- | --- | --- |
| `enable` | 關閉 | 啟用服務，啟用時至少需要一位使用者 |
| `listenIp` | 空 | 監聽位址，留空監聽所有位址 |
| `port` | 445 | 監聽連接埠。Windows 只能連線 445；飛牛、群暉等 NAS 內建的 Samba 已佔用 445 時需改用其他連接埠，macOS、Linux 與多數播放器可以在位址中指定連接埠 |
| `requireEncryption` | 關閉 | 要求所有連線使用 SMB 3 加密，只支援 SMB 2 的舊裝置與部分電視將無法連線 |
| `mdns` | 關閉 | 透過 Bonjour 廣播服務，macOS Finder 的「網路」中可以直接看到；Docker 部署需使用 host 網路才能廣播 |
| `users` | 無 | 多使用者，欄位同 WebDAV：`name`、`password`、`root`、`readOnly` 或 `mounts`，使用者名稱不區分大小寫且不可重複 |
| `sharedUsers` | 關閉 | 允許[共用使用者](#shared-users)登入本服務 |

共享依使用者劃分，使用者登入後只能看到並連線自己的共享：

- **單一目錄**：共享名稱為使用者名稱，例如使用者 `alice` 的位址為 `\\192.168.1.2\alice`。
- **多個目錄**：每個目錄是一個共享，`name` 即共享名稱（同一使用者內不區分大小寫、不重複），可分別設為唯讀。不同使用者可以有同名共享，各自指向自己的目錄。

使用者名稱與共享名稱不能包含 `" / \ [ ] : ; | = , + * ? < >`，也不能以 `.` 開頭或結尾。NTLM 認證需要明文密碼，密碼以 bcrypt 雜湊儲存的使用者需要重新設定密碼。檔案以 Gately 行程的身分讀寫。

頁面下方的「連線位址」依已儲存的設定列出每個共享的位址：

- Windows：檔案總管網址列輸入 `\\主機\共享名稱`，或執行 `net use Z: \\主機\共享名稱 /user:使用者名稱`
- macOS：Finder「前往 → 連接伺服器」，輸入 `smb://主機:連接埠/共享名稱`（445 連接埠可省略）
- Linux：`mount -t cifs //主機/共享名稱 /mnt -o username=使用者名稱,port=連接埠`

排查用戶端連不上或認證失敗時，可以設定環境變數 `GATELY_SMB_DEBUG=1` 後重新啟動 Gately，協定除錯日誌會寫入執行日誌；排查完成後移除該變數。

## 共用使用者 {#shared-users}

WebDAV、FTP 與 SMB 共用一份「共用使用者」，欄位與各服務的本機使用者相同（使用者名稱、密碼、`root` 與 `readOnly` 或 `mounts`），儲存在設定目錄的 `fileusers.json`。共用使用者在側邊欄「檔案服務」分組的「檔案共用使用者」頁面統一維護，儲存後三個服務同時生效；各服務原有的本機使用者不受影響。頁面上方列出各服務是否允許共用使用者登入，以及與本機使用者同名或無法用於 SMB 的共用使用者。

- 每個服務單獨決定是否接受共用使用者：在服務頁面「共用使用者」卡片中開啟「允許共用使用者登入」（欄位 `sharedUsers`）並儲存後，共用使用者即可登入該服務；卡片同時列出已儲存的共用使用者，點「管理共用使用者」進入「檔案共用使用者」頁面；
- 與本機使用者同名時只按本機使用者驗證（SMB 不區分大小寫）；
- 共用使用者名稱不區分大小寫不可重複，也不能包含冒號；
- 修改共用使用者立即生效：WebDAV 在下一個請求按新設定驗證，FTP 與 SMB 會重新啟動服務，已連線的用戶端需要重新連線；
- 開啟開關且至少有一個共用使用者時，沒有本機使用者也可以啟用服務；
- 只靠共用使用者啟用的服務，修改共用使用者後必須仍有可登入的共用使用者（SMB 要求能用於 SMB），否則拒絕儲存；
- `fileusers.json` 被手動改壞（如密碼為空、目錄不是絕對路徑）時，共用使用者都無法登入，頁面上會提示，修改後儲存即可恢復；
- SMB 只接受明文密碼，使用者名稱與共用名稱還要符合 SMB 的字元限制，不符合的共用使用者會在「檔案共用使用者」頁面、SMB 頁面與執行日誌中提示並略過，WebDAV 與 FTP 仍可使用。

介面為 `GET` / `PUT /api/fileusers`，API 權杖需要 `fileusers:read` / `fileusers:write` 權限，讀取結果中的 `services` 列出各服務的使用情況。原來各服務下的 `GET` / `PUT /api/<服務>/shared-users` 已棄用，仍可使用並讀寫同一份清單：讀取沿用所在服務的讀取權限，修改需要同時具備 `webdav:write`、`ftp:write` 與 `smb:write`。

**自助修改密碼**：在「檔案共用使用者」頁面開啟「自助修改密碼」並儲存後，共用使用者可以在任一開啟網頁認證的 Web 服務站點造訪 `https://站點網域/__gately_auth/password`，輸入使用者名稱、目前密碼和新密碼（8～128 個字元）修改自己的密碼，WebDAV、FTP、SMB 隨即使用新密碼（FTP 與 SMB 會重新啟動服務，已連線的用戶端需要重新連線）。原密碼為明文的仍以明文儲存，為 bcrypt 雜湊的以雜湊儲存；目前密碼錯誤與網頁登入共用限流與自動封鎖。開關儲存在 `fileusers.json` 的 `selfService` 欄位，介面欄位同名。網頁登入的共用使用者見 [Web 服務的自助修改密碼](/zh-TW/docs/modules/web#self-service-password)。

## File Browser {#filebrowser}

在「檔案服務」分組的「File Browser」中設定。Gately 託管 [FileBrowser Quantum](https://github.com/gtsteffaniak/filebrowser)：一個網頁檔案管理器，可以線上預覽圖片、影片與文件、上傳下載、產生分享連結，並有自己的多使用者與權限管理。原版 filebrowser/filebrowser 已於 2026 年封存、不再修復安全問題，因此託管的是仍在維護的 Quantum 分支。

程式不內建在 Gately 中。頁面下方的「線上安裝」卡片從 GitHub 下載 FileBrowser Quantum 到設定目錄的 `bin/filebrowser`，支援更新、安裝指定版本、單獨設定下載代理、回滾與解除安裝，更新或回滾後已啟用的實例自動重新啟動，詳見 [線上安裝、更新與回滾](/zh-TW/docs/modules/tunnels#install)。頁首的「恢復預設設定」會刪除全部實例及其資料，見 [恢復預設設定](/zh-TW/docs/modules/tunnels#reset)。也可以自行放置程式並填寫路徑。程式依以下順序尋找：填寫的路徑、設定目錄下的 `bin/`、`PATH`；找到的程式不是 Quantum（例如 `PATH` 中的原版）時頁面會提示。

每個實例是一個獨立的 File Browser 行程：

| 欄位 | 預設值 | 說明 |
| --- | --- | --- |
| `listenIp` | 空 | 監聽位址，留空監聽全部 IPv4 位址；只透過 Web 服務反向代理存取時可填 `127.0.0.1` |
| `port` | 8080 | 監聽連接埠 |
| `baseUrl` | `/` | 存取路徑，在 Web 服務中按路徑反向代理（例如 `/files`）時填寫相同的路徑 |
| `accessUrls` | 空 | 存取位址，可填多個，例如經 Web 服務反向代理的 `https://files.example.com/`，顯示在實例卡片上；留空時依監聽位址與連接埠推測。第一個位址的協定、主機與連接埠寫入 File Browser 的 `http.externalUrl`，分享連結使用該位址 |
| `sources` | 無 | 共享目錄，每項包含 `path`（絕對路徑）、`name`（顯示名稱，留空取目錄名）與 `readOnly`；新使用者預設可以存取全部目錄 |
| `authMethod` | `password` | 登入方式：`password` 帳號密碼，`proxy` 信任反向代理傳入的使用者名稱標頭，`noauth` 免登入 |
| `proxyHeader` | `Remote-User` | `proxy` 方式讀取使用者名稱的標頭，不存在的使用者自動建立 |
| `adminUser` | `admin` | 管理員使用者名稱 |
| `adminPassword` | 無 | 管理員密碼，至少 8 位，`password` 方式必填 |
| `trustProxyHeaders` | 關閉 | 信任 `X-Forwarded-*` 與 `X-Real-IP`，位於反向代理之後時開啟 |
| `disableWebDav` | 關閉 | 關閉 File Browser 內建的 WebDAV 介面 |
| `locale` | 空 | 新使用者的預設介面語言，如 `zh-tw`，留空為英文 |
| `tokenHours` | 2 | 登入有效期（小時） |

- 管理員密碼在每次啟動時同步到 File Browser，以 Gately 中的設定為準；其他使用者、分享連結與個人設定在 File Browser 中管理，儲存在實例自己的資料庫裡。
- 設定檔、資料庫與快取儲存在設定目錄的 `filebrowser/<實例 key>/` 下，設定檔權限 0600；管理員密碼透過環境變數傳給行程，不寫入檔案。刪除實例會一併刪除這個目錄，共享目錄中的檔案保持原樣。
- 共享目錄不存在時實例不會啟動，卡片上顯示原因。
- 對外存取建議在 [Web 服務](/zh-TW/docs/modules/web) 中新增反向代理站點指向 `http://127.0.0.1:連接埠`，用 HTTPS 與存取控制保護，並開啟「信任代理標頭」。`proxy` 方式要搭配 Web 服務的轉發驗證把使用者名稱放進標頭，File Browser 的連接埠不能對外開放，否則可以偽造標頭登入。
- 檔案以 Gately 行程的身分讀寫。

## 儲存掛載

在「檔案服務」分組的「儲存掛載」中新增掛載點，在頁面內線上管理檔案。

| 欄位 | 說明 |
| --- | --- |
| `name`、`remark`、`enable` | 名稱、備註與啟用開關 |
| `type` | `local`（本機目錄）、`webdav`（遠端 WebDAV）或 `smb`（SMB 共用） |
| `path` | 本機目錄，需為已存在的絕對路徑（`local` 使用） |
| `url`、`username`、`password` | 遠端位址與帳號，密碼在介面中遮蔽（`webdav`、`smb` 使用）。WebDAV 位址以 `http://` 或 `https://` 開頭；SMB 位址形如 `smb://主機[:連接埠]/共用名稱[/子目錄]`，支援 SMB2/3，使用者名稱留空時以來賓身分存取 |
| `skipVerify` | 略過 HTTPS 憑證驗證（`webdav` 使用） |
| `domain` | SMB 認證所用的網域或工作群組，通常留空（`smb` 使用） |
| `readOnly` | 唯讀掛載，檔案管理拒絕一切寫入操作 |

線上檔案管理支援瀏覽目錄、下載檔案、串流上傳（不把整個檔案載入記憶體）、新增目錄、重新命名和刪除。掛載點可作為備份工作的目標位置，備份檔案寫入掛載點中指定的目錄，並依保留策略清理舊檔案。

## 網路喚醒

在「網路喚醒」中管理裝置，向裝置發送魔術封包（6 位元組 `0xFF` 後接 16 次 MAC）。

| 欄位 | 預設值 | 說明 |
| --- | --- | --- |
| `name` | 無 | 裝置名稱，必填 |
| `macs` | 無 | MAC 位址，每行一個，至少一個，可為同一裝置填多個 |
| `broadcastIps` | 空 | IPv4 廣播位址，留空時向 `255.255.255.255` 及各啟用網卡的廣播位址發送 |
| `port` | 9 | 目標 UDP 連接埠 |
| `repeat` | 3 | 每個 MAC 重複發送次數，上限 20 |
| `checkAddr` | 空 | 上線偵測位址，格式 `主機:連接埠`，如 `192.168.1.10:3389`，留空不偵測 |
| `shutdownConn` | 空 | 遠端關機使用的 [Web 終端](/zh-TW/docs/modules/terminal) SSH 連線，留空不支援關機 |
| `shutdownCommand` | 無 | 關機命令，選擇了連線時必填，單行，如 `shutdown -h now`、`shutdown /s /t 0` |
| `remark` | 空 | 備註 |

在裝置清單上點擊喚醒即發送魔術封包，並依 `checkAddr` 顯示裝置是否在線。

### 遠端關機

為裝置選擇一個 Web 終端中的 SSH 連線並填寫關機命令後，裝置卡片上出現「關機」按鈕，介面為 `POST /api/wol/devices/{key}/shutdown`。

- 該連線必須已在 Web 終端中連線過一次、記錄了主機金鑰，Gately 不會無人值守地信任未知主機；
- 每次關機新建一條 SSH 連線，執行完即關閉，裝置重新開機後不會用到已失效的舊連線；命令最長執行 30 秒，同一裝置同時只執行一次；
- 關機常使 SSH 連線在返回結束碼之前中斷，這種情況視為已開始關機；
- 介面提供 Linux（`shutdown -h now`）、Windows（`shutdown /s /t 0`）與 macOS（`sudo -n shutdown -h now`）的常用命令。Windows 需開啟 OpenSSH 伺服器；Linux 非 root 使用者可改用 `sudo -n shutdown -h now` 並為該命令設定免密碼 sudo。

## 排程工作

依 cron 運算式定時執行命令或發起 HTTP 請求。

- `spec` 支援標準 5 段運算式、可選的秒欄位（6 段，如 `*/30 * * * * *`）以及 `@every 10m`、`@daily` 等描述符；
- `type` 為 `command` 時，透過 `sh -c`（Windows 為臨時 `.bat`）執行 `command`，環境變數中注入 `taskName` 及帶 `GATELY_` 前綴的同名大寫變數；
- `type` 為 `http` 時依 `http` 設定發起請求，欄位包括 `url`、`method`（GET、POST、PUT、PATCH、DELETE）、`headers`（每項 `Key: Value`）、`body`、`successContent`（回應本文需包含其中任意一項才算成功，為空時只檢查 2xx）、`timeout`、`insecureSkipVerify` 與 `proxy`（留空不指定代理，`update` 使用全域更新代理，或 `http://`、`https://`、`socks5://` 代理位址）；
- 工作可啟用或停用，頁面顯示下次執行時間，可手動立即執行；
- 同一工作上一次尚未結束時，本次觸發會被略過；
- 每個工作保留最近 20 次執行記錄，包含開始時間、耗時、成功與否、是否手動觸發，以及輸出（最多保留 4000 字元摘要）；
- 建立表單提供運算式解析驗證。

## 概覽與系統日誌

- 「概覽」頁顯示主機資訊：CPU 型號、使用率與負載，記憶體，磁碟，網卡位址與即時速率，公網 IPv4 與 IPv6（點擊複製），各模組的服務狀態，以及 Gately 程序的版本、提交、建置時間、啟動時間、PID、CPU、常駐記憶體、Go 堆積與 goroutine 數；
- Gately 託管外部程式（frp、Tailscale、NetBird、Cloudflare 隧道、File Browser）時，「概覽」頁另有「託管程序」卡片，逐一列出這些程序所屬的模組與實例、程式名、PID、執行時長、重新啟動次數、CPU 與常駐記憶體，卡片標題顯示合計，點擊一列進入對應模組；只統計程序本身，不含它再啟動的子程序，也不顯示路徑與啟動參數；
- 「系統日誌」頁彙整各模組日誌，包括 Docker、FTP、排程工作等模組的執行記錄。

## 主題

介面提供 15 個色系，分為 Gately 原創（翠綠、海洋、清新、櫻花、暮光、午夜、龐克）與經典配色（GitHub, Nord, Dracula, Catppuccin, Tokyo Night, Gruvbox, Rosé Pine, Solarized），每個色系都有亮色與暗色兩種版本。模式可選亮色、暗色或跟隨系統，跟隨系統時使用目前色系對應的亮色或暗色版本。主色可在任一色系上改為 10 種預設顏色之一，或用系統取色器自選，Logo 與登入頁漸層隨之變化，重新選擇色系會清除自訂主色；圓角可選直角、跟隨色系或圓潤。主題設定儲存在瀏覽器本機。
