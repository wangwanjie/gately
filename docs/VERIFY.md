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

## macOS 代码签名

macOS 压缩包中的 `gately` 使用 Apple Developer ID 证书签名（团队 ID `X6B6C6U6QV`）并经过 Apple 公证，下载解压后可以直接运行。可以这样查看签名：

```bash
codesign -dv --verbose=2 gately
spctl -a -vv -t install gately
```

输出中应包含 `TeamIdentifier=X6B6C6U6QV` 与 `source=Notarized Developer ID`。首次运行时系统会联网查询公证记录。

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
