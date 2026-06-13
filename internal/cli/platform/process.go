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
	operatorPostureFile = "operator.posture"
	operatorLogPath     = "operator.log"
	shutdownTimeout     = 10 * time.Second
	healthCheckInterval = 500 * time.Millisecond
	maxHealthChecks     = 20
	maxPortAttempts     = 100
)

// OperatorStartOptions holds configuration for starting the operator process.
// This replaces the 17 positional parameters previously used by StartOperator.
type OperatorStartOptions struct {
	Posture            string
	HTTPPort           int
	HTTPSPort          int
	DataDir            string
	PKIDir             string
	SecretsDir         string
	VaultDir           string
	VaultKeyPath       string
	VaultRequireUnlock bool
	PasskeyRpID        string
	PasskeyRpName      string
	RateLimitRPS       float64
	RateLimitBurst     int
	LogLevel           string
	CertIdentityMode   string
	IdentityData       []byte
}

type ProcessManager struct {
	projectRoot string
	runtimeDir  string
	pkiDir      string
	secretsDir  string
	dataDir     string
	logDir      string
	pidDir      string
	// findOperatorProcessFn allows mocking for tests
	findOperatorProcessFn func() int
}

func NewProcessManager(projectRoot string) (*ProcessManager, error) {
	// Initialize paths relative to projectRoot
	if err := constants.InitPathsWithBase(projectRoot); err != nil {
		return nil, fmt.Errorf("process manager: failed to initialize paths: %w", err)
	}

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

func (pm *ProcessManager) networkIdentityArgs(identityData []byte) ([]string, error) {
	if len(identityData) == 0 {
		return nil, nil
	}

	identityFile, err := pm.writeNetworkIdentityFile(identityData)
	if err != nil {
		return nil, err
	}

	return []string{"--network-identity-file", identityFile}, nil
}

func (pm *ProcessManager) writeNetworkIdentityFile(identityData []byte) (string, error) {
	identityFile := filepath.Join(pm.runtimeDir, "network-identity.json")
	if err := os.WriteFile(identityFile, identityData, 0600); err != nil {
		return "", fmt.Errorf("failed to write network identity file: %w", err)
	}
	return identityFile, nil
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

func (pm *ProcessManager) writePosture(posture string) error {
	postureFile := filepath.Join(pm.pidDir, operatorPostureFile)
	return os.WriteFile(postureFile, []byte(posture), 0600)
}

func (pm *ProcessManager) readPosture() (string, error) {
	postureFile := filepath.Join(pm.pidDir, operatorPostureFile)
	postureData, err := os.ReadFile(postureFile)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", fmt.Errorf("failed to read posture file: %w", err)
	}
	posture := string(postureData)
	// Validate posture is one of the allowed values
	if posture != "" && posture != "doctrine" && posture != "consensus" && posture != "notary" {
		return "", fmt.Errorf("invalid posture value '%s' in posture file: must be doctrine, consensus, or notary", posture)
	}
	return posture, nil
}

func (pm *ProcessManager) deletePosture() error {
	postureFile := filepath.Join(pm.pidDir, operatorPostureFile)
	if err := os.Remove(postureFile); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to delete posture file: %w", err)
	}
	return nil
}

func (pm *ProcessManager) ReadPosture() (string, error) {
	return pm.readPosture()
}

func (pm *ProcessManager) getOperatorBinary() (string, error) {
	exePath, err := os.Executable()
	if err == nil {
		return exePath, nil
	}
	return "./g8e", nil
}

