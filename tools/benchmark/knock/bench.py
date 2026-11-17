#!/usr/bin/env python3
"""端口敲门专项对比（在压测虚拟机上以 root 运行，入口为 run.sh）。

用法：bench.py [被测名 ...]   被测名：direct gately gately-web fwknop fnknock，默认全部，按顺序依次运行，互不重叠。

网络拓扑（全部在本机内，不出网）：
  客户端命名空间 knockc：veth knk-c 203.0.113.10/24（合法客户端，TEST-NET-3 公网地址），默认路由经 203.0.113.1
  攻击命名空间   knocka：veth kna-c 198.51.100.10/24（攻击者，TEST-NET-2），默认路由经 198.51.100.1
  宿主机：knk-h 203.0.113.1/24、kna-h 198.51.100.1/24；被测程序与源站均为 host 网络
受保护服务是源站 knock-origin（nginx，vCPU 2,3，监听 9100）。Gately、fwknop 在防火墙中拦截 tcp/9100，
fn-knock 用 TCP 端口映射 9200 -> 127.0.0.1:9100（use_auth）。客户端地址都是公网地址，三者的“内网免认证”都不生效。
客户端与攻击工具都绑在压测 CPU（默认 vCPU 4-7），被测程序在 vCPU 0,1。结果写 results/knock-<被测名>.json。
"""
import http.cookiejar
import json
import os
import random
import shutil
import statistics
import subprocess
import sys
import tempfile
import time
import urllib.request

HERE = os.path.dirname(os.path.abspath(__file__))
BASE = os.path.dirname(HERE)
LIB = os.path.join(BASE, "lib")
sys.path.insert(0, LIB)
import run as R  # noqa: E402  复用 wrk / oha 输出解析，口径与 run.py 一致

E = os.environ.get
SUT_CPUS = E("SUT_CPUS", "0,1")
ORIGIN_CPUS = E("ORIGIN_CPUS", "2,3")
LOAD_CPUS = E("LOAD_CPUS", "4-7")
SERVER, CLIENT, ATTACK = "203.0.113.1", "203.0.113.10", "198.51.100.10"
NS_C, NS_A = "knockc", "knocka"
PROT_PORT = int(E("PROT_PORT", "9100"))
FNK_PORT = int(E("FNK_PORT", "9200"))
KNOCK_PORT = int(E("KNOCK_PORT", "62201"))
WEB_PORT = int(E("GATELY_WEB_PORT", "8080"))
N_LAT = int(E("N_LAT", "60"))
FLOOD_RATES = [int(x) for x in E("FLOOD_RATES", "10000,50000").split(",")]
FLOOD_DUR = int(E("FLOOD_DUR", "40"))
FLOOD_KNOCKS = int(E("FLOOD_KNOCKS", "10"))
# 洪泛期间两次合法敲门之间至少间隔的秒数：超过 2 秒时来源网段在 Gately 的限速表里已空闲可清理，
# 每次敲门都相当于“一个新来源第一次敲门”，与真实用户偶尔敲一次的情形一致
FLOOD_PACE = float(E("FLOOD_PACE", "2.5"))
FLOOD_PROCS = int(E("FLOOD_PROCS", "2"))
DP_DUR = int(E("DP_DUR", "20"))
DP_ROUNDS = int(E("DP_ROUNDS", "3"))
GATELY_IMAGE = E("GATELY_IMAGE", "ghcr.io/wangwanjie/gately:v0.6.8.1")
FNK_IMAGE = E("FNK_IMAGE", "kcilnk/fn-knock:2.5.11")
FWK_IMAGE = E("FWK_IMAGE", "bench-fwknop:2.6.11")
ORIGIN_IMAGE = E("ORIGIN_IMAGE", "nginx:1.29-alpine")
PASSWORD = "Bench!Visit2026"
WORK = "/tmp/knock-bench"
# 结果目录：压测机上默认 /root/benchmark/results；本机试跑用 RESULTS_DIR 指到临时目录，不要写进仓库
RESULTS = E("RESULTS_DIR", os.path.join(BASE, "results"))


def sh(cmd, check=False, timeout=None, quiet=False):
    p = subprocess.run(cmd, shell=isinstance(cmd, str), capture_output=True, text=True, timeout=timeout)
    if check and p.returncode != 0:
        raise RuntimeError(f"{cmd}\n{p.stdout}\n{p.stderr}")
    return p.stdout + ("" if quiet else p.stderr)


def log(*a):
    print(time.strftime("%H:%M:%S"), *a, flush=True)


def pct(vals, p):
    """最近秩百分位（与 wrk/oha 的口径接近），vals 已排序。"""
    if not vals:
        return None
    k = max(0, min(len(vals) - 1, int(round(p / 100 * len(vals) + 0.5)) - 1))
    return round(vals[k], 3)


