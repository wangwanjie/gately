#!/usr/bin/env python3
"""汇总入侵防护专项结果：report.py <results 目录>，读取 guard-<被测>.json，输出对照表（纯文本）。"""
import json
import os
import sys

d = sys.argv[1]


def load(name):
    p = os.path.join(d, f"guard-{name}.json")
    if not os.path.exists(p):
        return None
    with open(p) as f:
        return json.load(f)


def mib(kib):
    return "-" if kib is None else f"{kib / 1024:.1f} MiB"


def ms(st, k):
    if not st or st.get("n", 0) == 0:
        return "-"
    return f"{st[k]:.0f}"


g, f, gw, gf = load("gately"), load("fail2ban"), load("gately-web"), load("gately-full")
rows = []
print("== 入侵防护：Gately vs fail2ban ==")
for r in (g, f, gw, gf):
    if r:
        print(f"{r['sut']}: {r['info'].get('version')}")
print()

print("-- 安装体积 --")
if g:
    print(f"Gately 程序 {g['info']['binaryBytes'] / 1048576:.1f} MiB（单文件，含全部模块），镜像 {g['info']['imageBytes'] / 1048576:.1f} MiB")
if f:
    ins = f["info"]["install"]
    print(f"fail2ban 包 {ins['fail2banKiB'] / 1024:.1f} MiB；fail2ban + Python 运行时 {ins['pythonKiB'] / 1024:.1f} MiB；"
          f"依赖闭包中的其余共享库 {ins['sharedLibsKiB'] / 1024:.1f} MiB（多数系统已装）")
print()

print("-- 空闲（就绪后等待，PSS） --")
for r in (g, f, gw, gf):
    if r and "idle" in r["tests"]:
        t = r["tests"]["idle"]
        print(f"{r['sut']:<12} PSS {mib(t['idlePssKiB'])}  空闲 CPU {t['idleCpuPercent']}%  就绪 {t['readySec']} 秒  cpuset {t['cpuset']}")
print()

print("-- 封禁延迟（达到阈值 → 进入 nftables 集合，毫秒）--")
print(f"{'被测':<12} {'n':>4} {'p50':>7} {'p90':>7} {'p99':>7} {'max':>7}  检测p50 同步p50 漏封")
for r in (g, f, gw):
    if r and "latency" in r["tests"]:
        t = r["tests"]["latency"]
        print(f"{r['sut']:<12} {t['n']:>4} {ms(t, 'p50ms'):>7} {ms(t, 'p90ms'):>7} {ms(t, 'p99ms'):>7} {ms(t, 'maxms'):>7}"
              f"  {ms(t.get('detect'), 'p50ms'):>6} {ms(t.get('sync'), 'p50ms'):>6} {t['missed']:>4}")
print()


def stream_line(name, t):
    lat = t["latency"]
    inj = t.get("inject", {})
    tot = t.get("total", {})
    return (f"{name:<12} {t['achievedRate']:>6} {t['expected']:>6} {t['banned']:>6} {t['missed']:>5} {t['falseBans']:>4} "
            f"{ms(lat, 'p50ms'):>7} {ms(lat, 'p99ms'):>7} {ms(lat, 'maxms'):>7} "
            f"{str(inj.get('cpuPercent', '-')):>6} {mib(tot.get('peakPssKiB')):>10} "
            f"{str(t.get('lastBanAfterInjectEndSec')):>7} {str(t.get('cpuBusyAfterInjectEndSec')):>7}")


hdr = (f"{'被测':<12} {'行/秒':>6} {'应封':>6} {'已封':>6} {'漏封':>5} {'误封':>4} {'p50ms':>7} {'p99ms':>7} {'maxms':>7} "
       f"{'CPU%':>6} {'峰值PSS':>10} {'末封s':>7} {'忙到s':>7}")
print("-- 日志吞吐（CPU% 为注入期间平均，100% = 1 个 vCPU；末封s = 注入结束到最后一个封禁；忙到s = 注入结束后 CPU 回落所需秒数）--")
print(hdr)
for r in (g, f):
    if r and "stream" in r["tests"]:
        for rate, t in r["tests"]["stream"].items():
            print(stream_line(r["sut"], t))
print()
print("-- 大规模攻击者（每个 IP 写 5 行 404，全部为恶意行）--")
for r in (g, f):
    if r and "big" in r["tests"]:
        t = r["tests"]["big"]
        print(f"{r['sut']:<12} 攻击者 {t['expected']}，注入 {t['injectSec']} 秒，已封 {t['banned']}，漏封 {t['missed']}，"
              f"全部封完 {t.get('allBannedSec')} 秒（从注入开始），注入结束后 {t.get('lastBanAfterInjectEndSec')} 秒封完，"
              f"延迟 p50/p99 {ms(t['latency'], 'p50ms')}/{ms(t['latency'], 'p99ms')} ms，同步段 p50/p99 "
              f"{ms(t.get('sync'), 'p50ms')}/{ms(t.get('sync'), 'p99ms')} ms，CPU {t['total'].get('cpuSeconds')} 秒，"
              f"峰值 PSS {mib(t['total'].get('peakPssKiB'))}，列集合耗时 {t.get('nftListSetSec')} 秒")
