#!/bin/sh
# Gately 网络唤醒客户端：Linux、macOS、OpenWrt 等类 Unix 系统。
#
# 客户端主动连接 Gately（长轮询），收到指令后在本机关机、重启或睡眠。只执行这三种固定操作，
# 服务端无法让它执行其他命令；设备不必开放端口，也不必开启 SSH。需要 curl 或 wget（含 OpenWrt 的 uclient-fetch）。
#
# 用法（以 root 运行）：
#   GATELY_TOKEN=<凭据> sh gately-agent.sh install <Gately 地址> [--insecure]   安装为系统服务并启动
#   sh gately-agent.sh uninstall                                               停止并删除服务
#   gately-agent run                                                           前台运行（由系统服务调用）
#   GATELY_TOKEN=<凭据> sh gately-agent.sh test <Gately 地址> [--insecure]       试运行：收到指令只打印，不执行
#
# --insecure 不校验 HTTPS 证书，仅用于局域网内使用自签名证书的 Gately。

VERSION=1
CONF=/etc/gately-agent.conf
SERVICE=gately-agent
# 试运行标记只由 test 子命令设置，不从环境变量继承
DRYRUN=

log() {
	echo "$(date '+%Y-%m-%d %H:%M:%S') $*"
}

die() {
	echo "gately-agent: $*" >&2
	exit 1
}

# is_openwrt / has_systemd / has_openrc 判断系统与服务管理器
is_openwrt() {
	[ -f /etc/openwrt_release ]
}

has_systemd() {
	[ -d /run/systemd/system ] && command -v systemctl >/dev/null 2>&1
}

has_openrc() {
	[ -x /sbin/openrc-run ] && command -v rc-update >/dev/null 2>&1
}

# 脚本安装位置：OpenWrt 没有 /usr/local
bin_path() {
	if is_openwrt; then
		echo /usr/bin/gately-agent
	else
		echo /usr/local/bin/gately-agent
	fi
}

# check_server / check_token 校验地址与凭据，它们会写进配置文件、拼进请求，不允许出现引号、空白等字符
check_server() {
	case "$1" in
	http://?*|https://?*) ;;
	*) die "Gately 地址须以 http:// 或 https:// 开头: $1" ;;
	esac
	case "$1" in
	*[!A-Za-z0-9:/._~%@\[\]-]*) die "Gately 地址含有不支持的字符: $1" ;;
	esac
}

