---
title: "下載"
description: "下載 Gately 最新版本的執行檔、飛牛 fnOS 與 OpenWrt 安裝包，或使用 Docker 映像檔部署。"
---

# 下載 Gately

## Docker 映像檔（建議）

映像檔同時提供 amd64、arm64、arm/v7：

```bash
docker pull ghcr.io/wangwanjie/gately:latest
```

也可以從 Docker Hub 拉取同一份映像檔（標籤一致）：

```bash
docker pull vanjayak/gately:latest
```

完整的部署參數請見 [安裝部署](/zh-TW/docs/guide/install)。

## 執行檔

下載對應平台的壓縮檔，解壓縮後執行 `./gately -c /etc/gately`。支援 Linux（amd64 / arm64 / arm / mips / mipsle / riscv64）、macOS、Windows 與 FreeBSD。

<DownloadList />

::: tip 下載速度慢？
GitHub 在部分網路環境下存取較慢，NAS 與軟路由使用者可以先透過代理拉取，或改用 Docker 映像檔。
:::

## 飛牛 fnOS 原生應用 {#fnos}

飛牛 fnOS 使用者可以直接安裝原生應用：在上方清單的 fnOS 分組中下載 `.fpk`（Intel / AMD 選 `x86`，ARM 裝置選 `arm`），然後在飛牛「應用中心 → 手動安裝」中選擇該檔案。

- 支援飛牛統一閘道：點擊桌面圖示即可開啟，遠端存取飛牛的網域同樣可用，無需額外開放連接埠；
- 支援飛牛管理員登入狀態直接登入：開啟後已登入飛牛的管理員無需再輸入 Gately 密碼與兩步驟驗證碼。

詳見 [安裝部署 · 飛牛 fnOS 原生應用](/zh-TW/docs/guide/install#fnos)。

## OpenWrt 安裝包 {#openwrt}

OpenWrt 使用者可以在上方清單的 OpenWrt 分組中下載安裝包：OpenWrt 24.10 及更早版本（含 iStoreOS）選 `.ipk`，執行 `opkg install` 安裝；OpenWrt 25.12 起選 `.apk`，執行 `apk add --allow-untrusted` 安裝。架構依 `/etc/openwrt_release` 中的 `DISTRIB_ARCH` 選擇，安裝後自動啟動，設定儲存在 `/etc/gately`。

同一分組中的 `luci-app-gately_<版本>_openwrt_all` 是 LuCI 外掛，與架構無關，裝好 Gately 後安裝即可在 LuCI「服務 → Gately」中管理服務。

詳見 [安裝部署 · OpenWrt](/zh-TW/docs/guide/install#openwrt)。

## 從原始碼建置

需要 Go 1.26+、Node.js 24+ 與 pnpm：

```bash
git clone https://github.com/wangwanjie/gately.git
cd gately
make web      # 建置前端
make build    # 輸出 bin/gately
```

### 自訂建置 {#custom-build}

可選模組與介面語言可以在編譯時去掉（說明見 [模組總覽 · 自訂建置](/zh-TW/docs/modules/#custom-build)）。最省事的方式是在 [build.gately.cn](https://build.gately.cn) 勾選後線上產生。自行編譯時：

- Go 以 build tag 去掉模組與語言：每個可選模組對應 `no_<模組>`（如 `no_waf`、`no_smb`），語言檔位為 `lang_zh`（只有簡體中文）或 `lang_cjken`（簡繁英），不帶 tag 即完整版；
- 前端以環境變數 `GATELY_LOCALES` 只打包所選語言（如 `zh-CN,zh-TW,en`），留空為全部；
- 儲存庫根目錄的 `modules.json` 列出全部可選模組、依賴與體積估算，`go run ./tools/custombuild resolve` 把選擇換算為 tag、語言與 Docker 建置參數，`build` 子命令直接產生 tar.gz 或飛牛安裝包。

```bash
# 只保留 DDNS 與 frp，介面只有簡體中文
go run ./tools/custombuild resolve -modules ddns,frp -lang zh
go run ./tools/custombuild build -version v0.6.4 -modules ddns,frp -lang zh -targets linux-arm64 -out dist
```

Docker 映像檔使用同一個 Dockerfile，建置參數 `GATELY_TAGS`、`GATELY_LOCALES` 對應上面的 tag 與語言，`WITH_DOCKER_CLI=0` 不安裝 docker-cli 與 compose（未包含 Docker 模組時），`WITH_NFTABLES=0` 不安裝 nftables（未包含入侵防護與 STUN 時）；`resolve -targets docker` 會輸出對應的參數。

## 校驗

每個版本都附有校驗碼檔案，下載後請先核對 SHA-256 再執行。

自 v0.5.0 起，校驗碼檔案與 Docker 映像都附有 cosign 無金鑰簽章，壓縮檔附帶建置來源證明，並提供 SBOM。驗證方法見[驗證發佈檔案](/zh-TW/docs/guide/verify)（英文）。
