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
// +build windows

package execution

import (
	"fmt"

	"os"
	"os/exec"
)

// setProcessGroup is a no-op on Windows
func setProcessGroup(cmd *exec.Cmd) {
	// Windows doesn't have process groups in the Unix sense
	// Process tree management is handled differently
}

// killProcessGroup kills a process on Windows
func killProcessGroup(pid int) error {
	// On Windows, we kill the process directly
	// Process tree termination requires different APIs
	proc, err := os.FindProcess(pid)
	if err != nil {
		return fmt.Errorf("execution: failed to find process %d: %w", pid, err)
	}
	if err := proc.Kill(); err != nil {
		return fmt.Errorf("execution: failed to kill process %d: %w", pid, err)
	}
	return nil
}
