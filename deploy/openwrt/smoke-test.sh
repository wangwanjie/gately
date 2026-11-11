#!/bin/sh
# OpenWrt 安装包冒烟测试：打出 amd64 的 .ipk / .apk 与 LuCI 插件，分别在 OpenWrt 24.10（opkg）与 25.12（apk）的官方 rootfs 容器中
# 安装、确认服务启动并登记到包管理器，检查 LuCI 插件的 rpcd 接口（状态、启停、UCI 端口覆盖），再卸载确认服务停止。
# 另外在 apk 中用 arm64 安装包覆盖安装（同版本替换与升级到新版本两种情况），确认装错架构时原有程序不被替换（issue #82）。需要 Docker，CI 中运行。
#
# 用法：deploy/openwrt/smoke-test.sh <linux/amd64 的 gately 二进制>
set -eu

# 打包镜像与 release.yml 保持一致；可用环境变量替换为本地已有的镜像
ALPINE=${ALPINE_IMAGE:-alpine:3.24@sha256:294b683cb724975bec92580e1e685676bd4b50bda910ddb8c51d4cabeaec77e6}
OPKG_IMAGE=${OPKG_IMAGE:-openwrt/rootfs:x86_64-24.10.4}
APK_IMAGE=${APK_IMAGE:-openwrt/rootfs:x86_64-25.12.0}

