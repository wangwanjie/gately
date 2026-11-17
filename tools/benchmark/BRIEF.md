# 压测环境约定（编写专项测试脚本时遵守）

- 压测机：B760 上的 KVM 虚拟机 Debian 13（bench-debian，192.168.18.49），8 vCPU 绑定宿主机 i5-13500T 4 个性能核的 8 个线程（宿主机 CPU 4-11），8 GiB 内存，Docker 29。
- CPU 划分（所有脚本必须遵守）：被测程序 vCPU 0,1（`docker run --cpuset-cpus 0,1`）；源站 nginx vCPU 2,3（容器名 `backend`，已常驻，`127.0.0.1:9000`，`/` 返回 1024 字节，`/1m.bin`、`/100m.bin`）；压测/攻击工具 vCPU 4-7（`taskset -c 4-7`）。
- 已装工具：wrk 4.1、oha 1.17、h2load 1.64、python3、curl、jq、nftables、iptables(nft 后端)、ipset。容器统一 `--network host`。
- 证书：`/etc/bench/certs/bench.crt|bench.key|bench.pem`（ECDSA P-256，CN/SAN bench.test、*.bench.test），`/etc/hosts` 已有 `127.0.0.1 bench.test auth.bench.test`。
- 采样工具：`/root/benchmark/lib/sample.py snapshot <cgroup>` / `watch <cgroup> <秒> <输出json>`，容器 cgroup 为 `/sys/fs/cgroup/system.slice/docker-<完整ID>.scope`；内存口径为进程 PSS 之和，CPU 为 cgroup usage_usec 差值。可参考 `/root/benchmark/lib/run.py`（本仓库 tools/benchmark/lib/run.py）。
- 压测机所在网络拉取 Docker Hub 很慢，镜像在别处用 crane 拉取后 `docker load`；脚本里用到的镜像写成固定 tag 并在 README 列出。已有：nginx:1.29-alpine、haproxy:3.2-alpine、caddy:2-alpine、traefik:v3、ghcr.io/wangwanjie/gately:v0.6.8.1、gdy666/lucky:v3.8.1_lucky、jc21/nginx-proxy-manager:2.17.0、authelia/authelia:4.39.28、kcilnk/fn-knock:2.5.11、jeessy/ddns-go:v6.17.9。apt 走 mirrors.ustc.edu.cn 可正常装包。
- Gately 被测配置方式参考 `tools/benchmark/suts/gately/gen.py`、`start.sh`：先空跑镜像生成默认配置，再直接写各模块 JSON（结构见 internal/modules/<模块>/config.go 或 docs/api/API.md），`onboarding.json` 写 `{"scenePending": false}`。默认管理员 admin / gately.cn，管理端 :16680。
- 结果写 JSON 到 `/root/benchmark/results/<专项>-<程序>.json`，口径与 run.py 一致（rps、p50ms、p99ms、sutCpuPercent、peakPssKiB、idle PSS 等）。
- 本机调试：可以在 colima 等本地 Docker 环境（arm64 也可）先跑通功能与脚本；涉及防火墙（nft/iptables）的调试放在独立网络命名空间的特权容器里做，不要改宿主机的防火墙。调试数据不要写进 `results/`。
- 代码约定：注释用中文；if/else/循环必须带大括号且受控语句另起一行；Python 同样保持清晰注释。
