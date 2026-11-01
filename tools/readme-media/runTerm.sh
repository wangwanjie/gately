#!/bin/bash
export PATH=$PATH:/opt/homebrew/bin
for l in "$@"; do
  timeout 60 colima ssh -p gately-demo -- sudo bash -c 'sh /opt/gately/restart.sh </dev/null >/dev/null 2>&1; sleep 9'
  node shots.mjs $l /tmp/gs/out/$l terminal 2>&1 | grep -E "ok|Error"
done
echo TERMDONE
