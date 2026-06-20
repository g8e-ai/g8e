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

package platform

import (
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"testing"

	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/g8e-ai/g8e/internal/paths"
)

func TestNewProcessManager(t *testing.T) {
	tmpDir := t.TempDir()

	pm, err := NewProcessManager(tmpDir)
	if err != nil {
		t.Fatalf("NewProcessManager failed: %v", err)
	}

	if pm.projectRoot != tmpDir {
		t.Errorf("expected projectRoot %s, got %s", tmpDir, pm.projectRoot)
	}

	// ProcessManager should use paths relative to projectRoot
	expectedRuntimeDir := filepath.Join(tmpDir, ".g8e")
	if pm.runtimeDir != expectedRuntimeDir {
		t.Errorf("expected runtimeDir %s, got %s", expectedRuntimeDir, pm.runtimeDir)
	}

	expectedPKIDir := filepath.Join(tmpDir, ".g8e/pki")
	if pm.pkiDir != expectedPKIDir {
		t.Errorf("expected pkiDir %s, got %s", expectedPKIDir, pm.pkiDir)
	}

}

func TestEnsureDirectories(t *testing.T) {
	tmpDir := t.TempDir()
	pm, err := NewProcessManager(tmpDir)
	if err != nil {
		t.Fatalf("NewProcessManager failed: %v", err)
	}

	if err := pm.ensureDirectories(); err != nil {
		t.Fatalf("ensureDirectories failed: %v", err)
	}

	dirs := []string{pm.runtimeDir, pm.pkiDir, pm.secretsDir, pm.dataDir, pm.logDir, pm.pidDir}
	for _, dir := range dirs {
		info, err := os.Stat(dir)
		if err != nil {
			t.Errorf("directory %s does not exist: %v", dir, err)
		}
		if !info.IsDir() {
			t.Errorf("%s is not a directory", dir)
		}
		// Verify permissions are 0700 on Unix systems
		// Windows uses ACLs and doesn't support Unix-style permissions
		if runtime.GOOS != "windows" {
			if info.Mode().Perm() != 0700 {
				t.Errorf("directory %s has incorrect permissions %o, expected 0700", dir, info.Mode().Perm())
			}
		}
	}
}

func TestFindAvailablePort(t *testing.T) {
	tmpDir := t.TempDir()
	pm, err := NewProcessManager(tmpDir)
	if err != nil {
		t.Fatalf("NewProcessManager failed: %v", err)
	}

	// Find an available port
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to find available port: %v", err)
	}
	addr := listener.Addr().(*net.TCPAddr)
	availablePort := addr.Port
	listener.Close()

	// Test available port
	port, err := pm.findAvailablePort(availablePort, "test")
	if err != nil {
		t.Errorf("port %d should be available: %v", availablePort, err)
	}
	if port != availablePort {
		t.Errorf("expected port %d, got %d", availablePort, port)
	}

	// Test port in use by untracked process
	listener, err = net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", availablePort))
	if err != nil {
		t.Fatalf("failed to listen on port %d: %v", availablePort, err)
	}
	defer listener.Close()

	port, err = pm.findAvailablePort(availablePort, "test")
	if err != nil {
		t.Errorf("should find next available port: %v", err)
	}
	if port == availablePort {
		t.Error("should return different port when default is in use")
	}
}

