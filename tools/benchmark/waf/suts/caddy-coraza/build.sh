#!/usr/bin/env bash
# 构建 Caddy + coraza-caddy 镜像（在有 Go 与 Docker 的开发机上运行，压测机不需要执行）。
# 版本全部固定：Caddy v2.11.4、coraza-caddy v2.6.2、Coraza v3.8.1、coraza-coreruleset v4.25.0（即 CRS 4.25.0），
# 与 Gately v0.6.8.1 内置的 Coraza 与 CRS 版本完全相同。
# 用 go build 直接构建（等同 xcaddy 生成的 main.go），避免在部分 macOS 上运行 xcaddy 出现 dyld 报错。
# 用法：build.sh [平台列表]，默认 linux/amd64,linux/arm64；产物为本地镜像 bench/caddy-coraza:2.11.4-c2.6.2-<架构>，
# 本机架构另打不带后缀的 tag；amd64 镜像导出到 /tmp/caddy-coraza-amd64.tar 供传到压测机。
set -euo pipefail
TAG=bench/caddy-coraza:2.11.4-c2.6.2
PLATS=${1:-linux/amd64,linux/arm64}
WORK=$(mktemp -d /tmp/bx-caddy-coraza.XXXX)
mkdir -p "$WORK/src"
cat > "$WORK/src/main.go" <<'GO'
package main

import (
	caddycmd "github.com/caddyserver/caddy/v2/cmd"

	_ "github.com/caddyserver/caddy/v2/modules/standard"
	_ "github.com/corazawaf/coraza-caddy/v2"
)

func main() {
	caddycmd.Main()
}
GO
(
  cd "$WORK/src"
  go mod init caddy >/dev/null 2>&1
  # 显式固定版本，避免依赖解析把 Coraza 或规则集升级
  go get github.com/caddyserver/caddy/v2@v2.11.4 github.com/corazawaf/coraza-caddy/v2@v2.6.2 \
    github.com/corazawaf/coraza/v3@v3.8.1 github.com/corazawaf/coraza-coreruleset/v4@v4.25.0
  go mod tidy
  # 与 Gately v0.6.8.1 发布构建所用的 Go 版本一致
  go mod edit -toolchain=go1.26.11
)
for p in ${PLATS//,/ }; do
  arch=${p#linux/}
  mkdir -p "$WORK/$arch"
  # 交叉编译，无需 qemu
  (cd "$WORK/src" && GOTOOLCHAIN=go1.26.11 GOOS=linux GOARCH=$arch CGO_ENABLED=0 go build -trimpath -ldflags "-s -w" -o "$WORK/$arch/caddy" .)
  go version -m "$WORK/$arch/caddy" | grep -E "coraza|caddyserver/caddy/v2[[:space:]]" > "$WORK/$arch/versions.txt"
  cat "$WORK/$arch/versions.txt"
  cat > "$WORK/$arch/Dockerfile" <<DF
FROM caddy:2.11.4-alpine
COPY caddy /usr/bin/caddy
COPY versions.txt /versions.txt
DF
  # 只有 COPY，没有 RUN，跨架构构建不需要模拟器
  docker buildx build --platform "$p" --load -t "$TAG-$arch" "$WORK/$arch"
done
native=$(docker version -f '{{.Server.Arch}}')
if docker image inspect "$TAG-$native" >/dev/null 2>&1; then
  docker tag "$TAG-$native" "$TAG"
fi
if docker image inspect "$TAG-amd64" >/dev/null 2>&1; then
  docker save "$TAG-amd64" -o /tmp/caddy-coraza-amd64.tar
  echo "amd64 镜像已导出到 /tmp/caddy-coraza-amd64.tar；压测机上 docker load 后执行：docker tag $TAG-amd64 $TAG"
fi
rm -rf "$WORK"
