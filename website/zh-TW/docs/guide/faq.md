# 常見問題

## 需要連線到專案方的伺服器嗎？

不需要。Gately 不連線任何專案方伺服器，也不回報裝置資訊；預設會匿名向 GitHub 查詢發布列表與安全公告，可以關閉，詳見[自動檢查更新](/zh-TW/docs/guide/install#update-check)。公網 IP 查詢等對外位址使用第三方公共服務，可以在對應工作中修改。

## 預設帳號和密碼是什麼？

預設帳號 `admin`，預設密碼 `gately.cn`。預設密碼是公開的，登入後請立即在「設定 → 帳號與安全」修改，修改之前管理介面頂端會一直提示。部署時設定了環境變數 `GATELY_INITIAL_PASSWORD` 的，以該密碼為準。

## 忘記管理員密碼怎麼辦？

在伺服器上執行 `gately reset-password`（Docker 為 `docker exec -it gately gately reset-password`），輸入 `yes` 確認後，密碼會重設為預設密碼 `gately.cn`，執行中的實例幾秒內生效。詳見 [重設密碼與兩步驟驗證](/zh-TW/docs/guide/install#reset)。

## 遺失了兩步驟驗證器怎麼辦？

在伺服器上執行 `gately reset-totp`（Docker 為 `docker exec -it gately gately reset-totp`），確認後關閉兩步驟驗證，再用密碼登入並重新啟用。

## 支援飛牛 fnOS 嗎？

支援。每個版本附帶飛牛離線安裝包（`.fpk`），在應用中心手動安裝。它接入飛牛統一閘道，點擊桌面圖示即可開啟，遠端存取飛牛的網域同樣可用；在「設定 → 存取控制」開啟「飛牛登入狀態自動登入」後，已登入的飛牛管理員可直接進入，無需再輸入密碼與兩步驟驗證碼。詳見 [飛牛 fnOS 原生應用](/zh-TW/docs/guide/install#fnos)。

## 支援 OpenWrt / iStoreOS 嗎？

支援。每個版本附帶 OpenWrt 安裝套件：OpenWrt 24.10 及以前（含 iStoreOS）用 `.ipk`，執行 `opkg install` 安裝；OpenWrt 25.12 起用 `.apk`，執行 `apk add --allow-untrusted` 安裝。安裝後自動啟動，設定儲存在 `/etc/gately`，升級時安裝新版安裝套件即可。程式約 32 MB，適合儲存空間較大的軟路由。詳見 [OpenWrt](/zh-TW/docs/guide/install#openwrt)。

## 為什麼建議使用 `--network host`？

連接埠轉發、STUN、Web 服務需要直接監聽主機的連接埠。使用橋接網路時，需要自行對應每一個用到的連接埠。

## Tailscale、frp、cloudflared 的程式從哪裡來？

這些模組託管官方外部程式，Gately 本體並未內建它們。在各模組頁面的「線上安裝」卡片中可以下載最新版或指定版本，支援單獨設定下載代理，升級後有問題可以回滾到舊版本，也可以解除安裝程式或恢復預設設定，詳見 [線上安裝、更新與回滾](/zh-TW/docs/modules/tunnels#install)。Tailscale 只在 Linux 上支援線上安裝。也可以自行放置程式，位置請見 [設定與執行參數](/zh-TW/docs/guide/configuration)。

## STUN 規則啟用後外網仍無法存取？

路由器 WAN 口位於電信級 NAT（100.64.0.0/10）時，外部無法存取穿透連接埠，這是網路環境本身的限制。可改用 [Tailscale / frp / Cloudflare 通道](/zh-TW/docs/modules/tunnels)。

## 如何回報問題或參與開發？

在 [GitHub Issues](https://github.com/wangwanjie/gately/issues) 提交問題，開發約定請見 [參與貢獻](/zh-TW/docs/guide/contributing)。
