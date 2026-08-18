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
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/internal/constants"
)

// mockWindowsProcessChecker is a mock implementation for testing
type mockWindowsProcessChecker struct {
	openProcessFunc  func(desiredAccess uint32, inheritHandle bool, processID uint32) (uintptr, error)
	closeHandleFunc  func(handle uintptr) error
	getExitCodeFunc  func(handle uintptr, exitCode *uint32) error
	openProcessCalls []openProcessCall
	closeHandleCalls []uintptr
	getExitCodeCalls []getExitCodeCall
}

type openProcessCall struct {
	desiredAccess uint32
	inheritHandle bool
	processID     uint32
}

type getExitCodeCall struct {
	handle   uintptr
	exitCode uint32
}

func (m *mockWindowsProcessChecker) OpenProcess(desiredAccess uint32, inheritHandle bool, processID uint32) (uintptr, error) {
	m.openProcessCalls = append(m.openProcessCalls, openProcessCall{
		desiredAccess: desiredAccess,
		inheritHandle: inheritHandle,
		processID:     processID,
	})
	if m.openProcessFunc != nil {
		return m.openProcessFunc(desiredAccess, inheritHandle, processID)
	}
	return uintptr(1), nil
}

func (m *mockWindowsProcessChecker) CloseHandle(handle uintptr) error {
	m.closeHandleCalls = append(m.closeHandleCalls, handle)
	if m.closeHandleFunc != nil {
		return m.closeHandleFunc(handle)
	}
	return nil
}

func (m *mockWindowsProcessChecker) GetExitCodeProcess(handle uintptr, exitCode *uint32) error {
	m.getExitCodeCalls = append(m.getExitCodeCalls, getExitCodeCall{
		handle:   handle,
		exitCode: *exitCode,
	})
	if m.getExitCodeFunc != nil {
		return m.getExitCodeFunc(handle, exitCode)
	}
	*exitCode = constants.StillActiveExitCode
	return nil
}

// mockCommandExecutor is a mock implementation for testing
type mockCommandExecutor struct {
	commandFunc  func(name string, args ...string) *exec.Cmd
	outputFunc   func(cmd *exec.Cmd) ([]byte, error)
	runFunc      func(cmd *exec.Cmd) error
	commandCalls []commandCall
	outputCalls  []*exec.Cmd
	runCalls     []*exec.Cmd
}

type commandCall struct {
	name string
	args []string
}

func (m *mockCommandExecutor) Command(name string, args ...string) *exec.Cmd {
	m.commandCalls = append(m.commandCalls, commandCall{
		name: name,
		args: args,
	})
	if m.commandFunc != nil {
		return m.commandFunc(name, args...)
	}
	return exec.Command(name, args...)
}

func (m *mockCommandExecutor) Output(cmd *exec.Cmd) ([]byte, error) {
	m.outputCalls = append(m.outputCalls, cmd)
	if m.outputFunc != nil {
		return m.outputFunc(cmd)
	}
	return []byte{}, nil
}

func (m *mockCommandExecutor) Run(cmd *exec.Cmd) error {
	m.runCalls = append(m.runCalls, cmd)
	if m.runFunc != nil {
		return m.runFunc(cmd)
	}
	return nil
}

func TestSetProcessGroup(t *testing.T) {
	// setProcessGroup is a no-op on Windows
	// This test ensures it doesn't panic or cause issues
	cmd := exec.Command("echo", "test")
	setProcessGroup(cmd)
	// If we get here without panic, the test passes
}

func TestIsProcessRunning_ZeroPID(t *testing.T) {
	pm := &ProcessManager{}

	result := pm.isProcessRunning(0)
	assert.False(t, result, "isProcessRunning should return false for PID 0")
}

