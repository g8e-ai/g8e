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
	"github.com/g8e-ai/g8e/internal/paths"
)

const (
	ShutdownTimeout     = 10 * time.Second
	HealthCheckInterval = 500 * time.Millisecond
	MaxHealthChecks     = 20
	MaxPortAttempts     = 100
	SigKillPollInterval = 100 * time.Millisecond
)

// CommandExecutor defines an interface for executing external commands.
// It uses *exec.Cmd which is cross-platform.
type CommandExecutor interface {
	Command(name string, args ...string) *exec.Cmd
	Output(cmd *exec.Cmd) ([]byte, error)
	Run(cmd *exec.Cmd) error
}

// WindowsProcessChecker defines an interface for Windows-specific process checks.
// It uses uintptr for handles to remain cross-platform compatible.
type WindowsProcessChecker interface {
	OpenProcess(desiredAccess uint32, inheritHandle bool, processID uint32) (uintptr, error)
	CloseHandle(handle uintptr) error
	GetExitCodeProcess(handle uintptr, exitCode *uint32) error
}

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
	PasskeyRpOrigins   []string
	RateLimitRPS       float64
	RateLimitBurst     int
	LogLevel           string
	CertIdentityMode   string
	IdentityData       []byte
	TribunalID         string
	TribunalURL        string
	TribunalBootstrap  string
	MCPDownstreamURL   string
	A2ADownstreamURL   string
}

type ProcessManager struct {
	projectRoot string
	runtimeDir  string
	pkiDir      string
	secretsDir  string
	dataDir     string
	logDir      string
	pidDir      string
	logFile     string
	postureFile string
	// findOperatorProcessFn allows mocking for tests
	findOperatorProcessFn func() int
	// Windows-specific dependencies for testing (used in process_windows.go)
	//nolint:unused // Used in platform-specific files and tests
	windowsProcessChecker WindowsProcessChecker
	//nolint:unused // Used in platform-specific files and tests
	commandExecutor CommandExecutor
	// isProcessRunningFn allows mocking for tests (used in process_windows.go)
	//nolint:unused // Used in platform-specific files and tests
	isProcessRunningFn func(pid int) bool
}

func NewProcessManager(projectRoot string) (*ProcessManager, error) {
	// Initialize paths relative to projectRoot
	if err := paths.InitWithBase(projectRoot); err != nil {
		return nil, fmt.Errorf("%w: %v", constants.ErrDirCreateFailed, err)
	}

	runtimeDir := paths.Infra.RuntimeDir
	pkiDir := paths.Infra.PkiDir
	secretsDir := paths.Infra.SecretsDir
	dataDir := paths.Infra.DataDir
	logDir := paths.Infra.LogDir
	pidDir := paths.Infra.PidDir
	logFile := paths.Infra.OperatorLogFile
	postureFile := paths.Infra.OperatorPostureFile

	return &ProcessManager{
		projectRoot: projectRoot,
		runtimeDir:  runtimeDir,
		pkiDir:      pkiDir,
		secretsDir:  secretsDir,
		dataDir:     dataDir,
		logDir:      logDir,
		pidDir:      pidDir,
		logFile:     logFile,
		postureFile: postureFile,
	}, nil
}

func (pm *ProcessManager) createDirectories() error {
	dirs := []string{pm.runtimeDir, pm.pkiDir, pm.secretsDir, pm.dataDir, pm.logDir, pm.pidDir}
	for _, dir := range dirs {
		if err := os.MkdirAll(dir, constants.PermDirPrivate); err != nil {
			return fmt.Errorf("%w: %s: %v", constants.ErrDirCreateFailed, dir, err)
		}
	}
	return nil
}

func (pm *ProcessManager) networkIdentityArgs(identityData []byte) ([]string, error) {
	if len(identityData) == 0 {
		return nil, nil
	}

	identityFile, err := pm.WriteNetworkIdentityFile(identityData)
	if err != nil {
		return nil, err
	}

	return []string{"--network-identity-file", identityFile}, nil
}

func (pm *ProcessManager) WriteNetworkIdentityFile(identityData []byte) (string, error) {
	identityFile := filepath.Join(pm.runtimeDir, constants.NetworkIdentityFilename)
	if err := os.WriteFile(identityFile, identityData, constants.PermFilePrivate); err != nil {
		return "", fmt.Errorf("%w: %v", constants.ErrPathValidation, err)
	}
	return identityFile, nil
}

