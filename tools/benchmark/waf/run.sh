#!/usr/bin/env bash
# WAF 专项对比入口（压测机上以 root 运行：bash /root/benchmark/waf/run.sh）
# 依次测 Gately、nginx + ModSecurity、Caddy + coraza-caddy，互不同时运行；最后生成汇总。
# 可用环境变量 SUTS 只跑其中几项，例如 SUTS="gately modsec" bash run.sh；其余变量见 bench.py 开头的说明。
set -euo pipefail
D=$(cd "$(dirname "$0")" && pwd)
SUTS=${SUTS:-"gately modsec caddy-coraza"}
ROUNDS=${ROUNDS:-3}
RES=${RESULTS:-$(dirname "$D")/results}
# 源站必须在线：/ 返回 1024 字节
curl -s -o /dev/null -w "backend %{http_code} %{size_download}\n" http://${BACKEND:-127.0.0.1:9000}/
# 先把三组样本直接发给源站一次，用来识别“源站自己就返回 403”的样本（汇总时排除）
mkdir -p "$RES"
BE=${BACKEND:-127.0.0.1:9000}
for s in attack benign crs-regression; do
  python3 $D/tools/replay.py $D/samples/$s.jsonl ${BE%:*} ${BE##*:} $RES/waf-backend-$s.json 8
done
for s in $SUTS; do
  echo "===== $s $(date '+%F %T')"
  python3 $D/bench.py $s $ROUNDS 2>&1 | tee $RES/waf-$s.log
done
python3 $D/report.py $RES | tee $RES/waf-report.md
