# 认证网关专项对比

场景：站点 `bench.test` 需要登录才能访问。测两件事：已登录用户的请求转发性能，以及登录本身的开销（正确密码的吞吐、错误密码的处理与防爆破）。

## 运行

压测机上以 root 执行（不需要任何环境变量）：

```bash
bash /root/benchmark/auth/run.sh                 # 依次测全部被测，约 30 分钟
bash /root/benchmark/auth/run.sh gately,lucky    # 只测指定被测
python3 /root/benchmark/auth/bench.py summary    # 汇总已有结果为 Markdown 表格
```

结果写到 `/root/benchmark/results/auth-<被测>.json`，结束时打印汇总表。预计耗时（DUR=20、ROUNDS=3）：gately 约 7 分钟，nginx-authelia 约 9 分钟，caddy-authelia 约 7 分钟，lucky 约 7 分钟。

使用的镜像（BRIEF 中均已列为压测机已有，无需另传）：`ghcr.io/wangwanjie/gately:v0.6.8.1`、`nginx:1.29-alpine`、`authelia/authelia:4.39.28`、`caddy:2-alpine`、`gdy666/lucky:v3.8.1_lucky`。另需压测机自带的 `openssl`（生成 apr1）与 `python3-cryptography`（Lucky 登录加密，`suts/lucky/setup.py` 已依赖）。

本机试跑：`bash tools/benchmark/auth/colima.sh [被测名]`（在 colima 里起 runner 容器，容器名 `bx-` 前缀，源站改 19100 端口，默认 DUR=8、ROUNDS=1，结果在 `/tmp/bx-auth/results`）。colima 数据只用于验证脚本与口径。

## 被测与端口

所有被测 `--network host --cpuset-cpus 0,1`，HTTPS，同一张证书，反代到源站 `127.0.0.1:9000`。同一个被测进程用不同端口承载不同认证方式，不加认证的端口就是同一进程的对照组。

| 被测 | 8443 | 8444 | 8445 | 8446 / 8447 |
|---|---|---|---|---|
| gately | 网页认证 + 共享用户（会话 Cookie） | 不加认证 | 基本认证 + 共享用户 | — |
| nginx-authelia | auth_request + Authelia（到 Authelia 走 keepalive 连接池）；`auth.bench.test:8443` 为 Authelia 门户 | 不加认证 | auth_basic bcrypt | 8446 auth_basic apr1；8447 Authelia 官方片段写法（proxy_pass 直接写 URL，不复用连接） |
| caddy-authelia | forward_auth + Authelia；`auth.bench.test:8443` 为门户 | 不加认证 | basic_auth bcrypt | — |
| lucky | 子规则网页认证 WebAuth（会话 Cookie） | 不加认证 | 子规则基本认证 | — |

nginx/Caddy 与 Authelia 是两个容器，都限制在 vCPU 0,1，CPU 与内存按两者之和计（JSON 里 `cpuPercentBy`、`peakPssBy` 给出分项）。Authelia 配置见 `conf/authelia.yml`：file 用户后端、`one_factor`、会话存内存（未配 redis）、存储为本地 SQLite、regulation 用默认值。nginx 配置见 `conf/nginx.conf`，Caddy 见 `conf/Caddyfile`，Gately 与 Lucky 的配置由 `bench.py`、`lucky_setup.py` 生成。

### 各方的登录接口、Cookie 与密码哈希

| 被测 | 登录接口 | 成功响应与 Cookie | 默认密码哈希 |
|---|---|---|---|
| Gately | `POST https://bench.test:8443/__gately_auth/login`，表单 `username`、`password`、`next` | 302，`gately_site_<12 位十六进制>`（按监听/站点作用域取 SHA-256 前 6 字节），HMAC 签名，有效期 7 天 | bcrypt cost 10（`golang.org/x/crypto/bcrypt` DefaultCost；共享用户、站点用户相同）。测试中共享用户的哈希直接写入配置，成本与接口保存明文时生成的相同 |
| Authelia | `POST https://auth.bench.test:8443/api/firstfactor`，JSON `{"username","password","keepMeLoggedIn":false,"targetURL"}` | 200，`authelia_session`，会话存 Authelia 内存 | argon2id，t=3、m=65536 KiB、p=4、密钥 32 字节、盐 16 字节（`authelia crypto hash generate argon2` 的默认值，校验时按哈希串自带参数计算） |
| nginx auth_basic | 每个请求带 `Authorization: Basic` | — | bcrypt cost 10（与 Gately 同成本；`htpasswd -B` 默认只有 cost 5）与 apr1（MD5，1000 轮） |
| Caddy basic_auth | 同上 | — | bcrypt cost 10 |
| Lucky | 先 `GET <登录页>/webauth_0xa2c2a_/api/login/challenge` 取 RSA-2048 公钥与 nonce，再 `POST .../api/login`（`{challengeId, cipherText}`，密文为 PKCS#1 v1.5 加密的账号/密码/nonce）；登录页路径从访问站点得到的跳转页中取 | `ret=0`，`LuckyWebAuthBrowserSession_<子规则标识>` | 无哈希：用户以 `用户名:密码` 明文保存在子规则 `BasicAuthUserList`，直接比较 |

哈希由 `bench.py prepare()` 一次性生成（`authelia crypto hash generate` 与 `openssl passwd -apr1`），所有被测用同一个明文密码。

## 测试项与口径

每个被测单独运行，测完删除容器后再测下一个；被测之间、错误密码各轮之间都重新启动（配置目录重建，防爆破状态不残留）。

