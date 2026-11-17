#!/usr/bin/env python3
"""认证网关专项对比（在压测虚拟机上以 root 运行，入口 run.sh）。

用法：bench.py [被测名,...]        不带参数时依次测全部被测
      bench.py summary             汇总 results/auth-*.json 输出 Markdown 表格

被测（每个都是 HTTPS，bench.test 反代到源站，详见 README.md）：
  gately          Gately 站点网页认证 + 共享用户（会话 Cookie）、基本认证
  nginx-authelia  nginx auth_request + Authelia；同一 nginx 另有 auth_basic（bcrypt / apr1）
  caddy-authelia  Caddy forward_auth + Authelia；同一 Caddy 另有 basic_auth（bcrypt）
  lucky           Lucky 3.8.1 站点基本认证（网页认证见 README 中的说明）

每项测试 ROUNDS 轮取中位数（按 rps 排序），被测各容器 CPU 求和、PSS 求和后取峰值。
可用环境变量覆盖的项目见下方常量（本机 colima 试跑由 colima.sh 设置）。
"""
import base64
import http.client
import importlib.util
import json
import os
import secrets
import shlex
import ssl
import subprocess
import sys
import threading
import time
import urllib.parse

HERE = os.path.dirname(os.path.abspath(__file__))
BASE = os.path.dirname(HERE)
LIB = os.path.join(BASE, "lib")


def _load(name, path):
    spec = importlib.util.spec_from_file_location(name, path)
    mod = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(mod)
    return mod


# 复用通用压测脚本的解析函数与 cgroup 采样函数，口径保持一致
RUN = _load("bench_run", os.path.join(LIB, "run.py"))
SAMPLE = _load("bench_sample", os.path.join(LIB, "sample.py"))
# Lucky 网页认证的挑战式登录客户端（需要 python3-cryptography，与 suts/lucky/setup.py 相同）
LOGINLOAD = _load("bench_loginload", os.path.join(HERE, "loginload.py"))

PREFIX = os.environ.get("NAME_PREFIX", "")            # 容器名前缀（本机调试为 bx-）
WORK = os.environ.get("AUTH_WORK", "/tmp/bench-auth")  # 被测配置目录（需与 Docker 守护进程看到的路径一致）
CERTS = os.environ.get("CERT_DIR", "/etc/bench/certs")
BACKEND = os.environ.get("BACKEND", "127.0.0.1:9000")
SUT_CPUS = os.environ.get("SUT_CPUS", "0,1")
LOAD_CPUS = os.environ.get("LOAD_CPUS", "4-7")
DUR = int(os.environ.get("DUR", "20"))
ROUNDS = int(os.environ.get("ROUNDS", "3"))
RESULTS = os.environ.get("RESULTS", os.path.join(BASE, "results"))
GATELY_ADMIN = os.environ.get("GATELY_ADMIN", "127.0.0.1:16680")
LUCKY_ADMIN = os.environ.get("LUCKY_ADMIN", "16601")

IMG = {
    "gately": os.environ.get("GATELY_IMAGE", "ghcr.io/wangwanjie/gately:v0.6.8.1"),
    "nginx": "nginx:1.29-alpine",
    "caddy": "caddy:2-alpine",
    "authelia": "authelia/authelia:4.39.28",
    "lucky": os.environ.get("LUCKY_IMAGE", "gdy666/lucky:v3.8.1_lucky"),
}

USER = "bench"
PASS = "Bench-Pass-2026"
WRONG = "Wrong-Pass-0000"
SUT = PREFIX + "auth-sut"
AUTHELIA = PREFIX + "auth-authelia"
# 登录吞吐的并发：8 个（Gately 同一来源进行中的校验 + 失败次数达到 10 就拒绝，见 README）；另测一次 32 并发
LOGIN_C = int(os.environ.get("LOGIN_C", "8"))
LOGIN_C_HIGH = int(os.environ.get("LOGIN_C_HIGH", "32"))
BAD_DUR = int(os.environ.get("BAD_DUR", "10"))


def sh(cmd, check=False, timeout=None):
    return RUN.sh(cmd, check=check, timeout=timeout)


def log(*a):
    print(time.strftime("%H:%M:%S"), *a, flush=True)


# ---------------------------------------------------------------- 采样


