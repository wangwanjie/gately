Gately diagnostics bundle / Gately 诊断包
=========================================

This archive helps maintainers locate problems. Every file is plain text or JSON:
open it with any editor and review it before attaching it to a GitHub issue.
本压缩包用于帮助维护者定位问题。所有文件均为纯文本或 JSON，提交 issue 前可以用任意编辑器检查。

Included / 包含
- summary.json   version, build info, OS/arch, CPU/memory/disk totals, uptime, Go runtime stats, network interface names with IP categories
                 版本与构建信息、系统与架构、CPU/内存/磁盘容量、运行时长、Go 运行时统计、网卡名称及其 IP 类别
- modules.json   module list with the status summary shown on the overview page
                 模块列表及总览页的状态摘要
- config/*.json  configuration structure: field names, numbers, booleans, protocol/mode enums; every other string is replaced by "<redacted>"
                 配置的结构视图：字段名、数字、布尔值、协议/模式等枚举；其余字符串全部替换为 "<redacted>"
- logs/*.log     recent runtime logs, including those persisted before a restart (only if enabled), with IPs, domains, e-mails, URLs, secrets, MAC addresses and user directories masked
                 最近的运行日志，包括重启前已落盘的部分（仅在勾选时导出），其中 IP、域名、邮箱、URL、密钥、MAC 地址和用户目录已被替换
- logs/crash.log abnormal exit records (only if any): Go runtime crash messages with stacks, and when the process was killed (e.g. by the OOM killer); stack frames are kept, other lines are masked like the logs
                 异常退出记录（发生过才有）：Go 运行时崩溃信息与堆栈，以及进程被强制结束（如 OOM）的时间；堆栈行原样保留，其余行与日志一样脱敏

Never included / 绝不包含
- Passwords, password hashes, API tokens, TOTP secrets, session keys, OAuth client secrets, DNS provider keys, SSH passwords/private keys, certificates and private keys
  密码、密码哈希、API 令牌、两步验证密钥、会话密钥、OAuth 密钥、DNS 服务商密钥、SSH 密码/私钥、证书与私钥
- Domain names, host names, real IP addresses, MAC addresses (only categories such as <ipv4:private> / <ipv4:public> are kept)
  域名、主机名、真实 IP 地址、MAC 地址（仅保留 <ipv4:private> / <ipv4:public> 这类类别）
- File and directory contents, Docker container/image names and environment variables, terminal sessions and traffic content
  文件与目录内容、Docker 容器/镜像名称与环境变量、终端会话与流量内容
- account.json (administrator account) and tokens.json (API tokens) are never read
  account.json（管理员账号）与 tokens.json（API 令牌）不会被读取

Note / 说明
Free-text names you typed (for example a rule remark) can appear inside log lines when they are not recognisable as an IP or a domain. Please skim logs/*.log before posting.
用户自填的名称（如规则备注）若出现在日志里，且不是 IP 或域名形态，可能无法被自动识别。发布前请快速浏览 logs/*.log。
