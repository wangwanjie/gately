#!/bin/bash
# 压测虚拟机初始化：系统参数、测试证书、源站文件与后端容器
set -e
cat > /etc/sysctl.d/90-bench.conf <<'S'
net.core.somaxconn = 65535
net.ipv4.tcp_max_syn_backlog = 65535
net.ipv4.ip_local_port_range = 1024 65000
net.ipv4.tcp_tw_reuse = 1
net.core.netdev_max_backlog = 65535
fs.file-max = 2000000
fs.nr_open = 2000000
S
sysctl -q --system
grep -q bench.test /etc/hosts || echo "127.0.0.1 bench.test auth.bench.test" >> /etc/hosts
cat > /etc/security/limits.d/90-bench.conf <<'S'
* soft nofile 1000000
* hard nofile 1000000
root soft nofile 1000000
root hard nofile 1000000
S
mkdir -p /srv/bench /etc/bench/certs
cd /srv/bench
head -c 1024 /dev/zero | tr '\0' 'a' > 1k.txt
head -c 1048576 /dev/urandom > 1m.bin
head -c 104857600 /dev/urandom > 100m.bin
cd /etc/bench/certs
if [ ! -f bench.crt ]; then
  # 所有被测程序共用同一张 ECDSA P-256 自签名证书，TLS 成本一致
  openssl req -x509 -newkey ec -pkeyopt ec_paramgen_curve:P-256 -nodes -days 3650 \
    -subj "/CN=bench.test" -addext "subjectAltName=DNS:bench.test,DNS:*.bench.test" \
    -keyout bench.key -out bench.crt
  cat bench.crt bench.key > bench.pem
fi
docker rm -f backend 2>/dev/null || true
docker run -d --name backend --network host --cpuset-cpus 2,3 --ulimit nofile=200000:200000 \
  -v /root/benchmark/backend/nginx.conf:/etc/nginx/nginx.conf:ro -v /srv/bench:/srv/bench:ro nginx:1.29-alpine
sleep 1
curl -s -o /dev/null -w "backend %{http_code} %{size_download}\n" http://127.0.0.1:9000/
