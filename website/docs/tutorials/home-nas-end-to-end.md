---
time: 90 分钟
description: 一篇从零做完的实战：把家里 NAS 上的相册、影音、文件和 Home Assistant 安全发布到外网，依次完成网络判断、DDNS、泛域名证书、HTTPS 监听与多站点、家人共享账号、入侵防护与 WAF、通知、家庭导航页和配置备份。
---

# 家庭 NAS 外网访问全流程

家里有一台 NAS，跑着 Immich 相册、Jellyfin 影音、文件管理和 Home Assistant。现在只能在家里用，想出门也能访问，又担心直接开端口被扫、被爆破。本文把整个过程串成一条线，从判断网络条件开始，一直做到家人有自己的账号、异常有通知、配置有备份。每一步给出关键配置和截图，细节链接到对应的教程，照着一路做下来，最后得到：

- `photos`、`media`、`files`、`ha`、`home` 五个 `*.example.com` 子域名，全部 HTTPS，证书自动续签；
- 家里公网 IP 变化后，解析几分钟内自动跟上；
- 家人用各自的账号登录文件和导航页，登录一次处处免登；
- 扫描、爆破、注入攻击被 WAF 拦截、被入侵防护封禁，封禁和证书问题推送到手机；
- `https://home.example.com` 是一张家庭主页，内网打开自动走内网地址；
- 配置每天自动备份，改坏了能回滚。

