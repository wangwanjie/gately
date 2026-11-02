# 监控指标（Prometheus / Grafana）

Gately 在 `/api/metrics` 以 Prometheus 文本格式（0.0.4）导出全局指标，覆盖进程状态、端口转发、STUN、DDNS、证书与 Web 服务。

## 抓取配置

1. 在「API 令牌」页创建一个令牌，只勾选「监控指标」的只读权限（`metrics:read`），该令牌无法访问其他接口。
2. 在 Prometheus 的 `scrape_configs` 中加入：

```yaml
scrape_configs:
  - job_name: gately
    metrics_path: /api/metrics
    scheme: http
    authorization:
      credentials: gtl_xxxx
    static_configs:
      - targets: ["gately.example.lan:16680"]
```

## 指标一览

| 指标 | 类型 | 说明 |
| --- | --- | --- |
| `gately_build_info{version,commit,goversion}` | gauge | 构建信息，值恒为 1 |
| `gately_uptime_seconds` | gauge | 进程运行时长 |
| `gately_goroutines` / `gately_memory_alloc_bytes` | gauge | 协程数与堆内存 |
| `gately_cert_days_remaining{cert}` | gauge | 证书剩余天数 |
| `gately_cert_expiry_timestamp_seconds{cert}` | gauge | 证书到期时间 |
| `gately_ddns_record_up{task,record,type}` | gauge | 最近一次同步是否成功（1 成功） |
| `gately_ddns_last_update_timestamp_seconds{...}` | gauge | 记录最近一次被更新的时间 |
| `gately_stun_up{rule}` | gauge | STUN 规则是否已获得有效映射 |
| `gately_stun_connections_total{rule}` | counter | STUN 内置转发处理的连接数 |
| `gately_portforward_active_connections{rule}` | gauge | 端口转发当前连接数 |
| `gately_portforward_connections_total{rule}` | counter | 端口转发累计连接数 |
| `gately_portforward_bytes_total{rule,direction}` | counter | 端口转发累计流量（`in` 为客户端到目标） |
| `gately_protoroute_active_connections{rule}` | gauge | 协议分流当前连接数 |
| `gately_protoroute_connections_total{rule}` | counter | 协议分流累计连接数 |
| `gately_protoroute_bytes_total{rule,direction}` | counter | 协议分流累计流量（`in` 为客户端到目标） |
| `gately_protoroute_route_connections_total{rule,route}` | counter | 协议分流各路由累计连接数，`route` 为路由名称（未命名时为序号），默认目标为 `default` |
| `gately_site_requests_total{listener,site}` | counter | 站点累计请求数 |
| `gately_site_responses_total{listener,site,class}` | counter | 按状态类别（2xx/3xx/4xx/5xx）的响应数 |
| `gately_site_response_bytes_total` / `gately_site_errors_total` / `gately_site_active_requests` | counter / counter / gauge | 站点响应字节数、后端错误数、进行中请求数 |
| `gately_site_blocked_total{listener,site,reason}` | counter | 按原因（`ban`、`ip`、`country`、`rateLimit`、`waf`）统计的拦截次数 |
| `gately_site_request_duration_seconds{listener,site,le}` | histogram | 站点首字节延迟直方图（秒），`le` 取 0.005 到 60 共 13 档；不含 WebSocket 等协议升级与被断开的请求 |
| `gately_upstream_up{listener,site,upstream[,route]}` | gauge | 后端是否健康；分路径与按用户分流的后端另带 `route` 标签（路径前缀或 `@用户列表`） |
| `gately_upstream_requests_total{listener,site,upstream[,route]}` | counter | 转发到后端的累计请求数 |
| `gately_upstream_response_bytes_total{listener,site,upstream[,route]}` | counter | 后端返回的累计响应体字节数（不含 WebSocket 升级后的数据） |
| `gately_listener_requests_total{listener,action}` | counter | 监听层直接处理的请求：`https_redirect` 明文跳转 HTTPS，`not_found` 未匹配站点的 404 |
| `gately_host_cpu_usage_percent` / `gately_host_memory_used_percent` / `gately_host_disk_used_percent` / `gately_host_load1` | gauge | 最近一次采样的主机 CPU、内存、配置目录所在分区的使用率与 1 分钟负载（性能监控模块开启时导出，取不到的项不导出） |
| `gately_process_cpu_usage_percent` / `gately_process_resident_memory_bytes` / `gately_process_open_fds` | gauge | 最近一次采样的 Gately 进程 CPU（占整机算力的比例）、常驻内存与打开的文件描述符数（性能监控模块开启时导出） |
| `gately_perf_alerts_firing{rule}` | gauge | 各条已启用的性能告警规则当前处于告警中的对象数 |

## 告警示例

```yaml
groups:
  - name: gately
    rules:
      - alert: GatelyCertExpiring
        expr: gately_cert_days_remaining < 14
        for: 1h
      - alert: GatelyDDNSFailing
        expr: gately_ddns_record_up == 0
        for: 15m
      - alert: GatelyUpstreamDown
        expr: gately_upstream_up == 0
        for: 2m
      - alert: GatelySiteSlow
        expr: histogram_quantile(0.95, sum by (le, listener, site) (rate(gately_site_request_duration_seconds_bucket[5m]))) > 2
        for: 5m
```

## Grafana 面板

在 Grafana 中选择 Import，上传 [deploy/grafana/gately-dashboard.json](../deploy/grafana/gately-dashboard.json)，并选择你的 Prometheus 数据源。
