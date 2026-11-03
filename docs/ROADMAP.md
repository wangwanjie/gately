# Gately 技术方案与路线图

## 目标

做一个开源、可审计、社区驱动的网络网关工具：覆盖家庭网络、NAS 与服务器用户最常用的 DDNS、STUN 穿透、端口转发、反向代理、证书管理，并支持导入现有 Lucky 配置。

设计原则：

1. **透明**：默认不连接任何项目方服务器；所有对外地址可见、可改、可关。
2. **可迁移**：提供 Lucky `.lkcf` 配置导入。
3. **可审计的配置**：配置为明文 JSON（文件权限 0600），可直接纳入 git 做 diff 和回滚。
4. **单文件部署**：一个静态二进制内嵌前端，支持 Docker、systemd、OpenWrt；可选模块与界面语言可在编译时去掉，生成更小的定制构建。

## 技术栈

| 层 | 选型 | 理由 |
| --- | --- | --- |
| 后端 | Go 1.26+ | 可直接复用 lego、pion/stun、frp、cloudflared、coraza、rclone 等成熟库；静态编译，交叉编译到 arm/arm64/mips/riscv 路由器和 NAS |
| HTTP | 标准库 `net/http`（Go 1.22 路由语法） | 依赖最少，便于审计 |
| 存储 | 每模块一个明文 JSON 文件，原子写入 | 便于导入、备份和 git diff |
| 认证 | bcrypt 密码 + HMAC 签名会话令牌，登录失败限流，后续支持 TOTP | 无外部依赖 |
| 日志 | 每模块内存环形缓冲 + JSON Lines 批量落盘（按大小轮转，重启后恢复）+ 增量拉取接口 | 日志轮询接口简单，前端增量刷新 |
| 前端 | Vue 3 + TypeScript + Vite + Element Plus，`go:embed` 打包 | 组件丰富，开发效率高 |
| 发布 | GoReleaser 多架构二进制 + 多架构 Docker 镜像 | — |

## 目录结构

```
cmd/gately/            程序入口
internal/app/          组装：加载配置、启动模块、HTTP 服务
internal/config/       模块化 JSON 配置存储
internal/auth/         管理员认证与会话
internal/logbuf/       模块日志环形缓冲
internal/httpx/        HTTP 工具与中间件
internal/module/       模块接口与注册表
internal/modules/*/    各功能模块
internal/modules/registry/  模块注册：可选模块各自一个 mod_<模块>.go（build tag no_<模块>），跨模块能力经注入提供
tools/custombuild/     定制构建：按 modules.json 换算 build tag、语言与 Docker 构建参数并打包
modules.json           可选模块、依赖、体积估算、语言档位、构建目标与预设（build.gately.cn 读取）
internal/luckyimport/  Lucky .lkcf 解密与字段映射
web/                   前端工程，构建产物由 web/embed.go 嵌入
```

## 里程碑

按用户实际使用规模排序（STUN 18、端口转发 25、反代 27、证书 5、DDNS 75 条记录）。

