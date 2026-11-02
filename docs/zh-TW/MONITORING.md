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
| `gately_site_blocked_total{listener,site,reason}` | counter | 依原因（`ban`、`ip`、`country`、`rateLimit`、`waf`）統計的攔截次數 |
| `gately_site_request_duration_seconds{listener,site,le}` | histogram | 站台首位元組延遲直方圖（秒），`le` 取 0.005 到 60 共 13 檔；不含 WebSocket 等協定升級與被中斷的請求 |
| `gately_upstream_up{listener,site,upstream[,route]}` | gauge | 後端是否健康；依路徑分流與依使用者分流的後端另帶 `route` 標籤（路徑前綴或 `@使用者列表`） |
| `gately_upstream_requests_total{listener,site,upstream[,route]}` | counter | 轉發到後端的累計請求數 |
| `gately_upstream_response_bytes_total{listener,site,upstream[,route]}` | counter | 後端回傳的累計回應主體位元組數（不含 WebSocket 升級後的資料） |
| `gately_listener_requests_total{listener,action}` | counter | 監聽層直接處理的請求：`https_redirect` 為明文請求跳轉 HTTPS，`not_found` 為未匹配站台的 404 |
| `gately_host_cpu_usage_percent` / `gately_host_memory_used_percent` / `gately_host_disk_used_percent` / `gately_host_load1` | gauge | 最近一次取樣的主機 CPU、記憶體、設定目錄所在分割區的使用率與 1 分鐘負載（效能監控模組開啟時匯出，取不到的項目不匯出） |
| `gately_process_cpu_usage_percent` / `gately_process_resident_memory_bytes` / `gately_process_open_fds` | gauge | 最近一次取樣的 Gately 程序 CPU（佔整機算力的比例）、常駐記憶體與開啟的檔案描述元數（效能監控模組開啟時匯出） |
| `gately_perf_alerts_firing{rule}` | gauge | 各條已啟用的效能告警規則目前處於告警中的對象數 |

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
      - alert: GatelySiteSlow
        expr: histogram_quantile(0.95, sum by (le, listener, site) (rate(gately_site_request_duration_seconds_bucket[5m]))) > 2
        for: 5m
```

## Grafana 面板

在 Grafana 中選擇 Import，上傳 [deploy/grafana/gately-dashboard.json](../../deploy/grafana/gately-dashboard.json)，並選擇你的 Prometheus 資料來源。
