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
	"bufio"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strconv"
	"syscall"
	"time"

	"github.com/g8e-ai/g8e/internal/constants"
)

const (
	operatorPIDFile     = "operator.pid"
	operatorLogPath     = "operator.log"
	shutdownTimeout     = 10 * time.Second
	healthCheckInterval = 500 * time.Millisecond
	maxHealthChecks     = 20
	maxPortAttempts     = 100
)

type ProcessManager struct {
	projectRoot string
	runtimeDir  string
	pkiDir      string
	secretsDir  string
	dataDir     string
	logDir      string
	pidDir      string
}

func NewProcessManager(projectRoot string) (*ProcessManager, error) {
	// Resolve paths for this project root
	constants.ResolvePaths(projectRoot)

	runtimeDir := constants.Paths.Infra.RuntimeDir
	pkiDir := constants.Paths.Infra.PkiDir
	secretsDir := constants.Paths.Infra.SecretsDir
	dataDir := constants.Paths.Infra.DataDir
	logDir := filepath.Join(runtimeDir, "logs")
	pidDir := filepath.Join(runtimeDir, "pids")

	return &ProcessManager{
		projectRoot: projectRoot,
		runtimeDir:  runtimeDir,
		pkiDir:      pkiDir,
		secretsDir:  secretsDir,
		dataDir:     dataDir,
		logDir:      logDir,
		pidDir:      pidDir,
	}, nil
}

func (pm *ProcessManager) ensureDirectories() error {
	dirs := []string{pm.runtimeDir, pm.pkiDir, pm.secretsDir, pm.dataDir, pm.logDir, pm.pidDir}
	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0700); err != nil {
			return fmt.Errorf("failed to create directory %s: %w", dir, err)
		}
	}
	return nil
}

func (pm *ProcessManager) checkPortAvailable(port int, name string) error {
	addr := fmt.Sprintf("127.0.0.1:%d", port)
	listener, err := net.Listen(string(constants.NetworkProtocolTCP), addr)
	if err != nil {
		return fmt.Errorf("port %d (%s) is already in use: %w", port, name, err)
	}
	listener.Close()
	return nil
}

func (pm *ProcessManager) findAvailablePort(startPort int, name string) (int, error) {
	pid, err := pm.readPID(operatorPIDFile)
	if err != nil {
		return 0, fmt.Errorf("failed to read pid file: %w", err)
	}

	if pid != 0 && !pm.isProcessRunning(pid) {
		if err := pm.deletePID(operatorPIDFile); err != nil {
			return 0, fmt.Errorf("failed to delete stale pid file %d: %w", pid, err)
		}
	}

	for attempt := 0; attempt < maxPortAttempts; attempt++ {
		port := startPort + attempt
		addr := fmt.Sprintf("127.0.0.1:%d", port)
		listener, err := net.Listen(string(constants.NetworkProtocolTCP), addr)
		if err == nil {
			listener.Close()
			return port, nil
		}

		conflictingPID := pm.findProcessOnPort(port)
		if conflictingPID > 0 && conflictingPID == pid {
			return 0, fmt.Errorf("port %d (%s) is already in use by tracked process %d", port, name, conflictingPID)
		}
	}

	return 0, fmt.Errorf("failed to find available port starting from %d after %d attempts", startPort, maxPortAttempts)
}

func (pm *ProcessManager) readPID(filename string) (int, error) {
	pidFile := filepath.Join(pm.pidDir, filename)
	pidData, err := os.ReadFile(pidFile)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}
		return 0, fmt.Errorf("failed to read pid file: %w", err)
	}

	var pid int
	if _, err := fmt.Sscanf(string(pidData), "%d", &pid); err != nil {
		return 0, fmt.Errorf("failed to parse pid: %w", err)
	}

	return pid, nil
}

func (pm *ProcessManager) writePID(filename string, pid int) error {
	pidFile := filepath.Join(pm.pidDir, filename)
	return os.WriteFile(pidFile, []byte(strconv.Itoa(pid)), 0600)
}

func (pm *ProcessManager) deletePID(filename string) error {
	pidFile := filepath.Join(pm.pidDir, filename)
	if err := os.Remove(pidFile); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to delete pid file: %w", err)
	}
	return nil
}

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

	timeout := time.After(shutdownTimeout)
	ticker := time.NewTicker(healthCheckInterval)
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

func (pm *ProcessManager) getOperatorBinary() (string, error) {
	exePath, err := os.Executable()
	if err == nil {
		return exePath, nil
	}
	return "./g8e", nil
}