def summarize(vals):
    v = sorted(x for x in vals if x is not None)
    if not v:
        return {"n": 0}
    return {"n": len(v), "p50ms": pct(v, 50), "p90ms": pct(v, 90), "p99ms": pct(v, 99), "maxms": round(v[-1], 3),
            "minms": round(v[0], 3), "meanms": round(statistics.mean(v), 3)}


# ---------------------------------------------------------------- 环境

def cgroup_of(name):
    cid = sh(f"docker inspect -f '{{{{.Id}}}}' {name}").strip()
    # 压测机为 systemd 驱动；本机调试的 Docker-in-Docker 为 cgroupfs 驱动
    for path in (f"/sys/fs/cgroup/system.slice/docker-{cid}.scope", f"/sys/fs/cgroup/docker/{cid}"):
        if os.path.isdir(path):
            return path
    raise RuntimeError("找不到容器 cgroup: " + cid)


def snapshot(cg):
    return json.loads(sh(f"python3 {LIB}/sample.py snapshot {cg}"))


def start_watch(cg, seconds, out):
    return subprocess.Popen(["python3", f"{LIB}/sample.py", "watch", cg, str(seconds), out])


def end_watch(p, out):
    p.wait()
    with open(out) as f:
        return json.load(f)


def in_ns(ns, cmd):
    return f"ip netns exec {ns} taskset -c {LOAD_CPUS} {cmd}"


def setup_net():
    teardown_net()
    for ns, h, c, hip, cip in ((NS_C, "knk-h", "knk-c", SERVER, CLIENT), (NS_A, "kna-h", "kna-c", "198.51.100.1", ATTACK)):
        sh(f"ip netns add {ns}", check=True)
        sh(f"ip link add {h} type veth peer name {c}", check=True)
        sh(f"ip link set {c} netns {ns}", check=True)
        sh(f"ip addr add {hip}/24 dev {h} && ip link set {h} up", check=True)
        sh(f"ip netns exec {ns} sh -c 'ip addr add {cip}/24 dev {c} && ip link set {c} up && ip link set lo up "
           f"&& ip route add default via {hip}'", check=True)
        # 新命名空间的网络参数是内核默认值，按压测机的 90-bench.conf 对齐（端口范围、TIME_WAIT 复用、监听队列），
        # 否则短连接测试会受客户端临时端口与 TIME_WAIT 限制
        sh(f"ip netns exec {ns} sysctl -qw net.ipv4.ip_local_port_range='1024 65000' net.ipv4.tcp_tw_reuse=1 "
           f"net.core.somaxconn=65535 net.ipv4.tcp_max_syn_backlog=65535", check=True)
        # 攻击命名空间伪造来源：宿主机这一侧关闭反向路径过滤（只影响这块测试网卡）
        sh(f"sysctl -qw net.ipv4.conf.{h}.rp_filter=0")
    # all.rp_filter 为 1（严格）时伪造来源会被丢弃，临时改为 2（宽松），结束后恢复
    old = sh("sysctl -n net.ipv4.conf.all.rp_filter").strip()
    with open(os.path.join(WORK, "rp_filter.all"), "w") as f:
        f.write(old)
    if old == "1":
        sh("sysctl -qw net.ipv4.conf.all.rp_filter=2")


def teardown_net():
    for ns, h in ((NS_C, "knk-h"), (NS_A, "kna-h")):
        sh(f"ip netns del {ns} 2>/dev/null; ip link del {h} 2>/dev/null")
    p = os.path.join(WORK, "rp_filter.all")
    if os.path.exists(p):
        sh(f"sysctl -qw net.ipv4.conf.all.rp_filter={open(p).read().strip()}")
        os.remove(p)


def notrack(on):
    """洪泛期间对敲门端口的 UDP 关闭连接跟踪：伪造来源的洪泛每个包都会新建一条连接跟踪，几秒就能占满
    nf_conntrack 表，表满后连合法敲门包也被内核丢弃，测到的是内核表容量而不是被测程序。三者同样处理。"""
    sh("nft delete table inet knockbench 2>/dev/null")
    if on:
        sh("nft -f - <<'N'\ntable inet knockbench {\n chain raw {\n  type filter hook prerouting priority raw; policy accept;\n"
           f"  udp dport {KNOCK_PORT} notrack\n }}\n}}\nN", check=True)


