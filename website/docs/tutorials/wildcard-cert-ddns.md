---
description: 用 Gately 十分钟配好动态域名与泛域名 HTTPS 证书：Cloudflare 与阿里云各一版，DNS-01 验证，无需开放 80 或 443 端口，到期自动续签。
---

# 泛域名证书与动态域名，10 分钟配好

家里或办公室的公网 IP 会变，又想给 `nas.example.com`、`photos.example.com` 这样的一批子域名统一配 HTTPS。做法是两件事：动态域名（DDNS）让域名始终指向当前 IP；一张 `*.example.com` 泛域名证书覆盖所有子域名，用 DNS-01 验证签发，不需要开放 80 或 443 端口。Cloudflare 与阿里云各写一版，选你的域名所在的服务商。

## 适用场景与前提

- 有一个自己的域名，解析托管在 Cloudflare 或阿里云（云解析 DNS）。其他服务商的操作相同，支持的服务商见[动态域名](/docs/modules/ddns#支持的服务商)；
- Gately 已部署并能访问外网（要访问 DNS 服务商、Let's Encrypt 与公网 IP 查询地址）；
- 一个真实可收信的邮箱，用于 ACME 账户。Let's Encrypt 会拒绝 `example.com` 这类保留域名的邮箱，日志里会写 `contact email has forbidden domain`；
- 本文用 `example.com` 做示例，请换成你的域名。

## 结果是什么样

- `example.com` 与 `*.example.com` 的 A 记录由 Gately 定期检查公网 IP，变化后自动更新；
- 一张覆盖 `example.com` 与 `*.example.com` 的证书由 Gately 签发并在到期前自动续签；
- 之后新增任何 `xxx.example.com` 站点，在 Web 服务里直接使用，不必再申请证书。

## 步骤

### 1. 创建 DNS 服务商的 API 密钥

DDNS 与证书验证都要通过服务商的 API 修改解析，建议为 Gately 单独创建只有解析读写权限的密钥，不要使用主账号密钥。

::: code-group

```text [Cloudflare]
1. 登录 Cloudflare 控制台，进入「我的资料 → API 令牌」，点「创建令牌」。
2. 选择「编辑区域 DNS」模板，或自定义以下两项权限：
   区域 · 区域 · 读取
   区域 · DNS · 编辑
3. 「区域资源」选「包括 · 特定区域 · example.com」。
4. 创建后复制令牌（只显示一次）。
```

```text [阿里云]
1. 登录阿里云控制台，进入「访问控制 RAM → 用户」，创建用户，
   勾选「OpenAPI 调用访问」，保存页面给出的 AccessKey ID 与 AccessKey Secret（Secret 只显示一次）。
2. 给这个用户授权，选系统策略 AliyunDNSFullAccess（云解析 DNS 完全权限），
   或自定义只含目标域名解析读写的策略。
```

:::

Gately 的任务和证书页面里服务商旁边的「获取凭据」链接会打开对应服务商的密钥管理页面。控制台的菜单名称会随服务商改版变化，以实际界面为准。

### 2. 添加动态域名任务

1. 打开侧边栏「网络 → 动态域名」，点「添加任务」。
2. 在「基本」页：
   - 任务名称：`家里的 NAS`；
   - DNS 服务商：Cloudflare 选「Cloudflare」，阿里云选「阿里云 DNS」；
   - 填写凭据：Cloudflare 把令牌填进「API Token / Global API Key」，「账号邮箱」留空（只有使用 Global API Key 时才填邮箱）；阿里云填「AccessKey ID」与「AccessKey Secret」；
   - Cloudflare 的「代理状态」建议填 `false`，让记录直接指向你的 IP，不经过 Cloudflare 的代理（橙色云朵）；
   - 其余保持默认（检查周期 300 秒）。

   ![动态域名任务的基本设置](/tutorials/wildcard-cert-ddns-task.zh-CN.webp)

3. 切到「解析记录」页，第一行「主机记录」填 `@`（根域名，也可以留空），「主域名」填 `example.com`，类型 `A`，记录值保持 `{ipv4Addr}`；点「IPv4 记录」再加一行，「主机记录」填 `*`，「主域名」`example.com`，同样 `{ipv4Addr}`。

   ![两条 A 记录：根域名与泛解析](/tutorials/wildcard-cert-ddns-records.zh-CN.webp)

4. 想让域名同时有 IPv6：切到「IP 来源」页打开「获取 IPv6」，回到「解析记录」页点「IPv6 记录」，类型 `AAAA`，记录值 `{ipv6Addr}`。
5. 点「保存」，在任务卡片上点「强制同步」。

「IP 来源」页默认使用「查询接口」获取 IPv4，列出的查询地址可以换成任意服务或自建服务；设备直接持有公网地址时可以改用「网卡」。

### 3. 申请泛域名证书

1. 打开侧边栏「Web → SSL 证书」，点「添加证书」。
2. 来源保持「ACME 自动申请」，填写：
   - 备注：`example.com`（也是映射文件的名字，可选）；
   - 域名：每行一个，`example.com` 与 `*.example.com`；泛域名证书不覆盖根域名本身，所以两行都要写；
   - 账户邮箱：你的真实邮箱；
   - 证书颁发机构：Let's Encrypt，验证方式：「DNS-01（支持通配符）」；
   - DNS 服务商与凭据：与上一步相同（Cloudflare 填令牌，阿里云填 AccessKey）；
   - 「主域名」留空，Gately 会自己找出验证记录所在的区域。

   ![证书设置：两个域名与 DNS-01](/tutorials/wildcard-cert-ddns-cert.zh-CN.webp)

3. 点「保存」。保存后立即在后台开始签发，页面下方的运行日志显示进度。Gately 通过服务商 API 添加 `_acme-challenge` TXT 记录，等它生效后验证并删除，一般 1 到 2 分钟完成。
4. 证书卡片显示「剩余 N 天」就签发成功。Let's Encrypt 证书有效期 90 天，默认在到期前 30 天自动续签，续签天数可以在页头的「续签设置」里改。

### 4. 给站点启用 HTTPS

1. 打开「Web → Web 服务」，添加监听：端口 `443`，保持 HTTPS 开启。
2. 在监听卡片点「添加站点」，域名填 `nas.example.com`，类型「反向代理」，后端地址填应用地址，保存。
3. 以后再加 `photos.example.com` 等站点，直接添加，证书按域名自动从「SSL 证书」里选，不需要额外设置。

路由器把外网 TCP 443 映射到这台机器，或者让[协议分流](/docs/tutorials/one-port-443)占用 443 后转给 Web 服务。

## 验证

```bash
# 解析：返回你家当前的公网 IP
dig +short nas.example.com
dig +short example.com

# 证书：主题备用名里有 *.example.com，颁发者是 Let's Encrypt
echo | openssl s_client -connect nas.example.com:443 -servername nas.example.com 2>/dev/null \
  | openssl x509 -noout -issuer -subject -ext subjectAltName -enddate
```

在 DDNS 任务卡片上，记录状态显示「已创建」「已更新」或「已是最新」，说明同步成功。证书卡片的有效期显示新的到期日。

## 常见问题

**同步失败，日志里是 Cloudflare 的认证类错误（例如 `6003 Invalid request headers`）。** 令牌填错、复制不完整或已失效，重新创建令牌并粘贴完整内容。同时确认「账号邮箱」是空的，邮箱有内容时 Gately 按 Global API Key 方式认证。

**同步失败：Cloudflare 账号下未找到域名。** 令牌没有包含该域名的「区域 · 区域 · 读取」权限，或「主域名」写成了子域名。

**阿里云同步失败，提示没有权限。** 子账号缺少云解析 DNS 的授权，按步骤 1 给用户授权 AliyunDNSFullAccess。

**证书签发失败，日志写 `contact email has forbidden domain`。** 账户邮箱用了 `example.com` 等不能使用的域名，改成真实邮箱后保存。

**证书签发卡在等待 TXT 记录生效，最终超时。** 主机有 IPv6 地址但 IPv6 实际不通时，查询权威 DNS 会超时，把「网络」选为「仅 IPv4」；国内网络也可以在「检查用 DNS」填 `223.5.5.5:53`。域名托管在别处、只把子域名委派给 Cloudflare 或阿里云时，在「主域名」里填写委派的那一级，详见 [SSL 证书的 DNS-01 验证](/docs/modules/ssl#dns-01-验证)。

**证书只覆盖子域名，访问根域名提示证书不匹配。** 域名里漏了 `example.com` 那一行。

**Cloudflare 的橙色云朵打开了。** 开启代理后，访客连到的是 Cloudflare 而不是你的 IP，非 HTTP 端口不通，且上传大小受限。个人 NAS 通常把「代理状态」设为 `false`。

**IP 变了但解析迟迟没更新。** 默认每 300 秒检查一次，可以在任务里缩短检查周期，或点「强制同步」。

## 相关文档

- [动态域名](/docs/modules/ddns)：服务商、记录值变量、批量添加记录；
- [SSL 证书](/docs/modules/ssl)：HTTP-01、证书映射与自动部署到其他主机；
- [Web 服务](/docs/modules/web)；
- [没有公网 IPv4 怎么从外面访问 NAS](/docs/tutorials/remote-access-without-public-ip)。