def cgroup_of(name):
    """容器的 cgroup 目录：systemd 驱动（压测机）或 cgroupfs 驱动（colima）。"""
    cid = sh(f"docker inspect -f '{{{{.Id}}}}' {name}").strip()
    for p in (f"/sys/fs/cgroup/system.slice/docker-{cid}.scope", f"/sys/fs/cgroup/docker/{cid}"):
        if os.path.isdir(p):
            return p
    raise RuntimeError("找不到容器 cgroup: " + name)


def snapshot_all(cgs):
    """各容器的快照与合计。"""
    per = {n: SAMPLE.snapshot(cg) for n, cg in cgs.items()}
    tot = {k: sum(v[k] for v in per.values()) for k in ("pssKiB", "rssKiB", "cgroupKiB", "procs")}
    tot["containers"] = per
    return tot


class Watch:
    """压测期间每 0.5 秒采样被测全部容器：PSS 合计的峰值，CPU 为各 cgroup usage_usec 差值之和。"""

    def __init__(self, cgs):
        self.cgs = cgs
        self.stop = threading.Event()
        self.peak = 0
        self.peak_per = {n: 0 for n in cgs}
        self.t = threading.Thread(target=self.loop, daemon=True)

    def loop(self):
        while not self.stop.is_set():
            tot = 0
            for n, cg in self.cgs.items():
                pss, _rss, _cur = SAMPLE.mem(cg)
                self.peak_per[n] = max(self.peak_per[n], pss)
                tot += pss
            self.peak = max(self.peak, tot)
            self.stop.wait(0.5)

    def __enter__(self):
        self.cpu0 = {n: SAMPLE.cpu_usec(cg) for n, cg in self.cgs.items()}
        self.t0 = time.time()
        self.t.start()
        return self

    def __exit__(self, *exc):
        self.stop.set()
        self.t.join()
        el = time.time() - self.t0
        used = {n: SAMPLE.cpu_usec(cg) - self.cpu0[n] for n, cg in self.cgs.items()}
        self.result = {
            "sutCpuPercent": round(sum(used.values()) / 1e6 / el * 100, 1),
            "cpuPercentBy": {n: round(u / 1e6 / el * 100, 1) for n, u in used.items()},
            "peakPssKiB": self.peak,
            "peakPssBy": self.peak_per,
        }


# ---------------------------------------------------------------- HTTP 工具


CTX = ssl.create_default_context()
CTX.check_hostname = False
CTX.verify_mode = ssl.CERT_NONE


def request(host, port, method, path, body=None, headers=None):
    """发一个 HTTPS 请求（不跟随跳转），返回 (状态码, 响应头列表, 响应体)。"""
    c = http.client.HTTPSConnection(host, port, context=CTX, timeout=30)
    try:
        c.request(method, path, body=body, headers=headers or {})
        r = c.getresponse()
        return r.status, r.getheaders(), r.read()
    finally:
        c.close()


def set_cookies(hdrs):
    """取出 Set-Cookie 的 名=值 部分。"""
    out = []
    for k, v in hdrs:
        if k.lower() == "set-cookie":
            nv = v.split(";", 1)[0]
            if nv.split("=", 1)[1]:
                out.append(nv)
    return out


def wait_http(host, port, path="/", want=200, timeout=120):
    t0 = time.time()
    last = None
    while time.time() - t0 < timeout:
        try:
            st, _h, _b = request(host, port, "GET", path)
            if st == want:
                return time.time() - t0
            last = st
        except Exception as e:
            last = e
        time.sleep(0.2)
    raise RuntimeError(f"等待 {host}:{port}{path} 超时，最后结果 {last}")


BASIC = "Basic " + base64.b64encode(f"{USER}:{PASS}".encode()).decode()


# ---------------------------------------------------------------- 准备：密码哈希


