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
| `gately_site_blocked_total{listener,site,reason}` | counter | Requisições bloqueadas por motivo (`ban`, `ip`, `country`, `rateLimit`, `waf`) |
| `gately_site_request_duration_seconds{listener,site,le}` | histogram | Histograma do tempo até o primeiro byte do site em segundos, 13 faixas `le` de 0.005 a 60; exclui WebSocket e outras requisições com troca de protocolo e requisições interrompidas |
| `gately_upstream_up{listener,site,upstream[,route]}` | gauge | Se o upstream está saudável; backends por caminho e por usuário também têm a label `route` (prefixo do caminho, ou `@` mais a lista de usuários) |
| `gately_upstream_requests_total{listener,site,upstream[,route]}` | counter | Requisições encaminhadas ao upstream |
| `gately_upstream_response_bytes_total{listener,site,upstream[,route]}` | counter | Bytes do corpo de resposta devolvidos pelo upstream (sem os dados após upgrade para WebSocket) |
| `gately_listener_requests_total{listener,action}` | counter | Requisições que o listener atende sozinho: `https_redirect` para HTTP sem criptografia redirecionado para HTTPS, `not_found` para o 404 quando nenhum site corresponde |
| `gately_host_cpu_usage_percent` / `gately_host_memory_used_percent` / `gately_host_disk_used_percent` / `gately_host_load1` | gauge | Uso de CPU, memória e partição do diretório de configuração do host e carga de 1 minuto da última amostra (exportados com o módulo Desempenho ligado; valores que não podem ser lidos são omitidos) |
| `gately_process_cpu_usage_percent` / `gately_process_resident_memory_bytes` / `gately_process_open_fds` | gauge | CPU do processo do Gately (parte da máquina inteira), memória residente e descritores de arquivo abertos da última amostra (exportados com o módulo Desempenho ligado) |
| `gately_perf_alerts_firing{rule}` | gauge | Número de objetos em alerta para cada regra de alerta de desempenho habilitada |
| `gately_guard_banned` | gauge | Endereços IP ou redes atualmente na lista de bloqueio |
| `gately_guard_firewall_active` | gauge | Se as regras de firewall da prevenção de intrusão (nftables ou iptables) estão aplicadas (1) ou não (0) |
| `gately_guard_dropped_packets_total{rule}` | counter | Pacotes descartados pelas regras do firewall, por regra |
| `gately_guard_jail_matches_total{jail}` | counter | Linhas de log que casaram com cada regra de logs |
| `gately_guard_jail_bans_total{jail}` | counter | Bloqueios disparados por cada regra de logs |
| `gately_knock_packets_total{result}` | counter | Pacotes de knock por resultado (`accepted`, `rejected`, `ignored`) |
| `gately_knock_grants` | gauge | Endereços de origem que têm atualmente uma autorização de knock |
| `gately_knock_firewall_active` | gauge | Se as regras de bloqueio do firewall do port knocking estão aplicadas (1) ou não (0) |
| `gately_knock_firewall_dropped_total` | counter | Novas conexões a portas protegidas descartadas pelo firewall |

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
      - alert: GatelySiteSlow
        expr: histogram_quantile(0.95, sum by (le, listener, site) (rate(gately_site_request_duration_seconds_bucket[5m]))) > 2
        for: 5m
```

## Dashboard do Grafana

No Grafana, escolha Import, envie o [deploy/grafana/gately-dashboard.json](../../deploy/grafana/gately-dashboard.json) e selecione a sua fonte de dados do Prometheus.
