// Copyright (c) 2026 Lateralus Labs, LLC.
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

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
