# 安裝部署

## Docker

映像檔位址 `ghcr.io/wangwanjie/gately`，同時發佈到 Docker Hub 的 `vanjayak/gately`（兩處為同一份映像檔，標籤一致），支援 amd64、arm64、arm/v7。存取 GHCR 不便時，把下文的映像檔名稱換成 `vanjayak/gately:latest` 即可，也可以搭配 Docker 的映像檔加速使用。

```bash
docker run -d --name gately --network host --cap-add NET_ADMIN \
  -v ./gately-data:/etc/gately \
  -v /var/run/docker.sock:/var/run/docker.sock \
  ghcr.io/wangwanjie/gately:latest
```

### Docker Compose

```yaml
services:
  gately:
    image: ghcr.io/wangwanjie/gately:latest
    container_name: gately
    restart: unless-stopped
    # 連接埠轉發、STUN 穿透、Web 服務都需要直接監聽主機的連接埠
    network_mode: host
    # 入侵防護把封鎖寫入主機防火牆（不需要可刪除）
    cap_add:
      - NET_ADMIN
    volumes:
      # 設定目錄（純文字 JSON，可納入 git 備份；ssl.json 中含憑證私鑰）
      - ./data:/etc/gately
      # Docker 管理（不需要可刪除）
      - /var/run/docker.sock:/var/run/docker.sock
    environment:
      - TZ=Asia/Taipei
```

`--cap-add NET_ADMIN` 與 `cap_add` 供[入侵防護](/zh-TW/docs/modules/guard)把封鎖寫入主機的 nftables 防火牆，不需要可以拿掉，封鎖仍對 Gately 自身的服務生效。

映像檔內建 `sh`、`bash` 與 `curl`，常見的通知指令碼可以直接執行。

## 執行檔

1. 從 [下載頁](/zh-TW/download) 取得對應平台的壓縮檔：Linux（amd64 / arm64 / arm / mips / mipsle / riscv64）、macOS、Windows、FreeBSD。
2. 解壓縮後執行：

```bash
./gately -c /etc/gately
```

### systemd

