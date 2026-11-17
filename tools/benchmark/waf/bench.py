#!/usr/bin/env python3
"""WAF 专项对比：Gately WAF / nginx + ModSecurity v3 / Caddy + coraza-caddy，三方都用 OWASP CRS 4.25.0、PL1、拦截模式。

用法：bench.py <被测名> [轮数]        被测名为 suts/ 下的目录名：gately、modsec、caddy-coraza
每个被测程序依次以“关闭 WAF”“开启 WAF”两种配置运行，每种配置：
  1. 启动 3 次，记录从 docker run 到可服务（开启 WAF 时还要能拦截探测攻击）的时间，取中位数；
  2. 空闲 10 秒后采样内存（PSS）；
  3. 压测 get / query / post4k / attack 四项，每项 3 轮取中位数（按 rps 排序取中间一轮），并行采样 CPU 与峰值内存；
  4. 逐条回放攻击、正常、CRS 回归三组样本，记录状态码（结果是确定的，只跑一轮）。
结果写到 results/waf-<被测名>.json，汇总由 report.py 生成。

环境变量（压测机上全部用默认值；本机 colima 调试时改端口、CPU 与目录）：
  HP/SP 被测 HTTP/HTTPS 端口（8080/8443）  BACKEND 源站（127.0.0.1:9000）  CERTS 证书目录（/etc/bench/certs）
  SUT_NAME 容器名（sut）  SUT_CPUS（0,1）  LOAD_CPUS（4-7）  WORKDIR 生成配置的目录（/tmp）
  DUR 每轮压测秒数（20）  ADMIN_PORT Gately 管理端口（空为默认 16680）  MODES 要跑的模式（off,on）
  TESTS 要跑的压测项（get,query,post4k,attack）  STARTS 启动计时次数（3）  RESULTS 结果目录（默认 ../results）
"""
import json
import os
import re
import statistics
import subprocess
import sys
import time

WAF_DIR = os.path.dirname(os.path.abspath(__file__))
BASE = os.path.dirname(WAF_DIR)
LIB = os.path.join(BASE, "lib")
SAMPLES = os.path.join(WAF_DIR, "samples")
ENV = {
    "HP": "8080", "SP": "8443", "BACKEND": "127.0.0.1:9000", "CERTS": "/etc/bench/certs", "SUT_NAME": "sut",
    "SUT_CPUS": "0,1", "LOAD_CPUS": "4-7", "WORKDIR": "/tmp", "DUR": "20", "ADMIN_PORT": "",
    "MODES": "off,on", "TESTS": "get,query,post4k,attack", "STARTS": "3", "RESULTS": "",
}
for k in ENV:
    ENV[k] = os.environ.get(k, ENV[k])
HP, SP = ENV["HP"], ENV["SP"]
DUR = int(ENV["DUR"])
LOAD = ENV["LOAD_CPUS"]
SUT = ENV["SUT_NAME"]
# 开启 WAF 时用来确认规则已生效的探测请求（三方都会以 403 拦截）
PROBE = "/?q=%3Cscript%3Ealert(1)%3C%2Fscript%3E"


def sh(cmd, check=False, timeout=None, env=None):
    p = subprocess.run(cmd, shell=True, capture_output=True, text=True, timeout=timeout, env=env)
    if check and p.returncode != 0:
        raise RuntimeError(f"{cmd}\n{p.stdout}\n{p.stderr}")
    return p.stdout + p.stderr


def cgroup_of(name):
    """容器 cgroup：压测机为 systemd 驱动（system.slice/docker-<ID>.scope），colima 为 cgroupfs 驱动（docker/<ID>）。"""
    cid = sh(f"docker inspect -f '{{{{.Id}}}}' {name}").strip()
    for path in (f"/sys/fs/cgroup/system.slice/docker-{cid}.scope", f"/sys/fs/cgroup/docker/{cid}"):
        if os.path.isdir(path):
            return path
    raise RuntimeError("找不到容器 cgroup: " + cid)