def prepare():
    """生成各方使用的密码哈希（同一个明文密码），写入 WORK/hashes.json，只生成一次。"""
    os.makedirs(WORK, exist_ok=True)
    p = os.path.join(WORK, "hashes.json")
    if os.path.exists(p):
        return json.load(open(p))

    def digest(args):
        out = sh(f"docker run --rm {IMG['authelia']} authelia crypto hash generate {args} --password {shlex.quote(PASS)}", check=True)
        return out.strip().splitlines()[-1].split("Digest: ", 1)[1].strip()

    h = {
        # Authelia 默认参数：argon2id t=3 m=65536 p=4
        "argon2id": digest("argon2"),
        # bcrypt cost 10：与 Gately（golang.org/x/crypto/bcrypt DefaultCost）一致，给 nginx/Caddy 用
        "bcrypt10": digest("bcrypt --cost 10"),
        "apr1": sh(f"openssl passwd -apr1 {shlex.quote(PASS)}", check=True).strip(),
    }
    json.dump(h, open(p, "w"), indent=1)
    return h


# ---------------------------------------------------------------- 被测：启动与登录


def rm(*names):
    sh("docker rm -f " + " ".join(names) + " >/dev/null 2>&1")


def docker_run(name, args, image, cmd=""):
    sh(f"docker run -d --name {name} --network host --cpuset-cpus {SUT_CPUS} --ulimit nofile=200000:200000 "
       f"{args} {image} {cmd}", check=True)


def fresh_dir(sub):
    d = os.path.join(WORK, sub)
    sh(f"rm -rf {d}")
    os.makedirs(d)
    return d


def start_authelia(h):
    d = fresh_dir("authelia")
    conf = open(os.path.join(HERE, "conf", "authelia.yml")).read()
    for k in ("JWT", "SESSION", "STORAGE"):
        conf = conf.replace(f"@SECRET_{k}@", secrets.token_hex(32))
    open(os.path.join(d, "configuration.yml"), "w").write(conf)
    users = {"users": {USER: {"displayname": "Bench", "password": h["argon2id"], "email": "bench@bench.test"}}}
    # YAML 是 JSON 的超集，直接写 JSON
    open(os.path.join(d, "users.yml"), "w").write(json.dumps(users))
    docker_run(AUTHELIA, f"-v {d}:/config", IMG["authelia"])


def start_gately(h):
    d = fresh_dir("gately")
    # 先空跑一次生成默认配置（管理员账号、密钥等），与 suts/gately/start.sh 相同
    sh(f"docker run --rm --entrypoint sh -v {d}:/etc/gately {IMG['gately']} -c 'timeout 3 gately >/dev/null 2>&1; true'")
    ws_path = os.path.join(d, "webservice.json")
    ws = json.load(open(ws_path))

    def site(key, **auth):
        s = {"key": key, "name": "bench", "enable": True, "domains": ["bench.test"], "type": "proxy",
             "targets": [f"http://{BACKEND}"], "forwardedHeaders": True, "accessLog": False}
        s.update(auth)
        return s

    def listener(key, port, s):
        return {"key": key, "name": key, "enable": True, "network": "tcp", "listenIp": "", "port": port,
                "tls": True, "sites": [s], "default": {"type": "close"}}

    ws["settings"]["enable"] = True
    # 共享用户：密码为 bcrypt cost 10 哈希（与 Gately 保存明文时自动生成的哈希相同成本）
    ws["settings"]["authUsers"] = [{"id": "benchuser", "name": USER, "password": h["bcrypt10"], "totpSecret": "",
                                    "remark": "bench"}]
    ws["listeners"] = [
        listener("l-web", 8443, site("s-web", webAuth=True, sharedUsers=True)),
        listener("l-none", 8444, site("s-none")),
        listener("l-basic", 8445, site("s-basic", basicAuth=True, sharedUsers=True)),
    ]
    json.dump(ws, open(ws_path, "w"), indent=2)
    ssl_conf = {"settings": {"enable": False, "renewBeforeDays": 30, "shortLivedRenewHours": 24},
                "certs": [{"key": "c-bench", "remark": "bench", "enable": True, "source": "upload",
                           "certPem": open(os.path.join(CERTS, "bench.crt")).read(),
                           "keyPem": open(os.path.join(CERTS, "bench.key")).read()}]}
    json.dump(ssl_conf, open(os.path.join(d, "ssl.json"), "w"), indent=2)
    json.dump({"scenePending": False}, open(os.path.join(d, "onboarding.json"), "w"))
    docker_run(SUT, f"-v {d}:/etc/gately", IMG["gately"], f"-listen {GATELY_ADMIN}")


