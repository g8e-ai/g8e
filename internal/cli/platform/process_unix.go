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

//go:build !windows
// +build !windows

package platform

import (
	"fmt"
	"os"
	"os/exec"
	"syscall"
	"time"
)

// setProcessGroup sets the process group for Unix systems
func setProcessGroup(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Setsid: true,
	}
}

// isProcessRunning checks if a process with the given PID is running on Unix systems.
// It uses syscall.Signal(0) which doesn't actually send a signal but checks if the process exists.
func (pm *ProcessManager) isProcessRunning(pid int) bool {
	if pid == 0 {
		return false
	}

	process, err := os.FindProcess(pid)
	if err != nil {
		return false
	}

	err = process.Signal(syscall.Signal(0))
	return err == nil
}

// findProcessOnPort finds the PID of the process listening on the given port on Unix systems.
// It uses lsof to find the process ID.
func (pm *ProcessManager) findProcessOnPort(port int) int {
	cmd := exec.Command("lsof", "-ti", fmt.Sprintf(":%d", port))
	output, err := cmd.Output()
	if err != nil {
		return 0
	}

	var pid int
	if _, err := fmt.Sscanf(string(output), "%d", &pid); err != nil {
		return 0
	}

	return pid
}

// findOperatorProcess finds the PID of the running g8e operator process using pgrep.
// This is used as a fallback when the PID file is missing or stale.
func (pm *ProcessManager) findOperatorProcess() int {
	cmd := exec.Command("pgrep", "-f", "g8e --doctrine")
	output, err := cmd.Output()
	if err != nil {
		return 0
	}

	var pid int
	if _, err := fmt.Sscanf(string(output), "%d", &pid); err != nil {
		return 0
	}

	return pid
}

// stopProcess stops a process with the given PID on Unix systems.
// It sends SIGTERM first, then SIGKILL if the process doesn't exit within the timeout.
func (pm *ProcessManager) stopProcess(pid int, name string) error {
	if pid == 0 {
		return nil
	}

	if !pm.isProcessRunning(pid) {
		return nil
	}

	process, err := os.FindProcess(pid)
	if err != nil {
		return fmt.Errorf("failed to find process: %w", err)
	}

	if err := process.Signal(syscall.SIGTERM); err != nil {
		return fmt.Errorf("failed to send SIGTERM: %w", err)
	}

	timeout := time.After(10 * time.Second)
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-timeout:
			if err := process.Signal(syscall.SIGKILL); err != nil {
				return fmt.Errorf("failed to send SIGKILL: %w", err)
			}
			return nil
		case <-ticker.C:
			if !pm.isProcessRunning(pid) {
				return nil
			}
		}
	}
}
