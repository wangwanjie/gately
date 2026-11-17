D=$(dirname "$0")
docker run -d --name sut --network host --cpuset-cpus 0,1 --ulimit nofile=200000:200000 \
  -v $(realpath $D)/traefik.yml:/etc/traefik/traefik.yml:ro -v $(realpath $D)/dynamic.yml:/etc/traefik/dynamic.yml:ro \
  -v /etc/bench/certs:/certs:ro traefik:v3