def snapshot(cg):
    return json.loads(sh(f"python3 {LIB}/sample.py snapshot {cg}"))


def with_watch(cg, seconds, cmd):
    """压测命令运行期间并行采样被测程序的内存峰值与 CPU（与 lib/run.py 相同）。"""
    out = os.path.join(ENV["WORKDIR"], "waf-watch.json")
    w = subprocess.Popen(["python3", f"{LIB}/sample.py", "watch", cg, str(seconds), out])
    text = sh(cmd, timeout=seconds + 120)
    w.wait()
    with open(out) as f:
        return text, json.load(f)


def parse_wrk(t):
    """解析 wrk --latency 输出（与 lib/run.py 口径一致）。"""
    def num(v, unit):
        return float(v) * {"us": 0.001, "ms": 1, "s": 1000}.get(unit, 1)
    r = {}
    m = re.search(r"Requests/sec:\s+([\d.]+)", t)
    r["rps"] = float(m.group(1)) if m else 0
    for p in ("50", "99"):
        m = re.search(rf"\s{p}(?:\.000)?%\s+([\d.]+)(us|ms|s)", t)
        if m:
            r[f"p{p}ms"] = round(num(m.group(1), m.group(2)), 3)
    m = re.search(r"Non-2xx or 3xx responses:\s+(\d+)", t)
    r["non2xx"] = int(m.group(1)) if m else 0
    m = re.search(r"Socket errors: connect (\d+), read (\d+), write (\d+), timeout (\d+)", t)
    r["sockErrors"] = sum(int(x) for x in m.groups()) if m else 0
    m = re.search(r"(\d+) requests in", t)
    r["requests"] = int(m.group(1)) if m else 0
    return r


def lua_str(b):
    """把字节串写成 Lua 字符串字面量（可打印 ASCII 原样，其余用 \\ddd 转义）。"""
    out = []
    for c in b:
        if 32 <= c < 127 and c not in (34, 92):
            out.append(chr(c))
        else:
            # 必须写满三位：\\ddd 后面紧跟数字时，不足三位会与后续数字连成另一个转义
            out.append("\\%03d" % c)
    return '"' + "".join(out) + '"'




def make_lua(path, reqs):
    """生成 wrk 脚本：reqs 为 [(方法, 路径, [[头, 值]], 请求体字节或 None)]，各线程从不同位置开始轮流发送。
    请求在 init 中预先格式化，不注册 response 回调，尽量减少 wrk 自身的 Lua 开销。"""
    lines = ["local data = {"]
    for m, p, hdr, body in reqs:
        h = ", ".join(f"[{lua_str(k.encode())}] = {lua_str(v.encode())}" for k, v in hdr)
        b = lua_str(body) if body is not None else "nil"
        lines.append(f"  {{{lua_str(m.encode())}, {lua_str(p.encode())}, {{{h}}}, {b}}},")
    lines.append("}")
    lines.append(r'''
local counter = 0
function setup(thread)
  thread:set("tid", counter)
  counter = counter + 1
end
local reqs = {}
local idx = 0
function init(args)
  for i, r in ipairs(data) do
    reqs[i] = wrk.format(r[1], r[2], r[3], r[4])
  end
  -- 各线程起点错开，避免所有连接同时发送同一条请求
  idx = (tid * 997) % #reqs
end
function request()
  idx = idx % #reqs + 1
  return reqs[idx]
end
''')
    with open(path, "w") as f:
        f.write("\n".join(lines))


def sample_req(s):
    import base64
    if "bodyB64" in s:
        body = base64.b64decode(s["bodyB64"])
    elif "body" in s:
        body = s["body"].encode()
    else:
        body = None
    hdr = [[k, v] for k, v in s["headers"] if k.lower() not in ("host", "content-length", "connection")]
    return (s["method"], s["path"], hdr, body)


