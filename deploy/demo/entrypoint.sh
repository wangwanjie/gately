#!/bin/sh
# 每次启动都重新生成演示数据（证书有效期、日志与访客时间以当前时间为基准），然后启动演示模式的 Gately。
# 之后的每小时重置由 Gately 自己完成：把配置目录恢复为启动时的快照并重启进程。
set -eu
dir="${GATELY_CONFIG_DIR:-/data}"
: "${GATELY_INITIAL_PASSWORD:?必须设置 GATELY_INITIAL_PASSWORD（管理员密码，登录页不提示）}"
rm -rf "$dir"/* "$dir"/.[!.]* 2>/dev/null || true
/usr/local/bin/demo-gen -out "$dir"
exec /usr/local/bin/gately -demo -c "$dir" -listen :16680
