# 模块总览

Gately 的每项能力是一个独立模块，可单独启停、单独配置，配置各自保存为一个明文 JSON 文件。

在「设置 → 功能模块」中可以关闭整个模块：关闭的模块只加载配置，不启动服务、不提供管理接口（接口返回 403），并从界面中隐藏；已有配置会保留，重新开启后恢复。被其他模块依赖的模块不能关闭，修改后重启生效。

## 定制构建 {#custom-build}

除端口转发、Web 服务与 SSL 证书三个核心模块外，其余模块都可以在编译时去掉，界面语言也可以只保留简体中文或简繁英三种，安装包与内存占用随之变小，适合存储空间有限的路由器。在 [build.gately.cn](https://build.gately.cn) 勾选模块与语言即可在线生成二进制、OpenWrt、飞牛安装包或多架构 Docker 镜像，页面会估算体积；也可以按 [从源码构建](/download#custom-build) 自行编译。

定制构建中没有编译进来的模块：配置文件原样保留，设置中的开关与 API 令牌的权限照常有效；接口返回 404 并说明未包含在此构建中，界面隐藏对应页面与相关选项（如站点的 WAF、证书部署到 SSH 或 Docker）；换回完整版后恢复。「关于」页列出本构建去掉的模块，并附带用相同选项打开构建器的链接。定制构建不能在线升级（在线升级下载的是完整版本），新版本发布后到构建器用相同选项重新生成，再「上传升级包」。

| 模块 | 能力 | 文档 |
| --- | --- | --- |
| 动态域名 | 11 家 DNS 服务商，URL / 网卡 / 命令取 IP，IPv4 / IPv6，Webhook | [查看](/docs/modules/ddns) |
| STUN 穿透 | TCP / UDP、保活、UPnP / NAT-PMP、内置转发、变化时触发 Webhook 与脚本 | [查看](/docs/modules/stun) |
| 端口转发 | TCP / UDP、IPv4 / IPv6、端口段、多目标故障转移、连接数限制、流量统计 | [查看](/docs/modules/portforward) |
| 协议分流 | 多个服务共用一个端口：TLS 按 SNI 与 ALPN、HTTP 按 Host，Minecraft 按服务器地址，另可识别 SSH、RDP、SOCKS、OpenVPN、RTSP、SIP、XMPP、MQTT、AMQP、PostgreSQL、Redis、SMB、Git、IRC、ADB、BitTorrent，VNC 等走等待超时路由 | [查看](/docs/modules/protoroute) |
| SSL 证书 | ACME DNS-01 自动签发与续期、上传、映射到目录并执行脚本 | [查看](/docs/modules/ssl) |
| Web 服务 | 反向代理、重定向、静态文件、HTTPS、认证、访问控制、负载均衡与健康检查、缓存与限流、访问统计、导航面板、访客审计、网络拓扑 | [查看](/docs/modules/web) |
| OIDC 单点登录 | 把共享用户以 OpenID Connect 提供给 Gitea、OpenList、Grafana 等应用登录，可替代 Authelia 的 OIDC | [查看](/docs/modules/oidc) |
| WAF 防火墙 | Coraza + OWASP CRS v4 | [查看](/docs/modules/waf) |
| 入侵防护 | 日志规则（类 fail2ban）、诱饵端口、端口扫描检测，封禁同步到 nftables | [查看](/docs/modules/guard) |
| 端口敲门 | 兼容 fwknop 的单包敲门（SPA），防火墙拦截 SSH 等端口，端口转发、协议分流、STUN 按敲门放行 | [查看](/docs/modules/knock) |
| Tailscale / NetBird / frp / cloudflared | 托管官方二进制的组网与内网穿透 | [查看](/docs/modules/tunnels) |
| Docker 管理 | 容器创建与编辑、日志与终端、Compose、镜像更新与升级、网络与卷 | [查看](/docs/modules/docker) |
| Web 终端 | SSH、Telnet、本机 Shell、SFTP 文件管理、持久会话 | [查看](/docs/modules/terminal) |
| 网络诊断 | Ping、路由追踪、端口探测、DNS 查询、测速、局域网扫描 | [查看](/docs/modules/nettools) |
| 通知中心 | 多渠道事件推送与规则 | [查看](/docs/modules/notify) |
| 性能监控 | 主机、进程与站点的性能趋势（含首字节延迟），阈值加持续时长告警 | [查看](/docs/modules/perf) |
| 配置备份 | 加密备份、保留策略、定时备份与恢复 | [查看](/docs/modules/backup) |
| 文件与运维工具 | WebDAV、FTP、SMB、File Browser、存储挂载、网络唤醒、计划任务 | [查看](/docs/modules/tools) |
