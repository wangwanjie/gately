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
| `gately_site_blocked_total{listener,site,reason}` | counter | Blockierte Anfragen nach Grund (`ban`, `ip`, `country`, `rateLimit`, `waf`) |
| `gately_site_request_duration_seconds{listener,site,le}` | histogram | Histogramm der Zeit bis zum ersten Byte der Website in Sekunden, 13 `le`-Stufen von 0,005 bis 60; ohne WebSocket und andere Protokollwechsel sowie abgebrochene Anfragen |
| `gately_upstream_up{listener,site,upstream[,route]}` | gauge | Ob das Backend gesund ist; Backends pro Pfad und pro Benutzer tragen zusätzlich ein Label `route` (Pfadpräfix oder `@` plus Benutzerliste) |
| `gately_upstream_requests_total{listener,site,upstream[,route]}` | counter | An das Backend weitergeleitete Anfragen |
| `gately_upstream_response_bytes_total{listener,site,upstream[,route]}` | counter | Vom Backend gelieferte Bytes des Antwort-Bodys (ohne Daten nach einem WebSocket-Upgrade) |
| `gately_listener_requests_total{listener,action}` | counter | Anfragen, die der Listener selbst beantwortet: `https_redirect` für unverschlüsseltes HTTP, das auf HTTPS umgeleitet wird, `not_found` für die 404, wenn keine Site passt |
| `gately_host_cpu_usage_percent` / `gately_host_memory_used_percent` / `gately_host_disk_used_percent` / `gately_host_load1` | gauge | Auslastung von Host-CPU, Speicher und der Partition mit dem Konfigurationsverzeichnis sowie 1-Minuten-Last aus der letzten Messung (exportiert, solange das Modul Leistung aktiv ist; nicht lesbare Werte fehlen) |
| `gately_process_cpu_usage_percent` / `gately_process_resident_memory_bytes` / `gately_process_open_fds` | gauge | CPU des Gately-Prozesses (Anteil am gesamten Rechner), residenter Speicher und offene Dateideskriptoren aus der letzten Messung (exportiert, solange das Modul Leistung aktiv ist) |
| `gately_perf_alerts_firing{rule}` | gauge | Anzahl der Objekte, die je aktivierter Leistungs-Warnregel gerade ausgelöst sind |
| `gately_guard_banned` | gauge | IP-Adressen oder Netze, die aktuell in der Sperrliste stehen |
| `gately_guard_firewall_active` | gauge | Ob die Firewall-Regeln des Einbruchschutzes (nftables oder iptables) aktiv sind (1) oder nicht (0) |
| `gately_guard_dropped_packets_total{rule}` | counter | Von den Firewall-Regeln verworfene Pakete, pro Regel |
| `gately_guard_jail_matches_total{jail}` | counter | Von jeder Protokollregel erkannte Protokollzeilen |
| `gately_guard_jail_bans_total{jail}` | counter | Von jeder Protokollregel ausgelöste Sperren |
| `gately_knock_packets_total{result}` | counter | Knock-Pakete nach Ergebnis (`accepted`, `rejected`, `ignored`) |
| `gately_knock_grants` | gauge | Quelladressen, die aktuell eine Knock-Freigabe besitzen |
| `gately_knock_firewall_active` | gauge | Ob die Sperrregeln der Firewall für Port Knocking aktiv sind (1) oder nicht (0) |
| `gately_knock_firewall_dropped_total` | counter | Von der Firewall verworfene neue Verbindungen zu geschützten Ports |

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
      - alert: GatelySiteSlow
        expr: histogram_quantile(0.95, sum by (le, listener, site) (rate(gately_site_request_duration_seconds_bucket[5m]))) > 2
        for: 5m
```

## Grafana-Dashboard

Wählen Sie in Grafana „Import“, laden Sie [deploy/grafana/gately-dashboard.json](../../deploy/grafana/gately-dashboard.json) hoch und wählen Sie Ihre Prometheus-Datenquelle aus.
