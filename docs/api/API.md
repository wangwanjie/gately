# Gately 管理 API

Gately 管理 API 用于脚本与自动化批量管理。

认证：在管理后台的“API 令牌”页创建令牌，请求头携带 `Authorization: Bearer gtl_xxxx`。

权限：每个令牌只拥有创建时勾选的权限，格式为 `<模块>:read` 或 `<模块>:write`，写权限包含读权限。账号、两步验证、令牌管理以及备份、恢复、重启、系统设置接口只能使用登录会话。

请求体使用 `Content-Type: application/json`，错误统一返回 `{"error": "说明"}`。

在「设置 → 功能模块」中关闭的模块不提供接口，请求其接口返回 403，运行中实例的接口文档也不再列出这些接口。

官网：https://gately.cn

机器可读的描述见同目录的 `openapi.json`（OpenAPI 3.0）。

## 快速开始

```bash
# 查看系统状态（需要 status:read）
curl -H "Authorization: Bearer gtl_xxxx" http://localhost:16680/api/status

# 列出全部 DDNS 任务（需要 ddns:read）
curl -H "Authorization: Bearer gtl_xxxx" http://localhost:16680/api/ddns/tasks

# 立即同步一个 DDNS 任务（需要 ddns:write，key 取自上一步返回的列表）
curl -X POST -H "Authorization: Bearer gtl_xxxx" http://localhost:16680/api/ddns/tasks/<key>/sync
```

## 权限

| 权限域 | 名称 | 可用权限 |
| --- | --- | --- |
| `portforward` | 端口转发 | `portforward:read`、`portforward:write` |
| `protoroute` | 协议分流 | `protoroute:read`、`protoroute:write` |
| `ddns` | 动态域名 | `ddns:read`、`ddns:write` |
| `stun` | STUN 穿透 | `stun:read`、`stun:write` |
| `tailscale` | Tailscale 组网 | `tailscale:read`、`tailscale:write` |
| `frp` | frp 内网穿透 | `frp:read`、`frp:write` |
| `cloudflared` | Cloudflare 隧道 | `cloudflared:read`、`cloudflared:write` |
| `ssl` | SSL 证书 | `ssl:read`、`ssl:write` |
| `waf` | WAF 防火墙 | `waf:read`、`waf:write` |
| `webservice` | Web 服务 | `webservice:read`、`webservice:write` |
| `guard` | 入侵防护 | `guard:read`、`guard:write` |
| `wol` | 网络唤醒 | `wol:read`、`wol:write` |
| `webterminal` | Web 终端 | `webterminal:read`、`webterminal:write` |
| `cron` | 计划任务 | `cron:read`、`cron:write` |
| `docker` | Docker 管理 | `docker:read`、`docker:write` |
| `webdav` | WebDAV 服务 | `webdav:read`、`webdav:write` |
| `ftp` | FTP 服务 | `ftp:read`、`ftp:write` |
| `smb` | SMB 服务 | `smb:read`、`smb:write` |
| `filebrowser` | File Browser | `filebrowser:read`、`filebrowser:write` |
| `storage` | 存储挂载 | `storage:read`、`storage:write` |
| `notify` | 通知中心 | `notify:read`、`notify:write` |
| `nettools` | 网络诊断 | `nettools:read`、`nettools:write` |
| `backup` | 配置备份 | `backup:read`、`backup:write` |
| `status` | 系统状态 | `status:read` |
| `logs` | 系统日志 | `logs:read` |
| `metrics` | 监控指标（Prometheus） | `metrics:read` |
| `import` | Lucky 配置导入 | `import:write` |
| `fileusers` | 文件服务共享用户 | `fileusers:read`、`fileusers:write` |

通配写法：`*:read` 表示全部模块只读，`*:write` 或 `*` 表示全部模块读写。

## 端口转发

| 方法 | 路径 | 所需权限 | 说明 |
| --- | --- | --- | --- |
| GET | `/api/portforward/logs` | `portforward:read` | 查看模块运行日志 |
| DELETE | `/api/portforward/logs` | `portforward:write` | 清空模块运行日志 |
| GET | `/api/portforward/rules` | `portforward:read` | 获取端口转发规则列表 |
| POST | `/api/portforward/rules` | `portforward:write` | 新增端口转发规则 |
| PUT | `/api/portforward/rules/{key}` | `portforward:write` | 修改端口转发规则 |
| DELETE | `/api/portforward/rules/{key}` | `portforward:write` | 删除端口转发规则 |
| PUT | `/api/portforward/rules/{key}/enable` | `portforward:write` | 启用或停用端口转发规则 |
| PUT | `/api/portforward/rules-order` | `portforward:write` | 调整端口转发规则排序 |
| PUT | `/api/portforward/rules-group` | `portforward:write` | 批量设置端口转发规则分组 |
| GET | `/api/portforward/settings` | `portforward:read` | 读取端口转发全局设置 |
| PUT | `/api/portforward/settings` | `portforward:write` | 保存端口转发全局设置 |

## 协议分流

| 方法 | 路径 | 所需权限 | 说明 |
| --- | --- | --- | --- |
| GET | `/api/protoroute/logs` | `protoroute:read` | 查看模块运行日志 |
| DELETE | `/api/protoroute/logs` | `protoroute:write` | 清空模块运行日志 |
| GET | `/api/protoroute/rules` | `protoroute:read` | 获取协议分流规则列表 |
| POST | `/api/protoroute/rules` | `protoroute:write` | 新增协议分流规则 |
| PUT | `/api/protoroute/rules/{key}` | `protoroute:write` | 修改协议分流规则 |
| DELETE | `/api/protoroute/rules/{key}` | `protoroute:write` | 删除协议分流规则 |
| PUT | `/api/protoroute/rules/{key}/enable` | `protoroute:write` | 启用或停用协议分流规则 |
| PUT | `/api/protoroute/rules-order` | `protoroute:write` | 调整协议分流规则排序 |
| PUT | `/api/protoroute/rules-group` | `protoroute:write` | 批量设置协议分流规则分组 |
| GET | `/api/protoroute/settings` | `protoroute:read` | 读取协议分流全局设置 |
| PUT | `/api/protoroute/settings` | `protoroute:write` | 保存协议分流全局设置 |

## 动态域名

| 方法 | 路径 | 所需权限 | 说明 |
| --- | --- | --- | --- |
| GET | `/api/ddns/logs` | `ddns:read` | 查看模块运行日志 |
| DELETE | `/api/ddns/logs` | `ddns:write` | 清空模块运行日志 |
| GET | `/api/ddns/tasks` | `ddns:read` | 获取DDNS 任务列表 |
| POST | `/api/ddns/tasks` | `ddns:write` | 新增DDNS 任务 |
| PUT | `/api/ddns/tasks/{key}` | `ddns:write` | 修改DDNS 任务 |
| DELETE | `/api/ddns/tasks/{key}` | `ddns:write` | 删除DDNS 任务 |
| PUT | `/api/ddns/tasks/{key}/enable` | `ddns:write` | 启用或停用DDNS 任务 |
| PUT | `/api/ddns/tasks-order` | `ddns:write` | 调整DDNS 任务排序 |
| PUT | `/api/ddns/tasks-group` | `ddns:write` | 批量设置DDNS 任务分组 |
| GET | `/api/ddns/tasks/{key}/secrets` | `ddns:write` | 查看密钥明文 |
| POST | `/api/ddns/tasks/{key}/sync` | `ddns:write` | 立即同步一个 DDNS 任务 |
| DELETE | `/api/ddns/tasks/{key}/records/{rec}` | `ddns:write` | 删除 DDNS 任务中的一条记录 |
| PUT | `/api/ddns/tasks/{key}/records-layout` | `ddns:write` | 调整任务内解析记录的顺序与分组 |
| GET | `/api/ddns/providers` | `ddns:read` | 列出支持的 DNS 服务商及其所需参数 |
| GET | `/api/ddns/netinterfaces` | `ddns:read` | 列出本机网卡及地址 |
| POST | `/api/ddns/iptest` | `ddns:write` | 测试获取公网 IP |
| GET | `/api/ddns/defaults` | `ddns:read` | 读取 DDNS 默认值 |
| GET | `/api/ddns/settings` | `ddns:read` | 读取 DDNS 全局设置 |
| PUT | `/api/ddns/settings` | `ddns:write` | 保存 DDNS 全局设置 |

## STUN 穿透

| 方法 | 路径 | 所需权限 | 说明 |
| --- | --- | --- | --- |
| GET | `/api/stun/logs` | `stun:read` | 查看模块运行日志 |
| DELETE | `/api/stun/logs` | `stun:write` | 清空模块运行日志 |
| GET | `/api/stun/rules` | `stun:read` | 获取STUN 规则列表 |
| POST | `/api/stun/rules` | `stun:write` | 新增STUN 规则 |
| PUT | `/api/stun/rules/{key}` | `stun:write` | 修改STUN 规则 |
| DELETE | `/api/stun/rules/{key}` | `stun:write` | 删除STUN 规则 |
| PUT | `/api/stun/rules/{key}/enable` | `stun:write` | 启用或停用STUN 规则 |
| PUT | `/api/stun/rules-order` | `stun:write` | 调整STUN 规则排序 |
| PUT | `/api/stun/rules-group` | `stun:write` | 批量设置STUN 规则分组 |
| POST | `/api/stun/rules/{key}/restart` | `stun:write` | 重启一条 STUN 规则 |
| GET | `/api/stun/netinterfaces` | `stun:read` | 列出本机网卡及地址 |
| GET | `/api/stun/settings` | `stun:read` | 读取 STUN 全局设置 |
| PUT | `/api/stun/settings` | `stun:write` | 保存 STUN 全局设置 |

## Tailscale 组网

| 方法 | 路径 | 所需权限 | 说明 |
| --- | --- | --- | --- |
| GET | `/api/tailscale/logs` | `tailscale:read` | 查看模块运行日志 |
| DELETE | `/api/tailscale/logs` | `tailscale:write` | 清空模块运行日志 |
| GET | `/api/tailscale/settings` | `tailscale:read` | 读取 Tailscale 设置 |
| PUT | `/api/tailscale/settings` | `tailscale:write` | 保存 Tailscale 设置 |
| GET | `/api/tailscale/secrets` | `tailscale:write` | 查看密钥明文 |
| GET | `/api/tailscale/status` | `tailscale:read` | 查看 Tailscale 连接状态 |
| POST | `/api/tailscale/restart` | `tailscale:write` | 重启 Tailscale |
| POST | `/api/tailscale/logout` | `tailscale:write` | 退出 Tailscale 登录 |
| GET | `/api/tailscale/install` | `tailscale:read` | 查看托管程序的安装信息与进度 |
| POST | `/api/tailscale/install` | `tailscale:write` | 在线安装或更新托管程序 |
| POST | `/api/tailscale/install/rollback` | `tailscale:write` | 切换到保存的旧版本 |
| DELETE | `/api/tailscale/install` | `tailscale:write` | 卸载托管程序 |
| DELETE | `/api/tailscale/install/backups/{id}` | `tailscale:write` | 删除保存的旧版本 |
| POST | `/api/tailscale/reset` | `tailscale:write` | 恢复模块默认配置 |