def start_origin():
    sh("docker rm -f knock-origin 2>/dev/null")
    os.makedirs(f"{WORK}/origin", exist_ok=True)
    with open(f"{WORK}/origin/1k.txt", "w") as f:
        f.write("a" * 1024)
    sh(f"docker run -d --name knock-origin --network host --cpuset-cpus {ORIGIN_CPUS} --ulimit nofile=200000:200000 "
       f"-v {HERE}/origin.conf:/etc/nginx/nginx.conf:ro -v {WORK}/origin:/srv/knock:ro {ORIGIN_IMAGE}", check=True)
    for _ in range(100):
        if sh(f"curl -s -o /dev/null -w '%{{http_code}}' http://127.0.0.1:{PROT_PORT}/").strip() == "200":
            return
        time.sleep(0.1)
    raise RuntimeError("源站未就绪")


class Client:
    """客户端命名空间里常驻的 client.py。"""

    def __init__(self):
        self.p = subprocess.Popen(in_ns(NS_C, f"python3 {HERE}/client.py"), shell=True, stdin=subprocess.PIPE,
                                  stdout=subprocess.PIPE, text=True)

    def call(self, **c):
        self.p.stdin.write(json.dumps(c) + "\n")
        self.p.stdin.flush()
        return json.loads(self.p.stdout.readline())

    def close(self):
        try:
            self.call(op="quit")
        except Exception:
            pass
        self.p.wait(timeout=5)


# ---------------------------------------------------------------- SPA 报文

def gen_packets(n, keys, out, allow=CLIENT):
    """用 fwknop 2.6.11 客户端（-T 只生成不发送）生成 n 个 SPA 报文，每行一个。报文带时间戳，120 秒内有效。"""
    enc, hmac = keys
    d = tempfile.mkdtemp(dir=WORK)
    cmd = (f"for i in $(seq {n}); do fwknop -A tcp/{PROT_PORT} -a {allow} -D {SERVER} --key-base64-rijndael {enc} "
           f"--key-base64-hmac {hmac} --use-hmac -T -B /out/p -b >/dev/null || exit 1; done")
    # 生成容器绑在压测 CPU，不占被测程序的 CPU
    sh(["docker", "run", "--rm", "--cpuset-cpus", LOAD_CPUS, "-v", f"{d}:/out", FWK_IMAGE, "sh", "-c", cmd], check=True)
    pkts = [x.strip() for x in open(f"{d}/p") if x.strip()]
    if out:
        shutil.move(f"{d}/p", out)
    shutil.rmtree(d, ignore_errors=True)
    if len(pkts) != n:
        raise RuntimeError(f"生成 SPA 报文数量不对: {len(pkts)}/{n}")
    return pkts


def new_keys():
    import base64
    return base64.b64encode(os.urandom(32)).decode(), base64.b64encode(os.urandom(64)).decode()


class PacketPool:
    """合法敲门报文池：报文有时间戳，按批生成，超过 60 秒的批次丢弃重生成。"""

    def __init__(self, keys, batch=30):
        self.keys, self.batch, self.pkts, self.at = keys, batch, [], 0

    def get(self):
        if not self.pkts or time.time() - self.at > 60:
            self.pkts, self.at = gen_packets(self.batch, self.keys, None), time.time()
        return self.pkts.pop()


# ---------------------------------------------------------------- 被测程序

class SUT:
    name = ""
    container = "knock-sut"
    image = ""
    prot = PROT_PORT
    kind = "spa"          # spa / web
    settle = 0.3          # 撤销并确认端口已关闭后，再等多久发起下一次敲门
    probe_ports = []

    def start(self):
        raise NotImplementedError

    def stop(self):
        sh(f"docker stop -t 15 {self.container} 2>/dev/null; docker rm -f -v {self.container} 2>/dev/null")

    def revoke(self):
        raise NotImplementedError

    def knock_cmd(self, timeout=5):
        raise NotImplementedError

    def info(self):
        return {}

    def floods(self):
        """返回 [(名称, 启动函数(速率)->Popen)]。"""
        return []


def http_json(opener, method, url, body=None):
    data = json.dumps(body).encode() if body is not None else None
    req = urllib.request.Request(url, data=data, method=method, headers={"Content-Type": "application/json"})
    with opener.open(req, timeout=15) as r:
        return json.loads(r.read() or b"{}")


def udp_flood(mode, pfile):
    def f(rate):
        return subprocess.Popen(in_ns(NS_A, f"python3 {HERE}/flood.py {mode} {SERVER} {KNOCK_PORT} {rate} {FLOOD_DUR} "
                                            f"{pfile} {FLOOD_PROCS} {ATTACK}"),
                                shell=True, stdout=subprocess.PIPE, text=True)
    return f


class SpaSUT(SUT):
    def __init__(self):
        self.keys = new_keys()
        self.pool = None

    def knock_cmd(self, timeout=5):
        return {"op": "spa", "pkt": self.pool.get(), "server": SERVER, "knockPort": KNOCK_PORT, "port": self.prot,
                "timeout": timeout}

    def floods(self):
        # 无效报文：用另一对随机密钥生成，格式与真实报文相同，两者都要做完 HMAC 才能判定无效
        pfile = f"{WORK}/invalid.txt"
        gen_packets(50, new_keys(), pfile)
        return [("udp-single", udp_flood("single", pfile)), ("udp-spoof", udp_flood("spoof", pfile))]


