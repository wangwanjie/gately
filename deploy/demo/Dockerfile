# 在线演示镜像：官方 Gately 程序 + 演示数据生成器。构建上下文为仓库根目录：
#   docker build -f deploy/demo/Dockerfile -t gately-demo .
# 基础镜像可用构建参数 GATELY_IMAGE 指定版本，例如 ghcr.io/wangwanjie/gately:v0.6.8
ARG GATELY_IMAGE=ghcr.io/wangwanjie/gately:latest

FROM golang:1.26.11-alpine AS gen
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY internal ./internal
COPY tools/demo ./tools/demo
RUN CGO_ENABLED=0 go build -trimpath -ldflags "-s -w" -o /out/demo-gen ./tools/demo

FROM ${GATELY_IMAGE}
COPY --from=gen /out/demo-gen /usr/local/bin/demo-gen
COPY deploy/demo/entrypoint.sh /usr/local/bin/demo-entrypoint
# 非 root 用户：管理端口 16680 无需特权
USER 10001:10001
ENV GATELY_DEMO=1 GATELY_CONFIG_DIR=/data
EXPOSE 16680
ENTRYPOINT ["/usr/local/bin/demo-entrypoint"]
