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
| `gately_upstream_up{listener,site,upstream}` | gauge | Whether the upstream is healthy |

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
```

## Grafana dashboard

In Grafana choose Import, upload [deploy/grafana/gately-dashboard.json](../../deploy/grafana/gately-dashboard.json) and select your Prometheus data source.
