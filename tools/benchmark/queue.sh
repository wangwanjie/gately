#!/bin/bash
# 按顺序跑完剩余专项（互不重叠），每段的输出写到 /root/queue-<段>.log
B=/root/benchmark
cd $B
step() { echo "===== $1 $(date '+%F %T')" >> /root/queue.log; }
step npm-h2;  NPM_KEEPALIVE_REQUESTS=1000000 python3 lib/run.py npm 3 h2 > /root/queue-npm-h2.log 2>&1; docker rm -f sut >/dev/null 2>&1
step auth;    bash auth/run.sh > /root/queue-auth.log 2>&1
step waf;     bash waf/run.sh > /root/queue-waf.log 2>&1
step guard;   bash guard/run.sh > /root/queue-guard.log 2>&1
step scale;   bash scale/run.sh 1000 > /root/queue-scale.log 2>&1
step ddns;    bash ddns/run.sh 20 > /root/queue-ddns.log 2>&1
step router;  bash router/run.sh > /root/queue-router.log 2>&1
step done
