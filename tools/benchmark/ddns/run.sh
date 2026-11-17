#!/bin/bash
# DDNS 资源占用对比：Gately / ddns-go / Lucky 各配 N 个任务（回调到本机源站，不碰真实 DNS），
# 周期 30 秒（Gately 允许的最小值），预热 60 秒后采样 300 秒的平均 CPU 与内存。
# 用法：bash ddns/run.sh [任务数]
set -e
B=/root/benchmark
N=${1:-20}
INT=30
BE=http://127.0.0.1:9000
mkdir -p $B/results
cg() { echo /sys/fs/cgroup/system.slice/docker-$(docker inspect -f '{{.Id}}' sut).scope; }
cbcount() { docker exec backend sh -c "grep -c 'tool=$1' /tmp/cb.log 2>/dev/null || true"; }
measure() {
  local name=$1
  sleep 60
  local before=$(cbcount $name)
  python3 $B/lib/sample.py watch $(cg) 300 /tmp/ddns-watch.json
  local after=$(cbcount $name)
  local idle=$(python3 $B/lib/sample.py snapshot $(cg))
  local img=$(docker inspect -f '{{.Config.Image}}' sut)
  python3 - "$name" "$N" "$before" "$after" "$idle" "$img" <<'P'
import json, subprocess, sys
name, n, before, after, idle, img = sys.argv[1:7]
w = json.load(open("/tmp/ddns-watch.json"))
size = int(subprocess.run(["docker", "image", "inspect", "-f", "{{.Size}}", img], capture_output=True, text=True).stdout.strip() or 0)
res = {"sut": name, "tasks": int(n), "intervalSec": 30, "window": w, "idle": json.loads(idle),
       "callbacksInWindow": int(after or 0) - int(before or 0), "image": img, "imageBytes": size}
json.dump(res, open(f"/root/benchmark/results/ddns-{name}.json", "w"), indent=1)
print(name, json.dumps(res))
P
  docker rm -f sut >/dev/null
}
docker rm -f sut >/dev/null 2>&1 || true

# Gately
rm -rf /tmp/sut-gately-ddns && mkdir -p /tmp/sut-gately-ddns
docker run --rm --entrypoint sh -v /tmp/sut-gately-ddns:/etc/gately ghcr.io/wangwanjie/gately:v0.6.8.1 -c 'timeout 3 gately >/dev/null 2>&1; true'
python3 $B/ddns/gen_gately.py /tmp/sut-gately-ddns $N $INT $BE
docker run -d --name sut --network host --cpuset-cpus 0,1 -v /tmp/sut-gately-ddns:/etc/gately ghcr.io/wangwanjie/gately:v0.6.8.1 >/dev/null
measure gately

# ddns-go（默认带网页界面，与其他两者一致）
rm -rf /tmp/sut-ddnsgo && mkdir -p /tmp/sut-ddnsgo
python3 $B/ddns/gen_ddnsgo.py /tmp/sut-ddnsgo/.ddns_go_config.yaml $N $BE
docker run -d --name sut --network host --cpuset-cpus 0,1 -v /tmp/sut-ddnsgo:/root jeessy/ddns-go:v6.17.9 -l :9876 -f $INT >/dev/null
measure ddnsgo

# Lucky
rm -rf /tmp/sut-lucky-ddns && mkdir -p /tmp/sut-lucky-ddns
docker run -d --name sut --network host --cpuset-cpus 0,1 -v /tmp/sut-lucky-ddns:/app/conf gdy666/lucky:v3.8.1_lucky >/dev/null
python3 $B/ddns/setup_lucky.py http://127.0.0.1:16601 $N $INT $BE
measure lucky
