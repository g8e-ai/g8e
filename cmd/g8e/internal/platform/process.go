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
	"strconv"
	"syscall"
	"time"
)

const (
	operatorPIDFile     = "operator.pid"
	g8eePIDFile         = "g8ee.pid"
	operatorLogPath     = "operator.log"
	g8eeLogPath         = "g8ee.log"
	shutdownTimeout     = 10 * time.Second
	healthCheckInterval = 500 * time.Millisecond
	maxHealthChecks     = 20
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
	runtimeDir := filepath.Join(projectRoot, ".g8e")
	pkiDir := filepath.Join(runtimeDir, "pki")
	secretsDir := filepath.Join(runtimeDir, "secrets")
	dataDir := filepath.Join(runtimeDir, "data")
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
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("port %d (%s) is already in use: %w", port, name, err)
	}
	listener.Close()
	return nil
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

func (pm *ProcessManager) stopProcess(pid int, name string) error {
	if pid == 0 {
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
	hostArch := "amd64"

	var uname syscall.Utsname
	if err := syscall.Uname(&uname); err == nil {
		machine := ""
		for _, b := range uname.Machine {
			if b == 0 {
				break
			}
			machine += string(byte(b))
		}
		switch machine {
		case "x86_64":
			hostArch = "amd64"
		case "aarch64", "arm64":
			hostArch = "arm64"
		case "i386", "i686":
			hostArch = "386"
		}
	}

	binPath := filepath.Join(pm.projectRoot, "services", "g8eo", "build", fmt.Sprintf("linux-%s", hostArch), "g8e.gateway")
	return binPath, nil
}

func (pm *ProcessManager) buildOperator() error {
	binPath, _ := pm.getOperatorBinary()
	if _, err := os.Stat(binPath); err == nil {
		return nil
	}

	cmd := exec.Command("make", "-C", filepath.Join(pm.projectRoot, "services", "g8eo"), "build-local")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func (pm *ProcessManager) StartOperator(httpPort, bootstrapPort, publicPort int) error {
	if err := pm.ensureDirectories(); err != nil {
		return err
	}

	if err := pm.checkPortAvailable(httpPort, "Operator HTTP API"); err != nil {
		return err
	}
	if err := pm.checkPortAvailable(bootstrapPort, "Operator Bootstrap"); err != nil {
		return err
	}
	if err := pm.checkPortAvailable(publicPort, "Operator Public API"); err != nil {
		return err
	}

	if err := pm.buildOperator(); err != nil {
		return fmt.Errorf("failed to build operator: %w", err)
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
		"--data-dir", pm.dataDir,
		"--pki-dir", pm.pkiDir,
		"--secrets-dir", pm.secretsDir,
		"--http-listen-port", strconv.Itoa(httpPort),
		"--bootstrap-listen-port", strconv.Itoa(bootstrapPort),
		"--public-listen-port", strconv.Itoa(publicPort),
	)
	cmd.Stdout = logHandle
	cmd.Stderr = logHandle
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Setsid: true,
	}

	if err := cmd.Start(); err != nil {
		logHandle.Close()
		return fmt.Errorf("failed to start operator: %w", err)
	}

	if err := pm.writePID(operatorPIDFile, cmd.Process.Pid); err != nil {
		cmd.Process.Kill()
		logHandle.Close()
		return fmt.Errorf("failed to write pid file: %w", err)
	}

	logHandle.Close()

	time.Sleep(2 * time.Second)
	if !pm.isProcessRunning(cmd.Process.Pid) {
		pm.deletePID(operatorPIDFile)
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

	g8eeDataDir := filepath.Join(pm.projectRoot, "services", "g8ee", "data")
	if err := os.RemoveAll(g8eeDataDir); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to wipe g8ee data directory: %w", err)
	}

	if err := pm.ensureDirectories(); err != nil {
		return fmt.Errorf("failed to recreate directories: %w", err)
	}

	return nil
}

func (pm *ProcessManager) getG8eeBinary() (string, error) {
	venvPython := filepath.Join(pm.projectRoot, ".venv", "bin", "python")
	if _, err := os.Stat(venvPython); os.IsNotExist(err) {
		return "", fmt.Errorf("g8ee venv not found at %s", venvPython)
	}
	return venvPython, nil
}

func (pm *ProcessManager) StartG8ee() error {
	if err := pm.ensureDirectories(); err != nil {
		return err
	}

	pythonBin, err := pm.getG8eeBinary()
	if err != nil {
		return err
	}

	g8eeDir := filepath.Join(pm.projectRoot, "services", "g8ee")
	logFile := filepath.Join(pm.logDir, g8eeLogPath)
	logHandle, err := os.OpenFile(logFile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
	if err != nil {
		return fmt.Errorf("failed to open log file: %w", err)
	}

	cmd := exec.Command(pythonBin, "-m", "uvicorn", "app.main:app", "--host", "0.0.0.0", "--port", "8443")
	cmd.Dir = g8eeDir
	cmd.Stdout = logHandle
	cmd.Stderr = logHandle
	cmd.Env = append(os.Environ(),
		"G8E_RUNTIME_DIR="+pm.runtimeDir,
		"G8E_PKI_DIR="+pm.pkiDir,
		"G8E_SECRETS_DIR="+pm.secretsDir,
	)
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Setsid: true,
	}

	if err := cmd.Start(); err != nil {
		logHandle.Close()
		return fmt.Errorf("failed to start g8ee: %w", err)
	}

	if err := pm.writePID(g8eePIDFile, cmd.Process.Pid); err != nil {
		cmd.Process.Kill()
		logHandle.Close()
		return fmt.Errorf("failed to write pid file: %w", err)
	}

	logHandle.Close()

	time.Sleep(2 * time.Second)
	if !pm.isProcessRunning(cmd.Process.Pid) {
		pm.deletePID(g8eePIDFile)
		return fmt.Errorf("g8ee failed to start, check %s", logFile)
	}

	return nil
}

func (pm *ProcessManager) StopG8ee() error {
	pid, err := pm.readPID(g8eePIDFile)
	if err != nil {
		return err
	}

	if pid == 0 {
		return nil
	}

	if err := pm.stopProcess(pid, "g8ee"); err != nil {
		return err
	}

	return pm.deletePID(g8eePIDFile)
}

func (pm *ProcessManager) G8eeStatus() (bool, int, error) {
	pid, err := pm.readPID(g8eePIDFile)
	if err != nil {
		return false, 0, err
	}

	if pid == 0 {
		return false, 0, nil
	}

	running := pm.isProcessRunning(pid)
	return running, pid, nil
}

func (pm *ProcessManager) Clean() error {
	if err := pm.StopOperator(); err != nil {
		return fmt.Errorf("failed to stop operator: %w", err)
	}

	if err := pm.StopG8ee(); err != nil {
		return fmt.Errorf("failed to stop g8ee: %w", err)
	}

	if err := os.RemoveAll(pm.runtimeDir); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to remove runtime directory: %w", err)
	}

	if err := filepath.Walk(pm.projectRoot, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if info.IsDir() && info.Name() == "__pycache__" {
			if err := os.RemoveAll(path); err != nil {
				return nil
			}
		}
		if !info.IsDir() {
			ext := filepath.Ext(path)
			if ext == ".pyc" || ext == ".pyo" {
				if err := os.Remove(path); err != nil {
					return nil
				}
			}
		}
		return nil
	}); err != nil {
		return fmt.Errorf("failed to clean Python caches: %w", err)
	}

	return nil
}
