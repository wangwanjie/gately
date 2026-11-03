# 校验发布产物

Gately 的每个正式版本都带有签名与来源证明，可以确认下载到的文件由本仓库的 GitHub Actions 构建，没有被篡改。所有签名使用 [Sigstore](https://www.sigstore.dev/) 无密钥方式生成，没有需要保管的私钥。

## 二进制压缩包

Release 页面提供 `checksums.txt`（各压缩包的 SHA-256）、`checksums.txt.sigstore.json`（对校验和文件的签名）与 SBOM 文件。

1. 校验压缩包的 SHA-256：

```bash
sha256sum -c checksums.txt --ignore-missing
```

2. 用 [cosign](https://docs.sigstore.dev/cosign/installation/) 校验 `checksums.txt` 的签名，签名身份必须是本仓库的发布工作流：

```bash
cosign verify-blob checksums.txt \
  --bundle checksums.txt.sigstore.json \
  --certificate-identity-regexp '^https://github.com/wangwanjie/gately/\.github/workflows/release\.yml@refs/tags/v' \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com
```

3. 也可以用 GitHub CLI 校验单个压缩包的构建来源证明：

```bash
gh attestation verify gately_v1.0.0_linux_amd64.tar.gz --repo wangwanjie/gately
```

## Docker 镜像

```bash
cosign verify ghcr.io/wangwanjie/gately:latest \
  --certificate-identity-regexp '^https://github.com/wangwanjie/gately/\.github/workflows/release\.yml@refs/tags/v' \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com
```

Docker Hub 上的 `vanjayak/gately` 是同一份镜像，签名身份相同：

```bash
cosign verify docker.io/vanjayak/gately:latest \
  --certificate-identity-regexp '^https://github.com/wangwanjie/gately/\.github/workflows/release\.yml@refs/tags/v' \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com
```

镜像仓库中还附带 SBOM 与构建来源证明，可以这样查看：

```bash
docker buildx imagetools inspect ghcr.io/wangwanjie/gately:latest --format '{{ json .SBOM }}'
```

## SBOM

Release 中的 `gately_<版本>_sbom.spdx.json` 为 SPDX 格式，列出二进制依赖的全部 Go 模块与前端依赖，可交给 `grype`、`trivy` 等工具做漏洞扫描。
