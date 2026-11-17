#!/usr/bin/env python3
"""通过 Lucky 管理接口配置压测站点（Lucky 3.x 的配置文件加密，只能走接口）。
用法：setup.py <管理地址> <证书> <私钥>；环境变量 BACKEND 为源站地址。"""
import base64
import os
import sys

sys.path.insert(0, os.path.join(os.path.dirname(os.path.abspath(__file__)), "..", "..", "lib"))
from lucky_api import Lucky  # noqa: E402

base, crt, key = sys.argv[1], sys.argv[2], sys.argv[3]
# 源站地址，默认为压测虚拟机上的 127.0.0.1:9000
backend = os.environ.get("BACKEND", "http://127.0.0.1:9000")
lk = Lucky(base)
req = lk.req

b64 = lambda p: base64.b64encode(open(p, "rb").read()).decode()
print("ssl", req("/api/ssl", {"Key": "", "Enable": True, "Remark": "bench", "CertBase64": b64(crt),
                              "KeyBase64": b64(key), "AddFrom": "file", "ExtParams": {}}).get("ret"))
for port, tls in ((8080, False), (8443, True)):
    rule = {"Name": f"bench{port}", "Enable": True, "ListenPort": port, "EnableTLS": tls, "Network": "tcp",
            "IPFilterRule": "blacklist",
            "DefaultProxy": {"Enable": True, "WebServiceType": "reverseproxy",
                             "Locations": [backend], "SafeIPMode": "blacklist"},
            "ProxyList": [{"Enable": True, "Domains": ["bench.test"], "WebServiceType": "reverseproxy",
                           "Locations": [backend], "SafeIPMode": "blacklist"}]}
    print("rule", port, req("/api/webservice/rules", rule).get("ret"))
