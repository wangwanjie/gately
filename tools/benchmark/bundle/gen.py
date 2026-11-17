#!/usr/bin/env python3
"""“一体化”配置：同一个 Gately 同时承担反代 + WAF + 网页认证 + DDNS + 入侵防护 + 端口敲门，
用来与“各用一个独立程序”的组合比较总内存。各部分配置复用对应专项的生成脚本：
  - 反代与 WAF：waf/suts/gately/gen.py（WAF=on，CRS PL1，bench.test 的 HTTP/HTTPS 站点）
  - 网页认证：在 HTTPS 监听器上加 auth.bench.test 站点（共享用户网页登录）
  - DDNS：ddns/gen_gately.py（20 个任务，回调到本机源站）
  - 入侵防护：一条读 nginx 访问日志的 404 规则（与 guard 专项相同的写法）
  - 端口敲门：一条 SPA 规则保护 tcp/9100（防火墙拦截）
用法：gen.py <配置目录>"""
import base64
import json
import os
import subprocess
import sys

d = sys.argv[1]
B = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
env = dict(os.environ, WAF="on")
subprocess.run([sys.executable, os.path.join(B, "waf/suts/gately/gen.py"), d], check=True, env=env)
subprocess.run([sys.executable, os.path.join(B, "ddns/gen_gately.py"), d, "20", "30", "http://127.0.0.1:9000"], check=True)

ws_path = os.path.join(d, "webservice.json")
ws = json.load(open(ws_path))
# 密码 Bench!Visit2026 的 bcrypt 哈希（cost 10）
bcrypt = "$2a$10$ponXXtMv6jZhVj5N0EaineFa8DZJMMK6ufk6qVTzlbqN3/gNYYj9e"
for l in ws["listeners"]:
    if l["tls"]:
        l["sites"].append({"key": "s-auth", "name": "auth", "enable": True, "domains": ["auth.bench.test"],
                           "type": "proxy", "targets": ["http://127.0.0.1:9000"], "forwardedHeaders": True,
                           "accessLog": False, "webAuth": True, "users": [f"bench:{bcrypt}"], "waf": "w-bench"})
json.dump(ws, open(ws_path, "w"), indent=2)

os.makedirs("/var/log/bench-bundle", exist_ok=True)
open("/var/log/bench-bundle/access.log", "a").close()
guard = {"settings": {"firewall": True, "banSeconds": 3600, "increment": False, "maxBanSeconds": 604800,
                      "ignorePrivate": True, "ignoreList": [],
                      "trap": {"enable": False, "ports": "", "banSeconds": 0},
                      "scan": {"enable": False, "threshold": 12, "excludePorts": "", "banSeconds": 0, "ipv6Strict": False}},
         "jails": [{"key": "j404", "name": "nginx-404", "enable": True, "source": "file",
                    "target": "/var/log/bench-bundle/access.log",
                    "filter": "^<HOST> \\S+ \\S+ \\[[^\\]]+\\] \"[^\"]*\" 404 ", "ignore": "",
                    "maxRetry": 5, "findTime": 60, "banTime": 3600, "remark": ""}]}
json.dump(guard, open(os.path.join(d, "guard.json"), "w"), indent=2)
k = lambda n: base64.b64encode(os.urandom(n)).decode()
knock = {"settings": {"port": 62201, "autoFirewall": False, "allowLan": True, "allowList": [], "maxAge": 120},
         "rules": [{"key": "k-bench", "name": "bench", "enable": True, "ports": ["tcp/9100"], "firewall": True,
                    "encKey": k(32), "hmacKey": k(64), "hmacType": "sha256", "timeout": 300, "maxTimeout": 0,
                    "user": "", "requireAddr": False, "remark": ""}]}
json.dump(knock, open(os.path.join(d, "knock.json"), "w"), indent=2)
