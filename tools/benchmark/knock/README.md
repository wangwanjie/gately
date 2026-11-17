# 端口敲门专项对比

对比三种“未授权时隐藏服务、授权后按来源 IP 放行”的方案：

| 被测名 | 程序 | 授权方式 | 拦截/放行方式 |
|---|---|---|---|
| `gately` | Gately v0.6.8.1 端口敲门模块（`internal/modules/knock`） | fwknop 兼容 SPA 报文（UDP 62201） | nftables 表 `inet gately_knock`，放行写入带超时的集合 |
| `gately-web` | 同一 Gately，站点“放行入口”（`knockGrant`，`internal/modules/webservice/knockgrant.go`） | 站点网页登录（用户名 + bcrypt 密码，无验证码） | 同上：同一条敲门规则的 nftables 集合 |
| `fwknop` | fwknop 2.6.11（Debian 13 包，`fwknopd -U` UDP 服务器模式） | SPA 报文（UDP 62201） | iptables（Debian 13 为 nft 后端）`FWKNOP_INPUT` 链插入 ACCEPT |
| `fnknock` | fn-knock 2.5.11（`kcilnk/fn-knock:2.5.11`，Rust 控制面 + Go 网关） | 网关 7999 网页登录（用户名 + 密码 + ALTCHA 工作量证明） | 用户态 TCP 端口映射 9200 → 源站，网关按来源 IP 查授权后转发 |
| `direct` | 对照组：没有敲门程序，客户端直连源站 | - | - |

fn-knock 在普通 Linux 的 Docker 部署里只能做“网页认证后按来源 IP 放行”的网关（`run_type 3`），防火墙直连模式只有飞牛 fpk 版可用
（容器内日志 `auto_manage_firewall -> false`、`host firewall is unavailable`），所以它和 Gately 的网页放行入口 `gately-web` 是同类，
和 SPA 报文的 `gately`、`fwknop` 机制不同。表格里四者并列，但结论要按机制分组看。

## 运行

```bash
bash /root/benchmark/knock/run.sh                      # 全部：direct gately gately-web fwknop fnknock
bash /root/benchmark/knock/run.sh gately fwknop        # 只跑部分
python3 /root/benchmark/knock/summary.py               # 汇总 results/knock-*.json 为 Markdown 表格
python3 /root/benchmark/knock/spoof_probe.py gately    # 补充：伪造来源洪泛的速率扫描（gately 或 fwknop，默认 1k～10k 包/秒）
```

结果：`/root/benchmark/results/knock-<被测名>.json`（`spoof_probe.py` 为 `knock-spoof-<名>.json`）。

预计耗时（默认参数，按本机试跑实测）：direct 约 2.5 分钟，gately 约 7 分钟，gately-web 约 5 分钟，fwknop 约 6.5 分钟，
fnknock 约 7.5 分钟（每次登录前要等网关的未授权缓存过期），全部约 30 分钟；`spoof_probe.py` 每个程序约 5 分钟。

可调环境变量：`SUT_CPUS`（默认 0,1）、`ORIGIN_CPUS`（2,3）、`LOAD_CPUS`（4-7）、`N_LAT`（延迟测试次数，60）、
`FLOOD_RATES`（10000,50000）、`FLOOD_DUR`（每轮洪泛秒数，40）、`FLOOD_KNOCKS`（洪泛期间合法敲门最多次数，10）、
`FLOOD_PACE`（洪泛期间两次合法敲门的间隔秒数，2.5）、`FLOOD_PROCS`（发包进程数，2）、`DP_DUR`（数据面每轮秒数，20）、
`DP_ROUNDS`（3，取 rps 中位数）、`RESULTS_DIR`（结果目录，默认 `/root/benchmark/results`；本机试跑请指到临时目录）、
`GATELY_IMAGE`、`FNK_IMAGE`、`FWK_IMAGE`。

### 事先准备

- 镜像：`ghcr.io/wangwanjie/gately:v0.6.8.1`、`kcilnk/fn-knock:2.5.11`、`nginx:1.29-alpine`（均已在压测机上），
  以及构建 fwknop 镜像用的基础镜像 `debian:trixie-slim`。`run.sh` 发现没有 `bench-fwknop:2.6.11` 时用 `fwknop/Dockerfile`
  构建（apt 走 mirrors.ustc.edu.cn）；也可以在别处构建好再 `docker load`。
- 命令：已有的 wrk、oha、python3、curl、nftables、iptables，另需 `iproute2`（ip netns）、`util-linux`（taskset）；
  `nmap` 可选（`apt install nmap`），没有时隐蔽性只用自带探测。
- 不需要公共源站 `backend`：本专项自带源站容器 `knock-origin`（nginx，vCPU 2,3，监听 9100，`/` 返回 1024 字节），结束时删除。

## 拓扑与公平性

