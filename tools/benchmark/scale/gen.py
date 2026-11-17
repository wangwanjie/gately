#!/usr/bin/env python3
"""为各被测程序生成 N 个站点的配置：域名 s0.bench.test … s{N-1}.bench.test，全部反代到源站。
HTTP 8080 与 HTTPS 8443，证书统一用 *.bench.test 通配证书。
用法：gen.py <程序> <N> <输出目录>"""
import json
import os
import sys

tool, n, out = sys.argv[1], int(sys.argv[2]), sys.argv[3]
os.makedirs(out, exist_ok=True)
BE = "127.0.0.1:9000"
names = [f"s{i}.bench.test" for i in range(n)]

if tool == "nginx":
    servers = []
    for d in names:
        servers.append(f"""  server {{
    listen 8080; listen 8443 ssl; server_name {d};
    location / {{ proxy_pass http://be; proxy_http_version 1.1; proxy_set_header Connection ""; proxy_set_header Host $host; }}
  }}""")
    conf = f"""worker_processes 2;
worker_rlimit_nofile 200000;
events {{ worker_connections 12000; }}
http {{
  access_log off;
  keepalive_requests 1000000;
  server_names_hash_max_size 4096;
  server_names_hash_bucket_size 128;
  ssl_certificate /certs/bench.crt;
  ssl_certificate_key /certs/bench.key;
  http2 on;
  upstream be {{ server {BE}; keepalive 64; }}
{chr(10).join(servers)}
}}
"""
    open(os.path.join(out, "nginx.conf"), "w").write(conf)
elif tool == "haproxy":
    lines = ["global", "  nbthread 2", "  maxconn 100000", "defaults", "  mode http", "  timeout connect 5s",
             "  timeout client 60s", "  timeout server 60s", "frontend fe", "  bind :8080",
             "  bind :8443 ssl crt /certs/bench.pem alpn h2,http/1.1"]
    # 每个站点一个独立后端（与其他程序“每站点一条规则”对应），按 Host 精确匹配选择后端
    for i, d in enumerate(names):
        lines.append(f"  use_backend be{i} if {{ hdr(host) -i {d} }}")
    for i, _ in enumerate(names):
        lines += [f"backend be{i}", f"  server s1 {BE}"]
    open(os.path.join(out, "haproxy.cfg"), "w").write("\n".join(lines) + "\n")
elif tool == "caddy":
    blocks = ["{\n\tauto_https off\n}"]
    for d in names:
        blocks.append(f"http://{d}:8080 {{\n\treverse_proxy {BE}\n}}")
        blocks.append(f"https://{d}:8443 {{\n\ttls /certs/bench.crt /certs/bench.key\n\treverse_proxy {BE}\n}}")
    open(os.path.join(out, "Caddyfile"), "w").write("\n".join(blocks) + "\n")
elif tool == "traefik":
    import textwrap
    rs, ss = [], []
    for i, d in enumerate(names):
        rs.append(f"    r{i}:\n      rule: \"Host(`{d}`)\"\n      service: b{i}\n      entryPoints: [web]")
        rs.append(f"    t{i}:\n      rule: \"Host(`{d}`)\"\n      service: b{i}\n      entryPoints: [websecure]\n      tls: {{}}")
        ss.append(f"    b{i}:\n      loadBalancer:\n        servers:\n          - url: \"http://{BE}\"")
    dyn = "http:\n  routers:\n" + "\n".join(rs) + "\n  services:\n" + "\n".join(ss) + \
          "\ntls:\n  certificates:\n    - certFile: /certs/bench.crt\n      keyFile: /certs/bench.key\n"
    open(os.path.join(out, "dynamic.yml"), "w").write(dyn)
elif tool == "gately":
    ws_path = os.path.join(out, "webservice.json")
    ws = json.load(open(ws_path)) if os.path.exists(ws_path) else {"settings": {"enable": True}}

    def sites(prefix):
        return [{"key": f"{prefix}{i}", "name": d, "enable": True, "domains": [d], "type": "proxy",
                 "targets": [f"http://{BE}"], "forwardedHeaders": True, "accessLog": False}
                for i, d in enumerate(names)]
    ws["listeners"] = [
        {"key": "l-http", "name": "http", "enable": True, "network": "tcp", "port": 8080, "tls": False,
         "sites": sites("h"), "default": {"type": "close"}},
        {"key": "l-https", "name": "https", "enable": True, "network": "tcp", "port": 8443, "tls": True,
         "sites": sites("t"), "default": {"type": "close"}},
    ]
    json.dump(ws, open(ws_path, "w"))
    ssl = {"settings": {"enable": False, "renewBeforeDays": 30, "shortLivedRenewHours": 24},
           "certs": [{"key": "c-bench", "remark": "bench", "enable": True, "source": "upload",
                      "certPem": open("/etc/bench/certs/bench.crt").read(),
                      "keyPem": open("/etc/bench/certs/bench.key").read()}]}
    json.dump(ssl, open(os.path.join(out, "ssl.json"), "w"))
    json.dump({"scenePending": False}, open(os.path.join(out, "onboarding.json"), "w"))
else:
    raise SystemExit("未知程序 " + tool)
