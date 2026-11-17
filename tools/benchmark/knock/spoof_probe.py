#!/usr/bin/env python3
"""补充测试：伪造来源 UDP 洪泛的速率扫描（只跑 SPA 类被测程序）。用法：spoof_probe.py <gately|fwknop> [速率,...]

每个速率持续 FLOOD_DUR 秒，期间合法客户端每隔 3 秒敲门一次（两次敲门之间来源网段空闲超过 2 秒，
对 Gately 来说相当于“第一次敲门的新来源”），记录成功率与被测程序 CPU。结果写 results/knock-spoof-<名>.json。
"""
import json
import os
import sys
import time

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import bench as B  # noqa: E402

name = sys.argv[1]
rates = [int(x) for x in (sys.argv[2] if len(sys.argv) > 2 else "1000,2000,3000,5000,10000").split(",")]
os.makedirs(B.WORK, exist_ok=True)
B.setup_net()
sut = B.SUTS[name]()
out = {"sut": name, "rates": {}}
try:
    B.start_origin()
    sut.stop()
    B.fw_cleanup()
    sut.start()
    cg = B.cgroup_of(sut.container)
    cli = B.Client()
    pfile = f"{B.WORK}/invalid.txt"
    B.gen_packets(50, B.new_keys(), pfile)
    flood = B.udp_flood("spoof", pfile)
    n = max(3, (B.FLOOD_DUR - 6) // 4)
    for rate in rates:
        B.notrack(True)
        sut.pool.pkts, sut.pool.at = B.gen_packets(n + 2, sut.keys, None), time.time()
        before = sut.status() if hasattr(sut, "status") else None
        w = B.start_watch(cg, B.FLOOD_DUR - 4, f"{B.WORK}/fw.json")
        fp = flood(rate)
        time.sleep(3)
        runs = B.knock_series(sut, cli, n, timeout=3, pace=3)
        s = B.end_watch(w, f"{B.WORK}/fw.json")
        ftxt = fp.communicate(timeout=B.FLOOD_DUR + 60)[0]
        B.notrack(False)
        r = {"flood": json.loads(ftxt[ftxt.index("{"):]), "sutCpuPercent": round(s["cpuPercent"], 1),
             "legitKnock": B.latency_summary(runs)}
        if before is not None:
            r["statusBefore"], r["statusAfter"] = before, sut.status()
        out["rates"][rate] = r
        B.log(name, rate, json.dumps(r, ensure_ascii=False))
        time.sleep(8)
    cli.close()
finally:
    sut.stop()
    B.notrack(False)
    B.fw_cleanup()
    B.sh("docker rm -f knock-origin 2>/dev/null")
    B.teardown_net()
with open(os.path.join(B.RESULTS, f"knock-spoof-{name}.json"), "w") as f:
    json.dump(out, f, indent=1, ensure_ascii=False)
