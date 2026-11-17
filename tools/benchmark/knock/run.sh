#!/bin/bash
# 端口敲门专项对比入口（压测机上以 root 运行）：bash /root/benchmark/knock/run.sh [被测名 ...]
# 被测名：direct gately gately-web fwknop fnknock，默认全部依次运行。可用环境变量调整，见 README.md。
set -e
D=$(cd "$(dirname "$0")" && pwd)
[ "$(id -u)" = 0 ] || { echo "需要 root" >&2; exit 1; }
for c in docker nft iptables ip taskset wrk oha python3 curl ss; do
  command -v $c >/dev/null || { echo "缺少命令 $c" >&2; exit 1; }
done
command -v nmap >/dev/null || echo "提示：未安装 nmap，隐蔽性测试只用自带的 TCP/UDP 探测" >&2
for img in ${GATELY_IMAGE:-ghcr.io/wangwanjie/gately:v0.6.8.1} ${FNK_IMAGE:-kcilnk/fn-knock:2.5.11} nginx:1.29-alpine; do
  docker image inspect "$img" >/dev/null 2>&1 || { echo "缺少镜像 $img" >&2; exit 1; }
done
# fwknop 镜像在本机构建（基础镜像 debian:trixie-slim，apt 走 USTC 镜像源）
FWK=${FWK_IMAGE:-bench-fwknop:2.6.11}
if ! docker image inspect "$FWK" >/dev/null 2>&1; then
  docker build -t "$FWK" "$D/fwknop"
fi
exec python3 "$D/bench.py" "$@"
