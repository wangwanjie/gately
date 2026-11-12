# Metrics (Prometheus / Grafana)

Gately exports global metrics in the Prometheus text format (0.0.4) at `/api/metrics`, covering the process, port forwarding, STUN, DDNS, certificates and web services.

## Scrape configuration

1. On the "API Tokens" page, create a token with only the read permission of "Metrics" (`metrics:read`). That token cannot reach any other endpoint.
2. Add to `scrape_configs` in Prometheus:

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

## Metrics

| Metric | Type | Description |
| --- | --- | --- |
| `gately_build_info{version,commit,goversion}` | gauge | Build information, always 1 |
| `gately_uptime_seconds` | gauge | Process uptime |
| `gately_goroutines` / `gately_memory_alloc_bytes` | gauge | Goroutines and heap memory |
| `gately_cert_days_remaining{cert}` | gauge | Days until the certificate expires |
| `gately_cert_expiry_timestamp_seconds{cert}` | gauge | Certificate expiry time |
| `gately_ddns_record_up{task,record,type}` | gauge | Whether the last sync succeeded (1 = success) |
| `gately_ddns_last_update_timestamp_seconds{...}` | gauge | When the record was last changed |
| `gately_stun_up{rule}` | gauge | Whether the STUN rule has a working mapping |
| `gately_stun_connections_total{rule}` | counter | Connections handled by the built-in STUN forwarder |
| `gately_portforward_active_connections{rule}` | gauge | Open connections of a forwarding rule |
| `gately_portforward_connections_total{rule}` | counter | Accepted connections of a forwarding rule |
| `gately_portforward_bytes_total{rule,direction}` | counter | Relayed bytes (`in` = client to target) |
| `gately_protoroute_active_connections{rule}` | gauge | Open connections of a protocol routing rule |
| `gately_protoroute_connections_total{rule}` | counter | Accepted connections of a protocol routing rule |
| `gately_protoroute_bytes_total{rule,direction}` | counter | Bytes relayed by a protocol routing rule (`in` = client to target) |
| `gately_protoroute_route_connections_total{rule,route}` | counter | Connections sent to each route; `route` is the route name (its number when unnamed), `default` for the default target |
| `gately_site_requests_total{listener,site}` | counter | Requests per site |
| `gately_site_responses_total{listener,site,class}` | counter | Responses by status class (2xx/3xx/4xx/5xx) |
| `gately_site_response_bytes_total` / `gately_site_errors_total` / `gately_site_active_requests` | counter / counter / gauge | Response bytes, backend errors, in-flight requests |
| `gately_site_blocked_total{listener,site,reason}` | counter | Blocked requests by reason (`ban`, `ip`, `country`, `rateLimit`, `waf`) |
| `gately_site_request_duration_seconds{listener,site,le}` | histogram | Histogram of the site's time to first byte in seconds, 13 `le` buckets from 0.005 to 60; WebSocket and other upgraded requests and aborted requests are excluded |
| `gately_upstream_up{listener,site,upstream[,route]}` | gauge | Whether the upstream is healthy; path-based and per-user backends also carry a `route` label (path prefix or `@` plus the user list) |
| `gately_upstream_requests_total{listener,site,upstream[,route]}` | counter | Requests forwarded to the upstream |
| `gately_upstream_response_bytes_total{listener,site,upstream[,route]}` | counter | Response body bytes returned by the upstream (excluding data after a WebSocket upgrade) |
| `gately_listener_requests_total{listener,action}` | counter | Requests the listener handles itself: `https_redirect` for plain HTTP redirected to HTTPS, `not_found` for the 404 when no site matches |
| `gately_host_cpu_usage_percent` / `gately_host_memory_used_percent` / `gately_host_disk_used_percent` / `gately_host_load1` | gauge | Host CPU, memory and config-directory partition usage and 1 minute load from the latest sample (exported while the Performance module is on; values that cannot be read are omitted) |
| `gately_process_cpu_usage_percent` / `gately_process_resident_memory_bytes` / `gately_process_open_fds` | gauge | CPU of the Gately process (share of the whole machine), resident memory and open file descriptors from the latest sample (exported while the Performance module is on) |
| `gately_perf_alerts_firing{rule}` | gauge | Number of objects currently firing for each enabled performance alert rule |
| `gately_guard_banned` | gauge | IP addresses or networks currently in the ban list |
| `gately_guard_firewall_active` | gauge | Whether the intrusion prevention firewall rules (nftables or iptables) are in place (1) or not (0) |
| `gately_guard_dropped_packets_total{rule}` | counter | Packets dropped by the firewall rules, per rule |
| `gately_guard_jail_matches_total{jail}` | counter | Log lines matched by each log rule |
| `gately_guard_jail_bans_total{jail}` | counter | Bans triggered by each log rule |
| `gately_knock_packets_total{result}` | counter | Knock packets by result (`accepted`, `rejected`, `ignored`) |
| `gately_knock_grants` | gauge | Source addresses currently holding a knock grant |
| `gately_knock_firewall_active` | gauge | Whether the port knocking firewall blocking rules are in place (1) or not (0) |
| `gately_knock_firewall_dropped_total` | counter | New connections to protected ports dropped by the firewall |

## Alert examples

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

## Grafana dashboard

In Grafana choose Import, upload [deploy/grafana/gately-dashboard.json](../../deploy/grafana/gately-dashboard.json) and select your Prometheus data source.
