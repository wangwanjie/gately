# WAF 对比结果

被测：Gately WAF（ghcr.io/wangwanjie/gately:v0.6.8.1）、nginx + ModSecurity（owasp/modsecurity-crs:4.25.0-nginx-alpine-202606290906）、Caddy + coraza-caddy（bench/caddy-coraza:2.11.4-c2.6.2）

## 1. 吞吐与延迟（wrk -t4 -c128，HTTP，3 轮中位数）

| 程序 | 压测项 | 关闭 WAF rps | 开启 WAF rps | 下降 | p50 关/开 ms | p99 关/开 ms | CPU% 关/开 | 每千请求 CPU ms 关/开 | 峰值 PSS MB 关/开 | 开启时 non2xx |
|---|---|---|---|---|---|---|---|---|---|---|
| Gately WAF | get | 31685 | 2498 | 92.1% | 3.68/48.75 | 11.54/89.83 | 198.8/200.2 | 62.57/799.06 | 66.4/139.7 | 0 |
| Gately WAF | query | 30638 | 1067 | 96.5% | 3.8/120.42 | 11.9/195.1 | 199.2/200.3 | 64.76/1872.41 | 65.9/170.1 | 0 |
| Gately WAF | post4k | 24730 | 72 | 99.7% | 4.72/1630.0 | 14.15/3070.0 | 198.6/206.2 | 80.09/28606.02 | 72.0/753.6 | 1442 |
| Gately WAF | attack | 27622 | 1218 | 95.6% | 4.23/111.66 | 13.34/462.15 | 199.6/201.1 | 71.91/1644.95 | 69.1/353.4 | 23144 |
| nginx + ModSecurity | get | 114128 | 1491 | 98.7% | 1.13/85.17 | 1.39/91.1 | 200.3/199.8 | 17.5/1336.22 | 24.7/67.8 | 0 |
| nginx + ModSecurity | query | 107140 | 591 | 99.4% | 1.18/215.12 | 1.43/1410.0 | 200.2/200.7 | 18.63/3386.79 | 24.7/67.1 | 0 |
| nginx + ModSecurity | post4k | 92182 | 102 | 99.9% | 1.39/1050.0 | 1.63/7000.0 | 200.7/205.3 | 21.67/20089.76 | 25.3/73.0 | 2044 |
| nginx + ModSecurity | attack | 98991 | 1014 | 99.0% | 1.21/108.59 | 6.25/1640.0 | 192.9/200.3 | 19.39/1969.07 | 25.3/66.2 | 19243 |
| Caddy + coraza-caddy | get | 24027 | 2265 | 90.6% | 4.88/55.92 | 13.97/95.8 | 198.0/200.2 | 82.22/881.28 | 78.6/147.2 | 0 |
| Caddy + coraza-caddy | query | 23832 | 943 | 96.0% | 4.93/135.52 | 14.22/212.23 | 197.9/200.5 | 82.84/2117.69 | 79.4/187.7 | 0 |
| Caddy + coraza-caddy | post4k | 19375 | 46 | 99.8% | 6.15/2580.0 | 16.79/3780.0 | 197.6/212.6 | 101.84/45924.96 | 78.4/791.9 | 926 |
| Caddy + coraza-caddy | attack | 21547 | 1004 | 95.3% | 5.46/116.92 | 15.59/365.41 | 199.0/200.8 | 91.92/1991.04 | 77.1/197.3 | 19090 |

## 2. 启动时间与空闲内存

| 程序 | 启动 s 关/开（中位数） | 规则加载耗时（差值）s | 空闲 PSS MB 关/开 | 规则占用内存（差值）MB | 回放后空闲 PSS MB（开） |
|---|---|---|---|---|---|
| Gately WAF | 0.616/0.753 | 0.14 | 47.6/88.3 | 40.7 | 233.7 |
| nginx + ModSecurity | 0.78/0.907 | 0.13 | 23.7/61.9 | 38.3 | 62.7 |
| Caddy + coraza-caddy | 0.645/0.734 | 0.09 | 55.3/95.9 | 40.6 | 179.7 |

## 3. 攻击样本（真阳性）

样本 557 条。

| 程序 | 拦截 | 其中非 403 的拦截 | 代理本身拒绝（不计入） | xss(217) | rce(84) | lfi(78) | sqli(37) | nosql(25) | scanner-ua(24) | rfi(21) | ldap(12) | mail(12) | ssi(12) | ssti(12) | xxe(9) | api-mixed(7) | crlf(7) |
|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|
| Gately WAF | 430 (77.2%) | 1 {400: 1} | 0 | 87.1% | 66.7% | 91.0% | 86.5% | 48.0% | 87.5% | 71.4% | 16.7% | 25.0% | 100.0% | 50.0% | 11.1% | 57.1% | 85.7% |
| nginx + ModSecurity | 430 (77.2%) | 1 {400: 1} | 1 | 86.6% | 66.7% | 91.0% | 86.5% | 48.0% | 87.5% | 71.4% | 16.7% | 25.0% | 100.0% | 58.3% | 11.1% | 57.1% | 85.7% |
| Caddy + coraza-caddy | 430 (77.2%) | 1 {400: 1} | 0 | 87.1% | 66.7% | 91.0% | 86.5% | 48.0% | 87.5% | 71.4% | 16.7% | 25.0% | 100.0% | 50.0% | 11.1% | 57.1% | 85.7% |