func (pm *ProcessManager) checkPortAvailable(port int, name string) error {
	addr := fmt.Sprintf("%s:%d", constants.LocalhostIP, port)
	listener, err := net.Listen(string(constants.NetworkProtocolTCP), addr)
	if err != nil {
		return fmt.Errorf("%w: port %d (%s): %v", constants.ErrPortUnavailable, port, name, err)
	}
	listener.Close()
	return nil
}

func (pm *ProcessManager) findAvailablePort(startPort int, name string) (int, error) {
	pid, err := pm.readPID(constants.OperatorPIDFilename)
	if err != nil {
		return 0, fmt.Errorf("%w: %v", constants.ErrPIDReadFailed, err)
	}

	if pid != 0 && !pm.isProcessRunning(pid) {
		if err := pm.deletePID(constants.OperatorPIDFilename); err != nil {
			return 0, fmt.Errorf("%w: pid %d: %v", constants.ErrPathValidation, pid, err)
		}
	}

	for attempt := 0; attempt < MaxPortAttempts; attempt++ {
		port := startPort + attempt
		addr := fmt.Sprintf("%s:%d", constants.LocalhostIP, port)
		listener, err := net.Listen(string(constants.NetworkProtocolTCP), addr)
		if err == nil {
			listener.Close()
			return port, nil
		}

		conflictingPID := pm.findProcessOnPort(port)
		if conflictingPID > 0 && conflictingPID == pid {
			return 0, fmt.Errorf("%w: port %d (%s) by process %d", constants.ErrPortUnavailable, port, name, conflictingPID)
		}
	}

	return 0, fmt.Errorf("%w: starting from %d after %d attempts", constants.ErrPortUnavailable, startPort, MaxPortAttempts)
}

func (pm *ProcessManager) readPID(filename string) (int, error) {
	pidFile := filepath.Join(pm.pidDir, filename)
	pidData, err := os.ReadFile(pidFile)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}
		return 0, fmt.Errorf("%w: %v", constants.ErrPIDReadFailed, err)
	}

	var pid int
	if _, err := fmt.Sscanf(string(pidData), "%d", &pid); err != nil {
		return 0, fmt.Errorf("%w: %v", constants.ErrPIDReadFailed, err)
	}

	return pid, nil
}

func (pm *ProcessManager) writePID(filename string, pid int) error {
	pidFile := filepath.Join(pm.pidDir, filename)
	return os.WriteFile(pidFile, []byte(strconv.Itoa(pid)), constants.PermFilePrivate)
}

func (pm *ProcessManager) deletePID(filename string) error {
	pidFile := filepath.Join(pm.pidDir, filename)
	if err := os.Remove(pidFile); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("%w: %v", constants.ErrPathValidation, err)
	}
	return nil
}

func (pm *ProcessManager) writePosture(posture string) error {
	return os.WriteFile(pm.postureFile, []byte(posture), constants.PermFilePrivate)
}

func (pm *ProcessManager) readPosture() (string, error) {
	postureData, err := os.ReadFile(pm.postureFile)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", fmt.Errorf("%w: %v", constants.ErrPostureReadFailed, err)
	}
	posture := string(postureData)
	// Validate posture is one of the allowed values
	if posture != "" && posture != constants.PostureDoctrine && posture != constants.PostureConsensus && posture != constants.PostureNotary {
		return "", fmt.Errorf("%w: invalid value '%s': must be %s, %s, or %s", constants.ErrInvalidPosture, posture, constants.PostureDoctrine, constants.PostureConsensus, constants.PostureNotary)
	}
	return posture, nil
}

func (pm *ProcessManager) deletePosture() error {
	if err := os.Remove(pm.postureFile); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("%w: %v", constants.ErrPathValidation, err)
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
	return constants.LocalBinaryName, nil
}

