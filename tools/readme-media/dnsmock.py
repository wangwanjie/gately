# 演示用 DNS 服务商 API 模拟：Cloudflare v4 与阿里云 DNS，记录保存在内存中
import json, ssl, http.server, urllib.parse, itertools
recs = {}  # (name, type) -> {id, content, ttl}
ids = itertools.count(1000)

class H(http.server.BaseHTTPRequestHandler):
    def log_message(self, *a):
        pass

    def send(self, obj, code=200):
        b = json.dumps(obj).encode()
        self.send_response(code)
        self.send_header('Content-Type', 'application/json')
        self.send_header('Content-Length', str(len(b)))
        self.end_headers()
        self.wfile.write(b)

    def body(self):
        n = int(self.headers.get('Content-Length') or 0)
        return json.loads(self.rfile.read(n) or b'{}')

    def cf(self, method):
        u = urllib.parse.urlparse(self.path)
        q = dict(urllib.parse.parse_qsl(u.query))
        p = u.path.replace('/client/v4', '')
        if p == '/zones':
            return self.send({'success': True, 'result': [{'id': 'z-' + q.get('name', ''), 'name': q.get('name', '')}]})
        if p.endswith('/dns_records') and method == 'GET':
            r = recs.get((q.get('name'), q.get('type')))
            out = [{'id': r['id'], 'type': q['type'], 'name': q['name'], 'content': r['content'], 'ttl': r['ttl'], 'proxied': False}] if r else []
            return self.send({'success': True, 'result': out})
        b = self.body()
        if method == 'PATCH':
            rid = p.rsplit('/', 1)[-1]
            for v in recs.values():
                if v['id'] == rid:
                    v['content'] = b.get('content', v['content'])
            return self.send({'success': True, 'result': {}})
        recs[(b['name'], b['type'])] = {'id': str(next(ids)), 'content': b['content'], 'ttl': b['ttl']}
        return self.send({'success': True, 'result': {}})

    def ali(self):
        q = dict(urllib.parse.parse_qsl(urllib.parse.urlparse(self.path).query))
        a = q.get('Action')
        if a == 'DescribeSubDomainRecords':
            r = recs.get((q['SubDomain'], q['Type']))
            lst = [{'RecordId': r['id'], 'RR': '', 'Type': q['Type'], 'Value': r['content'], 'TTL': r['ttl'], 'Line': 'default'}] if r else []
            return self.send({'TotalCount': len(lst), 'DomainRecords': {'Record': lst}})
        name = (q['RR'] + '.' if q['RR'] != '@' else '') + q.get('DomainName', '')
        if a == 'UpdateDomainRecord':
            for k, v in recs.items():
                if v['id'] == q['RecordId']:
                    v['content'] = q['Value']
            return self.send({'RecordId': q['RecordId']})
        recs[(name, q['Type'])] = {'id': str(next(ids)), 'content': q['Value'], 'ttl': int(q.get('TTL', 600))}
        return self.send({'RecordId': 'x'})

    def route(self, method):
        if 'aliyuncs' in (self.headers.get('Host') or ''):
            return self.ali()
        return self.cf(method)

    def do_GET(self):
        self.route('GET')

    def do_POST(self):
        self.route('POST')

    def do_PATCH(self):
        self.route("PATCH")

    def do_DELETE(self):
        self.route("DELETE")

    def do_PUT(self):
        self.route('PUT')

srv = http.server.ThreadingHTTPServer(('0.0.0.0', 443), H)
ctx = ssl.SSLContext(ssl.PROTOCOL_TLS_SERVER)
ctx.load_cert_chain('/opt/gately/demo/mock.crt', '/opt/gately/demo/mock.key')
srv.socket = ctx.wrap_socket(srv.socket, server_side=True)
srv.serve_forever()
