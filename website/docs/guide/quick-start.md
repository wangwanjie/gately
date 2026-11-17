# 快速开始

五分钟内完成部署、登录与第一条规则。

## 1. 启动

```bash
docker run -d --name gately --network host --cap-add NET_ADMIN \
  -v ./gately-data:/etc/gately \
  ghcr.io/wangwanjie/gately:latest
```

需要 Docker 管理时，在命令里加上 `-v /var/run/docker.sock:/var/run/docker.sock`。`--cap-add NET_ADMIN` 供入侵防护把封禁写入宿主机防火墙。端口转发、STUN、Web 服务需要监听宿主机端口，推荐使用 `--network host`。

访问 GHCR 较慢时，可以把镜像换成 Docker Hub 上的 `vanjayak/gately:latest`，两者是同一份镜像。

::: tip 飞牛 fnOS 用户
可以直接安装原生应用：经飞牛统一网关从桌面图标打开，并支持飞牛管理员登录态直接登录，见 [飞牛 fnOS 原生应用](/docs/guide/install#fnos)。
:::

## 2. 登录

浏览器打开 `http://<设备IP>:16680`，用户名 `admin`，默认密码 `gately.cn`。默认密码是公开的，登录后会弹出改密对话框，改成至少 10 位、不易猜到的新密码后才能继续使用。

全新安装改完密码后，会先询问「你打算用 Gately 做什么」：可多选「把内网服务发布成域名」「端口转发 / 打洞」「动态域名」「异地组网」「文件共享」「VPS 入口」或「全部功能」。只保留所选场景相关的功能模块，其余模块关闭（配置不删除），随后自动重启一次；之后随时可在「设置 → 功能模块」或侧边栏底部的「添加更多功能」里打开。选择「跳过」保留全部功能。升级的实例不会出现这一步。总览顶部的「开始使用」清单按所选场景列出接下来要做的事，并根据实际配置自动打勾，可收起或关闭。

## 3. 完成基础加固

登录后立即在「设置」中：

1. 开启两步验证（TOTP），或注册 Passkey；
2. 按需设置安全入口、仅内网访问或管理界面 HTTPS；
3. 查看「设置 → 账号与安全」顶部的安全评分，按提示处理扣分项。

更多建议见 [加固建议](/docs/guide/hardening)。

## 4. 添加第一条规则

Web 服务页的「快速建站」三步发布一个站点：填域名，填后端地址（可「测试连通」，也可从 Home Assistant、Jellyfin、Emby、Vaultwarden、Nextcloud、Immich、Proxmox VE、群晖 DSM、飞牛 fnOS 模板预填端口与推荐选项），再选 HTTP 或 HTTPS。监听自动复用或新建，详见 [Web 服务 · 快速建站](/docs/modules/web#quick-site)。

按需求选择一个模块开始：

| 需求 | 模块 |
| --- | --- |
| 家里的公网 IP 经常变化，需要固定域名访问 | [动态域名](/docs/modules/ddns) |
| 运营商没有给固定公网端口，想从外网访问家里的服务 | [STUN 穿透](/docs/modules/stun) |
| 把一个端口转发到内网另一台设备 | [端口转发](/docs/modules/portforward) |
| 用一个域名和 HTTPS 访问多个内网服务 | [SSL 证书](/docs/modules/ssl) + [Web 服务](/docs/modules/web) |
| 异地组网 | [Tailscale / frp](/docs/modules/tunnels) |

## 5. 下一步

按场景一步步做的教程：

- [没有公网 IPv4 怎么从外面访问 NAS](/docs/tutorials/remote-access-without-public-ip)：IPv6 + DDNS、STUN、frp、Cloudflare Tunnel 四条路线怎么选；
- [一个 443 端口跑所有服务](/docs/tutorials/one-port-443)：HTTPS、SSH、RDP 共用一个端口；
- [给 Jellyfin、Immich、Home Assistant、Vaultwarden 加 HTTPS 和登录](/docs/tutorials/https-and-login-for-apps)；
- [泛域名证书与动态域名，10 分钟配好](/docs/tutorials/wildcard-cert-ddns)；
- [从 nginx 或 Nginx Proxy Manager 迁移](/docs/tutorials/migrate-from-nginx)；
- [云服务器上替换 haproxy + nginx + fail2ban + acme.sh](/docs/tutorials/replace-haproxy-nginx-fail2ban)。
