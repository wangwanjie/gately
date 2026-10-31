# 모니터링 지표(Prometheus / Grafana)

Gately는 `/api/metrics`에서 Prometheus 텍스트 형식(0.0.4)으로 전역 지표를 내보냅니다. 프로세스, 포트 포워딩, STUN, DDNS, 인증서, 웹 서비스를 다룹니다.

## 수집 설정

1. 「API 토큰」 페이지에서 「모니터링 지표」의 읽기 권한(`metrics:read`)만 가진 토큰을 만듭니다. 이 토큰은 다른 엔드포인트에 접근할 수 없습니다.
2. Prometheus의 `scrape_configs`에 다음을 추가합니다.

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

## 지표 목록

| 지표 | 유형 | 설명 |
| --- | --- | --- |
| `gately_build_info{version,commit,goversion}` | gauge | 빌드 정보, 값은 항상 1 |
| `gately_uptime_seconds` | gauge | 프로세스 가동 시간 |
| `gately_goroutines` / `gately_memory_alloc_bytes` | gauge | 고루틴 수와 힙 메모리 |
| `gately_cert_days_remaining{cert}` | gauge | 인증서 만료까지 남은 일수 |
| `gately_cert_expiry_timestamp_seconds{cert}` | gauge | 인증서 만료 시각 |
| `gately_ddns_record_up{task,record,type}` | gauge | 마지막 동기화 성공 여부(1 = 성공) |
| `gately_ddns_last_update_timestamp_seconds{...}` | gauge | 레코드가 마지막으로 변경된 시각 |
| `gately_stun_up{rule}` | gauge | STUN 규칙이 유효한 매핑을 얻었는지 여부 |
| `gately_stun_connections_total{rule}` | counter | STUN 내장 포워딩이 처리한 연결 수 |
| `gately_portforward_active_connections{rule}` | gauge | 포워딩 규칙의 현재 연결 수 |
| `gately_portforward_connections_total{rule}` | counter | 포워딩 규칙이 받은 누적 연결 수 |
| `gately_portforward_bytes_total{rule,direction}` | counter | 중계한 누적 바이트(`in` = 클라이언트에서 대상 방향) |
| `gately_protoroute_active_connections{rule}` | gauge | 프로토콜 라우팅 규칙의 현재 연결 수 |
| `gately_protoroute_connections_total{rule}` | counter | 프로토콜 라우팅 규칙이 받은 누적 연결 수 |
| `gately_protoroute_bytes_total{rule,direction}` | counter | 프로토콜 라우팅 규칙이 중계한 누적 바이트(`in` = 클라이언트에서 대상 방향) |
| `gately_protoroute_route_connections_total{rule,route}` | counter | 라우트별 누적 연결 수. `route`는 라우트 이름(이름이 없으면 번호), 기본 대상은 `default` |
| `gately_site_requests_total{listener,site}` | counter | 사이트별 요청 수 |
| `gately_site_responses_total{listener,site,class}` | counter | 상태 분류(2xx/3xx/4xx/5xx)별 응답 수 |
| `gately_site_response_bytes_total` / `gately_site_errors_total` / `gately_site_active_requests` | counter / counter / gauge | 응답 바이트, 백엔드 오류, 처리 중인 요청 |
| `gately_upstream_up{listener,site,upstream}` | gauge | 백엔드 정상 여부 |

## 알림 예시

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

## Grafana 대시보드

Grafana에서 Import를 선택하고 [deploy/grafana/gately-dashboard.json](../../deploy/grafana/gately-dashboard.json)을 업로드한 뒤 사용하는 Prometheus 데이터 소스를 선택합니다.