func TestReadPID(t *testing.T) {
	tmpDir := t.TempDir()
	pm, err := NewProcessManager(tmpDir)
	if err != nil {
		t.Fatalf("NewProcessManager failed: %v", err)
	}

	if err := pm.ensureDirectories(); err != nil {
		t.Fatalf("ensureDirectories failed: %v", err)
	}

	// Test non-existent PID file
	pid, err := pm.readPID("nonexistent.pid")
	if err != nil {
		t.Errorf("readPID should return nil for non-existent file, got %v", err)
	}
	if pid != 0 {
		t.Errorf("expected pid 0 for non-existent file, got %d", pid)
	}

	// Test valid PID file
	testPID := 12345
	if err := pm.writePID("test.pid", testPID); err != nil {
		t.Fatalf("writePID failed: %v", err)
	}

	pid, err = pm.readPID("test.pid")
	if err != nil {
		t.Errorf("readPID failed: %v", err)
	}
	if pid != testPID {
		t.Errorf("expected pid %d, got %d", testPID, pid)
	}

	// Test malformed PID file
	malformedFile := filepath.Join(pm.pidDir, "malformed.pid")
	if err := os.WriteFile(malformedFile, []byte("not-a-number"), 0600); err != nil {
		t.Fatalf("failed to write malformed PID file: %v", err)
	}

	_, err = pm.readPID("malformed.pid")
	if err == nil {
		t.Error("expected error for malformed PID file, got nil")
	}
}

func TestWritePID(t *testing.T) {
	tmpDir := t.TempDir()
	pm, err := NewProcessManager(tmpDir)
	if err != nil {
		t.Fatalf("NewProcessManager failed: %v", err)
	}

	if err := pm.ensureDirectories(); err != nil {
		t.Fatalf("ensureDirectories failed: %v", err)
	}

	testPID := 54321
	if err := pm.writePID("test.pid", testPID); err != nil {
		t.Fatalf("writePID failed: %v", err)
	}

	pidFile := filepath.Join(pm.pidDir, "test.pid")
	data, err := os.ReadFile(pidFile)
	if err != nil {
		t.Fatalf("failed to read PID file: %v", err)
	}

	pid, err := strconv.Atoi(string(data))
	if err != nil {
		t.Fatalf("failed to parse PID: %v", err)
	}

	if pid != testPID {
		t.Errorf("expected PID %d, got %d", testPID, pid)
	}

	// Verify file permissions on Unix systems
	// Windows uses ACLs and doesn't support Unix-style permissions
	if runtime.GOOS != "windows" {
		info, err := os.Stat(pidFile)
		if err != nil {
			t.Fatalf("failed to stat PID file: %v", err)
		}
		if info.Mode().Perm() != 0600 {
			t.Errorf("PID file has incorrect permissions %o, expected 0600", info.Mode().Perm())
		}
	}
}

func TestDeletePID(t *testing.T) {
	tmpDir := t.TempDir()
	pm, err := NewProcessManager(tmpDir)
	if err != nil {
		t.Fatalf("NewProcessManager failed: %v", err)
	}

	if err := pm.ensureDirectories(); err != nil {
		t.Fatalf("ensureDirectories failed: %v", err)
	}

	// Test deleting existing PID file
	if err := pm.writePID("test.pid", 12345); err != nil {
		t.Fatalf("writePID failed: %v", err)
	}

	if err := pm.deletePID("test.pid"); err != nil {
		t.Errorf("deletePID failed: %v", err)
	}

	pidFile := filepath.Join(pm.pidDir, "test.pid")
	if _, err := os.Stat(pidFile); !os.IsNotExist(err) {
		t.Error("PID file should not exist after deletion")
	}

	// Test deleting non-existent PID file (should not error)
	if err := pm.deletePID("nonexistent.pid"); err != nil {
		t.Errorf("deletePID should not error for non-existent file: %v", err)
	}
}

func TestIsProcessRunning(t *testing.T) {
	tmpDir := t.TempDir()
	pm, err := NewProcessManager(tmpDir)
	if err != nil {
		t.Fatalf("NewProcessManager failed: %v", err)
	}

	// Test with PID 0
	if pm.isProcessRunning(0) {
		t.Error("isProcessRunning should return false for PID 0")
	}

	// Test with invalid PID
	if pm.isProcessRunning(999999) {
		t.Error("isProcessRunning should return false for invalid PID")
	}

	// Test with current process (should be running)
	// On Windows, isG8eProcess checks if the process is g8e.exe, which the test binary is not
	// So we skip this check on Windows
	if runtime.GOOS != "windows" {
		currentPID := os.Getpid()
		if !pm.isProcessRunning(currentPID) {
			t.Error("isProcessRunning should return true for current process")
		}
	}
}

