---
time: 10 分钟
description: 家里公网 IP 会变，用 Gately 的动态域名让域名自动跟着走：创建任务、填写服务商凭据、选择 IPv4 / IPv6 获取方式、添加解析记录、查看同步状态与日志、Webhook 通知，附 IPv6 场景说明。
---

# 动态域名入门

家庭宽带的公网 IP 隔几天就会变，今天用 `203.0.113.10` 能连回家，明天就不行了。动态域名（DDNS）定期检查当前的公网 IP，一旦变化就通过 DNS 服务商的 API 更新解析记录，让 `nas.example.com` 始终指向你家。本文从零创建一个任务，讲清楚 IP 从哪里来、记录怎么填、同步状态怎么看，最后说说只有 IPv6 时怎么做。

::: tip 可以先在演示里看看
在线演示 <https://demo.gately.cn> 的「网络 → 动态域名」里有 Cloudflare、阿里云、DNSPod、DuckDNS 的示例任务，点「演示账号一键进入」即可浏览。
:::

## 适用场景与前提

- 宽带有公网 IPv4，或者有公网 IPv6。不确定的话在概览点「网络自检」，看「公网 IPv4 地址」「位于 NAT 之后」「网卡上发现运营商级 NAT 地址」几条结果；没有公网 IP 时 DDNS 帮不上忙，看[没有公网 IPv4 怎么从外面访问 NAS](/docs/tutorials/remote-access-without-public-ip)；
- 一个自己的域名，解析托管在 Gately 支持的 DNS 服务商（Cloudflare、阿里云、腾讯云 DNSPod、华为云等三十多家，见[支持的服务商](/docs/modules/ddns#支持的服务商)）；
- 路由器已把要对外提供服务的端口转发到对应设备（IPv4），或者在防火墙里放行了入站端口（IPv6）。

## 结果是什么样

```text
Gately 每 300 秒：
  查询公网 IP ──▶ 203.0.113.10（和上次一样？）
                    │ 变了
                    ▼
  调用 Cloudflare API ──▶ example.com      A  203.0.113.10
                          nas.example.com  A  203.0.113.10
                    │
                    ▼
  调用 Webhook（可选）──▶ 手机收到「IP 已变为 203.0.113.10」
```

## 步骤

### 1. 创建 DNS 服务商的 API 密钥

Gately 要通过 API 修改解析，建议单独创建一个只有 DNS 编辑权限的密钥。

::: code-group

```text [Cloudflare]
1. 登录 Cloudflare 控制台，进入「我的资料 → API 令牌」，点「创建令牌」。
2. 选「编辑区域 DNS」模板，或自定义以下两项权限：
   区域 · 区域 · 读取
   区域 · DNS · 编辑
3. 「区域资源」选「包括 · 特定区域 · example.com」。
4. 创建后复制令牌（只显示一次）。
```

```text [阿里云]
1. 登录阿里云控制台，进入「访问控制 RAM → 用户」，创建用户，
   勾选「OpenAPI 调用访问」，保存 AccessKey ID 与 AccessKey Secret（Secret 只显示一次）。
2. 给这个用户授权系统策略 AliyunDNSFullAccess（云解析 DNS 完全权限）。
```

:::

同一个密钥以后还能用来[申请 HTTPS 证书](/docs/tutorials/https-certificate)的 DNS 验证。

### 2. 创建任务

1. 打开侧边栏「网络 → 动态域名」。还没有任务时，页面中间有「从常见场景开始：」三个按钮：「Cloudflare 家宽 IPv4」「阿里云 IPv4 + IPv6」「DNSPod 仅 IPv6」，点一个会打开预填好服务商、记录类型与 IP 来源的表单。也可以点右上角的「添加任务」从空白表单开始。

   ![从常见场景开始](/tutorials/ddns-basics-templates.zh-CN.webp)

2. 以「Cloudflare 家宽 IPv4」为例，在「基本」标签填写：
   - 任务名称：`家里的宽带`；
   - DNS 服务商：已选好「Cloudflare」。旁边的「获取凭据」打开服务商的密钥管理页；
   - 「API Token / Global API Key」：粘贴令牌；「账号邮箱」留空，只有使用 Global API Key 时才填；
   - 「代理状态」：填 `false`，让记录直接指向你的 IP，关闭 Cloudflare 的代理（橙色云朵）；
   - 「检查周期(秒)」：默认 `300`，每 5 分钟检查一次 IP；
   - 「强制同步(秒)」：默认 `3600`，即使 IP 没变也每小时向服务商核对一次，防止有人在控制台改了记录；`0` 表示关闭；
   - 「备注」：可选，会同步写到服务商的解析记录备注里。

   阿里云选「阿里云 DNS」，填「AccessKey ID」和「AccessKey Secret」。

   ![任务的基本设置](/tutorials/ddns-basics-task.zh-CN.webp)

### 3. 添加解析记录

切到「解析记录」标签。每一行是一条要维护的记录：

| 列 | 填什么 |
| --- | --- |
| 启用 | 停用后这条记录不再同步 |
| 主机记录 | 子域名部分：`nas` 表示 `nas.example.com`，`@` 或留空表示根域名 `example.com`，`*` 表示泛解析 |
| 主域名 | 你在服务商那里的域名，例如 `example.com` |
| 类型 | `A`（IPv4）、`AAAA`（IPv6）、`CNAME`、`TXT` |
| 记录值 | 保持 `{ipv4Addr}` 或 `{ipv6Addr}`，同步时替换为本任务获取到的地址 |
| TTL | `0` 表示用服务商的默认值 |

1. 第一行「主机记录」填 `@`，「主域名」填 `example.com`，类型 `A`，记录值 `{ipv4Addr}`。
2. 点「IPv4 记录」再加一行，「主机记录」填 `nas`，「主域名」`example.com`。
3. 要很多子域名时，点「批量添加」，每行写一个域名，一次加完，写法见[批量添加记录](/docs/modules/ddns#batch)。

![两条 A 记录：根域名与 nas](/tutorials/ddns-basics-records.zh-CN.webp)

所有子域名都指向同一个 IP 时，也可以只维护一条 `*`（泛解析）加一条 `@`，以后新加子域名不用再改 DDNS。

### 4. 选择 IP 来源

切到「IP 来源」标签，IPv4 和 IPv6 各有一组设置，打开「获取 IPv4」「获取 IPv6」决定是否获取。「获取方式」有三种：

- 「查询接口」（默认）：依次请求「查询地址」里的网址，从返回内容里取出公网 IP。适合 Gately 在路由器后面、本机只有内网地址的情况。列表里的地址可以换成任何返回 IP 的服务，包括自建的；
- 「网卡」：直接读取某块网卡上的地址。适合设备自己持有公网地址：拨号的软路由（如 `pppoe-wan`），或者 IPv6（每台设备都有自己的公网 IPv6）；
- 「命令」：执行一条命令，从输出里找 IP，例如 `curl -s https://ipv4.ddnspod.com`，适合特殊网络环境。

「筛选正则」可选，留空时自动选择第一个公网地址；网卡上有多个公网地址时用它挑出想要的那个。填好后点「测试获取」，下方显示「获取成功：…」就说明设置可用。

本例 IPv4 保持「查询接口」。家里也有 IPv6 时，打开「获取 IPv6」，「获取方式」选「网卡」，「网卡」选 Gately 所在设备连着局域网的那块（例如 `eth0`），再回到「解析记录」点「IPv6 记录」加一条类型 `AAAA`、记录值 `{ipv6Addr}` 的记录。

![IP 来源：IPv4 用查询接口，IPv6 用网卡](/tutorials/ddns-basics-ip-source.zh-CN.webp)

### 5. 设置 Webhook 通知（可选）

想在 IP 变化时收到通知，切到「Webhook」标签：

1. 打开「启用 Webhook」。
2. 「请求地址」填接收通知的地址，例如 ntfy、Bark、企业微信机器人或你自己的服务。
3. 「请求方法」选 `POST`，「请求体」写通知内容，可以用页面下方列出的变量：
   - `{ipAddr}`：本次的 IP（优先 IPv4，只有 IPv6 时为 IPv6），`{ipv4Addr}`、`{ipv6Addr}`；
   - `{successDomains}`、`{failedDomains}`：本次更新成功、失败的域名，逗号分隔；带 `Line` 后缀的版本按行分隔；
   - `{taskName}`、`{time}`。
4. 「请求头」按服务要求填，例如 `Content-Type: application/json`；「成功标识」留空时只检查 2xx 状态码；回调地址直连不通时在「代理」里选代理。

![Webhook：IP 变化时推送通知](/tutorials/ddns-basics-webhook.zh-CN.webp)

Webhook 只在有记录被创建、更新或同步失败时调用，IP 没变的检查不会触发。想把 DDNS 失败和其他告警汇总到一个地方，用[通知中心](/docs/modules/notify)订阅「动态域名：解析已更新」「动态域名：更新失败」事件更方便，见[通知与告警](/docs/tutorials/notify-alerts)。

### 6. 保存并同步

1. 点「保存」。任务立即开始第一次检查，提示「已保存，正在检查」。
2. 任务卡片上显示当前获取到的「IPv4」「IPv6」、「上次检查」与「下次检查」时间，下面是每条记录的当前值与状态：
   - 「已创建」：服务商处原来没有这条记录，Gately 新建了它；
   - 「已更新」：记录值变了，已经改好；
   - 「已是最新」：服务商处的值与当前 IP 一致，无需修改；
   - 同步出错时，状态处直接显示服务商返回的错误原因；任务右上角显示「N 条同步失败」或「获取 IP 失败」，详细过程看页面底部的日志。
3. 卡片底部的按钮：
   - 「检查」：马上执行一轮检查，只更新有变化或上次失败的记录；
   - 「强制同步」：马上把所有记录重新推送到服务商。刚建好任务或者在服务商控制台手动改过记录后用它；
   - 「编辑」「复制」「删除」。删除记录或任务时可以勾选「同时从 DNS 服务商删除解析」，只删除由 Gately 同步的那一条，详见[删除记录与任务](/docs/modules/ddns#delete)。

![任务卡片与「强制同步」](/tutorials/ddns-basics-card.zh-CN.webp)

页面底部的「运行日志」记录每一次 IP 变化、记录更新与 Webhook 调用，可以按任务（「来源」）、级别、关键词筛选，点「导出」下载。

![动态域名的运行日志](/tutorials/ddns-basics-logs.zh-CN.webp)

页头的「全局设置」里可以暂时停用整个动态域名模块，以及设置删除时「同时从 DNS 服务商删除解析」的默认勾选状态。

## IPv6 场景

很多地方的家宽没有公网 IPv4，但有公网 IPv6。IPv6 和 IPv4 的区别在于：

- 每台设备都有自己的公网 IPv6 地址，外网直接连到设备，省去路由器端口转发；
- 运营商下发的前缀会变，设备地址也随之变化，所以同样需要 DDNS；
- 光猫和路由器通常默认拦截从外网进来的 IPv6 连接，需要在路由器的 IPv6 防火墙里放行对应端口（例如 443），Gately 所在设备的防火墙也要放行。

推荐做法：

1. 任务的「IP 来源」关闭「获取 IPv4」，打开「获取 IPv6」，「获取方式」选「网卡」，选 Gately 所在设备的网卡。也可以直接点「DNSPod 仅 IPv6」模板，换成你的服务商；
2. 记录类型选 `AAAA`，记录值 `{ipv6Addr}`；
3. 「网络自检」出现「本机直接持有公网 IPv6 地址」说明这台设备拿到了公网 IPv6，外网可以直接访问它的服务；
4. 网卡上同时有固定地址和隐私扩展产生的临时地址时，「筛选正则」写固定地址的特征，例如地址后缀固定为 `::10` 时写 `::10$`。

DDNS 只更新 Gately 所在设备的地址。要让 `jellyfin.example.com` 访问另一台设备，常见的做法是让 AAAA 记录指向 Gately，再由 Gately 的 [Web 服务](/docs/tutorials/first-site)按域名反向代理到内网设备，这样只需要放行 Gately 这一台设备的端口。

只能从 IPv6 网络访问时，手机流量一般有 IPv6，公司或酒店网络不一定有。需要在纯 IPv4 网络里访问，看[没有公网 IPv4 怎么从外面访问 NAS](/docs/tutorials/remote-access-without-public-ip)里的 STUN、frp 与 Cloudflare 隧道方案。

## 验证

```bash
# 解析结果应等于你当前的公网 IP
dig +short example.com A
dig +short nas.example.com A
dig +short nas.example.com AAAA

# 对比当前的公网 IPv4
curl -s https://ipv4.ddnspod.com
```

界面上：任务卡片右上角是「正常」，记录状态是「已创建」「已更新」或「已是最新」；「网络自检」里出现「nas.example.com 解析到 203.0.113.10」。DNS 有缓存，刚改完时 `dig` 可能还返回旧值，等 TTL 过去或者直接查服务商的权威服务器。

## 常见问题

**同步失败，日志里是 Cloudflare 的认证类错误（例如 `6003 Invalid request headers`）。** 令牌填错、复制不完整或已失效，重新创建令牌并完整粘贴；同时确认「账号邮箱」是空的，邮箱有内容时 Gately 按 Global API Key 方式认证。

**同步失败：账号下未找到域名。** 「主域名」写成了子域名（例如写成了 `nas.example.com`），或者令牌没有包含该域名的「区域 · 区域 · 读取」权限。

**阿里云同步失败，提示没有权限。** RAM 子用户缺少云解析 DNS 的授权，给它加上 AliyunDNSFullAccess。

**获取 IP 失败。** 「查询接口」里的地址都访问不了：检查 Gately 能否访问外网，点「测试获取」确认，或换成别的查询地址。Docker 部署使用 bridge 网络时，「网卡」方式只能看到容器自己的网卡，改用 `--network host` 或「查询接口」。

**获取到的 IPv4 是 `100.64.x.x` 或 `10.x.x.x`。** 这是运营商级 NAT 地址或内网地址，你的宽带没有公网 IPv4。改用 IPv6，或者看[没有公网 IPv4 怎么从外面访问 NAS](/docs/tutorials/remote-access-without-public-ip)。

**解析已经更新，外网还是连不上。** DDNS 只负责让域名指向正确的 IP。继续检查：路由器是否把端口转发到了设备（IPv4）、IPv6 防火墙是否放行、运营商是否封锁了 80 / 443（国内家宽常见，换成 8443 等高位端口）。

**IP 变了，解析过了好几分钟才更新。** 默认每 300 秒检查一次，可以缩短「检查周期(秒)」，或者手动点「检查」。另外 DNS 解析本身有缓存，TTL 越短生效越快。

**Cloudflare 的橙色云朵被打开了。** 「代理状态」填了 `true`，或者在控制台手动开过。开启代理后访客连到的是 Cloudflare，非 HTTP 端口不通。个人 NAS 通常填 `false`，保存后点「强制同步」。

## 下一步

- [申请 HTTPS 证书](/docs/tutorials/https-certificate)：用同一个 DNS 密钥申请证书；
- [泛域名证书与动态域名，10 分钟配好](/docs/tutorials/wildcard-cert-ddns)：一张 `*.example.com` 证书加泛解析 DDNS；
- [端口转发入门](/docs/tutorials/port-forward-basics)：把外网端口转到内网设备；
- [STUN 穿透](/docs/tutorials/stun-traversal)：没有公网 IPv4 时在 NAT1 网络下获得可访问的端口，记录值可以用 `{STUN_规则名_PORT}` 等变量；
- 参考文档：[动态域名](/docs/modules/ddns)。