儲存庫提供完整的服務檔 [deploy/systemd/gately.service](https://github.com/wangwanjie/gately/blob/main/deploy/systemd/gately.service)，已授予監聽低位連接埠所需的能力：

```bash
sudo install -m 0755 gately /usr/local/bin/gately
sudo install -m 0644 gately.service /etc/systemd/system/gately.service
sudo systemctl daemon-reload && sudo systemctl enable --now gately
```

### OpenWrt {#openwrt}

每個版本附帶 OpenWrt 安裝包，包含程式、procd 啟動指令碼與刷寫韌體時保留設定的規則：

- OpenWrt 24.10 及更早版本（含 iStoreOS 等基於 24.10 的韌體）使用 `.ipk`，執行 `opkg install` 安裝；
- OpenWrt 25.12 起使用 `.apk`，執行 `apk add --allow-untrusted` 安裝（安裝包沒有簽章，需要加上 `--allow-untrusted`）。

先執行 `grep DISTRIB_ARCH /etc/openwrt_release` 查看裝置架構，再依下表在 [下載頁](/zh-TW/download) 的 OpenWrt 分組中選擇安裝包：

| DISTRIB_ARCH | 安裝包 |
| --- | --- |
| `x86_64` | `gately_<版本>_openwrt_amd64` |
| `aarch64_*` | `gately_<版本>_openwrt_arm64` |
| `arm_cortex-a*_*`（ARMv7，具備 VFP / NEON） | `gately_<版本>_openwrt_arm` |
| `mipsel_*` | `gately_<版本>_openwrt_mipsle` |
| `mips_*` | `gately_<版本>_openwrt_mips` |
| `riscv64_*` | `gately_<版本>_openwrt_riscv64` |

```bash
cd /tmp
# OpenWrt 24.10 / iStoreOS（以 x86_64 為例）
opkg install ./gately_<版本>_openwrt_amd64.ipk
# OpenWrt 25.12
apk add --allow-untrusted ./gately_<版本>_openwrt_amd64.apk
```

- 安裝後自動啟用開機自動啟動並啟動服務，管理介面為 `http://<路由器位址>:16680`。設定目錄為 `/etc/gately`，刷寫韌體（sysupgrade）時自動保留；
- 升級：下載新版安裝包，同樣執行 `opkg install` 或 `apk add --allow-untrusted`，設定保留。若停用了開機自動啟動，升級後維持停用；
- 解除安裝：`opkg remove gately` 或 `apk del gately`。設定目錄會保留，要徹底刪除請再執行 `rm -rf /etc/gately`；
- 安裝前會核對裝置架構。裝錯架構時 opkg 會中止安裝；OpenWrt 25.12 的 apk 仍會裝上但不啟動服務，請執行 `apk del gately` 後改用正確的安裝包；
- 程式約 32 MB，安裝需要同等大小的可用空間（以 `df -h /overlay` 查看），適合 x86 軟路由、ARM64 盒子等儲存空間較大的裝置，16 MB / 32 MB 快閃記憶體的路由器裝不下；
- arm 安裝包需要 ARMv7 且具備硬體浮點運算，ARMv5 / ARMv6 以及 `arm_cortex-a9` 這類沒有浮點運算單元的裝置暫不支援。

也可以手動部署：下載 Linux 壓縮檔，搭配 procd 啟動指令碼 [deploy/openwrt/gately.init](https://github.com/wangwanjie/gately/blob/main/deploy/openwrt/gately.init)：

```bash
cp gately /usr/bin/gately && chmod +x /usr/bin/gately
cp gately.init /etc/init.d/gately && chmod +x /etc/init.d/gately
/etc/init.d/gately enable && /etc/init.d/gately start
```

#### LuCI 外掛 {#openwrt-luci}

每個版本另附與架構無關的 LuCI 外掛 `luci-app-gately_<版本>_openwrt_all.ipk` / `.apk`，裝好 Gately 後再安裝：

```bash
# OpenWrt 24.10 / iStoreOS
opkg install ./luci-app-gately_<版本>_openwrt_all.ipk
# OpenWrt 25.12
apk add --allow-untrusted ./luci-app-gately_<版本>_openwrt_all.apk
```

安裝後在 LuCI 的「服務 → Gately」中可以：

- 查看執行狀態、版本、記憶體使用量與開機自動啟動狀態；
- 啟動、停止、重新啟動服務，啟用或停用開機自動啟動；
- 一鍵開啟 Gately 管理介面，連結自動帶上全域設定中的安全入口；
- 設定管理介面連接埠。連接埠儲存在 `/etc/config/gately`，填寫後替換 Gately 全域設定中監聽位址的連接埠（只監聽本機或內網位址的設定保持不變），連接埠衝突或改錯連接埠打不開管理介面時可以在這裡改回；留空則以全域設定為準。

其餘功能仍在 Gately 管理介面中設定。外掛介面跟隨 LuCI 的語言設定，支援簡體中文、繁體中文、英文、日文、韓文、德文、法文、西班牙文、俄文與葡萄牙文（巴西）。解除安裝時先執行 `opkg remove luci-app-gately` 或 `apk del luci-app-gately`，再解除安裝 gately。

## 飛牛 fnOS 原生應用 {#fnos}

每個版本附帶飛牛 fnOS 的離線安裝包：`gately_<版本>_fnos_x86.fpk`（Intel / AMD）與 `gately_<版本>_fnos_arm.fpk`（ARM）。安裝包沒有上架官方應用中心，需要手動安裝：

1. 從 [下載頁](/zh-TW/download#fnos) 取得與裝置架構對應的 `.fpk`；
2. 在飛牛開啟「應用中心 → 手動安裝」（左下角按鈕），選擇檔案並完成精靈；
3. 點擊飛牛桌面上的 Gately 圖示開啟管理介面。

### 支援飛牛統一閘道

桌面圖示經飛牛統一閘道開啟管理介面，位址為存取飛牛所用的位址加 `/app/gately/`，與飛牛共用同一個存取入口。區域網路位址、遠端存取飛牛的網域都能直接使用，無需額外開放或對應 16680 連接埠。

經閘道存取時由飛牛負責存取控制，只有飛牛管理員能開啟；安全入口與「僅允許內網存取」只約束直接存取 16680 連接埠。區域網路內仍可直接存取 `http://<NAS 位址>:16680`。

### 支援飛牛管理員登入狀態直接登入

在「設定 → 存取控制」開啟「飛牛登入狀態自動登入」後，已登入飛牛的管理員點擊桌面圖示即可直接進入 Gately，無需再輸入密碼與兩步驟驗證碼：

- 只有飛牛管理員會以 Gately 管理員身分登入，一般飛牛使用者仍需輸入帳號密碼；
- 只對經統一閘道的存取生效，直接存取 16680 連接埠時仍需正常登入；
- 主動登出後不會靜默重新登入，登入頁會顯示「使用飛牛帳號 &lt;使用者名稱&gt; 登入」按鈕；
- 該開關預設關閉，只在飛牛安裝包中顯示。

### 執行說明

- 應用以 `root` 身分執行，可以監聽 80/443、透過 `docker.sock` 管理 Docker、使用真實的 Web 終端機。請只安裝在自己掌控的 NAS 上。
- 設定儲存在應用的 `var` 目錄，解除安裝時預設保留；在解除安裝精靈中選擇「全部刪除」才會清除。
- 安裝包同時提供命令列入口 `/usr/local/bin/gately`，可使用 `gately ctl`、`gately mcp` 與 `gately reset-password`。
- 16680 連接埠與 Docker 映像檔相同，啟動原生應用前請先停止 `gately` 容器。

## 初始登入

管理介面預設監聽 `16680` 連接埠，預設帳號 `admin`，預設密碼 `gately.cn`。自動化部署時可用環境變數 `GATELY_INITIAL_PASSWORD` 指定初始密碼取代預設密碼。

預設密碼是公開的，仍在使用時管理介面頂端會一直提示修改。首次登入會出現啟動引導，依序提示修改密碼、開啟兩步驟驗證或 Passkey、執行網路自檢、從 Lucky 匯入與設定通知。

## 重設密碼與兩步驟驗證 {#reset}

忘記管理員密碼、遺失兩步驟驗證器（TOTP）時，在執行 Gately 的伺服器上執行指令重設。指令需要讀寫設定目錄，等同於擁有伺服器權限，因此無法透過網路使用：

- `gately reset-password`：將管理員密碼重設為預設密碼 `gately.cn`，用 `-p <新密碼>` 可直接指定新密碼；
- `gately reset-totp`：關閉管理員的兩步驟驗證，登入後可重新啟用。

執行前會顯示管理員使用者名稱和將要進行的操作，需要輸入 `yes` 確認，避免誤操作；指令碼等非互動環境加 `-y` 略過確認。執行中的 Gately 會在幾秒內自動生效，無需重新啟動；未在執行時，下次啟動時生效。重設後所有已登入的工作階段失效，登入失敗鎖定一併解除，使用者名稱、Passkey、唯讀帳號與其他設定保持不變。

| 安裝方式 | 指令 |
| --- | --- |
| Docker | `docker exec -it gately gately reset-password` |
| systemd | `sudo gately reset-password` |
| OpenWrt / iStoreOS | `gately reset-password` |
| 飛牛 fnOS | `sudo gately reset-password` |
| 其他 | `./gately reset-password -c <設定目錄>` |

未指定 `-c` 時依序尋找 `GATELY_CONFIG_DIR`、目前目錄下的 `data`、`/etc/gately` 與飛牛安裝包的設定目錄 `/var/apps/gately/var/config`，並顯示實際使用的目錄。重設操作會記入系統日誌與稽核日誌。

## 升級

### 自動檢查更新與安全提醒 {#update-check}

Gately 預設每 12 小時存取一次 GitHub，讀取 Gately 的發布列表與[已公開的安全公告](https://github.com/wangwanjie/gately/security/advisories)，並為已線上安裝的託管程式（frp、Cloudflare 通道、Tailscale、NetBird、File Browser）查詢上游最新版本。請求都是匿名 GET，不攜帶裝置、設定或使用情況等任何資訊。

- **新版本**：管理介面頂部顯示提示，可以查看更新內容，或忽略這個版本；
- **安全更新**：目前版本落在某個安全公告的受影響版本範圍內，或比目前版本新的發布說明帶有安全修正章節時，頂部以紅色提示並列出相關公告與版本。安全提示只能暫緩 24 小時，出現新的安全問題時立即重新提示；
- **通知**：發現新版本或新的安全問題時發布 `system.update` / `system.security` 事件，託管程式有新版本時發布 `program.update`，在[通知中心](/zh-TW/docs/modules/notify)用「版本更新」範本新增規則即可接收。同一版本、同一安全問題只通知一次，重新啟動後也不會重複。

檢查結果儲存在設定目錄的 `update-check.json`，升級後依新的版本重新判斷，無需再次檢查。啟動後約 3 分鐘進行首次檢查（距上次成功檢查不足 12 小時時等到滿 12 小時），失敗時 1 小時後重試；存取 GitHub 同樣經過下文的更新代理。在「關於」頁或「設定 → 網路與執行」中可以關閉自動檢查，關閉後只在點擊「檢查更新」時存取 GitHub。自動檢查只負責提示，不會下載或安裝任何版本。「檢查更新」按鈕只對管理員顯示；唯讀帳號與只有讀取權限的 API 權杖呼叫檢查介面時直接回傳上次的結果，不存取 GitHub。

### 線上升級 {#online-upgrade}

Linux 執行檔部署（systemd、PVE LXC 容器、手動部署的 OpenWrt 等）可以在管理介面的「關於」頁線上升級：點擊「檢查更新」，發現新版本後點擊「立即升級」。在 PVE 中用 OCI 映像檔建立的 LXC 容器同樣適用，無需刪除重建。Gately 會依序：

1. 從 GitHub 下載與本機平台對應的 `gately_<版本>_linux_<架構>.tar.gz`；
2. 對照同一版本的 `checksums.txt` 校驗 SHA-256；
3. 以 `-version` 試執行新程式，確認能在本機執行；
4. 替換可執行檔並以相同參數重新啟動，舊版本保留為同目錄下的 `gately.old`。

整個過程在對話框中進行：先展示該版本的更新內容，開始後顯示目前步驟、下載進度與速度。替換程式之前（取得版本、下載、校驗階段）可以隨時點擊「取消升級」，取消不會改動現有程式。

存取 GitHub 不穩定時，可以在「關於」頁或「設定 → 網路與執行」中設定**更新代理**，支援 `http://`、`https://` 與 `socks5://`，檢查更新與下載都經過該代理。留空時直連，仍會遵循 `HTTPS_PROXY` 環境變數。

無法連網時，從 [下載頁](/zh-TW/download) 取得 tar.gz，在「關於」頁點擊「上傳升級包」即可升級。上傳不限制版本高低，也可以用來回退到舊版本。

[自訂建置](/zh-TW/docs/modules/#custom-build)不提供線上升級：線上升級下載的是完整版本，會把去掉的模組裝回去。新版本發佈後請到 [build.gately.cn](https://build.gately.cn) 以相同選項（「關於」頁的連結已帶上）重新產生，再上傳升級包；Docker 自訂映像檔直接拉取新標籤。

線上升級只由管理員手動觸發，Gately 不會自動下載或安裝任何版本。新版本啟動異常時，可以用保留的舊版本手動回退（以 systemd 部署為例）：

```bash
mv /usr/local/bin/gately.old /usr/local/bin/gately
systemctl restart gately
```

用 `setcap` 給程式授予過低位埠權限的，替換後需要重新執行一次 `setcap`。

### 其他部署方式

- Docker：`docker pull` 新映像檔後重建容器（設定目錄維持掛載即可）；
- 飛牛 fnOS：在應用中心手動安裝新版 `.fpk`，設定自動保留；
- OpenWrt 安裝包：下載新版 `.ipk` / `.apk` 後執行 `opkg install` / `apk add --allow-untrusted`，設定自動保留（見 [OpenWrt](#openwrt)）；
- Windows、macOS、FreeBSD：替換可執行檔並重新啟動服務。

升級前建議匯出一份設定備份，或直接把設定目錄納入 git。下載的壓縮檔與映像檔可依 [驗證發佈檔案](/zh-TW/docs/guide/verify) 核對簽章。
