package test

import (
	"os"
	"strconv"
)

var (
	doBootstrap         = os.Getenv("BOOTSTRAP") == "1"
	doUpgrade           = os.Getenv("UPGRADE") == "1"
	doReboot            = os.Getenv("REBOOT") == "1"
	boot0               = os.Getenv("BOOT0")
	boot1               = os.Getenv("BOOT1")
	boot2               = os.Getenv("BOOT2")
	sshKeyFile          = os.Getenv("SSH_PRIVKEY")
	testID              = os.Getenv("TEST_ID")
	commitID            = os.Getenv("COMMIT_ID")
	externalPID         = os.Getenv("EXTERNAL_PID")
	withKind            = os.Getenv("KIND") == "1"
	numGrafanaDashboard = 0
)

func init() {
	num, err := strconv.Atoi(os.Getenv("NUM_DASHBOARD"))
	if err != nil {
		panic(err)
	}
	// NOTE: (the count of dashboard) = (the count of files in ../monitoring/base/grafana/dashboards) + 1
	// +1 comes from node-exporter-full dashboard, which downloaded by init-container
	numGrafanaDashboard = num + 1
}