class Gately(SpaSUT):
    name = "gately"
    image = GATELY_IMAGE
    probe_ports = [("tcp", PROT_PORT), ("udp", KNOCK_PORT), ("tcp", 16680)]

    def start(self):
        d = f"{WORK}/gately"
        shutil.rmtree(d, ignore_errors=True)
        os.makedirs(d)
        with open(f"{WORK}/keys", "w") as f:
            f.write(" ".join(self.keys))
        # 先空跑一次生成默认配置（管理员账号、密钥等），再写入敲门规则与放行入口站点
        sh(f"docker run --rm --entrypoint sh -v {d}:/etc/gately {self.image} -c 'timeout 3 gately >/dev/null 2>&1; true'")
        sh(f"PROT_PORT={PROT_PORT} GATELY_WEB_PORT={WEB_PORT} KNOCK_PORT={KNOCK_PORT} python3 {HERE}/gately_gen.py {d} {WORK}/keys",
           check=True)
        # 启动耗时：从 docker run 到敲门端口监听、拦截规则生效（fn-knock 为健康检查通过）
        t_run = time.time()
        sh(f"docker run -d --name {self.container} --network host --cpuset-cpus {SUT_CPUS} --ulimit nofile=200000:200000 "
           f"--cap-add NET_ADMIN --cap-add NET_RAW -v {d}:/etc/gately {self.image}", check=True)
        self.jar = urllib.request.build_opener(urllib.request.HTTPCookieProcessor(http.cookiejar.CookieJar()))
        for _ in range(200):
            try:
                http_json(self.jar, "POST", "http://127.0.0.1:16680/api/login", {"username": "admin", "password": PASSWORD})
                st = http_json(self.jar, "GET", "http://127.0.0.1:16680/api/knock/status")
                if st.get("listening") and st.get("firewall", {}).get("active"):
                    self.ready_sec = round(time.time() - t_run, 2)
                    break
            except Exception:
                pass
            time.sleep(0.1)
        else:
            raise RuntimeError("Gately 敲门模块未就绪: " + sh(f"docker logs {self.container} 2>&1 | tail -20"))
        # 报文时间戳早于 Gately 本次启动的会被拒绝（防重放），所以启动后再生成
        time.sleep(1.1)
        self.pool = PacketPool(self.keys)

    def stop(self):
        super().stop()
        # 正常停止时 Gately 会删除自己的表；被强制结束时兜底清理
        sh("nft delete table inet gately_knock 2>/dev/null; nft delete table inet gately 2>/dev/null")

    def revoke(self):
        http_json(self.jar, "DELETE", "http://127.0.0.1:16680/api/knock/grants", {"addr": CLIENT})

    def status(self):
        try:
            st = http_json(self.jar, "GET", "http://127.0.0.1:16680/api/knock/status")
            return {"counters": st.get("counters"), "firewall": {k: st.get("firewall", {}).get(k)
                                                                 for k in ("backend", "version", "dropped", "allowed")}}
        except Exception as e:
            return {"err": repr(e)}

    def info(self):
        prog = sh(f"docker run --rm --entrypoint sh {self.image} -c 'stat -c %s $(command -v gately)'").strip()
        ver = sh(f"docker exec {self.container} gately -version").strip()
        return {"version": ver, "programBytes": int(prog or 0), "programNote": "gately 单个静态二进制（含全部模块与管理界面）"}


class GatelyWeb(Gately):
    """Gately 站点“放行入口”：网页登录后为来源地址写入同一条敲门规则的防火墙放行。"""
    name = "gately-web"
    kind = "web"
    probe_ports = [("tcp", PROT_PORT), ("tcp", WEB_PORT), ("tcp", 16680)]

    def knock_cmd(self, timeout=5):
        return {"op": "gweb", "server": SERVER, "webPort": WEB_PORT, "domain": "knock.test", "user": "bench",
                "password": PASSWORD, "port": self.prot, "timeout": timeout}

    def floods(self):
        # 无效登录：错误密码的登录表单（Gately 网页登录没有验证码）
        body = f"{WORK}/gweb-bad.txt"
        with open(body, "w") as f:
            f.write("username=bench&password=wrong-password&next=/")
        url = f"http://{SERVER}:{WEB_PORT}/__gately_auth/login"
        hdr = (f"-H 'Host: knock.test:{WEB_PORT}' -H 'Origin: http://knock.test:{WEB_PORT}' "
               "-H 'Content-Type: application/x-www-form-urlencoded'")
        return [("http-badlogin", http_flood(url, hdr, body))]


