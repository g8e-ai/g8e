// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

//go:build windows
// +build windows

package execution

import (
	"fmt"
	"os"
	"os/exec"
	"syscall"

	"golang.org/x/sys/windows"

	"github.com/g8e-ai/g8e/v2/internal/constants"
)

// setProcessGroup keeps non-interactive Operator commands from opening a console.
func setProcessGroup(cmd *exec.Cmd) {
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.HideWindow = true
	cmd.SysProcAttr.CreationFlags |= windows.CREATE_NO_WINDOW
}

// killProcessGroup kills a process on Windows
func killProcessGroup(pid int) error {
	// On Windows, we kill the process directly
	// Process tree termination requires different APIs
	proc, err := os.FindProcess(pid)
	if err != nil {
		return fmt.Errorf("execution: failed to find process %d: %w", pid, constants.ErrProcessFindFailed)
	}
	if err := proc.Kill(); err != nil {
		return fmt.Errorf("execution: failed to kill process %d: %w", pid, constants.ErrProcessStopFailed)
	}
	return nil
}
