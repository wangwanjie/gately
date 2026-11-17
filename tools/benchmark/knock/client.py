#!/usr/bin/env python3
"""敲门专项的客户端：在客户端网络命名空间里常驻运行（由 bench.py 以 ip netns exec + taskset 启动），
从标准输入逐行读取 JSON 命令，每条命令输出一行 JSON 结果。计时全部在本进程内完成，不含进程启动开销。

命令（op）：
  spa       发送一个 SPA 报文（UDP），然后轮询受保护端口直到 HTTP 200，返回 ms（发包到首个 200 响应）
  fnk       fn-knock 网页登录：取 ALTCHA 题目、求解、提交登录，登录响应返回后轮询受保护端口
  gweb      Gately 站点网页登录（放行入口）：提交登录表单并跟随跳转（浏览器行为），然后轮询受保护端口
  state     对受保护端口做一次短探测，返回 open / blocked，用于每次敲门前确认已撤销
  probe     隐蔽性探测：TCP 端口返回 超时 / 拒绝 / 接受后的行为，UDP 端口返回 ICMP 不可达 / 无响应
  quit      退出

轮询口径：每次尝试新建 TCP 连接，0.5 毫秒内没有完成握手就放弃（被 DROP 的 SYN 不会有回应）并重试；
握手成功后发 GET /，收到 "HTTP/1.1 200" 即算放行。时间分辨率约 0.5～1 毫秒。
"""
import base64
import errno
import hashlib
import json
import select
import socket
import sys
import time
import urllib.parse

POLL_STEP = 0.0005


def now():
    return time.perf_counter()


def http_get_200(s, host):
    """在已连接的套接字上发 GET /，判断是否返回 200。"""
    try:
        s.setblocking(True)
        s.settimeout(1.0)
        s.sendall(f"GET / HTTP/1.1\r\nHost: {host}\r\nConnection: close\r\n\r\n".encode())
        buf = b""
        while b"\r\n" not in buf:
            chunk = s.recv(4096)
            if not chunk:
                break
            buf += chunk
        return buf.startswith(b"HTTP/1.1 200") or buf.startswith(b"HTTP/1.0 200")
    except OSError:
        return False


def try_once(ip, port, step=POLL_STEP):
    """尝试一次：在 step 秒内完成握手并拿到 200 返回 True。"""
    s = socket.socket(socket.AF_INET, socket.SOCK_STREAM)
    s.setblocking(False)
    try:
        rc = s.connect_ex((ip, port))
        if rc not in (0, errno.EINPROGRESS):
            return False
        _r, w, _x = select.select([], [s], [], step)
        if not w:
            return False
        if s.getsockopt(socket.SOL_SOCKET, socket.SO_ERROR) != 0:
            return False
        return http_get_200(s, ip)
    finally:
        s.close()


def wait_open(ip, port, timeout):
    """轮询直到受保护端口可用，返回完成时刻（perf_counter），超时返回 None。"""
    deadline = now() + timeout
    while now() < deadline:
        t = now()
        if try_once(ip, port):
            return now()
        # 被拒绝（RST）时 connect 立即失败，补足一个步长，避免空转压垮被测程序
        rest = POLL_STEP - (now() - t)
        if rest > 0:
            time.sleep(rest)
    return None


def http_request(ip, port, method, path, host, headers=None, body=b"", timeout=10):
    """极简 HTTP/1.1 客户端（每次新连接），返回 (状态码, 响应头字典, 响应体)。"""
    s = socket.create_connection((ip, port), timeout=timeout)
    try:
        h = {"Host": host, "Connection": "close", "Content-Length": str(len(body))}
        h.update(headers or {})
        req = f"{method} {path} HTTP/1.1\r\n" + "".join(f"{k}: {v}\r\n" for k, v in h.items()) + "\r\n"
        s.sendall(req.encode() + body)
        buf = b""
        while True:
            chunk = s.recv(65536)
            if not chunk:
                break
            buf += chunk
    finally:
        s.close()
    head, _, rest = buf.partition(b"\r\n\r\n")
    lines = head.decode("latin-1").split("\r\n")
    code = int(lines[0].split()[1]) if lines and len(lines[0].split()) > 1 else 0
    hdrs = {}
    for line in lines[1:]:
        k, _, v = line.partition(":")
        hdrs.setdefault(k.strip().lower(), []).append(v.strip())
    if "chunked" in ",".join(hdrs.get("transfer-encoding", [])):
        out, data = b"", rest
        while data:
            size_s, _, data = data.partition(b"\r\n")
            size = int(size_s.split(b";")[0] or b"0", 16)
            if size == 0:
                break
            out, data = out + data[:size], data[size + 2:]
        rest = out
    return code, hdrs, rest


def solve_altcha(ch):
    """求解 ALTCHA SHA-256 工作量证明：找 number 使 sha256(salt+number) == challenge。"""
    salt, target = ch["salt"], ch["challenge"]
    for n in range(int(ch["maxnumber"]) + 1):
        if hashlib.sha256(f"{salt}{n}".encode()).hexdigest() == target:
            return n
    return None


def op_spa(c):
    pkt = c["pkt"].encode()
    u = socket.socket(socket.AF_INET, socket.SOCK_DGRAM)
    t0 = now()
    u.sendto(pkt, (c["server"], c["knockPort"]))
    u.close()
    t1 = wait_open(c["server"], c["port"], c.get("timeout", 5))
    return {"ok": t1 is not None, "ms": (t1 - t0) * 1000 if t1 else None}


