# 監控指標（Prometheus / Grafana）

Gately 在 `/api/metrics` 以 Prometheus 文字格式（0.0.4）匯出全域指標，涵蓋行程狀態、連接埠轉發、STUN、DDNS、憑證與 Web 服務。

## 抓取設定

1. 在「API 權杖」頁建立一個權杖，只勾選「監控指標」的唯讀權限（`metrics:read`），該權杖無法存取其他介面。
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

## 指標一覽

| 指標 | 類型 | 說明 |
| --- | --- | --- |
| `gately_build_info{version,commit,goversion}` | gauge | 建置資訊，值恆為 1 |
| `gately_uptime_seconds` | gauge | 行程執行時間 |
| `gately_goroutines` / `gately_memory_alloc_bytes` | gauge | 協程數與堆積記憶體 |
| `gately_cert_days_remaining{cert}` | gauge | 憑證剩餘天數 |
| `gately_cert_expiry_timestamp_seconds{cert}` | gauge | 憑證到期時間 |
| `gately_ddns_record_up{task,record,type}` | gauge | 最近一次同步是否成功（1 為成功） |
| `gately_ddns_last_update_timestamp_seconds{...}` | gauge | 記錄最近一次被更新的時間 |
| `gately_stun_up{rule}` | gauge | STUN 規則是否已取得有效對應 |
| `gately_stun_connections_total{rule}` | counter | STUN 內建轉發處理的連線數 |
| `gately_portforward_active_connections{rule}` | gauge | 連接埠轉發目前連線數 |
| `gately_portforward_connections_total{rule}` | counter | 連接埠轉發累計連線數 |
| `gately_portforward_bytes_total{rule,direction}` | counter | 連接埠轉發累計流量（`in` 為用戶端到目標） |
| `gately_protoroute_active_connections{rule}` | gauge | 協定分流目前連線數 |
| `gately_protoroute_connections_total{rule}` | counter | 協定分流累計連線數 |
| `gately_protoroute_bytes_total{rule,direction}` | counter | 協定分流累計流量（`in` 為用戶端到目標） |
| `gately_protoroute_route_connections_total{rule,route}` | counter | 協定分流各路由累計連線數，`route` 為路由名稱（未命名時為序號），預設目標為 `default` |
| `gately_site_requests_total{listener,site}` | counter | 站台累計請求數 |
| `gately_site_responses_total{listener,site,class}` | counter | 依狀態類別（2xx/3xx/4xx/5xx）統計的回應數 |
| `gately_site_response_bytes_total` / `gately_site_errors_total` / `gately_site_active_requests` | counter / counter / gauge | 站台回應位元組數、後端錯誤數、進行中的請求數 |
| `gately_upstream_up{listener,site,upstream}` | gauge | 後端是否健康 |

## 告警範例

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

在 Grafana 中選擇 Import，上傳 [deploy/grafana/gately-dashboard.json](../../deploy/grafana/gately-dashboard.json)，並選擇你的 Prometheus 資料來源。