func TestOperatorStatus(t *testing.T) {
	tmpDir := t.TempDir()
	pm, err := NewProcessManager(tmpDir)
	if err != nil {
		t.Fatalf("NewProcessManager failed: %v", err)
	}

	if err := pm.ensureDirectories(); err != nil {
		t.Fatalf("ensureDirectories failed: %v", err)
	}

	// Mock findOperatorProcess to return 0 (no process found)
	pm.findOperatorProcessFn = func() int { return 0 }

	// Test no PID file
	running, pid, err := pm.OperatorStatus()
	if err != nil {
		t.Errorf("OperatorStatus failed: %v", err)
	}
	if running {
		t.Error("expected running=false when no PID file exists")
	}
	if pid != 0 {
		t.Errorf("expected pid=0 when no PID file exists, got %d", pid)
	}

	// Test with PID file for non-existent process
	// On Windows, isG8eProcess checks if the process is g8e.exe, which the test binary is not
	// So the PID file will be deleted and 0 will be returned
	if err := pm.writePID(constants.OperatorPIDFilename, 999999); err != nil {
		t.Fatalf("writePID failed: %v", err)
	}

	running, pid, err = pm.OperatorStatus()
	if err != nil {
		t.Errorf("OperatorStatus failed: %v", err)
	}
	if running {
		t.Error("expected running=false for non-existent process")
	}
	// The PID file is deleted when the process is not running (stale PID cleanup)
	if pid != 0 {
		t.Errorf("expected pid=0 after stale PID cleanup, got %d", pid)
	}

	// Test with PID file for current process
	// On Windows, isG8eProcess checks if the process is g8e.exe, which the test binary is not
	// So we skip this check on Windows
	if runtime.GOOS != "windows" {
		if err := pm.writePID(constants.OperatorPIDFilename, os.Getpid()); err != nil {
			t.Fatalf("writePID failed: %v", err)
		}

		running, pid, err = pm.OperatorStatus()
		if err != nil {
			t.Errorf("OperatorStatus failed: %v", err)
		}
		if !running {
			t.Error("expected running=true for current process")
		}
		if pid != os.Getpid() {
			t.Errorf("expected pid=%d, got %d", os.Getpid(), pid)
		}
	}
}

func TestStopOperator(t *testing.T) {
	tmpDir := t.TempDir()
	pm, err := NewProcessManager(tmpDir)
	if err != nil {
		t.Fatalf("NewProcessManager failed: %v", err)
	}

	if err := pm.ensureDirectories(); err != nil {
		t.Fatalf("ensureDirectories failed: %v", err)
	}

	// Test stopping when no PID file exists and no process found
	pm.findOperatorProcessFn = func() int { return 0 }
	if err := pm.StopOperator(); err != nil {
		t.Errorf("StopOperator should not error when no PID file exists: %v", err)
	}

	// Test stopping when no PID file exists but process is found via fallback
	// Mock findOperatorProcess to return a non-existent PID (simulating stale process)
	pm.findOperatorProcessFn = func() int { return 999998 }
	if err := pm.StopOperator(); err != nil {
		t.Errorf("StopOperator with fallback should attempt to stop process: %v", err)
	}
	// Reset mock
	pm.findOperatorProcessFn = func() int { return 0 }

	// Test stopping non-existent process with PID file
	if err := pm.writePID(constants.OperatorPIDFilename, 999999); err != nil {
		t.Fatalf("writePID failed: %v", err)
	}

	if err := pm.StopOperator(); err != nil {
		t.Errorf("StopOperator failed: %v", err)
	}

	// Verify PID file was deleted
	pidFile := filepath.Join(pm.pidDir, constants.OperatorPIDFilename)
	if _, err := os.Stat(pidFile); !os.IsNotExist(err) {
		t.Error("PID file should be deleted after stop")
	}
}

