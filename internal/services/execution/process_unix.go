// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

//go:build !windows
// +build !windows

package execution

import (
	"errors"
	"os/exec"
	"syscall"
)

// setProcessGroup sets the process group for Unix systems
func setProcessGroup(cmd *exec.Cmd) {
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.Setpgid = true
}

// killProcessGroup kills a process group on Unix.
// When Setpgid is true, the process group ID is equal to the process PID.
// We signal -pid directly so all surviving processes in the group are terminated
// even if the leader process has already exited and been reaped.
func killProcessGroup(pid int) error {
	if pid <= 0 {
		return nil
	}

	// First, send SIGKILL to the entire process group (-pid).
	err := syscall.Kill(-pid, syscall.SIGKILL)
	if err == nil || errors.Is(err, syscall.ESRCH) {
		_ = syscall.Kill(pid, syscall.SIGKILL)
		return nil
	}

	// Fallback: if killing -pid failed for an unexpected reason, try looking up Getpgid.
	if pgid, getErr := syscall.Getpgid(pid); getErr == nil && pgid != pid {
		_ = syscall.Kill(-pgid, syscall.SIGKILL)
	}
	_ = syscall.Kill(pid, syscall.SIGKILL)
	return nil
}
