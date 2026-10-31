# 監視メトリクス（Prometheus / Grafana）

Gately は `/api/metrics` で Prometheus テキスト形式（0.0.4）の全体メトリクスを出力します。プロセス、ポート転送、STUN、DDNS、証明書、Web サービスを対象とします。

## スクレイプの設定

1. 「API トークン」ページで、「監視メトリクス」の読み取り権限（`metrics:read`）だけを持つトークンを作成します。このトークンでは他のエンドポイントにはアクセスできません。
2. Prometheus の `scrape_configs` に次を追加します。

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

## メトリクス一覧

| メトリクス | 型 | 説明 |
| --- | --- | --- |
| `gately_build_info{version,commit,goversion}` | gauge | ビルド情報。値は常に 1 |
| `gately_uptime_seconds` | gauge | プロセスの稼働時間 |
| `gately_goroutines` / `gately_memory_alloc_bytes` | gauge | goroutine 数とヒープメモリ |
| `gately_cert_days_remaining{cert}` | gauge | 証明書の有効期限までの日数 |
| `gately_cert_expiry_timestamp_seconds{cert}` | gauge | 証明書の有効期限 |
| `gately_ddns_record_up{task,record,type}` | gauge | 直近の同期が成功したか（1 = 成功） |
| `gately_ddns_last_update_timestamp_seconds{...}` | gauge | レコードが最後に変更された時刻 |
| `gately_stun_up{rule}` | gauge | STUN ルールに有効なマッピングがあるか |
| `gately_stun_connections_total{rule}` | counter | STUN の内蔵フォワーディングが処理した接続数 |
| `gately_portforward_active_connections{rule}` | gauge | 転送ルールの現在の接続数 |
| `gately_portforward_connections_total{rule}` | counter | 転送ルールが受け付けた接続数 |
| `gately_portforward_bytes_total{rule,direction}` | counter | 中継したバイト数（`in` はクライアントから転送先方向） |
| `gately_protoroute_active_connections{rule}` | gauge | プロトコル振り分けルールの現在の接続数 |
| `gately_protoroute_connections_total{rule}` | counter | プロトコル振り分けルールが受け付けた累計接続数 |
| `gately_protoroute_bytes_total{rule,direction}` | counter | プロトコル振り分けルールが中継したバイト数（`in` はクライアントから転送先方向） |
| `gately_protoroute_route_connections_total{rule,route}` | counter | 各ルートに送られた累計接続数。`route` はルート名（名前がない場合は番号）、デフォルトターゲットは `default` |
| `gately_site_requests_total{listener,site}` | counter | サイトごとのリクエスト数 |
| `gately_site_responses_total{listener,site,class}` | counter | ステータスの種類（2xx/3xx/4xx/5xx）ごとのレスポンス数 |
| `gately_site_response_bytes_total` / `gately_site_errors_total` / `gately_site_active_requests` | counter / counter / gauge | レスポンスのバイト数、バックエンドのエラー数、処理中のリクエスト数 |
| `gately_upstream_up{listener,site,upstream}` | gauge | バックエンドが正常か |

## アラートの例

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

## Grafana ダッシュボード

Grafana で Import を選び、[deploy/grafana/gately-dashboard.json](../../deploy/grafana/gately-dashboard.json) をアップロードして、Prometheus のデータソースを選択します。
