---
time: 30 分钟
description: 把 Gately 作为 OIDC 身份提供方，让 Gitea、Grafana、OpenList 用共享用户登录：准备签发者域名、启用 OIDC、添加客户端，各应用的完整配置、与站点网页认证一起实现一次登录处处免登，以及常见报错的排查。
---

# 用 Gately 账号登录 Gitea、Grafana、OpenList（OIDC 单点登录）

家里或团队里跑着 Gitea、Grafana、OpenList，每个应用一套账号，家人或同事要记好几组密码，离职或换设备时还要逐个应用删账号。Gately 内置一个 OpenID Connect（OIDC）身份提供方：账号只在 Gately 的「认证用户」页维护一次，应用通过标准的 OIDC 协议跳到 Gately 登录，再拿到用户名、邮箱和分组，为每个人建立各自的账号与权限。做完本文，你会得到：

- 一个登录入口 `https://auth.example.com`，所有接入的应用都跳到这里登录；
- Gitea、Grafana、OpenList 首次登录自动建号，`admins` 分组的人自动成为管理员；
- 已经登录过 Gately 保护的站点后，打开这些应用直接进入，反过来也一样。

::: tip 先看看界面
在线演示 [demo.gately.cn](https://demo.gately.cn)（点「演示账号一键进入」，只读，每小时重置）的「Web → 认证用户」页就是本文操作的页面。
:::

## 适用场景与前提

- 应用支持 OIDC 或「通用 OAuth2」登录。Gitea、Grafana、OpenList、Nextcloud（装 OIDC 插件）、Jellyfin（装 SSO 插件）、Portainer、Outline、Vikunja、Immich 等都支持。没有账号体系的服务（例如路由器管理页、静态文档）改用站点的网页认证，见[给站点加上登录认证](/docs/tutorials/site-access-control)；
- Gately 已经有一个对外的 HTTPS 监听，例如 443。还没有时先按[添加第一个站点](/docs/tutorials/first-site)和[申请 HTTPS 证书](/docs/tutorials/https-certificate)完成；
- 有一张覆盖签发者域名的证书。推荐[泛域名证书](/docs/tutorials/wildcard-cert-ddns) `*.example.com`，签发者域名和应用域名都能用；
- 应用本身已经能通过 Gately 用 HTTPS 打开，例如 `https://git.example.com`、`https://grafana.example.com`、`https://pan.example.com`。

本文用到的示例值，照做时替换成你自己的：

| 项目 | 示例值 |
| --- | --- |
| 签发者域名 | `auth.example.com` |
| 单点登录域 | `example.com` |
| Gitea | `https://git.example.com` |
| Grafana | `https://grafana.example.com` |
| OpenList | `https://pan.example.com` |
| Gately 所在主机 | `192.168.1.2` |

## 结果是什么样

```text
               ① 打开应用，点「使用 Gately 登录」
浏览器 ───────────────────────────────▶ git.example.com（Gitea）
   │                                         │
   │ ② 跳到签发者登录（已登录则直接返回）       │ ④ 应用服务端直接请求：
   ▼                                         │    令牌、用户信息、公钥
auth.example.com（Gately 的 OIDC 路径）◀──────┘
   │
   │ ③ 带授权码回到应用的回调地址
   ▼
git.example.com/user/oauth2/gately/callback → 登录完成
```

两点决定了配置要怎么填：

- 浏览器要能打开 `auth.example.com`（第 ②、③ 步）；
- **应用的服务端**也要能访问 `auth.example.com`（第 ④ 步）。应用运行在 Docker 里时，容器内要能解析这个域名并连到 Gately 的端口，这是最常见的卡点，见下文「常见问题」。

## 步骤

### 1. 准备签发者域名

签发者域名是应用跳转登录的地址，建议单独用一个域名，例如 `auth.example.com`：

1. 在 DNS 里给 `auth.example.com` 加一条解析，指向 Gately 所在主机的公网地址；用了[动态域名](/docs/tutorials/ddns-basics)时，在任务的「解析记录」里加一条，或者直接用泛解析 `*.example.com`。只在内网用时，在路由器或内网 DNS 里把它解析到 `192.168.1.2`；
2. 打开「Web → SSL 证书」，确认有一张证书覆盖 `auth.example.com`（`*.example.com` 即可）。卡片上的「域名」一行能看到它覆盖的名字；
3. 确认有一个开启 HTTPS 的监听能收到这个域名的请求。默认监听所有地址的 443 监听就满足。

签发者域名**不需要建站点**。Gately 在所有监听上识别它的 OIDC 路径（`/.well-known/openid-configuration` 与 `/__gately_oidc/…`），这个域名的其他路径照常按站点处理。想把登录页限制在内网或某些国家，可以给这个域名建一个站点（类型随意，例如「固定文本」），站点上的 IP 黑白名单、订阅 IP 列表、国家过滤和限流同样作用于 OIDC 路径。

### 2. 给共享用户补齐邮箱、显示名和分组

OIDC 只接受「共享用户」。站点自己的用户和第三方登录身份用不了 OIDC。

1. 打开侧边栏「Web → 认证用户」；
2. 在「共享用户」卡片点「添加共享用户」，或在已有用户上点「编辑」；
3. 填写：
   - 「用户名」：例如 `alice`，应用里的用户名就是它（Gitea、Grafana、OpenList 都按下文配置读取 `preferred_username`）；
   - 「密码」：用一个足够长的随机密码，保存后以 bcrypt 哈希保存，之后无法再查看；
   - 「TOTP」：点「启用 TOTP」，用验证器 App 扫码，登录时多一步动态验证码，建议给管理员开；
   - 「邮箱」：`alice@example.com`。Gitea 自动注册必须有邮箱，Gately 会把它标记为已验证；
   - 「显示名」：`Alice`，留空时用用户名；
   - 「分组」：输入后回车添加，例如 `family`、`admins`。应用可以按分组授予管理员，Gately 也可以按分组限制谁能登录某个应用；
4. 点「确定」回到页面。

![编辑共享用户：邮箱与分组会提供给应用](/tutorials/oidc-sso-user.zh-CN.webp)

::: tip 分组怎么规划
一个够用的起点：所有家人或同事放进 `family`（或 `staff`），管理员再加一个 `admins`。下文 Gitea、Grafana 都用 `admins` 判断管理员。
:::

### 3. 设置单点登录域

还是在「共享用户」卡片，「单点登录域」填 `example.com`。

![单点登录域填 example.com](/tutorials/oidc-sso-domain.zh-CN.webp)

填了之后，共享用户在 `example.com` 及其任一子域名的站点登录一次，其他站点和签发者 `auth.example.com` 都认这次登录（父域 Cookie `gately_sso`，有效期 7 天）。签发者域名要落在单点登录域里面，「一次登录、处处免登」才成立。

同一张卡片上还有两个开关，按需打开：

- 「自助修改密码」：用户可以在 `https://auth.example.com/__gately_oidc/password` 自己改密码，改完后已签发的 OIDC 刷新令牌和 Access Token 立即作废；
- 「Passkey 登录」：用户在 `https://auth.example.com/__gately_oidc/passkey` 注册后，用指纹、面容或 PIN 登录。

### 4. 启用 OIDC 身份提供方

1. 在同一页面往下，找到「OIDC 身份提供方」卡片；
2. 打开「启用」；
3. 「签发者地址」填 `https://auth.example.com`。只写协议、域名和端口，结尾不要带 `/`；Gately 的 HTTPS 监听不在 443 时把端口写上，例如 `https://auth.example.com:8443`；
4. 填好后卡片显示「发现地址」，展开「需要手动填写端点时」能看到授权、令牌、用户信息、公钥、退出五个地址，每个都可以点右侧图标复制。

![启用 OIDC 并填写签发者地址](/tutorials/oidc-sso-provider.zh-CN.webp)

| 应用里的叫法 | 填什么 |
| --- | --- |
| Discovery URL、Auto Discovery URL、OpenID Configuration URL | `https://auth.example.com/.well-known/openid-configuration` |
| Issuer URL、签发者、Endpoint | `https://auth.example.com` |
| Authorization / Auth URL | `https://auth.example.com/__gately_oidc/authorize` |
| Token URL | `https://auth.example.com/__gately_oidc/token` |
| Userinfo / API URL | `https://auth.example.com/__gately_oidc/userinfo` |
| JWKS URL | `https://auth.example.com/__gately_oidc/jwks` |

签发者地址要与应用访问的地址一字不差。应用会核对发现文档里的 `issuer`，差一个端口、多一个斜杠都会报 issuer 不匹配。

首次保存时 Gately 自动生成 2048 位 RSA 签名密钥，保存在 Web 服务的配置文件里，随配置备份，界面和接口都不会显示它。

### 5. 添加客户端

每个应用对应一个客户端。先加 Gitea：

1. 在「客户端」一行点「添加客户端」；
2. 「名称」填 `Gitea`，登录页上会显示「验证身份后将继续登录 Gitea」；
3. 「Client ID」填 `gitea`，只能用字母、数字和 `- . _ ~`；
4. 「公共客户端」保持不勾选。Gitea、Grafana、OpenList 都是服务端应用，用 Client Secret 认证；
5. 「Client Secret」点「生成」，得到一串 43 位的随机密钥，**马上点输入框右侧的复制图标，存到密码管理器或直接填进应用**。保存后 Gately 只保留哈希，之后再也看不到原文；
6. 「回调地址」填 `https://git.example.com/user/oauth2/gately/callback`，每行一个；
7. 「允许的用户」填 `group:family` 和 `group:admins` 两行，表示这两个分组里的人能登录 Gitea。留空表示所有共享用户都能登录；
8. 点「确定」。

![添加 Gitea 客户端](/tutorials/oidc-sso-client.zh-CN.webp)

用同样的方法再加两个客户端：

| 名称 | Client ID | 回调地址（每行一个） | 允许的用户 |
| --- | --- | --- | --- |
| Grafana | `grafana` | `https://grafana.example.com/login/generic_oauth` | 留空 |
| OpenList | `openlist` | `https://pan.example.com/api/auth/sso_callback?method=sso_get_token`<br>`https://pan.example.com/api/auth/sso_callback?method=sso_get_ssoid` | `alice`、`bob` 各一行 |

每个客户端都要生成并复制各自的 Client Secret。三个客户端都加好后，列表里每项都有「新密钥待保存，请先复制」的提示：

![三个客户端，等待保存](/tutorials/oidc-sso-clients.zh-CN.webp)

确认密钥都复制好了，点页面底部的「保存设置」。页面上共享用户、单点登录域、OIDC 与站点勾选的修改一起保存，任何一项校验失败都不会保存，按提示改完再点一次。

::: tip 不确定回调地址时
先随便填一个地址保存，在应用里点一次登录。Gately 会停在「回调地址不匹配」的错误页，页面上显示应用实际发来的地址，原样复制到客户端的「回调地址」再保存即可。
:::

### 6. 配置 Gitea

在 Gitea 用管理员登录，打开「管理后台 → 认证源 → 添加认证源」：

| 字段 | 值 |
| --- | --- |
| 认证类型 | `OAuth2` |
| 认证名称 | `gately`（必须与回调地址里的 `/oauth2/gately/` 一致） |
| OAuth2 提供程序 | `OpenID Connect` |
| 客户端 ID（键） | `gitea` |
| 客户端密钥 | 第 5 步复制的 Client Secret |
| OpenID 连接自动发现 URL | `https://auth.example.com/.well-known/openid-configuration` |
| 附加授权范围 | `profile email groups` |
| 用于提供用户组名称的 Claim 声明名称 | `groups` |
| 管理员用户组的 Claim 声明值 | `admins` |

也可以在 Gitea 容器里用命令行添加：

```bash
gitea admin auth add-oauth --name gately --provider openidConnect \
  --key gitea --secret '<Client Secret>' \
  --auto-discover-url https://auth.example.com/.well-known/openid-configuration \
  --scopes "openid profile email groups" \
  --group-claim-name groups --admin-group admins
```

想让新用户首次登录时自动建号，在 Gitea 的 `app.ini` 里加上，然后重启 Gitea：

```ini
[oauth2_client]
ENABLE_AUTO_REGISTRATION = true
USERNAME = preferred_username
ACCOUNT_LINKING = auto
```

`ACCOUNT_LINKING = auto` 会把邮箱相同的已有 Gitea 账号与 Gately 用户关联起来，原来的仓库和权限都在。只想手动关联时去掉这一行，用户首次登录时 Gitea 会让他输入原账号密码完成绑定。

完成后 Gitea 登录页下方出现名为 gately 的第三方登录按钮。

### 7. 配置 Grafana

在 `grafana.ini`（Docker 部署时可以用同名的 `GF_` 环境变量）中：

```ini
[server]
root_url = https://grafana.example.com/

[auth.generic_oauth]
enabled = true
name = Gately
client_id = grafana
client_secret = <Client Secret>
scopes = openid profile email groups
auth_url = https://auth.example.com/__gately_oidc/authorize
token_url = https://auth.example.com/__gately_oidc/token
api_url = https://auth.example.com/__gately_oidc/userinfo
use_pkce = true
allow_sign_up = true
login_attribute_path = preferred_username
name_attribute_path = name
email_attribute_path = email
groups_attribute_path = groups
role_attribute_path = contains(groups[*], 'admins') && 'Admin' || 'Viewer'
```

- `root_url` 决定 Grafana 发出的回调地址，必须是 `https://grafana.example.com/`，否则回调地址会变成 `http://localhost:3000/login/generic_oauth`；
- `role_attribute_path` 让 `admins` 分组的人成为 Grafana 管理员，其他人为只读用户；
- 想让大家只用 Gately 登录，再加 `[auth] disable_login_form = true`，并保留一个本地管理员以备 Gately 不可用时登录。

重启 Grafana，登录页出现「Sign in with Gately」。

### 8. 配置 OpenList

用管理员登录 OpenList，打开「管理 → 设置 → 单点登录」：

| 设置项 | 值 |
| --- | --- |
| SSO 登录启用 | 开启 |
| SSO 登录平台 | `OIDC` |
| SSO 客户端 ID | `openlist` |
| SSO 客户端密钥 | 第 5 步复制的 Client Secret |
| SSO 端点名称 | `https://auth.example.com`（签发者地址，不带 `/.well-known/...`） |
| OIDC 用户名键 | `preferred_username`（默认值 `name` 取的是显示名） |
| SSO 自动注册 | 开启后首次登录自动创建 OpenList 用户 |

保存后 OpenList 登录页出现单点登录按钮。关闭自动注册时，用户先用 OpenList 原账号登录，在个人资料里绑定单点登录，这就是第二个回调地址 `method=sso_get_ssoid` 的用途。

### 9. 和站点网页认证一起用：一次登录，处处免登

很多服务没有自己的账号，例如 Prometheus、路由器管理页、文档站，这些用 Gately 站点的「网页认证」保护。让它们与 OIDC 应用共用一套账号：

1. 在「Web → Web 服务」编辑这类站点，切到「安全」页；
2. 「认证」勾选「网页认证（浏览器显示登录页）」；
3. 「共享用户」勾选「使用共享用户（支持 TOTP 与单点登录）」；
4. 保存。

![站点开启网页认证并使用共享用户](/tutorials/oidc-sso-site-auth.zh-CN.webp)

站点多时，在「认证用户」页的「使用共享用户的站点」卡片里按监听批量勾选更快，未开启认证的站点勾选后会同时开启网页认证。

这样配置后：

- 用户在任一站点的 Gately 登录页登录一次，之后打开 Gitea 点 gately 登录按钮，会直接回到 Gitea 并完成登录；
- 反过来，先在 Gitea 走过 OIDC 登录，再打开受保护的站点也直接进入；
- 两者都依赖第 3 步的单点登录域，签发者域名和站点域名都要在 `example.com` 之下。

::: warning 应用与网页认证二选一
已经接入 OIDC 的应用，它的站点只做反向代理，认证留给应用自己。叠加网页认证后，手机 App、Git 客户端（`git push`）、Grafana 的 API 都会被 Gately 登录页挡住；应用通过 OIDC 登录时已经由 Gately 验证过身份。
:::

## 验证

1. 发现文档能打开，`issuer` 与签发者地址一致：

   ```bash
   curl -s https://auth.example.com/.well-known/openid-configuration | jq '.issuer, .authorization_endpoint'
   # "https://auth.example.com"
   # "https://auth.example.com/__gately_oidc/authorize"
   ```

2. 应用服务端也能取到它。在应用容器里执行（容器没有 curl 时用 `wget -qO-`）：

   ```bash
   docker exec gitea wget -qO- https://auth.example.com/.well-known/openid-configuration | head -c 120
   ```

3. 用一个无痕窗口打开 `https://git.example.com`，点 gately 登录按钮，浏览器跳到 `https://auth.example.com/__gately_oidc/login`，页面写着「验证身份后将继续登录 Gitea」。输入 `alice` 的密码（和动态验证码），回到 Gitea，右上角是 `alice`，进「管理后台」能打开说明 `admins` 分组映射成功；
4. 在同一窗口打开 `https://grafana.example.com`，点「Sign in with Gately」，没有输入密码就进入了，右上角头像菜单显示 Alice 的角色为 Admin；
5. 用一个不在 Gitea 允许列表里的用户登录 Gitea，看到「没有使用该应用的权限」页面，说明允许列表生效；
6. 失败的令牌兑换会写进「Web → Web 服务」页底部的运行日志，例如「OIDC：客户端 gitea 认证失败，请检查 Client ID 与 Client Secret」。

## 常见问题

::: details 应用提示 issuer 不匹配（issuer mismatch / id token issued by a different provider）
签发者地址必须与应用配置里的地址完全一致，包括 `https`、端口，结尾不要有 `/`。Gately 的 HTTPS 监听在 8443 这样的非标准端口时，签发者地址要写成 `https://auth.example.com:8443`，应用里也写同样的地址。修改签发者地址后，Grafana、Gitea 等需要重启才会重新读取发现文档。
:::

::: details 登录后停在「回调地址不匹配」
页面下方显示了应用实际发来的回调地址。常见原因：

- Gitea 的认证名称与回调地址中间那段不一致（`/user/oauth2/gately/callback` 对应认证名称 `gately`，区分大小写）；
- Grafana 没设 `root_url`，发来的是 `http://localhost:3000/login/generic_oauth`；
- 应用在 Gately 后面却以为自己是 http，发来 `http://git.example.com/...`。Gitea 设 `ROOT_URL = https://git.example.com/`，并保持站点「添加 X-Forwarded-*」开启。

把页面上的地址原样加到客户端的「回调地址」里，保存后重试。
:::

::: details 应用报 invalid_client 或「客户端认证失败」
Client Secret 填错了，或者应用把 Gately 里勾选了「公共客户端」的客户端当成机密客户端用。Client Secret 只在保存前可见，遗失时在客户端编辑框里重新点「生成」、复制、保存，再更新到应用。令牌接口的认证失败与登录失败共用限流：同一来源连续失败 10 次后暂停 15 分钟，开启了自动封禁时还会封禁来源 IP，应用所在主机被封后在「安全 → 入侵防护」的封禁列表里解封。
:::

::: details 浏览器登录成功，应用报连接超时、证书错误或 502
这是第 ④ 步失败：应用的服务端连不上 `auth.example.com`。逐项检查：

- 容器内能否解析：`docker exec gitea nslookup auth.example.com`。内网 DNS 没有这个名字、或者解析到了公网地址而路由器不支持回流时，在 compose 里给应用加 `extra_hosts`：

  ```yaml
  services:
    gitea:
      extra_hosts:
        - "auth.example.com:192.168.1.2"
  ```

- 容器内能否连通：`docker exec gitea wget -qO- https://auth.example.com/.well-known/openid-configuration`；
- 证书是否受信：Gately 用的是 ACME 签发的公开证书时没有问题；用自签证书时，应用会拒绝连接，换成 ACME 证书。
:::

::: details ID Token 未生效、已过期（token used before issued / token is expired）
ID Token 带签发时间 `iat` 和过期时间 `exp`（1 小时），应用用自己的时钟校验。Gately 主机和应用主机的时间相差几分钟以上时，应用会认为令牌还没生效或已经过期。两边都开启 NTP 时间同步：

```bash
timedatectl status          # 查看「System clock synchronized: yes」
timedatectl set-ntp true
```

NAS、虚拟机长时间休眠后时钟容易漂移，恢复后先同步时间再排查别的。
:::

::: details 登录后显示「没有使用该应用的权限」
当前用户不在客户端的「允许的用户」里。按用户名匹配，用户改名后要同步修改；按 `group:分组名` 授权时，检查用户的「分组」是否拼写一致。页面上的「换个账号登录」会退出当前账号，用其他共享用户登录后继续原来的授权。
:::

::: details 改了用户的邮箱或分组，应用里没有变化
用户信息接口立即返回新值，ID Token 里的声明要等下次登录或刷新令牌时更新。Gitea、Grafana 在用户下次登录时同步分组和角色，让用户退出应用再登录一次。
:::

::: details 删除了用户，他在应用里还登录着
删除用户、改密码、改 TOTP、移出允许列表后，Gately 签发的授权码、刷新令牌和 Access Token 立即失效。应用自己建立的会话由应用管理，需要时在应用的管理后台让该用户下线或禁用账号。
:::

::: details 应用要求 form_post、隐式模式或设备码
Gately 只支持授权码模式（`response_type=code`），授权结果通过查询参数返回。大多数应用默认就是这种方式，在应用设置里找「Response mode」「Grant type」改成 code / query。
:::

## 下一步

- 完整的协议细节（scope、声明、令牌有效期、退出地址）：[OIDC 单点登录](/docs/modules/oidc)；
- 共享用户、TOTP、Passkey、自助修改密码：[Web 服务 · 共享用户](/docs/modules/web#shared-users)；
- 给没有账号体系的服务加登录：[给站点加上登录认证](/docs/tutorials/site-access-control)；
- 应用有手机 App 时登录怎么加：[给 Jellyfin、Immich、Home Assistant、Vaultwarden 加 HTTPS 和登录](/docs/tutorials/https-and-login-for-apps)；
- 登录页挡住暴力破解：[入侵防护与 WAF](/docs/tutorials/guard-waf)。