def http_flood(url, hdr, body_file, method="POST"):
    def f(rate):
        data = f"-D {body_file}" if body_file else ""
        return subprocess.Popen(in_ns(NS_A, f"oha --no-tui -z {FLOOD_DUR}s -q {rate} -c 256 -m {method} {hdr} {data} "
                                            f"--output-format json {url}"),
                                shell=True, stdout=subprocess.PIPE, text=True)
    return f


class Fwknop(SpaSUT):
    name = "fwknop"
    image = FWK_IMAGE
    probe_ports = [("tcp", PROT_PORT), ("udp", KNOCK_PORT)]

    def start(self):
        d = f"{WORK}/fwknop"
        shutil.rmtree(d, ignore_errors=True)
        os.makedirs(d)
        # UDP 服务器模式（-U，与 Gately 一样监听 UDP 62201）；Debian 默认配置为 NFQUEUE 抓包模式，这里关闭
        with open(f"{d}/fwknopd.conf", "w") as f:
            f.write(f"ENABLE_NFQ_CAPTURE N;\nENABLE_UDP_SERVER Y;\nUDPSERV_PORT {KNOCK_PORT};\n")
        with open(f"{d}/access.conf", "w") as f:
            f.write(f"SOURCE ANY\nOPEN_PORTS tcp/{PROT_PORT}\nKEY_BASE64 {self.keys[0]}\nHMAC_KEY_BASE64 {self.keys[1]}\n"
                    "FW_ACCESS_TIMEOUT 300\n")
        os.chmod(f"{d}/fwknopd.conf", 0o600)
        os.chmod(f"{d}/access.conf", 0o600)
        # fwknopd 只负责插入放行规则，受保护端口的默认拒绝要自己写（与 Gately 的拦截语义对齐：
        # 回环、已建立的连接放行，其余发往受保护端口的新连接丢弃）。fwknopd 把 FWKNOP_INPUT 跳转插在 INPUT 第一条
        fw_cleanup()
        sh("iptables -N KNOCK_BENCH && iptables -A KNOCK_BENCH -i lo -j RETURN "
           "&& iptables -A KNOCK_BENCH -m conntrack --ctstate ESTABLISHED,RELATED -j RETURN "
           f"&& iptables -A KNOCK_BENCH -p tcp --dport {PROT_PORT} -j DROP && iptables -A INPUT -j KNOCK_BENCH", check=True)
        # 启动耗时：从 docker run 到敲门端口监听、拦截规则生效（fn-knock 为健康检查通过）
        t_run = time.time()
        sh(f"docker run -d --name {self.container} --network host --cpuset-cpus {SUT_CPUS} --cap-add NET_ADMIN --cap-add NET_RAW "
           f"-v {d}:/etc/fwknop {self.image} fwknopd -f -c /etc/fwknop/fwknopd.conf -a /etc/fwknop/access.conf", check=True)
        for _ in range(100):
            if f":{KNOCK_PORT} " in sh(f"ss -lun 'sport = :{KNOCK_PORT}'") and "FWKNOP_INPUT" in sh("iptables -S INPUT"):
                self.ready_sec = round(time.time() - t_run, 2)
                break
            time.sleep(0.1)
        else:
            raise RuntimeError("fwknopd 未就绪: " + sh(f"docker logs {self.container} 2>&1 | tail -20"))
        self.pool = PacketPool(self.keys)

    def stop(self):
        super().stop()
        fw_cleanup()

    def revoke(self):
        sh("iptables -F FWKNOP_INPUT 2>/dev/null")

    def info(self):
        out = sh(["docker", "run", "--rm", self.image, "sh", "-c",
                  "fwknopd --version; stat -c %s /usr/sbin/fwknopd; stat -L -c %s /usr/lib/*/libfko.so.3; "
                  "dpkg-query -W -f '${Installed-Size}\\n' fwknop-server libfko3t64"], quiet=True).split("\n")
        try:
            return {"version": out[0], "programBytes": int(out[1]) + int(out[2]),
                    "programNote": "fwknopd + libfko.so（另依赖系统 iptables、libpcap 等）",
                    "debPackagesInstalledKiB": int(out[3]) + int(out[4])}
        except (ValueError, IndexError):
            return {"raw": out}


def fw_cleanup():
    sh("iptables -D INPUT -j KNOCK_BENCH 2>/dev/null; iptables -F KNOCK_BENCH 2>/dev/null; iptables -X KNOCK_BENCH 2>/dev/null")
    sh("iptables -D INPUT -j FWKNOP_INPUT 2>/dev/null; iptables -F FWKNOP_INPUT 2>/dev/null; iptables -X FWKNOP_INPUT 2>/dev/null")


