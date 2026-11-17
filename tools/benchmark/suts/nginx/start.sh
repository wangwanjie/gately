D=$(dirname "$0")
docker run -d --name sut --network host --cpuset-cpus 0,1 --ulimit nofile=200000:200000 \
  -v $(realpath $D)/nginx.conf:/etc/nginx/nginx.conf:ro -v /etc/bench/certs:/certs:ro nginx:1.29-alpine