func TestIsProcessRunning_OpenProcessFails(t *testing.T) {
	mockChecker := &mockWindowsProcessChecker{
		openProcessFunc: func(desiredAccess uint32, inheritHandle bool, processID uint32) (uintptr, error) {
			return uintptr(0), errors.New("access denied")
		},
	}
	pm := &ProcessManager{
		windowsProcessChecker: mockChecker,
	}

	result := pm.isProcessRunning(1234)
	assert.False(t, result, "isProcessRunning should return false when OpenProcess fails")
	assert.Len(t, mockChecker.openProcessCalls, 1, "OpenProcess should be called once")
	assert.Equal(t, uint32(1234), mockChecker.openProcessCalls[0].processID)
}

func TestIsProcessRunning_GetExitCodeFails(t *testing.T) {
	mockChecker := &mockWindowsProcessChecker{
		getExitCodeFunc: func(handle uintptr, exitCode *uint32) error {
			return errors.New("get exit code failed")
		},
	}
	pm := &ProcessManager{
		windowsProcessChecker: mockChecker,
	}

	result := pm.isProcessRunning(1234)
	assert.False(t, result, "isProcessRunning should return false when GetExitCodeProcess fails")
	assert.Len(t, mockChecker.getExitCodeCalls, 1, "GetExitCodeProcess should be called once")
}

func TestIsProcessRunning_ProcessNotActive(t *testing.T) {
	mockChecker := &mockWindowsProcessChecker{
		getExitCodeFunc: func(handle uintptr, exitCode *uint32) error {
			*exitCode = 0 // Process exited
			return nil
		},
	}
	pm := &ProcessManager{
		windowsProcessChecker: mockChecker,
	}

	result := pm.isProcessRunning(1234)
	assert.False(t, result, "isProcessRunning should return false when exit code is not STILL_ACTIVE")
}

func TestIsProcessRunning_ProcessActive(t *testing.T) {
	mockChecker := &mockWindowsProcessChecker{
		getExitCodeFunc: func(handle uintptr, exitCode *uint32) error {
			*exitCode = constants.StillActiveExitCode
			return nil
		},
	}
	pm := &ProcessManager{
		windowsProcessChecker: mockChecker,
	}

	result := pm.isProcessRunning(1234)
	assert.True(t, result, "isProcessRunning should return true when process is active")
}

func TestIsProcessRunning_HandleClosed(t *testing.T) {
	mockChecker := &mockWindowsProcessChecker{
		getExitCodeFunc: func(handle uintptr, exitCode *uint32) error {
			*exitCode = constants.StillActiveExitCode
			return nil
		},
	}
	pm := &ProcessManager{
		windowsProcessChecker: mockChecker,
	}

	pm.isProcessRunning(1234)
	assert.Len(t, mockChecker.closeHandleCalls, 1, "CloseHandle should be called once")
	assert.Equal(t, uintptr(1), mockChecker.closeHandleCalls[0])
}

func TestFindProcessOnPort_NetstatFails(t *testing.T) {
	mockExecutor := &mockCommandExecutor{
		outputFunc: func(cmd *exec.Cmd) ([]byte, error) {
			return nil, errors.New("netstat failed")
		},
	}
	pm := &ProcessManager{
		commandExecutor: mockExecutor,
	}

	result := pm.findProcessOnPort(8080)
	assert.Equal(t, 0, result, "findProcessOnPort should return 0 when netstat fails")
}

func TestFindProcessOnPort_PortFound(t *testing.T) {
	mockExecutor := &mockCommandExecutor{
		outputFunc: func(cmd *exec.Cmd) ([]byte, error) {
			// Typical netstat output format
			return []byte("  TCP    127.0.0.1:8080    0.0.0.0:0    LISTENING    1234"), nil
		},
	}
	pm := &ProcessManager{
		commandExecutor: mockExecutor,
	}

	result := pm.findProcessOnPort(8080)
	assert.Equal(t, 1234, result, "findProcessOnPort should return correct PID")
}

func TestFindProcessOnPort_PortNotFound(t *testing.T) {
	mockExecutor := &mockCommandExecutor{
		outputFunc: func(cmd *exec.Cmd) ([]byte, error) {
			return []byte("  TCP    127.0.0.1:9090    0.0.0.0:0    LISTENING    5678"), nil
		},
	}
	pm := &ProcessManager{
		commandExecutor: mockExecutor,
	}

	result := pm.findProcessOnPort(8080)
	assert.Equal(t, 0, result, "findProcessOnPort should return 0 when port not found")
}