class FnKnock(SUT):
    name = "fnknock"
    image = FNK_IMAGE
    kind = "web"
    prot = FNK_PORT
    # fn-knock 网关对“未授权”结果缓存 1 秒（auth_cache_unauthorized_ttl_seconds），确认端口已关闭的探测会写入这个缓存，
    # 所以等缓存过期再登录，测到的是登录本身到放行的时间
    settle = 1.3
    probe_ports = [("tcp", FNK_PORT), ("tcp", 7999), ("tcp", 7991)]
    A = "http://127.0.0.1:7991"

    def start(self):
        # 启动耗时：从 docker run 到敲门端口监听、拦截规则生效（fn-knock 为健康检查通过）
        t_run = time.time()
        sh(f"docker run -d --name {self.container} --network host --cpuset-cpus {SUT_CPUS} --ulimit nofile=200000:200000 "
           f"-e TZ=Asia/Shanghai -e FN_KNOCK_RUNTIME_TARGET=docker -e ADMIN_VIEW_HOST=0.0.0.0 -e BACKEND_HOST=127.0.0.1 "
           f"{self.image}", check=True)
        self.jar = urllib.request.build_opener(urllib.request.HTTPCookieProcessor(http.cookiejar.CookieJar()))
        for _ in range(600):
            try:
                if http_json(self.jar, "GET", self.A + "/api/admin/healthz").get("success"):
                    self.ready_sec = round(time.time() - t_run, 2)
                    break
            except Exception:
                pass
            time.sleep(0.1)
        else:
            raise RuntimeError("fn-knock 未就绪: " + sh(f"docker logs {self.container} 2>&1 | tail -20"))
        steps = [
            ("POST", "/api/admin/panel/password", {"password": PASSWORD}),
            ("POST", "/api/admin/config/run_type", {"run_type": 3}),
            ("POST", "/api/admin/config/protocol_mapping_feature", {"enabled": True}),
            ("POST", "/api/admin/config/stream_mappings",
             {"mappings": [{"protocol": "tcp", "listen_port": FNK_PORT, "target": f"127.0.0.1:{PROT_PORT}", "use_auth": True}]}),
            ("POST", "/api/admin/auth/accounts", {"username": "bench", "password": PASSWORD}),
            ("POST", "/api/admin/auth/mode/switch", {"mode": "password"}),
        ]
        for method, path, body in steps:
            r = http_json(self.jar, method, self.A + path, body)
            if not r.get("success"):
                raise RuntimeError(f"fn-knock 初始化失败 {path}: {r}")
        for _ in range(100):
            if f":{FNK_PORT} " in sh(f"ss -ltn 'sport = :{FNK_PORT}'"):
                break
            time.sleep(0.1)
        else:
            raise RuntimeError("fn-knock 端口映射未监听")

    def revoke(self):
        # 登录后的放行跟随会话（post_login_ip_grant_mode=follow_session），删除会话即撤销；顺带清掉自动白名单
        for s in http_json(self.jar, "GET", self.A + "/api/admin/sessions").get("data", []):
            http_json(self.jar, "DELETE", self.A + f"/api/admin/sessions/{s['id']}")
        for w in http_json(self.jar, "GET", self.A + "/api/admin/whitelist").get("data", []):
            if w.get("source") == "auto":
                http_json(self.jar, "DELETE", self.A + f"/api/admin/whitelist/{w['id']}")

    def knock_cmd(self, timeout=5):
        return {"op": "fnk", "server": SERVER, "authPort": 7999, "user": "bench", "password": PASSWORD,
                "port": self.prot, "timeout": timeout}

    def floods(self):
        # 无效登录：验证码证明无效的登录请求（不解题的攻击者只能这样发），fn-knock 在校验验证码后直接拒绝
        body = f"{WORK}/fnk-bad.json"
        with open(body, "w") as f:
            json.dump({"method": "password", "username": "bench", "password": "wrong",
                       "captcha": {"provider": "pow", "proof": "eyJhbGdvcml0aG0iOiJTSEEtMjU2In0="}}, f)
        login = http_flood(f"http://{SERVER}:7999/__auth__/api/auth/login", "-H 'Content-Type: application/json'", body)
        # 未授权连接：直接连受保护的映射端口（每个连接都由网关在用户态查授权后重置）
        conn = http_flood(f"http://{SERVER}:{FNK_PORT}/", "--disable-keepalive", None, "GET")
        return [("http-badlogin", login), ("tcp-unauth-conn", conn)]

    def info(self):
        out = sh(f"docker run --rm --entrypoint sh {self.image} -c 'du -sb /opt/fn-knock/bin | cut -f1; du -sb /opt/fn-knock | cut -f1'").split()
        try:
            return {"version": "2.5.11", "programBytes": int(out[0]), "appDirBytes": int(out[1]),
                    "programNote": "server-admin-rs（Rust 控制面）+ go-reauth-proxy（Go 网关）；appDirBytes 含界面与 acme.sh 包"}
        except (ValueError, IndexError):
            return {"raw": out}


