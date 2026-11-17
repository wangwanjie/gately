---
video: true
time: 15 分钟
description: 用 Gately 的「SSL 证书」免费申请 HTTPS 证书：DNS 验证与 HTTP 验证怎么选、证书颁发机构、自动续签与到期提醒，把 HTTP 监听升级为 HTTPS 并自动跳转，以及部署给其他程序。
---

# 申请 HTTPS 证书

[上一篇](/docs/tutorials/first-site)里 `http://nas.example.com` 已经能访问了，但浏览器地址栏显示「不安全」，密码以明文在网络上传输，Passkey、Vaultwarden 网页端这类功能也要求 HTTPS。本文用 Gately 的「SSL 证书」通过 ACME 向 Let's Encrypt 免费申请证书，Gately 会在到期前自动续签；再把监听升级成 HTTPS，让 HTTP 访问自动跳过去。做完之后，`https://nas.example.com` 显示锁形图标，证书永远不用手动更换。

::: tip 可以先在演示里看看
在线演示 <https://demo.gately.cn> 的「Web → SSL 证书」里有 ACME 签发与上传的示例证书，点「演示账号一键进入」即可浏览。
:::

## 适用场景与前提

- 已按[第一个反向代理站点](/docs/tutorials/first-site)建好 `nas.example.com` 站点；
- 有一个自己的域名，并能登录它的 DNS 服务商控制台。证书机构只给公网上真实存在的域名签发证书，本文的 `example.com` 请换成你的域名；
- Gately 能访问外网（Let's Encrypt 与 DNS 服务商的 API）；
- 一个真实可收信的邮箱，作为 ACME 账户邮箱。

## 结果是什么样

```text
浏览器 ── http://nas.example.com ──▶ :80  ──301──┐
浏览器 ── https://nas.example.com ─▶ :443 ◀──────┘   监听「局域网 HTTPS」
                                        │ 证书 nas.example.com（Let's Encrypt，自动续签）
                                        ▼
                                   NAS 192.168.1.10:5000
```

- 「SSL 证书」里有一张覆盖 `nas.example.com` 的证书，有效期 90 天，默认在到期前 30 天自动续签；
- 监听改成 HTTPS 443，同时在 80 端口接收 HTTP 请求并跳转到 HTTPS；
- 监听按访问的域名自动从「SSL 证书」里挑选证书，站点行显示「证书剩余 N 天」。

## 先选验证方式

证书机构签发前要确认你控制着这个域名，ACME 有两种验证方式：

| | DNS-01（支持通配符） | HTTP-01（无需 DNS 服务商） |
| --- | --- | --- |
| 原理 | Gately 用服务商 API 临时添加一条 `_acme-challenge` TXT 记录 | 证书机构从公网访问 `http://域名/.well-known/acme-challenge/...` |
| 需要 | DNS 服务商的 API 密钥 | 域名的公网解析指向你，公网 80 端口能连到 Gately |
| 通配符 `*.example.com` | 支持 | 不支持 |
| 只在局域网使用的域名 | 可以 | 不行 |

国内家庭宽带通常封锁入站 80 端口，域名也常常只在家里的 DNS 里指向内网地址，这两种情况都选 DNS-01。云服务器上、80 端口对公网开放时，HTTP-01 最省事。

## 步骤

### 1. 准备 DNS 服务商的 API 密钥（DNS-01）

为 Gately 单独创建一个只有 DNS 编辑权限的密钥。

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
2. 给这个用户授权系统策略 AliyunDNSFullAccess（云解析 DNS 完全权限），
   或自定义只含目标域名解析读写的策略。