func TestFindProcessOnPort_InvalidPID(t *testing.T) {
	mockExecutor := &mockCommandExecutor{
		outputFunc: func(cmd *exec.Cmd) ([]byte, error) {
			return []byte("  TCP    127.0.0.1:8080    0.0.0.0:0    LISTENING    invalid"), nil
		},
	}
	pm := &ProcessManager{
		commandExecutor: mockExecutor,
	}

	result := pm.findProcessOnPort(8080)
	assert.Equal(t, 0, result, "findProcessOnPort should return 0 when PID is invalid")
}

func TestFindProcessOnPort_InsufficientFields(t *testing.T) {
	mockExecutor := &mockCommandExecutor{
		outputFunc: func(cmd *exec.Cmd) ([]byte, error) {
			return []byte("  TCP    127.0.0.1:8080"), nil
		},
	}
	pm := &ProcessManager{
		commandExecutor: mockExecutor,
	}

	result := pm.findProcessOnPort(8080)
	assert.Equal(t, 0, result, "findProcessOnPort should return 0 when line has insufficient fields")
}

func TestFindProcessOnPort_MultipleLines(t *testing.T) {
	mockExecutor := &mockCommandExecutor{
		outputFunc: func(cmd *exec.Cmd) ([]byte, error) {
			return []byte("  TCP    127.0.0.1:9090    0.0.0.0:0    LISTENING    5678\n  TCP    127.0.0.1:8080    0.0.0.0:0    LISTENING    1234"), nil
		},
	}
	pm := &ProcessManager{
		commandExecutor: mockExecutor,
	}

	result := pm.findProcessOnPort(8080)
	assert.Equal(t, 1234, result, "findProcessOnPort should find port in multiple lines")
}

func TestFindProcessOnPort_CommandArguments(t *testing.T) {
	mockExecutor := &mockCommandExecutor{}
	pm := &ProcessManager{
		commandExecutor: mockExecutor,
	}

	pm.findProcessOnPort(8080)

	require.Len(t, mockExecutor.commandCalls, 1, "Command should be called once")
	call := mockExecutor.commandCalls[0]
	assert.Equal(t, "netstat", call.name)
	assert.Contains(t, call.args, "-ano")
}

func TestFindOperatorProcess_TasklistFails(t *testing.T) {
	mockExecutor := &mockCommandExecutor{
		outputFunc: func(cmd *exec.Cmd) ([]byte, error) {
			return nil, errors.New("tasklist failed")
		},
	}
	pm := &ProcessManager{
		commandExecutor: mockExecutor,
	}

	result := pm.findOperatorProcess()
	assert.Equal(t, 0, result, "findOperatorProcess should return 0 when tasklist fails")
}

func TestFindOperatorProcess_NoG8eProcess(t *testing.T) {
	mockExecutor := &mockCommandExecutor{
		outputFunc: func(cmd *exec.Cmd) ([]byte, error) {
			return []byte("INFO: No tasks are running which match the specified criteria."), nil
		},
	}
	pm := &ProcessManager{
		commandExecutor: mockExecutor,
	}

	result := pm.findOperatorProcess()
	assert.Equal(t, 0, result, "findOperatorProcess should return 0 when no g8e.exe process found")
}

func TestFindOperatorProcess_ProcessFound(t *testing.T) {
	mockExecutor := &mockCommandExecutor{
		outputFunc: func(cmd *exec.Cmd) ([]byte, error) {
			// CSV format with header and data
			return []byte("\"Image Name\",\"PID\",\"Session Name\",\"Session#\",\"Mem Usage\"\n\"g8e.exe\",\"1234\",\"Console\",\"1\",\"5,234 K\""), nil
		},
	}
	pm := &ProcessManager{
		commandExecutor: mockExecutor,
	}

	result := pm.findOperatorProcess()
	assert.Equal(t, 1234, result, "findOperatorProcess should return correct PID")
}

