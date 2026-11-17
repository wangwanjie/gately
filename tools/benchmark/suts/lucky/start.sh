# Lucky 官方镜像（默认配置），启动后用管理接口加证书与两个 Web 规则
D=$(dirname "$0")
IMG=${LUCKY_IMAGE:-gdy666/lucky:v3.8.1_lucky}
rm -rf /tmp/sut-lucky && mkdir -p /tmp/sut-lucky
docker run -d --name sut --network host --cpuset-cpus 0,1 --ulimit nofile=200000:200000 \
  -v /tmp/sut-lucky:/app/conf $IMG >/dev/null
python3 $D/setup.py http://127.0.0.1:16601 /etc/bench/certs/bench.crt /etc/bench/certs/bench.key