check_token() {
	case "$1" in
	''|*[!0-9a-f]*) die "凭据格式无效（应为十六进制字符串），请在 Gately 设备设置中复制" ;;
	esac
	[ ${#1} -ge 32 ] || die "凭据过短，请在 Gately 设备设置中复制"
}

# load_conf 读取配置文件。逐行取值，不 source，避免配置文件被改动后执行任意代码
load_conf() {
	[ -r "$CONF" ] || die "找不到配置文件 $CONF，请先运行 install"
	SERVER=$(sed -n 's/^SERVER=//p' "$CONF" | head -n 1)
	TOKEN=$(sed -n 's/^TOKEN=//p' "$CONF" | head -n 1)
	INSECURE=$(sed -n 's/^INSECURE=//p' "$CONF" | head -n 1)
	check_server "$SERVER"
	check_token "$TOKEN"
}

# setup_http 选择 HTTP 工具并创建临时目录。凭据只经标准输入或权限为 600 的临时文件传给 curl / wget，
# 不出现在命令行参数里（其他用户可经 ps 看到命令行）
setup_http() {
	SERVER=${SERVER%/}
	umask 077
	TMPD=$(mktemp -d 2>/dev/null || mktemp -d -t gately-agent) || die "无法创建临时目录"
	trap 'rm -rf "$TMPD"' EXIT
	# 收到停止信号时结束正在等待的 curl / wget / sleep 再退出：子进程放在后台并用 wait 等待，
	# 信号能立即打断 wait（前台子进程要等它结束 shell 才处理信号，轮询最长挂起 40 秒）
	trap '[ -n "$CHILD" ] && kill "$CHILD" 2>/dev/null; exit 130' INT TERM
	if command -v curl >/dev/null 2>&1; then
		HTTP=curl
	elif command -v wget >/dev/null 2>&1 && wget --help 2>&1 | grep -q -- '--post-file'; then
		HTTP=wget
	else
		die "需要 curl 或支持 --post-file 的 wget（OpenWrt 可执行 opkg install curl）"
	fi
}

CHILD=

# nap 秒数：可被停止信号打断的 sleep
nap() {
	sleep "$1" &
	CHILD=$!
	wait "$CHILD"
	CHILD=
}

# http_post 路径 表单（不含凭据） 超时秒数：响应内容写入 RESP，状态码写入 CODE（网络错误为 000）
http_post() {
	url="$SERVER$1"
	timeout=$3
	printf 'token=%s&%s' "$TOKEN" "$2" >"$TMPD/req"
	: >"$TMPD/resp"
	if [ "$HTTP" = curl ]; then
		set -- -sS --max-time "$timeout" -o "$TMPD/resp" -w '%{http_code}' \
			-H 'Content-Type: application/x-www-form-urlencoded' --data-binary "@$TMPD/req"
		[ "$INSECURE" = 1 ] && set -- "$@" -k
		curl "$@" "$url" >"$TMPD/code" 2>"$TMPD/err" <&- &
		CHILD=$!
		if wait "$CHILD"; then
			CODE=$(cat "$TMPD/code")
		else
			CODE=000
		fi
		CHILD=
		[ -n "$CODE" ] || CODE=000
	else
		# GNU wget、BusyBox wget 与 OpenWrt 的 uclient-fetch 都支持这几个参数；它们取不到错误响应的状态码，
		# 失败时一律按网络错误退避重试（最长 60 秒），确认失败时不执行指令
		set -- -q -O "$TMPD/resp" -T "$timeout" \
			--header='Content-Type: application/x-www-form-urlencoded' --post-file="$TMPD/req"
		[ "$INSECURE" = 1 ] && set -- "$@" --no-check-certificate
		wget "$@" "$url" 2>"$TMPD/err" <&- &
		CHILD=$!
		if wait "$CHILD"; then
			CODE=200
		else
			CODE=000
		fi
		CHILD=
	fi
	rm -f "$TMPD/req"
	RESP=$(head -c 512 "$TMPD/resp" | tr -d '\r')
}

# sysinfo 生成上报的主机名与系统描述，只保留字母、数字与 ._-
sysinfo() {
	HOSTNAME_=$(uname -n 2>/dev/null | tr -cd 'A-Za-z0-9._-' | cut -c1-60)
	if is_openwrt; then
		OSNAME="OpenWrt-$(sed -n "s/^DISTRIB_RELEASE='\{0,1\}\([^']*\)'\{0,1\}/\1/p" /etc/openwrt_release | head -n 1)"
	elif [ "$(uname -s)" = Darwin ]; then
		OSNAME="macOS-$(sw_vers -productVersion 2>/dev/null)"
	elif [ -r /etc/os-release ]; then
		OSNAME=$(. /etc/os-release 2>/dev/null && echo "${ID:-linux}-${VERSION_ID:-}")
	else
		OSNAME=$(uname -s)
	fi
	OSNAME=$(echo "$OSNAME-$(uname -m)" | tr -cd 'A-Za-z0-9._-' | cut -c1-60)
	# 本次运行的随机标识：服务端据此区分同一客户端重连与另一台机器误用同一凭据
	INSTANCE=$(od -An -N12 -tx1 /dev/urandom 2>/dev/null | tr -cd '0-9a-f')
	[ ${#INSTANCE} -ge 16 ] || INSTANCE="i$(date +%s)$$"
}

# do_action 在本机执行电源操作，失败时把错误输出写入 ACTION_ERR
do_action() {
	ACTION_ERR=
	if [ -n "$DRYRUN" ]; then
		log "[test] would run: $1"
		return 0
	fi
	case "$(uname -s)" in
	Darwin)
		case "$1" in
		shutdown) set -- shutdown -h now ;;
		reboot) set -- shutdown -r now ;;
		sleep) set -- pmset sleepnow ;;
		esac
		;;
	*)
		if has_systemd; then
			case "$1" in
			shutdown) set -- systemctl poweroff ;;
			reboot) set -- systemctl reboot ;;
			sleep) set -- systemctl suspend ;;
			esac
		else
			case "$1" in
			shutdown) set -- poweroff ;;
			reboot) set -- reboot ;;
			sleep)
				# 没有 systemd 时写内核接口挂起到内存
				if grep -q mem /sys/power/state 2>/dev/null; then
					# 放进子 shell：重定向失败（只读文件系统、权限不足）的报错也写进 act
					(echo mem >/sys/power/state) 2>"$TMPD/act" && return 0
					ACTION_ERR=$(head -c 200 "$TMPD/act" | tr -cd 'A-Za-z0-9 ._:/()-')
				fi
				[ -n "$ACTION_ERR" ] || ACTION_ERR="suspend not supported"
				return 1
				;;
			esac
		fi
		;;
	esac
	if "$@" >"$TMPD/act" 2>&1; then
		return 0
	fi
	ACTION_ERR=$(head -c 200 "$TMPD/act" | tr -cd 'A-Za-z0-9 ._:/()-')
	[ -n "$ACTION_ERR" ] || ACTION_ERR="$* failed"
	return 1
}

