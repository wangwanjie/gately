D=$(dirname "$0")
docker run -d --name sut --network host --cpuset-cpus 0,1 --ulimit nofile=200000:200000 \
  -v $(realpath $D)/Caddyfile:/etc/caddy/Caddyfile:ro -v /etc/bench/certs:/certs:ro caddy:2-alpine
