# Gately 官方镜像，默认配置之外只加两个监听器与一张文件证书
D=$(dirname "$0")
IMG=${GATELY_IMAGE:-ghcr.io/wangwanjie/gately:v0.6.8.1}
rm -rf /tmp/sut-gately && mkdir -p /tmp/sut-gately
# 先空跑一次生成默认配置（管理员账号、密钥等），再写入压测站点
docker run --rm --entrypoint sh -v /tmp/sut-gately:/etc/gately $IMG -c 'timeout 3 gately >/dev/null 2>&1; true'
python3 $D/gen.py /tmp/sut-gately
docker run -d --name sut --network host --cpuset-cpus 0,1 --ulimit nofile=200000:200000 \
  -v /tmp/sut-gately:/etc/gately -v /etc/bench/certs:/certs:ro $IMG