- 全部流量在压测机内部：客户端命名空间 `knockc`（veth，地址 203.0.113.10）、攻击命名空间 `knocka`（198.51.100.10），
  宿主机一侧 203.0.113.1 / 198.51.100.1。被测程序、源站都是 host 网络。客户端、攻击工具、SPA 报文生成容器都用 `taskset`/`--cpuset-cpus`
  绑在 `LOAD_CPUS`，被测程序在 `SUT_CPUS`，源站在 `ORIGIN_CPUS`。
- 客户端用公网地址（TEST-NET），三者的“内网免认证”都不生效：Gately 默认免敲门网段为 10/8、172.16/12、192.168/16、169.254/16 与回环
  （`iifname lo`）；fn-knock 的 `is_private_or_local_ip` 还包含 100.64/10 与 127/8、0/8。客户端不能走回环：Gately 的拦截规则对 `lo` 一律放行，
  所以用 veth 命名空间而不是在 lo 上加地址。
- 受保护端口：Gately、fwknop 拦截 tcp/9100（源站端口）；fn-knock 映射 9200 → 127.0.0.1:9100（`use_auth: true`）。
  Gately 网页放行入口为 HTTP 8080 上的站点 `knock.test`（网页认证 + `knockGrant`），该监听的默认站点设为“断开连接”，
  不带该域名的探测拿不到登录页；fn-knock 的登录页在 7999 对任何 Host 可见（它的默认行为）。
  fwknopd 只插放行规则，默认拒绝由脚本补一条与 Gately 语义一致的链 `KNOCK_BENCH`：回环与已建立连接放行，其余发往 9100 的包 DROP。
- 三者用同一组密钥/口令：SPA 两者共用同一对随机密钥（AES 32 字节 + HMAC-SHA256 64 字节）和同一个 fwknop 2.6.11 客户端生成的报文
  （`fwknop -A tcp/9100 -a 203.0.113.10 -T -B`，只生成不发送，由 `client.py` 发送并计时）；网页登录两者都是用户 `bench`、
  密码 `Bench!Visit2026`，Gately 存 bcrypt（cost 10），fn-knock 存它自己的口令哈希。
- 放行时长都设为 300 秒以上（Gately 规则 300 秒、网页放行 60 分钟；fwknop `FW_ACCESS_TIMEOUT 300`；fn-knock 登录会话 24 小时），
  保证数据面测试期间不过期。
- 三者不同时运行：每个被测程序启动前清理 `inet gately_knock`、`inet gately` 表与 `KNOCK_BENCH`、`FWKNOP_INPUT` 链，结束后 `docker stop` 并兜底清理。
- 每次敲门前先撤销授权，并用短探测确认受保护端口已不可访问：Gately 调 `DELETE /api/knock/grants`（脚本把管理员密码改成测试密码，
  默认密码不允许调用修改类接口），fwknop 清空 `FWKNOP_INPUT`，fn-knock 删除会话与自动白名单。确认后等 `settle` 秒再敲门：
  SPA 与 Gately 网页 0.3 秒；fn-knock 1.3 秒，因为它的网关把“未授权”结果缓存 1 秒（`auth_cache_unauthorized_ttl_seconds`），
  确认关闭的探测会写入缓存，不等过期测到的是缓存时长。

## 指标与口径

1. **资源与体积**：启动耗时（从 `docker run` 到可用：Gately 为敲门端口监听且拦截规则生效，fwknop 为 UDP 端口监听且 `FWKNOP_INPUT` 跳转已插入，
   fn-knock 为管理端健康检查通过，轮询间隔 0.1 秒；Gately 默认配置由镜像空跑生成，不计入）；
   就绪 10 秒后 `lib/sample.py snapshot`（cgroup 内全部进程 PSS 之和、进程数）；镜像大小；程序文件大小
   （Gately 为单个二进制；fwknop 为 fwknopd + libfko，并给 deb 包安装大小；fn-knock 为两个二进制与整个应用目录）。
   三者镜像基础不同（Gately、fn-knock 为 Alpine，fwknop 为 Debian slim + iptables），镜像大小只作参考。
   Gately 为默认配置（全部模块加载，只配置了敲门与一个站点），空闲内存包含其他空闲模块。
2. **敲门到放行延迟**（`N_LAT` 次，p50/p90/p99）：
   - SPA：从 `sendto` 发出报文到受保护端口返回首个 HTTP 200。轮询方式为每次新建 TCP 连接、0.5 毫秒内握手未完成即放弃重试
     （被 DROP 的 SYN 没有回应），握手后发 `GET /`；分辨率约 0.5～1 毫秒。
   - 网页登录：`total` 从发起第一个认证请求开始计时；fn-knock 依次为取 ALTCHA 题目、本地求解（Python hashlib，单线程）、提交登录，
     `captchaSolve` 单列，`exCaptcha` 为“提交登录到首个 200”（除去取题与求解）；Gately 网页为提交登录表单 + 跟随 302 访问站点
     （放行在带会话访问站点时登记，登录请求本身不登记，浏览器会自动跟随），没有验证码，`exCaptcha` 等于 `total`。
     网页类在登录响应返回后才开始轮询，`afterLogin` 为登录响应返回到首个 200 的时间。