func (pm *ProcessManager) StartOperator(httpPort, publicPort int) error {
	if err := pm.ensureDirectories(); err != nil {
		return err
	}

	// Find the first available port starting from httpPort
	availableHTTPPort, err := pm.findAvailablePort(httpPort, "Operator HTTP API")
	if err != nil {
		return fmt.Errorf("failed to find available HTTP API port: %w", err)
	}

	// Calculate offset from original httpPort to maintain port spacing
	offset := availableHTTPPort - httpPort
	availablePublicPort := publicPort + offset

	// Verify the calculated Public port is available (Bootstrap now shares this port)
	if err := pm.checkPortAvailable(availablePublicPort, "Operator Public API"); err != nil {
		return fmt.Errorf("failed to verify Public API port %d: %w", availablePublicPort, err)
	}

	binPath, err := pm.getOperatorBinary()
	if err != nil {
		return err
	}

	logFile := filepath.Join(pm.logDir, operatorLogPath)
	logHandle, err := os.OpenFile(logFile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
	if err != nil {
		return fmt.Errorf("failed to open log file: %w", err)
	}

	cmd := exec.Command(binPath,
		"--doctrine",
		"--working-dir", pm.projectRoot,
		"--data-dir", pm.dataDir,
		"--pki-dir", pm.pkiDir,
		"--secrets-dir", pm.secretsDir,
		"--http-listen-port", strconv.Itoa(availableHTTPPort),
		"--public-listen-port", strconv.Itoa(availablePublicPort),
	)
	cmd.Stdout = logHandle
	cmd.Stderr = logHandle
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Setsid: true,
	}

	if err := cmd.Start(); err != nil {
		if closeErr := logHandle.Close(); closeErr != nil {
			return fmt.Errorf("failed to start operator: %w (additionally failed to close log file: %v)", err, closeErr)
		}
		return fmt.Errorf("failed to start operator: %w", err)
	}

	if err := pm.writePID(operatorPIDFile, cmd.Process.Pid); err != nil {
		_ = cmd.Process.Kill()
		if closeErr := logHandle.Close(); closeErr != nil {
			return fmt.Errorf("failed to write pid file: %w (additionally failed to close log file: %v)", err, closeErr)
		}
		return fmt.Errorf("failed to write pid file: %w", err)
	}

	if err := logHandle.Close(); err != nil {
		return fmt.Errorf("failed to close log file: %w", err)
	}

	time.Sleep(2 * time.Second)
	if !pm.isProcessRunning(cmd.Process.Pid) {
		_ = pm.deletePID(operatorPIDFile)
		return fmt.Errorf("operator failed to start, check %s", logFile)
	}

	return nil
}

func (pm *ProcessManager) StopOperator() error {
	pid, err := pm.readPID(operatorPIDFile)
	if err != nil {
		return err
	}

	if pid == 0 {
		return nil
	}

	if err := pm.stopProcess(pid, "operator"); err != nil {
		return err
	}

	return pm.deletePID(operatorPIDFile)
}

func (pm *ProcessManager) OperatorStatus() (bool, int, error) {
	pid, err := pm.readPID(operatorPIDFile)
	if err != nil {
		return false, 0, err
	}

	if pid == 0 {
		return false, 0, nil
	}

	running := pm.isProcessRunning(pid)
	return running, pid, nil
}

func (pm *ProcessManager) GetLogPath() string {
	return filepath.Join(pm.logDir, operatorLogPath)
}

func (pm *ProcessManager) Reset() error {
	if err := pm.StopOperator(); err != nil {
		return fmt.Errorf("failed to stop operator: %w", err)
	}

	if err := os.RemoveAll(pm.dataDir); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to wipe data directory: %w", err)
	}

	if err := os.RemoveAll(pm.secretsDir); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to wipe secrets directory: %w", err)
	}

	if err := pm.ensureDirectories(); err != nil {
		return fmt.Errorf("failed to recreate directories: %w", err)
	}

	return nil
}

func (pm *ProcessManager) Clean() error {
	if err := pm.StopOperator(); err != nil {
		return fmt.Errorf("failed to stop operator: %w", err)
	}

	if err := os.RemoveAll(pm.runtimeDir); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to remove runtime directory: %w", err)
	}

	return nil
}

// TailLog prints a log file, optionally following new entries (like tail -f)
func TailLog(logPath string, follow bool) error {
	file, err := os.Open(logPath)
	if err != nil {
		return fmt.Errorf("failed to open log file: %w", err)
	}
	defer file.Close()

	// Print existing content
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return fmt.Errorf("failed to seek to start of file: %w", err)
	}
	if _, err := io.Copy(os.Stdout, file); err != nil {
		return fmt.Errorf("failed to print log content: %w", err)
	}

	if !follow {
		return nil
	}

	// Follow mode: seek to end and watch for new content
	if _, err := file.Seek(0, io.SeekEnd); err != nil {
		return fmt.Errorf("failed to seek to end of file: %w", err)
	}

	reader := bufio.NewReader(file)
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-sigChan:
			return nil
		case <-ticker.C:
			line, err := reader.ReadString('\n')
			if err != nil {
				if err == io.EOF {
					continue
				}
				return fmt.Errorf("failed to read log line: %w", err)
			}
			fmt.Print(line)
		}
	}
}