func (pm *ProcessManager) BuildReExecArgs(opts OperatorStartOptions) ([]string, error) {
	args := []string{
		"gateway", "serve",
		"--posture", opts.Posture,
		"--data-dir", opts.DataDir,
		"--pki-dir", opts.PKIDir,
		"--secrets-dir", opts.SecretsDir,
		"--http-port", strconv.Itoa(opts.HTTPPort),
		"--https-port", strconv.Itoa(opts.HTTPSPort),
		"--log", opts.LogLevel,
	}

	if opts.VaultDir != "" {
		args = append(args, "--vault-dir", opts.VaultDir)
	}
	if opts.VaultKeyPath != "" {
		args = append(args, "--vault-key", opts.VaultKeyPath)
	}
	if opts.VaultRequireUnlock {
		args = append(args, "--vault-require-unlock")
	}

	if opts.CertIdentityMode != "" {
		args = append(args, "--cert-mode", opts.CertIdentityMode)
	}
	if opts.TribunalID != "" {
		args = append(args, "--tribunal-id", opts.TribunalID)
	}
	if opts.TribunalURL != "" {
		args = append(args, "--tribunal-url", opts.TribunalURL)
	}
	if opts.TribunalBootstrap != "" {
		args = append(args, "--tribunal-bootstrap", opts.TribunalBootstrap)
	}
	if opts.MCPDownstreamURL != "" {
		args = append(args, "--mcp-downstream-url", opts.MCPDownstreamURL)
	}
	if opts.A2ADownstreamURL != "" {
		args = append(args, "--a2a-downstream-url", opts.A2ADownstreamURL)
	}

	if opts.PasskeyRpID != "" {
		args = append(args, "--passkey-rp-id", opts.PasskeyRpID)
	}
	if opts.PasskeyRpName != "" {
		args = append(args, "--passkey-rp-name", opts.PasskeyRpName)
	}
	for _, origin := range opts.PasskeyRpOrigins {
		args = append(args, "--passkey-rp-origin", origin)
	}
	if opts.RateLimitRPS > 0 {
		args = append(args, "--rate-limit-rps", fmt.Sprintf("%.1f", opts.RateLimitRPS))
	}
	if opts.RateLimitBurst > 0 {
		args = append(args, "--rate-limit-burst", strconv.Itoa(opts.RateLimitBurst))
	}

	identityArgs, err := pm.networkIdentityArgs(opts.IdentityData)
	if err != nil {
		return nil, err
	}
	args = append(args, identityArgs...)

	return args, nil
}