class Direct(SUT):
    """对照组：没有任何敲门程序与拦截规则，客户端直连源站。只测数据面。"""
    name = "direct"


SUTS = {"direct": Direct, "gately": Gately, "gately-web": GatelyWeb, "fwknop": Fwknop, "fnknock": FnKnock}


# ---------------------------------------------------------------- 测试

def ensure_blocked(sut, cli):
    for _ in range(3):
        sut.revoke()
        for _ in range(20):
            if cli.call(op="state", server=SERVER, port=sut.prot)["state"] == "blocked":
                time.sleep(sut.settle)
                return True
            time.sleep(0.1)
    return False


def knock_series(sut, cli, n, timeout=5, pace=0.0, deadline=None):
    """连续敲门 n 次。deadline 为最晚的开始时刻（洪泛结束前留出单次超时），过了就不再敲，免得在洪泛结束后才敲门。"""
    runs = []
    for i in range(n):
        if deadline and time.time() + sut.settle + 0.2 > deadline:
            break
        if not ensure_blocked(sut, cli):
            runs.append({"ok": False, "err": "撤销后端口仍可访问"})
            continue
        r = cli.call(**sut.knock_cmd(timeout))
        runs.append(r)
        if pace:
            time.sleep(pace)
    return runs


def latency_summary(runs):
    ok = [r for r in runs if r.get("ok")]
    res = {"attempts": len(runs), "ok": len(ok), "total": summarize([r["ms"] for r in ok])}
    if ok and "exCaptchaMs" in ok[0]:
        res["exCaptcha"] = summarize([r["exCaptchaMs"] for r in ok])
        res["afterLogin"] = summarize([r["afterLoginMs"] for r in ok])
        res["login"] = summarize([r["loginMs"] for r in ok])
    if ok and "solveMs" in ok[0]:
        res["captchaSolve"] = summarize([r["solveMs"] for r in ok])
        res["captchaFetch"] = summarize([r["challengeMs"] for r in ok])
    errs = [r.get("err") for r in runs if not r.get("ok")]
    if errs:
        res["errors"] = errs[:5]
    return res


def nmap_probe(sut):
    if not shutil.which("nmap"):
        return "未安装 nmap"
    tcp = ",".join(str(p) for proto, p in sut.probe_ports if proto == "tcp")
    udp = ",".join(str(p) for proto, p in sut.probe_ports if proto == "udp")
    ports = ",".join(x for x in (f"T:{tcp}" if tcp else "", f"U:{udp}" if udp else "") if x)
    scan = "-sS" + (" -sU" if udp else "")
    out = sh(in_ns(NS_C, f"nmap -Pn -n {scan} --max-retries 1 --host-timeout 60s -p {ports} {SERVER} -oG -"), timeout=120)
    return [ln for ln in out.splitlines() if "Ports:" in ln] or out[-300:]


