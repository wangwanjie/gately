---
description: 没有公网 IPv4 怎么从外面访问 NAS：IPv6 加 DDNS、STUN 穿透、frp（自有 VPS）、Cloudflare Tunnel 四条路线的前提、优缺点和在 Gately 里的配置步骤。
---

# 没有公网 IPv4 怎么从外面访问 NAS

家宽被运营商放在 CGNAT 后面，或者只给了内网 IPv4，从外面连不到家里的 NAS、相册、智能家居。常见出路有四条：IPv6 加 DDNS、STUN 穿透、frp（自己的 VPS）、Cloudflare Tunnel。它们的前提和代价不同，先判断自己的处境，再选一条照着做。

## 先判断自己的处境

打开 Gately 概览页的「网络自检」，看「NAT」一项：

- 提示「本机直接持有公网 IPv6 地址」：IPv6 路线可用；
- 提示「网卡上发现运营商级 NAT 地址」（`100.64.0.0/10`）或「位于 NAT 之后」：IPv4 没有公网地址，需要 IPv6、STUN 或隧道；
- 路由器 WAN 口拿到的就是公网 IPv4（与查询到的公网地址相同）：直接做端口映射即可，不需要本文。

## 四条路线怎么选

**IPv6 + DDNS**

- 前提：家宽有公网 IPv6，路由器允许入站；访问的一端也有 IPv6；
- 优点：免费、直连，速度只受家宽限制，任意 TCP / UDP 端口；
- 代价：访问端没有 IPv6（公司、酒店、部分手机网络）就连不上；部分运营商封锁入站的常用端口。

**STUN 穿透**

- 前提：运营商或上级路由是 NAT1（全锥形）；TCP 另需路由器配合；
- 优点：不需要服务器，直连；
- 代价：公网端口由运营商分配、会变，访问地址要带端口；对称型或端口受限型 NAT 完全打不通。

**frp（自有 VPS）**

- 前提：一台有公网 IP 的服务器，一个域名；
- 优点：最稳定，任意协议，端口固定，你控制全部流量；
- 代价：要有并维护一台 VPS，流量走 VPS 的带宽。

**Cloudflare Tunnel**

- 前提：域名托管在 Cloudflare，只需要访问网页类服务；
- 优点：不需要服务器和公网 IP，免费；
- 代价：流量经过 Cloudflare；上传大小与内容类型有限制；SSH 等非网页服务要求访问端安装客户端。

选择顺序建议：访问的设备都有 IPv6，选 IPv6；有 VPS 或愿意买一台，选 frp；只开放网页类服务、不想维护服务器，选 Cloudflare Tunnel；只有自己一个人用，且 NAT 是 NAT1，STUN 最省事。只有自己和家人访问时，也可以直接用 Tailscale、NetBird、EasyTier 组网，不对公网开放任何端口，见[组网与隧道](/docs/modules/tunnels)。

四条路线的共同部分是 HTTPS：域名、证书与站点的配置见[泛域名证书与动态域名](/docs/tutorials/wildcard-cert-ddns)。

## 路线 A：IPv6 + DDNS

### 前提

- 家宽拿到了公网 IPv6，NAS 自己有一个全局单播地址（`2` 或 `3` 开头，国内常见 `240e:`、`2408:`、`2409:`）；
- 路由器的 IPv6 防火墙允许放行到 NAS 的入站连接；
- 访问的设备有 IPv6，可以用手机关闭 Wi-Fi 后访问验证。

### 步骤

