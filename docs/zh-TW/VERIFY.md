# 驗證發佈檔案

Gately 的每個正式版本都附有簽章與來源證明，可以確認下載到的檔案由本儲存庫的 GitHub Actions 建置，沒有遭到竄改。`checksums.txt` 附有 minisign 簽章，對應的公鑰內建於 Gately 程式中，線上升級用它驗證下載內容；cosign 簽章與建置來源證明使用 [Sigstore](https://www.sigstore.dev/) 無金鑰方式產生。

## 執行檔壓縮檔

Release 頁面提供 `checksums.txt`（各壓縮檔的 SHA-256）、`checksums.txt.minisig`（校驗和檔案的 minisign 簽章）、`checksums.txt.sigstore.json`（校驗和檔案的 cosign 簽章）與 SBOM 檔案。簽章上線之前發布的版本沒有 `checksums.txt.minisig`。

1. 用 [minisign](https://jedisct1.github.io/minisign/) 驗證 `checksums.txt` 的簽章。Gately 發布公鑰（金鑰 ID `512082B4037581F0`）：

```text
RWTwgXUDtIIgUVNxL3scLhMObpyS0vKQbkCepGauqNA0/u0nQA86M50/
```

```bash
minisign -Vm checksums.txt -P RWTwgXUDtIIgUVNxL3scLhMObpyS0vKQbkCepGauqNA0/u0nQA86M50/
```

輸出 `Signature and comment signature verified` 表示簽章有效，`Trusted comment` 一行寫有版本號與簽章時間。

2. 驗證壓縮檔的 SHA-256：

```bash
sha256sum -c checksums.txt --ignore-missing
```

3. 用 [cosign](https://docs.sigstore.dev/cosign/installation/) 驗證 `checksums.txt` 的簽章，簽章身分必須是本儲存庫的發布工作流程：

```bash
cosign verify-blob checksums.txt \
  --bundle checksums.txt.sigstore.json \
  --certificate-identity-regexp '^https://github.com/wangwanjie/gately/\.github/workflows/release\.yml@refs/tags/v' \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com
```

4. 也可以用 GitHub CLI 驗證單一壓縮檔的建置來源證明：

```bash
gh attestation verify gately_v1.0.0_linux_amd64.tar.gz --repo wangwanjie/gately
```

## 線上升級的簽章驗證

Linux 執行檔部署的線上升級（「立即升級」）會先下載 `checksums.txt` 與 `checksums.txt.minisig`，用內建於程式的公鑰驗證簽章，通過後再比對升級包的 SHA-256：

- 簽章無效、來自未知金鑰或 `checksums.txt` 遭到修改：拒絕升級，升級視窗與系統日誌顯示原因，現有程式保持不變；
- 該版本沒有 `checksums.txt.minisig`（簽章上線之前發布）：照常升級，升級視窗與系統日誌提示該版本未簽章、僅核對了 SHA-256。

程式預留了第二個公鑰位置用於更換金鑰：新公鑰會先隨一個版本內建於程式並在本頁公布，之後的版本才改用新金鑰簽章，已部署的程式可以照常升級。「上傳升級包」不驗證簽章，上傳前可依上面的步驟手動驗證；Docker 映像檔的升級依靠下文的 cosign 映像檔簽章。

## macOS 程式碼簽章

macOS 壓縮檔中的 `gately` 使用 Apple Developer ID 憑證簽章（團隊 ID `X6B6C6U6QV`）並經過 Apple 公證，下載解壓後可以直接執行。可以這樣查看簽章：

```bash
codesign -dv --verbose=2 gately
spctl -a -vv -t install gately
```

輸出中應包含 `TeamIdentifier=X6B6C6U6QV` 與 `source=Notarized Developer ID`。首次執行時系統會連網查詢公證紀錄。

## Docker 映像檔

```bash
cosign verify ghcr.io/wangwanjie/gately:latest \
  --certificate-identity-regexp '^https://github.com/wangwanjie/gately/\.github/workflows/release\.yml@refs/tags/v' \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com
```

Docker Hub 上的 `vanjayak/gately` 是同一份映像檔，簽章身分相同：

```bash
cosign verify docker.io/vanjayak/gately:latest \
  --certificate-identity-regexp '^https://github.com/wangwanjie/gately/\.github/workflows/release\.yml@refs/tags/v' \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com
```

映像檔倉庫中還附帶 SBOM 與建置來源證明，可以這樣檢視：

```bash
docker buildx imagetools inspect ghcr.io/wangwanjie/gately:latest --format '{{ json .SBOM }}'
```

## SBOM

Release 中的 `gately_<版本>_sbom.spdx.json` 為 SPDX 格式，列出執行檔依賴的全部 Go 模組與前端相依套件，可交給 `grype`、`trivy` 等工具做漏洞掃描。