3. **隐蔽性**：未授权时从客户端命名空间探测：TCP 连接 3 秒超时为 DROP、立即被拒为 RST，握手成功后记录对方是返回数据、关闭还是重置；
   UDP 端口区分 ICMP 不可达与无响应；另附 `nmap -sS/-sU` 结果。同时列出管理端口（Gately 16680、fn-knock 7991）与认证入口是否对公网可见。
4. **抗洪泛**：每个场景持续 `FLOOD_DUR` 秒，攻击命名空间发送，开始 3 秒后合法客户端每隔 `FLOOD_PACE` 秒敲门一次，最多 `FLOOD_KNOCKS` 次，
   洪泛结束前 3 秒停止（单次 3 秒内没放行算失败）。间隔取 2.5 秒是因为 Gately 按来源网段限速，网段空闲 2 秒后限速记录可被清理，
   间隔更短时合法客户端一直持有自己的限速桶，结果偏乐观；真实用户通常是隔很久才敲一次。
   记录被测程序 CPU（cgroup，100% = 1 核）、峰值 PSS、合法敲门成功率与延迟、攻击实际速率；Gately 另记模块计数器（accepted/rejected/ignored）。
   - `udp-single`：单一来源 198.51.100.10 发送格式正确、用另一对密钥生成的 SPA 报文（必须做完 HMAC 才能判为无效）。
   - `udp-spoof`：原始套接字伪造随机来源（数万个 /24，排除内网与测试网段）。洪泛期间对敲门 UDP 端口加 `notrack`
     （表 `inet knockbench`），否则每个伪造来源新建一条连接跟踪，几秒占满 `nf_conntrack` 表后内核连合法报文都丢，测到的是内核表容量；三者同样处理。
   - `http-badlogin`（网页类）：oha 按目标速率发无效登录。fn-knock 为带无效验证码证明的登录（不解题的攻击者只能这样发）；
     Gately 网页为错误密码（Gately 网页登录没有验证码，同一来源 15 分钟内失败 10 次后不再校验密码）。
   - `tcp-unauth-conn`（仅 fn-knock）：未授权来源直接连受保护映射端口（短连接）。Gately、fwknop 的受保护端口在内核中丢弃 SYN，
     不进入用户态，这项对它们没有可比性，未测。
   - 速率 1 万、5 万是目标值；HTTP 场景的实际完成速率取决于被测程序（被限速或封禁时 oha 统计为连接错误），以 `flood.rps` 与 `statusCodes`/`errorDist` 为准。
5. **放行后的数据面**：授权后从客户端访问受保护端口，`wrk -t2 -c64`（长连接）与 `oha --disable-keepalive -c32`（每请求新连接），各 `DP_ROUNDS` 轮取 rps 中位数；
   对照组 `direct` 直连源站。Gately、fwknop 放行后流量不经过它们的进程（只有内核规则匹配：Gately 发往受保护端口的包跳到 gate 链，已建立连接的包在 `ct state established` 处返回，
新连接再查一次放行集合；fwknop 为 INPUT 链顺序匹配），
   被测 CPU 接近 0 是口径使然，内核 softirq 开销不计入任何 cgroup；fn-knock 每个连接经它的 Go 网关在用户态转发。
   wrk 结束时每组都会报约 2×连接数的 socket 读错误（对照组 direct 同样如此），是 wrk 收尾关闭连接的计数，不代表失败请求。

## 限制

- 单机 veth 环境，RTT 约几十微秒，延迟数字反映的是程序内部处理时间（解析/认证/写防火墙/查授权），不含公网 RTT。
- fwknopd 使用 UDP 服务器模式（`-U`，与 Gately 的 UDP 监听对等）；Debian 包的默认配置是 NFQUEUE 抓包模式，另有 libpcap 模式，未测。
- 伪造来源洪泛依赖 `rp_filter` 放行：脚本把两块测试网卡的 `rp_filter` 设为 0，`all.rp_filter` 为 1 时临时改为 2，结束后恢复。
- fn-knock 的 ALTCHA 难度为默认 `maxnumber 100000`，求解时间随机（平均约 5 万次 SHA-256），用 Python 求解比浏览器 Web Worker 慢；
  `exCaptcha` 一列不受此影响。
- 撤销授权的方式各不相同（API / iptables 清空 / 删会话），只用于测试之间复位，不计入任何指标。
- fn-knock 的 Go 网关 `go-reauth-proxy` 在仓库中只有预编译二进制，网关内部行为（缓存、限速、封禁）按黑盒观察记录。
- 本机（Apple Silicon colima，arm64、6 vCPU、Docker-in-Docker）的试跑只用于验证脚本与口径，数值不能代表压测机。
