# Caddy v2.11.4 + coraza-caddy v2.6.2（Coraza v3.8.1、CRS 4.25.0），镜像由 build.sh 构建
D=$(cd "$(dirname "$0")" && pwd)
IMG=${CADDY_CORAZA_IMAGE:-bench/caddy-coraza:2.11.4-c2.6.2}
docker run -d --name $SUT_NAME --network host --cpuset-cpus $SUT_CPUS --ulimit nofile=200000:200000 \
  -e BENCH_HP=$HP -e BENCH_SP=$SP -e BENCH_BACKEND=$BACKEND \
  -v $D/Caddyfile:/etc/caddy/Caddyfile:ro -v $D/waf-$WAF.caddy:/etc/caddy/waf.caddy:ro \
  -v $CERTS:/certs:ro $IMG