## frp 内网穿透

| 方法 | 路径 | 所需权限 | 说明 |
| --- | --- | --- | --- |
| GET | `/api/frp/logs` | `frp:read` | 查看模块运行日志 |
| DELETE | `/api/frp/logs` | `frp:write` | 清空模块运行日志 |
| GET | `/api/frp/clients` | `frp:read` | 获取frp 客户端列表 |
| POST | `/api/frp/clients` | `frp:write` | 新增frp 客户端 |
| PUT | `/api/frp/clients/{key}` | `frp:write` | 修改frp 客户端 |
| DELETE | `/api/frp/clients/{key}` | `frp:write` | 删除frp 客户端 |
| PUT | `/api/frp/clients/{key}/enable` | `frp:write` | 启用或停用frp 客户端 |
| PUT | `/api/frp/clients-order` | `frp:write` | 调整frp 客户端排序 |
| PUT | `/api/frp/clients-group` | `frp:write` | 批量设置frp 客户端分组 |
| GET | `/api/frp/clients/{key}/secrets` | `frp:write` | 查看密钥明文 |
| GET | `/api/frp/servers` | `frp:read` | 获取frp 服务端列表 |
| POST | `/api/frp/servers` | `frp:write` | 新增frp 服务端 |
| PUT | `/api/frp/servers/{key}` | `frp:write` | 修改frp 服务端 |
| DELETE | `/api/frp/servers/{key}` | `frp:write` | 删除frp 服务端 |
| PUT | `/api/frp/servers/{key}/enable` | `frp:write` | 启用或停用frp 服务端 |
| PUT | `/api/frp/servers-order` | `frp:write` | 调整frp 服务端排序 |
| PUT | `/api/frp/servers-group` | `frp:write` | 批量设置frp 服务端分组 |
| GET | `/api/frp/servers/{key}/secrets` | `frp:write` | 查看密钥明文 |
| GET | `/api/frp/proxies` | `frp:read` | 获取frp 代理列表 |
| POST | `/api/frp/proxies` | `frp:write` | 新增frp 代理 |
| PUT | `/api/frp/proxies/{key}` | `frp:write` | 修改frp 代理 |
| DELETE | `/api/frp/proxies/{key}` | `frp:write` | 删除frp 代理 |
| PUT | `/api/frp/proxies/{key}/enable` | `frp:write` | 启用或停用frp 代理 |
| PUT | `/api/frp/proxies-order` | `frp:write` | 调整frp 代理排序 |
| PUT | `/api/frp/proxies-group` | `frp:write` | 批量设置frp 代理分组 |
| GET | `/api/frp/proxies/{key}/secrets` | `frp:write` | 查看密钥明文 |
| POST | `/api/frp/import` | `frp:write` | 导入已有的 frpc / frps 配置（支持旧版 ini 与新版 toml）；apply 为 false 时仅预览 |
| GET | `/api/frp/install` | `frp:read` | 查看托管程序的安装信息与进度 |
| POST | `/api/frp/install` | `frp:write` | 在线安装或更新托管程序 |
| POST | `/api/frp/install/rollback` | `frp:write` | 切换到保存的旧版本 |
| DELETE | `/api/frp/install` | `frp:write` | 卸载托管程序 |
| DELETE | `/api/frp/install/backups/{id}` | `frp:write` | 删除保存的旧版本 |
| POST | `/api/frp/clients/{key}/restart` | `frp:write` | 重启 frpc 实例 |
| POST | `/api/frp/servers/{key}/restart` | `frp:write` | 重启 frps 实例 |
| GET | `/api/frp/settings` | `frp:read` | 读取程序路径与版本 |
| PUT | `/api/frp/settings` | `frp:write` | 保存程序路径 |
| POST | `/api/frp/reset` | `frp:write` | 恢复模块默认配置 |

## Cloudflare 隧道

| 方法 | 路径 | 所需权限 | 说明 |
| --- | --- | --- | --- |
| GET | `/api/cloudflared/logs` | `cloudflared:read` | 查看模块运行日志 |
| DELETE | `/api/cloudflared/logs` | `cloudflared:write` | 清空模块运行日志 |
| GET | `/api/cloudflared/tunnels` | `cloudflared:read` | 获取Cloudflare 隧道列表 |
| POST | `/api/cloudflared/tunnels` | `cloudflared:write` | 新增Cloudflare 隧道 |
| PUT | `/api/cloudflared/tunnels/{key}` | `cloudflared:write` | 修改Cloudflare 隧道 |
| DELETE | `/api/cloudflared/tunnels/{key}` | `cloudflared:write` | 删除Cloudflare 隧道 |
| PUT | `/api/cloudflared/tunnels/{key}/enable` | `cloudflared:write` | 启用或停用Cloudflare 隧道 |
| PUT | `/api/cloudflared/tunnels-order` | `cloudflared:write` | 调整Cloudflare 隧道排序 |
| PUT | `/api/cloudflared/tunnels-group` | `cloudflared:write` | 批量设置Cloudflare 隧道分组 |
| GET | `/api/cloudflared/tunnels/{key}/secrets` | `cloudflared:write` | 查看密钥明文 |
| POST | `/api/cloudflared/tunnels/{key}/restart` | `cloudflared:write` | 重启一条 Cloudflare 隧道 |
| GET | `/api/cloudflared/tunnels/{key}/ingress` | `cloudflared:read` | 读取令牌隧道的路由规则 |
| PUT | `/api/cloudflared/tunnels/{key}/ingress` | `cloudflared:write` | 保存令牌隧道的路由规则 |
| GET | `/api/cloudflared/settings` | `cloudflared:read` | 读取 Cloudflare 隧道设置 |
| PUT | `/api/cloudflared/settings` | `cloudflared:write` | 保存 Cloudflare 隧道设置 |
| GET | `/api/cloudflared/install` | `cloudflared:read` | 查看托管程序的安装信息与进度 |
| POST | `/api/cloudflared/install` | `cloudflared:write` | 在线安装或更新托管程序 |
| POST | `/api/cloudflared/install/rollback` | `cloudflared:write` | 切换到保存的旧版本 |
| DELETE | `/api/cloudflared/install` | `cloudflared:write` | 卸载托管程序 |
| DELETE | `/api/cloudflared/install/backups/{id}` | `cloudflared:write` | 删除保存的旧版本 |
| POST | `/api/cloudflared/reset` | `cloudflared:write` | 恢复模块默认配置 |

## SSL 证书

| 方法 | 路径 | 所需权限 | 说明 |
| --- | --- | --- | --- |
| GET | `/api/ssl/logs` | `ssl:read` | 查看模块运行日志 |
| DELETE | `/api/ssl/logs` | `ssl:write` | 清空模块运行日志 |
| GET | `/api/ssl/certs` | `ssl:read` | 获取证书列表 |
| POST | `/api/ssl/certs` | `ssl:write` | 新增证书 |
| PUT | `/api/ssl/certs/{key}` | `ssl:write` | 修改证书 |
| DELETE | `/api/ssl/certs/{key}` | `ssl:write` | 删除证书 |
| PUT | `/api/ssl/certs/{key}/enable` | `ssl:write` | 启用或停用证书 |
| PUT | `/api/ssl/certs-order` | `ssl:write` | 调整证书排序 |
| PUT | `/api/ssl/certs-group` | `ssl:write` | 批量设置证书分组 |
| GET | `/api/ssl/certs/{key}/secrets` | `ssl:write` | 查看密钥明文 |
| POST | `/api/ssl/certs/{key}/renew` | `ssl:write` | 立即签发或续期证书 |
| GET | `/api/ssl/certs/{key}/download` | `ssl:write` | 下载证书与私钥 |
| POST | `/api/ssl/certs/{key}/deploy` | `ssl:write` | 立即部署证书 |
| GET | `/api/ssl/deploy-targets` | `ssl:read` | 列出可选的部署目标 |
| GET | `/api/ssl/providers` | `ssl:read` | 列出支持的 DNS 验证服务商 |
| GET | `/api/ssl/settings` | `ssl:read` | 读取证书全局设置 |
| PUT | `/api/ssl/settings` | `ssl:write` | 保存证书全局设置 |

## WAF 防火墙

| 方法 | 路径 | 所需权限 | 说明 |
| --- | --- | --- | --- |
| GET | `/api/waf/logs` | `waf:read` | 查看模块运行日志 |
| DELETE | `/api/waf/logs` | `waf:write` | 清空模块运行日志 |
| GET | `/api/waf/instances` | `waf:read` | 获取WAF 实例列表 |
| POST | `/api/waf/instances` | `waf:write` | 新增WAF 实例 |
| PUT | `/api/waf/instances/{key}` | `waf:write` | 修改WAF 实例 |
| DELETE | `/api/waf/instances/{key}` | `waf:write` | 删除WAF 实例 |
| PUT | `/api/waf/instances/{key}/enable` | `waf:write` | 启用或停用WAF 实例 |
| PUT | `/api/waf/instances-order` | `waf:write` | 调整WAF 实例排序 |
| PUT | `/api/waf/instances-group` | `waf:write` | 批量设置WAF 实例分组 |
| GET | `/api/waf/crs` | `waf:read` | 列出 OWASP CRS 规则文件 |
| GET | `/api/waf/crs/{file}` | `waf:read` | 读取一个 CRS 规则文件 |

## Web 服务

