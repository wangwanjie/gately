---
description: 给 Jellyfin、Immich、Home Assistant、Vaultwarden 加 HTTPS 和登录：WebSocket、大文件上传与 WAF 请求体上限、信任代理设置，以及网页认证挡住手机 App 时的几种办法。
---

# 给 Jellyfin、Immich、Home Assistant、Vaultwarden 加 HTTPS 和登录

这四个应用的共同点是：既要在浏览器里用，又有手机或电视 App，还都用到 WebSocket。用 Gately 做反向代理加 HTTPS 很简单，难点在两处：应用要信任代理才能看到访客真实 IP，以及「登录」不能把 App 挡在外面。本文每个应用一小节，最后集中讲登录。

## 适用场景与前提

- 应用已经在内网跑起来，知道它的地址和端口；
- 有域名和覆盖它的证书，推荐[泛域名证书](/docs/tutorials/wildcard-cert-ddns)；
- 外网能访问到 Gately 的 443，见[没有公网 IPv4 怎么从外面访问 NAS](/docs/tutorials/remote-access-without-public-ip)或[一个 443 端口跑所有服务](/docs/tutorials/one-port-443)。

## 通用步骤

四个应用的 Gately 端配置相同：

1. 打开「Web → Web 服务」，没有监听时先「添加监听」，端口 `443`，保持 HTTPS。
2. 在监听卡片点「添加站点」：「域名」填应用的域名，「类型」选「反向代理」，「后端地址」填应用的内网地址（各应用的默认端口见下）。
3. 「选项」里保持「添加 X-Forwarded-*」勾选（默认开启），Gately 会把访客真实 IP、原始协议和域名写进请求头。
4. 点「保存」。

要点：

