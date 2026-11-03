# 驗證發佈檔案

Gately 的每個正式版本都帶有簽章與來源證明，可以確認下載到的檔案由本儲存庫的 GitHub Actions 建置，沒有被竄改。所有簽章都以 [Sigstore](https://www.sigstore.dev/) 無金鑰方式產生，沒有需要保管的私鑰。

## 執行檔壓縮檔

Release 頁面提供 `checksums.txt`（各壓縮檔的 SHA-256）、`checksums.txt.sigstore.json`（校驗和檔案的簽章）與 SBOM 檔案。

1. 驗證壓縮檔的 SHA-256：

```bash
sha256sum -c checksums.txt --ignore-missing
```

2. 用 [cosign](https://docs.sigstore.dev/cosign/installation/) 驗證 `checksums.txt` 的簽章，簽章身分必須是本儲存庫的發布工作流程：

```bash
cosign verify-blob checksums.txt \
  --bundle checksums.txt.sigstore.json \
  --certificate-identity-regexp '^https://github.com/wangwanjie/gately/\.github/workflows/release\.yml@refs/tags/v' \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com
```

3. 也可以用 GitHub CLI 驗證單一壓縮檔的建置來源證明：

```bash
gh attestation verify gately_v1.0.0_linux_amd64.tar.gz --repo wangwanjie/gately
```

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
