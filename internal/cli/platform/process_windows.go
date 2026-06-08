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

package platform

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"time"
)

// setProcessGroup is a no-op on Windows
func setProcessGroup(cmd *exec.Cmd) {
	// Windows doesn't have process groups in the Unix sense
	// Process management is handled differently
}

// isProcessRunning checks if a process with the given PID is running on Windows.
// It uses the Windows API to check if the process still exists and is active.
func (pm *ProcessManager) isProcessRunning(pid int) bool {
	if pid == 0 {
		return false
	}

	// On Windows, we can check if the process is running by calling
	// GetExitCodeProcess. If the process is still running, it returns STILL_ACTIVE (259).
	handle, err := syscall.OpenProcess(syscall.PROCESS_QUERY_INFORMATION, false, uint32(pid))
	if err != nil {
		return false
	}
	defer syscall.CloseHandle(handle)

	var exitCode uint32
	err = syscall.GetExitCodeProcess(handle, &exitCode)
	if err != nil {
		return false
	}

	// STILL_ACTIVE (259) indicates the process is still running
	if exitCode != 259 {
		return false
	}

	// Verify the process is actually g8e to prevent false positives from PID reuse
	return pm.isG8eProcess(pid)
}

// isG8eProcess verifies that the given PID belongs to a g8e.exe process.
func (pm *ProcessManager) isG8eProcess(pid int) bool {
	cmd := exec.Command("tasklist", "/FI", fmt.Sprintf("PID eq %d", pid), "/FI", "IMAGENAME eq g8e.exe", "/FO", "CSV", "/NH")
	output, err := cmd.Output()
	if err != nil {
		return false
	}
	for _, line := range strings.Split(strings.TrimSpace(string(output)), "\n") {
		line = strings.TrimSpace(line)
		if line != "" && !strings.HasPrefix(line, "INFO:") {
			return true
		}
	}
	return false
}

// findProcessOnPort finds the PID of the process listening on the given port on Windows.
// It uses netstat to find the process ID.
func (pm *ProcessManager) findProcessOnPort(port int) int {
	cmd := exec.Command("netstat", "-ano")
	output, err := cmd.Output()
	if err != nil {
		return 0
	}

	lines := strings.Split(string(output), "\n")
	for _, line := range lines {
		fields := strings.Fields(line)
		if len(fields) >= 5 {
			// Check if this line contains the port we're looking for
			// Format: proto  local_address  foreign_address  state  pid
			localAddr := fields[1]
			if strings.Contains(localAddr, fmt.Sprintf(":%d", port)) {
				pidStr := fields[len(fields)-1]
				var pid int
				if _, err := fmt.Sscanf(pidStr, "%d", &pid); err == nil {
					return pid
				}
			}
		}
	}

	return 0
}

// findOperatorProcess finds the PID of the running g8e operator process using tasklist.
// This is used as a fallback when the PID file is missing or stale.
// It excludes the current process's own PID to avoid detecting the CLI itself.
func (pm *ProcessManager) findOperatorProcess() int {
	ownPID := os.Getpid()
	cmd := exec.Command("tasklist", "/FI", "IMAGENAME eq g8e.exe", "/FO", "CSV")
	output, err := cmd.Output()
	if err != nil {
		return 0
	}

	lines := strings.Split(string(output), "\n")
	for i, line := range lines {
		if i == 0 { // Skip header line
			continue
		}
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		fields := strings.Split(line, ",")
		if len(fields) >= 2 {
			pidStr := strings.Trim(fields[1], "\"")
			var pid int
			if _, err := fmt.Sscanf(pidStr, "%d", &pid); err == nil && pid != ownPID {
				return pid
			}
		}
	}

	return 0
}

// stopProcess stops a process with the given PID on Windows.
// It uses taskkill to terminate the process gracefully, then forcefully if needed.
func (pm *ProcessManager) stopProcess(pid int, name string) error {
	if pid == 0 {
		return nil
	}

	if !pm.isProcessRunning(pid) {
		return nil
	}

	// Try graceful shutdown first
	cmd := exec.Command("taskkill", "/PID", fmt.Sprintf("%d", pid), "/T")
	if err := cmd.Run(); err == nil {
		// Wait a bit to see if it exits gracefully
		time.Sleep(500 * time.Millisecond)
		if !pm.isProcessRunning(pid) {
			return nil
		}
	}

	// Force kill if graceful shutdown failed
	cmd = exec.Command("taskkill", "/F", "/PID", fmt.Sprintf("%d", pid), "/T")
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to force kill process: %w", err)
	}

	// Wait for process to actually exit and release file handles
	for i := 0; i < 20; i++ {
		if !pm.isProcessRunning(pid) {
			return nil
		}
		time.Sleep(100 * time.Millisecond)
	}

	return nil
}
