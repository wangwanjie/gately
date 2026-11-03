---
title: 下载
description: 下载 Gately 最新版本的二进制、飞牛 fnOS 与 OpenWrt 安装包，或使用 Docker 镜像部署。
---

# 下载 Gately

## Docker 镜像（推荐）

镜像同时提供 amd64、arm64、arm/v7：

```bash
docker pull ghcr.io/wangwanjie/gately:latest
```

也可以从 Docker Hub 拉取同一份镜像（标签一致）：

```bash
docker pull vanjayak/gately:latest
```

完整的部署参数见 [安装部署](/docs/guide/install)。

## 二进制

下载对应平台的压缩包，解压后运行 `./gately -c /etc/gately`。支持 Linux（amd64 / arm64 / arm / mips / mipsle / riscv64）、macOS、Windows 与 FreeBSD。

<DownloadList />

::: tip 下载较慢？
GitHub 在部分网络环境下访问较慢，NAS 与软路由用户可以先通过代理拉取，或使用 Docker 镜像。
:::

## 飞牛 fnOS 原生应用 {#fnos}

飞牛 fnOS 用户可以直接安装原生应用：在上方列表的 fnOS 分组中下载 `.fpk`（Intel / AMD 选 `x86`，ARM 设备选 `arm`），然后在飞牛「应用中心 → 手动安装」中选择该文件。

- 支持飞牛统一网关：点击桌面图标即可打开，远程访问飞牛的域名同样可用，无需额外开放端口；
- 支持飞牛管理员登录态直接登录：开启后已登录飞牛的管理员无需再输入 Gately 密码与两步验证码。

详见 [安装部署 · 飞牛 fnOS 原生应用](/docs/guide/install#fnos)。

## OpenWrt 安装包 {#openwrt}

OpenWrt 用户可以在上方列表的 OpenWrt 分组中下载安装包：OpenWrt 24.10 及以前（含 iStoreOS）选 `.ipk`，执行 `opkg install` 安装；OpenWrt 25.12 起选 `.apk`，执行 `apk add --allow-untrusted` 安装。架构按 `/etc/openwrt_release` 中的 `DISTRIB_ARCH` 选择，安装后自动启动，配置保存在 `/etc/gately`。

同一分组中的 `luci-app-gately_<版本>_openwrt_all` 是 LuCI 插件，与架构无关，装好 Gately 后安装即可在 LuCI「服务 → Gately」中管理服务。

详见 [安装部署 · OpenWrt](/docs/guide/install#openwrt)。

## 从源码构建

依赖 Go 1.26+、Node.js 24+ 与 pnpm：

```bash
git clone https://github.com/wangwanjie/gately.git
cd gately
make web      # 构建前端
make build    # 输出 bin/gately
```

### 定制构建 {#custom-build}

可选模块与界面语言可以在编译时去掉（说明见 [模块总览 · 定制构建](/docs/modules/#custom-build)）。最省事的方式是在 [build.gately.cn](https://build.gately.cn) 勾选后在线生成。自行编译时：

- Go 用 build tag 去掉模块与语言：每个可选模块对应 `no_<模块>`（如 `no_waf`、`no_smb`），语言档位为 `lang_zh`（只有简体中文）或 `lang_cjken`（简繁英），不带 tag 即完整版；
- 前端用环境变量 `GATELY_LOCALES` 只打包所选语言（如 `zh-CN,zh-TW,en`），留空为全部；
- 仓库根目录的 `modules.json` 列出全部可选模块、依赖与体积估算，`go run ./tools/custombuild resolve` 把选择换算为 tag、语言与 Docker 构建参数，`build` 子命令直接生成 tar.gz 或飞牛安装包。

```bash
# 只保留 DDNS 与 frp，界面只有简体中文
go run ./tools/custombuild resolve -modules ddns,frp -lang zh
go run ./tools/custombuild build -version v0.6.4 -modules ddns,frp -lang zh -targets linux-arm64 -out dist
```

Docker 镜像用同一个 Dockerfile，构建参数 `GATELY_TAGS`、`GATELY_LOCALES` 对应上面的 tag 与语言，`WITH_DOCKER_CLI=0` 不安装 docker-cli 与 compose（未包含 Docker 模块时），`WITH_NFTABLES=0` 不安装 nftables（未包含入侵防护与 STUN 时）；`resolve -targets docker` 会输出对应的参数。

## 校验

每个版本附带校验和文件，下载后请核对 SHA-256，再运行。

从 v0.5.0 起，校验和文件与 Docker 镜像都带有 cosign 无密钥签名，压缩包附带构建来源证明，并提供 SBOM。校验方法见[校验发布产物](/docs/guide/verify)。
