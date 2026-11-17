#!/usr/bin/env python3
"""入侵防护专项：Gately 入侵防护模块 vs fail2ban（在压测虚拟机上以 root 运行，由 run.sh 调用）。

用法：guard.py <被测> [测试列表]
  被测：gately（只开入侵防护模块）、fail2ban（systemd 服务）、
        gately-full（默认开启全部模块，只测空闲内存）、gately-web（Web 服务内置路径探测，只测封禁延迟与空闲内存）
  测试：idle,latency,stream,big（逗号分隔，默认全部；gately-full 只有 idle，gately-web 只有 idle,latency）

场景：同一份 nginx combined 格式访问日志 /var/log/bxguard/access.log，规则为 60 秒内 5 次 404 即封禁（封禁 1 小时），
封禁写入 nftables 集合。日志由 inject.py 按指定速率直接追加（日志注入法），每个攻击者恰好写 5 行 404。
“封禁时刻”以 `nft monitor new elements` 收到该 IP 加入集合的通知为准（每秒再列一次集合兜底），
“达到阈值时刻”为攻击者第 5 行写入日志文件（write 返回）的时刻，两者都是本机墙钟。

CPU 划分与 run.py 一致：被测程序 vCPU 0,1；本脚本、注入器与 nft monitor 由 run.sh 固定在 vCPU 4-7。
CPU 与内存按 cgroup 采样（lib/sample.py 的口径：PSS 之和、cpu.stat usage_usec 差值），100% 表示占满 1 个 vCPU。
结果写到 results/guard-<被测>.json。
"""
import datetime
import json
import os
import random
import re
import shutil
import statistics
import subprocess
import sys
import threading
import time

HERE = os.path.dirname(os.path.abspath(__file__))
BASE = os.path.dirname(HERE)
LIB = os.path.join(BASE, "lib")
sys.path.insert(0, LIB)
import sample  # noqa: E402  lib/sample.py：PSS 与 CPU 采样

from inject import attacker_ip  # noqa: E402

LOG_DIR = "/var/log/bxguard"
LOG = os.path.join(LOG_DIR, "access.log")
SUT_CPUS = os.environ.get("SUT_CPUS", "0,1")
IMAGE = os.environ.get("GATELY_IMAGE", "ghcr.io/wangwanjie/gately:v0.6.8.1")
# 规则参数（两边相同）
MAXRETRY, FINDTIME, BANTIME = 5, 60, 3600
# 测试参数，可用环境变量缩短（本机调试时用）
RATES = [int(x) for x in os.environ.get("GUARD_RATES", "1000,5000,20000").split(",")]
STREAM_SEC = float(os.environ.get("GUARD_STREAM_SEC", "60"))
MAL_FRAC = float(os.environ.get("GUARD_MAL_FRAC", "0.02"))
LAT_SAMPLES = int(os.environ.get("GUARD_LAT_SAMPLES", "60"))
BIG_ATTACKERS = int(os.environ.get("GUARD_BIG_ATTACKERS", "10000"))
BIG_RATE = int(os.environ.get("GUARD_BIG_RATE", "5000"))
# 注入结束后等待封禁追平的上限（秒）与“无进展”判定时间
STREAM_DRAIN_CAP = int(os.environ.get("GUARD_STREAM_DRAIN_CAP", "600"))
BIG_DRAIN_CAP = int(os.environ.get("GUARD_BIG_DRAIN_CAP", "1200"))
NO_PROGRESS_SEC = 30
IDLE_WAIT = int(os.environ.get("GUARD_IDLE_WAIT", "30"))
# Gately Web 探测补充测试的监听端口
WEB_PORT = 18480

# Gately v0.6.8.1 的全部功能模块（除入侵防护外），只开入侵防护时全部写入 disabledModules
GATELY_MODULES = ["backup", "cloudflared", "cron", "ddns", "docker", "easytier", "filebrowser", "frp", "ftp",
                  "knock", "netbird", "nettools", "notify", "perf", "portforward", "protoroute", "smb", "ssl",
                  "storage", "stun", "tailscale", "waf", "webdav", "webservice", "webterminal", "wol"]

GATELY_FILTER = r'^<HOST> \S+ \S+ \[[^\]]+\] "[A-Z]+ [^"]*" 404\s'


def sh(cmd, check=False, timeout=None):
    p = subprocess.run(cmd, shell=True, capture_output=True, text=True, timeout=timeout)
    if check and p.returncode != 0:
        raise RuntimeError(f"{cmd}\n{p.stdout}\n{p.stderr}")
    return (p.stdout + p.stderr).strip()


