#!/bin/sh
# 路由器（OpenWrt，busybox）上的采样：按进程匹配模式统计 PSS 之和与 CPU 时间。
# 用法：rsample.sh snapshot <模式>          输出 "pssKiB rssKiB cpuTicks 进程数"
#       rsample.sh watch <模式> <秒>        每 0.5 秒采样，输出 "峰值PSS 平均CPU百分比"
pids() { pgrep -f "$1" | while read p; do [ "$p" != "$$" ] && echo $p; done; }
# OpenWrt 内核通常没有 smaps（CONFIG_PROC_PAGE_MONITOR 未开），改用 /proc/<pid>/statm 估算：
# 近似 PSS = 各进程私有驻留页（resident - shared）之和 + 最大的一份共享驻留页（多进程共享的代码段只算一次）
mem() {
  priv=0; maxsh=0; rss=0; n=0
  for p in $(pids "$1"); do
    [ -r /proc/$p/statm ] || continue
    set -- $(cat /proc/$p/statm)
    res=$(( $2 * 4 )); sh=$(( $3 * 4 ))
    priv=$((priv + res - sh)); rss=$((rss + res)); n=$((n + 1))
    [ $sh -gt $maxsh ] && maxsh=$sh
  done
  echo "$((priv + maxsh)) $rss $n"
}
ticks() {
  t=0
  for p in $(pids "$1"); do
    [ -r /proc/$p/stat ] || continue
    v=$(awk '{print $14+$15+$16+$17}' /proc/$p/stat)
    t=$((t + v))
  done
  echo $t
}
case "$1" in
  snapshot)
    set -- $(mem "$2") $(ticks "$2")
    echo "$1 $2 $4 $3" ;;
  watch)
    pat="$2"; secs="$3"
    t0=$(ticks "$pat"); s0=$(cut -d' ' -f1 /proc/uptime)
    peak=0; end=$(( $(date +%s) + secs ))
    while [ $(date +%s) -lt $end ]; do
      set -- $(mem "$pat"); [ $1 -gt $peak ] && peak=$1
      sleep 0.5
    done
    t1=$(ticks "$pat"); s1=$(cut -d' ' -f1 /proc/uptime)
    awk -v p=$peak -v a=$t0 -v b=$t1 -v x=$s0 -v y=$s1 'BEGIN{printf "%d %.1f\n", p, (b-a)/(y-x)}' ;;
esac
