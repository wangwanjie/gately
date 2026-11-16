---
description: 在云服务器上用 Gately 一个程序替换 haproxy + nginx + fail2ban + acme.sh：逐项对照、迁移顺序、回滚办法，以及同机对比的性能数据。
---

# 云服务器上替换 haproxy + nginx + fail2ban + acme.sh

一台小云服务器常见的组合是：haproxy 按 SNI 分流 443，nginx 终止 TLS 并反向代理，acme.sh 定时续证书，fail2ban 读日志封 IP。四个程序、四套配置。Gately 用协议分流、Web 服务、SSL 证书和入侵防护四个模块覆盖同样的事，一个程序、一个界面。本文给出逐项对照、迁移顺序和回滚办法。

## 适用场景与前提

- 一台 Linux 云服务器，现在运行 haproxy（TCP 模式按 SNI 分流）、nginx（TLS 与反向代理）、acme.sh、fail2ban 中的几个或全部；
- 可以用 root 登录，并能在云厂商控制台临时改安全组；
- 迁移期间两套并存，切换那一刻有几秒的端口交接。选在访问量低的时候做；
- 先阅读[一个 443 端口跑所有服务](/docs/tutorials/one-port-443)，本文复用其中的分流布局。

## 结果是什么样

| 原来 | 现在 |
| --- | --- |
| haproxy：`mode tcp`、`req_ssl_sni`、`send-proxy-v2` | [协议分流](/docs/modules/protoroute)：TLS 路由按域名，PROXY v2 |
| nginx：`server`、`proxy_pass`、`return 301`、`auth_request`、`add_header` | [Web 服务](/docs/modules/web)：监听、站点、重定向站点、转发认证、响应头 |
| acme.sh：cron、DNS API、`--reloadcmd` | [SSL 证书](/docs/modules/ssl)：ACME DNS-01 / HTTP-01，自动续签，自动部署 |
| fail2ban：`sshd`、`nginx-botsearch` 等 jail | [入侵防护](/docs/modules/guard)：日志规则模板、Web 路径探测、TLS 握手失败、诱饵端口，封禁写入 nftables |

对外看到的结构：

```text
访客 ─443─▶ 协议分流 ─┬─ vpn.example.com ─▶ 127.0.0.1:4443
                      └─ 其余 ─▶ 127.0.0.1:8443（Web 服务）─▶ 各应用
```

## 性能参考

Gately 仓库 README 的 Performance 一节在同一台机器上、用真实服务器的配置（90 个域名按 SNI 分流，两跳之间 PROXY v2，ECDSA P-256 证书，TLS 1.3）对比了 haproxy + nginx 与 Gately，每套限制为 2 个 CPU 核：

| 场景 | haproxy + nginx | Gately |
| --- | --- | --- |
| HTTPS 反向代理，keep-alive，1 KB 响应，64 连接 | 14,767 req/s（p99 35.9 ms） | 26,434 req/s（p99 5.7 ms） |
| 每个请求新建 TLS 连接，32 并发 | 2,557 req/s | 3,448 req/s |
| 1 MB 响应，16 连接 | 1,053 MB/s | 1,585 MB/s |
| TLS 流按 SNI 透传，4 连接 | 2,927 MB/s | 4,962 MB/s |
| SNI 透传，每秒新建连接，64 并发 | 6,334 /s | 6,266 /s（持平） |
| 负载下内存 | haproxy 约 21 MB + nginx 约 21 MB | 约 110 MB（一个进程含全部模块） |

测试机为 Intel Core i5-13500T，每个 case 跑 10 秒；nginx 使用生产站点的代理设置（会为每个请求新建后端连接），调优过上游 keep-alive 的 nginx 在小响应上的差距会缩小。在一台真实的 2 vCPU 云主机上，静态站点 Gately 为 5.5k req/s，对比 3.4k req/s；新建 TLS 连接两者持平（约 480 至 550 次每秒），客户端使用 X25519 时如此，使用 Go 客户端与新版浏览器提供的后量子混合密钥交换时，那一轮里 Gately 完成的握手少约 13%。