def start_nginx_authelia(h):
    start_authelia(h)
    d = fresh_dir("nginx")
    conf = open(os.path.join(HERE, "conf", "nginx.conf")).read().replace("@BACKEND@", BACKEND)
    open(os.path.join(d, "nginx.conf"), "w").write(conf)
    open(os.path.join(d, "htpasswd.bcrypt"), "w").write(f"{USER}:{h['bcrypt10']}\n")
    open(os.path.join(d, "htpasswd.apr1"), "w").write(f"{USER}:{h['apr1']}\n")
    docker_run(SUT, f"-v {d}/nginx.conf:/etc/nginx/nginx.conf:ro -v {d}:/work:ro -v {CERTS}:/certs:ro", IMG["nginx"])


def start_caddy_authelia(h):
    start_authelia(h)
    d = fresh_dir("caddy")
    conf = open(os.path.join(HERE, "conf", "Caddyfile")).read().replace("@BACKEND@", BACKEND)
    conf = conf.replace("@BCRYPT@", h["bcrypt10"])
    open(os.path.join(d, "Caddyfile"), "w").write(conf)
    docker_run(SUT, f"-v {d}/Caddyfile:/etc/caddy/Caddyfile:ro -v {CERTS}:/certs:ro", IMG["caddy"])


def start_lucky(h):
    d = fresh_dir("lucky")
    docker_run(SUT, f"-v {d}:/app/conf", IMG["lucky"])
    out = sh(f"python3 {HERE}/lucky_setup.py http://127.0.0.1:{LUCKY_ADMIN} {CERTS}/bench.crt {CERTS}/bench.key "
             f"{BACKEND} {USER} {PASS}", check=True, timeout=120)
    log("lucky setup", out.strip().replace("\n", " | "))


def login_gately():
    """Gately 网页认证登录：POST 表单到 /__gately_auth/login，成功 302 并下发站点会话 Cookie。"""
    body = urllib.parse.urlencode({"username": USER, "password": PASS, "next": "/"})
    st, hdrs, _ = request("bench.test", 8443, "POST", "/__gately_auth/login", body,
                          {"Content-Type": "application/x-www-form-urlencoded"})
    return st, set_cookies(hdrs)


def login_authelia():
    """Authelia 一次因素登录：POST JSON 到门户 /api/firstfactor，成功 200 并下发 authelia_session。"""
    body = json.dumps({"username": USER, "password": PASS, "keepMeLoggedIn": False,
                       "targetURL": "https://bench.test:8443/"})
    st, hdrs, _ = request("auth.bench.test", 8443, "POST", "/api/firstfactor", body,
                          {"Content-Type": "application/json"})
    return st, set_cookies(hdrs)


def oha_login(url, ctype, body, c, dur):
    return (f"taskset -c {LOAD_CPUS} oha --no-tui --insecure --redirect 0 -c {c} -z {dur}s -m POST "
            f"-T {shlex.quote(ctype)} -d {shlex.quote(body)} --output-format json {url}")


def oha_loader(url, ctype, body, ok_code):
    """登录压测（oha）：返回 函数(密码, 并发, 秒数) -> (命令, 解析函数)。"""
    return lambda pw, c, dur: (oha_login(url, ctype, body(pw), c, dur), login_parser(ok_code))


GATELY_LOAD = oha_loader("https://bench.test:8443/__gately_auth/login", "application/x-www-form-urlencoded",
                         lambda pw: urllib.parse.urlencode({"username": USER, "password": pw, "next": "/"}), 302)
AUTHELIA_LOAD = oha_loader("https://auth.bench.test:8443/api/firstfactor", "application/json",
                           lambda pw: json.dumps({"username": USER, "password": pw, "keepMeLoggedIn": False,
                                                  "targetURL": "https://bench.test:8443/"}), 200)


def lucky_load(pw, c, dur):
    """Lucky 挑战式登录无法用 oha 压，改用 loginload.py（多进程，每次登录 2 个请求）。"""
    return (f"taskset -c {LOAD_CPUS} python3 {HERE}/loginload.py 8443 {shlex.quote(pw)} {c} {dur}",
            lambda t: json.loads(t.strip().splitlines()[-1]))