| 方法 | 路径 | 所需权限 | 说明 |
| --- | --- | --- | --- |
| GET | `/api/webservice/logs` | `webservice:read` | 查看模块运行日志 |
| DELETE | `/api/webservice/logs` | `webservice:write` | 清空模块运行日志 |
| GET | `/api/webservice/listeners` | `webservice:read` | 获取Web 服务监听列表 |
| POST | `/api/webservice/listeners` | `webservice:write` | 新增Web 服务监听 |
| PUT | `/api/webservice/listeners/{key}` | `webservice:write` | 修改Web 服务监听 |
| DELETE | `/api/webservice/listeners/{key}` | `webservice:write` | 删除Web 服务监听 |
| PUT | `/api/webservice/listeners/{key}/enable` | `webservice:write` | 启用或停用Web 服务监听 |
| PUT | `/api/webservice/listeners-order` | `webservice:write` | 调整Web 服务监听排序 |
| GET | `/api/webservice/iplists` | `webservice:read` | 获取订阅 IP 列表 |
| POST | `/api/webservice/iplists` | `webservice:write` | 新增订阅 IP 列表 |
| PUT | `/api/webservice/iplists/{key}` | `webservice:write` | 修改订阅 IP 列表 |
| DELETE | `/api/webservice/iplists/{key}` | `webservice:write` | 删除订阅 IP 列表 |
| PUT | `/api/webservice/iplists/{key}/enable` | `webservice:write` | 启用或停用订阅 IP 列表 |
| PUT | `/api/webservice/iplists-order` | `webservice:write` | 调整订阅 IP 列表排序 |
| POST | `/api/webservice/iplists/{key}/refresh` | `webservice:write` | 立即刷新订阅 IP 列表 |
| GET | `/api/webservice/users` | `webservice:read` | 读取共享用户与 OIDC 身份提供方 |
| PUT | `/api/webservice/users` | `webservice:write` | 保存共享用户与 OIDC 身份提供方，批量设置站点 |
| POST | `/api/webservice/import/nginx` | `webservice:write` | 导入 nginx 配置 |
| GET | `/api/webservice/settings` | `webservice:read` | 读取 Web 服务全局设置 |
| GET | `/api/webservice/settings/secrets/totp` | `webservice:write` | 查看共享用户的 TOTP 密钥明文（按用户名索引） |
| GET | `/api/webservice/settings/secrets/oauth` | `webservice:write` | 查看第三方登录的 Client Secret 明文（按登录方式标识索引） |
| POST | `/api/webservice/totp-secret` | `webservice:write` | 为共享用户生成新的 TOTP 密钥（不保存，保存设置后生效） |
| PUT | `/api/webservice/settings` | `webservice:write` | 保存 Web 服务全局设置 |
| GET | `/api/webservice/stats` | `webservice:read` | 站点分钟级统计（?listener=&site=&minutes=） |
| GET | `/api/webservice/metrics` | `webservice:read` | Prometheus 文本格式指标 |
| GET | `/api/webservice/bans` | `webservice:read` | 当前被封禁的 IP |
| POST | `/api/webservice/bans` | `webservice:write` | 手动封禁 IP |
| DELETE | `/api/webservice/bans` | `webservice:write` | 解除封禁（body 为 {"key": 地址}） |
| GET | `/api/webservice/geoip` | `webservice:read` | GeoIP 数据库状态 |
| POST | `/api/webservice/geoip/upload` | `webservice:write` | 上传 GeoIP 数据库（multipart 字段 file，.mmdb） |
| POST | `/api/webservice/geoip/download` | `webservice:write` | 从地址下载 GeoIP 数据库（.mmdb） |
| DELETE | `/api/webservice/geoip` | `webservice:write` | 移除 GeoIP 数据库 |

## 入侵防护

| 方法 | 路径 | 所需权限 | 说明 |
| --- | --- | --- | --- |
| GET | `/api/guard/logs` | `guard:read` | 查看模块运行日志 |
| DELETE | `/api/guard/logs` | `guard:write` | 清空模块运行日志 |
| GET | `/api/guard/jails` | `guard:read` | 获取入侵防护规则列表 |
| POST | `/api/guard/jails` | `guard:write` | 新增入侵防护规则 |
| PUT | `/api/guard/jails/{key}` | `guard:write` | 修改入侵防护规则 |
| DELETE | `/api/guard/jails/{key}` | `guard:write` | 删除入侵防护规则 |
| PUT | `/api/guard/jails/{key}/enable` | `guard:write` | 启用或停用入侵防护规则 |
| PUT | `/api/guard/jails-order` | `guard:write` | 调整入侵防护规则排序 |
| PUT | `/api/guard/jails-group` | `guard:write` | 批量设置入侵防护规则分组 |
| GET | `/api/guard/presets` | `guard:read` | 常见服务的规则模板 |
| POST | `/api/guard/jails/test` | `guard:write` | 用日志样例测试过滤规则 |
| GET | `/api/guard/status` | `guard:read` | 防火墙同步状态与运行环境 |
| GET | `/api/guard/settings` | `guard:read` | 全局设置 |
| PUT | `/api/guard/settings` | `guard:write` | 修改全局设置 |
| GET | `/api/guard/bans` | `guard:read` | 当前封禁的 IP |
| POST | `/api/guard/bans` | `guard:write` | 手动封禁 IP 或网段 |
| DELETE | `/api/guard/bans` | `guard:write` | 解除封禁（body 为 {"key": 地址}） |
| GET | `/api/guard/firewall/rules` | `guard:read` | Gately 写入 nftables 的规则（文本） |

## 网络唤醒

| 方法 | 路径 | 所需权限 | 说明 |
| --- | --- | --- | --- |
| GET | `/api/wol/logs` | `wol:read` | 查看模块运行日志 |
| DELETE | `/api/wol/logs` | `wol:write` | 清空模块运行日志 |
| GET | `/api/wol/devices` | `wol:read` | 获取唤醒设备列表 |
| POST | `/api/wol/devices` | `wol:write` | 新增唤醒设备 |
| PUT | `/api/wol/devices/{key}` | `wol:write` | 修改唤醒设备 |
| DELETE | `/api/wol/devices/{key}` | `wol:write` | 删除唤醒设备 |
| PUT | `/api/wol/devices/{key}/enable` | `wol:write` | 启用或停用唤醒设备 |
| PUT | `/api/wol/devices-order` | `wol:write` | 调整唤醒设备排序 |
| PUT | `/api/wol/devices-group` | `wol:write` | 批量设置唤醒设备分组 |
| POST | `/api/wol/devices/{key}/wake` | `wol:write` | 向设备发送唤醒魔术包 |
| GET | `/api/wol/status` | `wol:read` | 查看设备在线状态 |

## Web 终端

| 方法 | 路径 | 所需权限 | 说明 |
| --- | --- | --- | --- |
| GET | `/api/webterminal/logs` | `webterminal:read` | 查看模块运行日志 |
| DELETE | `/api/webterminal/logs` | `webterminal:write` | 清空模块运行日志 |
| GET | `/api/webterminal/connections` | `webterminal:read` | 获取终端连接列表 |
| POST | `/api/webterminal/connections` | `webterminal:write` | 新增终端连接 |
| PUT | `/api/webterminal/connections/{key}` | `webterminal:write` | 修改终端连接 |
| DELETE | `/api/webterminal/connections/{key}` | `webterminal:write` | 删除终端连接 |
| PUT | `/api/webterminal/connections/{key}/enable` | `webterminal:write` | 启用或停用终端连接 |
| PUT | `/api/webterminal/connections-order` | `webterminal:write` | 调整终端连接排序 |
| PUT | `/api/webterminal/connections-group` | `webterminal:write` | 批量设置终端连接分组 |
| GET | `/api/webterminal/connections/{key}/secrets` | `webterminal:write` | 查看密钥明文 |
| DELETE | `/api/webterminal/connections/{key}/hostkey` | `webterminal:write` | 清除已记录的主机密钥 |
| POST | `/api/webterminal/connections/{key}/test` | `webterminal:write` | 测试已保存连接 |
| POST | `/api/webterminal/connections/test` | `webterminal:write` | 测试临时连接参数 |
| PUT | `/api/webterminal/connections/{key}/quickaccess` | `webterminal:write` | 设置连接的快捷入口 |
| GET | `/api/webterminal/credentials` | `webterminal:read` | 获取条目列表 |
| POST | `/api/webterminal/credentials` | `webterminal:write` | 新增条目 |
| PUT | `/api/webterminal/credentials/{key}` | `webterminal:write` | 修改条目 |
| DELETE | `/api/webterminal/credentials/{key}` | `webterminal:write` | 删除条目 |
| PUT | `/api/webterminal/credentials/{key}/enable` | `webterminal:write` | 启用或停用条目 |
| PUT | `/api/webterminal/credentials-order` | `webterminal:write` | 调整条目排序 |
| GET | `/api/webterminal/credentials/{key}/secrets` | `webterminal:write` | 查看密钥明文 |
| GET | `/api/webterminal/connections/{key}/ws` | `webterminal:write` | 打开终端会话（WebSocket） |
| GET | `/api/webterminal/sessions/{id}/ws` | `webterminal:write` | 接入已有终端会话（WebSocket） |
| GET | `/api/webterminal/sessions` | `webterminal:read` | 列出终端会话 |
| GET | `/api/webterminal/sessions/{id}` | `webterminal:read` | 查看终端会话详情 |
| PUT | `/api/webterminal/sessions/{id}/remark` | `webterminal:write` | 修改终端会话备注 |
| GET | `/api/webterminal/sessions/{id}/stats` | `webterminal:read` | 查看终端会话统计 |
| DELETE | `/api/webterminal/sessions/{id}` | `webterminal:write` | 关闭终端会话 |
| GET | `/api/webterminal/security` | 仅登录会话 | 查看终端安全设置状态 |
| POST | `/api/webterminal/security/agreement` | 仅登录会话 | 确认终端使用协议 |
| POST | `/api/webterminal/security/2fa/setup` | 仅登录会话 | 生成终端两步验证密钥 |
| POST | `/api/webterminal/security/2fa/enable` | 仅登录会话 | 启用终端两步验证 |
| POST | `/api/webterminal/security/2fa/disable` | 仅登录会话 | 关闭终端两步验证 |
| POST | `/api/webterminal/security/verify2fa` | 仅登录会话 | 校验终端两步验证码 |
| GET | `/api/webterminal/security/check2fa` | 仅登录会话 | 检查终端两步验证是否已通过 |
| GET | `/api/webterminal/sftp/{key}/list` | `webterminal:write` | 列出远程目录 |
| GET | `/api/webterminal/sftp/{key}/read` | `webterminal:write` | 读取远程文件 |
| POST | `/api/webterminal/sftp/{key}/write` | `webterminal:write` | 写入远程文件 |
| POST | `/api/webterminal/sftp/{key}/upload` | `webterminal:write` | 上传文件到远程主机 |
| POST | `/api/webterminal/sftp/{key}/mkdir` | `webterminal:write` | 创建远程目录 |
| POST | `/api/webterminal/sftp/{key}/touch` | `webterminal:write` | 创建远程空文件 |
| POST | `/api/webterminal/sftp/{key}/rename` | `webterminal:write` | 重命名或移动远程文件 |
| POST | `/api/webterminal/sftp/{key}/copy` | `webterminal:write` | 复制远程文件 |
| POST | `/api/webterminal/sftp/{key}/chmod` | `webterminal:write` | 修改远程文件权限 |
| DELETE | `/api/webterminal/sftp/{key}/remove` | `webterminal:write` | 删除远程文件 |
| POST | `/api/webterminal/sftp/{key}/compress` | `webterminal:write` | 在远程主机压缩文件 |
| POST | `/api/webterminal/sftp/{key}/decompress` | `webterminal:write` | 在远程主机解压文件 |
| GET | `/api/webterminal/sftp/{key}/preview-archive` | `webterminal:write` | 预览远程压缩包内容 |
| GET | `/api/webterminal/settings` | `webterminal:read` | 读取 Web 终端设置 |
| PUT | `/api/webterminal/settings` | `webterminal:write` | 保存 Web 终端设置 |
| GET | `/api/webterminal/shells` | `webterminal:read` | 列出本机可用 Shell |
| GET | `/api/webterminal/globalshortcuts` | `webterminal:read` | 读取全局快捷命令 |
| PUT | `/api/webterminal/globalshortcuts` | `webterminal:write` | 保存全局快捷命令 |
| GET | `/api/webterminal/splitlayout` | `webterminal:read` | 读取分屏布局 |
| PUT | `/api/webterminal/splitlayout` | `webterminal:write` | 保存分屏布局 |
| DELETE | `/api/webterminal/splitlayout` | `webterminal:write` | 清除分屏布局 |

