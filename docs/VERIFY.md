# 校验发布产物

Gately 的每个正式版本都带有签名与来源证明，可以确认下载到的文件由本仓库的 GitHub Actions 构建，没有被篡改。`checksums.txt` 带有 minisign 签名，对应的公钥编进了 Gately 程序，在线升级用它验证下载内容；cosign 签名与构建来源证明使用 [Sigstore](https://www.sigstore.dev/) 无密钥方式生成。

## 二进制压缩包

Release 页面提供 `checksums.txt`（各压缩包的 SHA-256）、`checksums.txt.minisig`（校验和文件的 minisign 签名）、`checksums.txt.sigstore.json`（校验和文件的 cosign 签名）与 SBOM 文件。签名上线之前发布的版本没有 `checksums.txt.minisig`。

1. 用 [minisign](https://jedisct1.github.io/minisign/) 校验 `checksums.txt` 的签名。Gately 发布公钥（密钥 ID `512082B4037581F0`）：

```text
RWTwgXUDtIIgUVNxL3scLhMObpyS0vKQbkCepGauqNA0/u0nQA86M50/
```

```bash
minisign -Vm checksums.txt -P RWTwgXUDtIIgUVNxL3scLhMObpyS0vKQbkCepGauqNA0/u0nQA86M50/
```

输出 `Signature and comment signature verified` 表示签名有效，`Trusted comment` 一行写有版本号与签名时间。

2. 校验压缩包的 SHA-256：

```bash
sha256sum -c checksums.txt --ignore-missing
```

3. 用 [cosign](https://docs.sigstore.dev/cosign/installation/) 校验 `checksums.txt` 的签名，签名身份必须是本仓库的发布工作流：

```bash
cosign verify-blob checksums.txt \
  --bundle checksums.txt.sigstore.json \
  --certificate-identity-regexp '^https://github.com/wangwanjie/gately/\.github/workflows/release\.yml@refs/tags/v' \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com
```

4. 也可以用 GitHub CLI 校验单个压缩包的构建来源证明：

```bash
gh attestation verify gately_v1.0.0_linux_amd64.tar.gz --repo wangwanjie/gately
```

## 在线升级的签名验证

Linux 二进制部署的在线升级（「立即升级」）先下载 `checksums.txt` 与 `checksums.txt.minisig`，用编进程序的公钥验证签名，通过后再比对升级包的 SHA-256：

- 签名无效、来自未知密钥或 `checksums.txt` 被改动：拒绝升级，升级弹窗与系统日志显示原因，现有程序保持不变；
- 该版本没有 `checksums.txt.minisig`（签名上线之前发布）：照常升级，升级弹窗与系统日志提示该版本未签名、只核对了 SHA-256。

程序预留了第二个公钥位用于更换密钥：新公钥先随一个版本编进程序并在本页公布，之后的版本才改用新密钥签名，已部署的程序可以照常升级。「上传升级包」不验证签名，上传前可按上面的步骤手动校验；Docker 镜像的升级依靠下文的 cosign 镜像签名。

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