# handle_cmd 指令 ID 操作：先确认，服务端回复 ok 才执行（指令已过期时服务端拒绝，避免执行迟到的指令）
handle_cmd() {
	case "$1" in
	''|*[!0-9a-f]*) log "ignored malformed command"; return ;;
	esac
	case "$2" in
	shutdown|reboot|sleep) ;;
	*) log "ignored unknown action: $2"; return ;;
	esac
	http_post /api/wol/public/agent/ack "instance=$INSTANCE&id=$1&result=ok" 10
	if [ "$CODE" != 200 ] || [ "$RESP" != ok ]; then
		log "command $2 expired or rejected ($CODE), skipped"
		return
	fi
	log "executing: $2"
	if do_action "$2"; then
		# 关机、重启已开始：等系统停止，不再轮询（若被取消，稍后恢复）；睡眠唤醒后继续轮询
		case "$2" in
		shutdown|reboot) [ -n "$DRYRUN" ] || nap 60 ;;
		*) nap 2 ;;
		esac
		return
	fi
	log "$2 failed: $ACTION_ERR"
	msg=$(printf '%s' "$ACTION_ERR" | sed 's/ /+/g')
	http_post /api/wol/public/agent/ack "instance=$INSTANCE&id=$1&result=fail&msg=$msg" 10
}

# run 主循环：长轮询取指令；出错时退避重试
run() {
	setup_http
	sysinfo
	log "gately-agent $VERSION started, server $SERVER ($HTTP)"
	backoff=5
	while :; do
		start=$(date +%s)
		http_post /api/wol/public/agent/poll "instance=$INSTANCE&host=$HOSTNAME_&os=$OSNAME&version=$VERSION" 40
		case "$CODE" in
		200)
			if [ "$backoff" -gt 5 ]; then
				log "connected"
			fi
			backoff=5
			case "$RESP" in
			cmd\ *)
				# shellcheck disable=SC2086
				set -- $RESP
				handle_cmd "$2" "$3"
				;;
			esac
			# 立即返回的轮询（例如被同一客户端的新连接替换）稍等再继续，避免空转
			[ $(($(date +%s) - start)) -ge 2 ] || nap 2
			;;
		401)
			log "token rejected by server; check the device settings in Gately"
			nap 60
			;;
		409)
			log "another client is using this token; retrying later"
			nap 15
			;;
		*)
			log "poll failed (HTTP $CODE): $(head -c 200 "$TMPD/err" | tr -d '\r\n')"
			nap "$backoff"
			backoff=$((backoff * 2))
			[ "$backoff" -le 60 ] || backoff=60
			;;
		esac
	done
}

