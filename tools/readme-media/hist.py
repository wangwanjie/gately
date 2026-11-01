import json, os, glob, copy
base='/opt/gately/data-en'
cur=json.load(open(base+'/webservice.json'))
d=base+'/history/webservice'
os.makedirs(d, exist_ok=True)
for f in glob.glob(d+'/*'): os.remove(f)
# 找到 rateLimit 已启用且 30/60/300 的站点
def site():
    for l in cur['listeners']:
        for s in l['sites']:
            rl=s.get('rateLimit') or {}
            if rl.get('enable') and rl.get('rate')==30:
                return s
v=[]
def mk(n, fn):
    c=copy.deepcopy(cur); 
    for l in c['listeners']:
        for s in l['sites']:
            rl=s.get('rateLimit') or {}
            if rl.get('enable') and rl.get('rate')==30:
                fn(s)
    names=['20261101T093012.123456789Z','20261101T114530.223456789Z','20261101T142208.323456789Z','20261101T163945.423456789Z','20261101T185512.523456789Z']
    json.dump(c, open(d+'/'+names[n]+'.json','w'), indent=2, ensure_ascii=False)
def f0(s): s['rateLimit'].update(rate=10,burst=20,maxConns=100); s['responseHeaders']=None
def f1(s): s['rateLimit'].update(rate=20,burst=40,maxConns=200); s['responseHeaders']=None
def f2(s): s['rateLimit'].update(rate=20,burst=40,maxConns=200)
def f3(s): s['rateLimit'].update(rate=10,burst=20,maxConns=100); s['responseHeaders']=['Strict-Transport-Security: max-age=31536000']
def f4(s): s['rateLimit'].update(rate=20,burst=40,maxConns=200); s['responseHeaders']=None
for i,fn in enumerate([f0,f1,f2,f3,f4]): mk(i,fn)
print(os.listdir(d), json.dumps(site()['responseHeaders']))