::: tip 先看看界面
在线演示 [demo.gately.cn](https://demo.gately.cn)（点「演示账号一键进入」，只读，每小时重置）里有和本文几乎一样的示例数据：监听、站点、证书、DDNS、入侵防护、通知与导航面板都能点开看。
:::

## 适用场景与前提

- 一台 NAS 或家用服务器（群晖、飞牛 fnOS、威联通、Unraid、普通 Linux 都可以），已经装好要发布的应用；
- Gately 装在这台 NAS 上（Docker 用 host 网络），或者装在家里另一台常开的设备、软路由上。还没装时先看[安装并登录 Gately](/docs/tutorials/getting-started)；
- 一个自己的域名 `example.com`，DNS 托管在 Cloudflare、阿里云、DNSPod 等 Gately 支持的服务商；
- 能登录家里的路由器做端口映射。

本文的示例环境，照做时换成自己的：

| 设备或服务 | 地址 | 对外域名 |
| --- | --- | --- |
| NAS（同时运行 Gately） | `192.168.1.10` | |
| Immich 相册 | `http://192.168.1.10:2283` | `photos.example.com` |
| Jellyfin 影音 | `http://192.168.1.10:8096` | `media.example.com` |
| 文件管理（File Browser 或 NAS 自带的网页文件管理） | `http://192.168.1.10:8080` | `files.example.com` |
| Home Assistant | `http://192.168.1.22:8123` | `ha.example.com` |
| 家庭导航页 | Gately 自己提供 | `home.example.com` |

截图来自演示实例，里面的后端地址是 `192.0.2.x` 这样的示例地址，和上表不同，不影响操作。

## 结果是什么样

```text
                        ┌──────────── 外网（手机流量、公司、酒店） ────────────┐
                        │  https://photos | media | files | ha | home.example.com │
                        └──────────────────────────┬───────────────────────────┘
                                                   │ DNS（DDNS 自动更新）
                                                   ▼
                                      家里公网 IP :443（路由器映射）
                                                   │
┌──────────────────────────── NAS 192.168.1.10 ────┼──────────────────────────────┐
│ Gately                                           ▼                              │
│   入侵防护（nftables 封禁） ─▶ Web 服务 · HTTPS 监听 :443（*.example.com 证书）  │
│                                 ├─ photos  ─▶ WAF ─▶ Immich      :2283           │
│                                 ├─ media   ─▶ WAF ─▶ Jellyfin    :8096           │
│                                 ├─ files   ─▶ 网页认证 + WAF ─▶ 文件管理 :8080   │
│                                 ├─ ha      ─▶ WAF ─▶ Home Assistant 192.168.1.22 │
│                                 ├─ home    ─▶ 网页认证 ─▶ 导航面板               │
│                                 └─ 默认站点 ─▶ 断开连接（按 IP 扫描的请求）      │
│   动态域名 · SSL 证书（ACME 自动续签） · 通知中心 · 定时备份                     │
└─────────────────────────────────────────────────────────────────────────────────┘
```

整个流程分十步，前五步让服务能从外面打开，后五步让它安全、好用、可恢复：

| 步骤 | 做什么 | 详细教程 |
| --- | --- | --- |
| 1 | 判断网络条件，选一条入口路线 | [没有公网 IPv4 怎么从外面访问 NAS](/docs/tutorials/remote-access-without-public-ip) |
| 2 | DDNS：域名跟着公网 IP 走 | [动态域名入门](/docs/tutorials/ddns-basics) |
| 3 | 泛域名证书 | [泛域名证书与动态域名](/docs/tutorials/wildcard-cert-ddns)、[申请 HTTPS 证书](/docs/tutorials/https-certificate) |
| 4 | HTTPS 监听与路由器映射 | [添加第一个站点](/docs/tutorials/first-site) |
| 5 | 逐个添加站点 | [给 Jellyfin、Immich、Home Assistant、Vaultwarden 加 HTTPS 和登录](/docs/tutorials/https-and-login-for-apps) |
| 6 | 网页登录与家人的共享账号 | [给站点加上登录认证](/docs/tutorials/site-access-control) |
| 7 | 入侵防护与 WAF | [入侵防护与 WAF](/docs/tutorials/guard-waf) |
| 8 | 通知 | [通知与告警](/docs/tutorials/notify-alerts) |
| 9 | 导航面板做家庭主页 | [导航面板](/docs/tutorials/navigation-panel) |
| 10 | 配置备份与回滚 | [配置历史与备份](/docs/tutorials/history-backup) |

## 步骤

### 1. 判断网络条件

先弄清楚外面的访客怎样才能连到家里，这决定后面端口映射和 DNS 记录怎么做。

1. 打开 Gately 的「概览」页，点「网络自检」；
2. 看「NAT」与「80/443 端口」几项的结论；
3. 对照下表选路线：

| 自检结论 | 你的处境 | 本文怎么走 |
| --- | --- | --- |
| 路由器 WAN 口拿到的地址与查询到的公网 IPv4 相同 | 有公网 IPv4 | 按本文默认路线：DDNS 写 A 记录，路由器把 443 映射到 NAS |
| 发现运营商级 NAT 地址（`100.64.0.0/10`）或位于 NAT 之后，但「本机直接持有公网 IPv6 地址」 | 只有公网 IPv6 | DDNS 改写 AAAA 记录，路由器放行 IPv6 入站 443。访问端也要有 IPv6 |
| 既没有公网 IPv4 也没有可用的 IPv6 | 内网 | 换成 VPS 做入口，见[云服务器做入口，回家保留真实 IP](/docs/tutorials/vps-front-door)；第 4 步之后的内容照常适用 |

再确认两件事：

- **运营商是否封 80、443。** 国内家宽普遍封锁入站 80 与 443。被封时把对外端口换成 `8443` 之类的高位端口（下文第 4 步），访问地址写成 `https://photos.example.com:8443`；
- **路由器能否做端口映射。** 光猫拨号时要在光猫上映射，或者把光猫改桥接、由路由器拨号。

下文以「有公网 IPv4、运营商未封 443」为例；被封时把出现的外部 443 换成你的端口。

### 2. 动态域名：让域名跟着公网 IP 走

家宽的公网 IP 会变，DDNS 定期检查并更新 DNS 记录。

1. 在 DNS 服务商处创建一个只有解析读写权限的 API 密钥（Cloudflare 的 API 令牌、阿里云的 AccessKey 等）；
2. 打开「网络 → 动态域名」，点「添加任务」；
3. 「基本」页选 DNS 服务商、填凭据；Cloudflare 的「代理状态」填 `false`；
4. 「解析记录」页添加两条记录：「主机记录」`@` 与 `*`，「主域名」`example.com`，类型 `A`，记录值保持 `{ipv4Addr}`。只有 IPv6 时改加 `AAAA` 记录、记录值 `{ipv6Addr}`，并在「IP 来源」页打开「获取 IPv6」；
5. 保存，在任务卡片上点「强制同步」。

![动态域名任务卡片：每条记录显示「已是最新」](/tutorials/home-nas-end-to-end-ddns.zh-CN.webp)

用 `*` 泛解析，之后加多少个子域名都不用再改 DNS。卡片上每条记录显示「已创建」「已更新」或「已是最新」即为成功。详细步骤与各服务商的凭据获取见[动态域名入门](/docs/tutorials/ddns-basics)。

### 3. 申请泛域名证书

一张 `*.example.com` 证书覆盖所有子域名，之后加站点不用再申请。

1. 打开「Web → SSL 证书」，点「添加证书」；
2. 来源保持「ACME 自动申请」；「域名」两行：`example.com` 与 `*.example.com`；「账户邮箱」填真实邮箱；
3. 验证方式选「DNS-01（支持通配符）」，DNS 服务商与凭据填第 2 步的那一套；
4. 保存，等页面下方的运行日志显示签发成功，证书卡片出现「剩余 N 天」。

DNS-01 验证不需要开放 80 端口，运营商封了 80、443 也能签发。证书默认在到期前 30 天自动续签。步骤截图见[泛域名证书与动态域名](/docs/tutorials/wildcard-cert-ddns)的第 3 步。

### 4. 建 HTTPS 监听并在路由器上映射端口

1. 打开「Web → Web 服务」，点「添加监听」：
   - 名称：`公网 HTTPS 入口`；
   - 端口：`443`。NAS 的 443 已被系统占用（群晖、飞牛的管理页常占用）时改用 `8443`；
   - HTTPS：保持开启，证书会按域名自动从「SSL 证书」里选；
   - 展开「高级设置」：「网络」保持「双栈」（同时接受 IPv4 与 IPv6）；「同端口 HTTP」选「跳转到 HTTPS」，输错成 `http://` 也能打开；Gately 运行在开启了防火墙的 Linux 或 OpenWrt 上时，打开「自动放行防火墙」；
2. 保存，监听卡片显示「运行中」；
3. 在路由器的「端口映射 / 虚拟服务器」里添加一条：外部端口 `443`，内部 IP `192.168.1.10`，内部端口为第 1 步填的端口，协议 TCP；
4. 外部端口与监听端口不同时（例如外部 443 映射到内部 8443），编辑监听，在「高级设置」的「访问地址」填外部端口，站点列表里生成的链接才正确。

监听加好后，默认站点处理所有未配置的域名。把默认站点的类型设为「断开连接」：按 IP 扫描的机器人拿不到任何响应，配合第 7 步的「未知域名扫描」规则直接封禁。

### 5. 逐个添加站点

在监听卡片上点「添加站点」，每个服务一个站点。以文件管理为例：

1. 「域名」填 `files.example.com`；
2. 「名称」填 `文件`；
3. 「类型」选「反向代理」；
4. 「后端地址」填 `http://192.168.1.10:8080`；
5. 「选项」保持「添加 X-Forwarded-*」勾选，应用才能拿到访客真实 IP 和原始协议；
6. 保存。

![添加站点：域名、类型与后端地址](/tutorials/home-nas-end-to-end-site-basic.zh-CN.webp)

其余几个站点按同样方法添加，每个应用还要在自己那边做一点设置：

| 域名 | 后端地址 | 应用侧要做的事 |
| --- | --- | --- |
| `photos.example.com` | `http://192.168.1.10:2283` | 给 `immich-server` 设置 `IMMICH_TRUSTED_PROXIES`；手机 App 的服务器地址改成 `https://photos.example.com` |
| `media.example.com` | `http://192.168.1.10:8096` | Jellyfin「控制台 → 网络」的「已知代理」填 Gately 的地址 |
| `ha.example.com` | `http://192.168.1.22:8123` | 打开「信任 X-Forwarded-For」并把 `192.168.1.10` 加入可信代理，否则返回 400 |

WebSocket 自动支持，Gately 也不限制上传大小。每个应用的细节（真实 IP、手机 App、大文件上传）见[给 Jellyfin、Immich、Home Assistant、Vaultwarden 加 HTTPS 和登录](/docs/tutorials/https-and-login-for-apps)。

::: tip 快速建站
不想手填时，用 Web 服务页头的「快速建站」：填域名和后端地址，在「常见应用」里选 Immich、Jellyfin、Home Assistant 等模板，端口和推荐选项（例如影音的长写出超时）自动填好。
:::

全部加完后，监听卡片是这个样子，每个站点下方显示证书剩余天数：

![一个 HTTPS 监听下的多个站点](/tutorials/home-nas-end-to-end-listener.zh-CN.webp)

到这里，用手机流量打开 `https://photos.example.com` 已经能看到 Immich。后面五步把它变得安全和好用。

### 6. 网页登录与家人的共享账号

先想清楚哪些站点加 Gately 的登录：

- **只在浏览器里用的**（文件管理、导航页）：开网页认证，访客先看到 Gately 的登录页；
- **有手机或电视 App 的**（Immich、Jellyfin、Home Assistant）：App 无法显示 Gately 的登录页，这些站点用应用自己的账号，再由第 7 步的 WAF 与入侵防护兜底。

给家人建共享账号：

1. 打开「Web → 认证用户」，在「共享用户」卡片点「添加共享用户」；
2. 给每位家人建一个账号：「用户名」`mom`、「密码」、「显示名」`妈妈`；管理员自己点「启用 TOTP」扫码，多一层动态验证码；
3. 「单点登录域」填 `example.com`：在任一站点登录后，其他站点不用再登录，有效期 7 天；
4. 想让家人能自己改密码，打开「自助修改密码」；
5. 点页面底部的「保存设置」。

给文件站点开登录：编辑 `files.example.com`，切到「安全」页：

1. 「认证」勾选「网页认证（浏览器显示登录页）」；
2. 「共享用户」勾选「使用共享用户（支持 TOTP 与单点登录）」；
3. 保存。

![文件站点：网页认证、共享用户与 WAF](/tutorials/home-nas-end-to-end-site-security.zh-CN.webp)

导航页 `home.example.com` 在第 9 步建好后用同样的方式开登录。站点多时，在「认证用户」页的「使用共享用户的站点」卡片里批量勾选更快。

有 App 的应用想再加一道门，可以在站点「安全」页把「国家过滤」设为「仅允许」中国，或者只给管理路径加登录，做法见[给站点加上登录认证](/docs/tutorials/site-access-control)。家人想用 Gately 账号直接登录应用本身（例如 Grafana、Gitea），见 [OIDC 单点登录](/docs/tutorials/oidc-sso)。

### 7. 入侵防护与 WAF

两者分工：WAF 检查每个请求的内容，拦住 SQL 注入、路径穿越这类攻击；入侵防护按行为统计来源 IP，扫描、爆破达到次数后封禁整个 IP。

**WAF：**

1. 打开「安全 → WAF 防火墙」，点「添加实例」，规则组保持 OWASP CRS，「模式」先选「仅检测」，名称 `站点防护`；
2. 回到各站点的「安全」页，「WAF 实例」选它（上一张截图的最后一项）；
3. 观察一两天，在 WAF 页的「拦截与命中日志」里确认正常使用没有被误判，再把实例改为「拦截」。

相册要上传大视频：给 Immich 单独建一个实例，「请求体检查」的「超过上限时」设为「只检查前面部分」，否则超过 12.5 MB 的上传会得到 413。

**入侵防护：**

1. 打开「安全 → 入侵防护」，切到「设置」页；
2. 先在「白名单地址」里填你常用的外部出口 IP（例如公司出口 `203.0.113.50`），「忽略内网地址」保持开启；
3. 打开「同步到系统防火墙」，封禁写进 nftables，被封的 IP 连 TCP 握手都完成不了；
4. 点「保存」。

![入侵防护设置：同步到系统防火墙与白名单](/tutorials/home-nas-end-to-end-guard.zh-CN.webp)

5. 切到「日志规则」页，点「从模板添加」，依次添加：
   - 「Web 诱饵路径」：访问 `/.env`、`/.git`、`wp-login.php` 一次即封；
   - 「未知域名扫描」：按 IP 访问或用未配置的域名访问，1 分钟内 5 次即封，配合第 4 步的默认站点「断开连接」；
   - 「Web 404 探测」：1 分钟内 10 次 404 即封；
   - 「WAF 拦截」：被 WAF 多次拦截的来源；
   - 「Home Assistant」：读取名为 `homeassistant` 的容器日志，封禁反复登录失败的 IP（需要先在 Docker 模块里连接 Docker）；
6. 站点的网页认证连续失败会按「认证失败限流与自动封禁」处理，不用另加规则。

Docker 部署时容器需要 host 网络和 `NET_ADMIN` 权限，防火墙同步才会生效，卡片顶部「系统防火墙」显示「已生效」。群晖等没有 nf_tables 的系统自动改用 iptables 与 ipset。完整说明见[入侵防护与 WAF](/docs/tutorials/guard-waf)。

### 8. 通知：出事第一时间知道

1. 打开「系统 → 通知中心」，点「添加渠道」，选 Telegram、Bark、企业微信、邮件等任一种，填好后点卡片上的「测试」，手机收到测试消息即可；
2. 点「添加规则」，从「模板」里选：
   - 「证书到期」：证书即将到期或续期失败；
   - 「关键故障」：DDNS 更新失败、上游不可用、备份失败等；
   - 「安全与登录」：IP 被封禁、登录失败、新地址登录；
3. 规则的「渠道」选刚加的渠道，「静默(秒)」保持默认，同一来源的同类事件在这段时间里只推送一次；
4. 保存。

![两条通知规则：安全事件与运维事件](/tutorials/home-nas-end-to-end-notify.zh-CN.webp)

入侵防护刚开的头几天封禁很多，「IP 已被封禁」推送会很频繁。可以把这条放进一个只发 Bark 或邮件的规则，或把「最低级别」调到「警告」。各渠道的申请方法见[通知与告警](/docs/tutorials/notify-alerts)。

### 9. 导航面板：家庭主页

家人记不住五个域名，给他们一个主页：

1. 打开「Web → 导航面板」，点「新建面板」，「面板名称」`家庭服务导航`；
2. 在「链接」页点「从 Web 服务站点添加」，勾选相册、影音、文件、Home Assistant，一次生成链接；
3. 编辑每个链接，「内网地址」填内网地址，例如相册填 `http://192.168.1.10:2283`。家人在家打开主页时自动走内网地址，速度更快，也不绕路由器回流；
4. 用分组把「家人常用」和「管理」分开，「页面设置」里改页面标题和背景；
5. 保存；
6. 回到「Web 服务」，在 443 监听上「添加站点」：「域名」`home.example.com`，「类型」选「导航面板」，「导航面板」选刚建的面板，再按第 6 步在「安全」页开网页认证与共享用户。

![导航面板编辑：按分组排列的链接](/tutorials/home-nas-end-to-end-panel.zh-CN.webp)

家人只记住 `home.example.com` 一个地址：登录一次，点开文件管理不用再登录（单点登录），点开相册、影音用 App 或网页里的应用账号。手机浏览器里「添加到主屏幕」后就像一个 App。更多布局和图标设置见[导航面板](/docs/tutorials/navigation-panel)。

### 10. 配置备份与回滚

所有配置都在 Gately 的配置目录里，丢了要从头再来，花五分钟设好备份：

1. 打开「系统 → 设置」，切到「备份与迁移」页；
2. 先点「下载备份」存一份到电脑；
3. 在「定时备份」卡片打开右上角的开关，「执行计划」保持 `0 3 * * *`（每天 03:00），「保留数量」`7`；
4. 「加密口令」填一个口令，备份里含证书私钥和密码哈希，加密后放在哪都安心。口令记到密码管理器里，丢了无法解密；
5. NAS 有别的磁盘或另一台机器的 SMB/WebDAV 共享时，先在「存储挂载」里挂上，再在「远程目的地」点「添加」，备份会同时写到那里；
6. 点「保存设置」，再点「立即备份」确认能成功。

![定时备份：每天 03:00，保留 7 份](/tutorials/home-nas-end-to-end-backup.zh-CN.webp)

另外，Gately 每次保存配置前都会保留旧版本：「系统 → 日志 → 配置历史」里可以对比任意两次修改的差异，一键回滚。改坏了某个站点时用它，整台机器出问题时用备份恢复。见[配置历史与备份](/docs/tutorials/history-backup)。

## 验证

在**家外面的网络**（手机关掉 Wi-Fi）做以下检查：

```bash
# 1. 解析指向家里的公网 IP
dig +short photos.example.com

# 2. 证书有效，HTTP 自动跳到 HTTPS
curl -I http://files.example.com/
curl -I https://photos.example.com/

# 3. 按 IP 访问被直接断开（默认站点为「断开连接」）
curl -kI https://203.0.113.10/

# 4. WAF 生效：带注入特征的请求被拦（实例为「拦截」模式时返回 403）
curl -s -o /dev/null -w '%{http_code}\n' "https://files.example.com/?id=1%20UNION%20SELECT%20password%20FROM%20users"
```

然后在界面上确认：

- 「概览」的「网络自检」各项通过；
- 打开 `https://home.example.com`，出现 Gately 登录页，用家人账号登录，再点「文件」直接进入；
- Immich、Jellyfin 的手机 App 用新地址登录并同步一次；
- Home Assistant 正常打开登录页；
- 「入侵防护」的封禁列表里陆续出现被封的扫描 IP，手机收到通知；
- 「设置 → 备份与迁移」的「最近一次」显示刚才的备份成功。

## 常见问题

::: details 在家里用域名打不开，外面可以
路由器不支持 NAT 回流（从内网访问自己的公网 IP）。三种办法任选：在路由器或内网 DNS 里把 `*.example.com` 解析到 `192.168.1.10`；在家时用导航面板，链接自动走内网地址；或换一台支持回流的路由器。
:::

::: details 外面打不开，连接超时
按顺序排查：`dig` 解析的 IP 是否等于家里当前公网 IP（不等就去 DDNS 卡片点「强制同步」）；路由器映射的内部 IP 与端口是否正确；运营商是否封锁 443（换高位端口试试）；光猫是否还有一层 NAT（光猫 WAN 地址是 `100.64.x.x` 或内网地址时，回到第 1 步换路线）。
:::

::: details Home Assistant 返回 400: Bad Request
Home Assistant 没有信任 Gately 这个反向代理，按第 5 步的表格设置可信代理并重启。
:::

::: details 应用里所有访客都显示成 NAS 的 IP
应用没有信任 Gately 的地址。Immich 设置 `IMMICH_TRUSTED_PROXIES`，Jellyfin 填「已知代理」，并确认站点「添加 X-Forwarded-*」开着。
:::

::: details 上传大视频失败，返回 413
相册站点用的 WAF 实例在检查请求体，按第 7 步给 Immich 单独建一个「只检查前面部分」的实例。
:::

::: details 把自己封了
在家里内网的地址不会被封（「忽略内网地址」）。在外面被封时，用手机流量登录管理界面，在「入侵防护」的封禁列表里点「解封」，并把出口 IP 加进白名单。管理界面也被挡住时，到家用内网地址登录处理。
:::

::: details 证书签发失败
多数是 DNS 凭据权限不够或填错，看「SSL 证书」页下方的运行日志。见[泛域名证书与动态域名](/docs/tutorials/wildcard-cert-ddns)的常见问题。
:::

## 下一步

- 没有公网 IP、想用云服务器做入口：[云服务器做入口，回家保留真实 IP](/docs/tutorials/vps-front-door)；
- 运营商只给一个端口，还想对外开 SSH、远程桌面：[一个 443 端口跑所有服务](/docs/tutorials/one-port-443)；
- 让应用本身用 Gately 账号登录：[OIDC 单点登录](/docs/tutorials/oidc-sso)；
- 不对公网开放任何端口，只给自己和家人用：[组网](/docs/tutorials/mesh-vpn)；
- 更进一步隐藏端口：[端口敲门](/docs/tutorials/port-knocking)；
- 模块参考：[Web 服务](/docs/modules/web)、[SSL 证书](/docs/modules/ssl)、[动态域名](/docs/modules/ddns)、[入侵防护](/docs/modules/guard)、[WAF 防火墙](/docs/modules/waf)、[通知中心](/docs/modules/notify)、[备份](/docs/modules/backup)。
