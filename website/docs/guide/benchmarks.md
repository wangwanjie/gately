# 性能对比

本页把 Gately 和它能替代的常见工具放在同一台机器、同样的 CPU 限额下对比：反向代理、网页认证、WAF、入侵防护、端口敲门、动态域名，以及在 OpenWrt 软路由上的表现。所有数字来自 2026 年 11 月的实测，测试脚本、配置和原始结果都在仓库的 [tools/benchmark](https://github.com/wangwanjie/gately/tree/main/tools/benchmark) 目录，可以照着复现。

<script setup>
const H = '越大越好'
const L = '越小越好'
</script>

## 结论

- **反向代理**：C 语言写的 nginx、HAProxy 吞吐最高，HTTP 约是 Gately 的 3.5 倍、HTTPS 约 2.6 倍。Gately 在 Go 实现的网关里属于第一梯队：HTTPS 吞吐比 Caddy 高 43%、是 Lucky 的 2.3 倍，与 Traefik 相差 9%；TLS 新建连接速率 7 个程序里最高。
- **站点多了不掉速**：1000 个站点时 Gately 的吞吐只降 6%，超过 HAProxy、Traefik 和 Caddy，仅次于 nginx。
- **网页认证**：已登录用户的请求，Gately 是 nginx + Authelia 的 1.9 倍、Caddy + Authelia 的 3.7 倍，内存只有它们的三分之一。
- **WAF**：同为 OWASP CRS 4.25，Gately 处理普通请求比 nginx + ModSecurity 快约 70%，拦截结果完全一致；检查 POST 请求体时 ModSecurity 更快、内存也低得多。
- **入侵防护**：1 万个攻击 IP 时 Gately 12 秒封完，fail2ban 要 51 秒；同样日志量下 CPU 只有 fail2ban 的一半以下。读日志文件时单次封禁延迟比 fail2ban 多约 0.2 秒。
- **端口敲门**：Gately 与 fwknop 敲门延迟都在 10 ms 内，放行后流量不经过敲门程序；飞牛敲门在普通 Linux 上经网关转发，短连接吞吐只有直连的 3%。
- **资源占用**：只算单一功能时，Gately 的内存比 fail2ban、fwknop、ddns-go 这类小工具大；同时承担反代、WAF、认证、DDNS、入侵防护与端口敲门时，一个 Gately 约 91 MiB，换成各自独立的工具合计约 204 MiB。详见[一体化](#一体化)。

## 测试环境

| 项目 | 说明 |
| --- | --- |
| 宿主机 | Intel Core i5-13500T，32 GB 内存，飞牛 fnOS（KVM） |
| 压测虚拟机 | Debian 13（内核 6.12），8 vCPU 绑定宿主机 4 个性能核的 8 个线程，8 GB 内存，Docker 29 |
| CPU 划分 | 被测程序 vCPU 0–1，源站 nginx vCPU 2–3，压测工具 vCPU 4–7，互不抢占 |
| 软路由 | ImmortalWrt 25.12.1 虚拟机，2 vCPU（同一个性能核的两个线程），1 GB 内存 |
| 压测工具 | wrk 4.1、oha 1.17、h2load 1.64（nghttp2） |
| 证书 | 所有程序共用同一张 ECDSA P-256 证书，TLS 1.3 |
| 源站 | nginx 返回 1 KB 文本；大文件测试为 100 MB |

被测版本：Gately v0.6.8.1、nginx 1.29.8、HAProxy 3.2.26、Caddy 2.11.9、Traefik 3.7.19、Lucky 3.8.1、Nginx Proxy Manager 2.17.0（OpenResty 1.31.8）、Authelia 4.39.28、ModSecurity 3.0.16（OWASP CRS 4.25.0）、fail2ban 1.1.0、fwknop 2.6.11、飞牛敲门 fn-knock 2.5.11、ddns-go 6.17.9。除 fail2ban、fwknop 用 Debian 软件包外，其余都用官方 Docker 镜像，网络为 host 模式。

**口径**

- 吞吐：每项 20 秒、测 3 轮取中位数。HTTP 与 HTTPS 用 wrk 128 条长连接；TLS 新建连接用 oha 64 并发、每个请求新开连接；HTTP/2 用 h2load 64 条连接、每条 10 路并发。
- 内存：被测容器内全部进程的 PSS 之和（多进程共享的内存按比例分摊，不会重复计算）。“空闲”是启动并配置完成 10 秒后的值，“峰值”是压测期间的最大值。
- CPU：被测容器 cgroup 的 CPU 时间，100% 等于一个 vCPU。
- 启动：配置就绪后重启程序，计时到第一个请求正确返回，取 3 次中位数。
- 各程序都关闭访问日志。nginx 开启到源站的长连接池并放宽每条连接的请求数上限（常见调优写法）；其余程序使用默认设置。

## 反向代理

域名 `bench.test` 反代到源站，HTTP 监听 8080，HTTPS 监听 8443（Nginx Proxy Manager 只能用 80/443）。

<BenchBars title="HTTPS 请求/秒（长连接）" unit="QPS" :better-label="H" :items="[['nginx',85434],['HAProxy',83953],['Traefik',35812],['Nginx Proxy Manager',32816],['Gately',32764],['Caddy',22871],['Lucky',14376]]" />

<BenchBars title="TLS 新建连接/秒（每个请求一次完整握手）" unit="次/秒" :better-label="H" :items="[['Gately',4636],['Traefik',4482],['Nginx Proxy Manager',4015],['Caddy',3987],['nginx',3796],['Lucky',3621],['HAProxy',3293]]" />

<BenchBars title="空闲内存" unit="MiB" better="lower" :digits="1" :better-label="L" :items="[['nginx',15.7],['HAProxy',36.4],['Gately',47.6],['Caddy',48.8],['Traefik',89.0],['Lucky',124.6],['Nginx Proxy Manager',155.7]]" note="Gately 的 47.6 MiB 包含全部功能模块（DDNS、证书、WAF、入侵防护等都在同一进程里）；Lucky 为 2 个进程，Nginx Proxy Manager 为 22 个进程（nginx、Node.js 后端等）。" />

| 程序 | HTTP | HTTPS | HTTP/2 | TLS 新建/秒 | 100 MB 大文件 | 5000 QPS 时 CPU | 空闲内存 | 压测峰值 | 每条空闲连接 | 重启耗时 | 镜像 |
| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |
| **Gately** | 35,364 | 32,764 | 21,591 | **4,636** | 4,157 MB/s | 67% | 47.6 MiB | 107 MiB | 21.2 KiB | 0.40 秒 | 229 MB |
| nginx | **123,343** | **85,434** | 78,110 | 3,796 | 2,161 MB/s | 55% | **15.7 MiB** | **25 MiB** | **0.6 KiB** | **0.32 秒** | 93 MB |
| HAProxy | 117,923 | 83,953 | **90,633** | 3,293 | 3,912 MB/s | **52%** | 36.4 MiB | 56 MiB | 1.0 KiB | 0.33 秒 | 58 MB |
| Caddy | 24,459 | 22,871 | 11,746 | 3,987 | 4,116 MB/s | 68% | 48.8 MiB | 132 MiB | 27.8 KiB | 0.45 秒 | 92 MB |
| Traefik | 39,112 | 35,812 | 19,606 | 4,482 | **4,311 MB/s** | 66% | 89.0 MiB | 253 MiB | 24.4 KiB | 0.53 秒 | 250 MB |
| Lucky | 15,150 | 14,376 | 10,261 | 3,621 | 3,963 MB/s | 80% | 124.6 MiB | 193 MiB | 26.2 KiB | 3.47 秒 | 58 MB |
| Nginx Proxy Manager | 45,257 | 32,816 | 34,710 | 4,015 | 2,150 MB/s | 58% | 155.7 MiB | 215 MiB | 预分配 | 1.50 秒 | 1,959 MB |

- HTTP、HTTPS、HTTP/2 的单位为请求/秒。“5000 QPS 时 CPU”是以固定 5000 请求/秒压测时被测程序占用的 CPU，越低越省电。
- nginx 默认每条连接处理 1000 个请求后关闭，而 h2load 不会重新建连，所以 HTTP/2 一列 nginx 与 Nginx Proxy Manager 都放宽了这一上限（Nginx Proxy Manager 只在 HTTP/2 测试时通过自定义配置放宽，其余测试保持默认）。
- 大文件一列 nginx 与 Nginx Proxy Manager 较低，是因为 nginx 默认把超出内存缓冲的响应先写进磁盘临时文件。
- “每条空闲连接”是保持 1 万条空闲长连接时增加的内存除以连接数。Go 程序每条连接有独立的读写缓冲与协程，约 20–28 KiB；nginx 与 HAProxy 不到 1 KiB。Nginx Proxy Manager 按配置预先分配了连接表，测不出增量。
- Gately 镜像较大是因为带了 Docker CLI、nftables 和 iptables 工具；程序本身是 40 MB 的单文件，下载压缩包 18 MB。

## 1000 个站点

每个程序配置 1000 个域名不同的反代站点，压测时每个请求随机选一个域名。

<BenchBars title="1000 个站点时的 HTTP 请求/秒" unit="QPS" :better-label="H" :items="[['nginx',122958],['Gately',33211],['Traefik',24229],['HAProxy',24057],['Caddy',9819]]" />

| 程序 | 请求/秒 | 相比 1 个站点 | p99 延迟 | 重启到最后一个站点可用 | 空闲内存 | 压测峰值 |
| --- | --- | --- | --- | --- | --- | --- |
| **Gately** | 33,211 | -6% | 29.8 ms | **0.43 秒** | 94 MiB | 463 MiB |
| nginx | **122,958** | **0%** | **1.2 ms** | 0.54 秒 | 57 MiB | **58 MiB** |
| HAProxy | 24,057 | -80% | 30.0 ms | **0.37 秒** | **51 MiB** | **58 MiB** |
| Caddy | 9,819 | -60% | 52.7 ms | 0.80 秒 | 80 MiB | 361 MiB |
| Traefik | 24,229 | -38% | 18.8 ms | 0.74 秒 | 115 MiB | 157 MiB |

- HAProxy 用 `use_backend ... if { hdr(host) -i 域名 }` 逐条匹配，站点越多越慢；改用 map 文件可以避免，这里保持“每个站点一条规则”的常见写法。
- Gately 的峰值内存主要来自每个站点的访问统计：v0.6.8.1 中有流量的站点各自保存一整块 24 小时的分钟级统计与延迟分位数（约 108 KiB）。开发版改为按小时分块存储后降到 225 MiB，见[开发版的优化](#开发版的优化)。

## 网页认证

站点需要登录才能访问，测试已登录用户的请求和登录本身。Gately 使用站点自带的网页认证与基本认证；对照组为 nginx 或 Caddy 前置 Authelia（`auth_request` / `forward_auth`），以及 nginx 自带的 `auth_basic`。全部走 HTTPS。

<BenchBars title="已登录用户的请求/秒（携带会话 Cookie）" unit="QPS" :better-label="H" :items="[['Gately',26726],['nginx + Authelia',14291],['Lucky',10786],['Caddy + Authelia',7141]]" />

| 方案 | 已登录请求/秒 | 不加认证 | 基本认证（bcrypt） | 成功登录/秒 | 登录时内存峰值 | 空闲内存 |
| --- | --- | --- | --- | --- | --- | --- |
| **Gately** | **26,726** | 32,520 | **28,598** | **38.3** | **70 MiB** | **45 MiB** |
| nginx + Authelia | 14,291 | 83,474 | 40 | 13.4 | 1,082 MiB | 120 MiB |
| Caddy + Authelia | 7,141 | 22,703 | 21,213 | 13.4 | 1,271 MiB | 149 MiB |
| Lucky | 10,786 | 13,841 | 10,107 | 见说明 | 209 MiB | 127 MiB |

- 密码哈希：Gately 与 nginx/Caddy 的基本认证为 bcrypt（cost 10），Authelia 为 argon2id（t=3、64 MiB、p=4），Lucky 以明文保存站点用户密码。Authelia 每次登录要占用 64 MiB 内存，8 个并发登录时内存超过 1 GB。
- nginx 的 `auth_basic` 每个请求都重新计算 bcrypt，所以只有 40 请求/秒；Gately 与 Caddy 会缓存校验结果。换成 apr1（MD5）时 nginx 为 13,082 请求/秒。
- 错误密码：Gately 同一来源失败 10 次后直接拒绝（每秒可处理 1.6 万个错误请求，CPU 不再做哈希）；Authelia 每次失败固定延迟约 1 秒。两者在锁定期间都会拒绝该来源的正确密码，已登录的会话不受影响。
- Lucky 在 8 个并发登录时全部返回 `create webauth session error`，未能测出登录吞吐。

## WAF

三方都使用 OWASP CRS 4.25.0、偏执等级 1、拦截模式。Gately 与 Caddy 用同一个 Coraza 引擎，nginx 用 ModSecurity v3。

<BenchBars title="开启 WAF 后的 GET 请求/秒" unit="QPS" :better-label="H" :items="[['Gately',2498],['Caddy + Coraza',2265],['nginx + ModSecurity',1491]]" />

| 程序 | GET | 带 8 个参数的 GET | 4 KB 表单 POST | 攻击流量 | 压测峰值内存 | 空闲内存（开/关） |
| --- | --- | --- | --- | --- | --- | --- |
| **Gately** | **2,498** | **1,067** | 72 | **1,218** | 754 MiB | 88 / 48 MiB |
| nginx + ModSecurity | 1,491 | 591 | **102** | 1,014 | **73 MiB** | 62 / 24 MiB |
| Caddy + Coraza | 2,265 | 943 | 46 | 1,004 | 792 MiB | 96 / 55 MiB |

| 检测效果 | Gately | nginx + ModSecurity | Caddy + Coraza |
| --- | --- | --- | --- |
| 攻击样本拦截（557 条） | 77.2% | 77.2% | 77.2% |
| 正常样本误报（389 条） | 20.1% | 20.1% | 20.1% |
| CRS 回归用例命中（2049 条） | 95.9% | 95.2% | 95.7% |

- CRS 规则本身很重：三方开启 WAF 后吞吐都只剩关闭时的 1%–10%，需要 WAF 的站点建议只对动态接口开启。
- 检查 POST 请求体时，Coraza 引擎（Gately 与 Caddy 都是）的内存峰值超过 700 MiB，ModSecurity 保持在 73 MiB；小内存设备上开启请求体检查要留意。
- 样本来自 gotestwaf（MIT）、CRS 自带回归测试（Apache-2.0）与手写的正常请求，误报主要是含 SQL/Shell 关键字的技术文章和 PUT/DELETE 接口（CRS 默认不允许这些方法）。

## 入侵防护

场景：Web 服务被扫描，60 秒内 5 次 404 即封禁 IP（nftables）。fail2ban 用 pyinotify 读 nginx 访问日志；Gately 用等价的日志文件规则读同一份日志，另测一组 Gately Web 服务内置的路径探测。

<BenchBars title="1 万个攻击 IP 全部封完所需时间" unit="秒" better="lower" :digits="1" :better-label="L" :items="[['Gately',12.3],['fail2ban',51.2]]" />

| 指标 | Gately（读日志文件） | Gately（Web 服务内置探测） | fail2ban |
| --- | --- | --- | --- |
| 单个攻击者封禁延迟 p50 / p99 | 478 / 996 ms | **10 / 13 ms** | 280 / 864 ms |
| 1000 行/秒日志的 CPU | **2.9%** | — | 20.4% |
| 2 万行/秒日志的 CPU | **35.7%** | — | 83.2% |
| 1 万个攻击 IP：全部封完 | **12.3 秒** | — | 51.2 秒 |
| 1 万个攻击 IP：消耗 CPU | **10.1 CPU 秒** | — | 60.3 CPU 秒 |
| 空闲内存 | 44.1 MiB | 40.7 MiB | **17.6 MiB** |
| 漏封 / 误封 | 0 / 0 | 0 / 0 | 0 / 0 |

- Gately 每秒读取一次日志文件，所以单次封禁延迟在 0–1 秒之间，平均比 fail2ban 的 inotify 慢约 0.2 秒；Web 服务内置探测在请求处理时直接计数，没有这段延迟。
- fail2ban 每次封禁都要启动一次 `nft` 进程，攻击者一多就排队：1 万个 IP 时最后一个在注入结束 41 秒后才被封。
- fail2ban 本身 2 MB，但依赖约 24 MB 的 Python 运行时。

## 端口敲门

对比三种做法：Gately 的端口敲门（fwknop 兼容的 SPA 单包授权，nftables 拦截）、fwknop 2.6.11（同样是 SPA，iptables 拦截），以及飞牛敲门 fn-knock 2.5.11（网页登录后按来源 IP 放行，由它的网关转发流量）。Gately 也提供网页登录后放行的入口，单独作为一组。受保护的是 TCP 端口上的一个 nginx 服务，客户端用公网测试地址，不受“内网免敲门”影响。

<BenchBars title="放行后经受保护端口的短连接请求/秒" unit="QPS" :better-label="H" :items="[['fwknop',65046],['Gately',64763],['Gately 网页放行',63556],['fn-knock',1886]]" note="不经任何敲门程序直连源站为 67,034 请求/秒。" />

| 指标 | Gately（SPA） | Gately（网页放行） | fwknop | fn-knock |
| --- | --- | --- | --- | --- |
| 敲门到端口可连 p50 / p99 | 8.4 / 13.4 ms | 58 / 73 ms | 9.0 / 18.1 ms | 84 / 143 ms |
| 未授权时受保护端口 | 丢弃（超时） | 丢弃（超时） | 丢弃（超时） | 握手后重置（可被探测到） |
| 放行后长连接 / 短连接请求/秒 | 143,867 / 64,764 | 143,839 / 63,556 | 148,371 / 65,046 | 140,137 / 1,886 |
| 放行后转发消耗的 CPU（短连接） | 2% | 1% | 0% | 134% |
| 单一来源 5 万个无效包/秒时的合法敲门 | **10/10**（CPU 26%） | — | 3/8（CPU 102%） | — |
| 伪造来源 1 万个无效包/秒时的合法敲门 | 1/7 | — | **10/10**（CPU 54%） | — |
| 空闲内存 | 48.7 MiB | 48.7 MiB | **2.2 MiB** | 63.2 MiB（3 个进程） |

- Gately 与 fwknop 放行后由防火墙直接放行，流量不经过敲门程序，所以吞吐与直连相同；fn-knock 在普通 Linux 上通过网关转发，每个新连接还要向控制面查询一次授权，短连接吞吐只有直连的 3%。fn-knock 在飞牛 fpk 版本中另有防火墙直连模式，这里测的是 Docker 版。
- fn-knock 的登录需要先完成 ALTCHA 工作量证明（本测试求解 p50 为 28 ms），不含求解时为 50 ms。
- fwknopd 单线程处理每个包，单一来源 5 万包/秒时跑满一个核心，合法敲门开始失败。Gately 对每个来源限速，单一来源的洪泛几乎不占 CPU。
- 已知问题：攻击者伪造大量不同来源地址时，Gately 的来源限速表会被占满，新来源共用的溢出配额被洪泛耗尽，合法敲门多数失败（此时 Gately 的 CPU 只有约 19%）。fwknop 不限速，在 CPU 跑满之前不受影响。

## 动态域名

每个程序配置 20 个 DDNS 任务、检查周期 30 秒，公网 IP 从本机源站查询，更新通过“自定义回调”发到本机源站，不碰真实的 DNS 服务商。启动并完成首次同步后，采样 5 分钟。

<BenchBars title="20 个 DDNS 任务时的平均内存" unit="MiB" better="lower" :digits="1" :better-label="L" :items="[['ddns-go',16.9],['Gately',51.6],['Lucky',131.3]]" />

| 程序 | 平均内存 | 峰值内存 | 平均 CPU | 镜像 |
| --- | --- | --- | --- | --- |
| ddns-go 6.17.9 | **16.9 MiB** | **18.2 MiB** | **0.04%** | 43 MB |
| Gately | 51.6 MiB | 55.4 MiB | 0.16% | 229 MB |
| Lucky 3.8.1 | 131.3 MiB | 134.9 MiB | 0.48% | 58 MB |

- 三者都只在 IP 变化或到了强制同步周期时才请求服务商，常驻开销都很小。只需要 DDNS 时 ddns-go 最省内存；Gately 的数字包含它的全部模块，同一进程里还可以同时跑证书、反代与其他功能。

## OpenWrt 软路由

在 ImmortalWrt 25.12.1 软路由虚拟机（2 vCPU、1 GB 内存）上运行各程序，压测机经局域网网桥访问路由器，路由器再反代到压测机上的源站。nginx 1.26.3 与 HAProxy 3.2.21 来自 ImmortalWrt 软件源，Gately 使用官方 OpenWrt apk 包，Lucky 使用官方 Linux 二进制。

<BenchBars title="软路由上的 HTTPS 请求/秒（长连接）" unit="QPS" :better-label="H" :items="[['nginx',40665],['HAProxy',36266],['Gately',15669],['Lucky',8113]]" />

| 程序 | HTTP | HTTPS | TLS 新建/秒 | 2000 QPS 时 CPU | 空闲内存 | 压测峰值 | 程序体积 |
| --- | --- | --- | --- | --- | --- | --- | --- |
| **Gately** | 16,439 | 15,669 | 2,665 | 27% | 43 MiB | 65 MiB | 40.5 MB |
| nginx | **49,566** | **40,665** | **3,111** | **12%** | **15 MiB** | **22 MiB** | **0.8 MB** |
| HAProxy | 43,046 | 36,266 | 2,337 | 12% | 24 MiB | 29 MiB | 3.7 MB |
| Lucky | 8,642 | 8,113 | 2,021 | 44% | 64 MiB | 81 MiB | 17.9 MB |

- 路由器上的瓶颈主要在内核网络收发（软中断），各程序本身的 CPU 都没有用满两个核心，所以差距比在压测机上小。Gately 的吞吐约是 Lucky 的 2 倍，内存少三分之一。
- OpenWrt 的内核没有 smaps，这里的内存按 `/proc/<pid>/statm` 估算（各进程私有驻留内存之和加一份共享部分）。测试期间临时关闭了防火墙默认的 SYN 洪泛保护（每秒约 25 个新连接），否则 TLS 新建连接测试会被它限住。
- 只做反代的话，软件源里的 nginx 体积和内存都最小；Gately 的 40 MB 包含全部模块，可以在 [build.gately.cn](https://build.gately.cn) 只选需要的模块生成更小的安装包。闪存较小的路由器建议先确认剩余空间。

## 一体化

同一个 Gately 同时开启：两个反代站点（HTTP 与 HTTPS，均启用 WAF）、一个网页认证站点、20 个 DDNS 任务、一条入侵防护日志规则和一条端口敲门规则。对照组是用独立工具实现同样功能的组合，取上文各自实测的空闲内存相加。

<BenchBars title="同样一组功能的空闲内存合计" unit="MiB" better="lower" :digits="1" :better-label="L" :items="[['Gately',90.8],['nginx + ModSecurity + Authelia + ddns-go + fail2ban + fwknop',203.5],['Nginx Proxy Manager + Authelia + ddns-go + fail2ban + fwknop',297.3]]" note="Nginx Proxy Manager 那一组没有 WAF。" />

| 功能 | 独立工具 | 空闲内存 | 安装体积 |
| --- | --- | --- | --- |
| 反向代理 + WAF | nginx + ModSecurity（CRS 4.25） | 61.9 MiB | 156 MB 镜像 |
| 反向代理（图形界面） | Nginx Proxy Manager | 155.7 MiB | 1,959 MB 镜像 |
| 网页认证 | Authelia | 104.4 MiB | 110 MB 镜像 |
| 动态域名 | ddns-go | 17.4 MiB | 43 MB 镜像 |
| 入侵防护 | fail2ban | 17.6 MiB | 24 MB（含 Python） |
| 端口敲门 | fwknop | 2.2 MiB | 0.3 MB |
| **全部** | **Gately** | **90.8 MiB** | **40 MB 程序（下载 18 MB）** |

- Gately 一体化实例在 1000 请求/秒经 WAF 的 HTTPS 流量下，内存峰值 114 MiB，CPU 71%；20 个 DDNS 任务全部同步成功。
- 证书申请（certbot、acme.sh）只在续期时运行，不计入常驻内存。

## 开发版的优化

根据上面的结果，开发版（尚未发布，提交 `3d579389` 之后）做了以下改动，并在同一环境复测。反向代理用提交 `3d579389` 的构建，1000 个站点与入侵防护用包含后续改动的构建；对照列是同一天重新测的 v0.6.8.1（只有 HTTP 与 HTTPS 两项当天重测，其余对照取上文数据）。

- 同一监听器内反代到同一后端、连接设置相同的站点共用一个连接池。
- 可用内存不少于 1 GiB 时，GC 百分比默认值由 100 改为 200（设置里可改，小内存设备保持 100）。
- 反向代理每个请求少复制一次请求、少建三个上下文值。
- 站点的 24 小时分钟统计按小时分块、用到才分配，桶字段压缩为 32 位计数（只有一小时流量的站点从约 108 KiB 降到约 5 KiB，全天有流量时约 50 KiB）。
- 入侵防护读日志文件改用 inotify，写入即读取（保留每秒轮询兜底）；封禁写入防火墙最多每 250 毫秒合并一次。

| 指标 | v0.6.8.1 | 开发版 | 变化 |
| --- | --- | --- | --- |
| HTTP 请求/秒（同日对照） | 35,968 | 38,358 | +6.6% |
| HTTPS 请求/秒（同日对照） | 34,072 | 36,079 | +5.9% |
| TLS 新建连接/秒 | 4,636 | 4,942 | +6.6% |
| HTTP/2 请求/秒 | 21,591 | 23,788 | +10% |
| 1000 个站点 请求/秒 | 33,211 | 37,423 | +13% |
| 1000 个站点 p99 延迟 | 29.8 ms | 12.9 ms | -57% |
| 1000 个站点 压测时内存峰值 | 463 MiB | 225 MiB | -51% |
| HTTP 压测时内存峰值 | 64.5 MiB | 81.1 MiB | +26% |
| 入侵防护：单个攻击者封禁延迟 p50 / p99 | 478 / 996 ms | 9.8 / 129 ms | |
| 入侵防护：持续攻击时封禁延迟 p50 / p99（2 万行/秒） | 509 / 1006 ms | 143 / 269 ms | |
| 入侵防护：1000 行/秒的 CPU | 2.9% | 8.0% | 升高 |
| 入侵防护：2 万行/秒的 CPU | 35.7% | 26.5% | -26% |
| 入侵防护：1 万个攻击 IP 全部封完 | 12.3 秒，10.1 CPU 秒 | 10.1 秒，2.5 CPU 秒 | |

- 吞吐的提升主要来自 GC 设置，代价是压测时内存峰值多出十几 MiB。剖析显示代理路径上剩下的开销几乎都在 Go 标准库的 HTTP 实现与 GC 中，与 nginx、HAProxy 的差距属于语言与运行时层面。
- 1000 个站点时吞吐提高、延迟下降，是因为共用连接池后到源站的连接少了很多；内存峰值减半来自统计的分块存储。
- 入侵防护的单次封禁延迟已低于 fail2ban（280 / 864 ms）。低速率日志下 CPU 比旧版高，是因为防火墙每秒最多写入 4 次，旧版每秒 1 次；仍只有 fail2ban（20.4%）的四成。

## 复现

```bash
git clone https://github.com/wangwanjie/gately && cd gately/tools/benchmark
# 在一台 Debian 13 虚拟机上以 root 运行，需要 Docker 与 wrk、oha、h2load
bash provision-debian.sh
python3 lib/run.py gately 3     # 反向代理，换成 nginx、haproxy、caddy、traefik、lucky、npm
bash auth/run.sh                # 网页认证
bash waf/run.sh                 # WAF
bash guard/run.sh               # 入侵防护
bash knock/run.sh               # 端口敲门
bash scale/run.sh 1000          # 1000 个站点
bash ddns/run.sh 20             # 动态域名
bash bundle/run.sh              # 一体化
bash router/run.sh              # 软路由（另需一台 OpenWrt）
```

每个目录的 README 写明了配置、口径和已知限制。结果受硬件、虚拟化和配置影响，建议用自己的业务负载复测。
