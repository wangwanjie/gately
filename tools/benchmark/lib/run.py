#!/usr/bin/env python3
"""反向代理性能对比主脚本（在压测虚拟机上以 root 运行）。

用法：run.py <被测名> [轮数]
被测程序由 suts/<名>/start.sh 启动，统一为：容器名 sut、host 网络、绑定 vCPU 0,1，
HTTP 8080 与 HTTPS 8443，域名 bench.test 反代到源站 127.0.0.1:9000。
压测工具固定在 vCPU 4-7，源站在 vCPU 2,3。结果写到 results/<名>.json。
"""
import json
import os
import re
import statistics
import subprocess
import sys
import time

BASE = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
LIB = os.path.join(BASE, "lib")
LOAD_CPUS = "4-7"
DUR = 20


def sh(cmd, check=False, timeout=None):
    p = subprocess.run(cmd, shell=True, capture_output=True, text=True, timeout=timeout)
    if check and p.returncode != 0:
        raise RuntimeError(f"{cmd}\n{p.stdout}\n{p.stderr}")
    return p.stdout + p.stderr


def cgroup_of(name):
    cid = sh(f"docker inspect -f '{{{{.Id}}}}' {name}").strip()
    path = f"/sys/fs/cgroup/system.slice/docker-{cid}.scope"
    if not os.path.isdir(path):
        raise RuntimeError("找不到容器 cgroup: " + path)
    return path


def snapshot(cg):
    return json.loads(sh(f"python3 {LIB}/sample.py snapshot {cg}"))


def with_watch(cg, seconds, cmd):
    """压测命令运行期间并行采样被测程序的内存峰值与 CPU。"""
    out = "/tmp/watch.json"
    w = subprocess.Popen(["python3", f"{LIB}/sample.py", "watch", cg, str(seconds), out])
    text = sh(cmd, timeout=seconds + 120)
    w.wait()
    with open(out) as f:
        return text, json.load(f)


def parse_wrk(t):
    def num(v, unit):
        v = float(v)
        return v * {"us": 0.001, "ms": 1, "s": 1000}.get(unit, 1)
    r = {}
    m = re.search(r"Requests/sec:\s+([\d.]+)", t)
    r["rps"] = float(m.group(1)) if m else 0
    m = re.search(r"Transfer/sec:\s+([\d.]+)(\w+)", t)
    if m:
        mult = {"B": 1 / 1048576, "KB": 1 / 1024, "MB": 1, "GB": 1024}[m.group(2)]
        r["MBps"] = float(m.group(1)) * mult
    for p in ("50", "99"):
        m = re.search(rf"\s{p}%\s+([\d.]+)(us|ms|s)", t)
        if m:
            r[f"p{p}ms"] = num(m.group(1), m.group(2))
    m = re.search(r"Non-2xx or 3xx responses:\s+(\d+)", t)
    r["non2xx"] = int(m.group(1)) if m else 0
    m = re.search(r"Socket errors: connect (\d+), read (\d+), write (\d+), timeout (\d+)", t)
    r["sockErrors"] = sum(int(x) for x in m.groups()) if m else 0
    return r


def parse_oha(t):
    j = json.loads(t[t.index("{"):])
    s = j["summary"]
    lp = j.get("latencyPercentiles", {})
    codes = j.get("statusCodeDistribution", {})
    errs = sum(j.get("errorDistribution", {}).values())
    return {"rps": s.get("requestsPerSec", 0), "p50ms": (lp.get("p50") or 0) * 1000,
            "p99ms": (lp.get("p99") or 0) * 1000, "ok": codes.get("200", 0),
            "non2xx": sum(v for k, v in codes.items() if not k.startswith("2")), "errors": errs}


def parse_h2load(t):
    r = {}
    m = re.search(r"finished in [\d.]+\w+, ([\d.]+) req/s", t)
    r["rps"] = float(m.group(1)) if m else 0
    m = re.search(r"requests: \d+ total, \d+ started, \d+ done, (\d+) succeeded, (\d+) failed, (\d+) errored", t)
    if m:
        r["ok"], r["failed"], r["errored"] = map(int, m.groups())
    m = re.search(r"Application protocol: (\S+)", t)
    r["proto"] = m.group(1) if m else ""
    return r


def build_tests(hp, sp):
    """hp/sp 为被测程序的 HTTP/HTTPS 端口（默认 8080/8443，NPM 只能用 80/443）。"""
    h, t = f"http://bench.test:{hp}", f"https://bench.test:{sp}"
    return {
        # 名称: (命令, 解析函数)
        "http_ka": (f"taskset -c {LOAD_CPUS} wrk -t4 -c128 -d{DUR}s --latency {h}/", parse_wrk),
        "https_ka": (f"taskset -c {LOAD_CPUS} wrk -t4 -c128 -d{DUR}s --latency {t}/", parse_wrk),
        "https_new": (f"taskset -c {LOAD_CPUS} oha --no-tui --insecure --disable-keepalive -c 64 -z {DUR}s --output-format json {t}/", parse_oha),
        "h2": (f"taskset -c {LOAD_CPUS} h2load -t4 -c64 -m10 -D {DUR} {t}/", parse_h2load),
        "fixed5k": (f"taskset -c {LOAD_CPUS} oha --no-tui -q 5000 -c 50 -z {DUR}s --output-format json {h}/", parse_oha),
        "big": (f"taskset -c {LOAD_CPUS} wrk -t2 -c4 -d{DUR}s {h}/100m.bin", parse_wrk),
    }


