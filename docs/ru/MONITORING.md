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
| `gately_site_blocked_total{listener,site,reason}` | counter | Заблокированные запросы по причинам (`ban`, `ip`, `country`, `rateLimit`, `waf`) |
| `gately_site_request_duration_seconds{listener,site,le}` | histogram | Гистограмма времени до первого байта сайта в секундах, 13 интервалов `le` от 0.005 до 60; WebSocket и другие запросы со сменой протокола, а также оборванные запросы не учитываются |
| `gately_upstream_up{listener,site,upstream[,route]}` | gauge | Работоспособен ли бэкенд; бэкенды по путям и по пользователям дополнительно имеют метку `route` (префикс пути или `@` со списком пользователей) |
| `gately_upstream_requests_total{listener,site,upstream[,route]}` | counter | Запросы, переданные бэкенду |
| `gately_upstream_response_bytes_total{listener,site,upstream[,route]}` | counter | Байты тела ответов бэкенда (без данных после перехода на WebSocket) |
| `gately_listener_requests_total{listener,action}` | counter | Запросы, которые слушатель обрабатывает сам: `https_redirect` — перенаправление открытого HTTP на HTTPS, `not_found` — 404, когда ни один сайт не совпал |
| `gately_host_cpu_usage_percent` / `gately_host_memory_used_percent` / `gately_host_disk_used_percent` / `gately_host_load1` | gauge | Загрузка ЦП, памяти и раздела с каталогом конфигурации хоста и нагрузка за 1 минуту по последнему замеру (экспортируются при включённом модуле «Производительность»; недоступные значения пропускаются) |
| `gately_process_cpu_usage_percent` / `gately_process_resident_memory_bytes` / `gately_process_open_fds` | gauge | ЦП процесса Gately (доля всей машины), резидентная память и число открытых файловых дескрипторов по последнему замеру (экспортируются при включённом модуле «Производительность») |
| `gately_perf_alerts_firing{rule}` | gauge | Число объектов, по которым сейчас сработало каждое включённое правило оповещений о производительности |

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
      - alert: GatelySiteSlow
        expr: histogram_quantile(0.95, sum by (le, listener, site) (rate(gately_site_request_duration_seconds_bucket[5m]))) > 2
        for: 5m
```

## Дашборд Grafana

В Grafana выберите Import, загрузите [deploy/grafana/gately-dashboard.json](../../deploy/grafana/gately-dashboard.json) и укажите свой источник данных Prometheus.