func TestGetLogPath(t *testing.T) {
	tmpDir := t.TempDir()
	pm, err := NewProcessManager(tmpDir)
	if err != nil {
		t.Fatalf("NewProcessManager failed: %v", err)
	}

	expectedPath := filepath.Join(pm.logDir, paths.OperatorLogPath)
	actualPath := pm.GetLogPath()

	if actualPath != expectedPath {
		t.Errorf("expected log path %s, got %s", expectedPath, actualPath)
	}
}

func TestGetOperatorNodeBinary(t *testing.T) {
	tmpDir := t.TempDir()
	pm, err := NewProcessManager(tmpDir)
	if err != nil {
		t.Fatalf("NewProcessManager failed: %v", err)
	}

	binPath, err := pm.getOperatorBinary()
	if err != nil {
		t.Errorf("getOperatorBinary failed: %v", err)
	}

	// During test execution, os.Executable() returns the test binary path
	// Just verify the path is not empty and is absolute
	if binPath == "" {
		t.Error("expected non-empty binary path")
	}
	if !filepath.IsAbs(binPath) {
		t.Errorf("expected absolute path, got %s", binPath)
	}
}

func TestReset(t *testing.T) {
	tmpDir := t.TempDir()
	pm, err := NewProcessManager(tmpDir)
	if err != nil {
		t.Fatalf("NewProcessManager failed: %v", err)
	}

	if err := pm.ensureDirectories(); err != nil {
		t.Fatalf("ensureDirectories failed: %v", err)
	}

	// Create some test data in dataDir
	testFile := filepath.Join(pm.dataDir, "test.txt")
	if err := os.WriteFile(testFile, []byte("test"), 0600); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	// Create some test data in secretsDir
	secretFile := filepath.Join(pm.secretsDir, "secret.txt")
	if err := os.WriteFile(secretFile, []byte("secret"), 0600); err != nil {
		t.Fatalf("failed to create secret file: %v", err)
	}

	// Run reset
	if err := pm.Reset(); err != nil {
		t.Fatalf("Reset failed: %v", err)
	}

	// Verify dataDir was wiped
	if _, err := os.Stat(testFile); !os.IsNotExist(err) {
		t.Error("dataDir should be wiped")
	}

	// Verify secretsDir was wiped
	if _, err := os.Stat(secretFile); !os.IsNotExist(err) {
		t.Error("secretsDir should be wiped")
	}

	// Verify directories were recreated
	dirs := []string{pm.dataDir, pm.secretsDir}
	for _, dir := range dirs {
		if _, err := os.Stat(dir); err != nil {
			t.Errorf("directory %s should be recreated: %v", dir, err)
		}
	}
}

func TestClean(t *testing.T) {
	tmpDir := t.TempDir()
	pm, err := NewProcessManager(tmpDir)
	if err != nil {
		t.Fatalf("NewProcessManager failed: %v", err)
	}

	if err := pm.ensureDirectories(); err != nil {
		t.Fatalf("ensureDirectories failed: %v", err)
	}

	// Create some test data
	testFile := filepath.Join(pm.runtimeDir, "test.txt")
	if err := os.WriteFile(testFile, []byte("test"), 0600); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	// Run clean
	if err := pm.Clean(); err != nil {
		t.Fatalf("Clean failed: %v", err)
	}

	// Verify runtimeDir was removed
	if _, err := os.Stat(pm.runtimeDir); !os.IsNotExist(err) {
		t.Error("runtimeDir should be removed")
	}
}

