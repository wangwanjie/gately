# Benchmarks

This page compares Gately with the common tools it can replace on the same machine under the same CPU limits: reverse proxy, web authentication, WAF, intrusion prevention, port knocking, dynamic DNS, and behavior on an OpenWrt soft router. All numbers come from measurements taken in November 2026. The test scripts, configurations and raw results are in the repository's [tools/benchmark](https://github.com/wangwanjie/gately/tree/main/tools/benchmark) directory, so you can reproduce them.

<script setup>
const H = 'Higher is better'
const L = 'Lower is better'
</script>

## Summary

- **Reverse proxy**: nginx and HAProxy, written in C, have the highest throughput, about 3.5 times Gately for HTTP and about 2.6 times for HTTPS. Among gateways written in Go, Gately is in the top tier: its HTTPS throughput is 43% higher than Caddy's, 2.3 times Lucky's, and 9% below Traefik's; its rate of new TLS connections is the highest of the 7 programs.
- **No slowdown with many sites**: with 1000 sites, Gately's throughput drops by only 6%, ahead of HAProxy, Traefik and Caddy and behind only nginx.
- **Web authentication**: for requests from signed-in users, Gately is 1.9 times nginx + Authelia and 3.7 times Caddy + Authelia, using one third of their memory.
- **WAF**: with the same OWASP CRS 4.25, Gately handles ordinary requests about 70% faster than nginx + ModSecurity, and the blocking results are identical; when inspecting POST bodies, ModSecurity is faster and uses far less memory.
- **Intrusion prevention**: with 10,000 attacker IPs, Gately finishes banning in 12 seconds and fail2ban takes 51 seconds; at the same log volume, CPU use is less than half of fail2ban's. When reading log files, the latency of a single ban is about 0.2 seconds longer than fail2ban's.
- **Port knocking**: knock latency is under 10 ms for both Gately and fwknop, and after the port is opened traffic does not pass through the knock program; fn-knock on ordinary Linux forwards traffic through its gateway, and its short-connection throughput is only 3% of a direct connection.
- **Resource use**: for a single function alone, Gately uses more memory than small tools such as fail2ban, fwknop and ddns-go; when one Gately handles reverse proxy, WAF, authentication, DDNS, intrusion prevention and port knocking together, it uses about 91 MiB, against about 204 MiB in total for separate tools. See [All in one](#all-in-one).

## Test environment

| Item | Description |
| --- | --- |
| Host | Intel Core i5-13500T, 32 GB RAM, fnOS (KVM) |
| Load-test VM | Debian 13 (kernel 6.12), 8 vCPUs pinned to 8 threads of 4 performance cores on the host, 8 GB RAM, Docker 29 |
| CPU allocation | Program under test on vCPU 0–1, origin nginx on vCPU 2–3, load tools on vCPU 4–7, with no contention between them |
| Soft router | ImmortalWrt 25.12.1 VM, 2 vCPUs (two threads of the same performance core), 1 GB RAM |
| Load tools | wrk 4.1, oha 1.17, h2load 1.64 (nghttp2) |
| Certificate | All programs share the same ECDSA P-256 certificate, TLS 1.3 |
| Origin | nginx returning 1 KB of text; 100 MB for the large-file test |

Versions tested: Gately v0.6.8.1, nginx 1.29.8, HAProxy 3.2.26, Caddy 2.11.9, Traefik 3.7.19, Lucky 3.8.1, Nginx Proxy Manager 2.17.0 (OpenResty 1.31.8), Authelia 4.39.28, ModSecurity 3.0.16 (OWASP CRS 4.25.0), fail2ban 1.1.0, fwknop 2.6.11, fn-knock 2.5.11 (fnOS knock), ddns-go 6.17.9. Apart from fail2ban and fwknop, which use Debian packages, everything runs from official Docker images with host networking.

**Methodology**

- Throughput: each item runs for 20 seconds, 3 rounds, and the median is taken. HTTP and HTTPS use wrk with 128 persistent connections; new TLS connections use oha with 64 concurrency and a fresh connection per request; HTTP/2 uses h2load with 64 connections and 10 concurrent streams each.
- Memory: the sum of PSS over all processes in the container under test (memory shared between processes is split proportionally, so it is not counted twice). "Idle" is the value 10 seconds after startup and configuration finish; "peak" is the maximum during the load test.
- CPU: CPU time of the container's cgroup, where 100% equals one vCPU.
- Startup: restart the program once the configuration is ready, time until the first request returns correctly, and take the median of 3 runs.
- Access logs are turned off in every program. nginx has the upstream keep-alive pool enabled and a raised per-connection request limit (a common tuning); the other programs use default settings.

## Reverse proxy

The domain `bench.test` is proxied to the origin, with HTTP on port 8080 and HTTPS on port 8443 (Nginx Proxy Manager can only use 80/443).

<BenchBars title="HTTPS requests/s (persistent connections)" unit="QPS" :better-label="H" :items="[['nginx',85434],['HAProxy',83953],['Traefik',35812],['Nginx Proxy Manager',32816],['Gately',32764],['Caddy',22871],['Lucky',14376]]" />

<BenchBars title="New TLS connections/s (one full handshake per request)" unit="conn/s" :better-label="H" :items="[['Gately',4636],['Traefik',4482],['Nginx Proxy Manager',4015],['Caddy',3987],['nginx',3796],['Lucky',3621],['HAProxy',3293]]" />

<BenchBars title="Idle memory" unit="MiB" better="lower" :digits="1" :better-label="L" :items="[['nginx',15.7],['HAProxy',36.4],['Gately',47.6],['Caddy',48.8],['Traefik',89.0],['Lucky',124.6],['Nginx Proxy Manager',155.7]]" note="Gately's 47.6 MiB includes all function modules (DDNS, certificates, WAF, intrusion prevention and more run in the same process); Lucky has 2 processes and Nginx Proxy Manager has 22 (nginx, the Node.js backend and others)." />

| Program | HTTP | HTTPS | HTTP/2 | New TLS/s | 100 MB file | CPU at 5000 QPS | Idle memory | Load peak | Per idle connection | Restart time | Image |
| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |
| **Gately** | 35,364 | 32,764 | 21,591 | **4,636** | 4,157 MB/s | 67% | 47.6 MiB | 107 MiB | 21.2 KiB | 0.40 s | 229 MB |
| nginx | **123,343** | **85,434** | 78,110 | 3,796 | 2,161 MB/s | 55% | **15.7 MiB** | **25 MiB** | **0.6 KiB** | **0.32 s** | 93 MB |
| HAProxy | 117,923 | 83,953 | **90,633** | 3,293 | 3,912 MB/s | **52%** | 36.4 MiB | 56 MiB | 1.0 KiB | 0.33 s | 58 MB |
| Caddy | 24,459 | 22,871 | 11,746 | 3,987 | 4,116 MB/s | 68% | 48.8 MiB | 132 MiB | 27.8 KiB | 0.45 s | 92 MB |
| Traefik | 39,112 | 35,812 | 19,606 | 4,482 | **4,311 MB/s** | 66% | 89.0 MiB | 253 MiB | 24.4 KiB | 0.53 s | 250 MB |
| Lucky | 15,150 | 14,376 | 10,261 | 3,621 | 3,963 MB/s | 80% | 124.6 MiB | 193 MiB | 26.2 KiB | 3.47 s | 58 MB |
| Nginx Proxy Manager | 45,257 | 32,816 | 34,710 | 4,015 | 2,150 MB/s | 58% | 155.7 MiB | 215 MiB | preallocated | 1.50 s | 1,959 MB |

- HTTP, HTTPS and HTTP/2 are in requests per second. "CPU at 5000 QPS" is the CPU used by the program under a fixed load of 5000 requests per second; lower is more power-efficient.
- By default nginx closes a connection after 1000 requests, while h2load does not reconnect, so in the HTTP/2 column the limit was raised for nginx and Nginx Proxy Manager (Nginx Proxy Manager was raised through a custom configuration only in the HTTP/2 test; the other tests kept the defaults).
- In the large-file column, nginx and Nginx Proxy Manager are lower because nginx by default writes responses that exceed the memory buffer to a temporary file on disk first.
- "Per idle connection" is the memory added while holding 10,000 idle persistent connections, divided by the number of connections. Go programs keep separate read and write buffers and a goroutine for each connection, about 20–28 KiB; nginx and HAProxy use under 1 KiB. Nginx Proxy Manager preallocates a connection table according to its configuration, so no increase could be measured.
- Gately's image is larger because it includes the Docker CLI, nftables and iptables tools; the program itself is a single 40 MB file, and the download archive is 18 MB.

## 1000 sites

Each program is configured with 1000 reverse proxy sites with different domains, and each request in the load test picks a random domain.

<BenchBars title="HTTP requests/s with 1000 sites" unit="QPS" :better-label="H" :items="[['nginx',122958],['Gately',33211],['Traefik',24229],['HAProxy',24057],['Caddy',9819]]" />

| Program | Requests/s | vs. 1 site | p99 latency | Restart until the last site is available | Idle memory | Load peak |
| --- | --- | --- | --- | --- | --- | --- |
| **Gately** | 33,211 | -6% | 29.8 ms | **0.43 s** | 94 MiB | 463 MiB |
| nginx | **122,958** | **0%** | **1.2 ms** | 0.54 s | 57 MiB | **58 MiB** |
| HAProxy | 24,057 | -80% | 30.0 ms | **0.37 s** | **51 MiB** | **58 MiB** |
| Caddy | 9,819 | -60% | 52.7 ms | 0.80 s | 80 MiB | 361 MiB |
| Traefik | 24,229 | -38% | 18.8 ms | 0.74 s | 115 MiB | 157 MiB |

- HAProxy matches rules one by one with `use_backend ... if { hdr(host) -i domain }`, so it slows down as sites grow; a map file avoids this, but the test keeps the common "one rule per site" style.
- Gately's peak memory comes mainly from per-site access statistics: each site with traffic keeps 24 hours of per-minute statistics, connection history and latency distribution, so with all 1000 sites receiving traffic it holds a little over 200 MiB resident.

## Web authentication

The site requires sign-in, and the test covers requests from signed-in users and the sign-in itself. Gately uses the site's built-in web authentication and basic auth; the comparison group is nginx or Caddy in front of Authelia (`auth_request` / `forward_auth`), plus nginx's own `auth_basic`. Everything goes over HTTPS.

<BenchBars title="Requests/s from signed-in users (with a session cookie)" unit="QPS" :better-label="H" :items="[['Gately',26726],['nginx + Authelia',14291],['Lucky',10786],['Caddy + Authelia',7141]]" />

| Setup | Signed-in requests/s | No authentication | Basic auth (bcrypt) | Successful sign-ins/s | Memory peak during sign-in | Idle memory |
| --- | --- | --- | --- | --- | --- | --- |
| **Gately** | **26,726** | 32,520 | **28,598** | **38.3** | **70 MiB** | **45 MiB** |
| nginx + Authelia | 14,291 | 83,474 | 40 | 13.4 | 1,082 MiB | 120 MiB |
| Caddy + Authelia | 7,141 | 22,703 | 21,213 | 13.4 | 1,271 MiB | 149 MiB |
| Lucky | 10,786 | 13,841 | 10,107 | see notes | 209 MiB | 127 MiB |

- Password hashing: basic auth in Gately and in nginx/Caddy uses bcrypt (cost 10), Authelia uses argon2id (t=3, 64 MiB, p=4), and Lucky stores site user passwords in plain text. Each Authelia sign-in takes 64 MiB of memory, and with 8 concurrent sign-ins memory exceeds 1 GB.
- nginx's `auth_basic` recomputes bcrypt on every request, so it reaches only 40 requests/s; Gately and Caddy cache the verification result. With apr1 (MD5), nginx reaches 13,082 requests/s.
- Wrong passwords: Gately rejects a source directly after 10 failures from it (it handles 16,000 wrong requests per second, and the CPU no longer computes hashes); Authelia adds a fixed delay of about 1 second to each failure. During the lockout, both reject the correct password from that source, and signed-in sessions are unaffected.
- With 8 concurrent sign-ins, Lucky returned `create webauth session error` every time, so its sign-in throughput could not be measured.

## WAF

All three use OWASP CRS 4.25.0, paranoia level 1, in blocking mode. Gately and Caddy use the same Coraza engine, and nginx uses ModSecurity v3.

<BenchBars title="GET requests/s with the WAF on" unit="QPS" :better-label="H" :items="[['Gately',2498],['Caddy + Coraza',2265],['nginx + ModSecurity',1491]]" />

| Program | GET | GET with 8 parameters | 4 KB form POST | Attack traffic | Load peak memory | Idle memory (on / off) |
| --- | --- | --- | --- | --- | --- | --- |
| **Gately** | **2,498** | **1,067** | 72 | **1,218** | 754 MiB | 88 / 48 MiB |
| nginx + ModSecurity | 1,491 | 591 | **102** | 1,014 | **73 MiB** | 62 / 24 MiB |
| Caddy + Coraza | 2,265 | 943 | 46 | 1,004 | 792 MiB | 96 / 55 MiB |

| Detection results | Gately | nginx + ModSecurity | Caddy + Coraza |
| --- | --- | --- | --- |
| Attack samples blocked (557) | 77.2% | 77.2% | 77.2% |
| False positives on normal samples (389) | 20.1% | 20.1% | 20.1% |
| CRS regression cases matched (2049) | 95.9% | 95.2% | 95.7% |

- The CRS rules are heavy: with the WAF on, throughput of all three falls to only 1%–10% of the value with it off, so for sites that need a WAF it is advisable to enable it only on dynamic endpoints.
- When inspecting POST bodies, the Coraza engine (in both Gately and Caddy) peaks above 700 MiB of memory, while ModSecurity stays at 73 MiB; take care when enabling body inspection on devices with little memory.
- The samples come from gotestwaf (MIT), the regression tests bundled with CRS (Apache-2.0) and hand-written normal requests; the false positives are mainly technical articles containing SQL/shell keywords and PUT/DELETE endpoints (CRS does not allow these methods by default).

## Intrusion prevention

Scenario: a Web Service is being scanned, and an IP is banned after 5 404 responses within 60 seconds (nftables). fail2ban reads the nginx access log with pyinotify; Gately reads the same log with an equivalent log-file rule, and one more group tests the path probing built into Gately Web Services.

<BenchBars title="Time to ban all 10,000 attacker IPs" unit="s" better="lower" :digits="1" :better-label="L" :items="[['Gately',12.3],['fail2ban',51.2]]" />

| Metric | Gately (reading log file) | Gately (built-in Web Service probing) | fail2ban |
| --- | --- | --- | --- |
| Ban latency for a single attacker p50 / p99 | 478 / 996 ms | **10 / 13 ms** | 280 / 864 ms |
| CPU at 1000 log lines/s | **2.9%** | — | 20.4% |
| CPU at 20,000 log lines/s | **35.7%** | — | 83.2% |
| 10,000 attacker IPs: all banned | **12.3 s** | — | 51.2 s |
| 10,000 attacker IPs: CPU consumed | **10.1 CPU s** | — | 60.3 CPU s |
| Idle memory | 44.1 MiB | 40.7 MiB | **17.6 MiB** |
| Missed / wrongly banned | 0 / 0 | 0 / 0 | 0 / 0 |

- Gately reads the log file once per second, so the latency of a single ban is between 0 and 1 second, on average about 0.2 seconds slower than fail2ban's inotify; the built-in Web Service probing counts directly while handling the request and has no such delay.
- fail2ban starts an `nft` process for every ban, so bans queue up when there are many attackers: with 10,000 IPs, the last one was banned 41 seconds after the injection ended.
- fail2ban itself is 2 MB, but depends on a Python runtime of about 24 MB.

## Port knocking

Three approaches are compared: Gately's port knocking (fwknop-compatible SPA single packet authorization, blocking with nftables), fwknop 2.6.11 (also SPA, blocking with iptables), and fn-knock 2.5.11 from fnOS (allows a source IP after a web sign-in, with its gateway forwarding the traffic). Gately also offers an entry that allows access after a web sign-in, tested as a separate group. The protected service is an nginx on a TCP port, and the client uses a public test address, so the "LAN needs no knock" exemption does not apply.

<BenchBars title="Short-connection requests/s through the protected port after opening" unit="QPS" :better-label="H" :items="[['fwknop',65046],['Gately',64763],['Gately web grant',63556],['fn-knock',1886]]" note="Connecting directly to the origin without any knock program gives 67,034 requests/s." />

| Metric | Gately (SPA) | Gately (web grant) | fwknop | fn-knock |
| --- | --- | --- | --- | --- |
| Knock to port reachable p50 / p99 | 8.4 / 13.4 ms | 58 / 73 ms | 9.0 / 18.1 ms | 84 / 143 ms |
| Protected port when unauthorized | Dropped (timeout) | Dropped (timeout) | Dropped (timeout) | Reset after handshake (detectable) |
| Persistent / short-connection requests/s after opening | 143,867 / 64,764 | 143,839 / 63,556 | 148,371 / 65,046 | 140,137 / 1,886 |
| CPU spent forwarding after opening (short connections) | 2% | 1% | 0% | 134% |
| Legitimate knocks under 50,000 invalid packets/s from a single source | **10/10** (CPU 26%) | — | 3/8 (CPU 102%) | — |
| Legitimate knocks under 10,000 invalid packets/s from spoofed sources | 1/7 | — | **10/10** (CPU 54%) | — |
| Idle memory | 48.7 MiB | 48.7 MiB | **2.2 MiB** | 63.2 MiB (3 processes) |

- With Gately and fwknop, the firewall lets traffic through directly after opening and the traffic does not pass through the knock program, so throughput equals a direct connection; fn-knock on ordinary Linux forwards through its gateway and queries the control plane for authorization on every new connection, so its short-connection throughput is only 3% of a direct connection. fn-knock also has a direct firewall mode in its fnOS fpk version; the Docker version was tested here.
- Signing in to fn-knock requires solving an ALTCHA proof of work first (p50 of 28 ms in this test), and 50 ms without the solving.
- fwknopd processes each packet on a single thread, and at 50,000 packets/s from one source it saturates a core and legitimate knocks begin to fail. Gately rate-limits each source, so a flood from a single source uses almost no CPU.
- Known issue: when an attacker spoofs many different source addresses, Gately's per-source rate-limit table fills up, the shared overflow quota for new sources is exhausted by the flood, and most legitimate knocks fail (Gately's CPU is only about 19% at that point). fwknop does not rate-limit and is unaffected until the CPU is saturated.

