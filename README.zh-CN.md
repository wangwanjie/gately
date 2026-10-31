<p align="center">
  <a href="https://gately.cn/"><img src="docs/images/banner.zh-CN.svg" alt="Gately — 一站式网络网关，开源透明，开箱即用" width="100%"></a>
</p>

<p align="center">
  <a href="https://github.com/wangwanjie/gately/releases/latest"><img src="https://img.shields.io/github/v/release/wangwanjie/gately?style=flat-square&color=0e7c66&label=release" alt="Release"></a>
  <a href="https://github.com/wangwanjie/gately/pkgs/container/gately"><img src="https://img.shields.io/badge/docker-ghcr.io%2Fwangwanjie%2Fgately-0e7c66?style=flat-square&logo=docker&logoColor=white" alt="Docker"></a>
  <a href="LICENSE"><img src="https://img.shields.io/github/license/wangwanjie/gately?style=flat-square&color=0e7c66" alt="License"></a>
  <a href="https://github.com/wangwanjie/gately/stargazers"><img src="https://img.shields.io/github/stars/wangwanjie/gately?style=flat-square&color=0e7c66" alt="Stars"></a>
  <a href="https://gately.cn/"><img src="https://img.shields.io/badge/%E5%AE%98%E7%BD%91-gately.cn-0e7c66?style=flat-square&logo=googlechrome&logoColor=white" alt="gately.cn"></a>
  <a href="https://t.me/gately_chat"><img src="https://img.shields.io/badge/Telegram-gately__chat-26A5E4?style=flat-square&logo=telegram&logoColor=white" alt="Telegram"></a>
</p>

<p align="center">
  <a href="https://gately.cn/"><b>官网</b></a> &nbsp;·&nbsp; <a href="https://gately.cn/docs/guide/quick-start"><b>快速开始</b></a> &nbsp;·&nbsp; <a href="https://gately.cn/docs/guide/install"><b>安装指南</b></a> &nbsp;·&nbsp; <a href="https://gately.cn/docs/guide/introduction"><b>文档</b></a> &nbsp;·&nbsp; <a href="https://github.com/wangwanjie/gately/releases/latest"><b>下载</b></a> &nbsp;·&nbsp; <a href="https://gately.cn/news/"><b>更新日志</b></a>
</p>

<p align="center">
  <a href="https://gately.cn/sponsor"><img src="https://img.shields.io/badge/%E8%AF%B7%E6%88%91%E5%96%9D%E6%9D%AF%E5%92%96%E5%95%A1-Gately-FFDD00?style=for-the-badge&logo=buymeacoffee&logoColor=black" alt="请我喝杯咖啡" height="34"></a>
</p>

<p align="center">
  <a href="README.md">English</a> | <b>简体中文</b> | <a href="README.zh-TW.md">繁體中文</a> | <a href="README.ja.md">日本語</a> | <a href="README.ko.md">한국어</a> | <a href="README.es.md">Español</a> | <a href="README.fr.md">Français</a> | <a href="README.de.md">Deutsch</a> | <a href="README.ru.md">Русский</a> | <a href="README.pt-BR.md">Português (Brasil)</a>
</p>

从域名解析、内网穿透，到反向代理、HTTPS 证书；从 WAF、入侵防护，到统一登录与文件共享，家庭网络和服务器入口要用的能力，Gately 都整合在一个后台里。装在 NAS、软路由或云服务器上，打开浏览器就能配置和运维。源码以 Apache-2.0 开源，配置和数据只留在你自己的设备上，不收集、不上报。

