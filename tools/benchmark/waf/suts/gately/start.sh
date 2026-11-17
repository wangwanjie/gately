# 启动 Gately（配置由 prepare.sh 生成）；ADMIN_PORT 非空时改管理端口，避免本机调试时与其他实例冲突
IMG=${GATELY_IMAGE:-ghcr.io/wangwanjie/gately:v0.6.8.1}
CFG=$WORKDIR/sut-waf-gately-$WAF
ARGS=""
if [ -n "$ADMIN_PORT" ]; then
  ARGS="-listen :$ADMIN_PORT"
fi
docker run -d --name $SUT_NAME --network host --cpuset-cpus $SUT_CPUS --ulimit nofile=200000:200000 \
  -v $CFG:/etc/gately $IMG $ARGS
