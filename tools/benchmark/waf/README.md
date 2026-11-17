# WAF 专项对比

对比三种“反向代理 + Coraza/ModSecurity + OWASP CRS”的组合在正常流量下的性能开销、检测效果、恶意流量下的性能，以及空闲内存与规则加载时间。

| 名称（suts/ 目录） | 程序 | WAF 引擎 | 规则集 | 镜像 |
|---|---|---|---|---|
| `gately` | Gately v0.6.8.1 | Coraza v3.8.1（内置于 Gately） | coraza-coreruleset v4.25.0 = CRS 4.25.0 | `ghcr.io/wangwanjie/gately:v0.6.8.1` |
| `modsec` | nginx 1.30.3 | ModSecurity v3.0.16 + ModSecurity-nginx v1.0.4 | CRS 4.25.0 | `owasp/modsecurity-crs:4.25.0-nginx-alpine-202606290906` |
| `caddy-coraza` | Caddy v2.11.4 | coraza-caddy v2.6.2 + Coraza v3.8.1 | coraza-coreruleset v4.25.0 = CRS 4.25.0 | `bench/caddy-coraza:2.11.4-c2.6.2`（本目录 `suts/caddy-coraza/build.sh` 构建） |

三方 CRS 版本完全相同（4.25.0）。Gately 与 Caddy 用的是同一个 Coraza 版本、同一个 Go 版本（1.26.11），差别只在宿主程序与 Gately 自己加的几条调整，适合用来区分“Coraza 引擎本身的开销”和“Gately 集成方式的开销”。Traefik 的 Coraza 插件以 Yaegi 解释执行，性能不具代表性，未纳入。

## 对齐的设置

| 设置 | Gately（界面新建实例的默认值） | nginx + ModSecurity（环境变量） | Caddy + coraza-caddy（`waf-on.caddy`） |
|---|---|---|---|
| 引擎模式 | `mode: block` → `SecRuleEngine On` | `MODSEC_RULE_ENGINE=on` | `SecRuleEngine On` |
| 偏执等级 | `paranoia: 1`（blocking PL1） | `BLOCKING_PARANOIA=1`、`DETECTION_PARANOIA=1` | `tx.blocking_paranoia_level=1` |
| 入站/出站异常阈值 | 5 / 4 | `ANOMALY_INBOUND=5`、`ANOMALY_OUTBOUND=4` | 5 / 4 |
| 请求体检查 | 开，上限 12.5 MB，超限拒绝 | `MODSEC_REQ_BODY_ACCESS=on`、`MODSEC_REQ_BODY_LIMIT=13107200`、`Reject` | `coraza.conf-recommended`：开，12.5 MB，Reject |
| 响应体检查 | 关（Gately 固定关闭） | `MODSEC_RESP_BODY_ACCESS=off` | `SecResponseBodyAccess Off` |
| 审计日志 | 无 | `MODSEC_AUDIT_ENGINE=off` | `SecAuditEngine Off` |
| 规则文件 | 全部 CRS 规则 | 镜像内全部 CRS 规则 | `Include @owasp_crs/*.conf` |
| 规则命中日志 | 写入内存日志缓冲（管理界面查看） | nginx error_log，写容器内 `/tmp/nginx-error.log` | Caddy 日志，写容器内 `/tmp/caddy.log` |

已知差异（没有强行抹平，结果解读时要考虑）：

