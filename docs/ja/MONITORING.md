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
| `gately_site_blocked_total{listener,site,reason}` | counter | 理由（`ban`、`ip`、`country`、`rateLimit`、`waf`）別のブロック数 |
| `gately_site_request_duration_seconds{listener,site,le}` | histogram | サイトの最初のバイトまでの時間のヒストグラム（秒）。`le` は 0.005〜60 の 13 段階。WebSocket などのプロトコルアップグレードと切断されたリクエストは含まない |
| `gately_upstream_up{listener,site,upstream[,route]}` | gauge | バックエンドが正常か。パス別とユーザー別のバックエンドには `route` ラベル（パスのプレフィックス、または `@` とユーザー一覧）も付く |
| `gately_upstream_requests_total{listener,site,upstream[,route]}` | counter | バックエンドへ転送したリクエスト数 |
| `gately_upstream_response_bytes_total{listener,site,upstream[,route]}` | counter | バックエンドが返したレスポンス本文のバイト数（WebSocket アップグレード後のデータは含まない） |
| `gately_listener_requests_total{listener,action}` | counter | リスナーが直接処理したリクエスト。`https_redirect` は平文 HTTP の HTTPS へのリダイレクト、`not_found` は一致するサイトがない場合の 404 |
| `gately_host_cpu_usage_percent` / `gately_host_memory_used_percent` / `gately_host_disk_used_percent` / `gately_host_load1` | gauge | 直近のサンプルのホスト CPU、メモリ、設定ディレクトリのパーティションの使用率と 1 分間の負荷（パフォーマンス監視モジュールがオンのときに出力、取得できない項目は出力しない） |
| `gately_process_cpu_usage_percent` / `gately_process_resident_memory_bytes` / `gately_process_open_fds` | gauge | 直近のサンプルの Gately プロセスの CPU（マシン全体に占める割合）、常駐メモリ、オープン中のファイルディスクリプタ数（パフォーマンス監視モジュールがオンのときに出力） |
| `gately_perf_alerts_firing{rule}` | gauge | 有効な各パフォーマンスアラートルールで現在発報中の対象数 |

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
      - alert: GatelySiteSlow
        expr: histogram_quantile(0.95, sum by (le, listener, site) (rate(gately_site_request_duration_seconds_bucket[5m]))) > 2
        for: 5m
```

## Grafana ダッシュボード

Grafana で Import を選び、[deploy/grafana/gately-dashboard.json](../../deploy/grafana/gately-dashboard.json) をアップロードして、Prometheus のデータソースを選択します。