> **飞牛 fnOS 用户**：Gately 提供飞牛原生应用。它接入**飞牛统一网关**，点击飞牛桌面图标即可打开，远程访问飞牛的域名同样可用，无需额外开放端口；开启后还支持**飞牛管理员登录态直接登录**，无需再输入 Gately 密码与两步验证码。详见[飞牛原生应用](https://gately.cn/docs/guide/install#fnos)。

## 快速开始

三步就能用：启动容器，打开 `http://设备IP:16680`，用 `admin` / `gately.cn` 登录。其他平台的装法见下方，完整说明见[安装指南](https://gately.cn/docs/guide/install)。

### Docker

镜像支持 amd64、arm64、arm/v7：

```bash
docker run -d --name gately --network host --cap-add NET_ADMIN \
  -v ./gately-data:/etc/gately \
  -v /var/run/docker.sock:/var/run/docker.sock \
  ghcr.io/wangwanjie/gately:latest
```

打开 `http://<设备IP>:16680`，用默认账号 `admin` 和默认密码 `gately.cn` 登录，然后在「设置」中修改密码并开启两步验证。完整的 Compose 示例见 [deploy/docker-compose.yml](deploy/docker-compose.yml)。

忘记密码或丢失两步验证器时，在服务器上运行 `gately reset-password` 或 `gately reset-totp`（Docker 为 `docker exec -it gately gately reset-password`），确认后立即生效，详见[重置密码与两步验证](https://gately.cn/docs/guide/install#reset)。

端口转发、STUN、Web 服务需要监听宿主机端口，推荐 `--network host`。不需要 Docker 管理时可去掉 `docker.sock` 挂载。`--cap-add NET_ADMIN` 供入侵防护把封禁写入宿主机的 nftables 防火墙；不加时封禁仍对 Gately 自身的服务生效。

想换种方式安装？从下面选一个。

<details>
<summary><b>二进制</b></summary>

从 [Releases](https://github.com/wangwanjie/gately/releases) 下载对应平台的压缩包（Linux amd64/arm64/arm/mips/mipsle/riscv64、macOS、Windows、FreeBSD），解压后运行：

```bash
./gately -c /etc/gately
```

参数：`-c` 配置目录（也可用环境变量 `GATELY_CONFIG_DIR`），`-listen` 覆盖管理界面监听地址，`-apply` 每次启动把目录中的 `<名称>.json` 覆盖到配置目录（环境变量 `GATELY_APPLY_DIR`，适合 GitOps 与 Kubernetes；账号与令牌文件不允许覆盖），`-version` 输出版本。无人值守安装时可设置环境变量 `GATELY_INITIAL_PASSWORD` 指定管理员初始密码，代替默认密码 `gately.cn`。

服务模板：[systemd](deploy/systemd/gately.service)、[OpenWrt procd](deploy/openwrt/gately.init)。校验下载的压缩包与镜像（cosign 签名、构建来源证明、SBOM）见[校验发布产物](docs/VERIFY.md)。

</details>

<details>
<summary><b>飞牛原生应用</b></summary>

每个 Release 附带飞牛 fnOS 的离线安装包 `gately_<版本>_fnos_x86.fpk`（Intel/AMD）与 `gately_<版本>_fnos_arm.fpk`（ARM）。它们没有上架官方应用中心，需要手动安装：打开 **应用中心 → 手动安装**（左下角按钮），选择文件并完成向导。

- **支持飞牛统一网关**：点击飞牛桌面上的 Gately 图标，管理界面经飞牛统一网关在 `/app/gately/` 下打开，与飞牛共用同一个访问入口。局域网地址、远程访问飞牛的域名都能直接使用，无需额外开放或映射 16680 端口。经网关访问时由飞牛负责访问控制（只有飞牛管理员能打开），安全入口与“仅允许内网访问”只约束直接访问 16680 端口。
- **支持飞牛管理员登录态直接登录**：在 **设置 → 访问控制 → 飞牛登录态自动登录** 开启后，已登录飞牛的管理员点击桌面图标即可进入 Gately，无需密码与两步验证码；普通飞牛用户仍需登录。主动退出后，登录页会显示“使用飞牛账号 &lt;用户名&gt; 登录”按钮，不会静默重新登录。该开关默认关闭，只在飞牛安装包中显示。
- 局域网内也可直接访问 `http://<NAS 地址>:16680`。默认账号 `admin`，默认密码 `gately.cn`。
- 应用以 `root` 身份运行，因此可以监听 80/443、通过 `docker.sock` 管理 Docker、使用真实的 Web 终端。请只安装在自己掌控的 NAS 上。
- 配置保存在应用的 `var` 目录，卸载时默认保留；在卸载向导里选择“全部删除”才会清除。升级时手动安装新版 `.fpk` 即可。
- 安装包同时提供命令行入口 `gately`（`/usr/local/bin/gately`），可使用 `gately ctl` 与 `gately mcp`。
- 16680 端口与 Docker 镜像相同，启动原生应用前请先停止 `gately` 容器。
- 下载后可按[校验发布产物](docs/VERIFY.md)校验，`.fpk` 的 SHA-256 同样列在 `checksums.txt`。

</details>

<details>
<summary><b>OpenWrt</b></summary>

每个 Release 附带 OpenWrt 安装包：`gately_<版本>_openwrt_<架构>.ipk` 用于 OpenWrt 24.10 及以前（含 iStoreOS），执行 `opkg install` 安装；`.apk` 用于 OpenWrt 25.12 起，执行 `apk add --allow-untrusted` 安装。架构有 amd64 / arm64 / arm / mipsle / mips / riscv64，按 `/etc/openwrt_release` 中的 `DISTRIB_ARCH` 选择，对照表见[安装文档](https://gately.cn/docs/guide/install#openwrt)。

- 安装后自动启用开机自启并启动，管理界面为 `http://<路由器地址>:16680`，默认账号 `admin`，默认密码 `gately.cn`。
- 配置在 `/etc/gately`，升级、卸载与刷写固件（sysupgrade）时都会保留。升级时安装新版安装包即可，「关于」页的在线升级会提示改用包管理器。
- 程序约 32 MB，适合 x86 软路由、ARM64 盒子等存储较大的设备；arm 安装包需要 ARMv7 且带硬件浮点。
- 另附 LuCI 插件 `luci-app-gately_<版本>_openwrt_all.ipk` / `.apk`（与架构无关），安装后在 LuCI「服务 → Gately」中查看运行状态、启停服务、设置开机自启与管理界面端口，并一键打开管理界面。

</details>

<details>
<summary><b>监控</b></summary>

创建一个只有 `metrics:read` 权限的 API 令牌，让 Prometheus 抓取 `/api/metrics`。指标列表、告警示例与 Grafana 面板见[监控指标](docs/MONITORING.md)。

</details>

<details>
<summary><b>API 调用</b></summary>

在管理后台“API 令牌”页创建令牌（可建多个，按模块分别授予只读或读写，可设过期时间，创建后明文只显示一次），脚本即可批量调用管理接口：

```bash
curl -H "Authorization: Bearer gtl_xxxx" http://localhost:16680/api/ddns/tasks
```

账号、两步验证、令牌管理以及备份、恢复、重启、系统设置接口只能使用登录会话。全部接口、所需权限与数据结构见管理后台“API 文档”页，也可以用 `make api-docs` 导出到 [docs/api](docs/api)（`openapi.json` 与 `API.md`），官网 https://gately.cn 发布时直接引用。

从旧版升级时，原来的全局 API 令牌会自动变成一条“遗留令牌”（值不变，拥有全部模块权限），建议在 API 令牌页收紧权限或换成新令牌。

</details>

<details>
<summary><b>Tailscale、frp、cloudflared</b></summary>

这三个模块托管官方的外部程序（自动重启、日志汇总到界面），Gately 本体不内置它们。各模块页面的「在线安装」卡片从上游下载最新正式版或指定版本，校验 SHA-256 后安装到配置目录的 `bin/`，并重启使用它的进程；可以为每个程序单独设置下载代理，留空时使用设置中的更新代理。替换下来的版本保留最近 3 个，新版本有问题时可一键回滚；不再需要时可以卸载程序。页头的「恢复默认配置」与卸载分开，会停止进程、清空模块设置并删除 Gately 为它生成的文件（frp 配置文件、File Browser 实例数据、Tailscale 登录状态），旧配置可在配置历史中恢复。Tailscale 官方只为 Linux 提供可直接运行的程序包，其他系统请用官方安装程序并填写路径。程序按以下顺序查找：模块设置中填写的路径、配置目录下的 `bin/`、`PATH`。也可以自行把二进制放进配置目录，Docker 部署时例如 `./gately-data/bin/frpc`；Tailscale 需要同时提供 `tailscale` 与 `tailscaled`。配置文件中的令牌与密钥以 0600 权限保存，cloudflared 的令牌通过环境变量传递，不会出现在进程列表中。

File Browser 模块托管 [FileBrowser Quantum](https://github.com/gtsteffaniak/filebrowser)（原版 filebrowser 已归档、不再修复安全问题），查找顺序与在线安装、更新、回滚同上，安装位置为 `bin/filebrowser`。管理员密码通过环境变量传递，不写入配置文件。

</details>

## 功能

先看几张管理界面的截图，完整功能清单在本节末尾。

### 一眼看清全局

概览页集中展示 CPU、内存、磁盘与流量，网卡及其公网 IPv4 / IPv6，以及每个模块的运行状态，哪里需要处理一目了然。

<img src="docs/images/overview.webp" alt="一眼看清全局" width="100%">

### 能替代 nginx 的 Web 服务

按域名做反向代理、静态文件和重定向，支持 HTTPS 与 HTTP/3。每个站点可以单独开 WAF、IP 规则和登录认证，改配置不断开现有连接。

<img src="docs/images/web.webp" alt="能替代 nginx 的 Web 服务" width="100%">

<p><b>站点统计</b> — 按分钟统计请求数、错误、后端健康状态，以及来源 IP 与国家排行。</p>

<img src="docs/images/site-stats.webp" alt="站点统计" width="100%">

### 一个 443 端口，给所有服务用

端口转发的 SNI 分流按域名把多个服务放在同一个端口上，TLS 原样透传，再用 PROXY 协议保留真实客户端地址。

<img src="docs/images/portforward-sni.webp" alt="一个 443 端口，给所有服务用" width="100%">

### 动态域名，主流服务商都支持

Cloudflare、阿里云、DNSPod、Route 53 等都能用。IPv4 与 IPv6，IP 可以从 URL、网卡或命令获取，地址一变就发通知。

<img src="docs/images/ddns.webp" alt="动态域名，主流服务商都支持" width="100%">

### 入侵防护

类 fail2ban 的规则读取日志文件、journald 和容器日志，诱饵端口与扫描检测抓出探测行为。封禁同步到 nftables，直接在内核里丢包。

<img src="docs/images/guard.webp" alt="入侵防护" width="100%">

### frp、Tailscale 与 Cloudflare 隧道

多个 frpc、frps 实例并行运行，和 Tailscale、cloudflared 隧道放在一起管理。这些程序的安装、更新和回滚都由 Gately 代劳。

<img src="docs/images/frp.webp" alt="frp、Tailscale 与 Cloudflare 隧道" width="100%">

### 还有这些

<table>
<tr>
<td width="50%" valign="top">

**Docker**<br>容器、Compose、日志与终端，镜像一键升级，失败自动回滚。

<img src="docs/images/docker.webp" alt="Docker" width="100%">

</td>
<td width="50%" valign="top">

**Web 终端**<br>在浏览器里用 SSH、Telnet 和本机 Shell，支持 SFTP 与分屏。

<img src="docs/images/terminal.webp" alt="Web 终端" width="100%">

</td>
</tr>
</table>

<table>
<tr>
<td width="50%" valign="top">

**配置历史**<br>每次修改都保留旧版本，可以对比差异、一键回滚。

<img src="docs/images/config-history.webp" alt="配置历史" width="100%">

</td>
<td width="50%" valign="top">

**API、命令行与 MCP**<br>按模块授权的 API 令牌、内置 OpenAPI 文档、`gately ctl`，以及接入 AI 助手的 MCP 服务。

<img src="docs/images/api-docs.webp" alt="API、命令行与 MCP" width="100%">

</td>
</tr>
</table>

<details>
<summary><b>完整功能清单</b></summary>

| 模块 | 能力 |
| --- | --- |
| 动态域名 | 阿里云、Cloudflare、腾讯云、DNSPod、华为云、GoDaddy、Porkbun、dynv6、DuckDNS、Amazon Route 53、OVHcloud、Gandi、NS1、Bunny DNS、Namecheap、DynDNS2（No-IP、Dynu 等）、自定义回调；URL/网卡/命令取 IP；IPv4/IPv6；Webhook；分组、排序与搜索 |
| STUN 穿透 | TCP/UDP、保活、UPnP/NAT-PMP、内置转发、地址变化时触发 Webhook 与脚本 |
| 端口转发 | TCP/UDP、IPv4/IPv6、端口段、多目标故障转移、连接数限制、流量统计、PROXY 协议（v1/v2）；SNI 分流，让多个服务共用 443 等同一个端口（按域名 TLS 透传、默认目标、每条路由单独设置 PROXY 协议）；分组、排序与搜索 |
| SSL 证书 | ACME 自动签发与续期，支持 DNS-01（通配符；委派子域名按 SOA 自动识别 DNS 区域）与 HTTP-01（无需 DNS 服务商）、短期证书与公网 IP 地址证书（Let's Encrypt shortlived 配置）、EAB、上传、读取文件、映射到目录并执行脚本；证书更新后经 SSH 推送到其他主机并执行命令，或重启使用证书的 Docker 容器 |
| Tailscale | 托管 tailscaled（用户态网络，容器友好）或接入系统已有守护进程；认证密钥/登录链接、Headscale、子网路由、出口节点、节点列表与 SOCKS5 代理、固定端口、tailscaled 额外参数与环境变量；Linux 上可在线安装、更新、回滚与卸载，可恢复默认配置 |
| frp | 可同时运行多个 frpc 与 frps 实例；frpc 客户端（tcp/udp/http/https/stcp/xtcp 代理，增删即生效）与 frps 服务端（端口白名单、子域名、控制台），可一键导入旧版 ini / 新版 toml 配置；在线安装、更新、回滚与卸载，可恢复默认配置 |
| Cloudflare 隧道 | cloudflared 令牌隧道与 trycloudflare 临时隧道，多隧道、自动重启、公网地址展示；在线安装、更新、回滚与卸载，可恢复默认配置；令牌隧道可通过 Cloudflare API 编辑路由规则并创建 DNS 记录 |
| Web 服务 | 按域名反向代理/重定向/静态文件/固定文本/断开连接（相当于 nginx return 444）；HTTPS 与 HTTP/3（QUIC）；HTTP 与 HTTPS 共用同一端口（跳转或直接服务）；WebSocket；PROXY 协议收发；基本认证、网页认证、GitHub/Google/QQ/OIDC 登录、带 TOTP 二次验证并可跨子域名单点登录的共享用户、内置 OIDC 身份提供方（让 Gitea、Grafana 等应用用共享用户登录）、转发认证到 Authelia 等外部服务、公网 Gately 可把登录 Cookie 转发给内网 Gately；IP 黑白名单与定时刷新的订阅 IP 列表（如省市 IP 段）、路径规则、临时分享链接、认证失败自动封禁、GeoIP 国家过滤；按路径分流后端、按登录用户分流到不同后端（类似 nginx map $remote_user）、HTTPS 后端按访问域名发送 SNI、后端权重/备用/负载均衡策略、主动与被动健康检查、压缩、限流、响应缓存、路径重写；静态站点支持无扩展名地址与自定义 404 页；修改站点不会断开已有连接；站点分组、排序与搜索；分钟级访问统计与 Prometheus 指标；共享用户可自助修改密码；访问被拒绝时站点可直接断开连接，不返回 403 |
| WAF 防火墙 | Coraza + OWASP CRS v4，可按文件启用规则、自定义 SecLang、拦截/仅检测；请求体检查上限可调，超限时只检查前段或按路径跳过，网盘、WebDAV 等上传站点也能挂 WAF |
| 入侵防护 | 类 fail2ban 的日志规则（日志文件、systemd 日志、命令输出、Docker 容器日志、管理后台登录失败、WAF 拦截），内置 SSH、Nginx、Vaultwarden 等模板；诱饵端口与端口扫描检测；统一封禁表，支持手动网段与永久封禁、递增时长与白名单；封禁同步到系统防火墙（nftables）在内核层丢弃，没有 nftables 的平台对 Gately 自身的服务生效；Web 路径探测封禁：按监听统计诱饵路径（/.env、/.git 等）、未知域名扫描与连续 404 |
| Docker 管理 | 容器创建与编辑、日志与终端、Compose、镜像更新检查与一键升级（失败自动回滚）、网络与卷 |
| Web 终端 | SSH（主机密钥首次信任）、Telnet、本机 Shell（默认关闭）；代理、持久会话、SFTP 文件管理、2FA 与协议确认、快捷命令、分屏 |
| 网络诊断 | Ping、路由追踪、端口探测、DNS 查询、测速、局域网扫描 |
| 通知中心 | Webhook、Telegram、Bark、ntfy、Gotify、Pushover、Discord、Slack、Server酱、PushPlus、企业微信、钉钉、飞书、邮件；级别、通配与静默去重规则；内置规则模板（关键故障、证书到期、地址变化、登录、配置变更） |
| 配置备份 | 加密备份（scrypt + AES-GCM）、保留策略、定时备份到本地目录或存储挂载点 |
| 命令行与 MCP | `gately ctl` 调用运行中的实例，`gately mcp` 以 stdio 提供 MCP 服务（默认只读） |
| 网络唤醒 | 魔术包广播、在线探测 |
| WebDAV | 多用户、独立目录、只读；WebDAV、FTP、SMB 共用一份共享用户 |
| FTP | 多用户、独立目录、只读，支持 FTPS 与被动端口范围；WebDAV、FTP、SMB 共用一份共享用户 |
| SMB | 多用户、按用户共享多个目录、只读，支持 SMB 2/3、加密与 Bonjour 发现；WebDAV、FTP、SMB 共用一份共享用户 |
| File Browser | 托管 FileBrowser Quantum 网页文件管理器：多实例、多个共享目录与只读、账号密码或反向代理认证；在线安装、更新、回滚与卸载，可恢复默认配置 |
| 存储挂载 | 本地目录、远程 WebDAV 与 SMB 共享挂载，在线文件管理 |
| 计划任务 | cron 表达式，命令或 HTTP 请求，执行记录 |
| 安全与设置 | Passkey（WebAuthn）登录、两步验证（TOTP）、只读账号、登录失败限流、安全入口、仅内网访问、管理界面 HTTPS、多 API 令牌（按模块读写授权）、已保存的密码与密钥可按需显示（需写权限）、自定义 DNS（支持 DoH）、时区、配置备份与恢复、信任本机反向代理传来的真实客户端地址、按需开关功能模块（关闭的模块不启动服务、不提供接口） |
| 飞牛原生应用 | 离线 `.fpk` 安装包（x86 / ARM）；接入飞牛统一网关，从飞牛桌面图标打开，远程域名同样可用；飞牛管理员可凭飞牛登录态直接登录（默认关闭）；以 root 运行，可监听 80/443 并管理 Docker |
| 审计与历史 | 操作审计日志：登录、配置变更、访问敏感内容（不记录请求体与任何密钥）；每次覆盖配置文件前保留旧版本，可对比差异并一键回滚；声明式配置目录（`-apply`），便于 GitOps |
| 监控与自检 | 全局 Prometheus 指标 `/api/metrics`（证书、DDNS、STUN、端口转发、Web 服务），附 Grafana 面板；一键网络自检（公网 IP、运营商 NAT、系统时间、80/443 端口、DDNS 解析、证书到期、HTTPS 站点缺少证书、frp 代理冲突） |
| 迁移 | 导入 nginx 配置（支持 `nginx -T` 输出）：反向代理与 upstream、静态目录、跳转、分路径后端、访问控制，`auth_request` 转换为转发认证 |

</details>

## 为什么做 Gately

Gately 完全开源，用户和社区可以直接审阅源码与配置。对于运行在网络边界、手握 DNS 密钥和证书私钥的软件，开源让用户能够看到它在做什么。Gately 的承诺：

- 源码以 Apache-2.0 开源，接受社区审查
- 默认不连接任何项目方服务器，不上报设备信息；第三方登录使用你自己的 OAuth 应用，不经过中转
- 所有对外地址（公网 IP 查询、STUN 服务器等）可见、可改、可关
- 配置为明文 JSON，可直接纳入 git 做审计和回滚

## 性能

端口转发的 SNI 分流加 Web 服务，可以替代常见的 haproxy（按 SNI 做 TCP 分流）+ nginx（TLS 与反向代理）组合。下表在同一台机器上、按一台真实服务器的配置对比两者：90 个域名按 SNI 分流，两跳之间使用 PROXY 协议 v2，ECDSA P-256 证书，TLS 1.3。

| 场景 | haproxy + nginx | Gately |
| --- | --- | --- |
| HTTPS 反向代理，长连接，1 KB 响应，64 并发 | 14,767 req/s（p99 35.9 ms） | **26,434 req/s**（p99 5.7 ms） |
| 同上，HTTP/2 | 未测试（nginx 配置使用 HTTP/1.1） | 27,795 req/s |
| 每个请求新建 TLS 连接，32 并发 | 2,557 req/s | **3,448 req/s** |
| 1 MB 响应，16 并发 | 1,053 MB/s | **1,585 MB/s** |
| SNI 透传 TLS 数据流，4 连接 | 2,927 MB/s | **4,962 MB/s** |
| SNI 透传，每秒新建连接数，64 并发 | 6,334 /s | 6,266 /s（持平） |
| 负载下的内存 | haproxy ≈ 21 MB + nginx ≈ 21 MB | ≈ 110 MB（包含全部模块的单个进程） |

<details>
<summary><b>测试方法</b></summary>

- 机器：Intel Core i5-13500T。为贴近小型云服务器，每套方案限制在 2 个 CPU 核心（haproxy 与 nginx 在 Docker 中用 `--cpuset-cpus`，Gately 用 `taskset` 绑核），后端与压测程序运行在其他核心。
- haproxy 使用 `mode tcp`、`req_ssl_sni` 规则与 `send-proxy-v2`。nginx 使用 `worker_processes 2` 与生产站点的代理设置，其中 `Connection: upgrade` 会让每个请求新建一条后端连接；开启上游 keep-alive 的 nginx 在小响应上会缩小差距。
- Gately 的路径是 SNI 规则、PROXY v2、再到 Web 服务监听，跳数与 haproxy → nginx 相同。
- 每个场景运行 10 秒，重复测试的场景差异在 3% 以内。在真实的 2 vCPU 云主机上，静态站点为 5.5k req/s 对 3.4k req/s；客户端使用 X25519 时新建 TLS 连接持平（双方都约每秒 480–550 个），使用 Go 客户端与新版浏览器默认提供的后量子混合密钥交换时，Gately 在该次测试中完成的握手少约 13%。
- 结果受硬件、配置与流量影响，仅用于确认 Gately 能跟上 haproxy 与 nginx，请用自己的业务负载实测。

</details>

## 开发

依赖：Go 1.26+、Node.js 24+、pnpm。

```bash
make web      # 构建前端（输出到 web/dist，由 go:embed 嵌入）
make build    # 编译 bin/gately
make test     # go test -race ./...
make lint     # gofmt、go vet、前端类型检查
```

前端开发时运行 `cd web && pnpm dev`，`/api` 会代理到本地 `127.0.0.1:16680`。

一些测试默认跳过，需要额外条件：

| 测试 | 启用方式 |
| --- | --- |
| ACME 端到端（Pebble） | 安装 pebble 与 pebble-challtestsrv，设置 `GATELY_PEBBLE_BIN`、`GATELY_PEBBLE_SRC`（CI 中自动运行） |
| 真实公共 STUN 服务器 | `GATELY_NET_TEST=1` |

目录结构与技术选型见 [docs/ROADMAP.md](docs/ROADMAP.md)。

官网（[gately.cn](https://gately.cn)）源码在 `website/`，基于 VitePress：`cd website && pnpm install && pnpm dev`；`pnpm deploy:site` 构建并同步到服务器。

## 贡献约定

- 兼容性代码只能基于公开可观察的行为、接口和配置格式，请勿提交从第三方二进制或前端产物中复制的代码。
- 新模块需要：配置结构与校验、运行时启停、REST 接口、单元测试；希望在总览页展示状态的模块实现 `module.Summarizer`。
- 代码注释使用中文；控制语句必须使用大括号。

## 交流群

欢迎加入 Telegram 群交流使用问题、反馈建议：[t.me/gately_chat](https://t.me/gately_chat)，也可以用手机扫描二维码加入：

<a href="https://t.me/gately_chat"><img src="docs/images/telegram-qr.svg" alt="扫码加入 Gately Telegram 群" width="160"></a>

Bug 请提交到 [GitHub Issues](https://github.com/wangwanjie/gately/issues)，并附上[诊断包](docs/DIAGNOSTICS.md)（设置 → 诊断）；诊断包在本机生成，不含密码、令牌、域名和真实 IP。

## 赞助

Gately 的全部功能免费开放。如果它帮到了你，欢迎通过 [爱发电](https://ifdian.net/a/gately)、微信或 USDT 赞助，收款码与地址见[官网赞助页](https://gately.cn/sponsor)。

## Star 趋势

[![Star History Chart](https://api.star-history.com/svg?repos=wangwanjie/gately&type=Date)](https://star-history.com/#wangwanjie/gately&Date)

## 许可证

[Apache License 2.0](LICENSE)
