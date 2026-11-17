#!/usr/bin/env python3
"""通过 Lucky 管理接口配置认证压测站点（Lucky 3.x 配置文件加密，只能走接口；登录方式同 suts/lucky/setup.py）。
用法：lucky_setup.py <管理地址> <证书> <私钥> <源站 host:port> <用户名> <密码>
建三个 HTTPS 监听规则，bench.test 子规则均反代到源站：
  8443 网页认证（WebAuth，登录页 + 会话 Cookie）  8444 不加认证  8445 基本认证（EnableBasicAuth）
Lucky 的站点认证用户写在子规则的 BasicAuthUserList（"用户名:密码"，明文保存）。"""
import base64
import json
import secrets
import sys
import time
import urllib.error
import urllib.request

from cryptography.hazmat.primitives import serialization
from cryptography.hazmat.primitives.asymmetric import padding

base, crt, key, backend, user, pw = sys.argv[1:7]
backend = "http://" + backend
sess = secrets.token_hex(32)
token = ""


def req(path, data=None, method=None):
    h = {"Lucky-Login-Session": sess, "Content-Type": "application/json"}
    if token:
        h["Lucky-Admin-Token"] = token
    body = json.dumps(data).encode() if data is not None else None
    r = urllib.request.Request(base + path, data=body, headers=h, method=method)
    # 刚启动时模块还在初始化，接口会短暂返回 503 或 404，重试即可
    for i in range(100):
        try:
            with urllib.request.urlopen(r, timeout=10) as resp:
                return json.load(resp)
        except urllib.error.HTTPError as e:
            if e.code not in (503, 404) or i == 99:
                raise
            time.sleep(0.2)


for _ in range(150):
    try:
        ch = req("/api/login/challenge")
        break
    except Exception:
        time.sleep(0.2)
pub = serialization.load_pem_public_key(ch["publicKey"].encode())
plain = json.dumps({"account": "666", "password": "666", "twoFA": "", "challengeId": ch["challengeId"],
                    "nonce": ch["nonce"]}, separators=(",", ":")).encode()
ct = base64.b64encode(pub.encrypt(plain, padding.PKCS1v15())).decode()
token = req("/api/login", {"challengeId": ch["challengeId"], "cipherText": ct})["token"]

b64 = lambda p: base64.b64encode(open(p, "rb").read()).decode()
print("ssl", req("/api/ssl", {"Key": "", "Enable": True, "Remark": "bench", "CertBase64": b64(crt),
                              "KeyBase64": b64(key), "AddFrom": "file", "ExtParams": {}}).get("ret"))


def sub(web_auth=False, basic=False):
    s = {"Enable": True, "Domains": ["bench.test"], "WebServiceType": "reverseproxy", "Locations": [backend],
         "SafeIPMode": "blacklist"}
    if web_auth or basic:
        s.update({"EnableBasicAuth": basic, "WebAuth": web_auth, "BasicAuthUserList": f"{user}:{pw}",
                  "OtherParams": {"WebAuth": web_auth}})
    return s


for port, s in ((8443, sub(web_auth=True)), (8444, sub()), (8445, sub(basic=True))):
    rule = {"Name": f"bench{port}", "Enable": True, "ListenPort": port, "EnableTLS": True, "Network": "tcp",
            "IPFilterRule": "blacklist",
            "DefaultProxy": {"Enable": True, "WebServiceType": "reverseproxy", "Locations": [backend],
                             "SafeIPMode": "blacklist"},
            "ProxyList": [s]}
    print("rule", port, req("/api/webservice/rules", rule).get("ret"))
