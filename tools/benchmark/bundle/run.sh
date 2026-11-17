#!/bin/bash
# 一体化：Gately 同时开反代 + WAF + 网页认证 + 20 个 DDNS 任务 + 入侵防护 + 端口敲门，
# 测启动后空闲内存（预热 120 秒）和 1000 QPS（经 WAF 的 HTTPS 站点）持续 60 秒时的内存与 CPU。
B=/root/benchmark
IMG=ghcr.io/wangwanjie/gately:v0.6.8.1
D=/tmp/sut-bundle
docker rm -f sut >/dev/null 2>&1
rm -rf $D && mkdir -p $D
docker run --rm --entrypoint sh -v $D:/etc/gately $IMG -c 'timeout 3 gately >/dev/null 2>&1; true'
python3 $B/bundle/gen.py $D || exit 1
docker run -d --name sut --network host --cpuset-cpus 0,1 --cap-add NET_ADMIN --ulimit nofile=200000:200000 \
  -v $D:/etc/gately -v /etc/bench/certs:/certs:ro -v /var/log/bench-bundle:/var/log/bench-bundle:ro $IMG >/dev/null
for i in $(seq 1 300); do
  [ "$(curl -sk -o /dev/null -w '%{http_code}' https://bench.test:8443/)" = 200 ] && break
  sleep 0.2
done
CG=/sys/fs/cgroup/system.slice/docker-$(docker inspect -f '{{.Id}}' sut).scope
sleep 120
IDLE=$(python3 $B/lib/sample.py snapshot $CG)
AUTH=$(curl -sk -o /dev/null -w '%{http_code}' https://auth.bench.test:8443/)
python3 $B/lib/sample.py watch $CG 62 /tmp/bundle-watch.json &
LOAD=$(taskset -c 4-7 oha --no-tui --insecure -q 1000 -c 32 -z 60s --output-format json https://bench.test:8443/)
wait
sleep 30
AFTER=$(python3 $B/lib/sample.py snapshot $CG)
CB=$(docker exec backend sh -c "grep -c 'tool=gately' /tmp/cb.log 2>/dev/null || true")
python3 - "$IDLE" "$AFTER" "$AUTH" "$LOAD" "$CB" <<'P'
import json, sys
idle, after, auth, load, cb = sys.argv[1:6]
j = json.loads(load[load.index("{"):])
res = {"sut": "gately-bundle", "idle": json.loads(idle), "idleAfterLoad": json.loads(after),
       "authStatus": auth, "load": {"rps": j["summary"]["requestsPerSec"],
       "p99ms": (j["latencyPercentiles"]["p99"] or 0) * 1000, "ok": j["statusCodeDistribution"].get("200", 0)},
       "watch": json.load(open("/tmp/bundle-watch.json")), "ddnsCallbacksTotal": cb}
json.dump(res, open("/root/benchmark/results/bundle-gately.json", "w"), indent=1)
print(json.dumps(res))
P
docker logs sut 2>&1 | grep -iE "error|错误" | grep -v 防火墙 | head -5
docker rm -f sut >/dev/null
