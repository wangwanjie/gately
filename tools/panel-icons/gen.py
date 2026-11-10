#!/usr/bin/env python3
# 生成导航面板的内置图标库 internal/webpage/panelicons.json。
# 品牌图标取自 Simple Icons（CC0-1.0），通用图标取自 Lucide（ISC），Gately 标志为项目自有。
# 用法：先下载两个包（npm pack simple-icons lucide-static 并解压），再运行
#   python3 tools/panel-icons/gen.py <simple-icons/package 目录> <lucide-static/package 目录>
# 输出：每个图标为 {id, name, cat, color, kind, body}，body 为 24x24 视图内的 SVG 元素（不含外层 <svg>）；
# kind 为 brand（填充）或 line（描边），渲染时由 internal/webpage/panelicons.go 包上外层 <svg> 并按底色选白色或深色图形。
import json, re, sys, os

si_dir, lu_dir = sys.argv[1], sys.argv[2]
root = os.path.dirname(os.path.dirname(os.path.dirname(os.path.abspath(__file__))))
out_path = os.path.join(root, 'internal', 'webpage', 'panelicons.json')

si_meta = {x['slug']: x for x in json.load(open(os.path.join(si_dir, 'data', 'simple-icons.json')))}

def brand(slug, name=None, color=None):
    svg = open(os.path.join(si_dir, 'icons', slug + '.svg')).read()
    paths = re.findall(r'<path d="([^"]+)"', svg)
    assert paths, slug
    body = ''.join(f'<path d="{d}"/>' for d in paths)
    m = si_meta[slug]
    return {'kind': 'brand', 'name': name or m['title'], 'color': '#' + (color or m['hex']).lower(), 'body': body}

def line(name_file, name, color):
    svg = open(os.path.join(lu_dir, 'icons', name_file + '.svg')).read()
    inner = svg[svg.index('>', svg.index('<svg')) + 1: svg.rindex('</svg>')]
    inner = re.sub(r'\s+', ' ', inner).replace('> <', '><').strip()
    inner = re.sub(r' />', '/>', inner)
    return {'kind': 'line', 'name': name, 'color': color, 'body': inner}

# Gately 标志：取自管理后台图标（32x32 视图），缩放到 24x24
GATELY = {'kind': 'brand', 'name': 'Gately', 'color': '#0e7c66',
          'body': '<g transform="scale(.75)"><path d="M21.7 10.3A8 8 0 1 0 24 16h-7" fill="none" stroke="currentColor" stroke-width="3" stroke-linecap="round" stroke-linejoin="round"/><circle cx="23.6" cy="8.6" r="2.2"/></g>'}

