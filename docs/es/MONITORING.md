# Métricas (Prometheus / Grafana)

Gately exporta métricas globales en el formato de texto de Prometheus (0.0.4) en `/api/metrics`, que cubren el proceso, el reenvío de puertos, STUN, DDNS, los certificados y los servicios web.

## Configuración del scrape

1. En la página «Tokens de API», crea un token que solo tenga el permiso de lectura de «Métricas» (`metrics:read`). Ese token no puede acceder a ningún otro endpoint.
2. Añade a `scrape_configs` en Prometheus:

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

## Métricas

| Métrica | Tipo | Descripción |
| --- | --- | --- |
| `gately_build_info{version,commit,goversion}` | gauge | Información de compilación, siempre 1 |
| `gately_uptime_seconds` | gauge | Tiempo en funcionamiento del proceso |
| `gately_goroutines` / `gately_memory_alloc_bytes` | gauge | Goroutines y memoria del heap |
| `gately_cert_days_remaining{cert}` | gauge | Días hasta que caduque el certificado |
| `gately_cert_expiry_timestamp_seconds{cert}` | gauge | Fecha de caducidad del certificado |
| `gately_ddns_record_up{task,record,type}` | gauge | Si la última sincronización fue correcta (1 = correcta) |
| `gately_ddns_last_update_timestamp_seconds{...}` | gauge | Cuándo se cambió el registro por última vez |
| `gately_stun_up{rule}` | gauge | Si la regla STUN tiene un mapeo operativo |
| `gately_stun_connections_total{rule}` | counter | Conexiones atendidas por el reenvío STUN integrado |
| `gately_portforward_active_connections{rule}` | gauge | Conexiones abiertas de una regla de reenvío |
| `gately_portforward_connections_total{rule}` | counter | Conexiones aceptadas de una regla de reenvío |
| `gately_portforward_bytes_total{rule,direction}` | counter | Bytes retransmitidos (`in` = del cliente al destino) |
| `gately_protoroute_active_connections{rule}` | gauge | Conexiones abiertas de una regla de enrutamiento por protocolo |
| `gately_protoroute_connections_total{rule}` | counter | Conexiones aceptadas de una regla de enrutamiento por protocolo |
| `gately_protoroute_bytes_total{rule,direction}` | counter | Bytes retransmitidos por una regla de enrutamiento por protocolo (`in` = del cliente al destino) |
| `gately_protoroute_route_connections_total{rule,route}` | counter | Conexiones enviadas a cada ruta; `route` es el nombre de la ruta (su número si no tiene nombre), `default` para el destino predeterminado |
| `gately_site_requests_total{listener,site}` | counter | Peticiones por sitio |
| `gately_site_responses_total{listener,site,class}` | counter | Respuestas por clase de estado (2xx/3xx/4xx/5xx) |
| `gately_site_response_bytes_total` / `gately_site_errors_total` / `gately_site_active_requests` | counter / counter / gauge | Bytes de respuesta, errores del backend, peticiones en curso |
| `gately_site_blocked_total{listener,site,reason}` | counter | Peticiones bloqueadas por motivo (`ban`, `ip`, `country`, `rateLimit`, `waf`) |
| `gately_site_request_duration_seconds{listener,site,le}` | histogram | Histograma del tiempo hasta el primer byte del sitio en segundos, 13 tramos `le` de 0.005 a 60; excluye WebSocket y otras peticiones con cambio de protocolo y las peticiones cortadas |
| `gately_upstream_up{listener,site,upstream[,route]}` | gauge | Si el upstream está sano; los backends por ruta y por usuario llevan además una etiqueta `route` (prefijo de ruta, o `@` más la lista de usuarios) |
| `gately_upstream_requests_total{listener,site,upstream[,route]}` | counter | Peticiones reenviadas al upstream |
| `gately_upstream_response_bytes_total{listener,site,upstream[,route]}` | counter | Bytes del cuerpo de respuesta devueltos por el upstream (sin los datos tras una actualización a WebSocket) |
| `gately_listener_requests_total{listener,action}` | counter | Peticiones que el listener atiende por sí mismo: `https_redirect` para HTTP sin cifrar redirigido a HTTPS, `not_found` para el 404 cuando ningún sitio coincide |
| `gately_host_cpu_usage_percent` / `gately_host_memory_used_percent` / `gately_host_disk_used_percent` / `gately_host_load1` | gauge | Uso de CPU, memoria y partición del directorio de configuración del host y carga de 1 minuto de la última muestra (se exportan con el módulo Rendimiento activado; los valores que no se pueden leer se omiten) |
| `gately_process_cpu_usage_percent` / `gately_process_resident_memory_bytes` / `gately_process_open_fds` | gauge | CPU del proceso de Gately (proporción de toda la máquina), memoria residente y descriptores de archivo abiertos de la última muestra (se exportan con el módulo Rendimiento activado) |
| `gately_perf_alerts_firing{rule}` | gauge | Número de objetos en alerta para cada regla de alerta de rendimiento habilitada |
| `gately_guard_banned` | gauge | Direcciones IP o redes actualmente en la lista de bloqueo |
| `gately_guard_firewall_active` | gauge | Si las reglas del cortafuegos de la prevención de intrusiones (nftables o iptables) están aplicadas (1) o no (0) |
| `gately_guard_dropped_packets_total{rule}` | counter | Paquetes descartados por las reglas del cortafuegos, por regla |
| `gately_guard_jail_matches_total{jail}` | counter | Líneas de registro coincidentes con cada regla de registros |
| `gately_guard_jail_bans_total{jail}` | counter | Bloqueos activados por cada regla de registros |
| `gately_knock_packets_total{result}` | counter | Paquetes de knock por resultado (`accepted`, `rejected`, `ignored`) |
| `gately_knock_grants` | gauge | Direcciones de origen que tienen ahora una autorización de knock |
| `gately_knock_firewall_active` | gauge | Si las reglas de bloqueo del cortafuegos del port knocking están aplicadas (1) o no (0) |
| `gately_knock_firewall_dropped_total` | counter | Conexiones nuevas a puertos protegidos descartadas por el cortafuegos |

## Ejemplos de alertas

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

## Panel de Grafana

En Grafana elige Import, sube [deploy/grafana/gately-dashboard.json](../../deploy/grafana/gately-dashboard.json) y selecciona tu origen de datos de Prometheus.
