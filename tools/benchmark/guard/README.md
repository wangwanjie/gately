# 入侵防护专项：Gately 入侵防护 vs fail2ban

对比 Gately 入侵防护模块（`internal/modules/guard`，类 fail2ban）与 fail2ban 1.1（Debian 13 apt 包，`nftables-multiport`）在同一场景下的资源占用、封禁延迟、日志吞吐与大规模攻击者处理能力。

## 运行

在压测虚拟机上以 root 运行（脚本以 `/root/benchmark` 为根）：

```bash
bash /root/benchmark/guard/run.sh                      # 全部：gately、fail2ban、gately-web、gately-full
bash /root/benchmark/guard/run.sh gately,fail2ban latency   # 只跑部分被测 / 测试
```

结果：`/root/benchmark/results/guard-<被测>.json`，汇总表 `results/guard-summary.txt`（`report.py` 生成），过程日志追加到 `results/guard-run.log`；吞吐与大规模测试另存逐 IP 原始数据 `results/guard-<被测>-raw-<测试>.json`（`[ip, 达到阈值, 进入集合, 决定-阈值]`，前两个时间为单调时钟秒）。

事先准备：

- `apt-get install -y fail2ban python3-pyinotify python3-systemd`（Debian 13 为 fail2ban 1.1.0-8；装完后服务会自启，脚本会先停掉，结束时 `systemctl disable fail2ban`）。
- 镜像 `ghcr.io/wangwanjie/gately:v0.6.8.1`（已有）。不需要其他镜像。
- 已有工具：python3、nft、docker、taskset、systemd。

预计耗时约 20 分钟（colima 试跑：gately 6 分钟、fail2ban 8 分钟、gately-web 与 gately-full 各约 1 分钟，另有每项启动与镜像检查）。每项吞吐测试的追平等待上限 600 秒、大规模测试 1200 秒，若某一方严重跟不上，最坏情况会延长到约 1 小时。

## 场景与配置（两边一致）

- 一个 Web 站点被扫描：nginx combined 格式访问日志 `/var/log/bxguard/access.log`，规则为 **60 秒内同一来源 5 次 404 即封禁 1 小时**，封禁写入 nftables 集合。
- fail2ban：jail `bx-404`（`fail2ban/bx-guard.local`，装到 `/etc/fail2ban/jail.d/zz-bx-guard.local`），`backend = pyinotify`，过滤器 `fail2ban/bx-404.conf`，`banaction = nftables-multiport`（`port = http,https`），关闭 Debian 默认的 sshd jail，其他保持 Debian 默认（含 sqlite 封禁数据库，每次启动前删除）。集合为 `inet f2b-table addr-set-bx-404`。systemd 服务，`systemctl set-property --runtime fail2ban.service AllowedCPUs=0,1`。
- Gately：官方镜像 `docker run --network host --cpuset-cpus 0,1 --cap-add NET_ADMIN`，日志目录只读挂入；`gately.json` 的 `disabledModules` 关闭除入侵防护外的全部模块；`guard.json` 一条来源为 `file` 的规则，正则与 fail2ban 同义（`^<HOST> \S+ \S+ \[[^\]]+\] "[A-Z]+ [^"]*" 404\s`），`firewall: true`、`ignorePrivate: true`、`increment: false`，诱饵端口与扫描检测关闭。集合为 `inet gately ban4`。
- 补充两组 Gately：
  - `gately-web`：不读日志，用 Web 服务内置的路径探测（来源 `web`，同样 60 秒 5 次 404）。监听 `127.0.0.1:18480` 无站点、默认站点未启用（全部返回 404），`realIPFrom 127.0.0.1` + `X-Forwarded-For`，攻击脚本每个请求带不同的 XFF 作为“攻击者 IP”。只测空闲内存与封禁延迟。
  - `gately-full`：不关任何模块（默认安装的形态），同一条日志规则，只测空闲内存。
- CPU：被测 vCPU 0,1；本脚本、注入器、`nft monitor` 由 `run.sh` 用 `taskset` 固定在 vCPU 4-7。两边依次运行，每项测试前重启被测程序、删除 nftables 表与持久化的封禁记录、清空日志文件。

## 伪造来源的办法：日志注入（主方法）

`inject.py` 直接向日志文件追加格式正确的行，按墙钟每 10 毫秒补齐“速率 × 已过时间”的行数，一次 `write` 写入，速率可精确控制（结果里有 `achievedRate`）。

- 攻击者地址取自 `198.18.0.0/15`（RFC 2544 基准测试保留段，两边的“忽略内网/本机”都不会跳过），每个攻击者**恰好写 5 行 404**（达到阈值即停，模拟被封后不再产生日志），同一攻击者的几行与其他攻击者交错、分散在约 1~2 秒内。结束时未写满 5 行的攻击者记为 partial，不应被封。
- 正常行来自 `203.0.113.0/24`、`192.0.2.0/24`，状态码 200，不计数。
- 日志时间戳为写入时刻（fail2ban 按日志时间判断窗口，Gately 按读到的时刻）。

`gately-web` 用真实 HTTP 请求（XFF 伪造来源）作为补充：Gately 的路径探测不经过日志文件，是另一条处理路径，与 fail2ban 没有直接可比的对象，只用来说明“同一程序内置探测”的延迟。

## 指标与口径

