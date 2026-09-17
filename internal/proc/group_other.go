//go:build !unix

package proc

import "os/exec"

// This build cannot honour the timeout guarantee.
//
// Killing the direct child leaves a descendant holding the stdout pipe, and the
// read that precedes Wait blocks past the deadline with no WaitDelay to rescue
// it. Terminating a process tree off Unix needs a platform primitive such as a
// Windows Job Object. Until one exists here, the constraint is declared rather
// than pretended: the package does not build on these targets.
//
// Every shipped path in this repository is POSIX.
func init() { panic("proc: process-group termination is unimplemented on this platform") }

func SetGroup(*exec.Cmd) {}

func KillGroup(cmd *exec.Cmd) error {
	if cmd.Process == nil {
		return nil
	}
	return cmd.Process.Kill()
}
