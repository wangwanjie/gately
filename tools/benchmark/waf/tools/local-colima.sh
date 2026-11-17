#!/usr/bin/env bash
# 仅供本机 colima 调试：在特权的驱动容器（host 网络、host PID、挂载 docker.sock 与 cgroup）里运行 run.sh，
# 端口、CPU 与目录都换成不与本机其他容器冲突的值。需事先在 colima 虚拟机里准备 /tmp/bx-waf
# （certs/bench.crt|key、srv/1k.txt、backend.conf）并启动源站容器 bx-waf-backend（127.0.0.1:19500）。
# 用法：tools/local-colima.sh [run.sh 的环境变量，如 SUTS=gately DUR=5 ROUNDS=1]
set -euo pipefail
W=$(cd "$(dirname "$0")/.." && pwd)
docker rm -f bx-waf-driver-run >/dev/null 2>&1 || true
docker run --rm --name bx-waf-driver-run --network host --pid host --privileged \
  --add-host bench.test:127.0.0.1 \
  -v /var/run/docker.sock:/var/run/docker.sock -v /sys/fs/cgroup:/sys/fs/cgroup \
  -v "$(dirname "$W"):$(dirname "$W")" -v /tmp/bx-waf:/tmp/bx-waf \
  -e HP=18180 -e SP=18543 -e BACKEND=127.0.0.1:19500 -e CERTS=/tmp/bx-waf/certs -e SUT_NAME=bx-waf-sut \
  -e SUT_CPUS=0,1 -e LOAD_CPUS=4,5 -e WORKDIR=/tmp/bx-waf -e ADMIN_PORT=16880 -e RESULTS=/tmp/bx-waf/results \
  $(for kv in "$@"; do printf -- '-e %s ' "$kv"; done) \
  bx-waf-driver bash "$W/run.sh"