## 计划任务

| 方法 | 路径 | 所需权限 | 说明 |
| --- | --- | --- | --- |
| GET | `/api/cron/logs` | `cron:read` | 查看模块运行日志 |
| DELETE | `/api/cron/logs` | `cron:write` | 清空模块运行日志 |
| GET | `/api/cron/tasks` | `cron:read` | 获取计划任务列表 |
| POST | `/api/cron/tasks` | `cron:write` | 新增计划任务 |
| PUT | `/api/cron/tasks/{key}` | `cron:write` | 修改计划任务 |
| DELETE | `/api/cron/tasks/{key}` | `cron:write` | 删除计划任务 |
| PUT | `/api/cron/tasks/{key}/enable` | `cron:write` | 启用或停用计划任务 |
| PUT | `/api/cron/tasks-order` | `cron:write` | 调整计划任务排序 |
| PUT | `/api/cron/tasks-group` | `cron:write` | 批量设置计划任务分组 |
| POST | `/api/cron/tasks/{key}/run` | `cron:write` | 立即执行一个计划任务 |
| POST | `/api/cron/parse` | `cron:write` | 解析并预览 Cron 表达式 |

## Docker 管理

| 方法 | 路径 | 所需权限 | 说明 |
| --- | --- | --- | --- |
| GET | `/api/docker/logs` | `docker:read` | 查看模块运行日志 |
| DELETE | `/api/docker/logs` | `docker:write` | 清空模块运行日志 |
| GET | `/api/docker/tasks` | `docker:read` | 列出后台任务 |
| GET | `/api/docker/tasks/{id}` | `docker:read` | 查看后台任务进度与输出 |
| DELETE | `/api/docker/tasks` | `docker:write` | 清理已结束的后台任务 |
| GET | `/api/docker/images` | `docker:read` | 列出镜像 |
| POST | `/api/docker/images/pull` | `docker:write` | 拉取镜像 |
| POST | `/api/docker/images/push` | `docker:write` | 推送镜像 |
| POST | `/api/docker/images/search` | `docker:write` | 搜索镜像 |
| POST | `/api/docker/images/load` | `docker:write` | 导入镜像文件 |
| POST | `/api/docker/images/build` | `docker:write` | 构建镜像 |
| GET | `/api/docker/images/{id}` | `docker:read` | 查看镜像详情 |
| POST | `/api/docker/images/{id}/tag` | `docker:write` | 为镜像打标签 |
| DELETE | `/api/docker/images/{id}` | `docker:write` | 删除镜像 |
| GET | `/api/docker/images/{id}/export` | `docker:write` | 导出镜像 |
| GET | `/api/docker/networks` | `docker:read` | 列出网络 |
| POST | `/api/docker/networks` | `docker:write` | 创建网络 |
| DELETE | `/api/docker/networks/{id}` | `docker:write` | 删除网络 |
| POST | `/api/docker/networks/{id}/connect` | `docker:write` | 把容器接入网络 |
| POST | `/api/docker/networks/{id}/disconnect` | `docker:write` | 把容器从网络断开 |
| GET | `/api/docker/volumes` | `docker:read` | 列出数据卷 |
| POST | `/api/docker/volumes` | `docker:write` | 创建数据卷 |
| DELETE | `/api/docker/volumes/{name}` | `docker:write` | 删除数据卷 |
| GET | `/api/docker/volumes/{name}/export` | `docker:write` | 导出数据卷 |
| POST | `/api/docker/volumes/{name}/import` | `docker:write` | 向数据卷导入数据 |
| GET | `/api/docker/compose/projects` | `docker:read` | 列出 Compose 项目 |
| POST | `/api/docker/compose/projects` | `docker:write` | 创建 Compose 项目 |
| POST | `/api/docker/compose/{project}/up` | `docker:write` | 启动 Compose 项目 |
| POST | `/api/docker/compose/{project}/pull` | `docker:write` | 拉取 Compose 项目镜像 |
| POST | `/api/docker/compose/{project}/{action}` | `docker:write` | 对 Compose 项目执行操作 |
| GET | `/api/docker/compose/{project}/config` | `docker:write` | 读取 Compose 配置文件 |
| PUT | `/api/docker/compose/{project}/config` | `docker:write` | 保存 Compose 配置文件 |
| GET | `/api/docker/compose/{project}/logs` | `docker:read` | 查看 Compose 项目日志 |
| DELETE | `/api/docker/compose/{project}` | `docker:write` | 删除 Compose 项目 |
| GET | `/api/docker/info` | `docker:read` | 查看 Docker 引擎信息 |
| GET | `/api/docker/disk-usage` | `docker:read` | 查看 Docker 磁盘占用 |
| POST | `/api/docker/prune` | `docker:write` | 清理无用的 Docker 资源 |
| GET | `/api/docker/settings` | `docker:read` | 读取 Docker 模块设置 |
| PUT | `/api/docker/settings` | `docker:write` | 保存 Docker 模块设置 |
| GET | `/api/docker/settings/secrets` | `docker:write` | 查看仓库密码明文（键为 `registry:<仓库地址>`） |
| GET | `/api/docker/containers` | `docker:read` | 列出容器 |
| POST | `/api/docker/containers` | `docker:write` | 创建容器 |
| POST | `/api/docker/containers/check-all` | `docker:write` | 检查全部容器镜像更新 |
| POST | `/api/docker/containers/upgrade-batch` | `docker:write` | 批量升级容器 |
| POST | `/api/docker/images/upgrade-check` | `docker:write` | 检查镜像是否有新版本 |
| GET | `/api/docker/containers/{id}` | `docker:read` | 查看容器详情 |
| GET | `/api/docker/containers/{id}/secrets` | `docker:write` | 查看容器环境变量与启动命令明文 |
| DELETE | `/api/docker/containers/{id}` | `docker:write` | 删除容器 |
| POST | `/api/docker/containers/{id}/edit` | `docker:write` | 修改容器配置并重建 |
| POST | `/api/docker/containers/{id}/rename` | `docker:write` | 重命名容器 |
| POST | `/api/docker/containers/{id}/commit` | `docker:write` | 把容器提交为镜像 |
| POST | `/api/docker/containers/{id}/check` | `docker:write` | 检查容器镜像是否有更新 |
| POST | `/api/docker/containers/{id}/upgrade` | `docker:write` | 升级容器镜像并重建 |
| POST | `/api/docker/containers/{id}/{action}` | `docker:write` | 对容器执行操作 |
| GET | `/api/docker/containers/{id}/logs` | `docker:read` | 查看容器日志 |
| GET | `/api/docker/containers/{id}/stats` | `docker:read` | 查看容器资源占用 |
| GET | `/api/docker/containers/{id}/processes` | `docker:read` | 查看容器内进程 |
| GET | `/api/docker/containers/{id}/files/list` | `docker:write` | 列出容器内目录 |
| GET | `/api/docker/containers/{id}/files/read` | `docker:write` | 读取容器内文件 |
| GET | `/api/docker/containers/{id}/files/download` | `docker:write` | 下载容器内文件 |
| POST | `/api/docker/containers/{id}/files/write` | `docker:write` | 写入容器内文件 |
| POST | `/api/docker/containers/{id}/files/mkdir` | `docker:write` | 在容器内创建目录 |
| POST | `/api/docker/containers/{id}/files/rename` | `docker:write` | 重命名容器内文件 |
| POST | `/api/docker/containers/{id}/files/upload` | `docker:write` | 上传文件到容器 |
| DELETE | `/api/docker/containers/{id}/files` | `docker:write` | 删除容器内文件 |
| GET | `/api/docker/containers/{id}/exec` | `docker:write` | 在容器内打开终端（WebSocket） |

## WebDAV 服务

| 方法 | 路径 | 所需权限 | 说明 |
| --- | --- | --- | --- |
| GET | `/api/webdav/logs` | `webdav:read` | 查看模块运行日志 |
| DELETE | `/api/webdav/logs` | `webdav:write` | 清空模块运行日志 |
| GET | `/api/webdav/settings` | `webdav:read` | 读取 WebDAV 设置 |
| GET | `/api/webdav/settings/secrets` | `webdav:write` | 查看本地用户密码明文 |
| GET | `/api/webdav/shared-users` | `webdav:read` | 读取文件服务共享用户（旧接口） |
| PUT | `/api/webdav/shared-users` | `webdav:write,ftp:write,smb:write` | 保存文件服务共享用户（旧接口） |
| PUT | `/api/webdav/settings` | `webdav:write` | 保存 WebDAV 设置 |

## FTP 服务

| 方法 | 路径 | 所需权限 | 说明 |
| --- | --- | --- | --- |
| GET | `/api/ftp/logs` | `ftp:read` | 查看模块运行日志 |
| DELETE | `/api/ftp/logs` | `ftp:write` | 清空模块运行日志 |
| GET | `/api/ftp/settings` | `ftp:read` | 读取 FTP 设置 |
| GET | `/api/ftp/settings/secrets` | `ftp:write` | 查看本地用户密码明文 |
| GET | `/api/ftp/shared-users` | `ftp:read` | 读取文件服务共享用户（旧接口） |
| PUT | `/api/ftp/shared-users` | `webdav:write,ftp:write,smb:write` | 保存文件服务共享用户（旧接口） |
| PUT | `/api/ftp/settings` | `ftp:write` | 保存 FTP 设置 |

## SMB 服务

| 方法 | 路径 | 所需权限 | 说明 |
| --- | --- | --- | --- |
| GET | `/api/smb/logs` | `smb:read` | 查看模块运行日志 |
| DELETE | `/api/smb/logs` | `smb:write` | 清空模块运行日志 |
| GET | `/api/smb/settings` | `smb:read` | 读取 SMB 设置 |
| GET | `/api/smb/settings/secrets` | `smb:write` | 查看本地用户密码明文 |
| GET | `/api/smb/shared-users` | `smb:read` | 读取文件服务共享用户（旧接口） |
| PUT | `/api/smb/shared-users` | `webdav:write,ftp:write,smb:write` | 保存文件服务共享用户（旧接口） |
| PUT | `/api/smb/settings` | `smb:write` | 保存 SMB 设置 |

