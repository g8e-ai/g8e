// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package platform

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"testing"

	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/g8e-ai/g8e/internal/services/fs"
	"github.com/g8e-ai/g8e/internal/testutil"
)

func newPlatformTestFileSvc(t *testing.T, baseDir string) fs.RuntimeFileService {
	t.Helper()
	fileSvc, err := fs.NewRuntimeFileService(baseDir, slog.Default())
	if err != nil {
		t.Fatalf("failed to create fileSvc: %v", err)
	}
	return fileSvc
}

func TestNewProcessManager(t *testing.T) {
	t.Run("returns error for nil fileSvc", func(t *testing.T) {
		_, err := NewProcessManager(nil)
		if err == nil {
			t.Error("expected error for nil fileSvc")
		}
	})

	t.Run("accepts valid fileSvc", func(t *testing.T) {
		tmpDir := testutil.TempDir(t)
		fileSvc := newPlatformTestFileSvc(t, tmpDir)

		pm, err := NewProcessManager(fileSvc)
		if err != nil {
			t.Fatalf("NewProcessManager failed: %v", err)
		}

		if pm.fileSvc == nil {
			t.Error("expected fileSvc to be set")
		}

		// Verify path resolution works
		expectedRuntimeDir := filepath.Join(tmpDir, constants.RuntimeDirname)
		if pm.fileSvc.Resolve("") != expectedRuntimeDir {
			t.Errorf("expected runtime dir %s, got %s", expectedRuntimeDir, pm.fileSvc.Resolve(""))
		}

		expectedPKIDir := filepath.Join(tmpDir, constants.RuntimeDirname, constants.PkiDirname)
		if pm.fileSvc.Resolve(constants.PkiDirname) != expectedPKIDir {
			t.Errorf("expected pkiDir %s, got %s", expectedPKIDir, pm.fileSvc.Resolve(constants.PkiDirname))
		}
	})
}

func TestEnsureDirectories(t *testing.T) {
	tmpDir := testutil.TempDir(t)
	fileSvc := newPlatformTestFileSvc(t, tmpDir)
	pm, err := NewProcessManager(fileSvc)
	if err != nil {
		t.Fatalf("NewProcessManager failed: %v", err)
	}

	if err := pm.CreateDirectories(); err != nil {
		t.Fatalf("ensureDirectories failed: %v", err)
	}

	dirs := []struct {
		path string
		mode os.FileMode
	}{
		{fileSvc.Resolve(""), constants.PermDirPrivate},
		{fileSvc.Resolve(constants.PkiDirname), constants.PermDirStandard},
		{fileSvc.Resolve(constants.SecretsDirname), constants.PermDirPrivate},
		{fileSvc.Resolve(constants.DataDirname), constants.PermDirStandard},
		{fileSvc.Resolve(constants.LogDirname), constants.PermDirStandard},
		{fileSvc.Resolve(constants.PidDirname), constants.PermDirStandard},
		{fileSvc.Resolve(constants.BinDirname), constants.PermDirStandard},
	}
	for _, d := range dirs {
		info, err := os.Stat(d.path)
		if err != nil {
			t.Errorf("directory %s does not exist: %v", d.path, err)
		}
		if !info.IsDir() {
			t.Errorf("%s is not a directory", d.path)
		}
		if runtime.GOOS != "windows" {
			if info.Mode().Perm() != d.mode {
				t.Errorf("directory %s has incorrect permissions %o, expected %o", d.path, info.Mode().Perm(), d.mode)
			}
		}
	}
}

