#!/bin/bash
# 第三段：等第二段结束后补测 Gately 的 DDNS（修正 httpTimeout 后），并只保留 Gately 一段
B=/root/benchmark
cd $B
until grep -q "===== done" /root/queue2.log 2>/dev/null; do sleep 20; done
step() { echo "===== $1 $(date '+%F %T')" >> /root/queue3.log; }
step ddns-gately
sed -n '1,/^measure gately/p' ddns/run.sh > /tmp/ddns-gately-only.sh
bash /tmp/ddns-gately-only.sh 20 > /root/queue-ddns-gately.log 2>&1
docker exec backend sh -c "grep -c tool=gately /tmp/cb.log" >> /root/queue-ddns-gately.log
docker rm -f sut >/dev/null 2>&1
step done
