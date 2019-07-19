#!/bin/bash

waitTimeSync() {
  target=$1
  while true; do
    target_time=$(ckecli ssh ${target} date +%s)
    host_time=$(date "+%s")
    if [ $(( host_time - target_time )) -lt 5 ]; then
      break
    fi
    sleep 1
  done
}

for worker in $(sabactl machines get --role worker | jq -r '.[] | .spec.ipv4[0]'); do
  for i in $(seq 300); do
    if ckecli ssh ${worker} sudo systemctl restart chronyd.service 2>/dev/null; then
      break
    fi
    sleep 1
  done
  waitTimeSync ${worker}
done
