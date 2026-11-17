#!/usr/bin/env python3
"""通过 Lucky 接口建 N 个 DDNS 任务（回调到源站 /cb），全局检查周期设为指定秒数。
用法：setup_lucky.py <管理地址> <任务数> <周期秒> <源站>"""
import json
import os
import sys

sys.path.insert(0, os.path.join(os.path.dirname(os.path.abspath(__file__)), "..", "lib"))
from lucky_api import Lucky  # noqa: E402

base, n, interval, be = sys.argv[1], int(sys.argv[2]), int(sys.argv[3]), sys.argv[4]
lk = Lucky(base)
tpl = json.load(open(os.path.join(os.path.dirname(os.path.abspath(__file__)), "lucky_task.json")))
conf = lk.req("/api/ddns/configure")
c = conf.get("ddnsconfigure") or {}
if c:
    c["Intervals"] = interval
    c["FirstCheckDelay"] = 1
    print("configure", lk.req("/api/ddns/configure", c, method="PUT").get("ret"))
for i in range(n):
    t = json.loads(json.dumps(tpl))
    t["TaskName"] = f"bench{i}"
    t["Intervals"] = interval
    t["FirstCheckDelay"] = 1
    t["V4QueryUrl"] = f"{be}/ip"
    t["Records"][0]["SyncRecordData"]["fullDomainName"] = f"t{i}.bench.test"
    t["DNS"]["Callback"]["URL"] = f"{be}/cb?tool=lucky&domain=#{{domain}}&ip=#{{ip}}"
    r = lk.req("/api/ddns", t)
    if r.get("ret") != 0:
        print("task", i, r)
print("tasks", n)
