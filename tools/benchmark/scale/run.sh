#!/bin/bash
# 站点数扩展性：各程序配置 N 个站点，测空闲内存、重启到可用的耗时、随机 Host 请求的 QPS。
# 用法：bash scale/run.sh [N] [程序...]
B=/root/benchmark
N=${1:-1000}; shift
TOOLS=${@:-gately nginx haproxy caddy traefik}
export SITES=$N
mkdir -p $B/results
cg() { echo /sys/fs/cgroup/system.slice/docker-$(docker inspect -f '{{.Id}}' sut).scope; }
ready() {
  # 等到最后一个站点也能正确响应
  local t0=$1
  for i in $(seq 1 6000); do
    [ "$(curl -s -o /dev/null -w '%{http_code} %{size_download}' --max-time 2 -H "Host: s$((N-1)).bench.test" http://127.0.0.1:8080/)" = "200 1024" ] && { python3 -c "import time;print(round(time.time()-$t0,3))"; return; }
    sleep 0.05
  done
  echo -1
}
for t in $TOOLS; do
  docker rm -f sut >/dev/null 2>&1
  D=/tmp/scale-$t; rm -rf $D; mkdir -p $D
  case $t in
    gately|gately-head)
      # gately-head 为本地构建的新版镜像（见 suts/gately-head），与正式版对照
      GIMG=ghcr.io/wangwanjie/gately:v0.6.8.1
      [ $t = gately-head ] && GIMG=${GATELY_HEAD_IMAGE:-gately:bench-head}
      docker run --rm --entrypoint sh -v $D:/etc/gately $GIMG -c 'timeout 3 gately >/dev/null 2>&1; true'
      python3 $B/scale/gen.py gately $N $D
      RUN="docker run -d --name sut --network host --cpuset-cpus 0,1 --ulimit nofile=200000:200000 -v $D:/etc/gately $GIMG" ;;
    nginx)
      python3 $B/scale/gen.py nginx $N $D
      RUN="docker run -d --name sut --network host --cpuset-cpus 0,1 --ulimit nofile=200000:200000 -v $D/nginx.conf:/etc/nginx/nginx.conf:ro -v /etc/bench/certs:/certs:ro nginx:1.29-alpine" ;;
    haproxy)
      python3 $B/scale/gen.py haproxy $N $D
      RUN="docker run -d --name sut --network host --cpuset-cpus 0,1 --ulimit nofile=400000:400000 -v $D/haproxy.cfg:/usr/local/etc/haproxy/haproxy.cfg:ro -v /etc/bench/certs:/certs:ro haproxy:3.2-alpine" ;;
    caddy)
      python3 $B/scale/gen.py caddy $N $D
      RUN="docker run -d --name sut --network host --cpuset-cpus 0,1 --ulimit nofile=200000:200000 -v $D/Caddyfile:/etc/caddy/Caddyfile:ro -v /etc/bench/certs:/certs:ro caddy:2-alpine" ;;
    traefik)
      python3 $B/scale/gen.py traefik $N $D
      cp $B/suts/traefik/traefik.yml $D/
      RUN="docker run -d --name sut --network host --cpuset-cpus 0,1 --ulimit nofile=200000:200000 -v $D/traefik.yml:/etc/traefik/traefik.yml:ro -v $D/dynamic.yml:/etc/traefik/dynamic.yml:ro -v /etc/bench/certs:/certs:ro traefik:v3" ;;
  esac
  $RUN >/dev/null
  ready $(date +%s.%N) >/dev/null
  # 重启 3 次测启动耗时（配置已就绪，计时到最后一个站点可用）
  ST=""
  for k in 1 2 3; do
    docker stop -t 10 sut >/dev/null; sync; echo 3 > /proc/sys/vm/drop_caches
    T0=$(date +%s.%N); docker start sut >/dev/null; ST="$ST $(ready $T0)"
  done
  sleep 15
  IDLE=$(python3 $B/lib/sample.py snapshot $(cg))
  python3 $B/lib/sample.py watch $(cg) 22 /tmp/scale-watch.json &
  W=$(taskset -c 4-7 wrk -t4 -c128 -d20s --latency -s $B/scale/hosts.lua http://127.0.0.1:8080/)
  wait
  python3 - "$t" "$N" "$ST" "$IDLE" "$W" <<'P'
import json, re, statistics, sys
t, n, st, idle, w = sys.argv[1:6]
rps = float(re.search(r"Requests/sec:\s+([\d.]+)", w).group(1))
non2xx = re.search(r"Non-2xx or 3xx responses:\s+(\d+)", w)
p99 = re.search(r"\s99%\s+([\d.]+)(us|ms|s)", w)
p99ms = float(p99.group(1)) * {"us": 0.001, "ms": 1, "s": 1000}[p99.group(2)] if p99 else None
starts = [float(x) for x in st.split()]
res = {"sut": t, "sites": int(n), "startupSec": statistics.median(starts), "startupRuns": starts,
       "idle": json.loads(idle), "rps": rps, "p99ms": p99ms, "non2xx": int(non2xx.group(1)) if non2xx else 0,
       "watch": json.load(open("/tmp/scale-watch.json"))}
json.dump(res, open(f"/root/benchmark/results/scale{n}-{t}.json", "w"), indent=1)
print(json.dumps(res))
P
  docker rm -f sut >/dev/null
done
