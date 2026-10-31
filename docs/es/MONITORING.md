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
| `gately_upstream_up{listener,site,upstream}` | gauge | Si el upstream está sano |

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
```

## Panel de Grafana

En Grafana elige Import, sube [deploy/grafana/gately-dashboard.json](../../deploy/grafana/gately-dashboard.json) y selecciona tu origen de datos de Prometheus.