def log(*a):
    print(time.strftime("%H:%M:%S"), *a, flush=True)


def pct(vals, p):
    """最近秩法分位数。"""
    if not vals:
        return None
    s = sorted(vals)
    k = max(0, min(len(s) - 1, int(round(p / 100 * len(s) + 0.5)) - 1))
    return s[k]


def lat_stats(vals):
    """延迟统计（输入秒，输出毫秒）。"""
    if not vals:
        return {"n": 0}
    ms = [v * 1000 for v in vals]
    return {"n": len(ms), "p50ms": round(pct(ms, 50), 1), "p90ms": round(pct(ms, 90), 1),
            "p99ms": round(pct(ms, 99), 1), "maxms": round(max(ms), 1), "minms": round(min(ms), 1),
            "meanms": round(statistics.mean(ms), 1)}


def nft_set_members(family, table, setname):
    """列出集合当前元素（IPv4 字符串集合）；表或集合不存在时返回 None。"""
    p = subprocess.run(["nft", "-j", "list", "set", family, table, setname], capture_output=True, text=True)
    if p.returncode != 0:
        return None
    out = set()
    for item in json.loads(p.stdout).get("nftables", []):
        st = item.get("set")
        if not st:
            continue
        for e in st.get("elem", []):
            if isinstance(e, str):
                out.add(e)
            elif isinstance(e, dict):
                v = e.get("elem", {}).get("val")
                if isinstance(v, str):
                    out.add(v)
    return out


class BanWatcher:
    """记录 IP 首次出现在指定 nftables 集合中的时刻（单调时钟与墙钟各一份）。

    主路径：`nft monitor new elements` 的逐元素通知（毫秒内送达）；
    兜底：每秒列一次集合，monitor 漏掉（例如 netlink 缓冲溢出）的元素按列出时刻记录，并单独计数。
    """

    RE = re.compile(r"^add element (\S+) (\S+) (\S+) \{ ([0-9a-fA-F:.]+)")

    def __init__(self, family, table, setname):
        self.key = (family, table, setname)
        self.lock = threading.Lock()
        self.seen = {}
        self.polled = set()
        self.errors = []
        self.stop_ev = threading.Event()
        self.p = subprocess.Popen(["nft", "monitor", "new", "elements"], stdout=subprocess.PIPE,
                                  stderr=subprocess.STDOUT, text=True, bufsize=1)
        threading.Thread(target=self._read, daemon=True).start()
        threading.Thread(target=self._poll, daemon=True).start()
        time.sleep(0.5)

    def _read(self):
        for line in self.p.stdout:
            t = (time.monotonic(), time.time())
            m = self.RE.match(line)
            if not m:
                # 其他集合的多行元素会折行，只记录真正的错误
                if "rror" in line or "No buffer" in line:
                    self.errors.append(line.strip()[:200])
                continue
            if (m.group(1), m.group(2), m.group(3)) != self.key:
                continue
            with self.lock:
                self.seen.setdefault(m.group(4), t)

    def _poll(self):
        while not self.stop_ev.wait(1.0):
            mem = nft_set_members(*self.key)
            if not mem:
                continue
            t = (time.monotonic(), time.time())
            with self.lock:
                for ip in mem:
                    if ip not in self.seen:
                        self.seen[ip] = t
                        self.polled.add(ip)

    def get(self, ip):
        """返回首次出现的单调时钟时刻，未出现时为 None。"""
        with self.lock:
            v = self.seen.get(ip)
        return v[0] if v else None

    def snapshot(self):
        """返回 {ip: (单调时钟, 墙钟)}。"""
        with self.lock:
            return dict(self.seen)

    def close(self):
        self.stop_ev.set()
        self.p.terminate()
        try:
            self.p.wait(5)
        except subprocess.TimeoutExpired:
            self.p.kill()