## Dynamic DNS

Each program is configured with 20 DDNS tasks and a 30-second check interval. The public IP is queried from the local origin, and updates are sent to the local origin through a "custom callback", without touching a real DNS provider. After startup and the first sync, a 5-minute sample is taken.

<BenchBars title="Average memory with 20 DDNS tasks" unit="MiB" better="lower" :digits="1" :better-label="L" :items="[['ddns-go',16.9],['Gately',51.6],['Lucky',131.3]]" />

| Program | Average memory | Peak memory | Average CPU | Image |
| --- | --- | --- | --- | --- |
| ddns-go 6.17.9 | **16.9 MiB** | **18.2 MiB** | **0.04%** | 43 MB |
| Gately | 51.6 MiB | 55.4 MiB | 0.16% | 229 MB |
| Lucky 3.8.1 | 131.3 MiB | 134.9 MiB | 0.48% | 58 MB |

- All three contact the provider only when the IP changes or the forced sync interval arrives, so the standing overhead is small for all of them. When only DDNS is needed, ddns-go uses the least memory; Gately's figure includes all of its modules, and the same process can also run certificates, reverse proxy and other functions.

## OpenWrt soft router

Each program runs on an ImmortalWrt 25.12.1 soft router VM (2 vCPUs, 1 GB RAM). The load machine reaches the router over a LAN bridge, and the router proxies to the origin on the load machine. nginx 1.26.3 and HAProxy 3.2.21 come from the ImmortalWrt package repository, Gately uses the official OpenWrt apk package, and Lucky uses the official Linux binary.

