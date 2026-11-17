#!/bin/bash
# 第二段队列：等第一段（queue.sh）结束后依次运行端口敲门、一体化与 Gately 启动耗时补测
B=/root/benchmark
cd $B
until grep -q "===== done" /root/queue.log 2>/dev/null; do sleep 20; done
step() { echo "===== $1 $(date '+%F %T')" >> /root/queue2.log; }
step prep
[ -f /root/img-debian.tar ] && docker load -i /root/img-debian.tar >/dev/null && docker tag $(docker load -i /root/img-debian.tar | awk '/Loaded image/{print $NF}') debian:trixie-slim 2>/dev/null
DEBIAN_FRONTEND=noninteractive apt-get install -y -qq nmap iproute2 >/dev/null 2>&1
step knock;        bash knock/run.sh > /root/queue-knock.log 2>&1
step knock-spoof;  python3 knock/spoof_probe.py gately > /root/queue-knock-spoof.log 2>&1
step bundle;       bash bundle/run.sh > /root/queue-bundle.log 2>&1
step gately-startup; python3 lib/run.py gately 1 startup > /root/queue-gately-startup.log 2>&1; docker rm -f sut >/dev/null 2>&1
step done
