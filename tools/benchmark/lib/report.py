#!/usr/bin/env python3
"""把 results/*.json 汇总成 Markdown 表格与官网图表用的 summary.json。
用法：report.py <results 目录> <输出目录>"""
import glob
import json
import os
import sys

src, out = sys.argv[1], sys.argv[2]
os.makedirs(out, exist_ok=True)
NAMES = {"gately": "Gately", "nginx": "nginx", "haproxy": "HAProxy", "caddy": "Caddy", "traefik": "Traefik",
         "lucky": "Lucky", "npm": "Nginx Proxy Manager"}
ORDER = ["gately", "nginx", "haproxy", "caddy", "traefik", "lucky", "npm"]


def load(pattern, exclude=None):
    res = {}
    for p in glob.glob(os.path.join(src, pattern)):
        if exclude and exclude in os.path.basename(p):
            continue
        j = json.load(open(p))
        # 结果目录里还有各专项的其他格式文件，只取带 sut 字段的对象
        if isinstance(j, dict) and "sut" in j:
            res[j["sut"]] = j
    return res


def mib(kib):
    return round(kib / 1024, 1)


# 反代主测试的结果文件名就是程序名（results/<名>.json），启动耗时补测为 <名>-startup.json
proxy, startup = {}, {}
for k in ORDER:
    p = os.path.join(src, f"{k}.json")
    if os.path.exists(p):
        proxy[k] = json.load(open(p))
    # HTTP/2 单独补测的结果（npm-h2.json）覆盖主测试里失真的 h2 一项
    p = os.path.join(src, f"{k}-h2.json")
    if os.path.exists(p) and k in proxy:
        proxy[k]["tests"]["h2"] = json.load(open(p))["tests"]["h2"]
    p = os.path.join(src, f"{k}-startup.json")
    if os.path.exists(p):
        startup[k] = json.load(open(p))
summary = {"proxy": {}}
rows = []
for k in [x for x in ORDER if x in proxy]:
    j = proxy[k]
    t = {n: v["median"] for n, v in j["tests"].items()}
    st = startup.get(k, {}).get("startupSec", j.get("startupSec"))
    s = {
        "name": NAMES.get(k, k), "image": j.get("image"), "imageMB": round(j.get("imageBytes", 0) / 1e6, 1),
        "idleMiB": mib(j["idle"]["pssKiB"]), "idleAfterMiB": mib(j.get("idleAfter", j["idle"])["pssKiB"]),
        "procs": j["idle"].get("procs"), "startupSec": st, "info": j.get("info", ""),
        "httpRps": round(t["http_ka"]["rps"]), "httpP99": t["http_ka"].get("p99ms"),
        "httpsRps": round(t["https_ka"]["rps"]), "httpsP99": t["https_ka"].get("p99ms"),
        "tlsNewRps": round(t["https_new"]["rps"]),
        # h2load 遇到服务端按请求数上限关闭连接时不会重连，结果失真：有失败请求时记为未测
        "h2Rps": round(t["h2"]["rps"]) if "h2" in t and not t["h2"].get("failed") else None,
        "fixedCpu": t["fixed5k"]["sutCpuPercent"], "fixedP99": round(t["fixed5k"]["p99ms"], 2),
        "bigMBps": round(t["big"].get("MBps", 0)),
        "peakMiB": mib(max(v["median"].get("peakPssKiB", 0) for v in j["tests"].values() if "peakPssKiB" in v["median"])),
        "perConnKiB": t.get("conn10k", {}).get("perConnKiB"),
    }
    summary["proxy"][k] = s
    rows.append(s)

md = ["| 程序 | HTTP QPS | HTTPS QPS | HTTP/2 QPS | TLS 新建/秒 | 5000 QPS 时 CPU | 100MB 大文件 MB/s | 空闲内存 MiB | 压测峰值 MiB | 每空闲连接 KiB | 启动秒 | 镜像 MB |",
      "|---|---|---|---|---|---|---|---|---|---|---|---|"]
for s in rows:
    md.append(f"| {s['name']} | {s['httpRps']:,} | {s['httpsRps']:,} | {(format(s['h2Rps'], ',') if s['h2Rps'] else '-')} | {s['tlsNewRps']:,} | "
              f"{s['fixedCpu']}% | {s['bigMBps']:,} | {s['idleMiB']} | {s['peakMiB']} | {s['perConnKiB']} | "
              f"{s['startupSec']} | {s['imageMB']} |")
open(os.path.join(out, "proxy.md"), "w").write("\n".join(md) + "\n")
json.dump(summary, open(os.path.join(out, "summary.json"), "w"), indent=1, ensure_ascii=False)
print("\n".join(md))