def login_lucky():
    """Lucky 网页认证登录：取挑战 → RSA 加密账号密码 → POST，成功 ret=0 并下发会话 Cookie（取最后一个）。"""
    base = LOGINLOAD.login_base(8443)
    c = http.client.HTTPSConnection("bench.test", 8443, context=CTX, timeout=30)
    ok, _dt, cookies, why = LOGINLOAD.login_once(c, base, PASS)
    c.close()
    # 失败时把 Lucky 返回的原因作为状态记录（Lucky 登录接口失败时也可能返回 HTTP 200）
    return (200 if ok else why), cookies[-1:] if ok else []

# 每个被测：容器、启动函数、就绪检查端口、登录方式、wrk 测试项（名称: (端口, 凭据类型)）
SUTS = {
    "gately": {
        "start": start_gately, "containers": [SUT], "images": [IMG["gately"]],
        "login": login_gately, "loginLoad": GATELY_LOAD,
        "wrk": {"cookie": (8443, "cookie"), "none": (8444, None), "basic": (8445, "basic")},
        "hash": "bcrypt cost 10（golang.org/x/crypto/bcrypt DefaultCost）",
    },
    "nginx-authelia": {
        "start": start_nginx_authelia, "containers": [SUT, AUTHELIA], "images": [IMG["nginx"], IMG["authelia"]],
        "login": login_authelia, "loginLoad": AUTHELIA_LOAD,
        "wrk": {"cookie": (8443, "cookie"), "cookieOfficialNoKeepalive": (8447, "cookie"), "none": (8444, None),
                "basicBcrypt": (8445, "basic"), "basicApr1": (8446, "basic")},
        "hash": "Authelia argon2id t=3 m=65536KiB p=4；auth_basic 为 bcrypt cost 10 与 apr1（MD5 1000 轮）",
    },
    "caddy-authelia": {
        "start": start_caddy_authelia, "containers": [SUT, AUTHELIA], "images": [IMG["caddy"], IMG["authelia"]],
        "login": login_authelia, "loginLoad": AUTHELIA_LOAD,
        "wrk": {"cookie": (8443, "cookie"), "none": (8444, None), "basicBcrypt": (8445, "basic")},
        "hash": "Authelia argon2id t=3 m=65536KiB p=4；basic_auth 为 bcrypt cost 10",
    },
    "lucky": {
        "start": start_lucky, "containers": [SUT], "images": [IMG["lucky"]],
        "login": login_lucky, "loginLoad": lucky_load,
        "wrk": {"cookie": (8443, "cookie"), "none": (8444, None), "basic": (8445, "basic")},
        "hash": "Lucky 站点认证用户以明文保存（BasicAuthUserList）；网页登录为 RSA-2048 挑战加密传输后直接比较明文",
    },
}


def start(name, h):
    """启动被测并等待就绪，返回 cgroup 字典与启动耗时。"""
    rm(SUT, AUTHELIA)
    t0 = time.time()
    SUTS[name]["start"](h)
    # 不加认证的端口能取到源站 1024 字节即视为反代就绪
    wait_http("bench.test", 8444)
    if AUTHELIA in SUTS[name]["containers"]:
        wait_http("auth.bench.test", 8443, "/api/health")
    startup = time.time() - t0
    return {c: cgroup_of(c) for c in SUTS[name]["containers"]}, startup


def stop(name):
    rm(*SUTS[name]["containers"])


# ---------------------------------------------------------------- 测试项


def wrk_cmd(port, header):
    h = f"-H {shlex.quote(header)} " if header else ""
    return (f"taskset -c {LOAD_CPUS} wrk -t4 -c128 -d{DUR}s --latency --timeout 10s {h}"
            f"https://bench.test:{port}/")


