# Tailscale / frp / cloudflared

三个模块以进程托管方式集成官方程序：自动重启、日志汇总到界面、配置在界面中完成。Gately 本体不内置这些程序，可以在页面中在线安装、更新与回滚。

## 在线安装、更新与回滚 {#install}

frp、Cloudflare 隧道、Tailscale 与 [File Browser](/docs/modules/tools#filebrowser) 页面下方都有「在线安装」卡片：

- **下载安装 / 更新到最新版**：从上游下载最新正式版，按上游记录的 SHA-256 校验，确认能在本机运行后安装到配置目录的 `bin/`，完成后自动重启使用该程序的进程。已经是最新版时不会重复下载；
- **检查更新**：只查询上游最新正式版，不下载；有新版本时卡片标题旁显示「新版本」。开启[自动检查更新](/docs/guide/install#update-check)时，Gately 每 12 小时为已安装的程序检查一次，发现新版本时发布 `program.update` 通知；
- **安装指定版本**：填写版本号（例如 frp 的 `v0.65.0`、cloudflared 的 `2025.8.0`、Tailscale 的 `1.90.2`），可用于降级；
- **下载代理**：每个程序可以单独设置，支持 http、https 与 socks5，开始安装时保存；留空时使用「设置」中的更新代理；
- **旧版本与回滚**：被更新或回滚替换下来的版本保存在 `bin/.versions/` 下，最多保留 3 个。新版本有问题时在列表中点「回滚」即可切回，当前版本同样会保留，之后还能再切回来；
- **卸载**：删除 `bin/` 下安装的程序与全部旧版本，模块配置保留。之后使用该程序的进程会重启，`PATH` 中有同名程序时改用它，没有时无法运行。

| 程序 | 来源 | 安装的文件 |
| --- | --- | --- |
| frp | GitHub fatedier/frp | `frpc`、`frps` |
| cloudflared | GitHub cloudflare/cloudflared | `cloudflared` |
| Tailscale | pkgs.tailscale.com 静态包，仅 Linux | `tailscale`、`tailscaled` |
| FileBrowser Quantum | GitHub gtsteffaniak/filebrowser | `filebrowser` |

Tailscale 在 macOS 与 Windows 上没有可直接运行的程序包，请使用官方安装程序，再在设置中填写路径。卡片会提示当前平台不支持的情况。

## 恢复默认配置 {#reset}

页头的「恢复默认配置」与卸载分开处理：停止该模块的全部进程，把设置（包括程序路径与下载代理）恢复为默认值，并删除 Gately 为它生成的文件，安装的程序保留。

| 模块 | 清除的内容 |
| --- | --- |
| frp | 全部 frpc / frps 实例、代理与程序设置，`frp/` 下生成的配置文件 |
| Cloudflare 隧道 | 全部隧道与程序设置 |
| Tailscale | 全部设置，默认状态目录中的登录状态；Gately 托管的 tailscaled 运行中时先退出登录。自定义的状态目录与系统已有的 tailscaled 不处理 |
| File Browser | 全部实例与程序设置，各实例的用户、分享链接与缓存；共享目录中的文件不受影响 |

旧配置保存在 [配置历史](/docs/guide/audit) 中，误操作时可以恢复。要彻底移除，先卸载程序，再恢复默认配置。接口为 `DELETE /api/{module}/install`（卸载）与 `POST /api/{module}/reset`（恢复默认配置）。

## 程序放置

按以下顺序查找可执行文件：

1. 模块设置中填写的路径；
2. 配置目录下的 `bin/` 子目录（在线安装的位置）；
3. `PATH`。

填写了路径时在线安装的版本不会生效，卡片会提示。也可以自行把二进制放进配置目录，Docker 部署时例如 `./gately-data/bin/frpc`。Tailscale 需要同时提供 `tailscale` 与 `tailscaled`；使用系统已有的 tailscaled 时，安装的 `tailscale` 命令最好与守护进程版本一致。

令牌与密钥以 0600 权限保存，cloudflared 的令牌通过环境变量传递，不会出现在进程列表中。

## Tailscale

- 托管 `tailscaled`（用户态网络，容器友好）或接入系统已有的守护进程；
- 认证密钥或登录链接，支持 Headscale；
- 子网路由与出口节点；
- 节点列表与 SOCKS5 代理；
- 固定 WireGuard UDP 端口，便于在防火墙放行直连；
- tailscaled 额外参数与环境变量。宿主机 iptables 是 nf_tables 后端时，在环境变量中加入 `TS_DEBUG_FIREWALL_MODE=nftables`，子网转发才会生效。

## frp

- **frpc 客户端**：可添加多个 frpc 实例，每个实例连接一台 frps，独立运行、单独启停与重启；代理归属某个实例，支持 tcp / udp / http / https / stcp / xtcp，增删改后只重启所属实例；
- **代理访问地址**：frpc 不知道 frps 的 HTTP / HTTPS 端口和子域名根域名，可在 frpc 实例的「访问地址」中填写（本机也有 frps 实例时可一键填入；frps 前面还有反向代理或 CDN 时填访客实际使用的协议和端口）。填写后 http / https 代理的域名（含子域名拼出的完整域名）可点击打开、可复制完整地址，未填写时照常显示文本。访问地址只用于生成链接，修改后 frpc 不重启。自定义域名保存时与 Web 服务一样统一为小写和 punycode，带端口、路径的写法会被拒绝；子域名只能填一级名称（如 `nas`），与 frps 的要求一致；
- **frps 服务端**：可添加多个 frps 实例（使用不同端口），每个实例有端口白名单、子域名、控制台；
- **从旧版本升级**：0.5.9 及更早版本的单个 frpc / frps 设置会自动迁移为名为 `frpc` / `frps` 的实例，原有代理归属该 frpc 实例，程序路径移到页面下方的「程序」卡片；
- **frps 监听地址**：「监听地址」决定 frpc 连接端口监听在哪，「代理监听地址」决定 HTTP / HTTPS 端口和 tcp / udp 代理的远程端口监听在哪，留空都监听所有地址。frps 放在 Gately 等反向代理之后时，把监听地址填 `127.0.0.1`，在 Web 服务中把一个站点反代到 `127.0.0.1:<绑定端口>`，frpc 的传输协议选 websocket 或 wss 并连接该站点；代理监听地址保持留空，tcp 代理的远程端口照常对外开放。导入 frps 配置时，原配置未写 `proxyBindAddr` 会沿用 `bindAddr`，与 frps 的行为一致；
- **frps 来源 IP 访问控制**：每个 frps 实例可设置白名单或黑名单，按来源 IP 决定 frpc 能否登录（连接绑定端口）。名单由手填的 IP / 网段和 Web 服务中的订阅 IP 列表组成，命中任意一项即视为在名单中；订阅列表由 Web 服务模块定时下载，刷新后立即生效，frps 不必重启，下载成功前按空列表处理。判断在 frps 校验令牌之前进行：Gately 在本机回环地址开一个检查端点，并在生成的 frps 配置末尾写入 `[[httpPlugins]]`（`ops = ["Login"]`），因此只对 frpc 登录生效，tcp / udp 代理的远程端口与 HTTP / HTTPS 端口照常对外开放。入侵防护的封禁优先于名单：被封禁的地址即使落在白名单网段内也无法登录，未设置访问控制的实例同样遵守封禁表，与是否开启防火墙同步无关。被拒绝的 frpc 会收到 `gately: client address ... is not in the allowlist` 之类的错误，实例卡片显示 Gately 启动以来的拒绝次数。名单在 frpc 登录时检查，作用于 frpc 连接 frps 的绑定端口：名单收紧或地址被封禁后，已登录的 frpc 保持连接，下次登录时才被拒绝，需要立即生效时重启该 frps 实例；stcp / xtcp 的 visitor 连接不经过登录检查，需要限制时配合[入侵防护](/docs/modules/guard)的防火墙同步。frps 放在反向代理之后（监听地址填 `127.0.0.1`）时，frps 看到的来源是代理的地址，请改在 Web 服务站点上做 IP 访问控制；放在协议分流或端口转发后面时，frps 看到的来源是本机地址，名单无法按访客判断，请在分流或转发规则上设置来源 IP 访问控制；
- **导入已有配置**：页头的「导入配置」支持旧版 `frpc.ini` / `frps.ini`（含 `[common]` 段）和 frp 0.52+ 的 `toml`，可选择文件或粘贴内容，并选择导入到已有实例或新建实例。先预览解析结果与被忽略的字段，再确认导入；导入到已有 frpc 实例时可选择追加（同名代理保持不变）或替换该实例的全部代理。导入到已有实例时沿用其启用状态，新建的实例默认停用；也可通过接口 `POST /api/frp/import` 调用。从 Lucky 迁移时，Lucky 中的每个 frpc / frps 实例都会导入为 Gately 的实例。

## Cloudflare 隧道

- 令牌隧道与 trycloudflare 临时隧道；
- 多隧道、自动重启；
- 展示公网地址。

### 路由规则 {#cloudflared-ingress}

令牌隧道的公网主机名与回源服务（路由规则）保存在 Cloudflare 上，运行中的 cloudflared 会自动拉取。在隧道设置中填写可选的 **API 令牌** 后，卡片上的「路由规则」可以直接编辑这些规则：

- 令牌需要「Cloudflare Tunnel：编辑」权限；要自动创建 DNS 记录，还需要对应域名的「DNS：编辑」权限。账号与隧道 ID 从隧道令牌中解析，无需另外填写。
- 每条规则包含主机名、可选的路径正则与服务（如 `http://localhost:8080`、`ssh://localhost:22`、`http_status:404`），按顺序匹配，命中第一条即转发；可设置「不校验回源证书」和回源 Host 头，其他回源设置原样保留。所有规则都未命中时使用兜底服务，默认 `http_status:404`。
- 保存后立即生效，无需重启隧道。若规则在此期间被 Cloudflare 控制台或其他地方修改，保存会提示并显示「重新加载」，避免覆盖别人的修改。HTTP 类服务不能带路径，末尾单独的 `/`（如 `http://nas:5000/`）保存时会自动去掉，否则 cloudflared 会拒绝整份配置。
- 隧道由本地配置文件管理时（界面会提示远程规则不生效），保存前需要再确认一次：写入远程配置后隧道改为远程管理，本地配置文件中的路由规则不再生效。
- 打开「同步 DNS」时，为每个主机名创建开启代理、指向 `<隧道ID>.cfargotunnel.com` 的 CNAME 记录；主机名已有其他记录，或已有指向隧道但未开启代理的 CNAME（这时流量不会进入隧道）时保持原样并在结果中列出。只在隧道所属账号下查找域名。删除规则时 DNS 记录保留，需要时在 Cloudflare 中删除。

## 如何选择

| 场景 | 推荐 |
| --- | --- |
| 在多台设备之间组成私有网络 | Tailscale |
| 自己有公网服务器，把内网服务映射出去 | frp |
| 没有公网 IP 也没有服务器，快速暴露 Web 服务 | Cloudflare 隧道 |
| 运营商给了可打洞的 NAT | [STUN 穿透](/docs/modules/stun) |