1. 打开「网络 → 动态域名」，点「添加任务」，在「基本」页选 DNS 服务商并填凭据（见[泛域名证书与动态域名](/docs/tutorials/wildcard-cert-ddns)的第 1、2 步）。
2. 在「IP 来源」页关闭「获取 IPv4」，打开「获取 IPv6」。NAS 自己持有公网 IPv6 时，「获取方式」选「网卡」并选择对应网卡，需要指定其中某个地址时填「筛选正则」；也可以保持「查询接口」。点「测试获取」确认拿到的是 NAS 的地址。
3. 在「解析记录」页添加一条记录：「主机记录」`nas`，「主域名」`example.com`，类型 `AAAA`，记录值 `{ipv6Addr}`。保存后点「强制同步」。
4. 在「Web → Web 服务」添加监听，网络选「双栈」或「仅 IPv6」，端口 `443`，保持 HTTPS，再添加站点 `nas.example.com`，后端填应用地址。HTTPS 站点需要覆盖该域名的证书。
5. 放行入站连接：
   - 路由器：在 IPv6 防火墙放行目标地址为 NAS、TCP 端口为 443（或你选的端口）的入站规则；运营商下发的前缀会变，地址可以按后缀匹配；
   - Gately 运行在 OpenWrt 或开启了防火墙的 Linux 主机上：监听打开「自动放行防火墙」，Gately 在本机防火墙放行；
   - 手动放行的写法见 [Web 服务 · 手动放行](/docs/modules/web#manual-firewall)。

### 验证

```bash
# 解析到 NAS 的 IPv6 地址
dig +short AAAA nas.example.com

# 在有 IPv6 的网络里访问（手机热点或另一台有 IPv6 的机器）
curl -6 -I https://nas.example.com/
```

### 常见问题

- **端口不通。** 先在内网用 `curl -6 http://[NAS 的 IPv6]:端口` 确认服务本身可达，再检查路由器 IPv6 防火墙，最后看运营商是否封锁了入站的 80 / 443，必要时换成 `8443` 之类的高位端口；
- **前缀变化后解析没更新。** 默认每 300 秒检查一次，在任务里缩短检查周期，或点「强制同步」；
- **解析到的不是 NAS 的地址。** IPv6 临时地址或路由器地址被当作了本机地址，改用「网卡」方式并用「筛选正则」指定地址。

## 路线 B：STUN 穿透

STUN 通过 NAT 打洞得到一个公网地址与端口，外部连接经 Gately 内置转发到达 NAS。

### 前提

- 运营商或上级路由的 NAT 是 NAT1（全锥形）。端口受限或对称型 NAT 无法穿透，换路线 A、C 或 D；
- TCP 方式下，部分路由器的「NAT1」只对 UDP 生效，需要开启 UPnP，或在路由器上把通道端口映射到运行 Gately 的设备；
- 主机开启了防火墙时，要固定通道本地端口并放行它。

### 步骤

1. 打开「组网与穿透 → STUN 穿透」，点「添加规则」。
2. 在「基本」页填写：规则名称 `nas`，穿透类型 `TCP`，「通道本地端口」填一个固定端口，例如 `50443`（`0` 是随机端口，每次启动都变），「目标地址」填 NAS 的内网地址，「目标端口」填服务端口。
3. 运行在路由器或开启了防火墙的 Linux 上时，打开「自动放行防火墙」；飞牛 fnOS 等不经过 nftables / iptables 的系统防火墙需要在系统设置里手动放行这个端口。
4. 保存。规则卡片上的「公网地址」显示穿透得到的 IP 与端口，点击可复制。
5. 想用域名访问：在动态域名任务的「解析记录」里添加一条 A 记录，记录值写 `{STUN_nas_IP}`；再添加一条 TXT 记录 `port={STUN_nas_PORT}` 记录端口。地址或端口变化时引用它的任务会立即同步。变量写法见[记录值变量](/docs/modules/ddns#record-vars)。
6. 公网端口会变，所以访问地址是 `https://nas.example.com:公网端口`。想少输入端口，可以在一个固定可访问的入口（例如路线 A 的 IPv6 站点）上用「重定向」站点，跳转目标写 `https://{host}:{STUN_nas_PORT}`，见 [Web 服务的重定向变量](/docs/modules/web#redirect-vars)。

### 验证

在外网（手机流量）打开卡片上显示的公网地址；STUN 的有效性检测默认会连接自身公网地址确认可达，卡片出现错误时按提示处理。

### 常见问题

- **公网地址出现了，外部仍然连不上。** 多半是主机防火墙丢弃入站连接，固定通道本地端口并放行，或用路由器的 UPnP；
- **端口隔几天就变。** 运营商重新分配了映射，DDNS 记录值里的 STUN 变量会自动更新，访问地址要重新取，不适合给不懂技术的家人用；
- **双层 NAT 下 UPnP 映射不对。** 路由器的外网地址与 STUN 得到的公网地址不同，说明外层还有一层 NAT，细节见 [STUN 穿透 · 限制与多层 NAT](/docs/modules/stun)。

## 路线 C：frp（自有 VPS）

家里的 Gately 作为 frp 客户端，连到 VPS 上的 frps，把 NAS 的端口映射到 VPS；VPS 上的 Gately 再用域名和 HTTPS 对外提供。

```text
访客 ─HTTPS─▶ VPS 上的 Gately（证书、认证、WAF）
                └─ 127.0.0.1:15000 ◀─ frps ◀─ frpc（家里）─▶ NAS:5000
```

### 前提

- 一台有公网 IPv4 的 VPS，装好 Gately，域名解析指向 VPS；
- 家里与 VPS 上都有 Gately（家里的也可以只装 frpc）。

### 步骤

**1. VPS 上：frps 服务端**

1. 打开「组网与穿透 → frp」，在「服务端 frps」页添加 frps 实例。如果卡片提示未安装，先在页面下方的「在线安装」点「下载安装」。
2. 填写：「绑定端口」`7000`；「令牌」填一串随机字符（强烈建议设置）；「代理监听地址」填 `127.0.0.1`，frpc 申请的远程端口只在 VPS 本机可见，之后由 Gately 的 Web 服务对外；「允许的远程端口」可以限制为 `15000-15100`。
3. 打开「自动放行防火墙」，或在云厂商安全组放行 TCP 7000。保存并启用。

**2. 家里：frpc 客户端**

1. 在「frp → 客户端 frpc」点「添加 frpc 实例」，「服务端地址」填 VPS 的 IP 或域名，「服务端端口」`7000`，「令牌」与 frps 相同，保存并启用。
2. 点「添加代理」：类型 `tcp`，名称 `nas`，「本地地址」填 NAS 的内网地址，「本地端口」填服务端口（如 `5000`），「远程端口」`15000`。
3. 实例卡片显示已连接。

**3. VPS 上：用域名和 HTTPS 提供服务**

1. 在「Web → SSL 证书」签发覆盖 `nas.example.com` 的证书（[泛域名证书与动态域名](/docs/tutorials/wildcard-cert-ddns)）；
2. 在「Web 服务」添加监听（端口 `443`，HTTPS），再添加站点：域名 `nas.example.com`，类型「反向代理」，后端地址 `http://127.0.0.1:15000`。

### 验证

```bash
curl -I https://nas.example.com/
```

frpc 实例卡片显示已连接，Web 服务站点卡片的请求数增长，说明流量经过了 VPS。

### 常见问题

- **frpc 连不上。** 令牌不一致、VPS 的 7000 端口没放行，或 frps 的「绑定端口」被占用；
- **站点返回 502。** frpc 的「远程端口」与站点后端端口不一致，或 NAS 的服务没起来；
- **想让 NAS 应用看到访客的真实 IP。** VPS 上站点的「添加 X-Forwarded-*」默认开启，NAS 应用需要信任这个来源；
- **不用 HTTPS、直接暴露 TCP 端口。** frps 的「代理监听地址」留空，并放行对应远程端口。

frp 的传输协议、KCP / QUIC、frps 来源 IP 白名单等见 [frp](/docs/modules/tunnels#frp)。

## 路线 D：Cloudflare Tunnel

Gately 托管的 `cloudflared` 从家里主动连到 Cloudflare，访客访问你的域名时，流量经 Cloudflare 隧道回到家里，不需要公网 IP，也不用开放端口。

### 前提

- 域名托管在 Cloudflare；
- 只需要访问网页类服务（HTTP、HTTPS、WebSocket）。SSH、RDP 等需要访问端安装 Cloudflare 的客户端，不在本文范围；
- 了解限制：流量经过 Cloudflare；免费套餐的单个请求体上限为 100 MB，影响大文件上传；大量视频或非网页内容受 Cloudflare 服务条款约束，请自行查阅。

### 步骤

1. 在 Cloudflare 的 Zero Trust 控制台创建隧道（Networks → Tunnels，连接方式选 Cloudflared），复制隧道令牌。控制台菜单随 Cloudflare 改版会变化，以实际界面为准。
2. 在 Gately 打开「组网与穿透 → Cloudflare 隧道」，如果提示未安装 cloudflared，先在页面下方的「在线安装」点「下载安装」。
3. 点「添加隧道」，「模式」选「令牌隧道」，粘贴「隧道令牌」，保存并启用。卡片显示边缘连接。
4. 在 Gately 里准备本地服务：「Web → Web 服务」添加监听，监听地址 `127.0.0.1`，端口 `8080`，关闭 HTTPS（证书由 Cloudflare 提供），再添加站点 `nas.example.com`，类型「反向代理」，后端填 NAS 地址。
5. 配置公网主机名，两种方式任选其一：
   - 在 Cloudflare 隧道的「Public Hostname」里添加 `nas.example.com`，服务填 `http://127.0.0.1:8080`；
   - 在 Gately 隧道设置里填写「API 令牌」（需要「Cloudflare Tunnel：编辑」权限，要自动创建 DNS 记录还需要「DNS：编辑」），然后在隧道卡片点「路由规则」，添加主机名 `nas.example.com`、服务 `http://127.0.0.1:8080`，打开「同步 DNS」后保存。
6. 想让访问日志、IP 名单里是访客真实 IP：在「Web 服务 → 全局设置 → 访客真实 IP」，「可信代理」填 `127.0.0.1`，「真实 IP 请求头」选 `CF-Connecting-IP`。因为 cloudflared 在本机连接 Gately，并且这个监听只监听回环地址，没有其他来源能伪造这个头。

### 验证

```bash
curl -I https://nas.example.com/
```

响应头里有 `server: cloudflare`，同时 Gately 站点的请求数增长，说明流量经隧道回到了家里。

### 常见问题

- **隧道卡片一直是「连接中」。** 网络限制了 UDP，把隧道的「协议」改为 `http2`；
- **Cloudflare 返回 502 或 1033。** cloudflared 没连上，或公网主机名指向的本地服务没有监听，检查第 4 步的端口；
- **大文件上传失败。** 超过 Cloudflare 的请求体上限，换路线 A 或 C；
- **WebSocket 应用异常。** Cloudflare 支持 WebSocket，先确认应用自己在反向代理下的设置，参考[给常见应用加 HTTPS 和登录](/docs/tutorials/https-and-login-for-apps)。

## 相关文档

- [STUN 穿透](/docs/modules/stun)、[动态域名](/docs/modules/ddns)；
- [Tailscale / NetBird / EasyTier / frp / cloudflared](/docs/modules/tunnels)；
- [一个 443 端口跑所有服务](/docs/tutorials/one-port-443)；
- [给常见应用加 HTTPS 和登录](/docs/tutorials/https-and-login-for-apps)。
