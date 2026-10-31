# Metriken (Prometheus / Grafana)

Gately exportiert unter `/api/metrics` globale Metriken im Prometheus-Textformat (0.0.4). Sie decken den Prozess, Portweiterleitung, STUN, DDNS, Zertifikate und Web-Dienste ab.

## Scrape-Konfiguration

1. Legen Sie auf der Seite „API-Tokens“ ein Token an, das nur die Leseberechtigung für „Metriken“ (`metrics:read`) hat. Dieses Token erreicht keinen anderen Endpunkt.
2. Ergänzen Sie in Prometheus unter `scrape_configs`:

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

## Metriken

| Metrik | Typ | Beschreibung |
| --- | --- | --- |
| `gately_build_info{version,commit,goversion}` | gauge | Build-Informationen, immer 1 |
| `gately_uptime_seconds` | gauge | Laufzeit des Prozesses |
| `gately_goroutines` / `gately_memory_alloc_bytes` | gauge | Goroutinen und Heap-Speicher |
| `gately_cert_days_remaining{cert}` | gauge | Tage bis zum Ablauf des Zertifikats |
| `gately_cert_expiry_timestamp_seconds{cert}` | gauge | Ablaufzeitpunkt des Zertifikats |
| `gately_ddns_record_up{task,record,type}` | gauge | Ob die letzte Synchronisierung erfolgreich war (1 = Erfolg) |
| `gately_ddns_last_update_timestamp_seconds{...}` | gauge | Wann der Eintrag zuletzt geändert wurde |
| `gately_stun_up{rule}` | gauge | Ob die STUN-Regel eine funktionierende Zuordnung hat |
| `gately_stun_connections_total{rule}` | counter | Von der integrierten STUN-Weiterleitung verarbeitete Verbindungen |
| `gately_portforward_active_connections{rule}` | gauge | Offene Verbindungen einer Weiterleitungsregel |
| `gately_portforward_connections_total{rule}` | counter | Angenommene Verbindungen einer Weiterleitungsregel |
| `gately_portforward_bytes_total{rule,direction}` | counter | Weitergeleitete Bytes (`in` = Client zum Ziel) |
| `gately_protoroute_active_connections{rule}` | gauge | Offene Verbindungen einer Protokoll-Routing-Regel |
| `gately_protoroute_connections_total{rule}` | counter | Angenommene Verbindungen einer Protokoll-Routing-Regel |
| `gately_protoroute_bytes_total{rule,direction}` | counter | Von einer Protokoll-Routing-Regel weitergeleitete Bytes (`in` = Client zum Ziel) |
| `gately_protoroute_route_connections_total{rule,route}` | counter | An jede Route gesendete Verbindungen; `route` ist der Routenname (bei unbenannten Routen ihre Nummer), `default` steht für das Standardziel |
| `gately_site_requests_total{listener,site}` | counter | Anfragen pro Site |
| `gately_site_responses_total{listener,site,class}` | counter | Antworten nach Statusklasse (2xx/3xx/4xx/5xx) |
| `gately_site_response_bytes_total` / `gately_site_errors_total` / `gately_site_active_requests` | counter / counter / gauge | Antwort-Bytes, Backend-Fehler, laufende Anfragen |
| `gately_upstream_up{listener,site,upstream}` | gauge | Ob das Backend gesund ist |

## Beispiele für Alarme

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

## Grafana-Dashboard

Wählen Sie in Grafana „Import“, laden Sie [deploy/grafana/gately-dashboard.json](../../deploy/grafana/gately-dashboard.json) hoch und wählen Sie Ihre Prometheus-Datenquelle aus.