def post4k_body():
    """约 4 KB 的普通表单：40 个字段，内容为常见的姓名、地址、留言等正常文本。"""
    words = ("hello world this is a normal comment about our weekend trip to the mountains with friends "
             "we took many photos and enjoyed the local food very much").split()
    fields = []
    i = 0
    while sum(len(f) + 1 for f in fields) < 4096:
        text = "+".join(words[(i * 3 + j) % len(words)] for j in range(12))
        fields.append(f"field{i}={text}")
        i += 1
    body = "&".join(fields)
    return body[:4096].rsplit("&", 1)[0] if len(body) > 4096 else body


def build_scripts():
    wd = ENV["WORKDIR"]
    std_hdr = [["User-Agent", "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/130.0.0.0 Safari/537.36"],
               ["Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8"],
               ["Accept-Language", "zh-CN,zh;q=0.9,en;q=0.8"]]
    q = "/?id=12345&name=alice&q=hello+world&page=2&sort=desc&lang=zh-CN&ts=1763386400&ref=home"
    body = post4k_body()
    make_lua(f"{wd}/waf-get.lua", [("GET", "/", std_hdr, None)])
    make_lua(f"{wd}/waf-query.lua", [("GET", q, std_hdr, None)])
    make_lua(f"{wd}/waf-post4k.lua", [("POST", "/", std_hdr + [["Content-Type", "application/x-www-form-urlencoded"]], body.encode())])
    attacks = [sample_req(json.loads(l)) for l in open(os.path.join(SAMPLES, "attack.jsonl"), encoding="utf-8") if l.strip()]
    make_lua(f"{wd}/waf-attack.lua", attacks)
    url = f"http://bench.test:{HP}"
    # 所有压测项都用 wrk -t4 -c128，经 Lua 脚本发送带浏览器常见请求头的请求（状态码由 replay 逐条核对，这里只看 non2xx）
    return {name: f"taskset -c {LOAD} wrk -t4 -c128 -d{DUR}s --latency --timeout 10s -s {wd}/waf-{name}.lua {url}/"
            for name in ("get", "query", "post4k", "attack")}, {"queryPath": q, "post4kBytes": len(body), "attackRequests": len(attacks)}


def curl_code(path):
    return sh(f"curl -s -o /dev/null -w '%{{http_code}} %{{size_download}}' --max-time 2 'http://bench.test:{HP}{path}'").strip()


def wait_ready(t0, waf_on, timeout=180):
    """就绪条件：GET / 返回源站的 1024 字节；开启 WAF 时探测攻击还要返回 403（规则已加载生效）。"""
    while time.time() - t0 < timeout:
        if curl_code("/") == "200 1024":
            if not waf_on or curl_code(PROBE).startswith("403"):
                return time.time() - t0
        time.sleep(0.05)
    raise RuntimeError("被测程序未就绪: " + sh(f"docker logs --tail 30 {SUT}"))


def sut_env(name, mode):
    e = dict(os.environ)
    e.update(ENV)
    e["WAF"] = mode
    return e


def start(name, mode):
    """删除旧容器、清页缓存后启动，返回启动耗时（秒）。"""
    sut_dir = os.path.join(WAF_DIR, "suts", name)
    env = sut_env(name, mode)
    sh(f"docker rm -f {SUT}")
    sh("sync; echo 3 > /proc/sys/vm/drop_caches 2>/dev/null")
    t0 = time.time()
    sh(f"bash {sut_dir}/start.sh", check=True, env=env)
    return wait_ready(t0, mode == "on")


