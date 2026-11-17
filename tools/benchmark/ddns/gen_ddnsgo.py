#!/usr/bin/env python3
"""生成 ddns-go 配置：N 个 dnsconf，各 1 个域名，IP 取自源站 /ip，用 callback 写到源站 /cb。
用法：gen_ddnsgo.py <输出文件> <任务数> <源站>"""
import sys

out, n, be = sys.argv[1], int(sys.argv[2]), sys.argv[3]
lines = ["dnsconf:"]
for i in range(n):
    lines += [
        f"  - name: bench{i}",
        "    ipv4:",
        "      enable: true",
        "      gettype: url",
        f"      url: {be}/ip",
        "      netinterface: \"\"",
        "      cmd: \"\"",
        "      domains:",
        f"        - t{i}.bench.test",
        "    ipv6: {enable: false, gettype: url, url: \"\", netinterface: \"\", cmd: \"\", ipv6reg: \"\", domains: []}",
        "    dns:",
        "      name: callback",
        f"      id: \"{be}/cb?tool=ddnsgo&domain=#{{domain}}&ip=#{{ip}}\"",
        "      secret: \"\"",
        "      extparam: \"\"",
        "    ttl: \"\"",
    ]
lines += ["user: {username: \"\", password: \"\"}", "webhook: {webhookurl: \"\", webhookrequestbody: \"\", webhookheaders: \"\"}",
          "notallowwanaccess: true", "lang: zh"]
open(out, "w").write("\n".join(lines) + "\n")