func TestFindOperatorProcess_ExcludesOwnPID(t *testing.T) {
	ownPID := os.Getpid()
	mockExecutor := &mockCommandExecutor{
		outputFunc: func(cmd *exec.Cmd) ([]byte, error) {
			// Include current process PID
			return []byte(fmt.Sprintf("\"Image Name\",\"PID\",\"Session Name\",\"Session#\",\"Mem Usage\"\n\"g8e.exe\",\"%d\",\"Console\",\"1\",\"5,234 K\"", ownPID)), nil
		},
	}
	pm := &ProcessManager{
		commandExecutor: mockExecutor,
	}

	result := pm.findOperatorProcess()
	assert.Equal(t, 0, result, "findOperatorProcess should exclude current process PID")
}

func TestFindOperatorProcess_MultipleProcesses(t *testing.T) {
	mockExecutor := &mockCommandExecutor{
		outputFunc: func(cmd *exec.Cmd) ([]byte, error) {
			// Multiple g8e.exe processes
			return []byte("\"Image Name\",\"PID\",\"Session Name\",\"Session#\",\"Mem Usage\"\n\"g8e.exe\",\"1234\",\"Console\",\"1\",\"5,234 K\"\n\"g8e.exe\",\"5678\",\"Console\",\"1\",\"6,234 K\""), nil
		},
	}
	pm := &ProcessManager{
		commandExecutor: mockExecutor,
	}

	result := pm.findOperatorProcess()
	assert.Equal(t, 1234, result, "findOperatorProcess should return first matching PID")
}

func TestFindOperatorProcess_EmptyLines(t *testing.T) {
	mockExecutor := &mockCommandExecutor{
		outputFunc: func(cmd *exec.Cmd) ([]byte, error) {
			return []byte("\"Image Name\",\"PID\",\"Session Name\",\"Session#\",\"Mem Usage\"\n\n\"g8e.exe\",\"1234\",\"Console\",\"1\",\"5,234 K\""), nil
		},
	}
	pm := &ProcessManager{
		commandExecutor: mockExecutor,
	}

	result := pm.findOperatorProcess()
	assert.Equal(t, 1234, result, "findOperatorProcess should handle empty lines")
}

func TestFindOperatorProcess_InvalidPID(t *testing.T) {
	mockExecutor := &mockCommandExecutor{
		outputFunc: func(cmd *exec.Cmd) ([]byte, error) {
			return []byte("\"Image Name\",\"PID\",\"Session Name\",\"Session#\",\"Mem Usage\"\n\"g8e.exe\",\"invalid\",\"Console\",\"1\",\"5,234 K\""), nil
		},
	}
	pm := &ProcessManager{
		commandExecutor: mockExecutor,
	}

	result := pm.findOperatorProcess()
	assert.Equal(t, 0, result, "findOperatorProcess should return 0 when PID is invalid")
}

func TestFindOperatorProcess_InsufficientFields(t *testing.T) {
	mockExecutor := &mockCommandExecutor{
		outputFunc: func(cmd *exec.Cmd) ([]byte, error) {
			return []byte("\"Image Name\",\"PID\",\"Session Name\",\"Session#\",\"Mem Usage\"\n\"g8e.exe\""), nil
		},
	}
	pm := &ProcessManager{
		commandExecutor: mockExecutor,
	}

	result := pm.findOperatorProcess()
	assert.Equal(t, 0, result, "findOperatorProcess should return 0 when line has insufficient fields")
}

func TestFindOperatorProcess_CommandArguments(t *testing.T) {
	mockExecutor := &mockCommandExecutor{}
	pm := &ProcessManager{
		commandExecutor: mockExecutor,
	}

	pm.findOperatorProcess()

	// The image name is derived from os.Executable() at runtime, falling
	// back to the constant only if that fails.
	expectedImage := constants.BinaryImageNameWindows
	if exePath, err := os.Executable(); err == nil {
		expectedImage = filepath.Base(exePath)
	}

	require.Len(t, mockExecutor.commandCalls, 1, "Command should be called once")
	call := mockExecutor.commandCalls[0]
	assert.Equal(t, "tasklist", call.name)
	assert.Contains(t, call.args, "/FI", fmt.Sprintf("IMAGENAME eq %s", expectedImage))
	assert.Contains(t, call.args, "/FO", "CSV")
}