## File Browser

| 方法 | 路径 | 所需权限 | 说明 |
| --- | --- | --- | --- |
| GET | `/api/filebrowser/logs` | `filebrowser:read` | 查看模块运行日志 |
| DELETE | `/api/filebrowser/logs` | `filebrowser:write` | 清空模块运行日志 |
| GET | `/api/filebrowser/instances` | `filebrowser:read` | 获取File Browser 实例列表 |
| POST | `/api/filebrowser/instances` | `filebrowser:write` | 新增File Browser 实例 |
| PUT | `/api/filebrowser/instances/{key}` | `filebrowser:write` | 修改File Browser 实例 |
| DELETE | `/api/filebrowser/instances/{key}` | `filebrowser:write` | 删除File Browser 实例 |
| PUT | `/api/filebrowser/instances/{key}/enable` | `filebrowser:write` | 启用或停用File Browser 实例 |
| PUT | `/api/filebrowser/instances-order` | `filebrowser:write` | 调整File Browser 实例排序 |
| PUT | `/api/filebrowser/instances-group` | `filebrowser:write` | 批量设置File Browser 实例分组 |
| GET | `/api/filebrowser/instances/{key}/secrets` | `filebrowser:write` | 查看密钥明文 |
| POST | `/api/filebrowser/instances/{key}/restart` | `filebrowser:write` | 重启一个 File Browser 实例 |
| GET | `/api/filebrowser/settings` | `filebrowser:read` | 读取 File Browser 程序设置 |
| PUT | `/api/filebrowser/settings` | `filebrowser:write` | 保存 File Browser 程序设置 |
| GET | `/api/filebrowser/install` | `filebrowser:read` | 查看托管程序的安装信息与进度 |
| POST | `/api/filebrowser/install` | `filebrowser:write` | 在线安装或更新托管程序 |
| POST | `/api/filebrowser/install/rollback` | `filebrowser:write` | 切换到保存的旧版本 |
| DELETE | `/api/filebrowser/install` | `filebrowser:write` | 卸载托管程序 |
| DELETE | `/api/filebrowser/install/backups/{id}` | `filebrowser:write` | 删除保存的旧版本 |
| POST | `/api/filebrowser/reset` | `filebrowser:write` | 恢复模块默认配置 |

## 存储挂载

| 方法 | 路径 | 所需权限 | 说明 |
| --- | --- | --- | --- |
| GET | `/api/storage/logs` | `storage:read` | 查看模块运行日志 |
| DELETE | `/api/storage/logs` | `storage:write` | 清空模块运行日志 |
| GET | `/api/storage/mounts` | `storage:read` | 获取存储挂载列表 |
| POST | `/api/storage/mounts` | `storage:write` | 新增存储挂载 |
| PUT | `/api/storage/mounts/{key}` | `storage:write` | 修改存储挂载 |
| DELETE | `/api/storage/mounts/{key}` | `storage:write` | 删除存储挂载 |
| PUT | `/api/storage/mounts/{key}/enable` | `storage:write` | 启用或停用存储挂载 |
| PUT | `/api/storage/mounts-order` | `storage:write` | 调整存储挂载排序 |
| PUT | `/api/storage/mounts-group` | `storage:write` | 批量设置存储挂载分组 |
| GET | `/api/storage/mounts/{key}/secrets` | `storage:write` | 查看密钥明文 |
| GET | `/api/storage/files/{key}/list` | `storage:write` | 列出存储目录 |
| GET | `/api/storage/files/{key}/download` | `storage:write` | 下载存储文件 |
| POST | `/api/storage/files/{key}/upload` | `storage:write` | 上传文件到存储 |
| POST | `/api/storage/files/{key}/mkdir` | `storage:write` | 创建存储目录 |
| POST | `/api/storage/files/{key}/rename` | `storage:write` | 重命名或移动存储文件 |
| DELETE | `/api/storage/files/{key}` | `storage:write` | 删除存储文件 |

## 通知中心

| 方法 | 路径 | 所需权限 | 说明 |
| --- | --- | --- | --- |
| GET | `/api/notify/logs` | `notify:read` | 查看模块运行日志 |
| DELETE | `/api/notify/logs` | `notify:write` | 清空模块运行日志 |
| GET | `/api/notify/channels` | `notify:read` | 获取通知渠道列表 |
| POST | `/api/notify/channels` | `notify:write` | 新增通知渠道 |
| PUT | `/api/notify/channels/{key}` | `notify:write` | 修改通知渠道 |
| DELETE | `/api/notify/channels/{key}` | `notify:write` | 删除通知渠道 |
| PUT | `/api/notify/channels/{key}/enable` | `notify:write` | 启用或停用通知渠道 |
| PUT | `/api/notify/channels-order` | `notify:write` | 调整通知渠道排序 |
| PUT | `/api/notify/channels-group` | `notify:write` | 批量设置通知渠道分组 |
| GET | `/api/notify/channels/{key}/secrets` | `notify:write` | 查看密钥明文 |
| GET | `/api/notify/rules` | `notify:read` | 获取通知规则列表 |
| POST | `/api/notify/rules` | `notify:write` | 新增通知规则 |
| PUT | `/api/notify/rules/{key}` | `notify:write` | 修改通知规则 |
| DELETE | `/api/notify/rules/{key}` | `notify:write` | 删除通知规则 |
| PUT | `/api/notify/rules/{key}/enable` | `notify:write` | 启用或停用通知规则 |
| PUT | `/api/notify/rules-order` | `notify:write` | 调整通知规则排序 |
| PUT | `/api/notify/rules-group` | `notify:write` | 批量设置通知规则分组 |
| GET | `/api/notify/event-types` | `notify:read` | 可订阅的事件类型 |
| GET | `/api/notify/history` | `notify:read` | 最近的投递记录 |
| POST | `/api/notify/channels/{key}/test` | `notify:write` | 向渠道发送测试通知 |

## 网络诊断

| 方法 | 路径 | 所需权限 | 说明 |
| --- | --- | --- | --- |
| GET | `/api/nettools/logs` | `nettools:read` | 查看模块运行日志 |
| DELETE | `/api/nettools/logs` | `nettools:write` | 清空模块运行日志 |
| POST | `/api/nettools/run` | `nettools:write` | 运行诊断工具，返回任务（tool: ping / traceroute / tcp / dns / speed / scan） |
| GET | `/api/nettools/tasks` | `nettools:read` | 最近的诊断任务 |
| GET | `/api/nettools/tasks/{id}` | `nettools:read` | 任务详情与输出 |
| DELETE | `/api/nettools/tasks/{id}` | `nettools:write` | 取消运行中的任务，或删除已结束的任务 |
| GET | `/api/nettools/settings` | `nettools:read` | 诊断设置 |
| PUT | `/api/nettools/settings` | `nettools:write` | 修改禁止探测列表 |
| GET | `/api/nettools/networks` | `nettools:read` | 本机可用于局域网扫描的网段 |

## 配置备份

| 方法 | 路径 | 所需权限 | 说明 |
| --- | --- | --- | --- |
| GET | `/api/backup/logs` | `backup:read` | 查看模块运行日志 |
| DELETE | `/api/backup/logs` | `backup:write` | 清空模块运行日志 |
| GET | `/api/backup/targets` | `backup:read` | 获取备份目的地列表 |
| POST | `/api/backup/targets` | `backup:write` | 新增备份目的地 |
| PUT | `/api/backup/targets/{key}` | `backup:write` | 修改备份目的地 |
| DELETE | `/api/backup/targets/{key}` | `backup:write` | 删除备份目的地 |
| PUT | `/api/backup/targets/{key}/enable` | `backup:write` | 启用或停用备份目的地 |
| PUT | `/api/backup/targets-order` | `backup:write` | 调整备份目的地排序 |
| GET | `/api/backup/settings` | `backup:read` | 备份设置 |
| PUT | `/api/backup/settings` | `backup:write` | 修改备份设置（口令传占位符表示保持不变） |
| GET | `/api/backup/secrets` | `backup:write` | 查看密钥明文 |
| POST | `/api/backup/run` | `backup:write` | 立即备份一次 |
| GET | `/api/backup/status` | `backup:read` | 最近一次备份结果与下次执行时间 |
| GET | `/api/backup/mounts` | `backup:read` | 可作为备份目的地的存储挂载点 |
| GET | `/api/backup/local` | 仅登录会话 | 本地备份文件列表 |
| GET | `/api/backup/local/{name}` | 仅登录会话 | 下载本地备份文件 |

## 文件服务共享用户

| 方法 | 路径 | 所需权限 | 说明 |
| --- | --- | --- | --- |
| GET | `/api/fileusers` | `fileusers:read` | 读取文件服务共享用户 |
| PUT | `/api/fileusers` | `fileusers:write` | 保存文件服务共享用户 |
| GET | `/api/fileusers/secrets` | `fileusers:write` | 查看共享用户密码明文 |

## 系统

| 方法 | 路径 | 所需权限 | 说明 |
| --- | --- | --- | --- |
| GET | `/api/session` | 任意有效令牌 | 查看当前认证身份 |
| GET | `/api/status` | `status:read` | 查看系统与各模块状态 |
| GET | `/api/logs` | `logs:read` | 查看系统日志 |
| GET | `/api/system/settings` | 仅登录会话 | 读取全局设置 |
| PUT | `/api/system/settings` | 仅登录会话 | 保存全局设置 |
| GET | `/api/system/settings/secrets` | 仅登录会话 | 查看第三方登录 Client Secret 明文 |
| GET | `/api/system/info` | `status:read` | 查看系统信息 |
| GET | `/api/system/update` | `status:read` | 检查新版本 |
| POST | `/api/system/upgrade` | 仅登录会话 | 在线升级到最新版本 |
| GET | `/api/system/upgrade/status` | 仅登录会话 | 查看在线升级进度 |
| POST | `/api/system/upgrade/cancel` | 仅登录会话 | 取消在线升级 |
| POST | `/api/system/upgrade/upload` | 仅登录会话 | 上传升级包升级 |
| GET | `/api/system/backup` | 仅登录会话 | 下载配置备份 |
| GET | `/api/system/diagnostics` | 仅登录会话 | 导出脱敏诊断包 |
| GET | `/api/audit` | 仅登录会话 | 查询操作审计日志 |
| GET | `/api/metrics` | `metrics:read` | Prometheus 全局指标 |
| GET | `/api/history` | 仅登录会话 | 列出配置历史版本 |
| GET | `/api/history/{name}/{id}/diff` | 仅登录会话 | 比较历史版本与当前配置 |
| POST | `/api/history/{name}/{id}/restore` | 仅登录会话 | 回滚配置到历史版本 |
| POST | `/api/system/restore` | 仅登录会话 | 从备份恢复配置 |
| POST | `/api/system/restart` | 仅登录会话 | 重启 Gately |
| POST | `/api/system/selfcheck` | 仅登录会话 | 网络与配置自检 |
| GET | `/api/system/onboarding` | 仅登录会话 | 是否需要首次启动引导 |
| POST | `/api/system/onboarding/dismiss` | 仅登录会话 | 完成或跳过首次启动引导 |
| GET | `/api/openapi.json` | 任意有效令牌 | 获取本 API 的 OpenAPI 描述 |