- Gately 在 CRS 之外默认多了三处调整：开启 `SecRxPreFilter`（正则预过滤，Gately 代码注释称可降约 16% 检测 CPU，并已知在 CRS 回归用例 942220 #2 上会漏判）；规则 930120 不检查 JSON 请求体的字段名（减少误报）；`tx.restricted_headers_basic` 去掉 `/expect/`（放行 `Expect: 100-continue`）。Caddy 用的是 coraza-caddy 文档推荐的原样 CRS 配置，不含这些调整，所以 Gately 与 Caddy 的检测差异基本来自这三处。要测关闭预过滤的 Gately：`GATELY_PREFILTER=off bash run.sh`。
- coraza-caddy 默认把 Coraza 调试日志接到 Caddy 日志，正常请求也会每次写 3 行 `key not found in collection` 警告；Gately 不接调试日志。Caddy 配置里加了 `SecDebugLogLevel 0` 关掉它，规则命中日志照常写。
- ModSecurity 镜像另有 `SecRequestBodyNoFilesLimit 131072`（非上传请求体超过 128 KB 拒绝），Coraza 不支持该指令。本测试的样本与压测请求体都小于 10 KB，不受影响。
- 三方日志写法不同（见上表）：Gately 写内存，另两方写容器内文件；写日志的开销都在被测容器的 cgroup 内，计入 CPU。
- “关闭 WAF”的对照组用同一程序、同一镜像：Gately 不建 WAF 实例、站点不引用；nginx 去掉 `modsecurity` 两条指令（模块仍加载、不加载规则）；Caddy 不写 `coraza_waf`（模块仍编译在内）。

网络与代理写法沿用 `../suts/` 的同名程序：HTTP 8080、HTTPS 8443、域名 bench.test、反代到 `127.0.0.1:9000`，nginx 2 个 worker 并对源站开 keepalive 连接池，被测容器 `--cpuset-cpus 0,1`、`--network host`。压测只走 HTTP 8080（避免 TLS 开销冲淡 WAF 的差别），HTTPS 只在启动后检查一次可用。

## 指标与口径

每个程序依次以“关闭 WAF”“开启 WAF”两种配置各跑一遍，互不同时运行。

1. **正常流量开销**（每项 3 轮 × 20 秒，按 rps 取中位数那一轮）：`wrk -t4 -c128 --timeout 10s`，经 Lua 脚本发送带浏览器常见请求头（User-Agent、Accept、Accept-Language）的请求：
   - `get`：`GET /`（源站返回 1024 字节）；
   - `query`：`GET /?id=12345&name=alice&q=hello+world&page=2&sort=desc&lang=zh-CN&ts=1763386400&ref=home`；
   - `post4k`：`POST /`，约 4 KB 的 `application/x-www-form-urlencoded` 表单（40 余个字段的普通英文文本）。源站是静态 nginx，对 POST 返回 405，三方相同；WAF 在转发前已完整检查请求体，不影响口径。
   - 记录 rps、p50/p99、被测容器 CPU%（cgroup `usage_usec` 差值 / 时长）、每千请求 CPU 毫秒、峰值 PSS（`../lib/sample.py`，0.5 秒采样）。下降比例 = 1 − 开启 WAF 的 rps / 关闭 WAF 的 rps。
2. **检测效果**：把三组样本逐条发给开启与关闭 WAF 的程序（`tools/replay.py`，每条单独建连、请求原样发送、并发 8），记录状态码。结果是确定的，只跑一轮。
   - 拦截 = 开启 WAF 时返回 403，或返回关闭 WAF 时没有出现的错误（4xx/5xx 中除源站常见的 404/405，连接被断开记为 0）。后者主要是 ModSecurity 解析请求体失败时按 `modsecurity.conf` 的 200002 规则返回的 400，以及三方解析畸形 XML 时的 400。先把样本直接发给源站一次，源站本身返回 403 的样本排除（目前没有）。
   - “代理本身拒绝”= 开启与关闭 WAF 时返回同一个错误码，是代理自己的协议解析拒绝，不计入拦截，单独列出。
   - 真阳性率 = 攻击集拦截数 / 攻击样本数；误报率 = 正常集拦截数 / 正常样本数。另列出三方结论不一致的样本（前 15 条，完整 ID 在 `waf-summary.json`），便于逐条核对。
3. **恶意流量下的性能**：`attack` 压测项，wrk 轮流发送全部 557 条攻击样本（各线程起点错开），开启与关闭 WAF 各测一次，记录 rps、CPU 与峰值内存。约八成样本会被拦截（403 不经源站），其余透传。
4. **空闲内存与启动时间**：每种配置启动 3 次，取“从 `docker run` 到 `GET /` 返回源站内容（开启 WAF 时还要求探测攻击 `/?q=<script>alert(1)</script>` 返回 403，即规则已加载生效）”的中位数；规则加载时间取开启与关闭 WAF 的差值（同一镜像，入口脚本等其他开销相同）。启动后空闲 10 秒采样 PSS；压测与回放结束 5 秒后再采一次（反映 Go 程序 GC 后保留的堆）。

