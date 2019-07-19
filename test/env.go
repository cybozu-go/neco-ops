package test

import (
	"os"
)

var (
	boot0       = os.Getenv("BOOT0")
	sshKeyFile  = os.Getenv("SSH_PRIVKEY")
	testID      = os.Getenv("TEST_ID")
	commitID    = os.Getenv("COMMIT_ID")
	externalPID = os.Getenv("EXTERNAL_PID")
)