```

:::

Gately 表单里服务商旁边的「获取凭据」链接会打开对应服务商的密钥管理页面。其他服务商（腾讯云 DNSPod、华为云、GoDaddy 等三十多家）见[动态域名支持的服务商](/docs/modules/ddns#支持的服务商)，表格里「可用于证书 DNS 验证」为「是」的都能用。

### 2. 添加证书

1. 打开侧边栏「Web → SSL 证书」，点右上角「添加证书」。
2. 「来源」保持「ACME 自动申请」。
3. 「域名」填 `nas.example.com`，每行一个，可以写多个，例如再加一行 `photos.example.com`。
4. 「方式」选「DNS-01（支持通配符）」。
5. 「DNS 服务商」与凭据：
   - Cloudflare：选「Cloudflare」，把令牌粘贴到「API Token / Global API Key」，「账号邮箱」留空（只有使用 Global API Key 时才填）；「代理状态」只对动态域名有用，这里留空；
   - 阿里云：选「阿里云 DNS」，填「AccessKey ID」与「AccessKey Secret」。

   ![添加证书：DNS-01，Cloudflare 令牌](/tutorials/https-certificate-dns.zh-CN.webp)

6. 展开「高级设置」：
   - 「账户邮箱」：填你的真实邮箱。留空时使用「续签设置」里的默认邮箱；
   - 「证书颁发机构」：保持「Let's Encrypt」。其他选项有「Let's Encrypt（测试环境）」（调试用，签出的证书浏览器不信任）、「ZeroSSL」与「Google Trust Services」（需要在各自控制台获取 EAB Key ID 与 EAB HMAC Key 填进来）、「自定义」（内网自建的 ACME 服务，填目录地址）；
   - 「密钥类型」保持「ECC P-256」，「证书配置」保持「默认（由 CA 决定，通常 90 天）」；
   - 「网络」保持「自动」。主机有 IPv6 地址但 IPv6 实际不通时改为「仅 IPv4」；
   - 「主域名」留空，Gately 会自动找出验证记录所在的 DNS 区域；
   - 「名称（文件名）」：填 `nas.example.com`，用作卡片标题，以及写入目录、部署时的文件名。

   ![高级设置：账户邮箱与证书颁发机构](/tutorials/https-certificate-advanced.zh-CN.webp)

7. 点「保存」。签发立即在后台开始，卡片状态依次是「待签发」「签发中」，页面底部的「运行日志」显示进度：添加 TXT 记录、等待生效、验证、删除记录。一般 1 到 2 分钟完成，卡片右上角变成「剩余 89 天」这样的天数。

#### 改用 HTTP-01 验证

满足两个条件时可以用 HTTP-01：域名的公网 A / AAAA 记录指向你的公网 IP（家宽用[动态域名](/docs/tutorials/ddns-basics)维护），公网 TCP 80 端口能连到 Gately（路由器把 80 端口转发到 Gately 所在设备）。

1. 「添加证书」里「域名」照常填写，「方式」选「HTTP-01（无需 DNS 服务商）」。
2. 「临时监听端口」保持 `0`（表示 80）。Web 服务已经在 80 端口监听时由它直接应答验证请求，现有站点不受影响；80 端口空闲时 Gately 在签发期间临时监听，完成后关闭。路由器把公网 80 转发到本机其他端口（例如 8080）时，这里填 `8080`。
3. 「高级设置」里填账户邮箱，点「保存」。

![HTTP-01 验证](/tutorials/https-certificate-http01.zh-CN.webp)

### 3. 检查续签设置

点 SSL 证书页头的「续签设置」：

- 「自动续签」：默认开启；
- 「到期前多少天续签」：默认 30。Let's Encrypt 证书有效期 90 天，相当于每 60 天自动换一次；
- 「短期证书提前小时数」：只对约 6 天有效期的短期证书生效，默认 24；
- 「默认账户邮箱」：新证书的「账户邮箱」留空时用它，填一次就不用每张证书都填。

![续签设置](/tutorials/https-certificate-renew.zh-CN.webp)

Gately 每 30 分钟检查一次所有证书，到了续签时间就在后台重新签发，新证书立即生效，监听保持运行。签发失败时 6 小时后重试，日志里有失败原因；卡片上的「续签」按钮可以随时手动续签。

证书卡片显示域名、「有效期至」、「颁发者」与「更新时间」，底部有「续签」「下载」（证书、私钥、颁发者证书）「编辑」「复制」「删除」。

![证书卡片](/tutorials/https-certificate-card.zh-CN.webp)

### 4. 设置到期提醒

续签是自动的，但 DNS 密钥被删、80 端口被封这类变化会让续签悄悄失败。Gately 在这些地方提醒你：

- 证书剩余不到 7 天时发出「证书：即将到期」事件（同一张证书每天最多一次），签发失败时发出「证书：续期失败」事件。在[通知中心](/docs/modules/notify)添加一个渠道（Telegram、邮件、企业微信等），套用「证书到期」模板订阅这两个事件，就能收到推送，做法见[通知与告警](/docs/tutorials/notify-alerts)；
- 概览「服务状态」里的「SSL 证书」显示最近到期的证书与剩余天数；
- 「网络自检」对 14 天内到期的证书给出警告；
- Web 服务的站点行在证书 14 天内到期或已过期时显示警告。

### 5. 把监听升级为 HTTPS 并自动跳转

1. 打开「Web → Web 服务」，在「局域网 HTTP」卡片底部点「编辑监听」。
2. 修改：
   - 名称：`局域网 HTTPS`；
   - 端口：`443`；
   - HTTPS：打开；
   - 展开「高级设置」，「HTTP 端口」填 `80`，右侧选「跳转到 HTTPS」。这样同一个监听再监听一个明文 80 端口，与 443 共用全部站点，HTTP 请求被重定向到 `https://`，一个监听同时处理 HTTP 与 HTTPS；
   - 「最低 TLS 版本」保持 `TLS 1.2`。

   ![编辑监听：443 + HTTPS，HTTP 端口 80 跳转](/tutorials/https-certificate-listener.zh-CN.webp)

