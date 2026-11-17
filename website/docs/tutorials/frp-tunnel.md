---
time: 30 分钟
description: 用 Gately 托管的 frps 与 frpc 把家里的服务映射到 VPS：在线安装 frp、配置 frps 的端口、令牌、KCP/QUIC、来源 IP 白名单与自动放行防火墙，在家里添加 TCP、HTTP/HTTPS 子域名与 STCP 代理，再查看状态、日志并保留访客真实 IP。
---

# 用 frp 把家里的服务映射到 VPS

家里没有公网 IP，或者公网 IP 与端口不稳定，但你有一台有公网 IPv4 的 VPS。frp 让家里的 frpc 主动连到 VPS 上的 frps，把 NAS、SSH、网站等服务映射到 VPS 的端口或域名上。Gately 在两端都能托管 frp：在线安装程序、在界面里填写配置、自动重启、日志汇总。做完本文，你会在 VPS 上有一个只接受自家 frpc 登录的 frps，在家里有一个 frpc 实例和几条代理，并知道怎样让家里的网站看到访客真实 IP。

几条无公网 IP 路线的对比见[没有公网 IPv4 怎么从外面访问 NAS](/docs/tutorials/remote-access-without-public-ip)，本文把其中的 frp 路线讲完整。想先看界面，可以打开[在线演示](https://demo.gately.cn)（「演示账号一键进入」，只读）里的「组网与穿透 → frp」。

## 适用场景与前提

- 一台有公网 IPv4 的 VPS，上面装好 Gately（[安装](/docs/guide/install)）；家里也装好 Gately；
- 一个域名，解析可以指向 VPS（用 HTTP / HTTPS 子域名代理或在 VPS 上做 HTTPS 时需要）；
- 两端的 Gately 都直接使用宿主机网络：Docker 部署使用 `--network host`，否则 frps 的端口还要逐个映射；
- 侧边栏没有「frp」时，点侧边栏底部的「添加更多功能」打开这个模块。

## 结果是什么样

```text
访客 ─▶ VPS（vps.example.com）
          ├─ :15000 ……………… frps tcp 代理 ─┐
          ├─ :8080 / :8443 …… frps 子域名代理 ─┤   隧道（frpc 主动连 :7000）
          └─ :7000 frps 绑定端口 ◀──────────────┘
                                              │
家里 Gately（frpc）───────────────────────────┘
          ├─ nas-web   → 192.168.1.10:5000
          ├─ nas-https → 192.168.1.10:5001（nas.frp.example.com）
          └─ nas-ssh   → 192.168.1.10:22（stcp，只给持有密钥的访问端）
```

## 步骤

### 1. 两端安装 frp 程序

Gately 不内置 frp 程序，第一次使用时在页面里在线安装：

1. 打开「组网与穿透 → frp」，滚动到页面下方的「frp 在线安装」卡片；
2. 点「下载安装」。Gately 从 GitHub 下载最新正式版，校验 SHA-256 后安装到配置目录的 `bin/`，同时装好 `frpc` 与 `frps`；
3. VPS 或家里访问 GitHub 不稳定时，先在「下载代理」填写 `http://`、`https://` 或 `socks5://` 代理再点安装；留空时使用「设置」里的全局更新代理；
4. 安装完成后卡片显示版本号。以后用「检查更新」「更新到最新版」升级，新版本有问题时在「旧版本」列表里点「回滚」，「安装指定版本」可以降级到某个版本（例如 `v0.65.0`）。

![frp 在线安装卡片](/tutorials/frp-tunnel-install.zh-CN.webp)

两端的 frp 版本最好一致或相近。也可以自己把 `frpc`、`frps` 放进配置目录的 `bin/`，或在页面中部的「程序」卡片填写路径。

### 2. VPS 上：添加 frps 实例

1. 在 VPS 的 Gately 打开「frp」，切到「服务端 frps」，点「添加 frps 实例」。
2. 填写基本项：
   - 「名称」：`公网 frps`；
   - 「监听地址」：留空，frpc 连接端口监听在所有地址；
   - 「绑定端口」：`7000`，frpc 连这个端口；
   - 「令牌」：一串足够长的随机字符，frpc 必须使用相同令牌。不设令牌时任何人都能用你的 frps；
   - 「代理监听地址」：两种选择，后面第 6 步会用到：
     - 留空：tcp 代理的远程端口直接对公网开放，访客直接连 `vps.example.com:15000`；
     - `127.0.0.1`：远程端口只在 VPS 本机可见，再由 VPS 上的 Gately Web 服务或端口转发对外，可以统一加 HTTPS、登录与 WAF；
   - 「允许的远程端口」：`15000-15100`，frpc 只能申请这个范围内的端口，避免占用 VPS 的 22、443 等端口。
3. 需要 HTTP / HTTPS 子域名代理时（第 4 步的方式 B）：
   - 「HTTP 端口」`8080`、「HTTPS 端口」`8443`（`0` 表示不启用）。VPS 的 80 / 443 已经被 Gately Web 服务占用时，用其他端口，再由 Web 服务按域名转过去；
   - 「子域名根域名」：`frp.example.com`，并把 `*.frp.example.com` 解析到 VPS。
4. 网络丢包较多时，可以另开 UDP 传输：「QUIC 端口（UDP）」填 `7001`，或「KCP 端口（UDP）」填 `7000`（KCP 端口可以与绑定端口相同，KCP 与 QUIC 端口不能相同）。三个「QUIC …」调节项保持 `0` 使用 frp 默认值。
5. 打开「自动放行防火墙」：Gately 在 VPS 的 nftables / iptables 中放行绑定端口（TCP）、KCP / QUIC 的 UDP 端口与 HTTP / HTTPS 端口，实例停用或删除时撤销。frpc 申请的远程端口（`15000-15100`）是动态的，不在自动放行范围内，需要对公网开放时自己放行。
6. 「IP 访问控制」选「白名单」，在「IP / 网段」填家里的出口地址或网段，例如 `198.51.100.0/24`；家里是动态 IP 时可以引用「订阅列表」（例如本省 IP 段），或保持「关闭」只靠令牌。白名单只决定 frpc 能否登录，在 frps 校验令牌之前判断；被入侵防护封禁的地址始终无法登录。
7. 「控制台端口」保持 `0`（不开 frps 自带的网页控制台），点「保存」。

![添加 frps 实例：端口、令牌、代理监听地址与自动放行防火墙](/tutorials/frp-tunnel-frps-dialog.zh-CN.webp)

保存后实例卡片显示「运行中」，以及 QUIC / KCP 端口、子域名根域名、允许的远程端口、IP 访问控制与「已拒绝登录」次数。

![frps 实例列表](/tutorials/frp-tunnel-frps-list.zh-CN.webp)

::: warning 云服务器安全组
「自动放行防火墙」只管 VPS 系统里的防火墙。云厂商控制台的安全组要另外放行：TCP 7000，用到时再加 UDP 7001（QUIC）/ UDP 7000（KCP）、HTTP / HTTPS 端口与要对公网开放的远程端口。
:::

一台 VPS 可以添加多个 frps 实例，使用不同的绑定端口，例如给朋友单独开一个带独立令牌与端口范围的实例。

### 3. 家里：添加 frpc 实例

1. 在家里的 Gately 打开「frp」，保持在「客户端 frpc」，点「添加 frpc 实例」。
2. 填写：
   - 「名称」：`VPS`；
   - 「服务端地址」：`vps.example.com`（或 VPS 的 IP）；
   - 「服务端端口」：`7000`；
   - 「令牌」：与 frps 相同；
   - 「用户名」：`home`。可选，frps 会把它作为代理名称前缀，多台设备连同一个 frps 时用不同的用户名避免代理重名；
   - 「传输协议」：默认 `tcp`。选 `quic` 时把「服务端端口」改为 frps 的 QUIC 端口 `7001`，选 `kcp` 时改为 KCP 端口；frps 放在 Web 服务之后时选 `websocket` / `wss`。
3. 用到 HTTP / HTTPS 子域名代理时，在「访问地址」里按 frps 的设置填写「http 代理」端口 `8080`、「https 代理」端口 `8443`、「子域名根域名」`frp.example.com`。这几项只用来在代理列表里生成可点击的链接，修改后 frpc 不重启。本机同时有 frps 实例时可以点「从本机 frps 实例填入」。
4. 点「保存」。

![添加 frpc 实例](/tutorials/frp-tunnel-frpc-dialog.zh-CN.webp)

实例卡片显示「运行中」说明进程已启动；连接 frps 的成败看页面底部的运行日志（下文「验证」）。一个家里的 Gately 可以添加多个 frpc 实例，分别连不同的 VPS，例如香港和美国各一台。

### 4. 家里：添加代理

在「代理」卡片点「添加代理」。「所属实例」选刚才的 `VPS`；代理增删改后只重启所属的 frpc 实例。

**方式 A：tcp 代理（任意 TCP 服务）**

1. 「名称」`nas-web`，「类型」`tcp`；
2. 「本地地址」`192.168.1.10`，「本地端口」`5000`（家里服务的地址与端口，本地地址可以是任意内网设备）；
3. 「远程端口」`15000`，必须在 frps「允许的远程端口」范围内；
4. 保存。访客访问 `vps.example.com:15000`（frps「代理监听地址」留空时），或由 VPS 上的 Gately 反向代理到 `127.0.0.1:15000`（填 `127.0.0.1` 时，见第 6 步）。

![添加 tcp 代理](/tutorials/frp-tunnel-proxy-tcp.zh-CN.webp)

UDP 服务（游戏、DNS 等）把类型选 `udp`，其余相同，并在安全组放行 UDP 远程端口。

**方式 B：http / https 代理（按域名共用 frps 的一个端口）**

1. 「名称」`nas-https`，「类型」`https`；
2. 「本地地址」`192.168.1.10`，「本地端口」`5001`；
3. 「子域名」填 `nas`，得到 `nas.frp.example.com`；或在「自定义域名」填完整域名（多个用逗号分隔），并把这些域名解析到 VPS；
4. 保存。代理卡片的「域名」一栏可以点击打开、复制完整地址。

![添加 https 子域名代理](/tutorials/frp-tunnel-proxy-https.zh-CN.webp)

`https` 类型由 frps 按 SNI 原样转发 TLS，证书在家里的服务上；`http` 类型按 Host 头转发明文 HTTP。想在 VPS 上统一签证书、加登录与 WAF，更推荐方式 A 加第 6 步。

**方式 C：stcp 私密代理（只给持有密钥的访问端）**

stcp 不在 frps 上开放任何端口，只有运行 frpc 访问端（visitor）、并且知道密钥的设备才能连，适合 SSH、远程桌面这类只给自己用的服务。

1. 「名称」`nas-ssh`，「类型」选「stcp（私密 TCP）」；
2. 「本地地址」`192.168.1.10`，「本地端口」`22`；
3. 「密钥」填一串随机字符，保存。

访问端在 Gately 中没有配置界面，在要连接的电脑上安装 frp，用下面的 `frpc.toml` 运行官方 frpc（frp 0.52 及以上的写法）：

```toml
serverAddr = "vps.example.com"
serverPort = 7000
auth.token = "与 frps 相同的令牌"

[[visitors]]
name = "nas-ssh-visitor"
type = "stcp"
# 家里 frpc 的用户名；家里没填用户名时删掉这一行
serverUser = "home"
serverName = "nas-ssh"
secretKey = "与代理相同的密钥"
bindAddr = "127.0.0.1"
bindPort = 6022
```

```bash
frpc -c frpc.toml
ssh -p 6022 user@127.0.0.1
```

`xtcp`（点对点 TCP）的配置方式与 stcp 相同，访问端会先尝试打洞直连，失败时连接不上，需要访问端自己准备回退方案。

### 5. 查看状态与日志

- **实例与代理卡片**：frpc / frps 实例显示「运行中」「已停用」「启动失败」「未运行」，鼠标悬停「启动失败」可以看到原因；代理卡片显示所属实例、类型、本地地址、远程端口或域名；
- **运行日志**：页面底部的运行日志汇总了 frpc / frps 的输出，用「来源」筛选某个实例，用关键词筛选 `login`、`start proxy` 等；
- **网络自检**：「概览 → 网络自检」会检查 frp 代理冲突，例如两个启用的代理重名，或 frps 报告「端口已被占用」「同名代理已存在」；
- **重启**：卡片上的「重启」只重启这一个实例；
- **导入已有配置**：页头「导入配置」可以粘贴原来的 `frpc.ini` / `frpc.toml` / `frps.toml`，先预览再导入。

![frpc 实例与代理列表](/tutorials/frp-tunnel-frpc-list.zh-CN.webp)

### 6. 让家里的网站看到访客真实 IP

frp 的 tcp 代理只搬运字节，家里服务看到的来源是 frpc 的地址。按你想在哪一端终止 HTTPS，有两种做法：

**做法一：在 VPS 上终止 HTTPS（推荐）**

1. VPS 的 frps「代理监听地址」填 `127.0.0.1`，家里的代理是 `nas-web`（远程端口 `15000`）；
2. 在 VPS 的「Web → SSL 证书」签发覆盖 `nas.example.com` 的证书；
3. 在 VPS 的「Web 服务」443 监听上添加站点：域名 `nas.example.com`，类型「反向代理」，后端地址 `http://127.0.0.1:15000`；
4. 站点的「添加 X-Forwarded-*」默认开启，家里的应用从 `X-Forwarded-For` / `X-Real-IP` 读取访客 IP（应用需要信任这个头）。

完整的 VPS 入口做法（证书、WAF、入侵防护、登录）见[用 VPS 做家里服务的公网入口](/docs/tutorials/vps-front-door)。

**做法二：在家里终止 HTTPS，用 PROXY 协议传递真实 IP**

适合家里的 Gately 已经有完整的站点、证书与认证，VPS 只做 TCP 中转：

1. 家里：在「Web 服务 → 全局设置」的「可信 PROXY 来源」填 `127.0.0.0/8`；添加一个监听，监听地址 `127.0.0.1`，端口 `8443`，开启 HTTPS 与「接收 PROXY 协议」，把站点挂在这个监听上；
2. 家里：添加 tcp 代理，「本地地址」`127.0.0.1`，「本地端口」`8443`，「远程端口」`15443`；
3. VPS：frps「代理监听地址」填 `127.0.0.1`；在「网络 → 端口转发」添加规则，监听端口 `443`，目标 `127.0.0.1:15443`，「高级设置」里「PROXY 协议」选「PROXY v2」。需要在 443 上同时分流 SSH 等协议时改用[协议分流](/docs/tutorials/one-port-443)，默认目标的「默认目标 PROXY」选「PROXY v2」；
4. PROXY 头由 VPS 写入，经 frp 隧道原样到达家里的监听，家里的访问日志、IP 名单与入侵防护都按访客真实 IP 工作。

::: danger 远程端口不能对公网开放
做法二中，任何能直接连到 `15443` 的人都可以伪造 PROXY 头冒充任意来源 IP。frps 的「代理监听地址」必须是 `127.0.0.1`，云厂商安全组也不要放行这个端口。
:::

## 验证

```bash
# VPS 上 frps 的绑定端口可达
nc -vz vps.example.com 7000

# tcp 代理（代理监听地址留空时）
curl -I http://vps.example.com:15000/

# 子域名代理
curl -I https://nas.frp.example.com:8443/

# 做法一：VPS 上终止 HTTPS
curl -I https://nas.example.com/
```

- frpc 运行日志中出现 `login to server success` 与 `start proxy success`，说明登录与代理都成功；
- frps 运行日志中出现对应代理名（带用户名前缀，如 `home.nas-web`）的 `new proxy` 记录；
- frps 开启了白名单时，从名单外的网络连接会在 frpc 日志里看到 `gately: client address ... is not in the allowlist`，frps 卡片上的「已拒绝登录」次数增长。

## 常见问题

**frpc 日志反复出现 `connect to server error` 或超时。** VPS 的 7000 端口没放行（安全组、系统防火墙），或「服务端地址」写错。在家里执行 `nc -vz vps.example.com 7000` 确认端口可达。

**`token in login doesn't match token from configuration`。** 两端令牌不一致。在 frps 与 frpc 的编辑框里点眼睛图标显示令牌核对。

**`port not allowed` 或 `port already used`。** 远程端口不在 frps「允许的远程端口」范围内，或已被别的代理、别的程序占用。换一个端口。

**`proxy [xxx] already exists`。** 同一个 frps 上已有同名代理，常见于多台设备复制了同一份配置。给每台设备的 frpc 设置不同的「用户名」。

**改成 quic / kcp 后连不上。** frpc 的「服务端端口」要改成 frps 的 QUIC / KCP 端口，并在安全组放行对应的 UDP 端口；部分网络会限制 UDP，退回 tcp 即可。

**子域名代理返回 frp 的 404 页面。** 访问的域名与代理的子域名或自定义域名不一致，或 frps 没有设置「子域名根域名」，或 DNS 没有指向 VPS。

**站点返回 502。** VPS 上站点后端的端口与 frpc 的「远程端口」不一致，或家里的服务没有运行。先在 VPS 上执行 `curl -I http://127.0.0.1:15000/` 确认隧道是通的。

## 下一步

- [用 VPS 做家里服务的公网入口](/docs/tutorials/vps-front-door)：VPS 上的证书、WAF 与入侵防护；
- [一个 443 端口跑所有服务](/docs/tutorials/one-port-443)：协议分流与 PROXY 协议；
- [STUN 穿透](/docs/tutorials/stun-traversal)、[异地组网](/docs/tutorials/mesh-vpn)：不用 VPS 的路线；
- 模块文档：[frp](/docs/modules/tunnels#frp)、[在线安装、更新与回滚](/docs/modules/tunnels#install)。
