#!/usr/bin/env python3
"""生成 Gately DDNS 配置：N 个任务，各 1 条 A 记录，IP 取自源站 /ip，用自定义回调写到源站 /cb。
用法：gen_gately.py <配置目录> <任务数> <检查周期秒> <源站>"""
import json
import os
import sys

d, n, interval, be = sys.argv[1], int(sys.argv[2]), int(sys.argv[3]), sys.argv[4]
tasks = []
for i in range(n):
    tasks.append({
        "key": f"t{i}", "name": f"bench{i}", "enable": True,
        "ipv4": {"enable": True, "type": "url", "urls": [f"{be}/ip"]},
        "ipv6": {"enable": False, "type": "url"},
        "provider": "callback",
        "credentials": {"id": "", "secret": "",
                        "extra": {"url": f"{be}/cb?tool=gately&domain={{domain}}&ip={{ip}}", "method": "GET"}},
        "records": [{"key": f"r{i}", "enable": True, "zone": "bench.test", "sub": f"t{i}", "type": "A", "value": "{ipv4Addr}", "ttl": 600}],
        "interval": interval,
        # 界面保存时会补默认超时；直接写配置文件时必须显式给出，否则加载后超时为 0
        "httpTimeout": 10,
    })
json.dump({"settings": {"enable": True}, "tasks": tasks}, open(os.path.join(d, "ddns.json"), "w"), indent=1)
json.dump({"scenePending": False}, open(os.path.join(d, "onboarding.json"), "w"))
