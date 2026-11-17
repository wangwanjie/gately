#!/usr/bin/env python3
"""生成端口敲门专项的 Gately 配置。用法：gately_gen.py <配置目录> <密钥文件>

- knock.json：一条规则 k-bench，保护 tcp/<受保护端口>，开启防火墙拦截（nftables 表 inet gately_knock），
  密钥与 fwknopd 的 access.conf 相同；放行 300 秒（数据面测试期间不过期），内网免敲门保持默认开启
  （测试客户端用的是 TEST-NET 公网地址，不受影响），不自动放行系统防火墙（压测机没有 fw4/firewalld）。
- webservice.json：HTTP 监听 <网页端口>，站点 knock.test 开启网页认证并作为该规则的放行入口（knockGrant），
  对应 fn-knock 的“网页登录后按来源 IP 放行”。用户密码为 bcrypt（cost 10）哈希，与真实部署一致。
其余为空跑镜像生成的默认配置。
"""
import json
import os
import sys

d, keyfile = sys.argv[1], sys.argv[2]
enc, hmac = open(keyfile).read().split()
port = int(os.environ.get("PROT_PORT", "9100"))
web_port = int(os.environ.get("GATELY_WEB_PORT", "8080"))
knock_port = int(os.environ.get("KNOCK_PORT", "62201"))
# 密码 Bench!Visit2026 的 bcrypt 哈希（cost 10），与 fn-knock 访客账号的密码相同
BCRYPT = "$2a$10$ponXXtMv6jZhVj5N0EaineFa8DZJMMK6ufk6qVTzlbqN3/gNYYj9e"

knock = {
    "settings": {"port": knock_port, "autoFirewall": False, "allowLan": True, "allowList": [], "maxAge": 120},
    "rules": [{
        "key": "k-bench", "name": "bench", "enable": True, "ports": [f"tcp/{port}"], "firewall": True,
        "encKey": enc, "hmacKey": hmac, "hmacType": "sha256", "timeout": 300, "maxTimeout": 0,
        "user": "", "requireAddr": False, "remark": "",
    }],
}
json.dump(knock, open(os.path.join(d, "knock.json"), "w"), indent=2)

ws_path = os.path.join(d, "webservice.json")
ws = json.load(open(ws_path)) if os.path.exists(ws_path) else {"settings": {"enable": True}}
ws.setdefault("settings", {})["enable"] = True
site = {"key": "s-knock", "name": "knock", "enable": True, "domains": ["knock.test"], "type": "text",
        "text": "ok", "contentType": "text/plain", "statusCode": 200, "accessLog": False,
        "webAuth": True, "users": [f"bench:{BCRYPT}"],
        # 放行时长（分钟）：数据面测试期间不过期
        "knockGrant": {"rule": "k-bench", "minutes": 60, "v6Prefix": 64}}
ws["listeners"] = [{"key": "l-knock", "name": "knock", "enable": True, "network": "tcp", "listenIp": "",
                    "port": web_port, "tls": False, "sites": [site],
                    # 默认站点启用“断开连接”（相当于 nginx 444）：不带 knock.test 域名的探测直接断开，不暴露登录页
                    "default": {"type": "close", "enable": True}}]
json.dump(ws, open(ws_path, "w"), indent=2)
# 管理员密码改为 Bench!Visit2026：默认密码不允许调用修改类接口，测试要经 DELETE /api/knock/grants 撤销授权
acc_path = os.path.join(d, "account.json")
acc = json.load(open(acc_path))
acc.update({"passwordHash": BCRYPT, "usingDefaultPassword": False, "weakPassword": False, "onboardingPending": False})
json.dump(acc, open(acc_path, "w"), indent=2)
# 跳过首次登录的使用场景向导（不影响服务）
json.dump({"scenePending": False}, open(os.path.join(d, "onboarding.json"), "w"))