# parse_target 解析 install / test 的参数：地址、--insecure；凭据取自环境变量 GATELY_TOKEN
parse_target() {
	SERVER=
	INSECURE=0
	for a in "$@"; do
		case "$a" in
		--insecure) INSECURE=1 ;;
		-*) die "未知参数: $a" ;;
		*) SERVER=$a ;;
		esac
	done
	[ -n "$SERVER" ] || die "请指定 Gately 地址，例如 https://gately.example.com:16601"
	SERVER=${SERVER%/}
	TOKEN=${GATELY_TOKEN:-}
	[ -n "$TOKEN" ] || die "请用环境变量 GATELY_TOKEN 传入凭据"
	check_server "$SERVER"
	check_token "$TOKEN"
}

need_root() {
	[ "$(id -u)" = 0 ] || die "请以 root 运行（例如 sudo）"
}

install_service() {
	need_root
	parse_target "$@"
	setup_http
	bin=$(bin_path)
	# 从 Gately 下载脚本本身：通过管道运行时没有脚本文件可复制，同时验证地址与凭据可用
	http_post /api/wol/public/agent/gately-agent.sh "" 30
	if [ "$CODE" != 200 ] || ! head -n 1 "$TMPD/resp" | grep -q '^#!/bin/sh'; then
		die "无法从 $SERVER 下载客户端（HTTP $CODE），请检查地址、凭据与网络: $(head -c 200 "$TMPD/err")"
	fi
	mkdir -p "$(dirname "$bin")"
	# 安装正在运行的这份脚本（安装命令已校验过它的 SHA-256）；经管道运行没有脚本文件时才用刚下载的内容
	src="$TMPD/resp"
	if [ -f "$0" ] && head -n 1 "$0" | grep -q '^#!/bin/sh' && grep -q 'Gately 网络唤醒客户端' "$0"; then
		src=$0
	fi
	cp "$src" "$bin.tmp" && chmod 755 "$bin.tmp" && mv -f "$bin.tmp" "$bin" || die "无法写入 $bin"
	# 配置文件只有 root 可读
	(umask 077 && printf 'SERVER=%s\nTOKEN=%s\nINSECURE=%s\n' "$SERVER" "$TOKEN" "$INSECURE" >"$CONF.tmp") && mv -f "$CONF.tmp" "$CONF" || die "无法写入 $CONF"
	chmod 600 "$CONF"

	if [ "$(uname -s)" = Darwin ]; then
		plist=/Library/LaunchDaemons/cn.gately.agent.plist
		launchctl bootout system "$plist" >/dev/null 2>&1
		cat >"$plist" <<EOF
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>Label</key><string>cn.gately.agent</string>
	<key>ProgramArguments</key><array><string>/bin/sh</string><string>$bin</string><string>run</string></array>
	<key>RunAtLoad</key><true/>
	<key>KeepAlive</key><true/>
	<key>StandardOutPath</key><string>/var/log/gately-agent.log</string>
	<key>StandardErrorPath</key><string>/var/log/gately-agent.log</string>
</dict>
</plist>
EOF
		chmod 644 "$plist"
		launchctl bootstrap system "$plist" || die "launchctl 加载失败"
		echo "已安装并启动（launchd: cn.gately.agent，日志 /var/log/gately-agent.log）"
	elif is_openwrt; then
		cat >/etc/init.d/$SERVICE <<EOF
#!/bin/sh /etc/rc.common
# Gately 网络唤醒客户端
START=99
USE_PROCD=1

start_service() {
	procd_open_instance
	# 直接执行脚本（带 #!/bin/sh），系统日志的标签才是 gately-agent，logread -e gately-agent 能筛出
	procd_set_param command $bin run
	procd_set_param respawn 3600 5 0
	procd_set_param stdout 1
	procd_set_param stderr 1
	procd_close_instance
}
EOF
		chmod 755 /etc/init.d/$SERVICE
		/etc/init.d/$SERVICE enable
		/etc/init.d/$SERVICE stop >/dev/null 2>&1
		/etc/init.d/$SERVICE start
		# 系统升级时保留配置文件
		grep -qx "$CONF" /etc/sysupgrade.conf 2>/dev/null || echo "$CONF" >>/etc/sysupgrade.conf
		echo "已安装并启动（procd: /etc/init.d/$SERVICE，日志 logread -e gately-agent）"
	elif has_systemd; then
		cat >/etc/systemd/system/$SERVICE.service <<EOF
[Unit]
Description=Gately WOL agent
Wants=network-online.target
After=network-online.target

[Service]
ExecStart=/bin/sh $bin run
Restart=always
RestartSec=5

[Install]
WantedBy=multi-user.target
EOF
		systemctl daemon-reload
		systemctl enable $SERVICE >/dev/null 2>&1
		systemctl restart $SERVICE || die "启动服务失败，见 journalctl -u $SERVICE"
		echo "已安装并启动（systemd: $SERVICE，日志 journalctl -u $SERVICE）"
	elif has_openrc; then
		cat >/etc/init.d/$SERVICE <<EOF
#!/sbin/openrc-run
# Gately 网络唤醒客户端
description="Gately WOL agent"
command=/bin/sh
command_args="$bin run"
command_background=true
pidfile=/run/$SERVICE.pid
output_log=/var/log/$SERVICE.log
error_log=/var/log/$SERVICE.log
depend() {
	need net
}
EOF
		chmod 755 /etc/init.d/$SERVICE
		rc-update add $SERVICE default >/dev/null
		rc-service $SERVICE restart || die "启动服务失败"
		echo "已安装并启动（OpenRC: $SERVICE，日志 /var/log/$SERVICE.log）"
	else
		echo "未识别的服务管理器：已安装 $bin 与 $CONF，请设置开机运行 $bin run"
	fi
}

