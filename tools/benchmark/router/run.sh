#!/bin/bash
# 路由器（ImmortalWrt 虚拟机 2 vCPU，绑定宿主机一个 P 核的两个超线程）反代对比。
# 在压测机上运行：压测工具在本机 vCPU 4-7，源站为本机 0.0.0.0:9001，被测程序跑在路由器上。
# 用法：bash router/run.sh [程序...]
B=/root/benchmark
R=${ROUTER:-root@192.168.18.48}
ME=${BENCH_IP:-192.168.18.49}
BE="$ME:9001"
TOOLS=${@:-gately nginx haproxy lucky}
DUR=20
SSH="ssh -o BatchMode=yes $R"
mkdir -p $B/results
grep -q r.bench.test /etc/hosts || echo "${R##*@} r.bench.test" >> /etc/hosts
$SSH 'mkdir -p /root/bench'
scp -O -q /etc/bench/certs/bench.crt /etc/bench/certs/bench.key /etc/bench/certs/bench.pem $B/router/rsample.sh $R:/root/bench/
$SSH 'chmod +x /root/bench/rsample.sh'
# OpenWrt 防火墙默认开启 SYN 洪泛保护（新连接约 25 个/秒），会把“每个请求新建 TLS 连接”的测试限住；
# 测试期间临时关闭，结束后恢复原值
SYNF=$($SSH 'uci -q get firewall.@defaults[0].synflood_protect || echo unset')
$SSH 'uci set firewall.@defaults[0].synflood_protect=0; /etc/init.d/firewall reload >/dev/null 2>&1'
restore_fw() {
  if [ "$SYNF" = unset ]; then
    $SSH 'uci -q delete firewall.@defaults[0].synflood_protect; /etc/init.d/firewall reload >/dev/null 2>&1'
  else
    $SSH "uci set firewall.@defaults[0].synflood_protect=$SYNF; /etc/init.d/firewall reload >/dev/null 2>&1"
  fi
}
trap restore_fw EXIT
stop_all() {
  $SSH 'killall gately nginx haproxy lucky ddns-go 2>/dev/null; sleep 1; killall -9 gately nginx haproxy lucky ddns-go 2>/dev/null; true'
}
ready() {
  for i in $(seq 1 600); do
    [ "$(curl -s -o /dev/null -w '%{http_code} %{size_download}' --max-time 2 http://r.bench.test:8080/)" = "200 1024" ] && return 0
    sleep 0.1
  done
  return 1
}
for t in $TOOLS; do
  stop_all
  case $t in
    gately)
      PAT=/usr/bin/gately
      rm -rf /tmp/rg && python3 $B/router/gen_gately.py /tmp/rg http://$BE /etc/bench/certs/bench.crt /etc/bench/certs/bench.key
      $SSH 'rm -rf /root/bench/gately && mkdir -p /root/bench/gately && cd /root/bench/gately && (GATELY_CONFIG_DIR=/root/bench/gately timeout 3 gately >/dev/null 2>&1; true)'
      scp -O -q /tmp/rg/* $R:/root/bench/gately/
      $SSH 'cd /root/bench/gately && GATELY_CONFIG_DIR=/root/bench/gately start-stop-daemon -S -b -x /usr/bin/gately' ;;
    nginx)
      PAT="nginx: "
      sed "s/BACKEND/$BE/" $B/router/nginx.conf > /tmp/r-nginx.conf; scp -O -q /tmp/r-nginx.conf $R:/root/bench/nginx.conf
      $SSH 'mkdir -p /var/lib/nginx /var/log/nginx; nginx -c /root/bench/nginx.conf' ;;
    haproxy)
      PAT=haproxy
      sed "s/BACKEND/$BE/" $B/router/haproxy.cfg > /tmp/r-haproxy.cfg; scp -O -q /tmp/r-haproxy.cfg $R:/root/bench/haproxy.cfg
      $SSH 'haproxy -D -f /root/bench/haproxy.cfg' ;;
    lucky)
      PAT=/opt/lucky/lucky
      $SSH 'rm -rf /root/bench/lucky && mkdir -p /root/bench/lucky && start-stop-daemon -S -b -x /opt/lucky/lucky -- -cd /root/bench/lucky -port 16601'
      sed -i 's/"Domains": \["bench.test"\]/"Domains": ["r.bench.test"]/' $B/suts/lucky/setup.py
      BACKEND=http://$BE python3 $B/suts/lucky/setup.py http://192.168.18.48:16601 /etc/bench/certs/bench.crt /etc/bench/certs/bench.key
      sed -i 's/"Domains": \["r.bench.test"\]/"Domains": ["bench.test"]/' $B/suts/lucky/setup.py ;;
  esac
  ready || { echo "$t 未就绪"; continue; }
  sleep 10
  IDLE=$($SSH "/root/bench/rsample.sh snapshot '$PAT'")
  OUT="{\"sut\":\"$t\",\"idle\":\"$IDLE\",\"tests\":{"
  first=1
  for spec in "http_ka|wrk -t4 -c64 -d${DUR}s --latency http://r.bench.test:8080/" \
              "https_ka|wrk -t4 -c64 -d${DUR}s --latency https://r.bench.test:8443/" \
              "https_new|oha --no-tui --insecure --disable-keepalive -c 32 -z ${DUR}s --output-format json https://r.bench.test:8443/" \
              "fixed2k|oha --no-tui -q 2000 -c 32 -z ${DUR}s --output-format json http://r.bench.test:8080/"; do
    name=${spec%%|*}; cmd=${spec#*|}
    for k in 1 2 3; do
      $SSH "/root/bench/rsample.sh watch '$PAT' $((DUR+1))" > /tmp/r-watch.txt &
      res=$(taskset -c 4-7 $cmd 2>&1)
      wait
      W=$(cat /tmp/r-watch.txt)
      J=$(python3 - "$res" "$W" <<'P'
import json, re, sys
t, w = sys.argv[1], sys.argv[2].split()
r = {}
if t.lstrip().startswith("{") or '"summary"' in t:
    j = json.loads(t[t.index("{"):]); s = j["summary"]; lp = j.get("latencyPercentiles", {})
    r = {"rps": s.get("requestsPerSec", 0), "p50ms": (lp.get("p50") or 0) * 1000, "p99ms": (lp.get("p99") or 0) * 1000,
         "ok": j.get("statusCodeDistribution", {}).get("200", 0)}
else:
    m = re.search(r"Requests/sec:\s+([\d.]+)", t); r["rps"] = float(m.group(1)) if m else 0
    for p in ("50", "99"):
        m = re.search(rf"\s{p}%\s+([\d.]+)(us|ms|s)", t)
        if m:
            r[f"p{p}ms"] = float(m.group(1)) * {"us": 0.001, "ms": 1, "s": 1000}[m.group(2)]
    m = re.search(r"Non-2xx or 3xx responses:\s+(\d+)", t); r["non2xx"] = int(m.group(1)) if m else 0
r["peakPssKiB"] = int(w[0]) if w else 0
r["sutCpuPercent"] = float(w[1]) if len(w) > 1 else 0
print(json.dumps(r))
P
)
      echo "$t $name $k $J"
      [ $first = 1 ] || OUT="$OUT,"; first=0
      OUT="$OUT\"$name-$k\":$J"
      sleep 3
    done
  done
  AFTER=$($SSH "/root/bench/rsample.sh snapshot '$PAT'")
  echo "$OUT},\"idleAfter\":\"$AFTER\"}" > $B/results/router-$t.json
  echo "DONE $t idle=$IDLE after=$AFTER"
done
stop_all