func TestStopProcess(t *testing.T) {
	tmpDir := t.TempDir()
	pm, err := NewProcessManager(tmpDir)
	if err != nil {
		t.Fatalf("NewProcessManager failed: %v", err)
	}

	// Test stopping with PID 0
	if err := pm.stopProcess(0, "test"); err != nil {
		t.Errorf("stopProcess should not error for PID 0: %v", err)
	}

	// Test stopping non-existent process
	if err := pm.stopProcess(999999, "test"); err != nil {
		t.Errorf("stopProcess should not error for non-existent process: %v", err)
	}
}

func TestTailLog(t *testing.T) {
	tmpDir := t.TempDir()
	logFile := filepath.Join(tmpDir, "test.log")

	// Create a log file with some content
	content := "line1\nline2\nline3\n"
	if err := os.WriteFile(logFile, []byte(content), 0600); err != nil {
		t.Fatalf("failed to create log file: %v", err)
	}

	// Test tailing a non-existent file
	err := TailLog(filepath.Join(tmpDir, "nonexistent.log"), false)
	if err == nil {
		t.Error("expected error for non-existent log file")
	}

	// Note: We cannot test the actual tailing behavior in a unit test
	// as it would block waiting for input. Integration tests would be needed
	// to verify the full tailing functionality.
}

func TestConstants(t *testing.T) {
	// Verify constants are set correctly
	if constants.OperatorPIDFilename == "" {
		t.Error("constants.OperatorPIDFilename should not be empty")
	}
	if paths.OperatorLogPath == "" {
		t.Error("paths.OperatorLogPath should not be empty")
	}
	if ShutdownTimeout == 0 {
		t.Error("ShutdownTimeout should not be zero")
	}
	if HealthCheckInterval == 0 {
		t.Error("HealthCheckInterval should not be zero")
	}
	if MaxHealthChecks == 0 {
		t.Error("MaxHealthChecks should not be zero")
	}
}

func TestProcessManagerConcurrency(t *testing.T) {
	tmpDir := t.TempDir()
	pm, err := NewProcessManager(tmpDir)
	if err != nil {
		t.Fatalf("NewProcessManager failed: %v", err)
	}

	if err := pm.ensureDirectories(); err != nil {
		t.Fatalf("ensureDirectories failed: %v", err)
	}

	// Test concurrent PID file operations
	done := make(chan bool)
	for i := 0; i < 10; i++ {
		go func(idx int) {
			pid := 10000 + idx
			filename := fmt.Sprintf("concurrent%d.pid", idx)
			pm.writePID(filename, pid)
			readPID, _ := pm.readPID(filename)
			if readPID != pid {
				t.Errorf("concurrent write/read mismatch: expected %d, got %d", pid, readPID)
			}
			pm.deletePID(filename)
			done <- true
		}(i)
	}

	for i := 0; i < 10; i++ {
		<-done
	}
}

func TestProcessManagerDirectoryPermissions(t *testing.T) {
	tmpDir := t.TempDir()
	pm, err := NewProcessManager(tmpDir)
	if err != nil {
		t.Fatalf("NewProcessManager failed: %v", err)
	}

	// Test that ensureDirectories creates directories with correct permissions
	if err := pm.ensureDirectories(); err != nil {
		t.Fatalf("ensureDirectories failed: %v", err)
	}

	// Check each directory has 0700 permissions on Unix systems
	// Windows uses ACLs and doesn't support Unix-style permissions
	if runtime.GOOS != "windows" {
		dirs := []string{pm.runtimeDir, pm.pkiDir, pm.secretsDir, pm.dataDir, pm.logDir, pm.pidDir}
		for _, dir := range dirs {
			info, err := os.Stat(dir)
			if err != nil {
				t.Fatalf("failed to stat directory %s: %v", dir, err)
			}
			if info.Mode().Perm() != 0700 {
				t.Errorf("directory %s has incorrect permissions %o, expected 0700", dir, info.Mode().Perm())
			}
		}
	}
}

