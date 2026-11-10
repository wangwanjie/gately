# 文件与运维工具

## WebDAV

在侧边栏「文件服务」分组的「WebDAV 服务」中配置。

| 字段 | 默认值 | 说明 |
| --- | --- | --- |
| `enable` | 关闭 | 启用服务，启用时至少需要一个用户 |
| `listenIp` | 空 | 监听地址，留空监听所有地址 |
| `port` | 18800 | 监听端口 |
| `tls` | 关闭 | 启用 HTTPS，证书由 [SSL 证书](/docs/modules/ssl) 模块按 SNI 提供 |
| `users` | 无 | 多用户，每个用户包含 `name`、`password`、`root`（绝对路径目录）与 `readOnly`，或使用 `mounts` 挂载多个目录 |
| `sharedUsers` | 关闭 | 允许[共享用户](#shared-users)登录本服务 |

每个用户可以映射到一个目录，也可以挂载多个目录：

- **单个目录**：填写 `root` 与 `readOnly`，登录后根目录就是该目录。
- **多个目录**：在界面中点「挂载多个目录」，或在 `mounts` 中填写多项，每项包含 `name`（显示名称，不能含 `/`，同一用户内不重复）、`path`（绝对路径）与 `readOnly`。登录后根目录按名称列出各目录，根目录本身不能写入，文件也不能在不同目录之间移动。

## FTP

在「文件服务」分组的「FTP 服务」中配置，支持 FTP 与 FTPS。

| 字段 | 默认值 | 说明 |
| --- | --- | --- |
| `enable` | 关闭 | 启用服务，启用时至少需要一个用户 |
| `listenIp` | 空 | 监听地址，留空监听所有地址 |
| `port` | 2121 | 监听端口 |
| `passiveStart` / `passiveEnd` | 0 | 被动模式数据端口范围，均为 0 时由系统随机分配；填写时需满足 1 ≤ 起始 ≤ 结束 ≤ 65535，并在防火墙与端口映射中放行该范围 |
| `publicHost` | 空 | 被动模式对外通告的 IP，位于 NAT 之后时填写公网或转发后的 IP，仅支持 IP 地址，留空自动取连接地址 |
| `tlsMode` | 0 | 0 为明文与 FTPS 均可，1 为必须使用显式 FTPS（`AUTH TLS`），2 为隐式 FTPS（连接即加密）；非 0 时证书由 [SSL 证书](/docs/modules/ssl) 模块按 SNI 提供 |
| `idleTimeout` | 900 | 连接空闲多少秒后断开 |
| `connectionTimeout` | 30 | 建立数据连接的超时秒数 |
| `disableActiveMode` | 关闭 | 仅使用被动模式，更适合 NAT 环境 |
| `users` | 无 | 多用户，字段同 WebDAV：`name`、`password`、`root`、`readOnly` 或 `mounts`，用户名不可重复 |
| `sharedUsers` | 关闭 | 允许[共享用户](#shared-users)登录本服务 |

## SMB

在「文件服务」分组的「SMB 服务」中配置，把本机目录共享给 Windows 资源管理器、macOS 访达、Linux 以及手机、电视上的 SMB 客户端。协议实现内置于 Gately（基于 [gosamba](https://github.com/ahmetozer/gosamba)），支持 SMB 2.0.2 ~ 3.1.1、NTLMv2 认证与签名，可要求 SMB 3 加密，不支持 SMB 1。仅 Linux 与 macOS 上可用。

| 字段 | 默认值 | 说明 |
| --- | --- | --- |
| `enable` | 关闭 | 启用服务，启用时至少需要一个用户 |
| `listenIp` | 空 | 监听地址，留空监听所有地址 |
| `port` | 445 | 监听端口。Windows 只能连接 445；飞牛、群晖等 NAS 自带的 Samba 已占用 445 时需改用其他端口，macOS、Linux 与多数播放器可以在地址中指定端口 |
| `requireEncryption` | 关闭 | 要求所有连接使用 SMB 3 加密，只支持 SMB 2 的旧设备与部分电视将无法连接 |
| `mdns` | 关闭 | 通过 Bonjour 广播服务，macOS 访达的「网络」中可以直接看到；Docker 部署需使用 host 网络才能广播 |
| `users` | 无 | 多用户，字段同 WebDAV：`name`、`password`、`root`、`readOnly` 或 `mounts`，用户名不区分大小写且不可重复 |
| `sharedUsers` | 关闭 | 允许[共享用户](#shared-users)登录本服务 |

共享按用户划分，用户登录后只能看到并连接自己的共享：

- **单个目录**：共享名为用户名，例如用户 `alice` 的地址为 `\\192.168.1.2\alice`。
- **多个目录**：每个目录是一个共享，`name` 即共享名（同一用户内不区分大小写、不重复），可分别设为只读。不同用户可以有同名共享，各自指向自己的目录。

用户名与共享名不能包含 `" / \ [ ] : ; | = , + * ? < >`，也不能以 `.` 开头或结尾。NTLM 认证需要明文密码，密码以 bcrypt 哈希保存的用户需要重新设置密码。文件以 Gately 进程的身份读写。

页面下方的「连接地址」按已保存的配置列出每个共享的地址：

- Windows：资源管理器地址栏输入 `\\主机\共享名`，或执行 `net use Z: \\主机\共享名 /user:用户名`
- macOS：访达「前往 → 连接服务器」，输入 `smb://主机:端口/共享名`（445 端口可省略）
- Linux：`mount -t cifs //主机/共享名 /mnt -o username=用户名,port=端口`

排查客户端连不上或认证失败时，可以设置环境变量 `GATELY_SMB_DEBUG=1` 后重启 Gately，协议调试日志会写入运行日志；排查完成后去掉该变量。

## 共享用户 {#shared-users}

WebDAV、FTP 与 SMB 共用一份「共享用户」，字段与各服务的本地用户相同（用户名、密码、`root` 与 `readOnly` 或 `mounts`），保存在配置目录的 `fileusers.json`。共享用户在侧边栏「文件服务」分组的「文件共享用户」页面统一维护，保存后三个服务同时生效；各服务原有的本地用户不受影响。页面上方列出各服务是否允许共享用户登录，以及与本地用户同名或无法用于 SMB 的共享用户。

- 每个服务单独决定是否接受共享用户：在服务页面「共享用户」卡片中打开「允许共享用户登录」（字段 `sharedUsers`）并保存后，共享用户即可登录该服务；卡片同时列出已保存的共享用户，点「管理共享用户」进入「文件共享用户」页面；
- 与本地用户同名时只按本地用户校验（SMB 不区分大小写）；
- 共享用户名不区分大小写不可重复，也不能包含冒号；
- 修改共享用户立即生效：WebDAV 在下一个请求按新设置校验，FTP 与 SMB 会重启服务，已连接的客户端需要重新连接；
- 打开开关且至少有一个共享用户时，没有本地用户也可以启用服务；
- 只靠共享用户启用的服务，修改共享用户后必须仍有可登录的共享用户（SMB 要求能用于 SMB），否则拒绝保存；
- `fileusers.json` 被手工改坏（如密码为空、目录不是绝对路径）时，共享用户都不能登录，页面上会提示，修改后保存即可恢复；
- SMB 只接受明文密码，用户名与共享名还要满足 SMB 的字符限制，不满足的共享用户会在「文件共享用户」页面、SMB 页面与运行日志中提示并跳过，WebDAV 与 FTP 仍可使用。

接口为 `GET` / `PUT /api/fileusers`，API 令牌需要 `fileusers:read` / `fileusers:write` 权限，读取结果中的 `services` 列出各服务的使用情况。原来各服务下的 `GET` / `PUT /api/<服务>/shared-users` 已弃用，仍可使用并读写同一份列表：读取沿用所在服务的读权限，修改需要同时具备 `webdav:write`、`ftp:write` 与 `smb:write`。

**自助修改密码**：在「文件共享用户」页面打开「自助修改密码」并保存后，共享用户可以在任一开启网页认证的 Web 服务站点访问 `https://站点域名/__gately_auth/password`，输入用户名、当前密码和新密码（8～128 个字符）修改自己的密码，WebDAV、FTP、SMB 随即使用新密码（FTP 与 SMB 会重启服务，已连接的客户端需要重新连接）。原密码为明文的仍按明文保存，为 bcrypt 哈希的按哈希保存；当前密码错误与网页登录共用限流与自动封禁。开关保存在 `fileusers.json` 的 `selfService` 字段，接口字段同名。网页登录的共享用户见 [Web 服务的自助修改密码](/docs/modules/web#self-service-password)。

## File Browser {#filebrowser}

在「文件服务」分组的「File Browser」中配置。Gately 托管 [FileBrowser Quantum](https://github.com/gtsteffaniak/filebrowser)：一个网页文件管理器，可以在线预览图片、视频与文档、上传下载、生成分享链接，并有自己的多用户与权限管理。原版 filebrowser/filebrowser 已于 2026 年归档、不再修复安全问题，因此托管的是仍在维护的 Quantum 分支。

程序不内置在 Gately 中。页面下方的「在线安装」卡片从 GitHub 下载 FileBrowser Quantum 到配置目录的 `bin/filebrowser`，支持更新、安装指定版本、单独设置下载代理、回滚与卸载，更新或回滚后已启用的实例自动重启，详见 [在线安装、更新与回滚](/docs/modules/tunnels#install)。页头的「恢复默认配置」会删除全部实例及其数据，见 [恢复默认配置](/docs/modules/tunnels#reset)。也可以自行放置程序并填写路径。程序按以下顺序查找：填写的路径、配置目录下的 `bin/`、`PATH`；找到的程序不是 Quantum（例如 `PATH` 中的原版）时页面会提示。

每个实例是一个独立的 File Browser 进程：

| 字段 | 默认值 | 说明 |
| --- | --- | --- |
| `listenIp` | 空 | 监听地址，留空监听全部 IPv4 地址；只通过 Web 服务反向代理访问时可填 `127.0.0.1` |
| `port` | 8080 | 监听端口 |
| `baseUrl` | `/` | 访问路径，在 Web 服务中按路径反向代理（例如 `/files`）时填写相同的路径 |
| `accessUrls` | 空 | 访问地址，可填多个，例如经 Web 服务反向代理的 `https://files.example.com/`，显示在实例卡片上；留空时按监听地址和端口推测。第一个地址的协议、主机与端口写入 File Browser 的 `http.externalUrl`，分享链接使用该地址 |
| `sources` | 无 | 共享目录，每项包含 `path`（绝对路径）、`name`（显示名称，留空取目录名）与 `readOnly`；新用户默认可以访问全部目录 |
| `authMethod` | `password` | 登录方式：`password` 账号密码，`proxy` 信任反向代理传入的用户名请求头，`noauth` 免登录 |
| `proxyHeader` | `Remote-User` | `proxy` 方式读取用户名的请求头，不存在的用户自动创建 |
| `adminUser` | `admin` | 管理员用户名 |
| `adminPassword` | 无 | 管理员密码，至少 8 位，`password` 方式必填 |
| `trustProxyHeaders` | 关闭 | 信任 `X-Forwarded-*` 与 `X-Real-IP`，位于反向代理之后时开启 |
| `disableWebDav` | 关闭 | 关闭 File Browser 自带的 WebDAV 接口 |
| `locale` | 空 | 新用户的默认界面语言，如 `zh-cn`，留空为英文 |
| `tokenHours` | 2 | 登录有效期（小时） |

- 管理员密码在每次启动时同步到 File Browser，以 Gately 中的设置为准；其他用户、分享链接与个人设置在 File Browser 中管理，保存在实例自己的数据库里。
- 配置文件、数据库与缓存保存在配置目录的 `filebrowser/<实例 key>/` 下，配置文件权限 0600；管理员密码通过环境变量传给进程，不写入文件。删除实例会一并删除这个目录，共享目录中的文件保持原样。
- 共享目录不存在时实例不会启动，卡片上显示原因。
- 对外访问推荐在 [Web 服务](/docs/modules/web) 中添加反向代理站点指向 `http://127.0.0.1:端口`，用 HTTPS 与访问控制保护，并开启「信任代理头」。`proxy` 方式要配合 Web 服务的转发认证把用户名放进请求头，File Browser 的端口不能对外开放，否则可以伪造请求头登录。
- 文件以 Gately 进程的身份读写。

## 存储挂载

在「文件服务」分组的「存储挂载」中添加挂载点，在页面内在线管理文件。

| 字段 | 说明 |
| --- | --- |
| `name`、`remark`、`enable` | 名称、备注与启用开关 |
| `type` | `local`（本地目录）、`webdav`（远程 WebDAV）或 `smb`（SMB 共享） |
| `path` | 本地目录，需为已存在的绝对路径（`local` 使用） |
| `url`、`username`、`password` | 远程地址与账号，密码在界面中脱敏（`webdav`、`smb` 使用）。WebDAV 地址以 `http://` 或 `https://` 开头；SMB 地址形如 `smb://主机[:端口]/共享名[/子目录]`，支持 SMB2/3，用户名留空时以来宾身份访问 |
| `skipVerify` | 跳过 HTTPS 证书校验（`webdav` 使用） |
| `domain` | SMB 认证所用的域或工作组，通常留空（`smb` 使用） |
| `readOnly` | 只读挂载，文件管理拒绝一切写操作 |

在线文件管理支持浏览目录、下载文件、流式上传（不把整个文件载入内存）、新建目录、重命名和删除。挂载点可作为备份任务的目标位置，备份文件写入挂载点中指定的目录，并按保留策略清理旧文件。

## 网络唤醒

在「网络唤醒」中管理设备，向设备发送魔术包（6 字节 `0xFF` 后接 16 次 MAC）。

| 字段 | 默认值 | 说明 |
| --- | --- | --- |
| `name` | 无 | 设备名称，必填 |
| `macs` | 无 | MAC 地址，每行一个，至少一个，可为同一设备填多个 |
| `broadcastIps` | 空 | IPv4 广播地址，留空时向 `255.255.255.255` 及各启用网卡的广播地址发送 |
| `port` | 9 | 目标 UDP 端口 |
| `repeat` | 3 | 每个 MAC 重复发送次数，上限 20 |
| `checkAddr` | 空 | 在线探测地址，格式 `主机:端口`，如 `192.168.1.10:3389`，留空不探测 |
| `shutdownConn` | 空 | 远程关机使用的 [Web 终端](/docs/modules/terminal) SSH 连接，留空不支持关机 |
| `shutdownCommand` | 无 | 关机命令，选择了连接时必填，单行，如 `shutdown -h now`、`shutdown /s /t 0` |
| `remark` | 空 | 备注 |

设备列表上点击唤醒即发送魔术包，并按 `checkAddr` 显示设备是否在线。

### 远程关机

为设备选择一个 Web 终端中的 SSH 连接并填写关机命令后，设备卡片上出现「关机」按钮，接口为 `POST /api/wol/devices/{key}/shutdown`。

- 该连接必须已在 Web 终端中连接过一次、记录了主机密钥，Gately 不会无人值守地信任未知主机；
- 每次关机新建一条 SSH 连接，执行完即关闭，设备重启后不会用到已失效的旧连接；命令最长执行 30 秒，同一设备同时只执行一次；
- 关机常使 SSH 连接在返回退出码之前断开，这种情况视为已开始关机；
- 界面提供 Linux（`shutdown -h now`）、Windows（`shutdown /s /t 0`）与 macOS（`sudo -n shutdown -h now`）的常用命令。Windows 需开启 OpenSSH 服务器；Linux 非 root 用户可改用 `sudo -n shutdown -h now` 并为该命令配置免密 sudo。

## 计划任务

按 cron 表达式定时执行命令或发起 HTTP 请求。

- `spec` 支持标准 5 段表达式、可选的秒字段（6 段，如 `*/30 * * * * *`）以及 `@every 10m`、`@daily` 等描述符；
- `type` 为 `command` 时，通过 `sh -c`（Windows 为临时 `.bat`）执行 `command`，环境变量中注入 `taskName` 及带 `GATELY_` 前缀的同名大写变量；
- `type` 为 `http` 时按 `http` 配置发起请求，字段包括 `url`、`method`（GET、POST、PUT、PATCH、DELETE）、`headers`（每项 `Key: Value`）、`body`、`successContent`（响应体需包含其中任意一项才算成功，为空时只检查 2xx）、`timeout`、`insecureSkipVerify` 与 `proxy`（留空不指定代理，`update` 使用全局更新代理，或 `http://`、`https://`、`socks5://` 代理地址）；
- 任务可启用或停用，页面显示下次执行时间，可手动立即运行；
- 同一任务上一次尚未结束时，本次触发被跳过；
- 每个任务保留最近 20 次执行记录，包含开始时间、耗时、成功与否、是否手动触发，以及输出（最多保留 4000 字符摘要）；
- 创建表单提供表达式解析校验。

## 概览与系统日志

- 「概览」页展示主机信息：CPU 型号、使用率与负载，内存，磁盘，网卡地址与实时速率，公网 IPv4 与 IPv6（点击复制），各模块的服务状态，以及 Gately 进程的版本、提交、构建时间、启动时间、PID、CPU、常驻内存、Go 堆与协程数；
- Gately 托管着外部程序（frp、Tailscale、NetBird、Cloudflare 隧道、File Browser）时，「概览」页另有「托管进程」卡片，逐个列出这些进程所属的模块与实例、程序名、PID、运行时长、重启次数、CPU 与常驻内存，卡片头显示合计，点击一行进入对应模块；只统计进程本身，不含它再启动的子进程，也不显示路径与启动参数；
- 「系统日志」页汇总各模块日志，包括 Docker、FTP、计划任务等模块的运行记录。

## 主题

界面提供 15 个色系，分为 Gately 原创（翠绿、海洋、清新、樱花、暮光、午夜、朋克）和经典配色（GitHub, Nord, Dracula, Catppuccin, Tokyo Night, Gruvbox, Rosé Pine, Solarized），每个色系都有亮色与暗色两种版本。模式可选亮色、暗色或跟随系统，跟随系统时使用当前色系对应的亮色或暗色版本。主色可在任一色系上改为 10 种预设颜色之一，或用系统取色器自选，Logo 与登录页渐变随之变化，重新选择色系会清除自定义主色；圆角可选直角、跟随色系或圆润。主题设置保存在浏览器本地。