## 样本集（`samples/`，由 `tools/gen_samples.py` 生成，已提交，压测机直接使用）

| 文件 | 条数 | 来源与许可 | 说明 |
|---|---|---|---|
| `attack.jsonl` | 557 | [gotestwaf](https://github.com/wallarm/gotestwaf)（MIT，提交 5daeba0）`testcases/owasp`、`owasp-api/rest.yml`、`community`；手写经典载荷 | SQLi、XSS、命令注入、路径穿越/LFI、RFI、CRLF、LDAP、邮件头注入、NoSQL、SSI、SSTI、XXE、扫描器 UA |
| `benign.jsonl` | 389 | gotestwaf `testcases/false-pos/texts.yml`（MIT）；手写 | 普通浏览（静态资源、带参数的 API、搜索、Cookie、Bearer 令牌）、JSON API（含 PUT/PATCH/DELETE）、技术内容（带 SQL/Shell 关键字的文章、代码片段、HTML 片段）、表单与上传 |
| `crs-regression.jsonl` | 2049 | CRS 4.25.0 自带回归测试（Apache-2.0，取自 `github.com/corazawaf/coraza-coreruleset/v4@v4.25.0/tests`） | 针对 PL1 规则、期望命中的单阶段用例；Host 统一改为 bench.test（原用例依赖特殊 Host 的 38 条跳过）、原始请求（encoded_request）、关闭自动补头与非 HTTP/1.x 的 14 条跳过 |

gotestwaf 载荷按其占位符语义展开（`URLParam`、`URLPath`、`Header`、`UserAgent`、`HTMLForm`、`HTMLMultipartForm`、`JSONRequest`、`XMLBody`；URL 编码器 = Go `url.PathEscape`，JSON 值按其 JSUnicode 编码器写成 `\uXXXX`），只用 `Plain` 与 `URL` 两种编码器：`Base64Flat` 编码的载荷 CRS 本来就不解码，测出来只是“三方都放过”；16 KB 及以上的超长载荷也不纳入（测的是请求体上限而不是检测）。gotestwaf 的 GraphQL、gRPC、SOAP、RawRequest 占位符未实现。

CRS 回归用例本是为 CRS 规则写的，期望的是“某条规则命中”，不等于“达到阈值被阻断”（单条 WARNING 级规则只加 3 分）。因此这组只用于比较三方引擎的一致性（ModSecurity 与 Coraza 对同一规则集的实现差异，以及 Gately 调整带来的差异），不作为检测率。

重新生成（开发机，需要 PyYAML 与 Go 模块缓存）：

```bash
git clone https://github.com/wallarm/gotestwaf /tmp/gotestwaf   # 提交 5daeba0
python3 tools/gen_samples.py /tmp/gotestwaf $(go env GOMODCACHE)/github.com/corazawaf/coraza-coreruleset/v4@v4.25.0 samples
```

## 运行

压测机（root，路径以 `/root/benchmark` 为根）：

```bash
bash /root/benchmark/waf/run.sh            # 三方依次运行，约 40~45 分钟
SUTS=gately bash /root/benchmark/waf/run.sh   # 只跑其中一个
```

结果：`/root/benchmark/results/waf-<程序>.json`（每种配置的启动、空闲、各压测项 3 轮明细与中位数、每条样本的状态码）、`waf-<程序>.log`、`waf-backend-*.json`（源站对样本的状态码）、`waf-summary.json` 与 `waf-report.md`（`report.py` 生成的汇总表）。

耗时估算：每个程序每种配置约 6~7 分钟（启动 3 次、空闲 10 秒、4 项 × 3 轮 × 25 秒、回放约 3000 条，开启 WAF 时回放约 1 分钟），两种配置约 13 分钟，三方合计约 40~45 分钟。本机 colima 以 `DUR=5 ROUNDS=1 STARTS=2` 跑完三方用时约 7 分钟。

需要事先传到压测机的镜像（固定 tag）：

| 镜像 | 说明 |
|---|---|
| `ghcr.io/wangwanjie/gately:v0.6.8.1` | 压测机已有 |
| `owasp/modsecurity-crs:4.25.0-nginx-alpine-202606290906` | Docker Hub 多架构镜像（amd64/arm64） |
| `bench/caddy-coraza:2.11.4-c2.6.2` | 开发机运行 `suts/caddy-coraza/build.sh` 构建（交叉编译，不需要模拟器），amd64 版导出到 `/tmp/caddy-coraza-amd64.tar`；压测机 `docker load -i caddy-coraza-amd64.tar && docker tag bench/caddy-coraza:2.11.4-c2.6.2-amd64 bench/caddy-coraza:2.11.4-c2.6.2`。基础镜像为 `caddy:2.11.4-alpine` |
| `nginx:1.29-alpine` | 源站，压测机已有 |

可调的环境变量见 `bench.py` 开头（端口、CPU、时长、轮数、只跑部分模式或压测项等）。本机 colima 调试用 `tools/local-colima.sh`（在特权驱动容器里运行，换用不冲突的端口与 CPU，需要先按脚本注释准备 `/tmp/bx-waf` 与源站容器 `bx-waf-backend`）。

## 解读要点（本机试跑已观察到的现象）

- 三方在攻击集与正常集上的结论几乎完全相同，检测效果基本由 CRS 决定；差异主要在 CRS 回归用例里：
  - Gately 漏判 `crs-942220-2`（`GET /get?i=2.2250738585072011e-308`，SQLi 规则 942220），Caddy 与 ModSecurity 都拦截。原因是 Gately 默认开启的 `SecRxPreFilter`：同一请求在 `regexPrefilter: false` 时被拦截（已验证）。Gately 源码注释里已记录这一点。
  - Gately 比另两方多拦 6 条（请求头 `X_Filename` 携带 `.php`、`.jsp`、`sess_` 文件名，规则 933110/933220/944140）。nginx 默认忽略含下划线的请求头，ModSecurity 看不到；Caddy 同样放过，原因未深究。
  - ModSecurity 有约 70 条以 400（请求体解析失败）拦截，已按上面的口径计为拦截。
- Coraza（Gately 与 Caddy 共用）检查请求体的代价明显高于 ModSecurity，且随请求体字节数增长：本机单请求 4 KB 表单 Gately 约 17~22 ms、ModSecurity 约 3~14 ms；`post4k` 压测下 Coraza 两方的峰值 PSS 都到 550~660 MB，ModSecurity 约 75 MB。这是引擎层面的特性，Gately 与 Caddy 表现相近。
- wrk 在压测结束时偶尔把仍在途的请求计为 `timeout`（本机只在 Gately 上见到，恰好等于连接数 128）。用独立的 Python 客户端以 128 条长连接持续请求 8 秒未见任何超时（最大延迟 44 ms），判断为 wrk 统计的边界效应，不影响 rps。

## 限制

- CRS 默认的误报本身就较高（例如 PUT/PATCH/DELETE 不在默认允许方法里，正常 REST 请求会被 911100 拦截；技术文章里的 SQL、Shell 片段会被当成注入）。这里测的是“开箱默认配置”的表现，三方都没有为样本做排除规则。
- 压测是闭环（wrk 128 条连接满负荷），WAF 很慢时 p50/p99 主要是排队时间，比较时以 rps 与每千请求 CPU 为主。
- 只测 HTTP/1.1 明文；HTTPS、HTTP/2 下 WAF 的额外开销相同，但占比会被 TLS 冲淡。
- 规则命中日志三方写法不同（见上），Gately 的写内存最省，这一项对 Gately 有利；`attack` 压测中三方都会大量写命中日志。
- CRS 回归用例的 Host 被统一改写，个别依赖 Host 的规则（如 920350）不在样本里。
- 样本的类别分布不均（gotestwaf 的 XSS 载荷最多），总体拦截率受类别权重影响，按类别看更可靠。
