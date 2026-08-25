// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

//go:build windows
// +build windows

package platform

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/g8e-ai/g8e/v2/internal/constants"
)

// realWindowsProcessChecker implements the interface using actual Windows syscalls
type realWindowsProcessChecker struct{}

func (r realWindowsProcessChecker) OpenProcess(desiredAccess uint32, inheritHandle bool, processID uint32) (uintptr, error) {
	h, err := syscall.OpenProcess(desiredAccess, inheritHandle, processID)
	return uintptr(h), err
}

func (r realWindowsProcessChecker) CloseHandle(handle uintptr) error {
	return syscall.CloseHandle(syscall.Handle(handle))
}

func (r realWindowsProcessChecker) GetExitCodeProcess(handle uintptr, exitCode *uint32) error {
	return syscall.GetExitCodeProcess(syscall.Handle(handle), exitCode)
}

// realCommandExecutor implements the interface using actual exec package
type realCommandExecutor struct{}

func (r realCommandExecutor) Command(name string, args ...string) *exec.Cmd {
	return exec.Command(name, args...)
}

func (r realCommandExecutor) Output(cmd *exec.Cmd) ([]byte, error) {
	return cmd.Output()
}

func (r realCommandExecutor) Run(cmd *exec.Cmd) error {
	return cmd.Run()
}

// setProcessGroup is a no-op on Windows
func setProcessGroup(cmd *exec.Cmd) {
	// Windows doesn't have process groups in the Unix sense
	// Process management is handled differently
}

// isProcessRunning checks if a process with the given PID is running on Windows.
// It uses the Windows API to check if the process still exists and is active.
func (pm *ProcessManager) isProcessRunning(pid int) bool {
	if pm.isProcessRunningFn != nil {
		return pm.isProcessRunningFn(pid)
	}

	if pid == 0 {
		return false
	}

	var checker WindowsProcessChecker
	if pm.windowsProcessChecker != nil {
		checker = pm.windowsProcessChecker
	} else {
		checker = realWindowsProcessChecker{}
	}

	// On Windows, we can check if the process is running by calling
	// GetExitCodeProcess. If the process is still running, it returns STILL_ACTIVE.
	handle, err := checker.OpenProcess(syscall.PROCESS_QUERY_INFORMATION, false, uint32(pid))
	if err != nil {
		return false
	}
	defer func() { _ = checker.CloseHandle(handle) }()

	var exitCode uint32
	err = checker.GetExitCodeProcess(handle, &exitCode)
	if err != nil {
		return false
	}

	// STILL_ACTIVE indicates the process is still running
	return exitCode == constants.StillActiveExitCode
}

// findProcessOnPort finds the PID of the process listening on the given port on Windows.
// It uses netstat to find the process ID.
func (pm *ProcessManager) findProcessOnPort(port int) int {
	var executor CommandExecutor
	if pm.commandExecutor != nil {
		executor = pm.commandExecutor
	} else {
		executor = realCommandExecutor{}
	}

	cmd := executor.Command("netstat", "-ano")
	output, err := executor.Output(cmd)
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
	var executor CommandExecutor
	if pm.commandExecutor != nil {
		executor = pm.commandExecutor
	} else {
		executor = realCommandExecutor{}
	}

	// Derive the actual image name from the running executable so that
	// renamed binaries (e.g. g8e-windows-amd64.exe) are found correctly.
	imageName := constants.BinaryImageNameWindows
	if exePath, err := os.Executable(); err == nil {
		imageName = filepath.Base(exePath)
	}

	ownPID := os.Getpid()
	cmd := executor.Command("tasklist", "/FI", fmt.Sprintf("IMAGENAME eq %s", imageName), "/FO", "CSV")
	output, err := executor.Output(cmd)
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

	var executor CommandExecutor
	if pm.commandExecutor != nil {
		executor = pm.commandExecutor
	} else {
		executor = realCommandExecutor{}
	}

	// Try graceful shutdown first
	cmd := executor.Command("taskkill", "/PID", fmt.Sprintf("%d", pid), "/T")
	if err := executor.Run(cmd); err == nil {
		// Wait a bit to see if it exits gracefully
		time.Sleep(500 * time.Millisecond)
		if !pm.isProcessRunning(pid) {
			return nil
		}
	}

	// Force kill if graceful shutdown failed
	cmd = executor.Command("taskkill", "/F", "/PID", fmt.Sprintf("%d", pid), "/T")
	if err := executor.Run(cmd); err != nil {
		return fmt.Errorf("process_manager: %w: %v", constants.ErrProcessStopFailed, err)
	}

	// Wait for process to actually exit and release file handles
	for i := 0; i < 20; i++ {
		if !pm.isProcessRunning(pid) {
			return nil
		}
		time.Sleep(100 * time.Millisecond)
	}

	return fmt.Errorf("process_manager: %w (pid %d)", constants.ErrProcessForceKillTimeout, pid)
}
