#!/usr/bin/env python3
"""把样本集逐条发给被测程序，记录每条请求的响应状态码。

用法：replay.py <样本 jsonl> <主机> <端口> <输出 json> [并发数]
每条请求单独建连、带 Connection: close，请求目标与请求体按样本原样发送（不做任何规范化），
Host 固定为 bench.test。读不到状态行（连接被重置、超时）时状态记为 0。
"""
import base64
import json
import socket
import sys
from concurrent.futures import ThreadPoolExecutor

HOST_NAME = "bench.test"
TIMEOUT = 10


def build(s):
    """按样本生成原始请求字节。"""
    if "bodyB64" in s:
        body = base64.b64decode(s["bodyB64"])
    elif "body" in s:
        body = s["body"].encode("utf-8")
    else:
        body = None
    ver = s.get("version", "HTTP/1.1")
    lines = [f"{s['method']} {s['path']} {ver}", f"Host: {HOST_NAME}"]
    for k, v in s["headers"]:
        lines.append(f"{k}: {v}")
    if body is not None:
        lines.append(f"Content-Length: {len(body)}")
    lines.append("Connection: close")
    head = ("\r\n".join(lines) + "\r\n\r\n").encode("utf-8")
    return head + (body or b"")


def send(addr, s):
    data = build(s)
    try:
        with socket.create_connection(addr, timeout=TIMEOUT) as c:
            c.sendall(data)
            buf = b""
            while b"\r\n" not in buf and len(buf) < 4096:
                chunk = c.recv(4096)
                if not chunk:
                    break
                buf += chunk
            # 跳过 100 Continue 之类的临时响应
            while buf.startswith(b"HTTP/1.1 1") and b"\r\n\r\n" in buf:
                buf = buf.split(b"\r\n\r\n", 1)[1]
                while b"\r\n" not in buf:
                    chunk = c.recv(4096)
                    if not chunk:
                        break
                    buf += chunk
            parts = buf.split(b" ", 2)
            if len(parts) >= 2 and parts[0].startswith(b"HTTP/"):
                return int(parts[1])
            return 0
    except (OSError, ValueError):
        return 0


def main():
    path, host, port, out = sys.argv[1:5]
    workers = int(sys.argv[5]) if len(sys.argv) > 5 else 8
    rows = [json.loads(l) for l in open(path, encoding="utf-8") if l.strip()]
    addr = (host, int(port))
    with ThreadPoolExecutor(workers) as ex:
        codes = list(ex.map(lambda s: send(addr, s), rows))
    res = {r["id"]: c for r, c in zip(rows, codes)}
    with open(out, "w") as f:
        json.dump(res, f)
    from collections import Counter
    print(path.rsplit("/", 1)[-1], dict(Counter(codes)))


if __name__ == "__main__":
    main()
