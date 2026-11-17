# 生成 Gately 配置（不计入启动时间）：先空跑镜像生成默认配置（管理员账号、密钥等），再写入压测站点与 WAF 实例
set -e
D=$(cd "$(dirname "$0")" && pwd)
IMG=${GATELY_IMAGE:-ghcr.io/wangwanjie/gately:v0.6.8.1}
CFG=$WORKDIR/sut-waf-gately-$WAF
rm -rf $CFG && mkdir -p $CFG
docker run --rm --entrypoint sh -v $CFG:/etc/gately $IMG -c 'timeout 3 gately >/dev/null 2>&1; true'
python3 $D/gen.py $CFG
