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
| `gately_upstream_up{listener,site,upstream}` | gauge | 后端是否健康 |

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
```

## Grafana 面板

在 Grafana 中选择 Import，上传 [deploy/grafana/gately-dashboard.json](../deploy/grafana/gately-dashboard.json)，并选择你的 Prometheus 数据源。