if [ $# -lt 1 ] || [ ! -f "$1" ]; then
	echo "用法: $0 <linux/amd64 的 gately 二进制>" >&2
	exit 1
fi
HERE=$(cd "$(dirname "$0")" && pwd)
REPO=$(cd "$HERE/../.." && pwd)
WORK=$(mktemp -d)
NAME=gately-openwrt-smoke-$$
trap 'docker rm -f "$NAME" >/dev/null 2>&1 || true; docker run --rm -v "$WORK:/work" "$ALPINE" rm -rf /work/out /work/wrong /work/gately /work/fake >/dev/null 2>&1 || true; rm -rf "$WORK"' EXIT
cp "$1" "$WORK/gately"

docker run --rm -v "$REPO:/w:ro" -v "$WORK:/work" -w /w "$ALPINE" \
	sh -ec 'deploy/openwrt/build.sh v0.0.0-rc1 amd64 /work/gately /work/out && deploy/openwrt/build-luci.sh v0.0.0-rc1 /work/out
		printf "not an amd64 program\n" >/work/fake && deploy/openwrt/build.sh v0.0.0-rc1 arm64 /work/fake /work/wrong
		deploy/openwrt/build.sh v0.0.1 arm64 /work/fake /work/wrong'

# 在容器中以 procd 为 init 启动 OpenWrt，执行安装、检查与卸载。
# sh -e 不会因 ! 取反的命令失败而退出，卸载后的检查用 if 显式判断
check() {
	image=$1
	script=$2
	echo "==== $image"
	docker run -d --name "$NAME" -v "$WORK/out:/pkgs:ro" -v "$WORK/wrong:/wrong:ro" "$image" /sbin/init >/dev/null
	sleep 5
	docker exec "$NAME" sh -ec "$script"
	docker rm -f "$NAME" >/dev/null
}

# LuCI 插件检查：UCI 配置已创建，rpcd 接口返回运行状态，修改 UCI 端口后服务按新端口重启，启停操作生效，未知操作被拒绝
LUCI_CHECK='
grep -q "config gately .main." /etc/config/gately
[ -f /www/luci-static/resources/view/gately/main.js ]
[ -f /usr/share/luci/menu.d/luci-app-gately.json ]
[ -f /usr/lib/lua/luci/i18n/gately.zh-cn.lmo ]
st=$(ubus call luci.gately status)
echo "$st"
[ "$(jsonfilter -s "$st" -e @.running)" = true ]
[ "$(jsonfilter -s "$st" -e @.enabled)" = true ]
# 全局设置只监听本机：LuCI 改端口时只换端口，主机部分保持不变，不能变成在所有地址上监听
/etc/init.d/gately stop
sleep 1
# 首次启动后还没保存过设置时没有 gately.json，只写监听地址即可，其余字段按默认值加载
if [ -f /etc/gately/gately.json ]; then
	sed -i "s/\"listen\": *\"[^\"]*\"/\"listen\": \"127.0.0.1:16680\"/" /etc/gately/gately.json
else
	echo "{\"listen\": \"127.0.0.1:16680\"}" >/etc/gately/gately.json
fi
grep -q "\"listen\": *\"127.0.0.1:16680\"" /etc/gately/gately.json
/etc/init.d/gately start
sleep 2
uci set gately.main.port=16681
uci commit gately
# 与 LuCI 保存并应用相同，直接发出 config.change 事件（reload_config 首次运行时没有基准，不会发事件）
ubus call service event "{\"type\":\"config.change\",\"data\":{\"package\":\"gately\"}}"
sleep 3
ubus call service list "{\"name\":\"gately\"}" | grep -q -- "-listen"
grep -q "0100007F:4129 " /proc/net/tcp
if grep -q "00000000:4129 " /proc/net/tcp || grep -q "00000000000000000000000000000000:4129 " /proc/net/tcp6 2>/dev/null; then
	echo "LuCI 端口覆盖丢掉了监听地址的主机部分" >&2
	exit 1
fi
[ "$(ubus call luci.gately status | jsonfilter -e @.port)" = 16681 ]
ubus call luci.gately action "{\"name\":\"stop\"}"
sleep 2
[ "$(ubus call luci.gately status | jsonfilter -e @.running)" = false ]
ubus call luci.gately action "{\"name\":\"start\"}"
sleep 2
[ "$(ubus call luci.gately status | jsonfilter -e @.running)" = true ]
ubus call luci.gately action "{\"name\":\"rm\"}" | grep -q error
'
# 装错架构（apk）：arm64 安装包必须失败，且不能替换已装好的程序，服务一直运行、开机自启不变。
# WRONG_ARCH_INSTALL 为覆盖安装命令，可以有多条，以分号分隔。
# opkg 在 preinst 失败时中止安装、不替换文件，不在此检查（中止升级后 opkg 自身的包状态会变成 deinstall，影响后续卸载检查）
WRONG_ARCH_CHECK='
sum=$(md5sum /usr/bin/gately)
IFS=";"
for cmd in $WRONG_ARCH_INSTALL; do
	IFS=" "
	if $cmd; then
		echo "装错架构的安装包没有报错: $cmd" >&2
		exit 1
	fi
	[ "$(md5sum /usr/bin/gately)" = "$sum" ]
done
unset IFS
[ ! -e /usr/bin/.gately.arch-keep ]
pidof gately
/etc/init.d/gately enabled
/etc/init.d/gately restart
sleep 2
pidof gately
'
GONE_CHECK='
if pidof gately || [ -e /etc/init.d/gately ] || [ -n "$(ls /etc/rc.d | grep gately)" ] || [ -e /usr/libexec/rpcd/luci.gately ]; then
	echo "卸载后仍有残留" >&2
	exit 1
fi
'

check "$OPKG_IMAGE" '
opkg install /pkgs/*.ipk
sleep 3
pidof gately
/etc/init.d/gately enabled
opkg list-installed | grep "^gately - 0.0.0~rc1-r1"
opkg list-installed | grep "^luci-app-gately - 0.0.0~rc1-r1"
'"$LUCI_CHECK"'
opkg remove luci-app-gately
opkg remove gately
sleep 1
'"$GONE_CHECK"'
'

check "$APK_IMAGE" '
apk add --allow-untrusted /pkgs/*.apk
sleep 3
pidof gately
/etc/init.d/gately enabled
grep -qx "P:gately" /lib/apk/db/installed
grep -qx "P:luci-app-gately" /lib/apk/db/installed
WRONG_ARCH_INSTALL="apk add --allow-untrusted /wrong/gately_v0.0.0-rc1_openwrt_arm64.apk;apk add --allow-untrusted /wrong/gately_v0.0.1_openwrt_arm64.apk"
'"$WRONG_ARCH_CHECK$LUCI_CHECK"'
apk del luci-app-gately gately
sleep 1
'"$GONE_CHECK"'
'

echo "OpenWrt 安装包冒烟测试通过"