def wait_ready(t0, hp, timeout=180):
    while time.time() - t0 < timeout:
        out = sh(f"curl -s -o /dev/null -w '%{{http_code}} %{{size_download}}' --max-time 2 http://bench.test:{hp}/")
        if out.strip() == "200 1024":
            return time.time() - t0
        time.sleep(0.05)
    raise RuntimeError("被测程序未就绪")


def measure_startup(hp, n=3):
    """配置完成后重启容器，计时到第一个正确响应；取 n 次中位数。"""
    vals = []
    for _ in range(n):
        sh("docker stop -t 10 sut")
        sh("sync; echo 3 > /proc/sys/vm/drop_caches")
        t0 = time.time()
        sh("docker start sut", check=True)
        vals.append(wait_ready(t0, hp))
        time.sleep(3)
    return round(statistics.median(vals), 3)


def main():
    name = sys.argv[1]
    rounds = int(sys.argv[2]) if len(sys.argv) > 2 else 3
    only = sys.argv[3].split(",") if len(sys.argv) > 3 else None
    start = os.path.join(BASE, "suts", name, "start.sh")
    hp, sp = 8080, 8443
    pf = os.path.join(BASE, "suts", name, "ports")
    if os.path.exists(pf):
        hp, sp = map(int, open(pf).read().split())
    tests = build_tests(hp, sp)
    sh("docker rm -f sut", check=False)
    sh("sync; echo 3 > /proc/sys/vm/drop_caches")
    # 首次启动含生成配置、调用管理接口等准备步骤，不计时；启动时间在最后用 docker restart 统一测量
    t0 = time.time()
    sh(f"bash {start}", check=True)
    wait_ready(t0, hp)
    # HTTPS 也要可用且证书正确
    chk = sh(f"curl -sk -o /dev/null -w '%{{http_code}} %{{size_download}} %{{http_version}}' https://bench.test:{sp}/")
    cg = cgroup_of("sut")
    time.sleep(10)
    res = {"sut": name, "httpsCheck": chk.strip(),
           "idle": snapshot(cg), "tests": {}}
    sut_extra = os.path.join(BASE, "suts", name, "info.sh")
    if os.path.exists(sut_extra):
        res["info"] = sh(f"bash {sut_extra}").strip()
    for tname, (cmd, parse) in tests.items():
        if only and tname not in only:
            continue
        runs = []
        for i in range(rounds):
            text, w = with_watch(cg, DUR + 2, cmd)
            r = parse(text)
            r.update({"sutCpuPercent": round(w["cpuPercent"], 1), "peakPssKiB": w["peakPssKiB"]})
            runs.append(r)
            print(name, tname, i, json.dumps(r), flush=True)
            time.sleep(3)
        med = sorted(runs, key=lambda x: x.get("rps", 0))[len(runs) // 2]
        res["tests"][tname] = {"median": med, "runs": runs}
    if not only or "conn10k" in only:
        # 1 万条空闲 keep-alive 连接：测每条连接的内存
        before = snapshot(cg)["pssKiB"]
        p = subprocess.Popen(f"taskset -c {LOAD_CPUS} python3 {LIB}/hold.py 127.0.0.1 {hp} 10000 20",
                             shell=True, stdout=subprocess.PIPE, text=True)
        okn = int(p.stdout.readline().strip() or 0)
        time.sleep(5)
        during = snapshot(cg)["pssKiB"]
        p.wait()
        res["tests"]["conn10k"] = {"median": {"ok": okn, "pssBeforeKiB": before, "pssDuringKiB": during,
                                              "perConnKiB": round((during - before) / max(okn, 1), 2)}}
        print(name, "conn10k", json.dumps(res["tests"]["conn10k"]), flush=True)
    time.sleep(10)
    res["idleAfter"] = snapshot(cg)
    res["startupSec"] = measure_startup(hp)
    res["image"] = sh("docker inspect -f '{{.Config.Image}}' sut").strip()
    res["imageBytes"] = int(sh(f"docker image inspect -f '{{{{.Size}}}}' {res['image']}").strip() or 0)
    os.makedirs(os.path.join(BASE, "results"), exist_ok=True)
    suffix = "" if not only else "-" + "-".join(only)
    with open(os.path.join(BASE, "results", f"{name}{suffix}.json"), "w") as f:
        json.dump(res, f, indent=1, ensure_ascii=False)
    print("DONE", name, json.dumps({"startup": res["startupSec"], "idle": res["idle"]}))


if __name__ == "__main__":
    main()