class Sampler:
    """每 0.5 秒采样一次 cgroup 的 PSS 与累计 CPU（时间用单调时钟）。"""

    def __init__(self, cg):
        self.cg = cg
        self.samples = []  # (时间, pssKiB, cpuUsec)
        self.stop_ev = threading.Event()
        self.th = threading.Thread(target=self._run, daemon=True)
        self.th.start()

    def _run(self):
        while True:
            try:
                pss, _rss, _cur = sample.mem(self.cg)
                self.samples.append((time.monotonic(), pss, sample.cpu_usec(self.cg)))
            except (FileNotFoundError, OSError):
                pass
            if self.stop_ev.wait(0.5):
                return

    def stop(self):
        self.stop_ev.set()
        self.th.join()

    def window(self, t0, t1):
        """[t0, t1] 内的峰值 PSS、CPU 秒数与平均 CPU%。"""
        ss = [s for s in self.samples if t0 <= s[0] <= t1]
        if len(ss) < 2:
            return {}
        cpu = (ss[-1][2] - ss[0][2]) / 1e6
        dur = ss[-1][0] - ss[0][0]
        return {"peakPssKiB": max(s[1] for s in ss), "cpuSeconds": round(cpu, 2),
                "cpuPercent": round(cpu / dur * 100, 1), "seconds": round(dur, 1)}

    def busy_until(self, after, idle_pct=5.0):
        """after 之后 CPU 回落到 idle_pct 以下（连续 2 秒）的时刻，用于估计积压处理完毕的时间。"""
        ss = [s for s in self.samples if s[0] >= after]
        for i in range(len(ss)):
            j = i
            while j < len(ss) and ss[j][0] - ss[i][0] < 2.0:
                j += 1
            if j >= len(ss):
                return None
            used = (ss[j][2] - ss[i][2]) / 1e6 / (ss[j][0] - ss[i][0]) * 100
            if used < idle_pct:
                return ss[i][0]
        return None

    def cpu_now(self, span=5.0):
        ss = self.samples
        if len(ss) < 2:
            return 100.0
        last = ss[-1]
        first = next((s for s in ss if last[0] - s[0] <= span), ss[0])
        if last[0] - first[0] < 1:
            return 100.0
        return (last[2] - first[2]) / 1e6 / (last[0] - first[0]) * 100


def docker_cgroup(name):
    cid = sh(f"docker inspect -f '{{{{.Id}}}}' {name}")
    for path in (f"/sys/fs/cgroup/system.slice/docker-{cid}.scope", f"/sys/fs/cgroup/docker/{cid}"):
        if os.path.isdir(path):
            return path
    raise RuntimeError("找不到容器 cgroup: " + cid)


def read_cpuset(cg):
    try:
        with open(os.path.join(cg, "cpuset.cpus.effective")) as f:
            return f.read().strip()
    except OSError:
        return ""


# ---------------------------------------------------------------- 被测程序