| 阶段 | 内容 | 验收 |
| --- | --- | --- |
| ✅ **M0 骨架** | 配置存储、管理员登录、模块框架、日志缓冲、管理 UI 外壳、Dockerfile | 登录后能看到总览与模块菜单 |
| ✅ **M1 端口转发** | TCP/UDP、v4/v6、多端口与端口段、多目标、连接数限制、启停与日志；导入 Lucky 端口转发规则 | 单元测试覆盖 TCP/UDP 转发；真实样本 25/25 条导入通过 |
| ✅ **M2 DDNS** | 取 IP（URL/网卡/命令）、IPv4/IPv6、阿里云/Cloudflare/DNSPod(腾讯云)/华为云/自定义回调、Webhook；导入 | 11 家服务商（阿里云签名经官方示例校验）；真实样本 2/2 任务、75 条记录导入通过 |
| ✅ **M3 STUN 穿透** | STUN 获取公网映射、TCP 保活、UPnP/NAT-PMP、内置转发、Webhook 与脚本变量；导入 | 本地端到端测试 + 家庭网络真实穿透；真实样本 18/18 条导入通过 |
| ✅ **M4 证书** | ACME（lego，DNS-01 复用 M2 Provider）、自动续期、导出到路径、变更脚本 | Pebble 端到端（通配符 + 续签）；真实样本 5/5 张导入且无需重签 |
| ✅ **M5 Web 服务** | 反向代理、重定向、静态文件、按域名路由、自动 TLS、BasicAuth、IP 过滤；导入 | 覆盖路由、4 种站点、认证、WebSocket、HTTPS、PROXY 协议收发；真实样本 27 个站点导入通过 |
| ✅ M6 运维 | 网络唤醒、Web 终端（SSH/本机 Shell）、计划任务、管理员两步验证 | WOL 3/3、终端连接 14/14 导入；TOTP 通过 RFC 6238 测试向量 |
| ✅ M7 生态（部分） | Coraza WAF、GitHub/Google/QQ/OIDC 第三方登录（站点认证与管理后台登录，可迁移 Lucky 身份表）、Docker 管理、WebDAV、FTP 服务、存储管理（本地/远程 WebDAV 挂载与在线文件管理） | WAF 用真实攻击载荷测试；Docker 在真实环境只读验证 |
| ✅ M7.5 组网穿透 | Tailscale、NetBird、frp（frpc、frps 均可多实例）、cloudflared，以进程托管方式集成官方二进制 | 配置生成、进程守护与日志采集有单元测试；不内置上游程序 |
| ✅ M8 可信与运维 | 操作审计日志、配置历史与回滚、声明式配置目录；Passkey 登录与只读账号；ACME HTTP-01；HTTP/3；全局 Prometheus 指标与 Grafana 面板；网络自检与首次启动引导；DDNS 新增 Route 53、OVH、Gandi、NS1、Bunny、Namecheap、DynDNS2；通知新增 ntfy、Gotify、Pushover、Discord、Slack、Server酱、PushPlus 与规则模板；发布产物 cosign 签名、构建来源证明与 SBOM；systemd 与 OpenWrt 服务模板 | WebAuthn 用软件验证器端到端测试；Route 53 签名经 AWS 官方 SigV4 测试向量校验；HTTP/3 用 quic-go 客户端实测 |
| ✅ M8.5 安装与升级 | 飞牛 fnOS 原生应用（.fpk，接入统一网关与飞牛登录态）；Linux 二进制在线升级；OpenWrt 安装包（24.10 及以前的 .ipk、25.12 起的 .apk） | fpk 在飞牛虚拟机实测；OpenWrt 安装包在 24.10 / 25.12 官方 rootfs 容器与 iStoreOS 实测，CI 冒烟测试覆盖安装与卸载 |
| ✅ M8.6 迁移与运维 | 导入 nginx 配置（`nginx -T`，含 include 展开、upstream、Authelia 的 `auth_request`）；证书自动部署（经 SSH 推送并执行命令、重启 Docker 容器）；WAF 请求体检查上限、超限只检查前段与按路径跳过；订阅 IP 列表（定时刷新、站点与路径规则引用）；自检增加 HTTPS 站点证书覆盖与 frp 代理冲突；在线升级支持四段版本号 | nginx 导入用真实生产配置（77 个站点、Authelia、upstream）验证；SSH 推送用 SFTP 测试服务端端到端测试；WAF 用 2 MB 请求体验证拒绝、只检查前段与跳过路径 |
| ✅ M8.7 入侵防护 | 类 fail2ban 的日志规则（日志文件、systemd 日志、命令输出、Docker 容器日志、管理后台登录失败、WAF 拦截）与常见服务模板；诱饵端口与端口扫描检测；统一封禁表（合并 Web 服务的认证失败封禁，持久化、递增时长、手动网段与永久封禁、白名单）；端口转发、Web 服务、STUN、WebDAV、FTP 与管理后台都拒绝被封禁的地址；封禁同步到 nftables（独立的 `inet gately` 表）在内核层丢弃 | 在 fnOS（nftables 1.0.6）与 iStoreOS 24.10（nftables 1.1.1）的独立网络命名空间中验证诱饵端口、扫描检测（IPv4/IPv6）、四种日志来源、持久化与规则自愈；Docker 容器验证权限检测与用户态诱饵端口回退 |
| M9 待定 | 多管理员与细粒度角色 | 视社区需求排期 |

## 设计取舍

- 在线升级只在 Linux 二进制部署中提供，且只由管理员手动触发：下载后核对 checksums.txt 的 SHA-256 并试运行新程序，替换时保留旧版本；Docker、飞牛 fnOS 与 OpenWrt 安装包仍由镜像、应用中心与包管理器负责升级。定制构建不提供在线升级（会装回完整版本），在 build.gately.cn 用相同选项重新生成后上传升级包。
- 不下发远程配置，不上报设备信息。
- 配置明文存储；敏感字段在 API 输出中脱敏。
