# Nginx Proxy Manager 官方镜像：只能监听 80/443，管理接口在 81
# 用 REST API 上传证书并建一个同时服务 HTTP 与 HTTPS 的代理主机
IMG=${NPM_IMAGE:-jc21/nginx-proxy-manager:2.17.0}
BACKEND_HOST=${BACKEND_HOST:-127.0.0.1}
BACKEND_PORT=${BACKEND_PORT:-9000}
CERTS=${CERTS:-/etc/bench/certs}
rm -rf /tmp/sut-npm && mkdir -p /tmp/sut-npm/data /tmp/sut-npm/le
# NPM_KEEPALIVE_REQUESTS 非空时写入 NPM 的自定义 http 配置：nginx 默认每条连接 1000 个请求后关闭，
# h2load 不会重连，HTTP/2 测试只在这一项上放宽（与 suts/nginx 的写法一致），其余测试保持 NPM 默认
if [ -n "$NPM_KEEPALIVE_REQUESTS" ]; then
  mkdir -p /tmp/sut-npm/data/nginx/custom
  echo "keepalive_requests $NPM_KEEPALIVE_REQUESTS;" > /tmp/sut-npm/data/nginx/custom/http.conf
fi
docker run -d --name sut --network host --cpuset-cpus ${SUT_CPUS:-0,1} --ulimit nofile=200000:200000 \
  -e INITIAL_ADMIN_EMAIL=admin@bench.test -e INITIAL_ADMIN_PASSWORD=Bench12345 -e DISABLE_IPV6=true \
  -v /tmp/sut-npm/data:/data -v /tmp/sut-npm/le:/etc/letsencrypt $IMG >/dev/null
for i in $(seq 1 300); do
  T=$(curl -s --max-time 2 http://127.0.0.1:81/api/tokens -H 'Content-Type: application/json' \
    -d '{"identity":"admin@bench.test","secret":"Bench12345"}' | sed -n 's/.*"token":"\([^"]*\)".*/\1/p')
  [ -n "$T" ] && break
  sleep 0.5
done
H="Authorization: Bearer $T"
CID=$(curl -s http://127.0.0.1:81/api/nginx/certificates -H "$H" -H 'Content-Type: application/json' \
  -d '{"provider":"other","nice_name":"bench"}' | sed -n 's/.*"id":\([0-9]*\).*/\1/p' | head -1)
curl -s -o /dev/null http://127.0.0.1:81/api/nginx/certificates/$CID/upload -H "$H" \
  -F certificate=@$CERTS/bench.crt -F certificate_key=@$CERTS/bench.key
# 关闭该站点的访问日志，与其他被测程序一致（NPM 默认按站点写访问日志）
curl -s http://127.0.0.1:81/api/nginx/proxy-hosts -H "$H" -H 'Content-Type: application/json' \
  -d "{\"domain_names\":[\"bench.test\"],\"forward_scheme\":\"http\",\"forward_host\":\"$BACKEND_HOST\",\"forward_port\":$BACKEND_PORT,\"certificate_id\":$CID,\"ssl_forced\":false,\"http2_support\":true,\"enabled\":true,\"advanced_config\":\"access_log off;\"}" | head -c 200; echo