func TestStopProcess_ZeroPID(t *testing.T) {
	pm := &ProcessManager{}

	err := pm.stopProcess(0, "test")
	assert.NoError(t, err, "stopProcess should not error for PID 0")
}

func TestStopProcess_ProcessNotRunning(t *testing.T) {
	mockChecker := &mockWindowsProcessChecker{
		openProcessFunc: func(desiredAccess uint32, inheritHandle bool, processID uint32) (uintptr, error) {
			return uintptr(0), errors.New("process not found")
		},
	}
	pm := &ProcessManager{
		windowsProcessChecker: mockChecker,
	}

	err := pm.stopProcess(1234, "test")
	assert.NoError(t, err, "stopProcess should not error when process is not running")
}

func TestStopProcess_GracefulShutdownSucceeds(t *testing.T) {
	mockChecker := &mockWindowsProcessChecker{
		getExitCodeFunc: func(handle uintptr, exitCode *uint32) error {
			*exitCode = constants.StillActiveExitCode
			return nil
		},
	}
	mockExecutor := &mockCommandExecutor{
		runFunc: func(cmd *exec.Cmd) error {
			// First call (graceful shutdown) succeeds
			return nil
		},
	}
	pm := &ProcessManager{
		windowsProcessChecker: mockChecker,
		commandExecutor:       mockExecutor,
	}

	// Override isProcessRunning to return false after graceful shutdown
	callCount := 0
	pm.isProcessRunningFn = func(pid int) bool {
		callCount++
		if callCount == 1 {
			return true // First check - process is running
		}
		return false // Second check - process stopped
	}

	err := pm.stopProcess(1234, "test")
	assert.NoError(t, err, "stopProcess should succeed with graceful shutdown")
	assert.Len(t, mockExecutor.runCalls, 1, "taskkill should be called once for graceful shutdown")
}

func TestStopProcess_GracefulShutdownFailsForceKillSucceeds(t *testing.T) {
	mockChecker := &mockWindowsProcessChecker{
		getExitCodeFunc: func(handle uintptr, exitCode *uint32) error {
			*exitCode = constants.StillActiveExitCode
			return nil
		},
	}
	runCallCount := 0
	mockExecutor := &mockCommandExecutor{
		runFunc: func(cmd *exec.Cmd) error {
			runCallCount++
			if runCallCount == 1 {
				return errors.New("graceful shutdown failed")
			}
			return nil // Force kill succeeds
		},
	}
	pm := &ProcessManager{
		windowsProcessChecker: mockChecker,
		commandExecutor:       mockExecutor,
	}

	// Override isProcessRunning to return false after force kill
	callCount := 0
	pm.isProcessRunningFn = func(pid int) bool {
		callCount++
		if callCount <= 2 {
			return true // Process still running after graceful attempt
		}
		return false // Process stopped after force kill
	}

	err := pm.stopProcess(1234, "test")
	assert.NoError(t, err, "stopProcess should succeed with force kill")
	assert.Len(t, mockExecutor.runCalls, 2, "taskkill should be called twice (graceful + force)")
}

func TestStopProcess_ForceKillFails(t *testing.T) {
	mockChecker := &mockWindowsProcessChecker{
		getExitCodeFunc: func(handle uintptr, exitCode *uint32) error {
			*exitCode = constants.StillActiveExitCode
			return nil
		},
	}
	mockExecutor := &mockCommandExecutor{
		runFunc: func(cmd *exec.Cmd) error {
			return errors.New("force kill failed")
		},
	}
	pm := &ProcessManager{
		windowsProcessChecker: mockChecker,
		commandExecutor:       mockExecutor,
	}

	// Override isProcessRunning to always return true
	pm.isProcessRunningFn = func(pid int) bool {
		return true
	}

	err := pm.stopProcess(1234, "test")
	assert.Error(t, err, "stopProcess should error when force kill fails")
	assert.ErrorIs(t, err, constants.ErrProcessStopFailed)
}

