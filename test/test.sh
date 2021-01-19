#!/bin/sh

EXTERNAL_PID=$(pmctl pod show external | jq .pid)
export EXTERNAL_PID

go mod download
sudo -E nsenter -t $(pmctl pod show operation | jq .pid) -n env PATH=$PATH $GINKGO