| 指标 | 口径 |
| --- | --- |
| 空闲内存 | 就绪后（金丝雀封禁完成）等待 30 秒，cgroup 内全部进程 PSS 之和（`lib/sample.py`），取 3 次快照平均；同时给出这 30 秒的平均 CPU |
| 安装体积 | Gately：镜像内 `/usr/local/bin/gately` 单个文件大小（含全部模块；镜像大小另列，含 Alpine 与 nftables）。fail2ban：dpkg `Installed-Size`，分为 fail2ban 包本身、fail2ban + Python 运行时（闭包中 `python3*`、`libpython3*` 包，含 pyinotify、python3-systemd）、闭包中其余共享库（libc6、libssl3、libsqlite3 等，多数系统已装，仅供参考）。不含 apt 默认装上的推荐包（如 python3-setuptools） |
| 封禁延迟 | 时间一律用 CLOCK_MONOTONIC（注入器与监视线程在同一台机器上，单调时钟可比；墙钟会被时间同步调整，colima 里实测 40 秒内跳 0.38 秒，曾造成负延迟）。起点：攻击者第 5 行写入前的时刻（`write` 调用前，write 本身几微秒）；Web 组为发出第 5 个请求前的时刻。终点：`nft monitor new elements` 收到该 IP 加入集合的通知时刻（同一台机器墙钟）。monitor 漏掉的元素由每秒一次的 `nft -j list set` 兜底并计入 `polledFallback`。延迟测试为 60 个攻击者依次进行，每个攻击者 5 行间隔 20 毫秒，攻击者之间随机间隔 0.3~1.3 秒（避免与被测程序轮询周期同相） |
| 延迟分段 | `detect` = 起点 → 程序日志里记下封禁决定（Gately 入侵防护日志 `guard.log*` 的“封禁 x”，纳秒时间戳；fail2ban `fail2ban.log` 的 `Ban x`，毫秒）；`sync` = 决定 → 出现在集合。这两段只能用墙钟对照，只作参考。注意 fail2ban 的 `Ban` 日志由动作线程在执行 `actionban` 前写，动作队列的排队时间算在 detect 段 |
| 日志吞吐 | 1000、5000、20000 行/秒各 60 秒，2% 为恶意行（分别约 240、1200、4800 个应封 IP）。注入期间平均 CPU（100% = 1 个 vCPU，上限 200%）、整段峰值 PSS；应封/已封/漏封/误封（集合中出现不应封的 IP）；封禁延迟分布；`lastBanAfterInjectEndSec` 注入结束到最后一个封禁；`cpuBusyAfterInjectEndSec` 注入结束后 CPU 连续 2 秒低于 5% 所需时间（处理积压的时间）。注入结束后等到全部封完，或 30 秒无新封禁且 CPU 已回落，上限 600 秒 |
| 大规模攻击者 | 1 万个不同 IP，每个 5 行 404，以 5000 行/秒注入（约 10 秒）。全部封完的时间（从注入开始，`allBannedSec`）、注入结束后多久封完、延迟分布与 sync 段（nftables 同步耗时）、整段 CPU 秒数、峰值 PSS、列出 1 万元素集合的耗时。上限 1200 秒 |

## 已知限制

- 日志注入绕开了 nginx 本身；真实场景下 nginx 写日志的开销两边相同，不影响对比。
- fail2ban 按日志时间戳判断窗口，若处理积压超过 60 秒，较早的行会被当作过期而漏封；Gately 按读到的时刻计数，没有这一行为。漏封数如实计入。
- `nftables-multiport` 只封 80/443（fail2ban 常用配置），Gately 封该来源全部流量（prerouting）；本测试只看集合元素出现的时刻，不受影响。
- fail2ban 的 `Ban` 日志与 Gately 的决定日志精度不同（毫秒/纳秒），分段只作参考；总延迟以 nft monitor 为准。
- Gately 的“程序体积”是包含全部模块的单文件；`build.gately.cn` 可构建只含部分模块的版本，本测试未使用。
- 压测机只有一份结果（每项一轮）；延迟样本 60 个，p99 实际接近最大值。
- 本机 colima（arm64，6 CPU，脚本/注入器只有 2 个 vCPU）上的试跑只用于验证脚本，数值不作结论。
- Gately 的文件来源每秒轮询一次文件（`internal/modules/guard/tail.go` 的 `tailPoll = time.Second`），fail2ban 用 pyinotify 事件驱动。读日志的封禁延迟主要由此决定（Gately p50 约 0.5 秒、最大约 1 秒），这是设计差异，测试如实反映。
- 每个攻击者恰好 5 行：真实扫描器被封后在 nftables 层被丢弃、不再产生日志，与此一致；但 fail2ban 的 multiport 只封 80/443，若扫描其他端口仍会产生其他日志，与本场景无关。

## 本机调试（colima）

`lab/lab.sh` 构建特权容器 `bx-guard-lab`（Debian 13，systemd 为 1 号进程，容器内自带 Docker，独立网络命名空间），与压测机相同地 apt 安装 fail2ban、在容器内 Docker 里 `--network host` 跑 Gately，然后执行 `run.sh`；`tools/benchmark` 以复制方式放入容器，结果复制到 `/tmp/bx-guard-results`。可用 `GUARD_RATES`、`GUARD_STREAM_SEC`、`GUARD_LAT_SAMPLES`、`GUARD_BIG_ATTACKERS`、`GUARD_IDLE_WAIT` 缩短。用完 `docker rm -f bx-guard-lab`。

## 文件

- `run.sh`：入口，依次运行各被测并生成汇总。
- `guard.py`：被测程序的启停与配置、封禁监视（nft monitor）、cgroup 采样、四项测试。
- `inject.py`：日志注入器。
- `report.py`：汇总表。
- `fail2ban/bx-404.conf`、`fail2ban/bx-guard.local`：fail2ban 过滤器与 jail。
- `lab/Dockerfile`、`lab/lab.sh`：本机调试环境。
