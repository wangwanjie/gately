# 快速開始

五分鐘內完成部署、登入與第一條規則。

## 1. 啟動

```bash
docker run -d --name gately --network host --cap-add NET_ADMIN \
  -v ./gately-data:/etc/gately \
  ghcr.io/wangwanjie/gately:latest
```

需要 Docker 管理時，在指令裡加上 `-v /var/run/docker.sock:/var/run/docker.sock`。`--cap-add NET_ADMIN` 供入侵防護把封鎖寫入主機防火牆。連接埠轉發、STUN、Web 服務需要監聽主機的連接埠，建議使用 `--network host`。

存取 GHCR 較慢時，可以把映像檔換成 Docker Hub 上的 `vanjayak/gately:latest`，兩者是同一份映像檔。

::: tip 飛牛 fnOS 使用者
可以直接安裝原生應用：經飛牛統一閘道從桌面圖示開啟，並支援飛牛管理員登入狀態直接登入，見 [飛牛 fnOS 原生應用](/zh-TW/docs/guide/install#fnos)。
:::

## 2. 登入

瀏覽器開啟 `http://<裝置IP>:16680`，使用者名稱 `admin`，預設密碼 `gately.cn`。預設密碼是公開的，登入後會跳出修改密碼對話框，改成至少 10 位、不易猜到的新密碼後才能繼續使用。

全新安裝修改密碼後，會先詢問「你打算用 Gately 做什麼？」：可複選「把內網服務發佈成網域」「連接埠轉發 / 打洞」「動態網域」「異地組網」「檔案共享」「VPS 入口」或「全部功能」。只保留所選情境相關的功能模組，其餘模組關閉（設定不會刪除），隨後自動重新啟動一次；之後隨時可在「設定 → 功能模組」或側邊欄底部的「新增更多功能」中開啟。選擇「略過」保留全部功能。升級的實例不會出現這一步。總覽頂部的「開始使用」清單依所選情境列出接下來要做的事，並根據實際設定自動打勾，可收合或關閉。

## 3. 完成基礎強化

登入後立即在「設定」中：

1. 開啟兩步驟驗證（TOTP），或註冊 Passkey；
2. 視需求設定安全入口、僅限內網存取或管理介面 HTTPS；
3. 查看「設定 → 帳號與安全」頂端的安全評分，依提示處理扣分項目。

更多建議請見 [強化建議](/zh-TW/docs/guide/hardening)。

## 4. 新增第一條規則

Web 服務頁的「快速建站」三步發佈一個站台：填網域，填後端位址（可「測試連線」，也可從 Home Assistant、Jellyfin、Emby、Vaultwarden、Nextcloud、Immich、Proxmox VE、Synology DSM、飛牛 fnOS 範本預填連接埠與建議選項），再選 HTTP 或 HTTPS。監聽自動重用或新增，詳見 [Web 服務 · 快速建站](/zh-TW/docs/modules/web#quick-site)。

依需求選擇一個模組開始：

| 需求 | 模組 |
| --- | --- |
| 家裡的公網 IP 經常變動，需要固定網域存取 | [動態網域](/zh-TW/docs/modules/ddns) |
| 電信業者沒有提供固定公網連接埠，想從外網存取家裡的服務 | [STUN 穿透](/zh-TW/docs/modules/stun) |
| 把一個連接埠轉發到內網另一台裝置 | [連接埠轉發](/zh-TW/docs/modules/portforward) |
| 用一個網域和 HTTPS 存取多個內網服務 | [SSL 憑證](/zh-TW/docs/modules/ssl) + [Web 服務](/zh-TW/docs/modules/web) |
| 異地組網 | [Tailscale / frp](/zh-TW/docs/modules/tunnels) |