class Gately:
    """Gately 官方镜像，host 网络 + NET_ADMIN，绑定 vCPU 0,1。"""

    family, table, setname = "inet", "gately", "ban4"
    name_prefix = "bx-guard-gately"
    cfg = "/tmp/bx-guard-gately"

    def __init__(self, variant):
        # variant：guard 只开入侵防护；full 默认全部模块；web 开 Web 服务并用内置路径探测规则
        self.variant = variant
        self.name = {"guard": "gately", "full": "gately-full", "web": "gately-web"}[variant]
        self.cg = None

    def prepare(self):
        sh(f"docker image inspect {IMAGE} >/dev/null", check=True)

    def write_config(self):
        shutil.rmtree(self.cfg, ignore_errors=True)
        os.makedirs(self.cfg)
        # 先空跑一次生成默认配置（管理员账号、密钥等），再写入被测规则
        sh(f"docker run --rm --entrypoint sh -v {self.cfg}:/etc/gately {IMAGE} -c 'timeout 3 gately >/dev/null 2>&1; true'")
        gp = os.path.join(self.cfg, "gately.json")
        g = json.load(open(gp)) if os.path.exists(gp) else {}
        if self.variant == "guard":
            g["disabledModules"] = list(GATELY_MODULES)
        elif self.variant == "web":
            g["disabledModules"] = [m for m in GATELY_MODULES if m != "webservice"]
        else:
            g["disabledModules"] = []
        json.dump(g, open(gp, "w"), indent=2)
        if self.variant == "web":
            jail = {"key": "bx404", "name": "bx-404", "enable": True, "source": "web", "target": "",
                    "filter": "", "ignore": "", "maxRetry": MAXRETRY, "findTime": FINDTIME, "banTime": BANTIME,
                    "remark": "压测：Web 服务内置路径探测，404 计数"}
            wp = os.path.join(self.cfg, "webservice.json")
            ws = json.load(open(wp)) if os.path.exists(wp) else {"settings": {}}
            ws.setdefault("settings", {})
            ws["settings"]["enable"] = True
            # 从 127.0.0.1 来的请求按 X-Forwarded-For 取访客地址，攻击脚本借此伪造大量来源
            ws["settings"]["realIPFrom"] = ["127.0.0.1/32", "::1/128"]
            ws["settings"]["realIPHeaders"] = ["X-Forwarded-For"]
            # 没有站点、默认站点未启用：所有请求返回 404 并交给路径探测规则计数
            ws["listeners"] = [{"key": "l-bx", "name": "bx", "enable": True, "network": "tcp", "listenIp": "127.0.0.1",
                                "port": WEB_PORT, "tls": False, "sites": [],
                                "default": {"key": "d-bx", "name": "default", "enable": False, "type": "close"}}]
            json.dump(ws, open(wp, "w"), indent=2)
        else:
            jail = {"key": "bx404", "name": "bx-404", "enable": True, "source": "file", "target": LOG,
                    "filter": GATELY_FILTER, "ignore": "", "maxRetry": MAXRETRY, "findTime": FINDTIME,
                    "banTime": BANTIME, "remark": "压测：nginx 访问日志 404"}
        guard = {"settings": {"firewall": True, "banSeconds": BANTIME, "increment": False, "maxBanSeconds": 7 * 86400,
                              "ignorePrivate": True, "ignoreList": [],
                              "trap": {"enable": False, "ports": "", "banSeconds": 0},
                              "scan": {"enable": False, "threshold": 12, "excludePorts": "", "banSeconds": 0,
                                       "ipv6Strict": False}},
                 "jails": [jail]}
        json.dump(guard, open(os.path.join(self.cfg, "guard.json"), "w"), indent=2)
        # 跳过首次登录的使用场景向导（不影响服务）
        json.dump({"scenePending": False}, open(os.path.join(self.cfg, "onboarding.json"), "w"))

    def start(self):
        self.stop()
        self.write_config()
        self.launched = time.monotonic()
        sh(f"docker run -d --name {self.name_prefix} --network host --cpuset-cpus {SUT_CPUS} --cap-add NET_ADMIN "
           f"-v {self.cfg}:/etc/gately -v {LOG_DIR}:{LOG_DIR}:ro {IMAGE}", check=True)
        self.cg = docker_cgroup(self.name_prefix)

    def stop(self):
        sh(f"docker stop -t 15 {self.name_prefix}; docker rm -f {self.name_prefix}")
        sh(f"nft delete table {self.family} {self.table}")

    def finish(self):
        self.stop()

    def decisions(self):
        """从 Gately 入侵防护日志（JSON Lines，UTC 纳秒时间）取每个 IP 的封禁决定时刻。
        日志约 2 MB 轮转一次（guard.log.1 …），按从旧到新的顺序全部读取。"""
        out = {}
        d = os.path.join(self.cfg, "logs")
        paths = sorted((p for p in os.listdir(d) if p.startswith("guard.log")),
                       key=lambda p: -int(p.rsplit(".", 1)[1]) if p[-1].isdigit() else 0) if os.path.isdir(d) else []
        for line in (ln for p in paths for ln in open(os.path.join(d, p), encoding="utf-8", errors="replace")):
            if "封禁 " not in line:
                continue
            try:
                j = json.loads(line)
            except ValueError:
                continue
            m = re.match(r"封禁 (\S+) 至", j.get("message", ""))
            if not m:
                continue
            ts = j["time"]
            # 纳秒截到微秒再解析
            mm = re.match(r"(\d{4}-\d\d-\d\dT\d\d:\d\d:\d\d)(?:\.(\d+))?(Z|[+-]\d\d:\d\d)$", ts)
            if not mm:
                continue
            frac = (mm.group(2) or "0")[:6].ljust(6, "0")
            tz = "+00:00" if mm.group(3) == "Z" else mm.group(3)
            dt = datetime.datetime.fromisoformat(f"{mm.group(1)}.{frac}{tz}")
            out.setdefault(m.group(1), dt.timestamp())
        return out

    def info(self):
        size = sh(f"docker run --rm --entrypoint stat {IMAGE} -c %s /usr/local/bin/gately")
        img = sh(f"docker image inspect -f '{{{{.Size}}}}' {IMAGE}")
        ver = sh(f"docker run --rm --entrypoint gately {IMAGE} -version")
        nft = sh(f"docker run --rm --entrypoint nft {IMAGE} --version")
        return {"version": ver, "image": IMAGE, "binaryBytes": int(size or 0), "imageBytes": int(img or 0),
                "nftInImage": nft, "variant": self.variant}


