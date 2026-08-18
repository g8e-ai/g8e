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
	"fmt"
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

// killProcessGroup kills a process group on Unix
func killProcessGroup(pid int) error {
	pgid, err := syscall.Getpgid(pid)
	if err != nil {
		return fmt.Errorf("execution: get process group ID: %w", err)
	}
	if err := syscall.Kill(-pgid, syscall.SIGKILL); err != nil {
		return fmt.Errorf("execution: kill process group: %w", err)
	}
	return nil
}
