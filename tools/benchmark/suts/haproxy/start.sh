D=$(dirname "$0")
docker run -d --name sut --network host --cpuset-cpus 0,1 --ulimit nofile=400000:400000 \
  -v $(realpath $D)/haproxy.cfg:/usr/local/etc/haproxy/haproxy.cfg:ro -v /etc/bench/certs:/certs:ro haproxy:3.2-alpine
