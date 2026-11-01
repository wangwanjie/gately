#!/bin/bash
# 阶段 A：英文数据，非终端场景
S=overview,web,site-stats,protoroute,ddns,guard,frp,docker,config-history,api-docs,webtopology
for l in "$@"; do node shots.mjs $l /tmp/gs/out/$l $S 2>&1 | grep -E "ok|Error" ; done
echo ALLDONE
