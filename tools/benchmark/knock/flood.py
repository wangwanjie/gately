#!/usr/bin/env python3
"""无效 SPA 报文洪泛（在客户端网络命名空间内运行，由 bench.py 以 taskset 绑到压测 CPU）。

用法：flood.py <模式> <目标IP> <端口> <包/秒> <秒数> <报文文件> [进程数] [单源地址]
  模式 single：从单源地址（默认 198.51.100.10）用普通 UDP 套接字发送；
  模式 spoof ：用原始套接字伪造随机来源地址（覆盖数万个 /24 网段，排除内网、回环、组播与测试客户端所在网段），
              模拟分布式或伪造来源的洪泛。UDP 校验和置 0（IPv4 允许），IP 头校验和与长度由内核填写。
报文文件每行一个格式正确、但用错误密钥生成的 SPA 报文（能通过格式预检，HMAC 校验失败），循环发送。
按 1 毫秒时间片补发控制速率；输出 JSON：实际发送数与实际速率。
"""
import json
import multiprocessing as mp
import random
import socket
import struct
import sys
import time


def rand_public_v4(rng):
    """随机取一个非内网、非保留、且不在测试网段的 IPv4 地址。"""
    while True:
        a = rng.getrandbits(32)
        o1, o2, o3 = a >> 24, (a >> 16) & 0xFF, (a >> 8) & 0xFF
        if o1 in (0, 10, 127) or o1 >= 224:
            continue
        if (o1 == 100 and 64 <= o2 <= 127) or (o1 == 169 and o2 == 254) or (o1 == 172 and 16 <= o2 <= 31):
            continue
        if (o1 == 192 and o2 == 168) or (o1 == 198 and o2 in (18, 19)):
            continue
        if (o1, o2, o3) in ((198, 51, 100), (203, 0, 113), (192, 0, 2)):
            continue
        return socket.inet_aton(f"{o1}.{o2}.{o3}.{a & 0xFF}")


def build_spoofed(payloads, dst, port, n, seed):
    rng = random.Random(seed)
    dst_b = socket.inet_aton(dst)
    out = []
    for i in range(n):
        p = payloads[i % len(payloads)]
        udp = struct.pack("!HHHH", rng.randint(1024, 65535), port, 8 + len(p), 0) + p
        # 版本/首部长度、TOS、总长（内核填写）、ID（内核填写）、分片、TTL、协议、校验和（内核填写）、源、目的
        ip = struct.pack("!BBHHHBBH4s4s", 0x45, 0, 0, 0, 0, 64, 17, 0, rand_public_v4(rng), dst_b)
        out.append(ip + udp)
    return out


def worker(mode, dst, port, rate, dur, payloads, src, seed, q):
    if mode == "spoof":
        s = socket.socket(socket.AF_INET, socket.SOCK_RAW, socket.IPPROTO_RAW)
        pkts = build_spoofed(payloads, dst, port, 65536, seed)
        addr = (dst, 0)
    else:
        s = socket.socket(socket.AF_INET, socket.SOCK_DGRAM)
        s.bind((src, 0))
        pkts = payloads
        addr = (dst, port)
    s.setsockopt(socket.SOL_SOCKET, socket.SO_SNDBUF, 4 << 20)
    n = len(pkts)
    sent = errs = 0
    i = 0
    send = s.sendto
    t0 = time.perf_counter()
    end = t0 + dur
    while True:
        t = time.perf_counter()
        if t >= end:
            break
        target = int((t - t0) * rate)
        # 每个时间片最多补发 1 毫秒的量的 4 倍，落后太多时不突发
        budget = min(target - sent, max(int(rate / 250), 1))
        if budget <= 0:
            time.sleep(0.0005)
            continue
        for _ in range(budget):
            try:
                send(pkts[i], addr)
                sent += 1
            except OSError:
                errs += 1
                sent += 1
            i += 1
            if i == n:
                i = 0
    q.put((sent, errs, time.perf_counter() - t0))


def main():
    mode, dst, port, rate, dur, pfile = sys.argv[1], sys.argv[2], int(sys.argv[3]), float(sys.argv[4]), float(sys.argv[5]), sys.argv[6]
    procs = int(sys.argv[7]) if len(sys.argv) > 7 else 1
    src = sys.argv[8] if len(sys.argv) > 8 else "198.51.100.10"
    payloads = [line.strip().encode() for line in open(pfile) if line.strip()]
    q = mp.Queue()
    ps = [mp.Process(target=worker, args=(mode, dst, port, rate / procs, dur, payloads, src, 1000 + k, q))
          for k in range(procs)]
    for p in ps:
        p.start()
    res = [q.get() for _ in ps]
    for p in ps:
        p.join()
    sent = sum(r[0] for r in res)
    errs = sum(r[1] for r in res)
    el = max(r[2] for r in res)
    print(json.dumps({"mode": mode, "targetPps": rate, "sent": sent, "sendErrors": errs,
                      "achievedPps": round(sent / el, 1), "seconds": round(el, 2)}), flush=True)


if __name__ == "__main__":
    main()
