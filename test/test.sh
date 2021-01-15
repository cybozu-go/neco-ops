#!/bin/sh

EXTERNAL_PID=$(pmctl pod show external | jq .pid)
export EXTERNAL_PID

sudo -E nsenter -t $(pmctl pod show operation | jq .pid) -n env PATH=$PATH SUITE=$SUITE $GINKGO
