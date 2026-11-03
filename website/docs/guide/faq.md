# 常见问题

## 需要连接项目方的服务器吗？

无需连接。Gately 不连接任何项目方服务器，也不上报设备信息；默认会匿名向 GitHub 查询发布列表与安全公告，可以关闭，详见[自动检查更新](/docs/guide/install#update-check)。公网 IP 查询等对外地址使用第三方公共服务，可以在对应任务中修改。

## 默认账号和密码是什么？

默认账号 `admin`，默认密码 `gately.cn`。默认密码是公开的，登录后请立即在「设置 → 账号与安全」修改，修改之前管理界面顶部会一直提示。部署时设置了环境变量 `GATELY_INITIAL_PASSWORD` 的，以该密码为准。

## 忘记管理员密码怎么办？

在服务器上运行 `gately reset-password`（Docker 为 `docker exec -it gately gately reset-password`），输入 `yes` 确认后，密码重置为默认密码 `gately.cn`，运行中的实例几秒内生效。详见 [重置密码与两步验证](/docs/guide/install#reset)。

## 丢失了两步验证器怎么办？

在服务器上运行 `gately reset-totp`（Docker 为 `docker exec -it gately gately reset-totp`），确认后关闭两步验证，再用密码登录并重新启用。

## 支持飞牛 fnOS 吗？

支持。每个版本附带飞牛离线安装包（`.fpk`），在应用中心手动安装。它接入飞牛统一网关，点击桌面图标即可打开，远程访问飞牛的域名同样可用；在「设置 → 访问控制」开启「飞牛登录态自动登录」后，已登录的飞牛管理员可直接进入，无需再输入密码与两步验证码。详见 [飞牛 fnOS 原生应用](/docs/guide/install#fnos)。

## 支持 OpenWrt / iStoreOS 吗？

支持。每个版本附带 OpenWrt 安装包：OpenWrt 24.10 及以前（含 iStoreOS）用 `.ipk`，执行 `opkg install` 安装；OpenWrt 25.12 起用 `.apk`，执行 `apk add --allow-untrusted` 安装。安装后自动启动，配置保存在 `/etc/gately`，升级时安装新版安装包即可。程序约 32 MB，适合存储较大的软路由。详见 [OpenWrt](/docs/guide/install#openwrt)。

## 为什么推荐 `--network host`？

端口转发、STUN、Web 服务需要直接监听宿主机端口。使用桥接网络时，需要自行映射每一个用到的端口。

## Tailscale、frp、cloudflared 的程序从哪里来？

这些模块托管官方外部程序，Gately 本体不内置它们。在各模块页面的「在线安装」卡片中可以下载最新版或指定版本，支持单独设置下载代理，升级后有问题可以回滚到旧版本，也可以卸载程序或恢复默认配置，详见 [在线安装、更新与回滚](/docs/modules/tunnels#install)。Tailscale 只在 Linux 上支持在线安装。也可以自行放置程序，位置见 [配置与运行参数](/docs/guide/configuration)。

## STUN 规则启用后外网仍无法访问？

路由器 WAN 口位于运营商级 NAT（100.64.0.0/10）时，外部无法访问穿透端口，这是网络环境本身的限制。可改用 [Tailscale / frp / Cloudflare 隧道](/docs/modules/tunnels)。

## 如何反馈问题或参与开发？

在 [GitHub Issues](https://github.com/wangwanjie/gately/issues) 提交问题，开发约定见 [参与贡献](/docs/guide/contributing)。
