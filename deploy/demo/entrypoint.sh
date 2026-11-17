#!/bin/sh
# 每次启动都重新生成演示数据（证书有效期、日志与访客时间以当前时间为基准），然后启动演示模式的 Gately。
# 之后的每小时重置由 Gately 自己完成：把配置目录恢复为启动时的快照并重启进程。
set -eu
dir="${GATELY_CONFIG_DIR:-/data}"
: "${GATELY_INITIAL_PASSWORD:?必须设置 GATELY_INITIAL_PASSWORD（管理员密码，登录页不提示）}"
rm -rf "$dir"/* "$dir"/.[!.]* 2>/dev/null || true
# 前置反向代理经 Docker 网络访问容器，地址不是本机；默认信任私有网段，才会采信它的 X-Forwarded-Proto（会话 Cookie 加 Secure）与访客 IP。
# 容器端口只发布在宿主机回环上时，能连进来的只有前置代理；设为空串则不信任任何代理
trusted="${GATELY_DEMO_TRUSTED_PROXIES-10.0.0.0/8,172.16.0.0/12,192.168.0.0/16,fc00::/7}"
/usr/local/bin/demo-gen -out "$dir" -trusted-proxies "$trusted"
exec /usr/local/bin/gately -demo -c "$dir" -listen :16680
