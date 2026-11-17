#!/bin/bash
# 第四段：等第三段结束后，用修正后的采样与防火墙设置重跑路由器对比，再补跑 ddns-go（路由器上）
B=/root/benchmark
cd $B
until grep -q "===== done" /root/queue3.log 2>/dev/null; do sleep 20; done
step() { echo "===== $1 $(date '+%F %T')" >> /root/queue4.log; }
step router
bash router/run.sh > /root/queue-router2.log 2>&1
step done