class Fail2ban:
    """Debian 13 apt 安装的 fail2ban（systemd 服务），AllowedCPUs 限制在 vCPU 0,1。"""

    family, table, setname = "inet", "f2b-table", "addr-set-bx-404"
    name = "fail2ban"
    unit = "fail2ban.service"
    f2b_log = "/var/log/fail2ban.log"
    cg_path = "/sys/fs/cgroup/system.slice/fail2ban.service"

    def __init__(self):
        self.cg = None

    def prepare(self):
        if not shutil.which("fail2ban-client"):
            raise RuntimeError("未安装 fail2ban：apt-get install -y fail2ban python3-pyinotify python3-systemd")
        sh("python3 -c 'import pyinotify'", check=True)
        shutil.copy(os.path.join(HERE, "fail2ban", "bx-404.conf"), "/etc/fail2ban/filter.d/bx-404.conf")
        shutil.copy(os.path.join(HERE, "fail2ban", "bx-guard.local"), "/etc/fail2ban/jail.d/zz-bx-guard.local")
        sh(f"systemctl stop {self.unit}")
        # 只在本次开机内生效（--runtime），重启后恢复
        sh(f"systemctl set-property --runtime {self.unit} AllowedCPUs={SUT_CPUS}", check=True)

    def start(self):
        self.stop()
        # 删除持久化的封禁数据库，保证每次从空状态开始（默认 sqlite 数据库，保持 Debian 默认配置）
        sh("rm -f /var/lib/fail2ban/fail2ban.sqlite3")
        self.log_offset = os.path.getsize(self.f2b_log) if os.path.exists(self.f2b_log) else 0
        self.launched = time.monotonic()
        sh(f"systemctl start {self.unit}", check=True)
        self.cg = self.cg_path
        t0 = time.monotonic()
        while time.monotonic() - t0 < 60:
            if "pong" in sh("fail2ban-client ping"):
                return
            time.sleep(0.2)
        raise RuntimeError("fail2ban 未就绪")

    def stop(self):
        sh(f"systemctl stop {self.unit}", timeout=300)
        sh(f"nft delete table {self.family} {self.table}")

    def decisions(self):
        """从 /var/log/fail2ban.log（本地时间，毫秒）取本次启动以来每个 IP 的 Ban 时刻。"""
        out = {}
        if not os.path.exists(self.f2b_log):
            return out
        with open(self.f2b_log, encoding="utf-8", errors="replace") as f:
            f.seek(self.log_offset)
            for line in f:
                m = re.match(r"(\d{4}-\d\d-\d\d \d\d:\d\d:\d\d),(\d{3}) .* Ban (\S+)\s*$", line)
                if m:
                    t = time.mktime(time.strptime(m.group(1), "%Y-%m-%d %H:%M:%S")) + int(m.group(2)) / 1000
                    out.setdefault(m.group(3), t)
        return out

    def finish(self):
        self.stop()
        # 压测结束后不让它随开机启动，避免影响其他专项
        sh(f"systemctl disable {self.unit}")

    def info(self):
        ver = sh("fail2ban-client --version")
        pkgs = sh("dpkg-query -W -f='${Package} ${Version} ${Installed-Size}\\n' fail2ban python3-pyinotify python3-systemd")
        return {"version": ver, "packages": pkgs.splitlines(), "install": fail2ban_install_size(),
                "python": sh("python3 --version"), "nft": sh("nft --version")}


def fail2ban_install_size():
    """fail2ban 的安装体积（dpkg Installed-Size，KiB）。

    取 fail2ban + python3-pyinotify + python3-systemd 的递归依赖闭包（不含推荐包）中已安装的包，分三类：
    - fail2banKiB：fail2ban 包本身；
    - pythonKiB：fail2ban 与 Python 运行时（包名以 python3 / libpython3 开头），没有 Python 的系统装 fail2ban 时至少要装这些；
    - sharedLibsKiB：闭包中其余的共享库（libc6、libssl3、libsqlite3 等，大多数系统已经装有，仅供参考）。
    """
    roots = "fail2ban python3-pyinotify python3-systemd"
    out = sh("apt-cache depends --recurse --installed --no-recommends --no-suggests --no-conflicts --no-breaks "
             f"--no-replaces --no-enhances {roots}")
    names = set()
    for line in out.splitlines():
        line = line.strip()
        if not line or line.startswith("<"):
            continue
        if ":" in line:
            line = line.split(":", 1)[1].strip()
        if line.startswith("<") or not re.match(r"^[a-z0-9][a-z0-9+.-]+$", line):
            continue
        names.add(line)
    core, libs = {}, {}
    for n in sorted(names):
        q = sh(f"dpkg-query -W -f='${{db:Status-Status}}|${{Installed-Size}}' {n} 2>/dev/null")
        parts = q.split("|")
        if len(parts) != 2 or parts[0] != "installed":
            continue
        size = int(parts[1] or 0)
        if n == "fail2ban" or n.startswith("python3") or n.startswith("libpython3"):
            core[n] = size
        else:
            libs[n] = size
    return {"fail2banKiB": core.get("fail2ban", 0), "pythonKiB": sum(core.values()), "pythonPkgs": core,
            "sharedLibsKiB": sum(libs.values()), "sharedLibPkgs": libs}


