#!/usr/bin/env python3
"""生成 Gately WAF 对比测试的配置：在 tools/benchmark/suts/gately/gen.py 的基础上，
按环境变量 WAF=on/off 决定站点是否引用 WAF 实例。用法：gen.py <配置目录>

WAF 实例与管理界面“新建 WAF 实例”的默认值一致：拦截模式、偏执等级 1、入站异常分数阈值 5、
出站阈值 4、一组启用全部 CRS 规则文件的规则组、请求体检查上限默认 12.5 MB（超限拒绝）、
正则预过滤开启。端口、源站与证书目录由环境变量 HP/SP/BACKEND/CERTS 指定。
"""
import json
import os
import sys

d = sys.argv[1]
waf_on = os.environ.get("WAF", "on") == "on"
hp, sp = int(os.environ.get("HP", "8080")), int(os.environ.get("SP", "8443"))
backend = os.environ.get("BACKEND", "127.0.0.1:9000")
certs = os.environ.get("CERTS", "/etc/bench/certs")
os.makedirs(d, exist_ok=True)


def site(key):
    s = {"key": key, "name": "bench", "enable": True, "domains": ["bench.test"], "type": "proxy",
         "targets": ["http://" + backend], "forwardedHeaders": True, "accessLog": False}
    if waf_on:
        s["waf"] = "w-bench"
    return s


ws_path = os.path.join(d, "webservice.json")
ws = json.load(open(ws_path)) if os.path.exists(ws_path) else {"settings": {"enable": True}}
ws["settings"]["enable"] = True
ws["listeners"] = [
    {"key": "l-http", "name": "http", "enable": True, "network": "tcp", "listenIp": "", "port": hp,
     "tls": False, "sites": [site("s-http")], "default": {"type": "close"}},
    {"key": "l-https", "name": "https", "enable": True, "network": "tcp", "listenIp": "", "port": sp,
     "tls": True, "sites": [site("s-https")], "default": {"type": "close"}},
]
json.dump(ws, open(ws_path, "w"), indent=2)
ssl = {"settings": {"enable": False, "renewBeforeDays": 30, "shortLivedRenewHours": 24},
       "certs": [{"key": "c-bench", "remark": "bench", "enable": True, "source": "upload",
                  "certPem": open(os.path.join(certs, "bench.crt")).read(),
                  "keyPem": open(os.path.join(certs, "bench.key")).read()}]}
json.dump(ssl, open(os.path.join(d, "ssl.json"), "w"), indent=2)
# WAF 实例始终写入（关闭 WAF 时只是站点不引用它），两种模式下加载的程序与模块相同
waf = {"instances": [{
    "key": "w-bench", "name": "bench", "group": "", "enable": True, "mode": "block", "paranoia": 1,
    "inboundThreshold": 5, "outboundThreshold": 4,
    "ruleSets": [{"enable": True, "remark": "CRS", "type": "crs", "param": ""}],
    "bodyLimitMB": 0, "bodyLimitAction": "reject", "skipBodyPaths": "", "webdav": False,
    "regexPrefilter": os.environ.get("GATELY_PREFILTER", "on") == "on", "remark": ""}]}
if not waf_on:
    # 关闭 WAF 的对照组不加载规则，避免规则编译占用的内存计入“关闭 WAF”的基线
    waf = {"instances": []}
json.dump(waf, open(os.path.join(d, "waf.json"), "w"), indent=2)
# 跳过首次登录的使用场景向导（不影响服务）
json.dump({"scenePending": False}, open(os.path.join(d, "onboarding.json"), "w"))
