#!/usr/bin/env python3
"""生成路由器上 Gately 的压测配置（在压测机上运行后拷到路由器）。
用法：gen_gately.py <输出目录> <源站 http://IP:端口> <证书> <私钥>"""
import json
import os
import sys

d, be, crt, key = sys.argv[1:5]
os.makedirs(d, exist_ok=True)


def site(k):
    return {"key": k, "name": "bench", "enable": True, "domains": ["r.bench.test"], "type": "proxy",
            "targets": [be], "forwardedHeaders": True, "accessLog": False}


ws = {"settings": {"enable": True}, "listeners": [
    {"key": "l-http", "name": "http", "enable": True, "network": "tcp", "port": 8080, "tls": False,
     "sites": [site("s-http")], "default": {"type": "close"}},
    {"key": "l-https", "name": "https", "enable": True, "network": "tcp", "port": 8443, "tls": True,
     "sites": [site("s-https")], "default": {"type": "close"}}]}
json.dump(ws, open(os.path.join(d, "webservice.json"), "w"), indent=1)
json.dump({"settings": {"enable": False, "renewBeforeDays": 30, "shortLivedRenewHours": 24},
           "certs": [{"key": "c", "remark": "bench", "enable": True, "source": "upload",
                      "certPem": open(crt).read(), "keyPem": open(key).read()}]},
          open(os.path.join(d, "ssl.json"), "w"), indent=1)
json.dump({"scenePending": False}, open(os.path.join(d, "onboarding.json"), "w"))