1. **已登录请求**：`wrk -t4 -c128 -d20s --latency --timeout 10s`，带 `Cookie`（登录后取得）或 `Authorization: Basic`，每项 3 轮取 rps 中位数的那一轮。同时测同一进程不加认证的端口作对照。CPU 为 cgroup `usage_usec` 差值（多容器求和），内存为 cgroup 内进程 PSS 之和的峰值（每 0.5 秒采样，多容器先求和再取峰值），与 `lib/run.py`、`lib/sample.py` 口径一致。wrk 加 `--timeout 10s` 是为了让 nginx bcrypt 这类每请求数十毫秒的场景不被默认 2 秒超时截断。
2. **登录吞吐（正确密码）**：Gately、Authelia 用 `oha -c 8 -z 20s -m POST --redirect 0`，只把成功状态码（Gately 302、Authelia 200）计为成功登录，结果 `okPerSec`；3 轮取中位数。另以 32 并发跑 1 轮 10 秒（`loginOkHighConc`），观察高并发下的拒绝与内存。Lucky 的挑战式登录 oha 做不到，用 `loginload.py`（8 个进程，各一条 keep-alive 连接，每次登录 2 个请求：取挑战 + 提交），`reasons` 记录失败原因。
3. **错误密码**：新启动被测，先正常登录一次拿到 Cookie，再以 8 并发连续提交错误密码 10 秒，记录处理速度、状态码分布（Lucky 为失败原因）、CPU/内存；之后用正确密码再登录一次（`correctLoginAfter`，看防爆破是否连正确密码一起拦），并用之前的 Cookie 访问（`existingCookieAfter`，已登录用户是否受影响）。3 轮各自重启。
4. **空闲**：启动就绪后等 10 秒、在任何登录之前取 PSS、进程数（`idle`）；另记首次登录后（`idleAfterFirstLogin`）、wrk 测完后、登录压测后（`idleAfterLogins`，含大量会话时的内存）。镜像体积为 `docker image inspect` 的 Size，多容器求和。

## 已知行为（试跑中确认，压测机数值以正式结果为准）

- **Gately 网页认证的防爆破按来源 IP 计数**（IPv6 按 /64）：15 分钟内失败次数加上正在校验的请求数达到 10 即拒绝，返回 401 与「失败次数过多，请稍后再试」，此时正确密码也被拒；成功登录不计数也不清零。`autoBanSeconds` 默认 0，不写封禁表。因为「正在校验」也计入上限，**同一 IP 同时超过 10 个正确密码的登录请求会被拒**，提示同样是「失败次数过多」（试跑：并发 30 次正确登录，10 次 302、20 次 401「失败次数过多」；随后单次登录正常）。所以登录吞吐用 8 并发，32 并发一项会看到大量 401。
- **Gately 基本认证会缓存验证通过的凭据**（10 分钟，最多 1024 条，键为 HMAC），之后同一凭据不再做 bcrypt；nginx auth_basic 每个请求都调用 `crypt()`，bcrypt cost 10 时吞吐只有个位到两位数 rps。Caddy basic_auth 也缓存哈希比较结果。
- **Authelia**：失败登录会被刻意延迟（防计时攻击），8 并发时错误密码的 p50 约 1 秒；regulation 默认按用户名在 2 分钟内失败 3 次封禁 5 分钟，之后正确密码也登不上，已登录的 Cookie 不受影响。argon2id 每次校验分配 64 MiB，并发登录时 Authelia 内存按并发数成倍上升（试跑 8 并发峰值约 1.1 GiB，32 并发约 3.1 GiB，见结果 `peakPssBy`）。
- **Lucky**：顺序 15 次错误密码后正确密码仍能登录（未见按次数锁定）；并发登录时返回 429「too many concurrent login attempts / pre-login requests」，持续高并发后出现「create webauth session error」（试跑中 32 并发时全部失败）。wrk 压 Lucky 时会出现少量 `connect` 错误。

## 限制

- 已登录请求只测 HTTP/1.1 keep-alive 下复用同一个 Cookie / 凭据；没有测大量不同用户、会话过期续期、TLS 新建连接等情况。
- 登录吞吐取决于各自默认的哈希成本，比较的是「默认配置下」的登录开销：bcrypt cost 10 与 argon2id(64 MiB) 的成本不同，Lucky 不做哈希。nginx/Caddy 的 bcrypt 成本是本测试统一选的 10，不是工具默认值。
- 所有请求来自 127.0.0.1，按 IP 计数的防爆破（Gately、Lucky）在测试中对全部请求生效；Authelia 按用户名计数。真实环境多来源时行为不同。
- Lucky 的登录是两次请求加客户端 RSA 加密，客户端为 Python 多进程，`okPerSec` 与 oha 测的单请求登录不完全同口径。
- Authelia 不提供纯内存的存储后端，这里用本地 SQLite（认证日志写库）；会话存内存。
- colima（arm64、宿主机负载不稳定）上的数值波动很大（同一配置 nginx 不加认证 34k～66k rps），且 wrk 偶发在结束时报出与连接数相同的 `timeout` 计数（Gately、nginx、Caddy 都出现过，不影响 rps），只用于验证流程。

## 文件

- `run.sh`：入口，把采样与压测进程固定在 vCPU 4-7。
- `bench.py`：启动被测、功能核对、压测、采样、写结果与汇总；复用 `lib/run.py` 的 wrk 解析与 `lib/sample.py` 的采样函数。
- `conf/nginx.conf`、`conf/Caddyfile`、`conf/authelia.yml`：配置模板（`@BACKEND@` 等占位符由 bench.py 替换）。
- `lucky_setup.py`：经 Lucky 管理接口建证书与三个监听规则。
- `loginload.py`：Lucky 挑战式登录的压测客户端。
- `colima.sh`：本机 colima 试跑。
