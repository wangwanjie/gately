#!/usr/bin/env python3
"""从 summary.json 生成 README 用的静态 SVG 图（小多图：每个指标一个横向条形图面板）。

配色与官网 BenchBars 组件一致：Gately 用品牌色突出，其他程序用中性灰；每根条直接标出程序名与数值，
颜色不是唯一的识别方式。SVG 内用 prefers-color-scheme 切换深浅色。
用法：charts.py <summary.json> <输出目录> [语言 zh|en]
"""
import html
import json
import os
import sys

summary = json.load(open(sys.argv[1]))
out = sys.argv[2]
lang = sys.argv[3] if len(sys.argv) > 3 else "en"
os.makedirs(out, exist_ok=True)

T = {
    "zh": {
        "proxy_title": "反向代理性能（2 vCPU，同一台 Debian 13 虚拟机）",
        "https": "HTTPS 请求/秒（keep-alive）", "tls": "TLS 新建连接/秒", "idle": "空闲内存 MiB（越小越好）",
        "h2": "HTTP/2 请求/秒", "higher": "越大越好", "lower": "越小越好", "na": "未测",
        "foot": "数据与脚本：tools/benchmark · 详见 gately.cn/docs/guide/benchmarks",
    },
    "en": {
        "proxy_title": "Reverse proxy performance (2 vCPU, same Debian 13 VM)",
        "https": "HTTPS requests/s (keep-alive)", "tls": "New TLS connections/s", "idle": "Idle memory MiB (lower is better)",
        "h2": "HTTP/2 requests/s", "higher": "higher is better", "lower": "lower is better", "na": "n/a",
        "foot": "Data and scripts: tools/benchmark · gately.cn/en/docs/guide/benchmarks",
    },
}[lang]

STYLE = """
<style>
  .t { font: 600 15px -apple-system, BlinkMacSystemFont, 'Segoe UI', Helvetica, Arial, sans-serif; fill: #1f1f1d; }
  .h { font: 600 13px -apple-system, BlinkMacSystemFont, 'Segoe UI', Helvetica, Arial, sans-serif; fill: #1f1f1d; }
  .n { font: 12px -apple-system, BlinkMacSystemFont, 'Segoe UI', Helvetica, Arial, sans-serif; fill: #52514e; }
  .v { font: 12px ui-monospace, SFMono-Regular, Menlo, monospace; fill: #52514e; }
  .hl .n, .hl .v { fill: #1f1f1d; font-weight: 600; }
  .bar { fill: #b4b3ad; }
  .hl .bar { fill: #0e7c66; }
  .f { font: 11px -apple-system, BlinkMacSystemFont, 'Segoe UI', Helvetica, Arial, sans-serif; fill: #6f6e69; }
  .panel { fill: none; stroke: #e2e1dc; }
  @media (prefers-color-scheme: dark) {
    .t, .h, .hl .n, .hl .v { fill: #f0efec; }
    .n, .v { fill: #c3c2b7; }
    .bar { fill: #55544f; }
    .hl .bar { fill: #1aa886; }
    .f { fill: #9b9a93; }
    .panel { stroke: #3a3a37; }
  }
</style>
"""


def panel(x, y, w, title, rows, better, unit_fmt):
    """rows: [(名称, 数值或 None)]，按更好的方向排序后画横向条形。"""
    rows = sorted(rows, key=lambda r: (r[1] is None, -(r[1] or 0) if better == "higher" else (r[1] or 0)))
    mx = max([v for _, v in rows if v is not None] + [1])
    name_w, val_w, row_h = 128, 70, 24
    bar_max = w - name_w - val_w - 24
    h = 40 + row_h * len(rows) + 8
    s = [f'<rect class="panel" x="{x}" y="{y}" width="{w}" height="{h}" rx="10"/>',
         f'<text class="h" x="{x + 14}" y="{y + 24}">{html.escape(title)}</text>']
    for i, (name, v) in enumerate(rows):
        cy = y + 40 + i * row_h
        cls = ' class="hl"' if name == "Gately" else ""
        s.append(f"<g{cls}>")
        s.append(f'<text class="n" x="{x + name_w}" y="{cy + 12}" text-anchor="end">{html.escape(name)}</text>')
        if v is None:
            s.append(f'<text class="v" x="{x + name_w + 10}" y="{cy + 12}">{T["na"]}</text>')
        else:
            bw = max(bar_max * v / mx, 3)
            bx = x + name_w + 10
            # 4px 圆角只在数据端：左侧贴基线的方角用一个窄矩形补齐
            s.append(f'<rect class="bar" x="{bx}" y="{cy + 3}" width="{bw:.1f}" height="13" rx="4"/>')
            s.append(f'<rect class="bar" x="{bx}" y="{cy + 3}" width="{min(bw, 4):.1f}" height="13"/>')
            s.append(f'<text class="v" x="{bx + bw + 6:.1f}" y="{cy + 12}">{unit_fmt(v)}</text>')
        s.append("</g>")
    return s, h


def num(v):
    return f"{v:,.0f}"


def mib(v):
    return f"{v:,.1f}"


def proxy_chart():
    p = summary["proxy"]
    order = list(p.keys())
    W, pw, gap = 960, 468, 24
    items = [
        (T["https"], [(p[k]["name"], p[k]["httpsRps"]) for k in order], "higher", num),
        (T["tls"], [(p[k]["name"], p[k]["tlsNewRps"]) for k in order], "higher", num),
        (T["h2"], [(p[k]["name"], p[k]["h2Rps"]) for k in order], "higher", num),
        (T["idle"], [(p[k]["name"], p[k]["idleMiB"]) for k in order], "lower", mib),
    ]
    body, y, heights = [], 50, []
    for i, (title, rows, better, fmt) in enumerate(items):
        x = 0 if i % 2 == 0 else pw + gap
        if i % 2 == 0 and i > 0:
            y += max(heights[-2:]) + gap
        s, h = panel(x + 1, y, pw - 2, title, rows, better, fmt)
        heights.append(h)
        body += s
    H = y + max(heights[-2:]) + 34
    svg = [f'<svg xmlns="http://www.w3.org/2000/svg" width="{W}" height="{H}" viewBox="0 0 {W} {H}" role="img" '
           f'aria-label="{html.escape(T["proxy_title"])}">', STYLE,
           f'<text class="t" x="2" y="24">{html.escape(T["proxy_title"])}</text>'] + body + \
          [f'<text class="f" x="2" y="{H - 10}">{html.escape(T["foot"])}</text>', "</svg>"]
    name = "benchmark-proxy.svg" if lang == "en" else f"benchmark-proxy.{lang}.svg"
    open(os.path.join(out, name), "w").write("\n".join(svg) + "\n")
    print(os.path.join(out, name))


proxy_chart()
