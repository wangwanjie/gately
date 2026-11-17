#!/usr/bin/env python3
"""采样一个 cgroup（容器或 systemd 单元）的内存与 CPU。

用法：
  sample.py snapshot <cgroup路径>             输出一次 PSS/RSS（KiB）与累计 CPU 微秒
  sample.py watch <cgroup路径> <秒> <输出json>  每 0.5 秒采样，输出峰值 PSS 与 CPU 用量

内存口径：cgroup 内全部进程 /proc/<pid>/smaps_rollup 的 Pss 之和（共享页按进程平摊，
多进程程序如 nginx worker 不会被重复计算）；同时给出 Rss 之和与 cgroup memory.current 供参考。
CPU 口径：cgroup cpu.stat 的 usage_usec 差值。
"""
import json
import os
import sys
import time


def procs(cg):
    pids = []
    for root, _dirs, files in os.walk(cg):
        if "cgroup.procs" in files:
            with open(os.path.join(root, "cgroup.procs")) as f:
                pids += [int(x) for x in f.read().split()]
    return pids


def mem(cg):
    pss = rss = 0
    for pid in procs(cg):
        try:
            with open(f"/proc/{pid}/smaps_rollup") as f:
                for line in f:
                    if line.startswith("Pss:"):
                        pss += int(line.split()[1])
                    elif line.startswith("Rss:"):
                        rss += int(line.split()[1])
        except (FileNotFoundError, ProcessLookupError, PermissionError):
            continue
    cur = 0
    try:
        with open(os.path.join(cg, "memory.current")) as f:
            cur = int(f.read()) // 1024
    except FileNotFoundError:
        pass
    return pss, rss, cur


def cpu_usec(cg):
    with open(os.path.join(cg, "cpu.stat")) as f:
        for line in f:
            if line.startswith("usage_usec"):
                return int(line.split()[1])
    return 0


def snapshot(cg):
    pss, rss, cur = mem(cg)
    return {"pssKiB": pss, "rssKiB": rss, "cgroupKiB": cur, "cpuUsec": cpu_usec(cg),
            "procs": len(procs(cg))}


def watch(cg, seconds, out):
    start_cpu = cpu_usec(cg)
    t0 = time.time()
    peak_pss = peak_rss = 0
    samples = []
    while time.time() - t0 < seconds:
        pss, rss, _ = mem(cg)
        peak_pss = max(peak_pss, pss)
        peak_rss = max(peak_rss, rss)
        samples.append(pss)
        time.sleep(0.5)
    elapsed = time.time() - t0
    used = cpu_usec(cg) - start_cpu
    res = {"peakPssKiB": peak_pss, "peakRssKiB": peak_rss,
           "avgPssKiB": sum(samples) // max(len(samples), 1),
           "cpuSeconds": used / 1e6, "cpuPercent": used / 1e6 / elapsed * 100,
           "elapsed": elapsed}
    with open(out, "w") as f:
        json.dump(res, f)


if __name__ == "__main__":
    if sys.argv[1] == "snapshot":
        print(json.dumps(snapshot(sys.argv[2])))
    else:
        watch(sys.argv[2], float(sys.argv[3]), sys.argv[4])
