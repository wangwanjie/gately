#!/usr/bin/env python3
"""汇总 WAF 对比结果：读取 results/waf-<被测>.json，输出 Markdown 表格，并写 results/waf-summary.json。

用法：report.py [结果目录]

口径：
- 拦截 = 开启 WAF 后返回 403，或返回其他错误（4xx/5xx 中除源站常见的 404/405，以及连接被断开记为 0）
  且同一样本在关闭 WAF 时没有这样被拒（例如 ModSecurity 解析请求体失败时按 modsecurity.conf 的 200002 规则返回 400）。
  源站对样本本身返回 403 的样本从统计中排除。
- “代理本身拒绝”= 开启与关闭 WAF 时返回同一个错误码，是代理自己的协议解析拒绝，不算 WAF 的效果，单独列出。
- 攻击集真阳性率 = 拦截数 / 样本数；正常集误报率 = 拦截数 / 样本数。
- 性能下降比例 = 1 - 开启 WAF 的 rps / 关闭 WAF 的 rps（同一程序、同一压测项、各取 3 轮中位数）。
"""
import json
import os
import sys
from collections import Counter, defaultdict

RES = sys.argv[1] if len(sys.argv) > 1 else os.path.join(os.path.dirname(os.path.dirname(os.path.abspath(__file__))), "results")
WAF_DIR = os.path.dirname(os.path.abspath(__file__))
ORDER = ["gately", "modsec", "caddy-coraza"]
NAMES = {"gately": "Gately WAF", "modsec": "nginx + ModSecurity", "caddy-coraza": "Caddy + coraza-caddy"}
BACKEND_OK = {200, 404, 405}


def load_samples(sset):
    return [json.loads(l) for l in open(os.path.join(WAF_DIR, "samples", f"{sset}.jsonl"), encoding="utf-8") if l.strip()]


def pct(a, b):
    return f"{a / b * 100:.1f}%" if b else "-"


def is_block(on, off, sid):
    """开启 WAF 时被拒：403，或关闭 WAF 时没有出现的错误码（含连接被断开）。"""
    code = on.get(sid)
    if code is None:
        return False
    if code == 403:
        return True
    return code not in BACKEND_OK and off.get(sid) != code


def cpu(r):
    """CPU% 取按压测时长计算的值（没有时退回与 lib/run.py 相同的口径）。"""
    return r.get("sutCpuPercentLoad", r.get("sutCpuPercent", "-"))


def kib_mb(k):
    return f"{k / 1024:.1f}"


