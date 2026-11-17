---
time: 30 分钟
description: 用 Gately 托管 Tailscale、NetBird、EasyTier 把家里、公司和手机组成私有网络：在线安装、登录与授权、子网路由与出口节点、节点状态、自建控制服务器与中继，以及用 Cloudflare 隧道发布网页服务。
---

# 异地组网：Tailscale、NetBird 与 EasyTier

只想让自己和家人的设备访问家里的服务，不打算对公网开放端口时，组网是最省心的路线：每台设备装一个客户端，加入同一个私有网络后，彼此用组网地址直接访问，优先点对点直连，打不通时经中继。Gately 可以托管 Tailscale、NetBird、EasyTier 三种组网程序：在线安装、在界面里登录与配置、进程自动重启、节点列表一目了然。做完本文，家里的 Gately 会加入组网并把整个家庭网段共享出去，手机在外面也能直接打开 `192.168.1.10` 上的 NAS。

几条无公网 IP 路线的对比见[没有公网 IPv4 怎么从外面访问 NAS](/docs/tutorials/remote-access-without-public-ip)。想先看界面，可以打开[在线演示](https://demo.gately.cn)（「演示账号一键进入」，只读）里的「组网与穿透 → 组网」。

## 三种组网怎么选

| | Tailscale | NetBird | EasyTier |
| --- | --- | --- | --- |
| 控制端 | 官方云（免费额度），或自建 Headscale | 官方云，或自建管理服务（开源） | 无中心控制端，组网名称 + 密钥 |
| 直连失败时 | 官方 DERP 中继（海外）或自建 DERP | 官方或自建中继 | 公共节点或自建节点中继，可选国内节点 |
| 客户端 | 全平台，手机 App 成熟 | 全平台 | 全平台，手机 App 可用 |
| 适合 | 想最快用起来 | 想客户端与管理端都开源、可自建 | 国内网络、UDP 打洞困难、想自选中继 |

三种可以同时运行，各用各的网段，互不影响。下面以 Tailscale 为主线写完整步骤，NetBird 与 EasyTier 只写不同之处。

## 适用场景与前提

- 访问家里服务的都是你能装客户端的设备（电脑、手机、平板）；
- Gately 运行在 Linux 上（路由器、NAS、小主机、VPS）。Tailscale 的在线安装只提供 Linux 程序包，macOS、Windows 请用官方安装程序；
- 要共享整个家庭网段（子网路由）时，Gately 所在设备要能访问那些内网设备；
- Docker 部署推荐 `--network host`。使用内核网卡模式时还需要 `cap_add: [NET_ADMIN]` 与 `/dev/net/tun`；做不到时开启「用户态网络」，不需要任何特权；
- 侧边栏没有「组网」时，点侧边栏底部的「添加更多功能」打开 Tailscale、NetBird 或 EasyTier 模块。

## 结果是什么样

```text
手机（Tailscale App，在 4G 网络上）
   │  组网（WireGuard 加密，直连优先）
   ▼
家里 Gately：tailnet 地址 100.x.y.z
   ├─ 发布子网路由 192.168.1.0/24 ─▶ NAS 192.168.1.10、路由器、摄像头……
   └─ 出口节点：手机在外可以让全部流量经家里出网
```

## 步骤：Tailscale

### 1. 安装程序

1. 打开「组网与穿透 → 组网」，切到「Tailscale」标签；
2. 页面下方「Tailscale 在线安装」卡片点「下载安装」，Gately 从 pkgs.tailscale.com 下载静态包，同时安装 `tailscale` 与 `tailscaled`；
3. 访问外网不稳定时先填「下载代理」。以后在同一张卡片上「检查更新」「更新到最新版」或「回滚」，见[在线安装、更新与回滚](/docs/modules/tunnels#install)。

系统里已经装了 Tailscale（例如 OpenWrt 软件包、群晖套件）时可以不装，后面添加实例时关闭「托管 tailscaled」，Gately 只负责执行 `tailscale up` 和显示状态。

### 2. 准备认证密钥（推荐）

在 Tailscale 管理后台的 Settings → Keys 生成一个 Auth key（`tskey-auth-` 开头）。无人值守的设备用密钥加入最省事；不填密钥时，Gately 会在卡片上显示登录链接，你在任意设备的浏览器里打开登录即可。

使用自建 Headscale 时，在 Headscale 上用 `headscale preauthkeys create` 生成密钥，后面在「控制服务器」填写 Headscale 的地址。

### 3. 添加 Tailscale 实例

1. 在「实例」卡片点「添加实例」，填写：
   - 「名称」：`家里`；
   - 「认证密钥」：粘贴上一步的密钥，或留空用登录链接；
   - 「设备名称」：`gately-home`，留空使用系统主机名；
   - 「控制服务器」：留空使用 Tailscale 官方；自建 Headscale 填 `https://hs.example.com`；
   - 「子网路由」：`192.168.1.0/24`，把家庭网段共享给组网内的其他设备，多个网段用逗号分隔；
   - 「出口节点」：打开，允许其他设备经这台机器访问互联网；
   - 「接受路由」：家里这台通常保持关闭；需要访问其他节点发布的网段时打开；
   - 「接受 DNS」：使用 tailnet 的 MagicDNS 配置，按需打开。
2. 「运行方式」：
   - 「托管 tailscaled」保持打开，由 Gately 启动和守护 tailscaled；
   - 「用户态网络」：容器内或没有 TUN 设备时打开。用户态网络下本机程序访问组网要经「SOCKS5 代理」（例如 `localhost:1055`，建议只监听 `127.0.0.1`）；其他设备访问本机与子网路由不受影响；
   - 「WireGuard 端口」：`0` 自动选择；想提高直连成功率时固定一个端口（例如 `41641`），并在路由器把这个 UDP 端口映射到本机；
   - 「状态目录」：留空即可。原来用 tailscale 容器的，填原容器挂载的状态目录，可以沿用原节点身份，不用重新注册；
   - 「环境变量」：宿主机 iptables 是 nf_tables 后端时（较新的 Debian、Ubuntu、OpenWrt 等），在内核模式下填 `TS_DEBUG_FIREWALL_MODE=nftables`，否则子网转发不生效。
3. 点「保存」。

![添加 Tailscale 实例：子网路由与出口节点](/tutorials/mesh-vpn-tailscale-dialog.zh-CN.webp)

::: tip 一台机器加入多个 tailnet
可以添加多个实例，各自运行独立的 tailscaled，例如一个连官方、一个连自建 Headscale。同时启用的内核模式实例最多一个，其余实例开启「用户态网络」。
:::

### 4. 登录与在控制台批准

1. 没有填认证密钥时，实例卡片会出现「打开此链接完成设备认证」，点开后用你的 Tailscale 账号登录，卡片状态从「等待登录」变为「已连接」；
2. 在 Tailscale 管理后台的 Machines 页找到 `gately-home`，在 Edit route settings 中勾选 `192.168.1.0/24` 子网路由与 Use as exit node。子网路由与出口节点需要在控制台批准后才生效；
3. 建议同时对这台设备 Disable key expiry，避免几个月后密钥过期掉线。

Headscale 中对应的命令是 `headscale nodes list-routes` 与 `headscale nodes approve-routes`（不同版本命令略有差异，以 Headscale 文档为准）。

### 5. 查看状态与节点

实例卡片显示运行方式、Tailscale IP、Tailnet 与子网路由；下方的节点卡片列出本机信息（Tailscale IP、Tailnet、MagicDNS）和每个节点的在线状态、地址与系统。

![Tailscale 节点列表](/tutorials/mesh-vpn-tailscale.zh-CN.webp)

- 卡片上的「重启」只重启这一个实例的 tailscaled；
- 「退出登录」会把本机这个节点从 tailnet 中移除，再次加入需要重新登录；
- 页面底部运行日志的来源带实例名，例如 `tailscaled:家里`。

### 6. 在手机上使用

1. 手机安装 Tailscale App，用同一个账号登录；
2. 在 App 设置里打开 Use Tailscale subnets（部分版本叫 Accept routes），手机就能直接访问 `192.168.1.10`；
3. 需要所有流量经家里出网时（例如在公共 Wi-Fi 上），在 App 里选 Exit Node 为 `gately-home`。

## NetBird 的不同之处

1. 切到「NetBird」标签，在「NetBird 在线安装」点「下载安装」；
2. 在 NetBird 管理端的 Setup Keys 页生成设置密钥（适合无人值守的设备）；
3. 「添加实例」时填写：
   - 「设置密钥」：粘贴设置密钥，留空则卡片上显示登录链接，登录页要求输入代码时按卡片上的代码填写。macOS 与 Windows 上通常只能在运行 Gately 的电脑上完成浏览器登录，远程管理请用设置密钥；
   - 「管理服务地址」：留空使用 NetBird 官方云，自建时填 `https://netbird.example.com`；
   - 「作为路由节点」：打开。NetBird 的子网路由与出口节点在管理端的 Network Routes / Networks 页配置，Gately 这里只决定是否允许本机转发；
   - 「接受路由」「接受 DNS」「拒绝入站」按需打开；
   - 「用户态网络」：容器内打开，此时「访问本机服务」允许其他节点访问本机上只监听 `127.0.0.1` 的服务，本机程序经「SOCKS5 代理」（默认 `127.0.0.1:1080`）访问组网。
4. 保存后在管理端把这台设备加入相应的分组与访问策略。

![添加 NetBird 实例：设置密钥与自建管理服务](/tutorials/mesh-vpn-netbird-dialog.zh-CN.webp)

NetBird 的节点卡片显示管理服务、信令服务与中继的连接状态，以及每个节点是「直连」还是「中继」、延迟多少。显示「空闲」的节点当前没有连接：管理端开启按需连接（默认开启）时，访问它会自动建立连接。

![NetBird 节点：直连、中继与延迟](/tutorials/mesh-vpn-netbird.zh-CN.webp)

## EasyTier 的不同之处

EasyTier 没有控制端：组网名称与网络密钥相同的节点就在同一个网络里。

1. 切到「EasyTier」标签，在「EasyTier 在线安装」点「下载安装」（同时安装 `easytier-core` 与 `easytier-cli`）；
2. 「添加实例」时填写：
   - 「组网名称」：`home-net`；
   - 「网络密钥」：一串足够长的随机字符。知道名称与密钥的人都能加入，共享网段或作为出口节点时必须设置；
   - 「节点地址」：每行一个启动时主动连接的节点，例如公共共享节点或你自己的中继 `tcp://relay.example.com:11010`；
   - 「自动分配 IP」：打开，由 EasyTier 在组网内分配虚拟 IP；也可以关闭后填固定的「虚拟 IP」，例如 `10.126.126.2`；
   - 「监听地址」：留空只主动连接其他节点，打洞与中继照常工作；作为中继节点时填 `tcp://0.0.0.0:11010`、`udp://0.0.0.0:11010` 并在防火墙放行；
   - 「子网代理」：`192.168.1.0/24`，把家庭网段共享给组网；
   - 「用户态网络」：容器内打开，此时用「端口转发」访问组网内的服务（例如 `tcp://127.0.0.1:2222/10.126.126.1:22`，建议只绑定 `127.0.0.1`）；
   - 「作为出口节点」「出口节点」「延迟优先」「KCP 代理」「QUIC 代理」按需打开，UDP 丢包严重的网络可以试 KCP / QUIC 代理。
3. 保存。其他设备用同样的组网名称与密钥、连接同一个节点地址加入。

![添加 EasyTier 实例](/tutorials/mesh-vpn-easytier-dialog.zh-CN.webp)

实例卡片会显示本机虚拟 IP、NAT 类型、公网地址、监听地址，以及每个节点是直连还是中继、延迟、丢包与流量。「NAT 类型」也可以用来判断[STUN 穿透](/docs/tutorials/stun-traversal)是否可行。

::: warning OpenWrt 上的 EasyTier
EasyTier 的 TUN 网卡（默认 `tun0`）不属于任何防火墙区域，fw4 默认丢弃它的入站，其他节点访问本机会不通。把网卡加入 lan 区域：

```bash
uci add_list firewall.@zone[0].device='tun0'
uci commit firewall
/etc/init.d/firewall reload
```

精简固件没有 `/dev/net/tun` 时先安装 `kmod-tun`，或改用「用户态网络」。
:::

## 自建控制服务器与中继

- **Headscale**：Tailscale 控制服务器的开源实现。部署在 VPS 上，用 Gately 的 [Web 服务](/docs/modules/web)给它加 HTTPS 反向代理，Tailscale 实例的「控制服务器」填 `https://hs.example.com`。官方 DERP 中继在海外，国内直连失败时延迟较高，可以在 Headscale 中配置自建 DERP；
- **NetBird 自建**：按 NetBird 官方文档部署管理服务、信令与中继，实例的「管理服务地址」填自建地址；
- **EasyTier 自建中继**：在 VPS 的 Gately 上添加一个 EasyTier 实例，「监听地址」填 `tcp://0.0.0.0:11010` 与 `udp://0.0.0.0:11010`，放行这两个端口，其他节点的「节点地址」填这台 VPS。

## 只发布网页服务：Cloudflare 隧道

组网要求每个访问设备装客户端。想让任何人用浏览器访问家里的某个网站、又没有公网 IP 和 VPS 时，用 Cloudflare 隧道：

1. 在 Cloudflare Zero Trust 控制台创建隧道，复制隧道令牌；
2. 打开「组网与穿透 → Cloudflare 隧道」，页面提示未安装时点「下载安装」；
3. 点「添加隧道」，「模式」选「令牌隧道」，粘贴「隧道令牌」；填写可选的「API 令牌」后可以在 Gately 里直接编辑路由规则并同步 DNS；网络限制 UDP 时把「协议」改为 `http2`；
4. 保存。卡片显示边缘连接后，在 Cloudflare 或卡片上的「路由规则」里把 `nas.example.com` 指向本地服务。

![添加 Cloudflare 隧道](/tutorials/mesh-vpn-cloudflared-dialog.zh-CN.webp)

「临时隧道」不需要账号，会分配一个 `trycloudflare.com` 随机地址，重启后地址会变，只适合临时分享。完整步骤与访客真实 IP 的设置见[没有公网 IPv4 怎么从外面访问 NAS · 路线 D](/docs/tutorials/remote-access-without-public-ip)，路由规则见 [Cloudflare 隧道](/docs/modules/tunnels#cloudflared-ingress)。

## 验证

```bash
# 在另一台已加入组网的电脑上
tailscale status                 # 能看到 gately-home 在线
tailscale ping gately-home       # 看是 direct 还是 via DERP
ping 192.168.1.10                # 子网路由生效后，直接访问家里的设备
curl -I http://192.168.1.10:5000/
```

- 手机关闭 Wi-Fi，用 4G 打开 `http://192.168.1.10:5000`，能打开说明子网路由生效；
- Gately 的节点卡片上，访问端显示「在线」（Tailscale）或「已连接」「直连」（NetBird、EasyTier）；
- 运行日志里没有反复重启或认证失败的记录。

## 常见问题

**卡片一直是「等待登录」。** 没填认证密钥，登录链接还没打开；或密钥已过期、已用完（一次性密钥只能用一次）。重新生成密钥后编辑实例填入，或打开卡片上的链接登录。

**手机能 ping 通 Gately，访问不了 192.168.1.x。** 子网路由没有在控制台批准，或手机 App 没有打开接受子网路由；内核模式下还要检查宿主机的 IP 转发（`net.ipv4.ip_forward=1`）与 `TS_DEBUG_FIREWALL_MODE=nftables`。

**启用实例时提示与其他实例冲突。** 同时启用的内核模式实例最多一个，状态目录、控制套接字与 WireGuard 端口也不能共用。给其中一个实例开启「用户态网络」，或换端口与目录。

**总是走中继，速度慢。** 双方都在严格的 NAT 后面时打洞会失败。固定「WireGuard 端口」并在路由器映射这个 UDP 端口；或者自建中继（Headscale DERP、NetBird 中继、EasyTier 节点）放在离你近的 VPS 上。

**在容器里报 TUN 设备不存在。** 开启「用户态网络」，或给容器加 `cap_add: [NET_ADMIN]` 与 `devices: ["/dev/net/tun"]`。

**组网地址与端口敲门、站点敲门准入。** Tailscale 与 NetBird 的地址在 `100.64.0.0/10`。[端口敲门](/docs/tutorials/port-knocking)的「内网免敲门」不包含这段地址，需要组网设备免敲门时把它加到「免敲门白名单」；Web 站点的「登录后放行」不会放行这段地址。

## 下一步

- [STUN 穿透](/docs/tutorials/stun-traversal)、[frp 内网穿透](/docs/tutorials/frp-tunnel)：对公网开放服务的路线；
- [给站点加上访问控制](/docs/tutorials/site-access-control)：只允许组网网段访问某个站点；
- [端口敲门](/docs/tutorials/port-knocking)；
- 模块文档：[Tailscale / NetBird / EasyTier / frp / cloudflared](/docs/modules/tunnels)。