Gately 的内存占用更高，因为它是一个包含所有模块的进程。数字取决于硬件、配置与流量，只用来说明 Gately 与 haproxy + nginx 处于同一量级，请用自己的负载实测。方法与全部细节见 [README 的 Performance 一节](https://github.com/wangwanjie/gately#performance)。

## 步骤

### 0. 盘点与备份

```bash
cp -a /etc/haproxy /etc/nginx /etc/fail2ban ~/.acme.sh /root/backup-$(date +%F)/ 2>/dev/null
nginx -T > /root/backup-$(date +%F)/nginx-T.txt
haproxy -c -f /etc/haproxy/haproxy.cfg
fail2ban-client status
```

记下：haproxy 里每个 SNI 域名分到哪个后端、有没有 `send-proxy`；nginx 监听的端口；acme.sh 签发了哪些证书、用哪家 DNS 的 API；fail2ban 启用了哪些 jail。

### 1. 安装 Gately，先不对外开放管理端口

Docker 部署时使用 host 网络并授予 `NET_ADMIN`，封禁才会写入宿主机防火墙（见[快速开始](/docs/guide/quick-start)）。想读宿主机日志给入侵防护用，把日志目录挂载进容器：

```bash
docker run -d --name gately --network host --cap-add NET_ADMIN \
  -v ./gately-data:/etc/gately \
  -v /var/log:/var/log/host:ro \
  ghcr.io/wangwanjie/gately:latest
```

管理端口默认 `16680`，首次登录后马上改密码并按[加固建议](/docs/guide/hardening)设置安全入口或限制来源。需要读 systemd 日志（`journalctl`）时，容器内不可用，改用直接安装的程序，见[安装部署](/docs/guide/install)。

### 2. 证书：用 Gately 接管 acme.sh

两种做法，任选其一：

- **重新签发**：在「SSL 证书」添加 ACME 证书，验证方式选 DNS-01，选与 acme.sh 相同的 DNS 服务商并填凭据，域名写 `example.com` 与 `*.example.com`，步骤见[泛域名证书与动态域名](/docs/tutorials/wildcard-cert-ddns)。证书之后由 Gately 自动续签；
- **先读取现有文件**：「添加证书」来源选「读取文件」，证书路径填 acme.sh 的 `fullchain.cer`，私钥路径填 `.key` 文件（Docker 部署时把 `~/.acme.sh` 挂载进容器）。Gately 定期检查文件变化并自动加载，等熟悉后再切换为 Gately 自己签发。

确认 Gately 签发或读取成功后，`crontab -e` 删掉 acme.sh 的续签任务（`acme.sh --uninstall-cronjob`），避免两边同时续签。

### 3. 导入 nginx 站点

在 nginx 上执行 `nginx -T`，在「Web → Web 服务」点「导入 nginx」粘贴输出，预览后导入（保持停用）。细节与 Nginx Proxy Manager 的处理见[从 nginx 或 Nginx Proxy Manager 迁移](/docs/tutorials/migrate-from-nginx)。

### 4. 还原 haproxy 的分流

用 haproxy 配置里的 `use_backend` 与 `default_backend` 对照，在「网络 → 协议分流」建一条规则（详见[协议分流](/docs/modules/protoroute)里的「示例：替换 haproxy」）：

| haproxy | 协议分流 |
| --- | --- |
| `bind 192.0.2.10:443` | 监听地址 `192.0.2.10`（服务器的公网地址），监听端口 `443` |
| `use_backend ocserv if { req_ssl_sni -i tz.example.com }` | 一条 TLS 路由：域名 `tz.example.com`，目标 `127.0.0.1`，端口 `443` |
| `server ocserv 127.0.0.1:443 send-proxy-v2` | 该路由的 PROXY 选「PROXY v2」 |
| `default_backend web` | 「默认目标」`127.0.0.1`，目标端口 `8443`，「默认目标 PROXY」选「PROXY v2」 |

监听地址填具体的公网地址，是为了与监听 `127.0.0.1:443` 的后端并存。网站由 Gately 的 Web 服务提供时：

1. 「Web 服务 → 全局设置」的「可信 PROXY 来源」填 `127.0.0.0/8`；
2. 把导入的 443 监听改为监听地址 `127.0.0.1`、端口 `8443`，打开「接收 PROXY 协议」；
3. 80 端口另建一个监听，放一个域名为 `*` 的重定向站点，目标 `https://{host}`，勾选「保留路径与参数」。

### 5. 还原 fail2ban 的规则

打开「入侵防护」：

1. 先在页面的「设置」里打开「同步到系统防火墙」，并在白名单里加上你自己的出口 IP；
2. 「从模板添加」：SSH（日志文件）读取 `/var/log/auth.log`（Docker 里填 `/var/log/host/auth.log`），或 SSH（systemd 日志）；nginx 的 jail 换成 Web 服务自带的「Web 诱饵路径」「未知域名扫描」「Web 404 探测」「TLS 异常握手」等模板，它们直接统计 Gately 处理的请求，不读日志；
3. 在「诱饵端口与扫描检测」页签打开「诱饵端口」与「端口扫描检测」（需要防火墙同步）；
4. 停用 fail2ban：`systemctl disable --now fail2ban`，避免两套规则重复处理。

封禁由 Gately 写入 nftables 的 `inet gately` 表，不改动系统已有的规则。

### 6. 切换

1. 在云厂商安全组确认放行 TCP 80、443；
2. 停止 haproxy 与 nginx（`systemctl stop haproxy nginx`）释放端口；
3. 启用 Gately 的协议分流规则，并启用 Web 服务的监听与站点；
4. 逐个站点验证（见下文），观察运行日志里没有错误。

### 回滚

停用 Gately 的协议分流规则与 Web 服务监听，启动 haproxy 与 nginx：

```bash
systemctl start nginx haproxy
```

fail2ban 与 acme.sh 的 cron 也恢复启用。Gately 的配置会保留，Gately 的[配置历史](/docs/guide/audit)可以回到修改前。

## 验证

```bash
# 每个域名：状态码与证书
curl -sI https://example.com/ | head -1
echo | openssl s_client -connect example.com:443 -servername example.com 2>/dev/null | openssl x509 -noout -dates

# 分流：VPN 域名到达 VPN 后端
curl -sk -o /dev/null -w "%{http_code}\n" --resolve tz.example.com:443:203.0.113.10 https://tz.example.com/

# 防火墙：Gately 的表存在
nft list table inet gately
```

在「协议分流」卡片上看每条路由与默认目标的连接数是否增长；在入侵防护页看封禁与防火墙同步状态；在 Web 服务站点里确认应用拿到了真实 IP。

## 常见问题

**切换后站点打不开，日志显示端口被占用。** haproxy 或 nginx 没有停干净，`ss -ltnp | grep -E ':(80|443)\b'` 找出占用者。

**应用看到的来源 IP 是 127.0.0.1。** 协议分流默认目标的 PROXY 没选 v2，或 Web 监听没有开「接收 PROXY 协议」，两端要一致。

**把自己封了。** 在入侵防护的白名单里加上自己的出口 IP；开启防火墙同步前先加好。

**证书续签失败。** acme.sh 与 Gately 同时续签，或 DNS 凭据权限不足，看 SSL 证书页卡片与运行日志，见[泛域名证书与动态域名](/docs/tutorials/wildcard-cert-ddns)的常见问题。

**fail2ban 的某条 jail 没有对应物。** 入侵防护的日志规则可以读文件、systemd 日志、命令输出与 Docker 容器日志，规则编辑时可以粘贴日志样例测试正则；自定义 `failregex` 改成规则的过滤正则，用 `<HOST>` 表示来源地址。

## 相关文档

- [协议分流](/docs/modules/protoroute)、[Web 服务](/docs/modules/web)、[SSL 证书](/docs/modules/ssl)、[入侵防护](/docs/modules/guard)；
- [一个 443 端口跑所有服务](/docs/tutorials/one-port-443)；
- [从 nginx 或 Nginx Proxy Manager 迁移](/docs/tutorials/migrate-from-nginx)。