def op_fnk(c):
    ip, port = c["server"], c["authPort"]
    host = f"{ip}:{port}"
    t0 = now()
    code, _h, body = http_request(ip, port, "GET", "/__auth__/api/auth/challenge", host)
    t1 = now()
    if code != 200:
        return {"ok": False, "err": f"challenge {code}"}
    ch = json.loads(body)
    n = solve_altcha(ch)
    t2 = now()
    proof = base64.b64encode(json.dumps({"algorithm": "SHA-256", "challenge": ch["challenge"], "number": n,
                                         "salt": ch["salt"], "signature": ch["signature"]}).encode()).decode()
    data = json.dumps({"method": "password", "username": c["user"], "password": c["password"],
                       "captcha": {"provider": "pow", "proof": proof}}).encode()
    code, _h, body = http_request(ip, port, "POST", "/__auth__/api/auth/login", host,
                                  {"Content-Type": "application/json"}, data)
    t3 = now()
    if code != 200:
        return {"ok": False, "err": f"login {code} {body[:200]!r}"}
    t4 = wait_open(ip, c["port"], c.get("timeout", 5))
    res = {"ok": t4 is not None, "challengeMs": (t1 - t0) * 1000, "solveMs": (t2 - t1) * 1000,
           "powNumber": n, "maxNumber": ch["maxnumber"], "loginMs": (t3 - t2) * 1000}
    if t4:
        # ms：从请求题目到首个 200；exCaptchaMs：从提交登录到首个 200（不含取题与求解）
        res["ms"] = (t4 - t0) * 1000
        res["exCaptchaMs"] = (t4 - t2) * 1000
        res["afterLoginMs"] = (t4 - t3) * 1000
    return res


def op_gweb(c):
    ip, port = c["server"], c["webPort"]
    host = f"{c['domain']}:{port}"
    form = urllib.parse.urlencode({"username": c["user"], "password": c["password"], "next": "/"}).encode()
    t0 = now()
    code, h, _b = http_request(ip, port, "POST", "/__gately_auth/login", host,
                               {"Content-Type": "application/x-www-form-urlencoded", "Origin": f"http://{host}"}, form)
    if code != 302:
        return {"ok": False, "err": f"login {code}"}
    cookie = "; ".join(v.split(";")[0] for v in h.get("set-cookie", []))
    t1 = now()
    # 浏览器随即跟随跳转：带会话访问站点时 Gately 登记放行（登录请求本身不登记）
    code, _h, _b = http_request(ip, port, "GET", "/", host, {"Cookie": cookie})
    t2 = now()
    if code != 200:
        return {"ok": False, "err": f"redirect {code}"}
    t3 = wait_open(ip, c["port"], c.get("timeout", 5))
    res = {"ok": t3 is not None, "loginMs": (t1 - t0) * 1000, "redirectMs": (t2 - t1) * 1000}
    if t3:
        res["ms"] = (t3 - t0) * 1000
        res["exCaptchaMs"] = res["ms"]
        res["afterLoginMs"] = (t3 - t2) * 1000
    return res


def op_state(c):
    # 给足 50 毫秒：放行状态下 veth 上握手只要几十微秒
    for _ in range(3):
        if try_once(c["server"], c["port"], 0.05):
            return {"state": "open"}
    return {"state": "blocked"}


def probe_tcp(ip, port, timeout=3.0):
    s = socket.socket(socket.AF_INET, socket.SOCK_STREAM)
    s.settimeout(timeout)
    t0 = now()
    try:
        s.connect((ip, port))
    except socket.timeout:
        return {"result": "超时（DROP）", "ms": round((now() - t0) * 1000, 1)}
    except ConnectionRefusedError:
        return {"result": "拒绝（RST）", "ms": round((now() - t0) * 1000, 2)}
    except OSError as e:
        return {"result": f"错误 {e}"}
    try:
        s.sendall(f"GET / HTTP/1.1\r\nHost: {ip}\r\nConnection: close\r\n\r\n".encode())
        data = s.recv(200)
        if data:
            return {"result": "握手成功并返回数据", "first": data.split(b"\r\n")[0].decode("latin-1")}
        return {"result": "握手成功后被关闭（FIN）"}
    except ConnectionResetError:
        return {"result": "握手成功后被重置（RST）"}
    except socket.timeout:
        return {"result": "握手成功后无响应"}
    finally:
        s.close()


def probe_udp(ip, port, timeout=1.5):
    s = socket.socket(socket.AF_INET, socket.SOCK_DGRAM)
    s.settimeout(timeout)
    try:
        s.connect((ip, port))
        s.send(b"probe")
        s.recv(100)
        return {"result": "有响应"}
    except ConnectionRefusedError:
        return {"result": "ICMP 端口不可达（关闭）"}
    except socket.timeout:
        return {"result": "无响应（开放或被过滤，无法区分）"}
    finally:
        s.close()


def op_probe(c):
    out = {}
    for proto, port in c["ports"]:
        out[f"{proto}/{port}"] = probe_tcp(c["server"], port) if proto == "tcp" else probe_udp(c["server"], port)
    return out


def main():
    for line in sys.stdin:
        line = line.strip()
        if not line:
            continue
        c = json.loads(line)
        op = c["op"]
        if op == "quit":
            break
        try:
            res = {"spa": op_spa, "fnk": op_fnk, "gweb": op_gweb, "state": op_state, "probe": op_probe}[op](c)
        except Exception as e:  # 单次失败不影响后续命令
            res = {"ok": False, "err": repr(e)}
        print(json.dumps(res), flush=True)


if __name__ == "__main__":
    main()
