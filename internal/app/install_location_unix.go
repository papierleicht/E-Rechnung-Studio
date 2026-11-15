//go:build !windows

package app

import (
	"os/exec"
	"strconv"
	"syscall"
)

// relaunchAfterExit starts target once the process pid has ended (at most
// about a minute later). The helper runs in its own session so it survives
// this process.
func relaunchAfterExit(target string, pid int) error {
	script := `i=0; while kill -0 "$1" 2>/dev/null && [ "$i" -lt 300 ]; do sleep 0.2; i=$((i+1)); done; ` + relaunchCommand
	cmd := exec.Command("/bin/sh", "-c", script, "sh", strconv.Itoa(pid), target)
	cmd.Env = relaunchEnv()
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	return cmd.Start()
}
