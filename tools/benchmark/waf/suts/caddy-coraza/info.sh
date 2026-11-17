docker exec $SUT_NAME caddy version
docker exec $SUT_NAME cat /versions.txt | awk '{print $2, $3}'
