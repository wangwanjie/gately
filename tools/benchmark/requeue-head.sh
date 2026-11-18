#!/bin/bash
# 优化后的新版 Gately 复测：反代、1000 站点、入侵防护，结果加 head 区分，不覆盖正式版数据
B=/root/benchmark
cd $B
step() { echo "===== $1 $(date '+%F %T')" >> /root/requeue.log; }
step proxy-head;  python3 lib/run.py gately-head 3 > /root/rq-proxy-head.log 2>&1
step proxy-base;  python3 lib/run.py gately 3 http_ka,https_ka > /root/rq-proxy-base.log 2>&1
step startup-head; python3 lib/run.py gately-head 1 startup > /root/rq-startup-head.log 2>&1
docker rm -f sut >/dev/null 2>&1
step scale;       bash scale/run.sh 1000 gately-head > /root/rq-scale.log 2>&1
step guard
mkdir -p /root/res-old && cp results/guard-gately*.json /root/res-old/
GATELY_IMAGE=gately:bench-head bash guard/run.sh gately > /root/rq-guard.log 2>&1
for f in results/guard-gately*.json; do mv "$f" "${f/guard-gately/guard-head-gately}"; done
cp /root/res-old/guard-gately*.json results/
step done
