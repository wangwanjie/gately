# 性能对比测试

Gately 与它能替代的工具（nginx、HAProxy、Caddy、Traefik、Lucky、Nginx Proxy Manager、Authelia、ModSecurity、fail2ban、fwknop、飞牛敲门、ddns-go）的对比脚本、配置与原始结果。结论与图表见官网 [性能对比](https://gately.cn/docs/guide/benchmarks)。

## 环境

- 压测机：KVM 虚拟机 Debian 13，8 vCPU 绑定宿主机（Intel Core i5-13500T）4 个性能核的 8 个线程，8 GB 内存，Docker。
- CPU 划分：被测程序 vCPU 0,1（`docker run --cpuset-cpus 0,1`，fail2ban/fwknopd 用 systemd `AllowedCPUs`），源站 nginx vCPU 2,3（容器 `backend`），压测与攻击工具 vCPU 4-7（`taskset -c 4-7`）。
- 软路由：ImmortalWrt 25.12.1 虚拟机，2 vCPU（宿主机同一性能核的两个线程），1 GB 内存；由压测机发压。
- 工具：wrk、oha、h2load；采样用 `lib/sample.py`（cgroup 内进程 PSS 之和、cgroup CPU 时间）。
- 更详细的约定见 [BRIEF.md](BRIEF.md)。

## 目录

| 目录 | 内容 | 入口 |
| --- | --- | --- |
| `provision-debian.sh`、`backend/` | 压测机初始化：系统参数、测试证书、源站 | `bash provision-debian.sh` |
| `lib/run.py`、`suts/` | 反向代理：HTTP/HTTPS 长连接、TLS 新建连接、HTTP/2、固定 5000 QPS 的 CPU、100 MB 大文件、1 万条空闲连接、重启耗时 | `python3 lib/run.py <程序> 3` |
| `scale/` | 1000 个站点：随机 Host 吞吐、重启到最后一个站点可用、内存 | `bash scale/run.sh 1000` |
| `auth/` | 网页认证：Gately、nginx/Caddy + Authelia、auth_basic、Lucky | `bash auth/run.sh` |
| `waf/` | WAF：Gately、nginx + ModSecurity、Caddy + Coraza，吞吐与检测样本 | `bash waf/run.sh` |
| `guard/` | 入侵防护：Gately 与 fail2ban 的封禁延迟、日志吞吐、1 万个攻击者 | `bash guard/run.sh` |
| `knock/` | 端口敲门：Gately、fwknop、飞牛敲门的延迟、隐蔽性、洪泛与放行后的数据面 | `bash knock/run.sh` |
| `ddns/` | 动态域名：Gately、ddns-go、Lucky 各 20 个任务的资源占用 | `bash ddns/run.sh 20` |
| `bundle/` | 一体化：一个 Gately 同时开反代、WAF、认证、DDNS、入侵防护、端口敲门 | `bash bundle/run.sh` |
| `router/` | 软路由上的反代对比（Gately、nginx、HAProxy、Lucky） | `bash router/run.sh` |
| `lib/report.py`、`lib/charts.py` | 汇总表格与 README 用的 SVG 图 | `python3 lib/report.py results /tmp/out` |
| `results/` | 2026-11-18 实测的原始结果（JSON） | |

`queue*.sh` 是这次实测时在压测机上串行跑完各专项用的队列脚本，保证各项测试互不重叠。

## 口径要点

- 每项吞吐测试 20 秒、3 轮取中位数；内存为被测容器全部进程 PSS 之和；CPU 100% 等于一个 vCPU。
- 各程序关闭访问日志；nginx 开启到源站的连接池并放宽每条连接的请求数上限；其余用默认设置。
- 启动耗时：配置就绪后 `docker restart` 计时到第一个正确响应，3 次中位数。
- OpenWrt 内核没有 smaps，路由器上的内存用 `/proc/<pid>/statm` 估算（各进程私有驻留页之和加最大的一份共享页）；测试期间临时关闭 fw4 的 SYN 洪泛保护（默认每秒约 25 个新连接），结束后恢复。
- nginx 默认每条连接 1000 个请求后关闭连接，h2load 不会重连：Nginx Proxy Manager 的 HTTP/2 单独补测（`NPM_KEEPALIVE_REQUESTS=1000000 python3 lib/run.py npm 3 h2`，结果 `results/npm-h2.json`）。
- 结果受硬件、虚拟化与配置影响，请用自己的负载复测。