func TestProcessManagerErrorHandling(t *testing.T) {
	tmpDir := t.TempDir()
	pm, err := NewProcessManager(tmpDir)
	if err != nil {
		t.Fatalf("NewProcessManager failed: %v", err)
	}

	// Test reading PID from a directory instead of a file
	if err := pm.ensureDirectories(); err != nil {
		t.Fatalf("ensureDirectories failed: %v", err)
	}

	dirAsFile := filepath.Join(pm.pidDir, "dir_as_pid")
	if err := os.Mkdir(dirAsFile, 0700); err != nil {
		t.Fatalf("failed to create directory: %v", err)
	}

	_, err = pm.readPID("dir_as_pid")
	if err == nil {
		t.Error("expected error when reading PID from a directory")
	}
}

func TestWritePIDPermissions(t *testing.T) {
	tmpDir := t.TempDir()
	pm, err := NewProcessManager(tmpDir)
	if err != nil {
		t.Fatalf("NewProcessManager failed: %v", err)
	}

	if err := pm.ensureDirectories(); err != nil {
		t.Fatalf("ensureDirectories failed: %v", err)
	}

	// Test that PID files are written with 0600 permissions on Unix systems
	// Windows uses ACLs and doesn't support Unix-style permissions
	if runtime.GOOS != "windows" {
		testPID := 99999
		if err := pm.writePID("perms.pid", testPID); err != nil {
			t.Fatalf("writePID failed: %v", err)
		}

		pidFile := filepath.Join(pm.pidDir, "perms.pid")
		info, err := os.Stat(pidFile)
		if err != nil {
			t.Fatalf("failed to stat PID file: %v", err)
		}

		if info.Mode().Perm() != 0600 {
			t.Errorf("PID file has incorrect permissions %o, expected 0600", info.Mode().Perm())
		}
	}
}

func TestReadPIDEmptyFile(t *testing.T) {
	tmpDir := t.TempDir()
	pm, err := NewProcessManager(tmpDir)
	if err != nil {
		t.Fatalf("NewProcessManager failed: %v", err)
	}

	if err := pm.ensureDirectories(); err != nil {
		t.Fatalf("ensureDirectories failed: %v", err)
	}

	// Test reading an empty PID file
	emptyFile := filepath.Join(pm.pidDir, "empty.pid")
	if err := os.WriteFile(emptyFile, []byte(""), 0600); err != nil {
		t.Fatalf("failed to write empty PID file: %v", err)
	}

	_, err = pm.readPID("empty.pid")
	if err == nil {
		t.Error("expected error when reading empty PID file")
	}
}

func TestReadPIDWhitespace(t *testing.T) {
	tmpDir := t.TempDir()
	pm, err := NewProcessManager(tmpDir)
	if err != nil {
		t.Fatalf("NewProcessManager failed: %v", err)
	}

	if err := pm.ensureDirectories(); err != nil {
		t.Fatalf("ensureDirectories failed: %v", err)
	}

	// Test reading PID file with whitespace
	whitespaceFile := filepath.Join(pm.pidDir, "whitespace.pid")
	if err := os.WriteFile(whitespaceFile, []byte("  12345  "), 0600); err != nil {
		t.Fatalf("failed to write whitespace PID file: %v", err)
	}

	pid, err := pm.readPID("whitespace.pid")
	if err != nil {
		t.Errorf("readPID failed: %v", err)
	}
	if pid != 12345 {
		t.Errorf("expected pid 12345, got %d", pid)
	}
}

func TestFindAvailablePortInvalidPort(t *testing.T) {
	tmpDir := t.TempDir()
	pm, err := NewProcessManager(tmpDir)
	if err != nil {
		t.Fatalf("NewProcessManager failed: %v", err)
	}

	// Test with a port that's out of valid range (should still work for the check)
	// The actual bind will fail, but the check itself should attempt it
	_, err = pm.findAvailablePort(70000, "test")
	if err == nil {
		t.Error("expected error for invalid port 70000")
	}
}