# ---------------------------------------------------------------- 测试


def split_stats(thresholds, seen, decisions):
    """把封禁延迟拆成两段：检测（达到阈值 → 程序日志里记下封禁决定）与同步（决定 → 出现在 nftables 集合）。
    决定时刻来自各自日志里的墙钟时间戳（Gately 纳秒、fail2ban 毫秒），只能用墙钟对照，墙钟被调整时会有偏差，只作参考。
    thresholds 为 {ip: (单调, 墙钟)}，seen 为 BanWatcher.snapshot()。"""
    det, syn = [], []
    for ip, t in thresholds.items():
        d = decisions.get(ip)
        if d is None:
            continue
        det.append(d - t[1])
        if ip in seen:
            syn.append(seen[ip][1] - d)
    return {"detect": lat_stats(det), "sync": lat_stats(syn)}


def reset_log():
    os.makedirs(LOG_DIR, exist_ok=True)
    with open(LOG, "w"):
        pass


def write_attack(fd, ip, n=MAXRETRY, gap=0.0):
    """为 ip 写 n 行 404，返回最后一行开始写入的时刻（write 只需几微秒；取写入前的时刻，
    避免本进程在 write 返回后被调度出去而把等待时间算成负延迟）。"""
    t = (0.0, 0.0)
    for i in range(n):
        ts = time.strftime("%d/%b/%Y:%H:%M:%S %z")
        t = (time.monotonic(), time.time())
        os.write(fd, f'{ip} - - [{ts}] "GET /wp-login.php HTTP/1.1" 404 153 "-" "bx-scan"\n'.encode())
        if gap and i < n - 1:
            time.sleep(gap)
    return t


def http_attack(ip, n=MAXRETRY, gap=0.0):
    """对 Gately Web 监听发 n 个请求（X-Forwarded-For 为 ip），返回发出最后一个请求前的时刻
    （与日志注入一致：以第 n 次失败发生的时刻为起点，延迟包含这个请求本身的处理）。"""
    import http.client
    c = http.client.HTTPConnection("127.0.0.1", WEB_PORT, timeout=5)
    t = (0.0, 0.0)
    for i in range(n):
        t = (time.monotonic(), time.time())
        c.request("GET", "/wp-login.php", headers={"Host": "bx.test", "X-Forwarded-For": ip})
        c.getresponse().read()
        if gap and i < n - 1:
            time.sleep(gap)
    c.close()
    return t


class Session:
    """一次被测程序的运行：启动、就绪确认（金丝雀封禁）、监视封禁、采样。"""

    canary_n = 0

    def __init__(self, sut, web=False):
        self.sut, self.web = sut, web

    def __enter__(self):
        reset_log()
        self.sut.start()
        t0 = self.sut.launched
        self.watch = BanWatcher(self.sut.family, self.sut.table, self.sut.setname)
        self.sampler = Sampler(self.sut.cg)
        self.canaries = set()
        try:
            self.ready_sec = self._ready(t0)
        except Exception:
            self.__exit__(None, None, None)
            raise
        return self

    def _ready(self, t0, timeout=180):
        """反复写金丝雀攻击（198.19.255.x），直到它出现在集合里：说明读日志、计数、写 nftables 全链路已就绪。"""
        fd = os.open(LOG, os.O_WRONLY | os.O_APPEND)
        try:
            while time.monotonic() - t0 < timeout:
                Session.canary_n += 1
                ip = f"198.19.255.{Session.canary_n % 250 + 1}"
                self.canaries.add(ip)
                if self.web:
                    try:
                        http_attack(ip)
                    except OSError:
                        time.sleep(1)
                        continue
                else:
                    write_attack(fd, ip)
                t1 = time.monotonic()
                while time.monotonic() - t1 < 5:
                    if self.watch.get(ip):
                        return round(time.monotonic() - t0, 2)
                    time.sleep(0.05)
        finally:
            os.close(fd)
        raise RuntimeError(f"{self.sut.name} 在 {timeout} 秒内未完成金丝雀封禁")

    def __exit__(self, *exc):
        self.sampler.stop()
        self.watch.close()
        self.sut.stop()