## 配置导入

| 方法 | 路径 | 所需权限 | 说明 |
| --- | --- | --- | --- |
| POST | `/api/import/lucky` | `import:write` | 导入 Lucky 配置 |

## API 令牌

| 方法 | 路径 | 所需权限 | 说明 |
| --- | --- | --- | --- |
| GET | `/api/tokens` | 仅登录会话 | 列出 API 令牌 |
| GET | `/api/tokens/scopes` | 仅登录会话 | 列出可授权的权限域 |
| POST | `/api/tokens` | 仅登录会话 | 创建 API 令牌 |
| PUT | `/api/tokens/order` | 仅登录会话 | 调整 API 令牌的排序 |
| PUT | `/api/tokens/group` | 仅登录会话 | 批量设置 API 令牌的分组 |
| PUT | `/api/tokens/{id}` | 仅登录会话 | 修改 API 令牌的名称、分组、权限、过期时间与启停 |
| POST | `/api/tokens/{id}/regenerate` | 仅登录会话 | 重新生成令牌明文 |
| DELETE | `/api/tokens/{id}` | 仅登录会话 | 删除 API 令牌 |

## 账号

| 方法 | 路径 | 所需权限 | 说明 |
| --- | --- | --- | --- |
| PUT | `/api/account` | 仅登录会话 | 修改管理员用户名与密码 |
| PUT | `/api/account/ui-language` | 仅登录会话 | 记录管理界面语言 |
| GET | `/api/account/passkeys` | 仅登录会话 | 列出已注册的 Passkey |
| POST | `/api/account/passkeys/begin` | 仅登录会话 | 开始注册 Passkey |
| POST | `/api/account/passkeys` | 仅登录会话 | 完成注册 Passkey |
| DELETE | `/api/account/passkeys/{id}` | 仅登录会话 | 删除 Passkey |
| GET | `/api/account/users` | 仅登录会话 | 列出只读账号 |
| POST | `/api/account/users` | 仅登录会话 | 创建只读账号 |
| PUT | `/api/account/users/{name}` | 仅登录会话 | 重置只读账号密码 |
| DELETE | `/api/account/users/{name}` | 仅登录会话 | 删除只读账号 |
| GET | `/api/account/totp` | 仅登录会话 | 查看两步验证状态 |
| POST | `/api/account/totp/setup` | 仅登录会话 | 生成两步验证密钥 |
| POST | `/api/account/totp/enable` | 仅登录会话 | 启用两步验证 |
| POST | `/api/account/totp/disable` | 仅登录会话 | 关闭两步验证 |

## 数据结构

### backup.LocalFile

| 字段 | 类型 |
| --- | --- |
| `modTime` | 时间 (RFC 3339) |
| `name` | string |
| `size` | integer |

### backup.Result

| 字段 | 类型 |
| --- | --- |
| `error` | string |
| `file` | string |
| `ok` | boolean |
| `pruned` | integer |
| `target` | string |

### backup.Settings

| 字段 | 类型 |
| --- | --- |
| `enable` | boolean |
| `keep` | integer |
| `localEnable` | boolean |
| `password` | string |
| `schedule` | string |

### backup.Status

| 字段 | 类型 |
| --- | --- |
| `lastRun` | 时间 (RFC 3339) |
| `manual` | boolean |
| `nextRun` | 时间 (RFC 3339) |
| `results` | 数组 of `backup.Result` |
| `running` | boolean |

### backup.Target

| 字段 | 类型 |
| --- | --- |
| `dir` | string |
| `enable` | boolean |
| `key` | string |
| `mount` | string |
| `name` | string |

### backup.settingsView

| 字段 | 类型 |
| --- | --- |
| `enable` | boolean |
| `hasPassword` | boolean |
| `keep` | integer |
| `localEnable` | boolean |
| `password` | string |
| `schedule` | string |

### banlist.Entry

| 字段 | 类型 |
| --- | --- |
| `count` | integer |
| `created` | 时间 (RFC 3339) |
| `key` | string |
| `reason` | string |
| `source` | string |
| `until` | 时间 (RFC 3339) |

### cloudflared.Tunnel

| 字段 | 类型 |
| --- | --- |
| `apiToken` | string |
| `edgeIpVersion` | string |
| `enable` | boolean |
| `group` | string |
| `key` | string |
| `mode` | string |
| `name` | string |
| `protocol` | string |
| `remark` | string |
| `service` | string |
| `token` | string |

### cron.Task

| 字段 | 类型 |
| --- | --- |
| `command` | string |
| `enable` | boolean |
| `group` | string |
| `http` | `webhook.Config` |
| `key` | string |
| `name` | string |
| `remark` | string |
| `spec` | string |
| `type` | string |

### crud.enableRequest

| 字段 | 类型 |
| --- | --- |
| `enable` | boolean |

### crud.groupRequest

| 字段 | 类型 |
| --- | --- |
| `group` | string |
| `keys` | 数组 of string |

### crud.orderRequest

| 字段 | 类型 |
| --- | --- |
| `keys` | 数组 of string |

### ddns.Record

| 字段 | 类型 |
| --- | --- |
| `enable` | boolean |
| `group` | string |
| `key` | string |
| `line` | string |
| `remark` | string |
| `sub` | string |
| `ttl` | integer |
| `type` | string |
| `value` | string |
| `zone` | string |

### ddns.Task

| 字段 | 类型 |
| --- | --- |
| `credentials` | `dnsprovider.Credentials` |
| `enable` | boolean |
| `forceInterval` | integer |
| `group` | string |
| `httpTimeout` | integer |
| `interval` | integer |
| `ipv4` | `ipsource.Source` |
| `ipv6` | `ipsource.Source` |
| `key` | string |
| `name` | string |
| `provider` | string |
| `records` | 数组 of `ddns.Record` |
| `remark` | string |
| `webhook` | `webhook.Config` |

### ddns.recordLayout

| 字段 | 类型 |
| --- | --- |
| `group` | string |
| `key` | string |

### ddns.recordsLayoutRequest

| 字段 | 类型 |
| --- | --- |
| `records` | 数组 of `ddns.recordLayout` |

### dnsprovider.Credentials

| 字段 | 类型 |
| --- | --- |
| `extra` | 映射 of string |
| `id` | string |
| `secret` | string |

### events.TypeInfo

| 字段 | 类型 |
| --- | --- |
| `level` | string |
| `name` | string |
| `type` | string |

### filebrowser.Instance

| 字段 | 类型 |
| --- | --- |
| `accessUrls` | 数组 of string |
| `adminPassword` | string |
| `adminUser` | string |
| `authMethod` | string |
| `baseUrl` | string |
| `disableWebDav` | boolean |
| `enable` | boolean |
| `group` | string |
| `key` | string |
| `listenIp` | string |
| `locale` | string |
| `name` | string |
| `port` | integer |
| `proxyHeader` | string |
| `remark` | string |
| `sources` | 数组 of `filebrowser.Source` |
| `tokenHours` | integer |
| `trustProxyHeaders` | boolean |

### filebrowser.Source

| 字段 | 类型 |
| --- | --- |
| `name` | string |
| `path` | string |
| `readOnly` | boolean |

### frp.BinaryView

| 字段 | 类型 |
| --- | --- |
| `binary` | string |
| `error` | string |
| `version` | string |

### frp.Client

| 字段 | 类型 |
| --- | --- |
| `enable` | boolean |
| `group` | string |
| `key` | string |
| `name` | string |
| `protocol` | string |
| `remark` | string |
| `serverAddr` | string |
| `serverPort` | integer |
| `token` | string |
| `user` | string |

### frp.ClientSettings

| 字段 | 类型 |
| --- | --- |
| `protocol` | string |
| `serverAddr` | string |
| `serverPort` | integer |
| `token` | string |
| `user` | string |

### frp.ImportRequest

| 字段 | 类型 |
| --- | --- |
| `apply` | boolean |
| `content` | string |
| `name` | string |
| `proxyMode` | string |
| `role` | string |
| `target` | string |

### frp.ImportResponse

| 字段 | 类型 |
| --- | --- |
| `added` | integer |
| `applied` | boolean |
| `client` | `frp.ClientSettings` |
| `key` | string |
| `proxies` | 数组 of `frp.Proxy` |
| `role` | string |
| `server` | `frp.ServerSettings` |
| `warnings` | 数组 of string |

### frp.Proxy

| 字段 | 类型 |
| --- | --- |
| `client` | string |
| `customDomains` | 数组 of string |
| `enable` | boolean |
| `group` | string |
| `key` | string |
| `localIp` | string |
| `localPort` | integer |
| `name` | string |
| `remark` | string |
| `remotePort` | integer |
| `secretKey` | string |
| `subdomain` | string |
| `type` | string |

### frp.Server

| 字段 | 类型 |
| --- | --- |
| `allowPorts` | string |
| `bindAddr` | string |
| `bindPort` | integer |
| `dashboardAddr` | string |
| `dashboardPort` | integer |
| `dashboardPwd` | string |
| `dashboardUser` | string |
| `enable` | boolean |
| `group` | string |
| `key` | string |
| `name` | string |
| `proxyBindAddr` | string |
| `remark` | string |
| `subdomainHost` | string |
| `token` | string |
| `vhostHttpPort` | integer |
| `vhostHttpsPort` | integer |

### frp.ServerSettings

| 字段 | 类型 |
| --- | --- |
| `allowPorts` | string |
| `bindAddr` | string |
| `bindPort` | integer |
| `dashboardAddr` | string |
| `dashboardPort` | integer |
| `dashboardPwd` | string |
| `dashboardUser` | string |
| `proxyBindAddr` | string |
| `subdomainHost` | string |
| `token` | string |
| `vhostHttpPort` | integer |
| `vhostHttpsPort` | integer |

### frp.Settings

| 字段 | 类型 |
| --- | --- |
| `frpcPath` | string |
| `frpsPath` | string |

### frp.SettingsView

| 字段 | 类型 |
| --- | --- |
| `frpc` | `frp.BinaryView` |
| `frps` | `frp.BinaryView` |
| `settings` | `frp.Settings` |

### geoip.Info

| 字段 | 类型 |
| --- | --- |
| `buildTime` | 时间 (RFC 3339) |
| `loaded` | boolean |
| `size` | integer |
| `type` | string |

