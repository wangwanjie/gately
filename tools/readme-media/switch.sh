#!/bin/sh
# 用法：switch.sh zh|en  切换演示实例的数据目录并重启
pkill -x gately
sleep 1
rm -rf /opt/gately/data
cp -a /opt/gately/data-$1 /opt/gately/data
cd /opt/gately && env -u http_proxy -u https_proxy -u HTTP_PROXY -u HTTPS_PROXY nohup /opt/gately/gately -c /opt/gately/data -listen 0.0.0.0:16750 >> gately.log 2>&1 &