def test_idle(sut, web=False):
    with Session(sut, web) as s:
        log(sut.name, "就绪（金丝雀封禁完成）", s.ready_sec, "秒；空闲等待", IDLE_WAIT, "秒")
        time.sleep(IDLE_WAIT)
        snaps = []
        for _ in range(3):
            snaps.append(sample.snapshot(sut.cg))
            time.sleep(1)
        t1 = time.monotonic()
        w = s.sampler.window(t1 - IDLE_WAIT, t1)
        res = {"readySec": s.ready_sec, "idle": snaps[-1], "idlePssKiB": round(statistics.mean(x["pssKiB"] for x in snaps)),
               "idleCpuPercent": w.get("cpuPercent"), "cpuset": read_cpuset(sut.cg)}
        log(sut.name, "idle", json.dumps(res, ensure_ascii=False))
        return res


def test_latency(sut, web=False, ip_start=0):
    """顺序产生 LAT_SAMPLES 个攻击者，每个间隔 20 毫秒写 5 行（或发 5 个请求），测达到阈值到进入集合的时间。"""
    rnd = random.Random(7)
    with Session(sut, web) as s:
        lats, missed, thr = [], [], {}
        fd = os.open(LOG, os.O_WRONLY | os.O_APPEND)
        for i in range(LAT_SAMPLES):
            ip = attacker_ip(ip_start + i)
            t0 = http_attack(ip, gap=0.02) if web else write_attack(fd, ip, gap=0.02)
            thr[ip] = t0
            while time.monotonic() - t0[0] < 10:
                tb = s.watch.get(ip)
                if tb:
                    lats.append(tb - t0[0])
                    break
                time.sleep(0.002)
            else:
                missed.append(ip)
            # 随机间隔，避免与被测程序的轮询周期同相
            time.sleep(rnd.uniform(0.3, 1.3))
        os.close(fd)
        res = lat_stats(lats)
        time.sleep(1)
        res.update({"missed": len(missed), "polledFallback": len(s.watch.polled - s.canaries),
                    "monitorErrors": s.watch.errors[:5]})
        res.update(split_stats(thr, s.watch.snapshot(), sut.decisions()))
        log(sut.name, "latency", json.dumps(res, ensure_ascii=False))
        return res


def dump_raw(sut, label, expected, seen, decisions):
    """逐 IP 原始数据：[ip, 达到阈值(单调,秒), 进入集合(单调,秒或 null), 决定-阈值(墙钟,秒或 null)]，便于事后分析。"""
    rows = []
    for ip, t in expected.items():
        b = seen.get(ip)
        d = decisions.get(ip)
        rows.append([ip, round(t[0], 4), round(b[0], 4) if b else None, round(d - t[1], 4) if d is not None else None])
    rows.sort(key=lambda r: r[1])
    path = os.path.join(BASE, "results", f"guard-{sut.name}-raw-{label}.json")
    os.makedirs(os.path.dirname(path), exist_ok=True)
    with open(path, "w") as f:
        json.dump(rows, f)


def run_stream(sut, rate, duration, mal_frac, attackers, ip_start, drain_cap, label):
    """以 rate 行/秒注入日志，统计应封/已封/漏封/误封、封禁延迟、CPU 与内存。"""
    out = "/tmp/bx-guard-inject.json"
    with Session(sut) as s:
        time.sleep(3)
        base = s.sampler.window(time.monotonic() - 3, time.monotonic())
        cmd = [sys.executable, os.path.join(HERE, "inject.py"), "--log", LOG, "--rate", str(rate),
               "--duration", str(duration), "--mal-frac", str(mal_frac), "--maxretry", str(MAXRETRY),
               "--ip-start", str(ip_start), "--attackers", str(attackers), "--out", out]
        t_start = time.monotonic()
        subprocess.run(cmd, check=True)
        inj = json.load(open(out))
        t_end = inj["monoEnd"]
        expected = inj["thresholds"]
        log(sut.name, f"注入完成 {inj['lines']} 行（实际 {inj['achievedRate']:.0f} 行/秒），应封 {len(expected)}，"
                      f"等待封禁追平（上限 {drain_cap} 秒）")
        # 等待：全部封完，或 NO_PROGRESS_SEC 内没有新封禁且 CPU 已回落，或到达上限
        last_n, last_change = -1, time.monotonic()
        while time.monotonic() - t_end < drain_cap:
            seen = s.watch.snapshot()
            n = sum(1 for ip in expected if ip in seen)
            if n != last_n:
                last_n, last_change = n, time.monotonic()
            if n == len(expected) and time.monotonic() - t_end > 3:
                break
            if time.monotonic() - last_change > NO_PROGRESS_SEC and s.sampler.cpu_now() < 5:
                break
            time.sleep(1)
        time.sleep(2)
        t_done = time.monotonic()
        seen = s.watch.snapshot()
        final = nft_set_members(sut.family, sut.table, sut.setname) or set()
        lats = [seen[ip][0] - t[0] for ip, t in expected.items() if ip in seen]
        banned_times = [seen[ip][0] for ip in expected if ip in seen]
        partial = set(inj["partial"])
        false_bans = [ip for ip in seen if ip not in expected and ip not in s.canaries]
        t_list = time.monotonic()
        nft_set_members(sut.family, sut.table, sut.setname)
        list_sec = time.monotonic() - t_list
        busy = s.sampler.busy_until(t_end)
        decisions = sut.decisions()
        parts = split_stats(expected, seen, decisions)
        dump_raw(sut, label, expected, seen, decisions)
        res = {
            "rate": rate, "achievedRate": round(inj["achievedRate"]), "lines": inj["lines"], "malLines": inj["malLines"],
            "injectSec": round(t_end - inj["monoStart"], 2),
            "expected": len(expected), "banned": len(banned_times), "missed": len(expected) - len(banned_times),
            "inSetAtEnd": len(final & set(expected)), "falseBans": len(false_bans),
            "partialBanned": len([ip for ip in false_bans if ip in partial]),
            "latency": lat_stats(lats), "detect": parts["detect"], "sync": parts["sync"],
            "lastBanAfterInjectEndSec": round(max(banned_times) - inj["monoEnd"], 2) if banned_times else None,
            "allBannedSec": round(max(banned_times) - inj["monoStart"], 2) if banned_times and len(banned_times) == len(expected) else None,
            "cpuBusyAfterInjectEndSec": round(busy - t_end, 1) if busy else None,
            "inject": s.sampler.window(inj["monoStart"], inj["monoEnd"]),
            "total": s.sampler.window(t_start, t_done),
            "baselineCpuPercent": base.get("cpuPercent"),
            "polledFallback": len(s.watch.polled - s.canaries), "monitorErrors": s.watch.errors[:5],
            "nftListSetSec": round(list_sec, 3), "readySec": s.ready_sec,
        }
        return res, inj["nextIp"]