func TestFindAvailablePort(t *testing.T) {
	tmpDir := testutil.TempDir(t)
	fileSvc := newPlatformTestFileSvc(t, tmpDir)
	pm, err := NewProcessManager(fileSvc)
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
	tmpDir := testutil.TempDir(t)
	fileSvc := newPlatformTestFileSvc(t, tmpDir)
	pm, err := NewProcessManager(fileSvc)
	if err != nil {
		t.Fatalf("NewProcessManager failed: %v", err)
	}

	if err := pm.CreateDirectories(); err != nil {
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
	if err := fileSvc.WriteFile(context.Background(), filepath.Join(constants.PidDirname, "malformed.pid"), []byte("not-a-number"), constants.PermFilePrivate); err != nil {
		t.Fatalf("failed to write malformed PID file: %v", err)
	}

	_, err = pm.readPID("malformed.pid")
	if err == nil {
		t.Error("expected error for malformed PID file, got nil")
	}
}

func TestWritePID(t *testing.T) {
	tmpDir := testutil.TempDir(t)
	fileSvc := newPlatformTestFileSvc(t, tmpDir)
	pm, err := NewProcessManager(fileSvc)
	if err != nil {
		t.Fatalf("NewProcessManager failed: %v", err)
	}

	if err := pm.CreateDirectories(); err != nil {
		t.Fatalf("ensureDirectories failed: %v", err)
	}

	testPID := 54321
	if err := pm.writePID("test.pid", testPID); err != nil {
		t.Fatalf("writePID failed: %v", err)
	}

	pidFile := fileSvc.Resolve(filepath.Join(constants.PidDirname, "test.pid"))
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

	if runtime.GOOS != "windows" {
		info, err := os.Stat(pidFile)
		if err != nil {
			t.Fatalf("failed to stat PID file: %v", err)
		}
		if info.Mode().Perm() != constants.PermFilePrivate {
			t.Errorf("PID file has incorrect permissions %o, expected %o", info.Mode().Perm(), constants.PermFilePrivate)
		}
	}
}

func TestDeletePID(t *testing.T) {
	tmpDir := testutil.TempDir(t)
	fileSvc := newPlatformTestFileSvc(t, tmpDir)
	pm, err := NewProcessManager(fileSvc)
	if err != nil {
		t.Fatalf("NewProcessManager failed: %v", err)
	}

	if err := pm.CreateDirectories(); err != nil {
		t.Fatalf("ensureDirectories failed: %v", err)
	}

	// Test deleting existing PID file
	if err := pm.writePID("test.pid", 12345); err != nil {
		t.Fatalf("writePID failed: %v", err)
	}

	if err := pm.deletePID("test.pid"); err != nil {
		t.Errorf("deletePID failed: %v", err)
	}

	pidFile := fileSvc.Resolve(filepath.Join(constants.PidDirname, "test.pid"))
	exists, err := fileSvc.FileExists(context.Background(), filepath.Join(constants.PidDirname, "test.pid"))
	if err != nil {
		t.Fatalf("FileExists failed: %v", err)
	}
	if exists {
		t.Errorf("PID file should not exist after deletion: %s", pidFile)
	}

	// Test deleting non-existent PID file (should not error)
	if err := pm.deletePID("nonexistent.pid"); err != nil {
		t.Errorf("deletePID should not error for non-existent file: %v", err)
	}
}

func TestIsProcessRunning(t *testing.T) {
	tmpDir := testutil.TempDir(t)
	fileSvc := newPlatformTestFileSvc(t, tmpDir)
	pm, err := NewProcessManager(fileSvc)
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
	if runtime.GOOS != "windows" {
		currentPID := os.Getpid()
		if !pm.isProcessRunning(currentPID) {
			t.Error("isProcessRunning should return true for current process")
		}
	}
}

func TestOperatorStatus(t *testing.T) {
	tmpDir := testutil.TempDir(t)
	fileSvc := newPlatformTestFileSvc(t, tmpDir)
	pm, err := NewProcessManager(fileSvc)
	if err != nil {
		t.Fatalf("NewProcessManager failed: %v", err)
	}

	if err := pm.CreateDirectories(); err != nil {
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
	tmpDir := testutil.TempDir(t)
	fileSvc := newPlatformTestFileSvc(t, tmpDir)
	pm, err := NewProcessManager(fileSvc)
	if err != nil {
		t.Fatalf("NewProcessManager failed: %v", err)
	}

	if err := pm.CreateDirectories(); err != nil {
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
	pidRelPath := filepath.Join(constants.PidDirname, constants.OperatorPIDFilename)
	exists, err := fileSvc.FileExists(context.Background(), pidRelPath)
	if err != nil {
		t.Fatalf("FileExists failed: %v", err)
	}
	if exists {
		t.Error("PID file should be deleted after stop")
	}
}

func TestGetLogPath(t *testing.T) {
	tmpDir := testutil.TempDir(t)
	fileSvc := newPlatformTestFileSvc(t, tmpDir)
	pm, err := NewProcessManager(fileSvc)
	if err != nil {
		t.Fatalf("NewProcessManager failed: %v", err)
	}

	expectedPath := filepath.Join(fileSvc.Resolve(constants.LogDirname), constants.G8eLogFilename)
	actualPath := pm.GetLogPath()

	if actualPath != expectedPath {
		t.Errorf("expected log path %s, got %s", expectedPath, actualPath)
	}
}

func TestCopyBinaryToBinDir_CopiesExecutableToBinDir(t *testing.T) {
	tmpDir := testutil.TempDir(t)
	fileSvc := newPlatformTestFileSvc(t, tmpDir)
	pm, err := NewProcessManager(fileSvc)
	if err != nil {
		t.Fatalf("NewProcessManager failed: %v", err)
	}

	binPath, err := pm.copyBinaryToBinDir()
	if err != nil {
		t.Fatalf("copyBinaryToBinDir failed: %v", err)
	}

	if binPath == "" {
		t.Error("expected non-empty binary path")
	}
	if !filepath.IsAbs(binPath) {
		t.Errorf("expected absolute path, got %s", binPath)
	}

	// Verify the binary was copied to .g8e/bin/<name>
	expectedName := constants.BinaryImageName
	if runtime.GOOS == "windows" {
		expectedName = constants.BinaryImageNameWindows
	}
	expectedPath := filepath.Join(tmpDir, constants.RuntimeDirname, constants.BinDirname, expectedName)
	if binPath != expectedPath {
		t.Errorf("expected bin path %s, got %s", expectedPath, binPath)
	}

	// Verify the file exists and is executable
	info, err := os.Stat(binPath)
	if err != nil {
		t.Fatalf("copied binary not found: %v", err)
	}
	if info.Size() == 0 {
		t.Error("copied binary is empty")
	}
	if runtime.GOOS != "windows" {
		if info.Mode().Perm() != constants.PermFileExecutable {
			t.Errorf("copied binary has permissions %o, expected %o", info.Mode().Perm(), constants.PermFileExecutable)
		}
	}
}

func TestClean(t *testing.T) {
	tmpDir := testutil.TempDir(t)
	fileSvc := newPlatformTestFileSvc(t, tmpDir)
	pm, err := NewProcessManager(fileSvc)
	if err != nil {
		t.Fatalf("NewProcessManager failed: %v", err)
	}

	if err := pm.CreateDirectories(); err != nil {
		t.Fatalf("ensureDirectories failed: %v", err)
	}

	// Create some test data
	testFile := fileSvc.Resolve("test.txt")
	if err := os.WriteFile(testFile, []byte("test"), constants.PermFilePrivate); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	// Run clean
	if err := pm.Clean(); err != nil {
		t.Fatalf("Clean failed: %v", err)
	}

	// Verify runtimeDir was removed
	runtimeExists, err := fileSvc.FileExists(context.Background(), "test.txt")
	if err != nil {
		t.Fatalf("FileExists failed: %v", err)
	}
	if runtimeExists {
		t.Error("runtimeDir should be removed")
	}
}

func TestStopProcess(t *testing.T) {
	tmpDir := testutil.TempDir(t)
	fileSvc := newPlatformTestFileSvc(t, tmpDir)
	pm, err := NewProcessManager(fileSvc)
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
	tmpDir := testutil.TempDir(t)
	logFile := filepath.Join(tmpDir, "test.log")

	// Create a log file with some content
	content := "line1\nline2\nline3\n"
	if err := os.WriteFile(logFile, []byte(content), constants.PermFilePrivate); err != nil {
		t.Fatalf("failed to create log file: %v", err)
	}

	// TailLog now takes an io.ReadSeeker opened by the caller. The
	// nonexistent-file case is handled by the caller via LogFileExists /
	// OpenLogForRead returning constants.ErrNotFound (covered by
	// TestLogService_OpenLogForRead_NotFound in the logging package), so
	// TailLog no longer handles os.Open errors.
	f, err := os.Open(logFile)
	if err != nil {
		t.Fatalf("failed to open log file: %v", err)
	}
	defer f.Close()

	if err := TailLog(f, false); err != nil {
		t.Fatalf("TailLog returned error: %v", err)
	}

	// Note: We cannot test the follow-mode tailing behavior in a unit test
	// as it would block waiting for input. Integration tests would be needed
	// to verify the full tailing functionality.
}

func TestConstants(t *testing.T) {
	// Verify constants are set correctly
	if constants.OperatorPIDFilename == "" {
		t.Error("constants.OperatorPIDFilename should not be empty")
	}
	if constants.G8eLogFilename == "" {
		t.Error("constants.G8eLogFilename should not be empty")
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
	tmpDir := testutil.TempDir(t)
	fileSvc := newPlatformTestFileSvc(t, tmpDir)
	pm, err := NewProcessManager(fileSvc)
	if err != nil {
		t.Fatalf("NewProcessManager failed: %v", err)
	}

	if err := pm.CreateDirectories(); err != nil {
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
	tmpDir := testutil.TempDir(t)
	fileSvc := newPlatformTestFileSvc(t, tmpDir)
	pm, err := NewProcessManager(fileSvc)
	if err != nil {
		t.Fatalf("NewProcessManager failed: %v", err)
	}

	// Test that ensureDirectories creates directories with correct permissions
	if err := pm.CreateDirectories(); err != nil {
		t.Fatalf("ensureDirectories failed: %v", err)
	}

	// Check each directory has correct permissions on Unix systems
	// Windows uses ACLs and doesn't support Unix-style permissions
	if runtime.GOOS != "windows" {
		dirs := []struct {
			path string
			mode os.FileMode
		}{
			{fileSvc.Resolve(""), constants.PermDirPrivate},
			{fileSvc.Resolve(constants.PkiDirname), constants.PermDirStandard},
			{fileSvc.Resolve(constants.SecretsDirname), constants.PermDirPrivate},
			{fileSvc.Resolve(constants.DataDirname), constants.PermDirStandard},
			{fileSvc.Resolve(constants.LogDirname), constants.PermDirStandard},
			{fileSvc.Resolve(constants.PidDirname), constants.PermDirStandard},
		}
		for _, d := range dirs {
			info, err := os.Stat(d.path)
			if err != nil {
				t.Fatalf("failed to stat directory %s: %v", d.path, err)
			}
			if info.Mode().Perm() != d.mode {
				t.Errorf("directory %s has incorrect permissions %o, expected %o", d.path, info.Mode().Perm(), d.mode)
			}
		}
	}
}

func TestProcessManagerErrorHandling(t *testing.T) {
	tmpDir := testutil.TempDir(t)
	fileSvc := newPlatformTestFileSvc(t, tmpDir)
	pm, err := NewProcessManager(fileSvc)
	if err != nil {
		t.Fatalf("NewProcessManager failed: %v", err)
	}

	// Test reading PID from a directory instead of a file
	if err := pm.CreateDirectories(); err != nil {
		t.Fatalf("ensureDirectories failed: %v", err)
	}

	dirAsFile := fileSvc.Resolve(filepath.Join(constants.PidDirname, "dir_as_pid"))
	if err := os.Mkdir(dirAsFile, constants.PermDirPrivate); err != nil {
		t.Fatalf("failed to create directory: %v", err)
	}

	_, err = pm.readPID("dir_as_pid")
	if err == nil {
		t.Error("expected error when reading PID from a directory")
	}
}

func TestWritePIDPermissions(t *testing.T) {
	tmpDir := testutil.TempDir(t)
	fileSvc := newPlatformTestFileSvc(t, tmpDir)
	pm, err := NewProcessManager(fileSvc)
	if err != nil {
		t.Fatalf("NewProcessManager failed: %v", err)
	}

	if err := pm.CreateDirectories(); err != nil {
		t.Fatalf("ensureDirectories failed: %v", err)
	}

	// Test that PID files are written with correct permissions on Unix systems
	// Windows uses ACLs and doesn't support Unix-style permissions
	if runtime.GOOS != "windows" {
		testPID := 99999
		if err := pm.writePID("perms.pid", testPID); err != nil {
			t.Fatalf("writePID failed: %v", err)
		}

		pidFile := fileSvc.Resolve(filepath.Join(constants.PidDirname, "perms.pid"))
		info, err := os.Stat(pidFile)
		if err != nil {
			t.Fatalf("failed to stat PID file: %v", err)
		}

		if info.Mode().Perm() != constants.PermFilePrivate {
			t.Errorf("PID file has incorrect permissions %o, expected %o", info.Mode().Perm(), constants.PermFilePrivate)
		}
	}
}

func TestReadPIDEmptyFile(t *testing.T) {
	tmpDir := testutil.TempDir(t)
	fileSvc := newPlatformTestFileSvc(t, tmpDir)
	pm, err := NewProcessManager(fileSvc)
	if err != nil {
		t.Fatalf("NewProcessManager failed: %v", err)
	}

	if err := pm.CreateDirectories(); err != nil {
		t.Fatalf("ensureDirectories failed: %v", err)
	}

	// Test reading an empty PID file
	emptyFile := fileSvc.Resolve(filepath.Join(constants.PidDirname, "empty.pid"))
	if err := os.WriteFile(emptyFile, []byte(""), constants.PermFilePrivate); err != nil {
		t.Fatalf("failed to write empty PID file: %v", err)
	}

	_, err = pm.readPID("empty.pid")
	if err == nil {
		t.Error("expected error when reading empty PID file")
	}
}

func TestReadPIDWhitespace(t *testing.T) {
	tmpDir := testutil.TempDir(t)
	fileSvc := newPlatformTestFileSvc(t, tmpDir)
	pm, err := NewProcessManager(fileSvc)
	if err != nil {
		t.Fatalf("NewProcessManager failed: %v", err)
	}

	if err := pm.CreateDirectories(); err != nil {
		t.Fatalf("ensureDirectories failed: %v", err)
	}

	// Test reading PID file with whitespace
	whitespaceFile := fileSvc.Resolve(filepath.Join(constants.PidDirname, "whitespace.pid"))
	if err := os.WriteFile(whitespaceFile, []byte("  12345  "), constants.PermFilePrivate); err != nil {
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
	tmpDir := testutil.TempDir(t)
	fileSvc := newPlatformTestFileSvc(t, tmpDir)
	pm, err := NewProcessManager(fileSvc)
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
	tmpDir := testutil.TempDir(t)
	fileSvc := newPlatformTestFileSvc(t, tmpDir)
	pm, err := NewProcessManager(fileSvc)
	if err != nil {
		t.Fatalf("NewProcessManager failed: %v", err)
	}

	if err := pm.CreateDirectories(); err != nil {
		t.Fatalf("ensureDirectories failed: %v", err)
	}

	// Test deleting a PID file that doesn't exist (should not error)
	if err := pm.deletePID("does_not_exist.pid"); err != nil {
		t.Errorf("deletePID should not error for non-existent file: %v", err)
	}
}

func TestIsProcessRunningNegativePID(t *testing.T) {
	tmpDir := testutil.TempDir(t)
	fileSvc := newPlatformTestFileSvc(t, tmpDir)
	pm, err := NewProcessManager(fileSvc)
	if err != nil {
		t.Fatalf("NewProcessManager failed: %v", err)
	}

	// Test with negative PID
	if pm.isProcessRunning(-1) {
		t.Error("isProcessRunning should return false for negative PID")
	}
}

func TestCopyBinaryToBinDir_SkipsWhenIdentical(t *testing.T) {
	tmpDir := testutil.TempDir(t)
	fileSvc := newPlatformTestFileSvc(t, tmpDir)
	pm, err := NewProcessManager(fileSvc)
	if err != nil {
		t.Fatalf("NewProcessManager failed: %v", err)
	}

	// First copy creates the binary
	firstPath, err := pm.copyBinaryToBinDir()
	if err != nil {
		t.Fatalf("first copyBinaryToBinDir failed: %v", err)
	}

	firstInfo, err := os.Stat(firstPath)
	if err != nil {
		t.Fatalf("stat first copy failed: %v", err)
	}

	// Second call should skip the copy (same size and modtime)
	secondPath, err := pm.copyBinaryToBinDir()
	if err != nil {
		t.Fatalf("second copyBinaryToBinDir failed: %v", err)
	}

	if firstPath != secondPath {
		t.Errorf("expected same path both calls, got %s then %s", firstPath, secondPath)
	}

	secondInfo, err := os.Stat(secondPath)
	if err != nil {
		t.Fatalf("stat second copy failed: %v", err)
	}

	// Modtime should be unchanged (not rewritten)
	if !firstInfo.ModTime().Equal(secondInfo.ModTime()) {
		t.Errorf("modtime changed between calls: %v -> %v", firstInfo.ModTime(), secondInfo.ModTime())
	}
}

func TestCleanWithNonExistentRuntime(t *testing.T) {
	tmpDir := testutil.TempDir(t)
	fileSvc := newPlatformTestFileSvc(t, tmpDir)
	pm, err := NewProcessManager(fileSvc)
	if err != nil {
		t.Fatalf("NewProcessManager failed: %v", err)
	}

	// Don't create runtime directory - it should not error
	if err := pm.Clean(); err != nil {
		t.Errorf("Clean should not error when runtime doesn't exist: %v", err)
	}
}

func TestCheckPortAvailable(t *testing.T) {
	tmpDir := testutil.TempDir(t)
	fileSvc := newPlatformTestFileSvc(t, tmpDir)
	pm, err := NewProcessManager(fileSvc)
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
	tmpDir := testutil.TempDir(t)
	fileSvc := newPlatformTestFileSvc(t, tmpDir)
	pm, err := NewProcessManager(fileSvc)
	if err != nil {
		t.Fatalf("NewProcessManager failed: %v", err)
	}

	if err := pm.CreateDirectories(); err != nil {
		t.Fatalf("ensureDirectories failed: %v", err)
	}

	testPosture := "doctrine"
	if err := pm.writePosture(testPosture); err != nil {
		t.Fatalf("writePosture failed: %v", err)
	}

	postureFile := fileSvc.Resolve(filepath.Join(constants.PidDirname, constants.OperatorPostureFilename))
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
		if info.Mode().Perm() != constants.PermFilePrivate {
			t.Errorf("posture file has incorrect permissions %o, expected %o", info.Mode().Perm(), constants.PermFilePrivate)
		}
	}
}

func TestReadPosture(t *testing.T) {
	tmpDir := testutil.TempDir(t)
	fileSvc := newPlatformTestFileSvc(t, tmpDir)
	pm, err := NewProcessManager(fileSvc)
	if err != nil {
		t.Fatalf("NewProcessManager failed: %v", err)
	}

	if err := pm.CreateDirectories(); err != nil {
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
	validPostures := []string{constants.PostureDoctrine, constants.PostureConsensus, constants.PostureNotary}
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
	tmpDir := testutil.TempDir(t)
	fileSvc := newPlatformTestFileSvc(t, tmpDir)
	pm, err := NewProcessManager(fileSvc)
	if err != nil {
		t.Fatalf("NewProcessManager failed: %v", err)
	}

	if err := pm.CreateDirectories(); err != nil {
		t.Fatalf("ensureDirectories failed: %v", err)
	}

	// Test deleting existing posture file
	if err := pm.writePosture("doctrine"); err != nil {
		t.Fatalf("writePosture failed: %v", err)
	}

	if err := pm.deletePosture(); err != nil {
		t.Errorf("deletePosture failed: %v", err)
	}

	postureRelPath := filepath.Join(constants.PidDirname, constants.OperatorPostureFilename)
	exists, err := fileSvc.FileExists(context.Background(), postureRelPath)
	if err != nil {
		t.Fatalf("FileExists failed: %v", err)
	}
	if exists {
		t.Error("posture file should not exist after deletion")
	}

	// Test deleting non-existent posture file (should not error)
	if err := pm.deletePosture(); err != nil {
		t.Errorf("deletePosture should not error for non-existent file: %v", err)
	}
}

func TestReadPosturePublic(t *testing.T) {
	tmpDir := testutil.TempDir(t)
	fileSvc := newPlatformTestFileSvc(t, tmpDir)
	pm, err := NewProcessManager(fileSvc)
	if err != nil {
		t.Fatalf("NewProcessManager failed: %v", err)
	}

	if err := pm.CreateDirectories(); err != nil {
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
