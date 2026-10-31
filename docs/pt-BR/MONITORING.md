# Métricas (Prometheus / Grafana)

O Gately exporta métricas globais no formato de texto do Prometheus (0.0.4) em `/api/metrics`, cobrindo o processo, o redirecionamento de portas, STUN, DDNS, certificados e serviços web.

## Configuração da coleta

1. Na página "Tokens de API", crie um token apenas com a permissão de leitura de "Metrics" (`metrics:read`). Esse token não acessa nenhum outro endpoint.
2. Adicione ao `scrape_configs` do Prometheus:

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

| Métrica | Tipo | Descrição |
| --- | --- | --- |
| `gately_build_info{version,commit,goversion}` | gauge | Informações de build, sempre 1 |
| `gately_uptime_seconds` | gauge | Tempo de atividade do processo |
| `gately_goroutines` / `gately_memory_alloc_bytes` | gauge | Goroutines e memória heap |
| `gately_cert_days_remaining{cert}` | gauge | Dias até o certificado expirar |
| `gately_cert_expiry_timestamp_seconds{cert}` | gauge | Momento de expiração do certificado |
| `gately_ddns_record_up{task,record,type}` | gauge | Se a última sincronização teve sucesso (1 = sucesso) |
| `gately_ddns_last_update_timestamp_seconds{...}` | gauge | Quando o registro foi alterado pela última vez |
| `gately_stun_up{rule}` | gauge | Se a regra STUN tem um mapeamento funcionando |
| `gately_stun_connections_total{rule}` | counter | Conexões tratadas pelo encaminhador STUN embutido |
| `gately_portforward_active_connections{rule}` | gauge | Conexões abertas de uma regra de redirecionamento |
| `gately_portforward_connections_total{rule}` | counter | Conexões aceitas de uma regra de redirecionamento |
| `gately_portforward_bytes_total{rule,direction}` | counter | Bytes repassados (`in` = do cliente para o destino) |
| `gately_protoroute_active_connections{rule}` | gauge | Conexões abertas de uma regra de roteamento por protocolo |
| `gately_protoroute_connections_total{rule}` | counter | Conexões aceitas de uma regra de roteamento por protocolo |
| `gately_protoroute_bytes_total{rule,direction}` | counter | Bytes repassados por uma regra de roteamento por protocolo (`in` = do cliente para o destino) |
| `gately_protoroute_route_connections_total{rule,route}` | counter | Conexões enviadas a cada rota; `route` é o nome da rota (seu número quando não tem nome), `default` para o destino padrão |
| `gately_site_requests_total{listener,site}` | counter | Requisições por site |
| `gately_site_responses_total{listener,site,class}` | counter | Respostas por classe de status (2xx/3xx/4xx/5xx) |
| `gately_site_response_bytes_total` / `gately_site_errors_total` / `gately_site_active_requests` | counter / counter / gauge | Bytes de resposta, erros de backend, requisições em andamento |
| `gately_upstream_up{listener,site,upstream}` | gauge | Se o upstream está saudável |

## Exemplos de alertas

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

## Dashboard do Grafana

No Grafana, escolha Import, envie o [deploy/grafana/gately-dashboard.json](../../deploy/grafana/gately-dashboard.json) e selecione a sua fonte de dados do Prometheus.
