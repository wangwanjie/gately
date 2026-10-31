# Метрики (Prometheus / Grafana)

Gately экспортирует глобальные метрики в текстовом формате Prometheus (0.0.4) по адресу `/api/metrics`: процесс, проброс портов, STUN, DDNS, сертификаты и веб-службы.

## Настройка сбора

1. На странице «API-токены» создайте токен только с правом чтения «Метрики (Prometheus)» (`metrics:read`). Этот токен не даёт доступа ни к каким другим эндпоинтам.
2. Добавьте в `scrape_configs` в Prometheus:

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

## Метрики

| Метрика | Тип | Описание |
| --- | --- | --- |
| `gately_build_info{version,commit,goversion}` | gauge | Сведения о сборке, всегда 1 |
| `gately_uptime_seconds` | gauge | Время работы процесса |
| `gately_goroutines` / `gately_memory_alloc_bytes` | gauge | Число горутин и память кучи |
| `gately_cert_days_remaining{cert}` | gauge | Дней до истечения сертификата |
| `gately_cert_expiry_timestamp_seconds{cert}` | gauge | Время истечения сертификата |
| `gately_ddns_record_up{task,record,type}` | gauge | Успешна ли последняя синхронизация (1 = успешно) |
| `gately_ddns_last_update_timestamp_seconds{...}` | gauge | Когда запись последний раз изменялась |
| `gately_stun_up{rule}` | gauge | Есть ли у правила STUN рабочее сопоставление |
| `gately_stun_connections_total{rule}` | counter | Соединения, обработанные встроенной переадресацией STUN |
| `gately_portforward_active_connections{rule}` | gauge | Открытые соединения правила проброса |
| `gately_portforward_connections_total{rule}` | counter | Принятые соединения правила проброса |
| `gately_portforward_bytes_total{rule,direction}` | counter | Переданные байты (`in` = от клиента к цели) |
| `gately_protoroute_active_connections{rule}` | gauge | Открытые соединения правила маршрутизации по протоколу |
| `gately_protoroute_connections_total{rule}` | counter | Принятые соединения правила маршрутизации по протоколу |
| `gately_protoroute_bytes_total{rule,direction}` | counter | Байты, переданные правилом маршрутизации по протоколу (`in` = от клиента к цели) |
| `gately_protoroute_route_connections_total{rule,route}` | counter | Соединения, отправленные на каждый маршрут; `route` — имя маршрута (номер, если имени нет), `default` — цель по умолчанию |
| `gately_site_requests_total{listener,site}` | counter | Запросы по сайтам |
| `gately_site_responses_total{listener,site,class}` | counter | Ответы по классам статуса (2xx/3xx/4xx/5xx) |
| `gately_site_response_bytes_total` / `gately_site_errors_total` / `gately_site_active_requests` | counter / counter / gauge | Байты ответов, ошибки бэкендов, выполняющиеся запросы |
| `gately_upstream_up{listener,site,upstream}` | gauge | Работоспособен ли бэкенд |

## Примеры оповещений

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

## Дашборд Grafana

В Grafana выберите Import, загрузите [deploy/grafana/gately-dashboard.json](../../deploy/grafana/gately-dashboard.json) и укажите свой источник данных Prometheus.
