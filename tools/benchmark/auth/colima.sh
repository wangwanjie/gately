#!/bin/bash
# 本机 colima 试跑（只用于验证脚本、功能与口径，数值不作结论）：bash tools/benchmark/auth/colima.sh [被测名,...]
# 做法：在 colima 虚拟机里起一个装有 wrk/oha/python3 的 runner 容器（host 网络、共享宿主 PID 与 cgroup），
# 由它通过 Docker 套接字启动被测；容器名统一 bx- 前缀，源站改用 127.0.0.1:19100（colima 的 9000 已被占用），
# Gately 管理端口改为 127.0.0.1:16681（16680 已被占用）。DUR/ROUNDS 默认缩短。
set -e
REPO=$(cd "$(dirname "$0")/.." && pwd)
W=/tmp/bx-auth
RUNNER=bx-auth-runner:2
docker image inspect $RUNNER >/dev/null 2>&1 || docker build -q -t $RUNNER - <<'DF'
FROM alpine:3.22
RUN apk add --no-cache wrk python3 py3-cryptography curl docker-cli util-linux openssl jq bash && \
    apk add --no-cache oha --repository https://dl-cdn.alpinelinux.org/alpine/edge/testing
DF
# 证书、源站文件与源站配置（与 provision-debian.sh 相同，只改端口）
docker run --rm -v $W:$W -v $REPO:$REPO $RUNNER sh -c "
  set -e; mkdir -p $W/certs $W/srv $W/results
  cd $W/certs; [ -f bench.crt ] || openssl req -x509 -newkey ec -pkeyopt ec_paramgen_curve:P-256 -nodes -days 30 \
    -subj /CN=bench.test -addext subjectAltName=DNS:bench.test,DNS:*.bench.test -keyout bench.key -out bench.crt 2>/dev/null
  cd $W/srv; [ -f 1k.txt ] || head -c 1024 /dev/zero | tr '\0' a > 1k.txt
  sed 's/127.0.0.1:9000/127.0.0.1:19100/' $REPO/backend/nginx.conf > $W/backend.conf"
docker rm -f bx-auth-backend >/dev/null 2>&1 || true
docker run -d --name bx-auth-backend --network host --cpuset-cpus 2,3 \
  -v $W/backend.conf:/etc/nginx/nginx.conf:ro -v $W/srv:/srv/bench:ro nginx:1.29-alpine >/dev/null
docker run --rm --privileged --pid host --cgroupns host --network host \
  --add-host bench.test:127.0.0.1 --add-host auth.bench.test:127.0.0.1 \
  -v /var/run/docker.sock:/var/run/docker.sock -v /sys/fs/cgroup:/sys/fs/cgroup \
  -v $REPO:$REPO -v $W:$W \
  -e NAME_PREFIX=bx- -e AUTH_WORK=$W/work -e CERT_DIR=$W/certs -e BACKEND=127.0.0.1:19100 \
  -e SUT_CPUS=0,1 -e LOAD_CPUS=4,5 -e GATELY_ADMIN=127.0.0.1:16681 -e RESULTS=$W/results \
  -e DUR=${DUR:-8} -e ROUNDS=${ROUNDS:-1} -e BAD_DUR=${BAD_DUR:-6} \
  $RUNNER bash $REPO/auth/run.sh "$@" || rc=$?
docker rm -f bx-auth-backend bx-auth-sut bx-auth-authelia >/dev/null 2>&1 || true
# 结果在 colima 虚拟机的 $W/results，复制到本机同名目录便于查看
mkdir -p $W/results
for f in $(docker run --rm -v $W:$W $RUNNER sh -c "ls $W/results"); do
  docker run --rm -v $W:$W $RUNNER cat $W/results/$f > $W/results/$f
done
exit ${rc:-0}