3. 点「保存」。监听卡片的标签变成 `https://*:443` 和「HTTP :80 跳转」，站点行出现「证书剩余 N 天」。

   ![升级后的监听与站点证书](/tutorials/https-certificate-site.zh-CN.webp)

监听按访问的域名自动从「SSL 证书」里选证书，多张证书覆盖同一域名时用到期最晚的一张，站点里不需要单独指定。站点行显示「缺少证书」时，点旁边的「申请证书」会打开 SSL 证书页的新建表单并填好这些域名。

HTTP-01 验证在这种「HTTP 端口跳转」配置下照常工作：验证请求先于跳转被应答。

::: tip 想一张证书覆盖所有子域名
以后要加 `photos.example.com`、`jellyfin.example.com` 等一批子域名，可以申请一张 `*.example.com` 泛域名证书（只能用 DNS-01），新站点直接使用这张证书。步骤见[泛域名证书与动态域名，10 分钟配好](/docs/tutorials/wildcard-cert-ddns)。
:::

### 6. 把证书给其他程序使用（可选）

同一张证书还要给 NAS 自带的 Web 服务、另一台机器上的 nginx 或某个容器用时，在证书的「编辑」里配置，证书每次签发、续期或更新后自动执行：

- 「证书映射」的「写入目录」：把 `名称.crt`、`.key`、`.pem` 与 `_issuerCertificate.crt` 写进一个绝对路径目录，并执行「变化后执行」里的命令，例如 `docker exec nginx nginx -s reload`；
- 「自动部署」的「添加部署目标」，可以加多个：
  - 「复制到本机目录」：写入本机目录后执行「复制后执行」的命令；「文件名」可以填 `fullchain.pem` 与 `privkey.pem` 这样的固定名字（两个都填才生效）；
  - 「上传到 SSH 主机」：选 [Web 终端](/docs/modules/terminal) 里已有的 SSH 连接（需要先在 Web 终端里连接一次以记录主机密钥），上传到「远程目录」后执行「上传后执行」，例如 `systemctl reload nginx`；
  - 「重启本机容器」：证书变化后依次重启所选的 Docker 容器。

![自动部署：复制到本机目录并重启容器](/tutorials/https-certificate-deploy.zh-CN.webp)

命令里可以使用 `$certName`、`$certDir`、`$certFile`、`$keyFile`。Gately 运行在 Docker 中时，目录与命令都在容器内，目录需要挂载出来。部署失败时每小时重试一次并发出「证书：部署失败」事件，证书卡片上的「立即部署」可以马上重试并显示每个目标的结果。详见 [SSL 证书 · 自动部署](/docs/modules/ssl)。

### 7. 管理界面也用上 HTTPS（可选）

证书签好后，管理界面也可以走 HTTPS：

1. 再申请一张 `gately.example.com` 的证书（或者直接用泛域名证书），并在局域网 DNS 里把 `gately.example.com` 指向 Gately 所在设备；
2. 「设置 → 访问控制 → 管理界面监听」的「HTTPS 监听」填 `:16643`，点「保存设置」后按提示重启；
3. 用 `https://gately.example.com:16643` 访问管理界面。证书按访问域名从「SSL 证书」里选择，所以要用证书里的域名访问；确认无误后可以打开「强制 HTTPS」，让 HTTP 访问自动跳转。