- **WebSocket 自动支持**，不需要像 nginx 那样写 `Upgrade` 与 `Connection` 头；
- **Gately 不限制上传大小**，也不会先把整个请求体缓存到磁盘。能卡住大文件的是 WAF 的请求体检查，见 Immich 一节；
- **超时**：后端「等待响应头」默认不限制，「访客超时」只在一次读写卡住时计时，数据持续传输的大文件、长连接不受影响，见 [Web 服务 · 后端超时](/docs/modules/web#backend-timeouts)。

| 应用与默认后端地址 | 要在应用里做的事 |
| --- | --- |
| Jellyfin<br>`http://192.168.1.20:8096` | 把 Gately 的地址加进「已知代理」 |
| Immich<br>`http://192.168.1.21:2283` | 设置 `IMMICH_TRUSTED_PROXIES`，WAF 放宽大文件 |
| Home Assistant<br>`http://192.168.1.22:8123` | 信任 `X-Forwarded-For` 并填可信代理，否则返回 400 |
| Vaultwarden<br>`http://192.168.1.23:80` | 设置 `DOMAIN` 为 https 地址 |

地址里的 IP 只是示例。

## Jellyfin

**后端**：`http://<Jellyfin 地址>:8096`，域名如 `jellyfin.example.com`。

**让 Jellyfin 看到真实 IP。** 在 Jellyfin 管理页的「控制台 → 网络」里，把 Gately 访问 Jellyfin 时使用的地址填进「已知代理」（支持 IP、CIDR），保存后重启 Jellyfin。Jellyfin 10.10.7 起只信任来自已知代理的 `X-Forwarded-For`，不填的话所有访客在 Jellyfin 里都显示成代理的地址。Gately 与 Jellyfin 在同一台机器、Docker 使用 host 网络时填 `127.0.0.1`；Jellyfin 在 bridge 网络的容器里时，填它看到的来源地址（通常是 Docker 网桥地址，如 `172.17.0.1`）。

**WebSocket。** Jellyfin 的远程控制、同播（SyncPlay）依赖 WebSocket，Gately 自动支持。

**手机与电视 App。** 它们用 Jellyfin 自己的账号登录。不要在这个站点开网页认证或基本认证，否则 App 连不上。需要额外保护时用 IP 访问控制、国家过滤（见下文「登录怎么加」）。

## Immich

**后端**：`http://<Immich 地址>:2283`。Immich 必须在域名的根路径提供服务，不能放在 `/immich` 这样的子路径下。

**让 Immich 看到真实 IP。** 给 `immich-server` 容器设置环境变量 `IMMICH_TRUSTED_PROXIES`，值填 Gately 的地址，多个用逗号分隔，然后重建容器（`docker compose up -d`，仅重启不会读取新的环境变量）。

**大文件与视频上传。** Immich 备份照片与视频会上传很大的文件。Gately 本身不限制，需要留意两处：

1. 站点没有挂 WAF 时，不用处理；
2. 站点挂了 WAF 时，WAF 默认只检查不超过 12.5 MB 的请求体，更大的请求会得到 413。给 Immich 单独建一个 WAF 实例（「安全 → WAF 防火墙」→「添加实例」），在「请求体检查」里把「超过上限时」设为「只检查前面部分」，必要时调大「检查上限」，再把这个实例选为站点的「WAF 实例」。也可以把上传接口 `/api/assets` 加进「跳过请求体的路径」，请求头、URL 与参数仍会照常检查。先用「仅检测」观察日志，确认没有误报再切换为「拦截」。

**超时。** 超大视频上传需要的时间长，Gately 的默认设置不会中断持续传输的上传。如果你在站点「流量」页给「等待响应头」设置了超时，调到 600 秒或更大。

**手机 App。** App 用服务器地址加 Immich 账号登录，不要在这个站点开网页认证。

## Home Assistant

**后端**：`http://<HA 地址>:8123`（以你的 Home Assistant 当前监听的端口为准）。

**必须信任代理，否则返回 400。** 反向代理会带上 `X-Forwarded-For`，Home Assistant 默认拒绝来自非可信代理的这类请求，网页只显示 `400: Bad Request`。

- Home Assistant 2026.8 及更新版本：「设置 → 系统 → 网络」的「HTTP 服务器」里打开「信任 X-Forwarded-For」，并在「可信代理」里添加 Gately 的地址或网段；
- 更早的版本：在 `configuration.yaml` 里设置后重启：

  ```yaml
  http:
    use_x_forwarded_for: true
    trusted_proxies:
      - 192.168.1.0/24   # Gately 所在的地址或网段，写网络地址
  ```

**WebSocket。** Home Assistant 前端与 App 通过 `/api/websocket` 通信，Gately 自动支持。

**登录失败自动封禁。** Home Assistant 把登录失败记在日志里，信任代理后记的是访客真实 IP。在「入侵防护」点「从模板添加」选「Home Assistant」，它读取名为 `homeassistant` 的 Docker 容器日志（需要在 Gately 的 Docker 模块里连接 Docker，并开启 Home Assistant 的 http 日志），同一 IP 反复失败后封禁。容器名不同时在规则里改目标。

**手机 App。** Companion App 用自己的令牌与 Home Assistant 通信，不要在这个站点开网页认证或基本认证。

## Vaultwarden

**后端**：`http://<Vaultwarden 地址>:80`（容器映射到宿主机端口时填映射后的端口）。

**设置 `DOMAIN`。** 给 Vaultwarden 设置环境变量 `DOMAIN=https://vault.example.com`，附件、邮件链接与 WebAuthn 都依赖它；Vaultwarden 要知道自己对外是 https。

**真实 IP。** Vaultwarden 默认从 `X-Real-IP` 读取访客地址，Gately 开着「添加 X-Forwarded-*」时会写入这个头，无需额外设置。

**WebSocket。** Vaultwarden 的实时同步与主服务在同一个端口上，由 Gately 自动支持，不需要单独转发旧版本使用的 3012 端口。

**保护管理页。** Vaultwarden 的网页保险库与所有客户端走 API，管理页 `/admin` 另有一层。给 `/admin` 单独加 Gately 的登录，其余路径保持公开，浏览器扩展与手机 App 不受影响：

1. 在站点编辑的「安全」页，「认证」勾选「网页认证」，在「用户」里添加 `用户名:密码`（或勾选「使用共享用户」）；
2. 勾选「认证范围」下的「仅对下方标记为「必须认证」的路径启用认证，其余路径公开」；
3. 「路径规则」点「添加规则」，前缀填 `/admin`，「认证」选「必须认证」；
4. 保存。

![只给 /admin 加登录的设置](/tutorials/https-and-login-for-apps-path-rule.zh-CN.webp)

访问 `/admin` 先出现 Gately 的登录页，通过后才到 Vaultwarden 自己的管理令牌页。其他路径不受影响。

**登录失败自动封禁。** 「入侵防护」→「从模板添加」→「Vaultwarden」，读取名为 `vaultwarden` 的容器日志中的密码错误与两步验证失败，按真实 IP 计数封禁。

## 登录怎么加：以及网页认证挡住 App 怎么办

网页认证（Gately 的登录页）适合只在浏览器里用的服务。上面四个应用都有各自的 App，这些 App 不会渲染 Gately 的登录页；基本认证需要占用 `Authorization` 请求头，多数 App 也没有弹窗。所以不要在整个站点上开这两种认证。可以选的办法：

| 办法 | 做法与对 App 的影响 |
| --- | --- |
| 只用应用自己的账号 | 应用本身有登录、双因素，再加 HTTPS、WAF、入侵防护。App 不受影响，最省事 |
| 按路径放行 | 站点开启网页认证后勾选「认证范围」，只给管理页等路径加「必须认证」（Vaultwarden 一节的做法）。App 走的 API 路径保持公开 |
| IP 白名单、国家过滤 | 站点「安全」页的「IP 访问控制」选「白名单」，或「国家过滤」选「仅允许」，并把「拒绝方式」设为「断开连接」。对 App 透明；手机在移动网络下 IP 会变，白名单更适合固定出口 |
| 登录后放行 | 一个入口站点用网页认证登录，成功后把访客 IP 加入[端口敲门](/docs/modules/knock)规则的放行列表，各应用站点开「敲门准入」。登录一次后，同一出口 IP 的 App 都能用，期限默认 6 小时 |
| 共享用户与 Passkey | 在「认证用户」页管理账号，开启 TOTP 与 Passkey 登录，网页端用指纹或面容登录。只作用于浏览器里的登录页 |
| OIDC 单点登录 | 应用自己支持 OIDC / OAuth 时，Gately 作为身份提供方，见 [OIDC 单点登录](/docs/modules/oidc)。App 能否使用取决于应用 |

「登录后放行」与「敲门准入」的细节、限制（内网与运营商级 NAT 地址不会被放行等）见 [Web 服务 · 敲门准入与登录后放行](/docs/modules/web#knock)。

## 验证

```bash
# 证书有效、HTTP/2 协商成功
curl -I https://jellyfin.example.com/

# Home Assistant：返回应用页面而不是 400
curl -I https://ha.example.com/
```

- 在浏览器里登录应用，打开开发者工具确认 WebSocket 连接（状态 101）；
- 应用的访问日志，或 Gately 站点卡片上的「连接」里看到的是访客真实 IP，不是 Gately 的地址；
- Vaultwarden：`/` 与 `/api/…` 直接可用，`/admin` 先出现登录页；
- 用手机 App 登录一次并同步。

## 常见问题

**浏览器显示 400 或 502。** Home Assistant 没有信任代理会返回 400。后端地址写错、后端不可达（Docker 部署用 `--network host` 或可路由的地址）会返回 502。

**应用里所有访客都是同一个 IP。** 应用没有信任 Gately 的地址，按各节设置「已知代理」「可信代理」或 `IMMICH_TRUSTED_PROXIES`。

**Immich 上传到一半失败，返回 413。** WAF 的请求体检查上限，按 Immich 一节调整。

**登录后循环跳转或链接变成 http。** 应用不知道自己在 https 后面，保持「添加 X-Forwarded-*」开启；Vaultwarden 检查 `DOMAIN`。

**WebSocket 连不上。** 在 Gately 前面还有别的反向代理或 CDN 时，确认它们也放行 WebSocket。

## 相关文档

- [Web 服务](/docs/modules/web)：认证、访问控制、路径规则、WAF 与超时；
- [WAF 防火墙](/docs/modules/waf)：请求体检查与上传大文件的站点；
- [入侵防护](/docs/modules/guard)：从日志识别暴力破解；
- [端口敲门](/docs/modules/knock)。
