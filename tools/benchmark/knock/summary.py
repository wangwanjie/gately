#!/usr/bin/env python3
"""把 results/knock-*.json 汇总成 Markdown 表格。用法：summary.py [结果目录]"""
import glob
import json
import os
import sys

d = sys.argv[1] if len(sys.argv) > 1 else os.path.join(os.path.dirname(os.path.dirname(os.path.abspath(__file__))), "results")
order = ["direct", "gately", "gately-web", "fwknop", "fnknock"]
res = {}
for p in glob.glob(os.path.join(d, "knock-*.json")):
    j = json.load(open(p))
    res[j["sut"]] = j
names = [n for n in order if n in res]


def g(j, *path, default="-"):
    for k in path:
        if not isinstance(j, dict) or k not in j:
            return default
        j = j[k]
    return j


def f1(v):
    return f"{v:.1f}" if isinstance(v, (int, float)) else str(v)


print("## 资源与体积\n")
print("| 程序 | 空闲 PSS MiB | 进程数 | 启动秒 | 镜像 MB | 程序文件 MB |")
print("|---|---|---|---|---|---|")
for n in names:
    if n == "direct":
        continue
    j = res[n]
    print(f"| {n} | {f1(g(j, 'idle', 'pssKiB', default=0) / 1024)} | {g(j, 'idle', 'procs')} | {g(j, 'startupSec')} | "
          f"{f1(g(j, 'imageBytes', default=0) / 1e6)} | {f1(g(j, 'info', 'programBytes', default=0) / 1e6)} |")

print("\n## 敲门到放行延迟（ms）\n")
print("| 程序 | 成功/次数 | p50 | p90 | p99 | 不含验证码 p50 | 不含验证码 p99 | 验证码求解 p50 | 登录响应后到放行 p50 |")
print("|---|---|---|---|---|---|---|---|---|")
for n in names:
    L = res[n].get("latency")
    if not L:
        continue
    print(f"| {n} | {L['ok']}/{L['attempts']} | {g(L, 'total', 'p50ms')} | {g(L, 'total', 'p90ms')} | {g(L, 'total', 'p99ms')} | "
          f"{g(L, 'exCaptcha', 'p50ms')} | {g(L, 'exCaptcha', 'p99ms')} | {g(L, 'captchaSolve', 'p50ms')} | {g(L, 'afterLogin', 'p50ms')} |")

print("\n## 隐蔽性（未授权来源探测）\n")
for n in names:
    s = res[n].get("stealth")
    if not s:
        continue
    probes = "；".join(f"{k} {v.get('result')}" for k, v in s["probe"].items())
    print(f"- {n}：{probes}")

print("\n## 洪泛\n")
print("HTTP 场景的“实际速率”为 oha 每秒完成的尝试数（含连接错误），“得到响应”为收到 HTTP 响应的请求数，其余为连接被拒绝或重置。\n")
print("| 程序 | 场景 | 实际速率 | 得到响应 | 被测 CPU % | 峰值 PSS MiB | 合法敲门成功 | 合法敲门 p50 ms | p99 ms |")
print("|---|---|---|---|---|---|---|---|---|")
for n in names:
    for k, v in (res[n].get("flood") or {}).items():
        fl = v["flood"]
        rate = fl.get("achievedPps") or fl.get("rps")
        L = v["legitKnock"]
        answered = "-" if "achievedPps" in fl else sum((fl.get("statusCodes") or {}).values())
        print(f"| {n} | {k} | {f1(rate)} | {answered} | {v['sutCpuPercent']} | {f1(v['peakPssKiB'] / 1024)} | {L['ok']}/{L['attempts']} | "
              f"{g(L, 'total', 'p50ms')} | {g(L, 'total', 'p99ms')} |")

print("\n## 放行后的数据面（中位数）\n")
print("| 程序 | 长连接 rps | p50 ms | p99 ms | 被测 CPU % | 短连接 rps | p50 ms | p99 ms | 被测 CPU % |")
print("|---|---|---|---|---|---|---|---|---|")
for n in names:
    dp = res[n].get("dataplane")
    if not dp:
        continue
    a, b = dp["keepalive"]["median"], dp["newconn"]["median"]
    print(f"| {n} | {f1(a.get('rps'))} | {f1(a.get('p50ms'))} | {f1(a.get('p99ms'))} | {a.get('sutCpuPercent', '-')} | "
          f"{f1(b.get('rps'))} | {f1(b.get('p50ms'))} | {f1(b.get('p99ms'))} | {b.get('sutCpuPercent', '-')} |")