三方结论不一致 2 条：gately=放/modsec=拦/caddy-coraza=放 1 条；gately=拦/modsec=放/caddy-coraza=拦 1 条

- `gtw-ssti-d2b82659` ssti URLPath `GET /aaaa%5Cu0027%252b%23%7B16%2A8787%7D%252b%5Cu0027bbb` '' → gately:404, modsec:403, caddy-coraza:404
- `gtw-xss-841c59b6` xss URLPath `GET /%3C%3Cscr%00ipt%2Fsrc=http:%2F%2Fxss.com%2Fxss.js%3E%3C%2Fscript` '' → gately:403, modsec:400, caddy-coraza:403

## 3. 正常样本（误报）

样本 389 条。

| 程序 | 拦截 | 其中非 403 的拦截 | 代理本身拒绝（不计入） | tech-content(179) | gotestwaf-fp(141) | browse(40) | api-json(20) | form(9) |
|---|---|---|---|---|---|---|---|---|
| Gately WAF | 78 (20.1%) | 0  | 0 | 33.0% | 9.2% | 0.0% | 30.0% | 0.0% |
| nginx + ModSecurity | 78 (20.1%) | 0  | 0 | 33.0% | 9.2% | 0.0% | 30.0% | 0.0% |
| Caddy + coraza-caddy | 78 (20.1%) | 0  | 0 | 33.0% | 9.2% | 0.0% | 30.0% | 0.0% |

三方结论不一致 0 条：


## 3. CRS 4.25.0 回归用例（PL1，期望命中的规则不一定足以阻断，只看三方一致性）

样本 2049 条。

| 程序 | 拦截 | 其中非 403 的拦截 | 代理本身拒绝（不计入） |  |
|---|---|---|---|
| Gately WAF | 1965 (95.9%) | 1 {400: 1} | 0 |  |
| nginx + ModSecurity | 1951 (95.2%) | 71 {400: 71} | 1 |  |
| Caddy + coraza-caddy | 1960 (95.7%) | 1 {400: 1} | 0 |  |

三方结论不一致 16 条：gately=拦/modsec=放/caddy-coraza=拦 9 条；gately=拦/modsec=放/caddy-coraza=放 6 条；gately=放/modsec=拦/caddy-coraza=拦 1 条

- `crs-920100-14` REQUEST-920-PROTOCOL-ENFORCEMENT ftw `|GET /get` '' → gately:403, modsec:400, caddy-coraza:403
- `crs-933110-3` REQUEST-933-APPLICATION-ATTACK-PHP ftw `GET /upload2` '' → gately:403, modsec:404, caddy-coraza:404
- `crs-933110-13` REQUEST-933-APPLICATION-ATTACK-PHP ftw `GET /upload6` '' → gately:403, modsec:404, caddy-coraza:404
- `crs-933110-14` REQUEST-933-APPLICATION-ATTACK-PHP ftw `GET /upload7` '' → gately:403, modsec:404, caddy-coraza:404
- `crs-933110-20` REQUEST-933-APPLICATION-ATTACK-PHP ftw `GET /upload2` '' → gately:403, modsec:404, caddy-coraza:403
- `crs-933110-21` REQUEST-933-APPLICATION-ATTACK-PHP ftw `GET /upload6` '' → gately:403, modsec:404, caddy-coraza:403
- `crs-933110-22` REQUEST-933-APPLICATION-ATTACK-PHP ftw `GET /upload7` '' → gately:403, modsec:404, caddy-coraza:403
- `crs-933110-24` REQUEST-933-APPLICATION-ATTACK-PHP ftw `GET /upload7` '' → gately:403, modsec:404, caddy-coraza:403
- `crs-933110-25` REQUEST-933-APPLICATION-ATTACK-PHP ftw `GET /upload7` '' → gately:403, modsec:404, caddy-coraza:403
- `crs-933110-26` REQUEST-933-APPLICATION-ATTACK-PHP ftw `GET /upload7` '' → gately:403, modsec:404, caddy-coraza:403
- `crs-933110-27` REQUEST-933-APPLICATION-ATTACK-PHP ftw `GET /upload7` '' → gately:403, modsec:404, caddy-coraza:403
- `crs-933220-2` REQUEST-933-APPLICATION-ATTACK-PHP ftw `GET /upload` '' → gately:403, modsec:404, caddy-coraza:404
- `crs-933220-4` REQUEST-933-APPLICATION-ATTACK-PHP ftw `GET /upload` '' → gately:403, modsec:404, caddy-coraza:403
- `crs-942220-2` REQUEST-942-APPLICATION-ATTACK-SQLI ftw `GET /get?i=2.2250738585072011e-308` '' → gately:404, modsec:403, caddy-coraza:403
- `crs-944140-3` REQUEST-944-APPLICATION-ATTACK-JAVA ftw `GET /get/upload2` '' → gately:403, modsec:404, caddy-coraza:404

