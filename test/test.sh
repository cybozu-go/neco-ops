#!/bin/sh

SUDO_OPTION=-E
if [ "$SUDO" = "" ]; then
    SUDO_OPTION=""
fi

go build ./...
go test -c .
rm test.test
$SUDO $SUDO_OPTION ip netns exec operation env PATH=$PATH SUITE=$SUITE $GINKGO
