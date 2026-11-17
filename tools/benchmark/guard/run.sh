#!/bin/bash
# 入侵防护专项：Gately 入侵防护模块 vs fail2ban 1.1（Debian 13 apt 包，nftables-multiport）。
# 在压测虚拟机上以 root 运行：bash /root/benchmark/guard/run.sh [被测列表] [测试列表]
#   被测列表默认 gately,fail2ban,gately-web,gately-full（依次运行，互不重叠）
#   测试列表默认全部（idle,latency,stream,big），例如只跑延迟：run.sh gately,fail2ban latency
# 结果：/root/benchmark/results/guard-<被测>.json，过程输出同时写到 results/guard-run.log。
set -e
D=$(cd "$(dirname "$0")" && pwd)
ROOT=$(dirname "$D")
SUTS=${1:-gately,fail2ban,gately-web,gately-full}
TESTS=${2:-}
# 本脚本、注入器与 nft monitor 固定在压测工具的 vCPU 上
LOAD_CPUS=${LOAD_CPUS:-4-7}
export LOAD_CPUS

[ "$(id -u)" = 0 ] || { echo "需要 root"; exit 1; }
command -v nft >/dev/null || { echo "缺少 nft"; exit 1; }
mkdir -p "$ROOT/results"

# 两边的表都不应残留（上一次中断时可能留下）
nft delete table inet gately 2>/dev/null || true
nft delete table inet f2b-table 2>/dev/null || true
docker rm -f bx-guard-gately >/dev/null 2>&1 || true
systemctl stop fail2ban 2>/dev/null || true

for s in ${SUTS//,/ }; do
  echo "==== $s ===="
  # 本机调试（lab 容器）时设 SKIP_DROP_CACHES=1，不清理宿主机页缓存
  [ -n "$SKIP_DROP_CACHES" ] || { sync; echo 3 > /proc/sys/vm/drop_caches; }
  taskset -c "$LOAD_CPUS" python3 "$D/guard.py" "$s" $TESTS 2>&1 | tee -a "$ROOT/results/guard-run.log"
  sleep 5
done
python3 "$D/report.py" "$ROOT/results" | tee "$ROOT/results/guard-summary.txt"
