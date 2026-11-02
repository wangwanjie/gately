# README 展示素材生成

生成 `docs/images/<语言>/*.webp`（10 种语言的界面截图与拓扑动图）和功能巡览视频。脚本用本机 Chrome 驱动演示实例截图，演示实例跑在 colima 的 `gately-demo` 虚拟机里。

## 演示环境

- 虚拟机 `colima start -p gately-demo`（Ubuntu arm64，主机名 nas-gately）。Gately 在 `/opt/gately`，监听 `16750`，账号 `admin` / `Gately-Demo-2026`。
- 数据目录有两份：`data-zh`（中文名称的演示数据）和 `data-en`（`map.txt` 译成英文名称，供非中文语言使用）。`switch.sh zh|en` 切换并重启，统计数据会清零。
- DDNS 成功状态靠 `dnsmock.py`（Cloudflare / 阿里云接口模拟）：在 netns `mock` 里监听 10.99.0.2:443，宿主侧网卡命名为 `vethmock`，Gately 不会把它列进网卡信息。
- 访客流量用 `traffic.sh <分钟>`，通过 PROXY 协议伪造来源 IP。统计图需要先跑 20 分钟以上。
- 虚拟机里不要用 `pkill -f 关键字`（会杀掉 ssh 会话自身），用 `pkill -x 进程名`。
- 配置历史页的对比图用 `hist.py` 生成干净的 webservice 历史版本。

## 生成流程

1. `npm install`（只需 `playwright-core`），然后切到对应数据并跑流量。
2. 截图：`./runA.sh <语言...>` 生成除终端外的页面，`./runTerm.sh <语言...>` 逐语言重启后截终端页（终端会话会累积标签页）。
3. 拓扑动图：`./topoAll.sh <语言...>`，输出 `/tmp/gs/vid/<语言>/topology.webp`。
4. 巡览视频：`node tour.mjs en|zh <输出目录>`，再用 ffmpeg 转 mp4：`ffmpeg -i tour.webm -an -c:v libx264 -preset slow -crf 27 -pix_fmt yuv420p -movflags +faststart gately-tour.<en|zh>.mp4`。README 里的视频由 GitHub 托管播放：在任一 issue 评论框粘贴 mp4（10 MB 以内）拿到 `https://github.com/user-attachments/assets/...` 链接，不提交评论，把链接单独放一行替换 README 顶部的旧链接；`website/public/videos/` 里的同名文件只供官网使用。
5. 截图转 webp：`cwebp -q 78 -resize 2000 1250 in.png -o out.webp`。

脚本里的路径（`/tmp/gs/...`）按本机习惯写死，换机器时先全局替换。