def run_mode(name, mode, rounds, cmds):
    sut_dir = os.path.join(WAF_DIR, "suts", name)
    env = sut_env(name, mode)
    sh(f"bash {sut_dir}/prepare.sh", check=True, env=env)
    starts = []
    for _ in range(int(ENV["STARTS"])):
        starts.append(round(start(name, mode), 3))
        time.sleep(1)
    res = {"mode": mode, "startupSec": statistics.median(starts), "startupRuns": starts}
    res["httpsCheck"] = sh(f"curl -sk -o /dev/null -w '%{{http_code}} %{{size_download}} %{{http_version}}' https://bench.test:{SP}/").strip()
    res["probe"] = curl_code(PROBE)
    cg = cgroup_of(SUT)
    time.sleep(10)
    res["idle"] = snapshot(cg)
    res["info"] = sh(f"bash {sut_dir}/info.sh", env=env).strip()
    res["tests"] = {}
    for tname in ENV["TESTS"].split(","):
        runs = []
        for i in range(rounds):
            text, w = with_watch(cg, DUR + 2, cmds[tname])
            # wrk 的 Lua 脚本出错时只打印错误、改发默认的 GET /，这里直接中止，避免得到错误的数据
            if ".lua:" in text or "PANIC" in text:
                raise RuntimeError("wrk 脚本出错: " + text[:500])
            r = parse_wrk(text)
            # sutCpuPercent 与 lib/run.py 口径相同（CPU 秒 / 采样时长 DUR+2 秒）；sutCpuPercentLoad 按压测时长 DUR 计算，更接近满载时的实际占用
            r.update({"sutCpuPercent": round(w["cpuPercent"], 1), "sutCpuPercentLoad": round(w["cpuSeconds"] / DUR * 100, 1),
                      "peakPssKiB": w["peakPssKiB"],
                      "cpuMsPerKReq": round(w["cpuSeconds"] * 1e6 / max(r["requests"], 1), 2)})
            runs.append(r)
            print(name, mode, tname, i, json.dumps(r), flush=True)
            time.sleep(3)
        med = sorted(runs, key=lambda x: x.get("rps", 0))[len(runs) // 2]
        res["tests"][tname] = {"median": med, "runs": runs}
    # 逐条回放三组样本（并发 8，每条单独建连）
    res["replay"] = {}
    for sset in ("attack", "benign", "crs-regression"):
        out = os.path.join(ENV["WORKDIR"], f"waf-replay-{name}-{mode}-{sset}.json")
        print(sh(f"taskset -c {LOAD} python3 {WAF_DIR}/tools/replay.py {SAMPLES}/{sset}.jsonl 127.0.0.1 {HP} {out} 8").strip(), flush=True)
        res["replay"][sset] = json.load(open(out))
    time.sleep(5)
    res["idleAfter"] = snapshot(cg)
    res["image"] = sh(f"docker inspect -f '{{{{.Config.Image}}}}' {SUT}").strip()
    # 检查规则命中日志的数量级，确认三方都在记录（只取行数）
    res["logTail"] = sh(f"docker logs --tail 5 {SUT} 2>&1 | cut -c1-300").strip()
    sh(f"docker rm -f {SUT}")
    return res


def main():
    name = sys.argv[1]
    rounds = int(sys.argv[2]) if len(sys.argv) > 2 else 3
    cmds, meta = build_scripts()
    res = {"sut": name, "env": {k: ENV[k] for k in ("HP", "SP", "BACKEND", "SUT_CPUS", "LOAD_CPUS", "DUR")},
           "rounds": rounds, "workload": meta, "modes": {}}
    for mode in ENV["MODES"].split(","):
        res["modes"][mode] = run_mode(name, mode, rounds, cmds)
        print("DONE", name, mode, json.dumps({"startup": res["modes"][mode]["startupSec"], "idle": res["modes"][mode]["idle"]}), flush=True)
    rdir = ENV["RESULTS"] or os.path.join(BASE, "results")
    os.makedirs(rdir, exist_ok=True)
    with open(os.path.join(rdir, f"waf-{name}.json"), "w") as f:
        json.dump(res, f, indent=1, ensure_ascii=False)


if __name__ == "__main__":
    main()