# 分类：gately 为 Gately 与周边模块，apps 为常用自托管应用，system 为系统与网络设备，generic 为通用图标
spec = [
    ('gately', [
        ('gately', lambda: GATELY),
        ('frp', lambda: line('waypoints', 'frp', '#2563eb')),
        ('stun', lambda: line('radar', 'STUN', '#0e7490')),
        ('ddns', lambda: line('globe', 'DDNS', '#0284c7')),
        ('portforward', lambda: line('arrow-left-right', '端口转发', '#7c3aed')),
        ('webservice', lambda: line('server', 'Web 服务', '#0e7c66')),
        ('waf', lambda: line('shield-half', 'WAF', '#dc2626')),
        ('ssl', lambda: line('shield-check', 'SSL 证书', '#15803d')),
        ('guard', lambda: line('shield-ban', '入侵防护', '#b91c1c')),
        ('webdav', lambda: line('folder-sync', 'WebDAV', '#ea580c')),
        ('smb', lambda: line('folder-tree', 'SMB', '#b45309')),
        ('ftp', lambda: line('hard-drive-upload', 'FTP', '#0f766e')),
        ('filebrowser', lambda: line('folder-open', 'File Browser', '#2563eb')),
        ('storage', lambda: line('hard-drive', '存储', '#475569')),
        ('terminal', lambda: line('square-terminal', 'Web 终端', '#1f2937')),
        ('wol', lambda: line('power', '网络唤醒', '#16a34a')),
        ('docker', lambda: brand('docker')),
        ('tailscale', lambda: brand('tailscale', color='242424')),
        ('netbird', lambda: line('bird', 'NetBird', '#f68330')),
        ('cloudflare', lambda: brand('cloudflare')),
        ('wireguard', lambda: brand('wireguard')),
        ('zerotier', lambda: brand('zerotier', color='e8a01b')),
    ]),
    ('apps', [(s, (lambda s=s: brand(s))) for s in [
        'jellyfin', 'emby', 'plex', 'kodi', 'immich', 'nextcloud', 'syncthing', 'alist', 'qbittorrent', 'transmission',
        'homeassistant', 'nodered', 'esphome', 'zigbee2mqtt', 'frigate', 'vaultwarden', 'bitwarden', 'portainer',
        'grafana', 'prometheus', 'influxdb', 'uptimekuma', 'gitea', 'github', 'gitlab', 'adguard', 'pihole', 'nginx',
        'paperlessngx', 'calibreweb', 'radarr', 'sonarr', 'ollama', 'minio', 'mariadb', 'mysql', 'postgresql', 'redis',
        'authelia', 'authentik', 'keycloak', 'homepage', 'speedtest']]),
    ('system', [(s, (lambda s=s: brand(s))) for s in [
        'openwrt', 'synology', 'truenas', 'unraid', 'proxmox', 'openmediavault', 'ubuntu', 'debian', 'alpinelinux']]),
    ('generic', [(n, (lambda n=n, label=label, c=c: line(n, label, c))) for n, label, c in [
        ('house', '主页', '#0e7c66'), ('router', '路由器', '#2563eb'), ('server', '服务器', '#475569'), ('hard-drive', '硬盘', '#64748b'),
        ('monitor', '电脑', '#4f46e5'), ('laptop', '笔记本', '#6366f1'), ('smartphone', '手机', '#0ea5e9'), ('tv', '电视', '#7c3aed'),
        ('camera', '摄像头', '#db2777'), ('printer', '打印机', '#64748b'), ('cloud', '云', '#0284c7'), ('globe', '网站', '#0e7490'),
        ('mail', '邮件', '#ea580c'), ('message-circle', '聊天', '#16a34a'), ('music', '音乐', '#e11d48'), ('film', '影视', '#7c3aed'),
        ('image', '相册', '#d97706'), ('download', '下载', '#2563eb'), ('book-open', '书籍', '#b45309'), ('code', '代码', '#1f2937'),
        ('database', '数据库', '#0f766e'), ('shield', '安全', '#15803d'), ('lock', '密码', '#b91c1c'), ('key-round', '密钥', '#ca8a04'),
        ('chart-line', '监控', '#2563eb'), ('settings', '设置', '#475569'), ('gamepad-2', '游戏', '#9333ea'), ('wifi', 'Wi-Fi', '#0284c7'),
        ('network', '网络', '#0e7490'), ('cpu', '硬件', '#4f46e5'), ('bot', 'AI', '#0e7c66'), ('calendar', '日历', '#dc2626'),
        ('file-text', '文档', '#475569'), ('notebook-pen', '笔记', '#ca8a04'), ('rss', '订阅', '#ea580c'), ('link', '链接', '#2563eb'),
    ]]),
]

icons = []
for cat, items in spec:
    for id_, fn in items:
        it = fn()
        icons.append({'id': id_, 'name': it['name'], 'cat': cat, 'color': it['color'], 'kind': it['kind'], 'body': it['body']})
ids = [i['id'] for i in icons]
assert len(ids) == len(set(ids)), 'duplicate id'
with open(out_path, 'w') as f:
    json.dump(icons, f, ensure_ascii=False, indent=1)
    f.write('\n')
print(len(icons), 'icons ->', out_path, os.path.getsize(out_path), 'bytes')
