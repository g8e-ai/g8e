// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

//go:build windows

package cmd

import (
	"os/exec"
	"syscall"
)

const (
	// CREATE_NEW_PROCESS_GROUP creates a new process group for the subprocess
	// This enables process tree management via taskkill /T
	CREATE_NEW_PROCESS_GROUP = 0x00000200
)

func setSysProcAttr(cmd *exec.Cmd) {
	// On Windows, set CreationFlags to create a new process group
	// This allows proper process tree termination via taskkill /T
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.CreationFlags = CREATE_NEW_PROCESS_GROUP
}
