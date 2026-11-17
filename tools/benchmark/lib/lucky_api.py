#!/usr/bin/env python3
"""Lucky 3.x 管理接口客户端：默认账号 666/666，登录为 RSA 挑战（取 challenge → 公钥加密账号密码 → 换 token）。"""
import json
import secrets
import time
import urllib.error
import urllib.request

from cryptography.hazmat.primitives import serialization
from cryptography.hazmat.primitives.asymmetric import padding
import base64


class Lucky:
    def __init__(self, base, account="666", password="666"):
        self.base = base
        self.sess = secrets.token_hex(32)
        self.token = ""
        ch = None
        for _ in range(300):
            try:
                ch = self.req("/api/login/challenge")
                break
            except Exception:
                time.sleep(0.2)
        pub = serialization.load_pem_public_key(ch["publicKey"].encode())
        plain = json.dumps({"account": account, "password": password, "twoFA": "", "challengeId": ch["challengeId"],
                            "nonce": ch["nonce"]}, separators=(",", ":")).encode()
        ct = base64.b64encode(pub.encrypt(plain, padding.PKCS1v15())).decode()
        self.token = self.req("/api/login", {"challengeId": ch["challengeId"], "cipherText": ct})["token"]

    def req(self, path, data=None, method=None):
        h = {"Lucky-Login-Session": self.sess, "Content-Type": "application/json"}
        if self.token:
            h["Lucky-Admin-Token"] = self.token
        body = json.dumps(data).encode() if data is not None else None
        r = urllib.request.Request(self.base + path, data=body, headers=h, method=method)
        # 刚启动时模块还在初始化，接口会短暂返回 503 或 404（模块尚未注册路由），重试即可
        for i in range(150):
            try:
                with urllib.request.urlopen(r, timeout=10) as resp:
                    return json.load(resp)
            except urllib.error.HTTPError as e:
                if e.code not in (503, 404) or i == 149:
                    raise
                time.sleep(0.2)