func TestDeletePIDNonExistent(t *testing.T) {
	tmpDir := t.TempDir()
	pm, err := NewProcessManager(tmpDir)
	if err != nil {
		t.Fatalf("NewProcessManager failed: %v", err)
	}

	if err := pm.ensureDirectories(); err != nil {
		t.Fatalf("ensureDirectories failed: %v", err)
	}

	// Test deleting a PID file that doesn't exist (should not error)
	if err := pm.deletePID("does_not_exist.pid"); err != nil {
		t.Errorf("deletePID should not error for non-existent file: %v", err)
	}
}

func TestIsProcessRunningNegativePID(t *testing.T) {
	tmpDir := t.TempDir()
	pm, err := NewProcessManager(tmpDir)
	if err != nil {
		t.Fatalf("NewProcessManager failed: %v", err)
	}

	// Test with negative PID
	if pm.isProcessRunning(-1) {
		t.Error("isProcessRunning should return false for negative PID")
	}
}

func TestGetOperatorNodeBinaryPath(t *testing.T) {
	tmpDir := t.TempDir()
	pm, err := NewProcessManager(tmpDir)
	if err != nil {
		t.Fatalf("NewProcessManager failed: %v", err)
	}

	binPath, err := pm.getOperatorBinary()
	if err != nil {
		t.Errorf("getOperatorBinary failed: %v", err)
	}

	// During test execution, os.Executable() returns the test binary path
	// Just verify the path is not empty and is absolute
	if binPath == "" {
		t.Error("expected non-empty binary path")
	}
	if !filepath.IsAbs(binPath) {
		t.Errorf("expected absolute path, got %s", binPath)
	}
}

func TestCleanWithNonExistentRuntime(t *testing.T) {
	tmpDir := t.TempDir()
	pm, err := NewProcessManager(tmpDir)
	if err != nil {
		t.Fatalf("NewProcessManager failed: %v", err)
	}

	// Don't create runtime directory - it should not error
	if err := pm.Clean(); err != nil {
		t.Errorf("Clean should not error when runtime doesn't exist: %v", err)
	}
}

func TestCheckPortAvailable(t *testing.T) {
	tmpDir := t.TempDir()
	pm, err := NewProcessManager(tmpDir)
	if err != nil {
		t.Fatalf("NewProcessManager failed: %v", err)
	}

	// Find an available port
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to find available port: %v", err)
	}
	addr := listener.Addr().(*net.TCPAddr)
	availablePort := addr.Port
	listener.Close()

	// Test available port
	if err := pm.checkPortAvailable(availablePort, "test"); err != nil {
		t.Errorf("port %d should be available: %v", availablePort, err)
	}

	// Test port in use
	listener, err = net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", availablePort))
	if err != nil {
		t.Fatalf("failed to listen on port %d: %v", availablePort, err)
	}
	defer listener.Close()

	err = pm.checkPortAvailable(availablePort, "test")
	if err == nil {
		t.Error("expected error for port in use")
	}
}

func TestWritePosture(t *testing.T) {
	tmpDir := t.TempDir()
	pm, err := NewProcessManager(tmpDir)
	if err != nil {
		t.Fatalf("NewProcessManager failed: %v", err)
	}

	if err := pm.ensureDirectories(); err != nil {
		t.Fatalf("ensureDirectories failed: %v", err)
	}

	testPosture := "doctrine"
	if err := pm.writePosture(testPosture); err != nil {
		t.Fatalf("writePosture failed: %v", err)
	}

	postureFile := filepath.Join(pm.pidDir, constants.OperatorPostureFilename)
	data, err := os.ReadFile(postureFile)
	if err != nil {
		t.Fatalf("failed to read posture file: %v", err)
	}

	if string(data) != testPosture {
		t.Errorf("expected posture %s, got %s", testPosture, string(data))
	}

	// Verify file permissions on Unix systems
	if runtime.GOOS != "windows" {
		info, err := os.Stat(postureFile)
		if err != nil {
			t.Fatalf("failed to stat posture file: %v", err)
		}
		if info.Mode().Perm() != 0600 {
			t.Errorf("posture file has incorrect permissions %o, expected 0600", info.Mode().Perm())
		}
	}
}

