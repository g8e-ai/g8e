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
	"path/filepath"
	"strconv"
	"testing"
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

	expectedRuntimeDir := filepath.Join(tmpDir, ".g8e")
	if pm.runtimeDir != expectedRuntimeDir {
		t.Errorf("expected runtimeDir %s, got %s", expectedRuntimeDir, pm.runtimeDir)
	}

	expectedPKIDir := filepath.Join(expectedRuntimeDir, "pki")
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
		// Verify permissions are 0700
		if info.Mode().Perm() != 0700 {
			t.Errorf("directory %s has incorrect permissions %o, expected 0700", dir, info.Mode().Perm())
		}
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
		t.Error("expected error for port in use, got nil")
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

	// Verify file permissions
	info, err := os.Stat(pidFile)
	if err != nil {
		t.Fatalf("failed to stat PID file: %v", err)
	}
	if info.Mode().Perm() != 0600 {
		t.Errorf("PID file has incorrect permissions %o, expected 0600", info.Mode().Perm())
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
	currentPID := os.Getpid()
	if !pm.isProcessRunning(currentPID) {
		t.Error("isProcessRunning should return true for current process")
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
	if err := pm.writePID(operatorPIDFile, 999999); err != nil {
		t.Fatalf("writePID failed: %v", err)
	}

	running, pid, err = pm.OperatorStatus()
	if err != nil {
		t.Errorf("OperatorStatus failed: %v", err)
	}
	if running {
		t.Error("expected running=false for non-existent process")
	}
	if pid != 999999 {
		t.Errorf("expected pid=999999, got %d", pid)
	}

	// Test with PID file for current process
	if err := pm.writePID(operatorPIDFile, os.Getpid()); err != nil {
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

func TestStopOperator(t *testing.T) {
	tmpDir := t.TempDir()
	pm, err := NewProcessManager(tmpDir)
	if err != nil {
		t.Fatalf("NewProcessManager failed: %v", err)
	}

	if err := pm.ensureDirectories(); err != nil {
		t.Fatalf("ensureDirectories failed: %v", err)
	}

	// Test stopping when no PID file exists
	if err := pm.StopOperator(); err != nil {
		t.Errorf("StopOperator should not error when no PID file exists: %v", err)
	}

	// Test stopping non-existent process
	if err := pm.writePID(operatorPIDFile, 999999); err != nil {
		t.Fatalf("writePID failed: %v", err)
	}

	if err := pm.StopOperator(); err != nil {
		t.Errorf("StopOperator failed: %v", err)
	}

	// Verify PID file was deleted
	pidFile := filepath.Join(pm.pidDir, operatorPIDFile)
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

	expectedPath := filepath.Join(pm.logDir, operatorLogPath)
	actualPath := pm.GetLogPath()

	if actualPath != expectedPath {
		t.Errorf("expected log path %s, got %s", expectedPath, actualPath)
	}
}

func TestGetOperatorBinary(t *testing.T) {
	tmpDir := t.TempDir()
	pm, err := NewProcessManager(tmpDir)
	if err != nil {
		t.Fatalf("NewProcessManager failed: %v", err)
	}

	binPath, err := pm.getOperatorBinary()
	if err != nil {
		t.Errorf("getOperatorBinary failed: %v", err)
	}

	expectedPath := filepath.Join(pm.projectRoot, "bin", "g8e")
	if binPath != expectedPath {
		t.Errorf("expected binary path %s, got %s", expectedPath, binPath)
	}

	// Verify the path ends with "g8e"
	if filepath.Base(binPath) != "g8e" {
		t.Errorf("expected binary name 'g8e', got %s", filepath.Base(binPath))
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
	err := TailLog(filepath.Join(tmpDir, "nonexistent.log"))
	if err == nil {
		t.Error("expected error for non-existent log file")
	}

	// Note: We cannot test the actual tailing behavior in a unit test
	// as it would block waiting for input. Integration tests would be needed
	// to verify the full tailing functionality.
}

func TestConstants(t *testing.T) {
	// Verify constants are set correctly
	if operatorPIDFile == "" {
		t.Error("operatorPIDFile should not be empty")
	}
	if operatorLogPath == "" {
		t.Error("operatorLogPath should not be empty")
	}
	if shutdownTimeout == 0 {
		t.Error("shutdownTimeout should not be zero")
	}
	if healthCheckInterval == 0 {
		t.Error("healthCheckInterval should not be zero")
	}
	if maxHealthChecks == 0 {
		t.Error("maxHealthChecks should not be zero")
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

	// Check each directory has 0700 permissions
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

	// Test that PID files are written with 0600 permissions
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

func TestCheckPortAvailableInvalidPort(t *testing.T) {
	tmpDir := t.TempDir()
	pm, err := NewProcessManager(tmpDir)
	if err != nil {
		t.Fatalf("NewProcessManager failed: %v", err)
	}

	// Test with a port that's out of valid range (should still work for the check)
	// The actual bind will fail, but the check itself should attempt it
	err = pm.checkPortAvailable(-1, "test")
	if err == nil {
		t.Error("expected error for invalid port -1")
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

func TestGetOperatorBinaryPath(t *testing.T) {
	tmpDir := t.TempDir()
	pm, err := NewProcessManager(tmpDir)
	if err != nil {
		t.Fatalf("NewProcessManager failed: %v", err)
	}

	binPath, err := pm.getOperatorBinary()
	if err != nil {
		t.Errorf("getOperatorBinary failed: %v", err)
	}

	// Verify the path structure
	expectedPath := filepath.Join(pm.projectRoot, "bin", "g8e")
	if binPath != expectedPath {
		t.Errorf("binary path should be %s, got %s", expectedPath, binPath)
	}

	// Verify the parent directory is "bin"
	parentDir := filepath.Dir(binPath)
	if filepath.Base(parentDir) != "bin" {
		t.Errorf("expected parent directory to be 'bin', got %s", parentDir)
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
