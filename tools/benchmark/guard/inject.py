#!/usr/bin/env python3
"""日志注入器：以固定速率向 nginx combined 格式的访问日志追加行（正常请求 + 扫描 404）。

用法：
  inject.py --log <文件> --rate <行/秒> --duration <秒> --mal-frac <恶意行比例> \
            --maxretry 5 --ip-start <起始序号> --out <结果json> [--attackers <攻击者上限>]

做法：
- 每 10 毫秒按“应写行数 = 速率 × 已过时间 - 已写行数”补齐，一次 write 写入，速率由墙钟控制，不会越写越慢。
- 恶意行来自攻击者池：每个攻击者恰好写 maxretry 行 404（达到阈值即停，模拟被封后不再有日志），
  池大小按恶意行速率取约 0.3 秒的量，所以同一攻击者的几行分散在约 1~2 秒内、与其他攻击者交错出现。
  含第 maxretry 行的那一批开始写入（write 调用前）的时间记为该 IP 的“达到阈值时间”，
  同时记 CLOCK_MONOTONIC 与墙钟：延迟用单调时钟计算（墙钟可能被时间同步调整，colima 里实测 40 秒内跳 0.38 秒），
  墙钟只用于与被测程序日志里的时间戳对照。
- 攻击者地址取自 198.18.0.0/15（RFC 2544 基准测试保留段，不是内网段，两边都不会因“忽略内网”而跳过），
  按 --ip-start 起的序号依次分配，不同测试互不重复；正常请求来自 203.0.113.0/24 与 192.0.2.0/24，状态码 200。
- 设置 --attackers 时，攻击者数达到上限且全部写完后提前结束（用于“1 万个攻击者”测试）。
- 结束时仍未写满 maxretry 行的攻击者记为 partial（不应被封禁）。
"""
import argparse
import json
import os
import random
import time

# 扫描器常见路径，只用于让日志看起来像真实扫描，规则不看路径
SCAN_PATHS = ["/wp-login.php", "/.env", "/.git/config", "/phpmyadmin/index.php", "/admin/config.php",
              "/vendor/phpunit/phpunit/src/Util/PHP/eval-stdin.php", "/cgi-bin/luci", "/actuator/health",
              "/boaform/admin/formLogin", "/xmlrpc.php", "/HNAP1/", "/solr/admin/info/system"]
OK_PATHS = ["/", "/index.html", "/static/app.js", "/static/app.css", "/api/list?page=2", "/img/logo.png"]
UA = "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/130.0 Safari/537.36"


def attacker_ip(n):
    """第 n 个攻击者的地址：198.18.0.0/15 内顺序分配（共 131072 个，跳过 .0 与 .255 结尾）。"""
    n = n % 128000
    hi, lo = divmod(n, 254)
    return f"198.{18 + hi // 256}.{hi % 256}.{lo + 1}"


def benign_ips():
    out = []
    for net in ("203.0.113", "192.0.2"):
        for i in range(1, 255):
            out.append(f"{net}.{i}")
    return out


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--log", required=True)
    ap.add_argument("--rate", type=float, required=True)
    ap.add_argument("--duration", type=float, default=60)
    ap.add_argument("--mal-frac", type=float, default=0.02)
    ap.add_argument("--maxretry", type=int, default=5)
    ap.add_argument("--ip-start", type=int, default=0)
    ap.add_argument("--attackers", type=int, default=0)
    ap.add_argument("--out", required=True)
    a = ap.parse_args()

    rnd = random.Random(12345)
    good = benign_ips()
    mal_rate = a.rate * a.mal_frac
    # 攻击者池大小：约 0.3 秒的恶意行量，至少 1 个
    pool_size = max(1, int(mal_rate * 0.3))
    pool = []  # 每项为 [ip, 已写行数]
    next_id = a.ip_start
    started = 0
    thresholds = {}
    written = mal_written = 0
    # 恶意行用累加器按比例确定性分配，避免随机波动
    mal_acc = 0.0

    fd = os.open(a.log, os.O_WRONLY | os.O_APPEND | os.O_CREAT, 0o644)
    t0 = time.time()
    m0 = time.monotonic()
    # 速率按单调时钟控制，墙钟只用于日志里的时间戳
    stop_at = m0 + a.duration if a.duration > 0 else float("inf")
    tick = 0.01
    while True:
        now = time.monotonic()
        if now >= stop_at:
            break
        if a.attackers and started >= a.attackers and not pool:
            break
        want = int((now - m0) * a.rate) - written
        if want <= 0:
            time.sleep(tick)
            continue
        ts = time.strftime("%d/%b/%Y:%H:%M:%S %z")
        lines = []
        done_now = []
        for _ in range(want):
            mal_acc += a.mal_frac
            is_mal = mal_acc >= 1.0
            if is_mal:
                mal_acc -= 1.0
                # 补充攻击者池
                while len(pool) < pool_size and (not a.attackers or started < a.attackers):
                    pool.append([attacker_ip(next_id), 0])
                    next_id += 1
                    started += 1
                if not pool:
                    # 已达攻击者上限：剩余名额按正常行处理（只会在收尾阶段出现）
                    is_mal = False
            if is_mal:
                i = rnd.randrange(len(pool))
                ent = pool[i]
                ent[1] += 1
                path = SCAN_PATHS[rnd.randrange(len(SCAN_PATHS))]
                lines.append(f'{ent[0]} - - [{ts}] "GET {path} HTTP/1.1" 404 153 "-" "{UA}"\n')
                mal_written += 1
                if ent[1] >= a.maxretry:
                    done_now.append(ent[0])
                    pool[i] = pool[-1]
                    pool.pop()
            else:
                ip = good[rnd.randrange(len(good))]
                path = OK_PATHS[rnd.randrange(len(OK_PATHS))]
                lines.append(f'{ip} - - [{ts}] "GET {path} HTTP/1.1" 200 1024 "-" "{UA}"\n')
        data = "".join(lines).encode()
        # 取写入前的时刻作为达到阈值时刻：write 本身只需几十微秒，写入后取时间则可能因本进程被调度出去而偏晚
        tw = [time.monotonic(), time.time()]
        os.write(fd, data)
        for ip in done_now:
            thresholds[ip] = tw
        written += want
    os.close(fd)
    end = time.time()
    m1 = time.monotonic()
    res = {"start": t0, "end": end, "monoStart": m0, "monoEnd": m1, "lines": written, "malLines": mal_written,
           "targetRate": a.rate, "achievedRate": written / max(m1 - m0, 1e-6),
           "attackersStarted": started, "thresholds": thresholds,
           "partial": [p[0] for p in pool], "nextIp": next_id}
    with open(a.out, "w") as f:
        json.dump(res, f)


if __name__ == "__main__":
    main()
