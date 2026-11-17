# nginx + ModSecurity v3.0.16 + CRS 4.25.0（官方镜像 owasp/modsecurity-crs 的 nginx-alpine 变体）
# 与 Gately 对齐的设置：拦截模式、阻断偏执等级 1、入站阈值 5、出站阈值 4、检查请求体（上限 12.5 MB，超限拒绝）、
# 不检查响应体、关闭审计日志（Gately 没有审计日志）。关闭镜像自带的健康检查（每次在容器内执行 curl，会计入被测 cgroup）。其余保持镜像默认。
D=$(cd "$(dirname "$0")" && pwd)
IMG=${MODSEC_IMAGE:-owasp/modsecurity-crs:4.25.0-nginx-alpine-202606290906}
if [ "$WAF" = "on" ]; then
  CONF="modsecurity on; modsecurity_rules_file /etc/modsecurity.d/setup.conf;"
else
  CONF=""
fi
docker run -d --name $SUT_NAME --network host --cpuset-cpus $SUT_CPUS --ulimit nofile=200000:200000 --no-healthcheck \
  -e BENCH_HP=$HP -e BENCH_SP=$SP -e BENCH_BACKEND=$BACKEND -e "BENCH_MODSEC_CONF=$CONF" \
  -e PORT=$HP -e SSL_PORT=$SP \
  -e MODSEC_RULE_ENGINE=on -e BLOCKING_PARANOIA=1 -e DETECTION_PARANOIA=1 -e ANOMALY_INBOUND=5 -e ANOMALY_OUTBOUND=4 \
  -e MODSEC_REQ_BODY_ACCESS=on -e MODSEC_REQ_BODY_LIMIT=13107200 -e MODSEC_REQ_BODY_LIMIT_ACTION=Reject \
  -e MODSEC_RESP_BODY_ACCESS=off -e MODSEC_AUDIT_ENGINE=off \
  -v $D/nginx.conf.template:/etc/nginx/templates/nginx.conf.template:ro \
  -v $CERTS:/certs:ro $IMG
