//go:build unix

// Package proc holds the process-group handling a timeout needs to be worth
// anything: a spawned command's children outlive it otherwise, and whoever
// waited on the pipe waits past the deadline.
package proc

import (
	"os/exec"
	"syscall"
)

// SetGroup puts the command in its own process group.
func SetGroup(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}

// KillGroup signals the whole group so a shell cannot leave its children
// running after a timeout.
func KillGroup(cmd *exec.Cmd) error {
	if cmd.Process == nil {
		return nil
	}
	if err := syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL); err != nil {
		return cmd.Process.Kill()
	}
	return nil
}