def dataplane(sut, cg, port):
    url = f"http://{SERVER}:{port}/"
    tests = {
        "keepalive": (in_ns(NS_C, f"wrk -t2 -c64 -d{DP_DUR}s --latency {url}"), R.parse_wrk),
        "newconn": (in_ns(NS_C, f"oha --no-tui --disable-keepalive -c 32 -z {DP_DUR}s --output-format json {url}"), R.parse_oha),
    }
    out = {}
    for name, (cmd, parse) in tests.items():
        runs = []
        for i in range(DP_ROUNDS):
            w = start_watch(cg, DP_DUR + 1, f"{WORK}/w.json") if cg else None
            text = sh(cmd, timeout=DP_DUR + 120)
            r = parse(text)
            if w:
                s = end_watch(w, f"{WORK}/w.json")
                r.update({"sutCpuPercent": round(s["cpuPercent"], 1), "peakPssKiB": s["peakPssKiB"]})
            runs.append(r)
            log(sut.name, "dataplane", name, i, json.dumps(r))
            time.sleep(2)
        med = sorted(runs, key=lambda x: x.get("rps", 0))[len(runs) // 2]
        out[name] = {"median": med, "runs": runs}
    return out


def flood_tests(sut, cli, cg):
    out = {}
    for label, launcher in sut.floods():
        for rate in FLOOD_RATES:
            key = f"{label}-{rate}"
            notrack(True)
            before = sut.status() if hasattr(sut, "status") else None
            if sut.kind == "spa":
                # 合法敲门报文在洪泛开始前生成好（洪泛期间不再启动生成容器）
                sut.pool.pkts, sut.pool.at = gen_packets(FLOOD_KNOCKS + 3, sut.keys, None), time.time()
            w = start_watch(cg, FLOOD_DUR - 4, f"{WORK}/fw.json")
            fp = launcher(rate)
            t_flood = time.time()
            time.sleep(3)
            runs = knock_series(sut, cli, FLOOD_KNOCKS, timeout=3, pace=FLOOD_PACE, deadline=t_flood + FLOOD_DUR - 3)
            s = end_watch(w, f"{WORK}/fw.json")
            ftxt = fp.communicate(timeout=FLOOD_DUR + 120)[0]
            notrack(False)
            try:
                fres = json.loads(ftxt[ftxt.index("{"):]) if label.startswith("udp") else R.parse_oha(ftxt)
            except (ValueError, KeyError) as e:
                fres = {"parseError": repr(e), "raw": ftxt[-500:]}
            if not label.startswith("udp"):
                # oha 的 rps 为实际完成的请求速率；状态码分布能看出被测程序如何拒绝
                try:
                    j = json.loads(ftxt[ftxt.index("{"):])
                    fres["statusCodes"] = j.get("statusCodeDistribution")
                    fres["errorDist"] = dict(list(j.get("errorDistribution", {}).items())[:5])
                except ValueError:
                    pass
            r = {"flood": fres, "sutCpuPercent": round(s["cpuPercent"], 1), "peakPssKiB": s["peakPssKiB"],
                 "legitKnock": latency_summary(runs)}
            if before is not None:
                r["statusBefore"], r["statusAfter"] = before, sut.status()
            out[key] = r
            log(sut.name, "flood", key, json.dumps({k: v for k, v in r.items() if k != "legitKnock"}),
                json.dumps(r["legitKnock"].get("total")), r["legitKnock"]["ok"], "/", r["legitKnock"]["attempts"])
            # 等被测程序从洪泛中恢复（积压、限速桶回满）
            time.sleep(8)
    return out


def run_sut(name):
    sut = SUTS[name]()
    res = {"sut": name, "env": {"sutCpus": SUT_CPUS, "loadCpus": LOAD_CPUS, "kernel": os.uname().release,
                                "arch": os.uname().machine}}
    if name == "direct":
        res["dataplane"] = dataplane(sut, None, PROT_PORT)
        return res
    sut.stop()
    fw_cleanup()
    sh("sync; echo 3 > /proc/sys/vm/drop_caches")
    sut.start()
    res["startupSec"] = sut.ready_sec
    cg = cgroup_of(sut.container)
    cli = Client()
    try:
        time.sleep(10)
        res["idle"] = snapshot(cg)
        res["image"] = sut.image
        res["imageBytes"] = int(sh(f"docker image inspect -f '{{{{.Size}}}}' {sut.image}").strip() or 0)
        res["info"] = sut.info()
        log(name, "idle", json.dumps(res["idle"]), json.dumps(res["info"]))
        # 隐蔽性：未敲门时从公网地址探测
        sut.revoke()
        res["stealth"] = {"probe": cli.call(op="probe", server=SERVER, ports=sut.probe_ports), "nmap": nmap_probe(sut)}
        log(name, "stealth", json.dumps(res["stealth"], ensure_ascii=False))
        # 敲门到放行延迟
        runs = knock_series(sut, cli, N_LAT)
        res["latency"] = latency_summary(runs)
        res["latency"]["runs"] = runs
        log(name, "latency", json.dumps({k: v for k, v in res["latency"].items() if k != "runs"}, ensure_ascii=False))
        # 抗洪泛
        res["flood"] = flood_tests(sut, cli, cg)
        res["afterFlood"] = snapshot(cg)
        # 放行后的数据面
        ensure_blocked(sut, cli)
        r = cli.call(**sut.knock_cmd(5))
        if not r.get("ok"):
            raise RuntimeError(f"数据面测试前敲门失败: {r}")
        res["dataplane"] = dataplane(sut, cg, sut.prot)
        time.sleep(5)
        res["idleAfter"] = snapshot(cg)
    finally:
        cli.close()
        sut.stop()
    return res


def main():
    names = sys.argv[1:] or ["direct", "gately", "gately-web", "fwknop", "fnknock"]
    for n in names:
        if n not in SUTS:
            sys.exit(f"未知被测名 {n}，可选 {' '.join(SUTS)}")
    os.makedirs(WORK, exist_ok=True)
    os.makedirs(RESULTS, exist_ok=True)
    setup_net()
    try:
        start_origin()
        for n in names:
            log("=====", n)
            random.seed(1)
            res = run_sut(n)
            with open(os.path.join(RESULTS, f"knock-{n}.json"), "w") as f:
                json.dump(res, f, indent=1, ensure_ascii=False)
            log("DONE", n)
    finally:
        notrack(False)
        fw_cleanup()
        sh("docker rm -f knock-origin 2>/dev/null")
        teardown_net()


if __name__ == "__main__":
    main()
