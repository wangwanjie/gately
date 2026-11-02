# Performance

Performance puts the host, the Gately process and every site on one page, and sends an alert through the [Notification Center](/en/docs/modules/notify) when a metric stays over its threshold, followed by a recovery notice when the condition clears. Open it from the sidebar under System → Performance.

It covers cases such as the CPU maxing out at night, a site's connections surging or a backend slowing down without anyone noticing, and keeps its data in memory only. For history longer than 24 hours, aggregation across instances or complex queries, use [Prometheus and Grafana](/en/docs/reference/monitoring).

## Dashboard

Switch between 60 seconds, 1 hour and 24 hours at the top right of the page.

### Host and Gately process

| Chart | Content |
| --- | --- |
| Host CPU | Average usage across all cores of the machine |
| Host memory / disk | Used memory as a share of physical memory (reclaimable cache not counted), used share of the partition holding the config directory |
| System load | 1 minute load |
| Gately process CPU | Share of the whole machine used by the Gately process, measured like host CPU |
| Gately process memory | Resident memory (MB) |
| File descriptors / goroutines | Open file descriptors and Go goroutines of the process |

A sample is taken every 5 seconds: the 60 second range shows every sample; the 1 hour range one point per minute (the minute's average); the 24 hour range one point per 5 minutes. The dashed line in the CPU charts is the peak within each point, so short spikes are not averaged away. Hover a chart to read the values at a given moment.

Some platforms cannot provide every metric (for example there is no CPU usage on macOS); those charts show "No data".

### Sites

With the Web Services module on, all running sites (including default sites) are listed:

| Column | Meaning |
| --- | --- |
| In flight | Requests the site is handling right now, including long-lived connections such as WebSocket |
| Requests/s | Average requests per second over roughly the last 60 seconds |
| 5xx ratio | 5xx responses as a percentage of completed responses over roughly the last 60 seconds |
| Latency P95 | Estimated 95th percentile of time to first byte over roughly the last 60 seconds |
| Backends | Healthy backends / all backends, and the connections being forwarded |

Click a site to open its [site statistics](/en/docs/modules/web#site-stats) with 24 hour trends for requests, errors and latency.

### Time to first byte

Latency is "time to first byte": from request arrival until the response headers are sent, which reflects how fast the backend or the site itself responds. Not counted:

- the transfer time of the response body (large downloads do not raise latency);
- WebSocket and other upgraded requests;
- requests dropped by access control or closed by an administrator.

Each request falls into one of 50 fine-grained buckets (1 ms to 60 s, neighbours about 10% to 25% apart). Every bucket keeps both a count and the total time, percentiles use the average time of the bucket they fall in, so the error is never larger than the bucket width, and no data about individual requests is kept. The `gately_site_request_duration_seconds` histogram exported to Prometheus uses 13 common boundaries out of these.

## Alert rules

Click "Add alert rule". When creating one you can click a template to prefill a common rule (host CPU high, disk space low, site 5xx errors, site slow responses and more) and adjust it.

| Field | Description |
| --- | --- |
| Name | Required, up to 64 characters, shown in the notification title |
| Metric | See the table below |
| Threshold | Operator `>`, `>=`, `<` or `<=` plus a value; the unit depends on the metric |
| Duration | 1 to 1440 minutes. The alert is sent only after the condition has held this long; a single drop below the threshold restarts the timer |
| Sites | For site and backend node metrics; leave empty for all sites |
| Minimum requests | For the 5xx ratio and latency metrics, default 20. No evaluation while the last ~60 seconds have fewer requests, so one or two errors or slow requests on a quiet site do not trigger an alert |
| Alert level | Warning or error, sets the level of the notification event |
| Remark, enable | Optional |

| Metric | Unit | Evaluated per |
| --- | --- | --- |
| Host CPU usage | % | This host |
| Host memory usage | % | This host |
| Host disk usage (partition holding the config directory) | % | This host |
| Gately process CPU usage | % | Gately process |
| Gately process resident memory | MB | Gately process |
| Site requests in flight | count | Each site |
| Site requests per second | req/s | Each site |
| Site 5xx response ratio | % | Each site |
| Site time to first byte P95 | ms | Each site |
| Backend node connections | count | Each backend node |

Site and backend node metrics are evaluated and alerted on per object, so one "5xx ratio > 5%" rule can alert on two sites separately. "Site requests per second < 1" detects traffic that suddenly stopped.

### Evaluation and notifications

1. All enabled rules are evaluated every 5 seconds.
2. When the condition starts to hold the object becomes "Pending"; after it has held for the duration it becomes "Firing" and a `perf.alert` event is published.
3. When the condition of a firing alert clears, a `perf.recovered` event is published. A site's 5xx ratio and latency cannot be evaluated while the last ~60 seconds have too few requests; that also counts as cleared, and the recovery notice says so.
4. When a site is deleted or disabled its alerts are dropped without a recovery notice.

The top of the page lists pending and firing objects with their current value and how long they have lasted.

Changing a rule's name, level or remark keeps a running timer; changing the metric, threshold, duration, sites or minimum requests, or disabling the rule, clears the rule's timers and alerts. Timers and alert states live in memory only and start over after a restart.

Alerts are sent through the Notification Center: add a notification rule that subscribes to "Performance: alert firing" and "Performance: alert recovered" (the "Performance alerts" template does this), otherwise alerts are only written to the log on this page and the page shows a hint. Notification Center silence also applies to performance alerts; different objects have different event sources, so they do not suppress each other. The duration already debounces alerts, so silence is usually not needed.

## Resources and limits

- History is kept in memory only: the host and process have one point per minute for 24 hours; site latency keeps P50 and P95 per minute, about 23 KB for a site with traffic.
- Up to 200 alert rules, up to 200 sites per rule.
- With the Web Services module on, each request costs one extra clock read and a few counter updates.

## Monitoring metrics

While Performance is on, `/api/metrics` also exports the host and process metrics from the latest sample and the number of firing alerts per rule; see [Monitoring](/en/docs/reference/monitoring).

## API

| Endpoint | Description |
| --- | --- |
| `GET /api/perf/overview?range=60s\|1h\|24h` | Host and process history |
| `GET /api/perf/sites` | Each site's metrics over roughly the last 60 seconds |
| `GET /api/perf/alerts` | Pending and firing objects |
| `GET /api/perf/metrics` | Metrics available for alerts |
| `/api/perf/rules` | Create, read, update and delete alert rules, enable, order and group them |

Reading needs the `perf:read` scope and changing rules needs `perf:write`; see [API tokens](/en/docs/reference/api).