def run_rounds(cgs, label, cmd, parse, rounds):
    runs = []
    for i in range(rounds):
        with Watch(cgs) as w:
            text = sh(cmd, timeout=DUR * 3 + 120)
        r = parse(text)
        # 保留 wrk 的错误明细行，便于区分连接、读写错误与超时
        for line in text.splitlines():
            if "Socket errors" in line or "Non-2xx" in line:
                r.setdefault("errorLines", []).append(line.strip())
        r.update(w.result)
        runs.append(r)
        log(label, i, json.dumps({k: v for k, v in r.items() if k not in ("peakPssBy", "cpuPercentBy")}))
        time.sleep(3)
    med = sorted(runs, key=lambda x: x.get("rps", 0))[len(runs) // 2]
    return {"median": med, "runs": runs}


def parse_oha_codes(t):
    """oha JSON：rps、延迟分位与状态码分布。"""
    j = json.loads(t[t.index("{"):])
    s = j["summary"]
    lp = j.get("latencyPercentiles", {})
    codes = {str(k): v for k, v in j.get("statusCodeDistribution", {}).items()}
    total = sum(codes.values())
    return {"rps": s.get("requestsPerSec", 0), "p50ms": round((lp.get("p50") or 0) * 1000, 2),
            "p99ms": round((lp.get("p99") or 0) * 1000, 2), "codes": codes, "total": total,
            "errors": sum(j.get("errorDistribution", {}).values()),
            "elapsed": s.get("total", 0)}


def login_parser(ok_code):
    def parse(t):
        r = parse_oha_codes(t)
        ok = r["codes"].get(str(ok_code), 0)
        # 成功登录次数每秒：只计成功状态码（Gately 302、Authelia 200）
        r["okPerSec"] = round(ok / r["elapsed"], 1) if r["elapsed"] else 0
        r["ok"] = ok
        return r
    return parse


def checks(name):
    """功能核对：未登录被拦、登录成功、带凭据能取到源站内容。返回 (检查结果, 凭据头)。"""
    s = SUTS[name]
    out, creds = {}, {}
    st, _h, b = request("bench.test", 8443, "GET", "/")
    out["anonStatus"] = st
    if s["login"]:
        lst, cookies = s["login"]()
        out["loginStatus"], out["cookieNames"] = lst, [c.split("=", 1)[0] for c in cookies]
        if not cookies:
            raise RuntimeError(f"{name} 登录失败：{lst}")
        creds["cookie"] = "Cookie: " + "; ".join(cookies)
        st, _h, b = request("bench.test", 8443, "GET", "/", headers={"Cookie": "; ".join(cookies)})
        out["cookieGet"] = f"{st} {len(b)}"
        if st != 200 or len(b) != 1024:
            raise RuntimeError(f"{name} 带 Cookie 访问失败：{st} {len(b)}")
    creds["basic"] = "Authorization: " + BASIC
    for tname, (port, kind) in s["wrk"].items():
        if kind == "basic":
            st, _h, b = request("bench.test", port, "GET", "/")
            st2, _h, b2 = request("bench.test", port, "GET", "/", headers={"Authorization": BASIC})
            out[f"{tname}Anon"], out[f"{tname}Get"] = st, f"{st2} {len(b2)}"
            if st2 != 200 or len(b2) != 1024:
                raise RuntimeError(f"{name} {tname} 基本认证访问失败：{st2}")
    return out, creds


def bad_login_round(name, h):
    """错误密码：新启动被测，先正常登录一次拿到 Cookie，再以 8 并发连续提交错误密码 BAD_DUR 秒，
    之后检查正确密码能否登录（是否被防爆破拦住）、已登录的 Cookie 是否仍可用。"""
    s = SUTS[name]
    cgs, _ = start(name, h)
    _st, cookies = s["login"]()
    cmd, parse = s["loginLoad"](WRONG, LOGIN_C, BAD_DUR)
    with Watch(cgs) as w:
        text = sh(cmd, timeout=BAD_DUR + 120)
    r = parse(text)
    r.update(w.result)
    lst, lc = s["login"]()
    r["correctLoginAfter"] = {"status": lst, "gotCookie": bool(lc)}
    st, _h, b = request("bench.test", 8443, "GET", "/", headers={"Cookie": "; ".join(cookies)})
    r["existingCookieAfter"] = f"{st} {len(b)}"
    stop(name)
    return r


def bench_one(name, h):
    s = SUTS[name]
    log("=== 开始", name)
    cgs, startup = start(name, h)
    res = {"sut": name, "startupSec": round(startup, 3), "hash": s["hash"], "images": {}, "tests": {}}
    # 空闲内存在任何登录之前取：Authelia 做一次 argon2id 会分配 64 MiB，之后 Go 运行时不一定立即归还
    time.sleep(10)
    res["idle"] = snapshot_all(cgs)
    res["checks"], creds = checks(name)
    log("checks", json.dumps(res["checks"], ensure_ascii=False))
    time.sleep(3)
    res["idleAfterFirstLogin"] = snapshot_all(cgs)
    for img in s["images"]:
        res["images"][img] = int(sh(f"docker image inspect -f '{{{{.Size}}}}' {img}").strip() or 0)
    res["imageBytes"] = sum(res["images"].values())
    # 1. 已登录请求的转发性能（含同一进程不加认证的对照）
    for tname, (port, kind) in s["wrk"].items():
        # Cookie 在有效期内反复使用：Authelia 会话默认 inactivity 5 分钟、expiration 1 小时，单项测试远短于此
        hdr = creds.get(kind) if kind else None
        res["tests"][tname] = run_rounds(cgs, f"{name} {tname}", wrk_cmd(port, hdr), RUN.parse_wrk, ROUNDS)
    res["idleAfterWrk"] = snapshot_all(cgs)
    # 2. 登录吞吐：正确密码
    if s["loginLoad"]:
        cmd, parse = s["loginLoad"](PASS, LOGIN_C, DUR)
        res["tests"]["loginOk"] = run_rounds(cgs, f"{name} loginOk c{LOGIN_C}", cmd, parse, ROUNDS)
        cmd, parse = s["loginLoad"](PASS, LOGIN_C_HIGH, 10)
        res["tests"]["loginOkHighConc"] = run_rounds(cgs, f"{name} loginOk c{LOGIN_C_HIGH}", cmd, parse, 1)
        time.sleep(5)
        res["idleAfterLogins"] = snapshot_all(cgs)
    stop(name)
    # 3. 错误密码：每轮重新启动（防爆破状态不跨轮）
    if s["loginLoad"]:
        runs = []
        for i in range(ROUNDS):
            r = bad_login_round(name, h)
            log(f"{name} loginBad", i, json.dumps({k: v for k, v in r.items() if k not in ("peakPssBy", "cpuPercentBy")},
                                                 ensure_ascii=False))
            runs.append(r)
        med = sorted(runs, key=lambda x: x.get("rps", 0))[len(runs) // 2]
        res["tests"]["loginBad"] = {"median": med, "runs": runs}
    os.makedirs(RESULTS, exist_ok=True)
    with open(os.path.join(RESULTS, f"auth-{name}.json"), "w") as f:
        json.dump(res, f, indent=1, ensure_ascii=False)
    log("=== 完成", name)


# ---------------------------------------------------------------- 汇总


def summary():
    rows = []
    for name in SUTS:
        p = os.path.join(RESULTS, f"auth-{name}.json")
        if not os.path.exists(p):
            continue
        r = json.load(open(p))
        print(f"\n### {name}  启动 {r['startupSec']}s  空闲 PSS {r['idle']['pssKiB'] / 1024:.1f} MiB  "
              f"进程 {r['idle']['procs']}  镜像 {r['imageBytes'] / 1e6:.1f} MB  哈希：{r['hash']}")
        print("| 测试 | rps / 成功登录每秒 | p50 ms | p99 ms | CPU % | 峰值 PSS MiB | 非 2xx/状态码 |")
        print("|---|---|---|---|---|---|---|")
        for t, v in r["tests"].items():
            m = v["median"]
            rate = m.get("okPerSec", m.get("rps"))
            extra = m.get("codes") or m.get("reasons") or m.get("non2xx", "")
            if t == "loginBad":
                extra = (f"{m.get('codes') or m.get('reasons')} 之后正确登录 {m['correctLoginAfter']['status']}，"
                         f"旧 Cookie {m['existingCookieAfter']}")
                rate = f"{m['rps']:.1f}（全部请求）"
            print(f"| {t} | {rate if isinstance(rate, str) else round(rate, 1)} | {m.get('p50ms', 0):.2f} | "
                  f"{m.get('p99ms', 0):.2f} | {m['sutCpuPercent']} | {m['peakPssKiB'] / 1024:.1f} | {extra} |")


def main():
    if len(sys.argv) > 1 and sys.argv[1] == "summary":
        summary()
        return
    names = sys.argv[1].split(",") if len(sys.argv) > 1 else list(SUTS)
    h = prepare()
    for n in names:
        try:
            bench_one(n, h)
        except Exception as e:
            log("失败", n, repr(e))
            print(sh(f"docker logs --tail 30 {SUT} 2>&1; docker logs --tail 30 {AUTHELIA} 2>&1"))
            stop(n)
            raise
    summary()


if __name__ == "__main__":
    main()