### guard.BanView

| 字段 | 类型 |
| --- | --- |
| `count` | integer |
| `created` | 时间 (RFC 3339) |
| `jail` | string |
| `key` | string |
| `reason` | string |
| `source` | string |
| `until` | 时间 (RFC 3339) |

### guard.FirewallStatus

| 字段 | 类型 |
| --- | --- |
| `active` | boolean |
| `checkedAt` | 时间 (RFC 3339) |
| `dropped` | 映射 of integer |
| `enabled` | boolean |
| `error` | string |
| `hits` | 映射 of integer |
| `openPorts` | integer |
| `reason` | string |
| `trapPorts` | string |
| `trapSkipped` | 数组 of integer |
| `version` | string |

### guard.Jail

| 字段 | 类型 |
| --- | --- |
| `anyStatus` | boolean |
| `banTime` | integer |
| `enable` | boolean |
| `filter` | string |
| `findTime` | integer |
| `group` | string |
| `ignore` | string |
| `key` | string |
| `maxRetry` | integer |
| `name` | string |
| `preset` | string |
| `remark` | string |
| `source` | string |
| `target` | string |
| `unknownHost` | boolean |

### guard.Platform

| 字段 | 类型 |
| --- | --- |
| `docker` | boolean |
| `inDocker` | boolean |
| `journal` | boolean |
| `os` | string |

### guard.ScanSettings

| 字段 | 类型 |
| --- | --- |
| `banSeconds` | integer |
| `enable` | boolean |
| `excludePorts` | string |
| `threshold` | integer |

### guard.Settings

| 字段 | 类型 |
| --- | --- |
| `banSeconds` | integer |
| `firewall` | boolean |
| `ignoreList` | 数组 of string |
| `ignorePrivate` | boolean |
| `increment` | boolean |
| `maxBanSeconds` | integer |
| `scan` | `guard.ScanSettings` |
| `trap` | `guard.TrapSettings` |

### guard.Status

| 字段 | 类型 |
| --- | --- |
| `bans` | integer |
| `firewall` | `guard.FirewallStatus` |
| `platform` | `guard.Platform` |
| `userTrapPorts` | 数组 of integer |

### guard.TestResult

| 字段 | 类型 |
| --- | --- |
| `host` | string |
| `ignored` | boolean |
| `line` | string |
| `matched` | boolean |

### guard.TrapSettings

| 字段 | 类型 |
| --- | --- |
| `banSeconds` | integer |
| `enable` | boolean |
| `ports` | string |

### guard.banRequest

| 字段 | 类型 |
| --- | --- |
| `key` | string |
| `reason` | string |
| `seconds` | integer |

### guard.testRequest

| 字段 | 类型 |
| --- | --- |
| `filter` | string |
| `ignore` | string |
| `lines` | string |
| `source` | string |

### ipsource.Source

| 字段 | 类型 |
| --- | --- |
| `command` | string |
| `enable` | boolean |
| `interface` | string |
| `regex` | string |
| `type` | string |
| `urls` | 数组 of string |

### nettools.Request

| 字段 | 类型 |
| --- | --- |
| `cidr` | string |
| `count` | integer |
| `ports` | string |
| `seconds` | integer |
| `server` | string |
| `target` | string |
| `tool` | string |
| `type` | string |
| `url` | string |

### nettools.Settings

| 字段 | 类型 |
| --- | --- |
| `deny` | 数组 of string |

### nettools.Task

| 字段 | 类型 |
| --- | --- |
| `error` | string |
| `finished` | 时间 (RFC 3339) |
| `id` | string |
| `lines` | 数组 of string |
| `result` | 任意 |
| `started` | 时间 (RFC 3339) |
| `status` | string |
| `title` | string |
| `tool` | string |

### notify.Channel

| 字段 | 类型 |
| --- | --- |
| `enable` | boolean |
| `from` | string |
| `group` | string |
| `host` | string |
| `key` | string |
| `kind` | string |
| `name` | string |
| `port` | integer |
| `remark` | string |
| `secret` | string |
| `security` | string |
| `target` | string |
| `url` | string |
| `user` | string |
| `webhook` | `webhook.Config` |

### notify.Record

| 字段 | 类型 |
| --- | --- |
| `channel` | string |
| `error` | string |
| `ok` | boolean |
| `rule` | string |
| `time` | 时间 (RFC 3339) |
| `title` | string |
| `type` | string |

### notify.Rule

| 字段 | 类型 |
| --- | --- |
| `channels` | 数组 of string |
| `enable` | boolean |
| `events` | 数组 of string |
| `group` | string |
| `key` | string |
| `minLevel` | string |
| `name` | string |
| `silence` | integer |

### portforward.Rule

| 字段 | 类型 |
| --- | --- |
| `enable` | boolean |
| `group` | string |
| `key` | string |
| `listenAddress` | string |
| `listenPorts` | string |
| `maxConnections` | integer |
| `mode` | string |
| `name` | string |
| `protocols` | 数组 of string |
| `proxyProtocol` | string |
| `remark` | string |
| `targetAddresses` | 数组 of string |
| `targetPorts` | string |
| `udpSessionTimeout` | integer |

### protoroute.Route

| 字段 | 类型 |
| --- | --- |
| `alpn` | 数组 of string |
| `domains` | 数组 of string |
| `name` | string |
| `protocol` | string |
| `proxyProtocol` | string |
| `targetAddresses` | 数组 of string |
| `targetPort` | integer |

### protoroute.Rule

| 字段 | 类型 |
| --- | --- |
| `enable` | boolean |
| `group` | string |
| `key` | string |
| `listenAddress` | string |
| `listenPorts` | string |
| `maxConnections` | integer |
| `name` | string |
| `protocols` | 数组 of string |
| `proxyProtocol` | string |
| `remark` | string |
| `routes` | 数组 of `protoroute.Route` |
| `sniffTimeout` | number |
| `targetAddresses` | 数组 of string |
| `targetPorts` | string |

### ssl.ACMEConfig

| 字段 | 类型 |
| --- | --- |
| `ca` | string |
| `challenge` | string |
| `credentials` | `dnsprovider.Credentials` |
| `directoryUrl` | string |
| `disableAuthoritativeCheck` | boolean |
| `domains` | 数组 of string |
| `eabHmac` | string |
| `eabKid` | string |
| `email` | string |
| `httpPort` | integer |
| `keyType` | string |
| `network` | string |
| `profile` | string |
| `propagationTimeout` | integer |
| `provider` | string |
| `resolvers` | 数组 of string |
| `skipPropagationCheck` | boolean |
| `zone` | string |

### ssl.Cert

| 字段 | 类型 |
| --- | --- |
| `acme` | `ssl.ACMEConfig` |
| `certFile` | string |
| `certPem` | string |
| `deploys` | 数组 of `ssl.Deploy` |
| `enable` | boolean |
| `group` | string |
| `issuedProfile` | string |
| `issuerPem` | string |
| `key` | string |
| `keyFile` | string |
| `keyPem` | string |
| `mapping` | `ssl.Mapping` |
| `remark` | string |
| `source` | string |
| `updatedAt` | 时间 (RFC 3339) |

### ssl.Deploy

| 字段 | 类型 |
| --- | --- |
| `certName` | string |
| `command` | string |
| `connection` | string |
| `containers` | 数组 of string |
| `deployedAt` | 时间 (RFC 3339) |
| `deployedHash` | string |
| `dir` | string |
| `enable` | boolean |
| `keyName` | string |
| `remark` | string |
| `type` | string |

### ssl.Mapping

| 字段 | 类型 |
| --- | --- |
| `dir` | string |
| `enable` | boolean |
| `script` | string |

### storage.Mount

| 字段 | 类型 |
| --- | --- |
| `domain` | string |
| `enable` | boolean |
| `group` | string |
| `key` | string |
| `name` | string |
| `password` | string |
| `path` | string |
| `readOnly` | boolean |
| `remark` | string |
| `skipVerify` | boolean |
| `type` | string |
| `url` | string |
| `username` | string |

### storage.MountInfo

| 字段 | 类型 |
| --- | --- |
| `enable` | boolean |
| `key` | string |
| `name` | string |
| `readOnly` | boolean |
| `type` | string |

### stun.Bind

| 字段 | 类型 |
| --- | --- |
| `interface` | string |
| `ip` | string |
| `mode` | string |
| `regex` | string |

### stun.Rule

| 字段 | 类型 |
| --- | --- |
| `autoRetry` | boolean |
| `bind` | `stun.Bind` |
| `disableCheck` | boolean |
| `disableForward` | boolean |
| `enable` | boolean |
| `group` | string |
| `heartbeatInterval` | integer |
| `keepAliveServers` | 数组 of string |
| `key` | string |
| `localPort` | integer |
| `maxConnections` | integer |
| `name` | string |
| `natpmp` | boolean |
| `natpmpGateway` | string |
| `protocol` | string |
| `remark` | string |
| `retryInterval` | integer |
| `script` | string |
| `scriptEnable` | boolean |
| `stunServers` | 数组 of string |
| `stunTimeout` | integer |
| `targetAddresses` | 数组 of string |
| `targetPort` | integer |
| `udpSessionTimeout` | integer |
| `upnp` | boolean |
| `upnpControlUrl` | string |
| `upnpGateway` | string |
| `upnpInternalPort` | integer |
| `upnpLocalHost` | string |
| `useGlobalServers` | boolean |
| `useGlobalWebhook` | boolean |
| `webhook` | `webhook.Config` |
| `webhookOnlyOnChange` | boolean |

### thirdauth.Provider

| 字段 | 类型 |
| --- | --- |
| `clientId` | string |
| `clientSecret` | string |
| `issuer` | string |
| `key` | string |
| `name` | string |
| `proxy` | string |
| `type` | string |

### waf.Instance

| 字段 | 类型 |
| --- | --- |
| `bodyLimitAction` | string |
| `bodyLimitMB` | integer |
| `enable` | boolean |
| `group` | string |
| `inboundThreshold` | integer |
| `key` | string |
| `mode` | string |
| `name` | string |
| `outboundThreshold` | integer |
| `paranoia` | integer |
| `remark` | string |
| `ruleSets` | 数组 of `waf.RuleSet` |
| `skipBodyPaths` | string |
| `webdav` | boolean |

### waf.RuleSet

| 字段 | 类型 |
| --- | --- |
| `enable` | boolean |
| `param` | string |
| `remark` | string |
| `type` | string |

### webhook.Config

| 字段 | 类型 |
| --- | --- |
| `body` | string |
| `enable` | boolean |
| `headers` | 数组 of string |
| `insecureSkipVerify` | boolean |
| `method` | string |
| `successContent` | 数组 of string |
| `timeout` | integer |
| `url` | string |

### webservice.AuthUser