func TestReadPosture(t *testing.T) {
	tmpDir := t.TempDir()
	pm, err := NewProcessManager(tmpDir)
	if err != nil {
		t.Fatalf("NewProcessManager failed: %v", err)
	}

	if err := pm.ensureDirectories(); err != nil {
		t.Fatalf("ensureDirectories failed: %v", err)
	}

	// Test non-existent posture file
	posture, err := pm.readPosture()
	if err != nil {
		t.Errorf("readPosture should return nil for non-existent file: %v", err)
	}
	if posture != "" {
		t.Errorf("expected empty posture for non-existent file, got %s", posture)
	}

	// Test valid posture
	validPostures := []string{"doctrine", "consensus", "notary"}
	for _, p := range validPostures {
		if err := pm.writePosture(p); err != nil {
			t.Fatalf("writePosture failed: %v", err)
		}

		posture, err = pm.readPosture()
		if err != nil {
			t.Errorf("readPosture failed for %s: %v", p, err)
		}
		if posture != p {
			t.Errorf("expected posture %s, got %s", p, posture)
		}
	}

	// Test invalid posture
	if err := pm.writePosture("invalid"); err != nil {
		t.Fatalf("writePosture failed: %v", err)
	}

	_, err = pm.readPosture()
	if err == nil {
		t.Error("expected error for invalid posture value")
	}
}

func TestDeletePosture(t *testing.T) {
	tmpDir := t.TempDir()
	pm, err := NewProcessManager(tmpDir)
	if err != nil {
		t.Fatalf("NewProcessManager failed: %v", err)
	}

	if err := pm.ensureDirectories(); err != nil {
		t.Fatalf("ensureDirectories failed: %v", err)
	}

	// Test deleting existing posture file
	if err := pm.writePosture("doctrine"); err != nil {
		t.Fatalf("writePosture failed: %v", err)
	}

	if err := pm.deletePosture(); err != nil {
		t.Errorf("deletePosture failed: %v", err)
	}

	postureFile := filepath.Join(pm.pidDir, constants.OperatorPostureFilename)
	if _, err := os.Stat(postureFile); !os.IsNotExist(err) {
		t.Error("posture file should not exist after deletion")
	}

	// Test deleting non-existent posture file (should not error)
	if err := pm.deletePosture(); err != nil {
		t.Errorf("deletePosture should not error for non-existent file: %v", err)
	}
}

func TestReadPosturePublic(t *testing.T) {
	tmpDir := t.TempDir()
	pm, err := NewProcessManager(tmpDir)
	if err != nil {
		t.Fatalf("NewProcessManager failed: %v", err)
	}

	if err := pm.ensureDirectories(); err != nil {
		t.Fatalf("ensureDirectories failed: %v", err)
	}

	// Test ReadPosture public method
	if err := pm.writePosture("consensus"); err != nil {
		t.Fatalf("writePosture failed: %v", err)
	}

	posture, err := pm.ReadPosture()
	if err != nil {
		t.Errorf("ReadPosture failed: %v", err)
	}
	if posture != "consensus" {
		t.Errorf("expected posture consensus, got %s", posture)
	}
}

func TestSetProcessGroup(t *testing.T) {
	// Test that setProcessGroup sets the appropriate process group attributes
	cmd := exec.Command("echo", "test")
	setProcessGroup(cmd)

	// On Unix, SysProcAttr should be set
	// On Windows, it's a no-op
	if runtime.GOOS != "windows" {
		if cmd.SysProcAttr == nil {
			t.Error("SysProcAttr should be set on Unix")
		}
		if !cmd.SysProcAttr.Setsid {
			t.Error("Setsid should be true on Unix")
		}
	}
}
