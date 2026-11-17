#!/usr/bin/env python3
"""Lucky 网页认证的登录压测（oha 无法完成挑战式登录，这里用多进程客户端）。
用法：loginload.py <端口> <密码> <并发进程数> <秒数>
每次登录 = GET 挑战（RSA 公钥 + 一次性 nonce）→ 用公钥加密 账号/密码/nonce → POST 登录，
返回 ret=0 计为成功。每个进程一条 HTTPS keep-alive 连接。输出 JSON：成功次数每秒、全部请求数与延迟分位。"""
import base64
import http.client
import json
import multiprocessing as mp
import re
import ssl
import sys
import time

from cryptography.hazmat.primitives import serialization
from cryptography.hazmat.primitives.asymmetric import padding

PREFIX = "webauth_0xa2c2a_"
CTX = ssl.create_default_context()
CTX.check_hostname = False
CTX.verify_mode = ssl.CERT_NONE


def login_base(port):
    """浏览器访问站点得到跳转页，从中取出登录页路径（每个子规则不同）。"""
    c = http.client.HTTPSConnection("bench.test", port, context=CTX)
    c.request("GET", "/", headers={"User-Agent": "Mozilla/5.0", "Accept": "text/html"})
    b = c.getresponse().read().decode()
    return re.search(r'var target = "([^"]+)"', b).group(1)


def login_once(c, base, pw):
    """完成一次登录，返回 (是否成功, 耗时秒, Set-Cookie 列表, 失败原因)。"""
    t0 = time.time()
    h = {"Accept": "application/json"}
    c.request("GET", base + PREFIX + "/api/login/challenge", headers=h)
    r = c.getresponse()
    raw = r.read()
    try:
        ch = json.loads(raw)
    except ValueError:
        ch = {}
    if not ch.get("publicKey"):
        # 挑战被拒（例如登录限流），记下状态码与提示
        return False, time.time() - t0, [], f"challenge {r.status} {ch.get('ret')} {ch.get('msg', raw[:60])}"
    pub = serialization.load_pem_public_key(ch["publicKey"].encode())
    plain = json.dumps({"account": "bench", "password": pw, "twoFA": "", "challengeId": ch["challengeId"],
                        "nonce": ch["nonce"]}, separators=(",", ":")).encode()
    body = json.dumps({"challengeId": ch["challengeId"],
                       "cipherText": base64.b64encode(pub.encrypt(plain, padding.PKCS1v15())).decode()})
    c.request("POST", base + PREFIX + "/api/login", body=body, headers=dict(h, **{"Content-Type": "application/json"}))
    r = c.getresponse()
    out = json.loads(r.read())
    cookies = [v.split(";", 1)[0] for k, v in r.getheaders() if k.lower() == "set-cookie"]
    return out.get("ret") == 0, time.time() - t0, cookies, f"login {r.status} {out.get('ret')} {out.get('msg', '')}"


def worker(port, base, pw, dur, q):
    c = http.client.HTTPSConnection("bench.test", port, context=CTX, timeout=30)
    ok = bad = err = 0
    lat = []
    # 各类失败原因的计数（含异常类型），便于判断是否触发了防爆破或限流
    reasons = {}
    end = time.time() + dur
    while time.time() < end:
        try:
            good, dt, _, why = login_once(c, base, pw)
            lat.append(dt)
            if good:
                ok += 1
            else:
                bad += 1
                reasons[why] = reasons.get(why, 0) + 1
        except Exception as e:
            err += 1
            k = "error " + type(e).__name__
            reasons[k] = reasons.get(k, 0) + 1
            c.close()
            c = http.client.HTTPSConnection("bench.test", port, context=CTX, timeout=30)
    q.put((ok, bad, err, lat, reasons))


def main():
    port, pw, conc, dur = int(sys.argv[1]), sys.argv[2], int(sys.argv[3]), float(sys.argv[4])
    base = login_base(port)
    q = mp.Queue()
    t0 = time.time()
    ps = [mp.Process(target=worker, args=(port, base, pw, dur, q)) for _ in range(conc)]
    for p in ps:
        p.start()
    res = [q.get() for _ in ps]
    for p in ps:
        p.join()
    el = time.time() - t0
    ok, bad, err = (sum(r[i] for r in res) for i in range(3))
    lat = sorted(x for r in res for x in r[3])
    reasons = {}
    for r in res:
        for k, v in r[4].items():
            reasons[k] = reasons.get(k, 0) + v
    pct = lambda p: round(lat[min(len(lat) - 1, int(len(lat) * p))] * 1000, 2) if lat else 0
    print(json.dumps({"ok": ok, "rejected": bad, "errors": err, "total": ok + bad, "elapsed": el,
                      "okPerSec": round(ok / el, 1), "rps": round((ok + bad) / el, 1),
                      "p50ms": pct(0.5), "p99ms": pct(0.99), "requestsPerLogin": 2,
                      "reasons": reasons}, ensure_ascii=False))


if __name__ == "__main__":
    main()