| 字段 | 类型 |
| --- | --- |
| `displayName` | string |
| `email` | string |
| `groups` | 数组 of string |
| `id` | string |
| `name` | string |
| `password` | string |
| `remark` | string |
| `totpSecret` | string |

### webservice.BanInfo

| 字段 | 类型 |
| --- | --- |
| `key` | string |
| `manual` | boolean |
| `permanent` | boolean |
| `reason` | string |
| `source` | string |
| `until` | 时间 (RFC 3339) |

### webservice.CacheConfig

| 字段 | 类型 |
| --- | --- |
| `enable` | boolean |
| `maxMb` | integer |
| `ttl` | integer |

### webservice.ForwardAuth

| 字段 | 类型 |
| --- | --- |
| `copyHeaders` | 数组 of string |
| `loginUrl` | string |
| `url` | string |

### webservice.HealthCheck

| 字段 | 类型 |
| --- | --- |
| `enable` | boolean |
| `fails` | integer |
| `interval` | integer |
| `mode` | string |
| `passes` | integer |
| `path` | string |
| `timeout` | integer |

### webservice.IPList

| 字段 | 类型 |
| --- | --- |
| `count` | integer |
| `enable` | boolean |
| `key` | string |
| `name` | string |
| `refreshHours` | integer |
| `remark` | string |
| `updatedAt` | 时间 (RFC 3339) |
| `url` | string |

### webservice.Listener

| 字段 | 类型 |
| --- | --- |
| `acceptProxyProtocol` | boolean |
| `default` | `webservice.Site` |
| `enable` | boolean |
| `http3` | boolean |
| `key` | string |
| `listenIp` | string |
| `minTls` | string |
| `name` | string |
| `network` | string |
| `plainHttp` | string |
| `port` | integer |
| `remark` | string |
| `sites` | 数组 of `webservice.Site` |
| `tls` | boolean |

### webservice.Location

| 字段 | 类型 |
| --- | --- |
| `prefix` | string |
| `requestHeaders` | 数组 of string |
| `targets` | 数组 of string |

### webservice.OIDCClient

| 字段 | 类型 |
| --- | --- |
| `allow` | 数组 of string |
| `id` | string |
| `name` | string |
| `public` | boolean |
| `redirectUris` | 数组 of string |
| `remark` | string |
| `secret` | string |

### webservice.OIDCSettings

| 字段 | 类型 |
| --- | --- |
| `clients` | 数组 of `webservice.OIDCClient` |
| `enable` | boolean |
| `issuer` | string |
| `signingKey` | string |

### webservice.PathRule

| 字段 | 类型 |
| --- | --- |
| `auth` | string |
| `ipList` | 数组 of string |
| `ipMode` | string |
| `ipSets` | 数组 of string |
| `prefix` | string |

### webservice.Point

| 字段 | 类型 |
| --- | --- |
| `bytes` | integer |
| `requests` | integer |
| `s4xx` | integer |
| `s5xx` | integer |
| `t` | integer |

### webservice.RateLimit

| 字段 | 类型 |
| --- | --- |
| `burst` | integer |
| `enable` | boolean |
| `maxConns` | integer |
| `rate` | number |

### webservice.Rewrite

| 字段 | 类型 |
| --- | --- |
| `code` | integer |
| `pattern` | string |
| `redirect` | boolean |
| `to` | string |

### webservice.Settings

| 字段 | 类型 |
| --- | --- |
| `authSecret` | string |
| `authUsers` | 数组 of `webservice.AuthUser` |
| `autoBanSeconds` | integer |
| `enable` | boolean |
| `oauthCallbackBase` | string |
| `oauthProviders` | 数组 of `thirdauth.Provider` |
| `oidc` | `webservice.OIDCSettings` |
| `selfServicePassword` | boolean |
| `ssoDomain` | string |
| `trustedProxies` | 数组 of string |

### webservice.ShareLink

| 字段 | 类型 |
| --- | --- |
| `expiresAt` | 时间 (RFC 3339) |
| `key` | string |
| `maxUses` | integer |
| `name` | string |
| `pathPrefix` | string |
| `token` | string |

### webservice.Site

| 字段 | 类型 |
| --- | --- |
| `accessLog` | boolean |
| `authOptional` | boolean |
| `basicAuth` | boolean |
| `cache` | `webservice.CacheConfig` |
| `cleanUrls` | boolean |
| `compress` | boolean |
| `contentType` | string |
| `cors` | boolean |
| `countries` | 数组 of string |
| `countryMode` | string |
| `denyClose` | boolean |
| `disableKeepAlives` | boolean |
| `domains` | 数组 of string |
| `enable` | boolean |
| `forwardAuth` | `webservice.ForwardAuth` |
| `forwardedHeaders` | boolean |
| `group` | string |
| `health` | `webservice.HealthCheck` |
| `insecureSkipVerify` | boolean |
| `ipList` | 数组 of string |
| `ipMode` | string |
| `ipSets` | 数组 of string |
| `keepPath` | boolean |
| `key` | string |
| `lbPolicy` | string |
| `listDir` | boolean |
| `locations` | 数组 of `webservice.Location` |
| `name` | string |
| `notFoundPage` | string |
| `oauthUsers` | 数组 of string |
| `passAuthCookies` | boolean |
| `rateLimit` | `webservice.RateLimit` |
| `redirectCode` | integer |
| `redirectTo` | string |
| `remark` | string |
| `requestHeaders` | 数组 of string |
| `responseHeaders` | 数组 of string |
| `rewrites` | 数组 of `webservice.Rewrite` |
| `root` | string |
| `rules` | 数组 of `webservice.PathRule` |
| `sendProxyProtocol` | boolean |
| `shareLinks` | 数组 of `webservice.ShareLink` |
| `sharedUsers` | boolean |
| `statusCode` | integer |
| `targets` | 数组 of string |
| `text` | string |
| `type` | string |
| `upstreamSniFromHost` | boolean |
| `useTargetHost` | boolean |
| `userHeader` | string |
| `userRoutes` | 数组 of `webservice.UserRoute` |
| `users` | 数组 of string |
| `waf` | string |
| `webAuth` | boolean |

### webservice.SiteSharedUsers

| 字段 | 类型 |
| --- | --- |
| `listener` | string |
| `sharedUsers` | boolean |
| `site` | string |

### webservice.SiteStatsView

| 字段 | 类型 |
| --- | --- |
| `bytesOut` | integer |
| `series` | 数组 of `webservice.Point` |
| `status` | 映射 of integer |
| `topCountries` | 数组 of `webservice.TopItem` |
| `topIps` | 数组 of `webservice.TopItem` |
| `upstreams` | 数组 of `webservice.UpstreamStatus` |

### webservice.TopItem

| 字段 | 类型 |
| --- | --- |
| `count` | integer |
| `key` | string |

### webservice.UpstreamStatus

| 字段 | 类型 |
| --- | --- |
| `backup` | boolean |
| `conns` | integer |
| `healthy` | boolean |
| `url` | string |
| `weight` | integer |

### webservice.UserRoute

| 字段 | 类型 |
| --- | --- |
| `targets` | 数组 of string |
| `users` | 数组 of string |

### webservice.UserSite

| 字段 | 类型 |
| --- | --- |
| `basicAuth` | boolean |
| `default` | boolean |
| `domains` | 数组 of string |
| `enable` | boolean |
| `forwardAuth` | boolean |
| `group` | string |
| `listener` | string |
| `listenerEnable` | boolean |
| `listenerName` | string |
| `name` | string |
| `otherAuth` | boolean |
| `port` | integer |
| `sharedUsers` | boolean |
| `site` | string |
| `tls` | boolean |
| `type` | string |
| `webAuth` | boolean |

### webservice.UsersRequest

| 字段 | 类型 |
| --- | --- |
| `authUsers` | 数组 of `webservice.AuthUser` |
| `oidc` | `webservice.OIDCSettings` |
| `selfServicePassword` | boolean |
| `sites` | 数组 of `webservice.SiteSharedUsers` |
| `ssoDomain` | string |

### webservice.UsersView

| 字段 | 类型 |
| --- | --- |
| `authUsers` | 数组 of `webservice.AuthUser` |
| `oidc` | `webservice.OIDCSettings` |
| `selfServicePassword` | boolean |
| `sites` | 数组 of `webservice.UserSite` |
| `ssoDomain` | string |

### webservice.banRequest

| 字段 | 类型 |
| --- | --- |
| `key` | string |
| `seconds` | integer |

### webservice.geoipDownload

| 字段 | 类型 |
| --- | --- |
| `url` | string |

### webservice.totpRequest

| 字段 | 类型 |
| --- | --- |
| `name` | string |

### webservice.totpResponse

| 字段 | 类型 |
| --- | --- |
| `secret` | string |
| `url` | string |

### webterminal.Connection

| 字段 | 类型 |
| --- | --- |
| `enable` | boolean |
| `group` | string |
| `key` | string |
| `local` | `webterminal.LocalConfig` |
| `name` | string |
| `quickAccessDirs` | 数组 of string |
| `remark` | string |
| `ssh` | `webterminal.SSHConfig` |
| `telnet` | `webterminal.TelnetConfig` |
| `type` | string |

### webterminal.Credential

| 字段 | 类型 |
| --- | --- |
| `authType` | string |
| `enable` | boolean |
| `key` | string |
| `keyFile` | string |
| `name` | string |
| `passphrase` | string |
| `password` | string |
| `privateKey` | string |
| `remark` | string |
| `username` | string |

### webterminal.LocalConfig

| 字段 | 类型 |
| --- | --- |
| `env` | 映射 of string |
| `shell` | string |
| `workDir` | string |

### webterminal.Proxy

| 字段 | 类型 |
| --- | --- |
| `enabled` | boolean |
| `host` | string |
| `password` | string |
| `port` | integer |
| `type` | string |
| `username` | string |

### webterminal.SSHConfig

| 字段 | 类型 |
| --- | --- |
| `authType` | string |
| `credentialKey` | string |
| `host` | string |
| `hostKey` | string |
| `keepAlive` | integer |
| `keyFile` | string |
| `passphrase` | string |
| `password` | string |
| `port` | integer |
| `privateKey` | string |
| `proxy` | `webterminal.Proxy` |
| `timeout` | integer |
| `username` | string |
| `workDir` | string |

### webterminal.TelnetConfig

| 字段 | 类型 |
| --- | --- |
| `host` | string |
| `password` | string |
| `port` | integer |
| `proxy` | `webterminal.Proxy` |
| `timeout` | integer |
| `username` | string |

### wol.Device

| 字段 | 类型 |
| --- | --- |
| `broadcastIps` | 数组 of string |
| `checkAddr` | string |
| `enable` | boolean |
| `group` | string |
| `key` | string |
| `macs` | 数组 of string |
| `name` | string |
| `port` | integer |
| `remark` | string |
| `repeat` | integer |