func (pm *ProcessManager) StartOperator(opts OperatorStartOptions) error {
	if err := pm.ensureDirectories(); err != nil {
		return err
	}

	// Use provided values or defaults
	effectiveHTTPPort := opts.HTTPPort
	effectiveHTTPSPort := opts.HTTPSPort
	effectiveDataDir := opts.DataDir
	effectivePKIDir := opts.PKIDir
	effectiveSecretsDir := opts.SecretsDir
	effectiveVaultDir := opts.VaultDir
	effectiveVaultKeyPath := opts.VaultKeyPath
	effectivePasskeyRpID := opts.PasskeyRpID
	effectivePasskeyRpName := opts.PasskeyRpName

	// Normalize 127.0.0.1 to localhost for passkey RP ID
	// WebAuthn requires RP ID to be a valid domain, not an IP address
	if effectivePasskeyRpID == "127.0.0.1" {
		effectivePasskeyRpID = "localhost"
	}
	effectiveRateLimitRPS := opts.RateLimitRPS
	effectiveRateLimitBurst := opts.RateLimitBurst
	effectiveLogLevel := opts.LogLevel

	// Use defaults if not provided
	if effectiveHTTPPort == 0 {
		effectiveHTTPPort = constants.Ports.OperatorHttp
	}
	if effectiveHTTPSPort == 0 {
		effectiveHTTPSPort = constants.Ports.OperatorHttps
	}
	if effectiveDataDir == "" {
		effectiveDataDir = pm.dataDir
	}
	if effectivePKIDir == "" {
		effectivePKIDir = pm.pkiDir
	}
	if effectiveSecretsDir == "" {
		effectiveSecretsDir = pm.secretsDir
	}
	if effectiveLogLevel == "" {
		effectiveLogLevel = "info"
	}

	// Find the first available port starting from httpPort
	availableHTTPPort, err := pm.findAvailablePort(effectiveHTTPPort, "Operator HTTP")
	if err != nil {
		return fmt.Errorf("failed to find available HTTP port: %w", err)
	}

	// Calculate offset from original httpPort to maintain port spacing
	offset := availableHTTPPort - effectiveHTTPPort
	availableHTTPSPort := effectiveHTTPSPort + offset

	// Verify the calculated HTTPS port is available
	if err := pm.checkPortAvailable(availableHTTPSPort, "Operator HTTPS"); err != nil {
		return fmt.Errorf("failed to verify HTTPS port %d: %w", availableHTTPSPort, err)
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

	args := []string{
		"--" + opts.Posture,
		"--working-dir", pm.projectRoot,
		"--data-dir", effectiveDataDir,
		"--pki-dir", effectivePKIDir,
		"--secrets-dir", effectiveSecretsDir,
		"--http-port", strconv.Itoa(availableHTTPPort),
		"--https-port", strconv.Itoa(availableHTTPSPort),
		"--log", effectiveLogLevel,
	}

	if effectiveVaultDir != "" {
		args = append(args, "--vault-dir", effectiveVaultDir)
	}
	if effectiveVaultKeyPath != "" {
		args = append(args, "--vault-key", effectiveVaultKeyPath)
	}
	if opts.VaultRequireUnlock {
		args = append(args, "--vault-require-unlock")
	}

	if opts.CertIdentityMode != "" {
		args = append(args, "--cert-mode", opts.CertIdentityMode)
	}

	if effectivePasskeyRpID != "" {
		args = append(args, "--passkey-rp-id", effectivePasskeyRpID)
	}
	if effectivePasskeyRpName != "" {
		args = append(args, "--passkey-rp-name", effectivePasskeyRpName)
	}
	if effectiveRateLimitRPS > 0 {
		args = append(args, "--rate-limit-rps", fmt.Sprintf("%.1f", effectiveRateLimitRPS))
	}
	if effectiveRateLimitBurst > 0 {
		args = append(args, "--rate-limit-burst", strconv.Itoa(effectiveRateLimitBurst))
	}

	identityArgs, err := pm.networkIdentityArgs(opts.IdentityData)
	if err != nil {
		return err
	}
	args = append(args, identityArgs...)

	cmd := exec.Command(binPath, args...)
	cmd.Stdout = logHandle
	cmd.Stderr = logHandle
	setProcessGroup(cmd)

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

	if err := pm.writePosture(opts.Posture); err != nil {
		_ = cmd.Process.Kill()
		_ = pm.deletePID(operatorPIDFile)
		if closeErr := logHandle.Close(); closeErr != nil {
			return fmt.Errorf("failed to write posture file: %w (additionally failed to close log file: %v)", err, closeErr)
		}
		return fmt.Errorf("failed to write posture file: %w", err)
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

	if pid != 0 && !pm.isProcessRunning(pid) {
		// Stale PID file - clean it up and fall through to process discovery
		_ = pm.deletePID(operatorPIDFile)
		pid = 0
	}

	if pid == 0 {
		// PID file missing or stale, try to find process via discovery
		if pm.findOperatorProcessFn != nil {
			pid = pm.findOperatorProcessFn()
		} else {
			pid = pm.findOperatorProcess()
		}
		if pid == 0 {
			return nil
		}
	}

	if err := pm.stopProcess(pid, "operator"); err != nil {
		return err
	}

	if err := pm.deletePID(operatorPIDFile); err != nil {
		return err
	}

	return pm.deletePosture()
}

func (pm *ProcessManager) OperatorStatus() (bool, int, error) {
	pid, err := pm.readPID(operatorPIDFile)
	if err != nil {
		return false, 0, err
	}

	if pid != 0 {
		if pm.isProcessRunning(pid) {
			return true, pid, nil
		}
		// Stale PID file - clean it up and fall through to process discovery
		_ = pm.deletePID(operatorPIDFile)
	}

	// PID file missing or stale, try to find the process
	if pm.findOperatorProcessFn != nil {
		pid = pm.findOperatorProcessFn()
	} else {
		pid = pm.findOperatorProcess()
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

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	defer signal.Stop(sigChan)

	// Use a goroutine to read lines and send to channel
	lineChan := make(chan string)
	errChan := make(chan error, 1)
	done := make(chan struct{})

	go func() {
		defer close(lineChan)
		reader := bufio.NewReader(file)
		for {
			select {
			case <-done:
				return
			default:
				line, err := reader.ReadString('\n')
				if err != nil {
					if err == io.EOF {
						time.Sleep(100 * time.Millisecond)
						continue
					}
					errChan <- fmt.Errorf("failed to read log line: %w", err)
					return
				}
				lineChan <- line
			}
		}
	}()

	// Track parent PID to detect if parent has died
	parentPID := os.Getppid()

	for {
		select {
		case <-sigChan:
			// Exit gracefully on interrupt signal
			close(done)
			return nil
		case err := <-errChan:
			close(done)
			return err
		case line := <-lineChan:
			fmt.Print(line)
		case <-time.After(100 * time.Millisecond):
			// Check if parent process is still alive
			// If parent died (PID changed to 1 or process doesn't exist), exit
			currentParentPID := os.Getppid()
			if currentParentPID != parentPID {
				// Parent died, we were orphaned and adopted by init (PID 1)
				close(done)
				return nil
			}
		}
	}
}
