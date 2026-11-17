#!/bin/bash
# 认证网关专项对比入口（压测机上以 root 运行）：bash /root/benchmark/auth/run.sh [被测名,...]
# 不带参数依次测 gately、nginx-authelia、caddy-authelia、lucky；结果写 /root/benchmark/results/auth-<被测>.json
# 压测机上不需要设置任何环境变量；本机 colima 试跑见 colima.sh
set -e
D=$(cd "$(dirname "$0")" && pwd)
# 采样与压测工具都固定在压测 CPU 上，不占被测与源站的 CPU
exec taskset -c "${LOAD_CPUS:-4-7}" python3 "$D/bench.py" "$@"