管理界面通过 HTTPS 域名访问后，就可以按[安装后的第一步](/docs/tutorials/getting-started)注册 Passkey 了。

## 验证

```bash
# 证书：颁发者为 Let's Encrypt，主题备用名包含 nas.example.com，查看到期时间
echo | openssl s_client -connect nas.example.com:443 -servername nas.example.com 2>/dev/null \
  | openssl x509 -noout -issuer -subject -ext subjectAltName -enddate

# HTTP 跳转：返回 301/308，Location 为 https://nas.example.com/
curl -sI http://nas.example.com/ | grep -iE '^(HTTP|location)'

# HTTPS 访问后端：返回 NAS 的响应
curl -sI https://nas.example.com/ | head -n 1
```

只在局域网解析的域名，在局域网内的电脑上执行。界面上：证书卡片显示「剩余 N 天」，站点行显示「证书剩余 N 天」，浏览器地址栏显示锁形图标。

## 常见问题

**签发失败，日志写 `contact email has forbidden domain`。** 账户邮箱用了 `example.com` 这类不能使用的域名，换成真实邮箱后点「续签」。

**DNS-01 一直等待 TXT 记录生效，最后超时。** 主机有 IPv6 地址但 IPv6 不通时，查询权威 DNS 会超时，把「网络」改为「仅 IPv4」；国内网络也可以在「检查用 DNS」填 `223.5.5.5:53`。域名托管在别处、只把子域名委派给 Cloudflare 或阿里云时，在「主域名」里填委派的那一级，详见 [SSL 证书的 DNS-01 验证](/docs/modules/ssl#dns-01-验证)。

**Cloudflare 报认证错误（例如 `6003 Invalid request headers`）。** 令牌复制不完整或已失效，重新创建并完整粘贴；同时确认「账号邮箱」是空的，邮箱有内容时 Gately 按 Global API Key 方式认证。

**阿里云提示没有权限。** RAM 子用户没有云解析 DNS 的授权，给它加上 AliyunDNSFullAccess。

**HTTP-01 验证失败，提示连接超时或 404。** 证书机构从公网连不到你的 80 端口：运营商封了 80、路由器没转发、域名公网解析没有指向你的公网 IP，或者前面有 CDN。家宽环境改用 DNS-01。日志写「监听 80 端口失败」时，80 端口被其他程序占用，又不是 Gately 的 Web 服务，停掉那个程序或改用 DNS-01。

**Let's Encrypt 提示请求过多（`too many certificates` / `rateLimited`）。** 同一组域名一周内签发次数有上限。调试时先用「Let's Encrypt（测试环境）」，确认流程正常后再切回正式环境。

**浏览器仍然提示证书不受信任。** 检查访问的域名是否在证书的域名列表里，`*.example.com` 不覆盖 `example.com` 本身，也不覆盖 `a.b.example.com`；证书来源是「Let's Encrypt（测试环境）」时浏览器不信任。

**改成 HTTPS 后，后端应用里的跳转还是 http://。** 应用需要知道自己在 HTTPS 后面。保持站点勾选「添加 X-Forwarded-*」，并在应用里把外部访问地址改成 `https://nas.example.com`，常见应用的设置见[给 Jellyfin、Immich、Home Assistant、Vaultwarden 加 HTTPS 和登录](/docs/tutorials/https-and-login-for-apps)。

## 下一步

<TutorialVideo name="https-certificate" caption="演示：用 DNS 验证申请证书，并在监听上启用 HTTPS（约 1 分钟）" />

- [动态域名入门](/docs/tutorials/ddns-basics)：让域名跟着家里变化的公网 IP；
- [泛域名证书与动态域名，10 分钟配好](/docs/tutorials/wildcard-cert-ddns)：一张 `*.example.com` 覆盖所有子域名；
- [通知与告警](/docs/tutorials/notify-alerts)：证书即将到期、续期失败时推送到手机；
- [给站点加上访问控制](/docs/tutorials/site-access-control)：公网开放前先加上登录与 IP 限制；
- 参考文档：[SSL 证书](/docs/modules/ssl)、[Web 服务](/docs/modules/web)。