uninstall_service() {
	need_root
	if [ "$(uname -s)" = Darwin ]; then
		launchctl bootout system /Library/LaunchDaemons/cn.gately.agent.plist >/dev/null 2>&1
		rm -f /Library/LaunchDaemons/cn.gately.agent.plist
	elif is_openwrt; then
		/etc/init.d/$SERVICE stop >/dev/null 2>&1
		/etc/init.d/$SERVICE disable >/dev/null 2>&1
		rm -f /etc/init.d/$SERVICE
		sed -i "\|^$CONF\$|d" /etc/sysupgrade.conf 2>/dev/null
	elif has_systemd; then
		systemctl disable --now $SERVICE >/dev/null 2>&1
		rm -f /etc/systemd/system/$SERVICE.service
		systemctl daemon-reload
	elif has_openrc; then
		rc-service $SERVICE stop >/dev/null 2>&1
		rc-update del $SERVICE default >/dev/null 2>&1
		rm -f /etc/init.d/$SERVICE
	fi
	rm -f "$(bin_path)" "$CONF"
	echo "已卸载"
}

cmd=${1:-}
[ $# -gt 0 ] && shift
case "$cmd" in
install) install_service "$@" ;;
uninstall) uninstall_service ;;
run)
	load_conf
	run
	;;
test)
	parse_target "$@"
	DRYRUN=1
	run
	;;
*)
	sed -n '2,15p' "$0" 2>/dev/null | sed 's/^# \{0,1\}//'
	[ -n "$cmd" ] && exit 1
	exit 0
	;;
esac
