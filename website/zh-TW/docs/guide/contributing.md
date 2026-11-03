# 參與貢獻

Gately 是社群驅動的開源專案，歡迎提交 Issue 與 Pull Request。儲存庫：<https://github.com/wangwanjie/gately>

## 本機開發

需求：Go 1.26+、Node.js 24+、pnpm。

```bash
make web      # 建置前端（輸出到 web/dist，由 go:embed 嵌入）
make build    # 編譯 bin/gately
make test     # go test -race ./...
make lint     # gofmt、go vet、前端型別檢查
```

前端開發時執行 `cd web && pnpm dev`，`/api` 會代理到本機 `127.0.0.1:16680`。

## 貢獻約定

- 相容性程式碼只能建立在公開可觀察的行為、介面和設定格式上，請勿提交從第三方執行檔或前端產物中複製的程式碼。
- 新模組需要：設定結構與驗證、執行期啟停、REST 介面、單元測試；能從 Lucky 遷移的模組實作 `luckyimport.Importer`，希望在總覽頁顯示狀態的模組實作 `module.Summarizer`。
- 可選模組在 `internal/modules/registry/mod_<模組>.go` 中註冊，檔案帶 `//go:build !no_<模組>`，同時登記到 `catalog.go` 與根目錄的 `modules.json`（`TestManifestMatchesCode` 會檢查三者一致）；模組之間的協作（站台的 WAF、憑證部署到 SSH 與 Docker 等）透過注入的函式提供，缺少對方模組時回傳「未包含在此建置中」，核心程式碼不能直接引用可選模組的套件。CI 會以三個預設的 tag 組合執行 vet 與編譯。
- 程式碼註解使用中文；控制語句必須使用大括號。

## 額外測試

部分測試預設略過，需要額外條件：

| 測試 | 啟用方式 |
| --- | --- |
| ACME 端對端（Pebble） | 安裝 pebble 與 pebble-challtestsrv，設定 `GATELY_PEBBLE_BIN`、`GATELY_PEBBLE_SRC` |
| 真實公共 STUN 伺服器 | `GATELY_NET_TEST=1` |
| 用真實 Lucky 設定驗證匯入 | `GATELY_LUCKY_SAMPLE` 等，見各模組 `TestImportRealSample` |

## 授權條款

[Apache License 2.0](https://github.com/wangwanjie/gately/blob/main/LICENSE)

## 多語言

- 管理介面文案位於 `web/src/locales/<語言>/<命名空間>.json`，來源語言為簡體中文，新增或修改文案時至少同步更新 `zh-CN` 與 `en`，提交前執行 `cd web && node scripts/check-i18n.mjs`。約定詳見 `web/src/locales/README.md`。
- 官網語言包位於 `website/.vitepress/i18n/<語言>.json`，各語言頁面位於 `website/<語言目錄>/`。
- 後端執行期文案（錯誤說明、日誌、通知、介面文件）以原始碼裡的中文字面值為鍵，譯文在 `internal/i18n/locales/<語言>.json`。新增或修改中文文案後執行 `go run ./internal/i18n/cmd/i18nsync -root .`，把新鍵同步到各語言檔案並補齊譯文；`go test ./internal/i18n` 會檢查所有語言是否完整、格式動詞是否與原文一致。需要填入數值的句子請使用 `fmt.Sprintf` 的整句格式字串，讓各語言能依自己的語序翻譯；拼接多段中文會導致無法翻譯。
- 介面文件由程式依各語言產生：`make api-docs` 輸出到 `docs/api/<語言>/`，無需手動同步。
- 目前支援：简体中文、English、繁體中文、日本語、한국어、Español、Français、Deutsch、Русский、Português (Brasil)。