def main():
    data = {}
    for n in ORDER:
        p = os.path.join(RES, f"waf-{n}.json")
        if os.path.exists(p):
            data[n] = json.load(open(p))
    if not data:
        print("没有结果")
        return
    summary = {"perf": {}, "detect": {}}
    out = []
    w = out.append
    w("# WAF 对比结果\n")
    w("被测：" + "、".join(f"{NAMES[n]}（{data[n]['modes'].get('on', {}).get('image', '')}）" for n in data) + "\n")

    # 1. 性能：开启/关闭 WAF 对比
    tests = ["get", "query", "post4k", "attack"]
    w("## 1. 吞吐与延迟（wrk -t4 -c128，HTTP，3 轮中位数）\n")
    w("| 程序 | 压测项 | 关闭 WAF rps | 开启 WAF rps | 下降 | p50 关/开 ms | p99 关/开 ms | CPU% 关/开 | 每千请求 CPU ms 关/开 | 峰值 PSS MB 关/开 | 开启时 non2xx |")
    w("|---|---|---|---|---|---|---|---|---|---|---|")
    for n, d in data.items():
        for t in tests:
            off = d["modes"].get("off", {}).get("tests", {}).get(t, {}).get("median")
            on = d["modes"].get("on", {}).get("tests", {}).get(t, {}).get("median")
            if not off and not on:
                continue
            off = off or {}
            on = on or {}
            drop = (1 - on.get("rps", 0) / off["rps"]) if off.get("rps") else None
            summary["perf"].setdefault(n, {})[t] = {"off": off, "on": on, "drop": drop}
            w(f"| {NAMES[n]} | {t} | {off.get('rps', 0):.0f} | {on.get('rps', 0):.0f} | "
              f"{'-' if drop is None else f'{drop * 100:.1f}%'} | {off.get('p50ms', '-')}/{on.get('p50ms', '-')} | "
              f"{off.get('p99ms', '-')}/{on.get('p99ms', '-')} | {cpu(off)}/{cpu(on)} | "
              f"{off.get('cpuMsPerKReq', '-')}/{on.get('cpuMsPerKReq', '-')} | "
              f"{kib_mb(off.get('peakPssKiB', 0))}/{kib_mb(on.get('peakPssKiB', 0))} | {on.get('non2xx', '-')} |")
    w("")

    # 2. 启动与空闲内存
    w("## 2. 启动时间与空闲内存\n")
    w("| 程序 | 启动 s 关/开（中位数） | 规则加载耗时（差值）s | 空闲 PSS MB 关/开 | 规则占用内存（差值）MB | 回放后空闲 PSS MB（开） |")
    w("|---|---|---|---|---|---|")
    for n, d in data.items():
        off, on = d["modes"].get("off", {}), d["modes"].get("on", {})
        so, sn = off.get("startupSec"), on.get("startupSec")
        io, inn = off.get("idle", {}).get("pssKiB"), on.get("idle", {}).get("pssKiB")
        summary["perf"].setdefault(n, {})["startup"] = {"off": so, "on": sn, "offRuns": off.get("startupRuns"), "onRuns": on.get("startupRuns")}
        summary["perf"][n]["idlePssKiB"] = {"off": io, "on": inn, "onAfter": on.get("idleAfter", {}).get("pssKiB")}
        w(f"| {NAMES[n]} | {so if so is None else round(so, 3)}/{sn if sn is None else round(sn, 3)} | {'-' if so is None or sn is None else f'{sn - so:.2f}'} | "
          f"{kib_mb(io or 0)}/{kib_mb(inn or 0)} | {'-' if io is None or inn is None else kib_mb(inn - io)} | "
          f"{kib_mb(on.get('idleAfter', {}).get('pssKiB', 0))} |")
    w("")

    # 3. 检测效果
    backend = {}
    for sset in ("attack", "benign", "crs-regression"):
        p = os.path.join(RES, f"waf-backend-{sset}.json")
        backend[sset] = json.load(open(p)) if os.path.exists(p) else {}
    for sset, title in (("attack", "攻击样本（真阳性）"), ("benign", "正常样本（误报）"), ("crs-regression", "CRS 4.25.0 回归用例（PL1，期望命中的规则不一定足以阻断，只看三方一致性）")):
        rows = load_samples(sset)
        excl = {r["id"] for r in rows if backend[sset].get(r["id"]) == 403}
        rows = [r for r in rows if r["id"] not in excl]
        w(f"## 3. {title}\n")
        w(f"样本 {len(rows)} 条" + (f"（排除源站自身返回 403 的 {len(excl)} 条）" if excl else "") + "。\n")
        cats = sorted(Counter(r["cat"] for r in rows).items(), key=lambda x: -x[1])
        if sset == "crs-regression":
            cats = []
        hdr = "| 程序 | 拦截 | 其中非 403 的拦截 | 代理本身拒绝（不计入） | " + " | ".join(f"{c}({k})" for c, k in cats) + " |"
        w(hdr)
        w("|" + "---|" * (4 + len(cats)))
        for n, d in data.items():
            on = d["modes"].get("on", {}).get("replay", {}).get(sset, {})
            off = d["modes"].get("off", {}).get("replay", {}).get(sset, {})
            blocked = [r for r in rows if is_block(on, off, r["id"])]
            # 拦截中返回码不是 403 的（如 ModSecurity 的 400），单独计数
            other = [r for r in blocked if on.get(r["id"]) != 403]
            other_proxy = [r for r in rows if r["id"] in on and on.get(r["id"]) not in BACKEND_OK and on.get(r["id"]) != 403
                           and off.get(r["id"]) == on.get(r["id"])]
            by_cat = defaultdict(int)
            for r in blocked:
                by_cat[r["cat"]] += 1
            summary["detect"].setdefault(n, {})[sset] = {
                "total": len(rows), "blocked": len(blocked), "blockedNon403": len(other), "proxyReject": len(other_proxy),
                "byCat": {c: [by_cat[c], k] for c, k in Counter(r["cat"] for r in rows).items()},
                "otherCodes": dict(Counter(on.get(r["id"]) for r in other)),
                "offBlocked403": sum(1 for r in rows if off.get(r["id"]) == 403)}
            w(f"| {NAMES[n]} | {len(blocked)} ({pct(len(blocked), len(rows))}) | {len(other)} {dict(Counter(on.get(r['id']) for r in other)) or ''} | {len(other_proxy)} | "
              + " | ".join(pct(by_cat[c], k) for c, k in cats) + " |")
        w("")
        # 三方结论不一致的样本（只看是否 403），列出前若干条便于核对
        if len(data) > 1:
            diff = []
            for r in rows:
                v = tuple(is_block(data[n]["modes"].get("on", {}).get("replay", {}).get(sset, {}),
                               data[n]["modes"].get("off", {}).get("replay", {}).get(sset, {}), r["id"]) for n in data)
                if len(set(v)) > 1:
                    diff.append((r, v))
            pat = Counter(v for _, v in diff)
            summary["detect"].setdefault("_disagree", {})[sset] = {
                "count": len(diff), "patterns": {"/".join(f"{n}={'拦' if b else '放'}" for n, b in zip(data, k)): c for k, c in pat.items()},
                "ids": {"/".join(f"{n}={'拦' if b else '放'}" for n, b in zip(data, k)): [r["id"] for r, v in diff if v == k][:200] for k in pat}}
            label = lambda k: "/".join(f"{n}={'拦' if b else '放'}" for n, b in zip(data, k))
            w(f"三方结论不一致 {len(diff)} 条：" + "；".join(f"{label(k)} {c} 条" for k, c in pat.most_common()) + "\n")
            for r, v in diff[:15]:
                body = (r.get("body") or "")[:80].replace("\n", "\\n").replace("|", "\\|")
                w(f"- `{r['id']}` {r['cat']} {r['place']} `{r['method']} {r['path'][:100].replace('|', '%7C')}` {body!r} → "
                  + ", ".join(f"{n}:{data[n]['modes'].get('on', {}).get('replay', {}).get(sset, {}).get(r['id'])}" for n in data))
            w("")
    with open(os.path.join(RES, "waf-summary.json"), "w") as f:
        json.dump(summary, f, indent=1, ensure_ascii=False)
    print("\n".join(out))


if __name__ == "__main__":
    main()
