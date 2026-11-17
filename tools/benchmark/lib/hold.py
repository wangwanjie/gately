#!/usr/bin/env python3
"""打开 N 条 keep-alive 连接，每条先发一个请求，然后全部空闲保持若干秒。
用法：hold.py <host> <port> <N> <保持秒数> [tls]
输出成功建立并收到响应的连接数。用于测量被测程序每条空闲连接的内存。"""
import asyncio
import ssl
import sys

host, port, n, hold = sys.argv[1], int(sys.argv[2]), int(sys.argv[3]), float(sys.argv[4])
use_tls = len(sys.argv) > 5 and sys.argv[5] == "tls"


async def one(sem, ok, conns):
    async with sem:
        try:
            ctx = None
            if use_tls:
                ctx = ssl.create_default_context()
                ctx.check_hostname = False
                ctx.verify_mode = ssl.CERT_NONE
            r, w = await asyncio.wait_for(asyncio.open_connection(host, port, ssl=ctx,
                                                                  server_hostname="bench.test" if use_tls else None), 10)
            w.write(b"GET / HTTP/1.1\r\nHost: bench.test\r\n\r\n")
            await w.drain()
            head = await asyncio.wait_for(r.readuntil(b"\r\n\r\n"), 10)
            if b" 200 " in head.split(b"\r\n")[0]:
                await asyncio.wait_for(r.readexactly(1024), 10)
                ok.append(1)
            conns.append(w)
        except Exception:
            pass


async def main():
    sem = asyncio.Semaphore(500)
    ok, conns = [], []
    await asyncio.gather(*(one(sem, ok, conns) for _ in range(n)))
    print(len(ok), flush=True)
    await asyncio.sleep(hold)
    for w in conns:
        w.close()

asyncio.run(main())