def main():
    name = sys.argv[1]
    only = sys.argv[2].split(",") if len(sys.argv) > 2 else None
    web = name == "gately-web"
    if name == "fail2ban":
        sut = Fail2ban()
        tests = ["idle", "latency", "stream", "big"]
    elif name == "gately":
        sut = Gately("guard")
        tests = ["idle", "latency", "stream", "big"]
    elif name == "gately-full":
        sut = Gately("full")
        tests = ["idle"]
    elif name == "gately-web":
        sut = Gately("web")
        tests = ["idle", "latency"]
    else:
        raise SystemExit("未知被测：" + name)
    if only:
        tests = [t for t in tests if t in only]
    sut.prepare()
    res = {"sut": name, "params": {"maxretry": MAXRETRY, "findtime": FINDTIME, "bantime": BANTIME, "rates": RATES,
                                   "streamSec": STREAM_SEC, "malFrac": MAL_FRAC, "latSamples": LAT_SAMPLES,
                                   "bigAttackers": BIG_ATTACKERS, "bigRate": BIG_RATE, "sutCpus": SUT_CPUS},
           "info": sut.info(), "tests": {}}
    log(name, "info", json.dumps(res["info"], ensure_ascii=False)[:600])
    ip_next = 0
    try:
        if "idle" in tests:
            res["tests"]["idle"] = test_idle(sut, web)
        if "latency" in tests:
            res["tests"]["latency"] = test_latency(sut, web, ip_start=ip_next)
            ip_next += LAT_SAMPLES
        if "stream" in tests:
            res["tests"]["stream"] = {}
            for rate in RATES:
                r, ip_next = run_stream(sut, rate, STREAM_SEC, MAL_FRAC, 0, ip_next, STREAM_DRAIN_CAP, f"stream{rate}")
                res["tests"]["stream"][str(rate)] = r
                log(name, f"stream {rate}", json.dumps(r, ensure_ascii=False))
        if "big" in tests:
            r, ip_next = run_stream(sut, BIG_RATE, 0, 1.0, BIG_ATTACKERS, ip_next, BIG_DRAIN_CAP, "big")
            res["tests"]["big"] = r
            log(name, "big", json.dumps(r, ensure_ascii=False))
    finally:
        sut.finish()
        if os.path.exists(LOG):
            os.remove(LOG)
    os.makedirs(os.path.join(BASE, "results"), exist_ok=True)
    suffix = "" if not only else "-" + "-".join(only)
    path = os.path.join(BASE, "results", f"guard-{name}{suffix}.json")
    with open(path, "w") as f:
        json.dump(res, f, indent=1, ensure_ascii=False)
    log("DONE", name, path)


if __name__ == "__main__":
    main()