func (pm *ProcessManager) StartOperator(opts OperatorStartOptions) error {
	if err := pm.createDirectories(); err != nil {
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
	if effectivePasskeyRpID == constants.LocalhostIP {
		effectivePasskeyRpID = constants.LocalhostHostname
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
		effectiveLogLevel = constants.LogLevelDefault
	}

	// Find the first available port starting from httpPort
	availableHTTPPort, err := pm.findAvailablePort(effectiveHTTPPort, "Operator HTTP")
	if err != nil {
		return fmt.Errorf("%w: HTTP port: %v", constants.ErrPortUnavailable, err)
	}

	// Calculate offset from original httpPort to maintain port spacing
	offset := availableHTTPPort - effectiveHTTPPort
	availableHTTPSPort := effectiveHTTPSPort + offset

	// Verify the calculated HTTPS port is available
	if err := pm.checkPortAvailable(availableHTTPSPort, "Operator HTTPS"); err != nil {
		return fmt.Errorf("%w: HTTPS port %d: %v", constants.ErrPortUnavailable, availableHTTPSPort, err)
	}

	binPath, err := pm.getOperatorBinary()
	if err != nil {
		return err
	}

	logHandle, err := os.OpenFile(pm.logFile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, constants.PermFilePrivate)
	if err != nil {
		return fmt.Errorf("%w: %v", constants.ErrPathValidation, err)
	}

	opts.HTTPPort = availableHTTPPort
	opts.HTTPSPort = availableHTTPSPort
	opts.DataDir = effectiveDataDir
	opts.PKIDir = effectivePKIDir
	opts.SecretsDir = effectiveSecretsDir
	opts.VaultDir = effectiveVaultDir
	opts.VaultKeyPath = effectiveVaultKeyPath
	opts.PasskeyRpID = effectivePasskeyRpID
	opts.PasskeyRpName = effectivePasskeyRpName
	opts.RateLimitRPS = effectiveRateLimitRPS
	opts.RateLimitBurst = effectiveRateLimitBurst
	opts.LogLevel = effectiveLogLevel

	args, err := pm.BuildReExecArgs(opts)
	if err != nil {
		if closeErr := logHandle.Close(); closeErr != nil {
			return fmt.Errorf("%w: %v (additionally failed to close log file: %v)", constants.ErrPathValidation, err, closeErr)
		}
		return err
	}

	cmd := exec.Command(binPath, args...)
	cmd.Stdout = logHandle
	cmd.Stderr = logHandle
	setProcessGroup(cmd)

	if err := cmd.Start(); err != nil {
		if closeErr := logHandle.Close(); closeErr != nil {
			return fmt.Errorf("%w: %v (additionally failed to close log file: %v)", constants.ErrProcessStartFailed, err, closeErr)
		}
		return fmt.Errorf("%w: %v", constants.ErrProcessStartFailed, err)
	}

	if err := pm.writePID(constants.OperatorPIDFilename, cmd.Process.Pid); err != nil {
		_ = cmd.Process.Kill()
		if closeErr := logHandle.Close(); closeErr != nil {
			return fmt.Errorf("%w: %v (additionally failed to close log file: %v)", constants.ErrPIDWriteFailed, err, closeErr)
		}
		return fmt.Errorf("%w: %v", constants.ErrPIDWriteFailed, err)
	}

	if err := pm.writePosture(opts.Posture); err != nil {
		_ = cmd.Process.Kill()
		_ = pm.deletePID(constants.OperatorPIDFilename)
		if closeErr := logHandle.Close(); closeErr != nil {
			return fmt.Errorf("%w: %v (additionally failed to close log file: %v)", constants.ErrPostureWriteFailed, err, closeErr)
		}
		return fmt.Errorf("%w: %v", constants.ErrPostureWriteFailed, err)
	}

	if err := logHandle.Close(); err != nil {
		return fmt.Errorf("%w: %v", constants.ErrPathValidation, err)
	}

	time.Sleep(2 * time.Second)
	if !pm.isProcessRunning(cmd.Process.Pid) {
		_ = pm.deletePID(constants.OperatorPIDFilename)
		return fmt.Errorf("%w: check %s", constants.ErrProcessStartFailed, pm.logFile)
	}

	return nil
}

func (pm *ProcessManager) StopOperator() error {
	pid, err := pm.readPID(constants.OperatorPIDFilename)
	if err != nil {
		return err
	}

	if pid != 0 && !pm.isProcessRunning(pid) {
		// Stale PID file - clean it up and fall through to process discovery
		_ = pm.deletePID(constants.OperatorPIDFilename)
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
		return fmt.Errorf("%w: %v", constants.ErrProcessStopFailed, err)
	}

	if err := pm.deletePID(constants.OperatorPIDFilename); err != nil {
		return err
	}

	return pm.deletePosture()
}

func (pm *ProcessManager) OperatorStatus() (bool, int, error) {
	pid, err := pm.readPID(constants.OperatorPIDFilename)
	if err != nil {
		return false, 0, err
	}

	if pid != 0 {
		if pm.isProcessRunning(pid) {
			return true, pid, nil
		}
		// Stale PID file - clean it up and fall through to process discovery
		_ = pm.deletePID(constants.OperatorPIDFilename)
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
	return pm.logFile
}

func (pm *ProcessManager) Reset() error {
	if err := pm.StopOperator(); err != nil {
		return fmt.Errorf("%w: %v", constants.ErrProcessStopFailed, err)
	}

	if err := os.RemoveAll(pm.dataDir); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("%w: data directory: %v", constants.ErrPathValidation, err)
	}

	if err := os.RemoveAll(pm.secretsDir); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("%w: secrets directory: %v", constants.ErrPathValidation, err)
	}

	if err := pm.createDirectories(); err != nil {
		return fmt.Errorf("%w: %v", constants.ErrDirCreateFailed, err)
	}

	return nil
}

func (pm *ProcessManager) Clean() error {
	if err := pm.StopOperator(); err != nil {
		return fmt.Errorf("%w: %v", constants.ErrProcessStopFailed, err)
	}

	if err := os.RemoveAll(pm.runtimeDir); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("%w: runtime directory: %v", constants.ErrPathValidation, err)
	}

	return nil
}

// TailLog prints a log file, optionally following new entries (like tail -f)
func TailLog(logPath string, follow bool) error {
	file, err := os.Open(logPath)
	if err != nil {
		return fmt.Errorf("%w: %v", constants.ErrPathValidation, err)
	}
	defer file.Close()

	// Print existing content
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return fmt.Errorf("%w: seek to start: %v", constants.ErrDirectoryRead, err)
	}
	if _, err := io.Copy(os.Stdout, file); err != nil {
		return fmt.Errorf("%w: print content: %v", constants.ErrDirectoryRead, err)
	}

	if !follow {
		return nil
	}

	// Follow mode: seek to end and watch for new content
	if _, err := file.Seek(0, io.SeekEnd); err != nil {
		return fmt.Errorf("%w: seek to end: %v", constants.ErrDirectoryRead, err)
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
					errChan <- fmt.Errorf("%w: read line: %v", constants.ErrDirectoryRead, err)
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
