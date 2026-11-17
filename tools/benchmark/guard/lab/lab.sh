#!/bin/bash
# 在本机 colima 里搭“迷你压测机”跑通 guard 专项（只用于验证脚本，数值不作结论）。
# 用法：bash tools/benchmark/guard/lab/lab.sh [被测列表] [测试列表]
# 做法：特权容器 bx-guard-lab（独立网络命名空间，systemd 为 1 号进程，容器内自带 Docker），
# 与压测机一样 apt 安装 fail2ban、在容器内 Docker 里以 --network host 跑 Gately；
# 把 tools/benchmark 复制进容器的 /root/benchmark（不挂载，结果不写回仓库），跑完把结果拷到 /tmp/bx-guard-results。
# colima 只有 6 个 CPU：被测 0,1，脚本与注入器 4,5。
set -e
D=$(cd "$(dirname "$0")" && pwd)
BENCH=$(cd "$D/../.." && pwd)
IMG=${GATELY_IMAGE:-ghcr.io/wangwanjie/gately:v0.6.8.1}
docker build -q -t bx-guard-lab:1 "$D" >/dev/null
docker image inspect "$IMG" >/dev/null 2>&1 || docker pull "$IMG"
docker rm -f bx-guard-lab >/dev/null 2>&1 || true
docker run -d --name bx-guard-lab --privileged --cgroupns=private --tmpfs /run --tmpfs /run/lock bx-guard-lab:1 >/dev/null
# 等 systemd 与容器内 dockerd 起来
for i in $(seq 60); do
  docker exec bx-guard-lab docker info >/dev/null 2>&1 && break
  sleep 1
done
docker save "$IMG" | docker exec -i bx-guard-lab docker load >/dev/null
docker exec bx-guard-lab sh -c 'apt-get update -qq && apt-get install -y -qq fail2ban python3-pyinotify python3-systemd >/dev/null'
docker exec bx-guard-lab mkdir -p /root/benchmark
docker cp "$BENCH/lib" bx-guard-lab:/root/benchmark/lib
docker cp "$BENCH/guard" bx-guard-lab:/root/benchmark/guard
docker exec -e LOAD_CPUS=4-5 -e SKIP_DROP_CACHES=1 \
  -e GUARD_RATES -e GUARD_STREAM_SEC -e GUARD_LAT_SAMPLES -e GUARD_BIG_ATTACKERS -e GUARD_IDLE_WAIT \
  bx-guard-lab bash /root/benchmark/guard/run.sh "$@"
rm -rf /tmp/bx-guard-results
docker cp bx-guard-lab:/root/benchmark/results /tmp/bx-guard-results
echo "结果已复制到 /tmp/bx-guard-results；用完删除：docker rm -f bx-guard-lab"