func TestStopProcess_WaitForExit(t *testing.T) {
	mockChecker := &mockWindowsProcessChecker{
		getExitCodeFunc: func(handle uintptr, exitCode *uint32) error {
			*exitCode = constants.StillActiveExitCode
			return nil
		},
	}
	mockExecutor := &mockCommandExecutor{
		runFunc: func(cmd *exec.Cmd) error {
			return nil
		},
	}
	pm := &ProcessManager{
		windowsProcessChecker: mockChecker,
		commandExecutor:       mockExecutor,
	}

	// Override isProcessRunning to return false after a few checks
	callCount := 0
	pm.isProcessRunningFn = func(pid int) bool {
		callCount++
		return callCount < 3 // Return true for first 2 checks, false on 3rd
	}

	err := pm.stopProcess(1234, "test")
	assert.NoError(t, err, "stopProcess should succeed after waiting for exit")
}

func TestStopProcess_CommandArguments(t *testing.T) {
	mockChecker := &mockWindowsProcessChecker{
		getExitCodeFunc: func(handle uintptr, exitCode *uint32) error {
			*exitCode = constants.StillActiveExitCode
			return nil
		},
	}
	mockExecutor := &mockCommandExecutor{
		runFunc: func(cmd *exec.Cmd) error {
			return errors.New("force kill")
		},
	}
	pm := &ProcessManager{
		windowsProcessChecker: mockChecker,
		commandExecutor:       mockExecutor,
	}

	// Override isProcessRunning to always return true
	pm.isProcessRunningFn = func(pid int) bool {
		return true
	}

	pm.stopProcess(1234, "test")

	require.Len(t, mockExecutor.commandCalls, 2, "Command should be called twice")

	// First call - graceful shutdown
	call1 := mockExecutor.commandCalls[0]
	assert.Equal(t, "taskkill", call1.name)
	assert.Contains(t, call1.args, "/PID", "1234")
	assert.Contains(t, call1.args, "/T")
	assert.NotContains(t, call1.args, "/F")

	// Second call - force kill
	call2 := mockExecutor.commandCalls[1]
	assert.Equal(t, "taskkill", call2.name)
	assert.Contains(t, call2.args, "/PID", "1234")
	assert.Contains(t, call2.args, "/T")
	assert.Contains(t, call2.args, "/F")
}

func TestStopProcess_ForceKillSucceedsButProcessDoesNotExit(t *testing.T) {
	mockChecker := &mockWindowsProcessChecker{
		getExitCodeFunc: func(handle uintptr, exitCode *uint32) error {
			*exitCode = constants.StillActiveExitCode
			return nil
		},
	}
	mockExecutor := &mockCommandExecutor{
		runFunc: func(cmd *exec.Cmd) error {
			return nil
		},
	}
	pm := &ProcessManager{
		windowsProcessChecker: mockChecker,
		commandExecutor:       mockExecutor,
	}

	// Override isProcessRunning to always return true (process never exits)
	pm.isProcessRunningFn = func(pid int) bool {
		return true
	}

	err := pm.stopProcess(1234, "test")
	assert.Error(t, err, "stopProcess should error when process does not exit after force kill")
	assert.ErrorIs(t, err, constants.ErrProcessForceKillTimeout)
}

func TestRealWindowsProcessChecker(t *testing.T) {
	// Test that real implementations satisfy the interfaces
	var _ WindowsProcessChecker = realWindowsProcessChecker{}
	var _ CommandExecutor = realCommandExecutor{}
}

func TestProcessManager_NilDependencies(t *testing.T) {
	// Test that ProcessManager works with nil dependencies (uses real implementations)
	pm := &ProcessManager{
		windowsProcessChecker: nil,
		commandExecutor:       nil,
	}

	// Should not panic when checking process with PID 0
	result := pm.isProcessRunning(0)
	assert.False(t, result)
}
