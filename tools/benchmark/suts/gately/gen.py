#!/usr/bin/env python3
"""生成 Gately 压测配置：HTTP 8080 与 HTTPS 8443 各一个监听器，bench.test 反代到源站。
证书以上传方式写入，内容取自与其他被测程序相同的证书。用法：gen.py <配置目录>"""
import json
import os
import sys

d = sys.argv[1]
os.makedirs(d, exist_ok=True)


def site(key):
    return {"key": key, "name": "bench", "enable": True, "domains": ["bench.test"], "type": "proxy",
            "targets": ["http://127.0.0.1:9000"], "forwardedHeaders": True, "accessLog": False}


ws_path = os.path.join(d, "webservice.json")
ws = json.load(open(ws_path)) if os.path.exists(ws_path) else {"settings": {"enable": True}}
ws["settings"]["enable"] = True
ws["listeners"] = [
    {"key": "l-http", "name": "http", "enable": True, "network": "tcp", "listenIp": "", "port": 8080,
     "tls": False, "sites": [site("s-http")], "default": {"type": "close"}},
    {"key": "l-https", "name": "https", "enable": True, "network": "tcp", "listenIp": "", "port": 8443,
     "tls": True, "sites": [site("s-https")], "default": {"type": "close"}},
]
json.dump(ws, open(ws_path, "w"), indent=2)
ssl = {"settings": {"enable": False, "renewBeforeDays": 30, "shortLivedRenewHours": 24},
       "certs": [{"key": "c-bench", "remark": "bench", "enable": True, "source": "upload",
                  "certPem": open("/etc/bench/certs/bench.crt").read(),
                  "keyPem": open("/etc/bench/certs/bench.key").read()}]}
json.dump(ssl, open(os.path.join(d, "ssl.json"), "w"), indent=2)
# 跳过首次登录的使用场景向导（不影响服务）
json.dump({"scenePending": False}, open(os.path.join(d, "onboarding.json"), "w"))