<BenchBars title="HTTPS requests/s on the soft router (persistent connections)" unit="QPS" :better-label="H" :items="[['nginx',40665],['HAProxy',36266],['Gately',15669],['Lucky',8113]]" />

| Program | HTTP | HTTPS | New TLS/s | CPU at 2000 QPS | Idle memory | Load peak | Program size |
| --- | --- | --- | --- | --- | --- | --- | --- |
| **Gately** | 16,439 | 15,669 | 2,665 | 27% | 43 MiB | 65 MiB | 40.5 MB |
| nginx | **49,566** | **40,665** | **3,111** | **12%** | **15 MiB** | **22 MiB** | **0.8 MB** |
| HAProxy | 43,046 | 36,266 | 2,337 | 12% | 24 MiB | 29 MiB | 3.7 MB |
| Lucky | 8,642 | 8,113 | 2,021 | 44% | 64 MiB | 81 MiB | 17.9 MB |

- On the router the bottleneck is mainly kernel network send and receive (soft interrupts), and no program uses both cores fully, so the gaps are smaller than on the load machine. Gately's throughput is about 2 times Lucky's, with one third less memory.
- The OpenWrt kernel has no smaps, so memory here is estimated from `/proc/<pid>/statm` (the sum of each process's private resident memory plus one copy of the shared part). During the test the firewall's default SYN flood protection (about 25 new connections per second) was turned off temporarily, otherwise it would cap the new TLS connection test.
- For a reverse proxy alone, nginx from the package repository has the smallest size and memory; Gately's 40 MB includes all modules, and at [build.gately.cn](https://build.gately.cn) you can pick only the modules you need to produce a smaller package. On routers with little flash, check the free space first.

## All in one

One Gately runs all of these at once: two reverse proxy sites (HTTP and HTTPS, both with the WAF on), one web authentication site, 20 DDNS tasks, one intrusion prevention log rule and one port knocking rule. The comparison group is a combination of separate tools providing the same functions, adding up the idle memory measured above for each.

<BenchBars title="Total idle memory for the same set of functions" unit="MiB" better="lower" :digits="1" :better-label="L" :items="[['Gately',90.8],['nginx + ModSecurity + Authelia + ddns-go + fail2ban + fwknop',203.5],['Nginx Proxy Manager + Authelia + ddns-go + fail2ban + fwknop',297.3]]" note="The Nginx Proxy Manager group has no WAF." />

| Function | Separate tool | Idle memory | Install size |
| --- | --- | --- | --- |
| Reverse proxy + WAF | nginx + ModSecurity (CRS 4.25) | 61.9 MiB | 156 MB image |
| Reverse proxy (graphical UI) | Nginx Proxy Manager | 155.7 MiB | 1,959 MB image |
| Web authentication | Authelia | 104.4 MiB | 110 MB image |
| Dynamic DNS | ddns-go | 17.4 MiB | 43 MB image |
| Intrusion prevention | fail2ban | 17.6 MiB | 24 MB (with Python) |
| Port knocking | fwknop | 2.2 MiB | 0.3 MB |
| **All** | **Gately** | **90.8 MiB** | **40 MB program (18 MB download)** |

- Under 1000 requests/s of HTTPS traffic through the WAF, the all-in-one Gately instance peaks at 114 MiB of memory and 71% CPU; all 20 DDNS tasks synced successfully.
- Certificate issuance (certbot, acme.sh) runs only at renewal time and is not counted in resident memory.

## Optimizations in the development version

Based on the results above, the development version (unreleased, after commit `3d579389`) made the following changes and was re-tested in the same environment. Reverse proxy and 1000 sites use the build at commit `3d579389`; intrusion prevention uses a build that includes the last two changes. The baseline column is v0.6.8.1 re-measured on the same day (only the HTTP and HTTPS rows were re-measured that day; the other baselines are taken from the data above).

- Sites in the same listener that reverse proxy to the same backend with identical connection settings share one connection pool.
- When at least 1 GiB of memory is available, the default GC percent changes from 100 to 200 (adjustable in settings; small-memory devices keep 100).
- The reverse proxy copies the request once less and creates three fewer context values per request.
- Intrusion prevention reads log files with inotify, so a write is read immediately (the one-second poll stays as a fallback); ban writes to the firewall are merged at most once every 250 milliseconds.

| Metric | v0.6.8.1 | Development version | Change |
| --- | --- | --- | --- |
| HTTP requests/s (same-day baseline) | 35,968 | 38,358 | +6.6% |
| HTTPS requests/s (same-day baseline) | 34,072 | 36,079 | +5.9% |
| TLS new connections/s | 4,636 | 4,942 | +6.6% |
| HTTP/2 requests/s | 21,591 | 23,788 | +10% |
| 1000 sites requests/s | 33,211 | 38,839 | +17% |
| 1000 sites p99 latency | 29.8 ms | 11.7 ms | -61% |
| Peak memory during HTTP load | 64.5 MiB | 81.1 MiB | +26% |
| Intrusion prevention: single attacker ban latency p50 / p99 | 478 / 996 ms | 9.8 / 129 ms | |
| Intrusion prevention: ban latency under sustained attack p50 / p99 (20,000 lines/s) | 509 / 1006 ms | 143 / 269 ms | |
| Intrusion prevention: CPU at 1000 lines/s | 2.9% | 8.0% | higher |
| Intrusion prevention: CPU at 20,000 lines/s | 35.7% | 26.5% | -26% |
| Intrusion prevention: banning all 10,000 attacker IPs | 12.3 s, 10.1 CPU s | 10.1 s, 2.5 CPU s | |

- Most of the throughput gain comes from the GC setting, at the cost of a peak memory about ten-plus MiB higher during the load test. Profiling shows that almost all the remaining overhead on the proxy path is in Go's standard HTTP implementation and in GC; the gap to nginx and HAProxy comes from the language and runtime.
- With 1000 sites, throughput rose and latency fell because the shared connection pool means far fewer connections to the origin; peak memory did not drop, for the reason given above.
- The latency of a single ban is now lower than fail2ban's (280 / 864 ms). At low log rates CPU is higher than in the old version because the firewall is written at most 4 times per second, against once per second before; it is still about 40% of fail2ban's (20.4%).

## Reproduce

```bash
git clone https://github.com/wangwanjie/gately && cd gately/tools/benchmark
# Run as root on a Debian 13 VM; needs Docker plus wrk, oha and h2load
bash provision-debian.sh
python3 lib/run.py gately 3     # reverse proxy; replace with nginx, haproxy, caddy, traefik, lucky, npm
bash auth/run.sh                # web authentication
bash waf/run.sh                 # WAF
bash guard/run.sh               # intrusion prevention
bash knock/run.sh               # port knocking
bash scale/run.sh 1000          # 1000 sites
bash ddns/run.sh 20             # dynamic DNS
bash bundle/run.sh              # all in one
bash router/run.sh              # soft router (needs an additional OpenWrt machine)
```

Each directory's README describes the configuration, methodology and known limitations. Results depend on hardware, virtualization and configuration, so retest with your own workload.
