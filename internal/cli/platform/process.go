// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package platform

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"strconv"
	"syscall"
	"time"

	"github.com/g8e-ai/g8e/internal/cli/serve"
	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/g8e-ai/g8e/internal/services/fs"
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
// It embeds serve.GatewayConfig so that all gateway fields are shared in a
// single struct definition, eliminating the previous triple-duplication.
type OperatorStartOptions struct {
	serve.GatewayConfig
}

type ProcessManager struct {
	fileSvc fs.RuntimeFileService
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

func NewProcessManager(fileSvc fs.RuntimeFileService) (*ProcessManager, error) {
	if fileSvc == nil {
		return nil, fmt.Errorf("%w: fileSvc is required", constants.ErrPathValidation)
	}

	return &ProcessManager{
		fileSvc: fileSvc,
	}, nil
}

func (pm *ProcessManager) CreateDirectories() error {
	return pm.fileSvc.CreateRuntimeTree(context.Background())
}

func (pm *ProcessManager) WriteNetworkIdentityFile(identityData []byte) (string, error) {
	relPath := constants.NetworkIdentityFilename
	if err := pm.fileSvc.WriteFile(context.Background(), relPath, identityData, constants.PermFilePrivate); err != nil {
		return "", fmt.Errorf("%w: %w", constants.ErrFileWriteFailed, err)
	}
	return pm.fileSvc.Resolve(relPath), nil
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
	relPath := filepath.Join(constants.PidDirname, filename)
	pidData, err := pm.fileSvc.ReadFile(context.Background(), relPath)
	if err != nil {
		if errors.Is(err, constants.ErrNotFound) {
			return 0, nil
		}
		return 0, fmt.Errorf("%w: %w", constants.ErrPIDReadFailed, err)
	}

	var pid int
	if _, err := fmt.Sscanf(string(pidData), "%d", &pid); err != nil {
		return 0, fmt.Errorf("%w: %w", constants.ErrPIDReadFailed, err)
	}

	return pid, nil
}

func (pm *ProcessManager) writePID(filename string, pid int) error {
	relPath := filepath.Join(constants.PidDirname, filename)
	return pm.fileSvc.WriteFile(context.Background(), relPath, []byte(strconv.Itoa(pid)), constants.PermFilePrivate)
}

func (pm *ProcessManager) deletePID(filename string) error {
	relPath := filepath.Join(constants.PidDirname, filename)
	return pm.fileSvc.Remove(context.Background(), relPath)
}

func (pm *ProcessManager) writePosture(posture string) error {
	relPath := filepath.Join(constants.PidDirname, constants.OperatorPostureFilename)
	return pm.fileSvc.WriteFile(context.Background(), relPath, []byte(posture), constants.PermFilePrivate)
}

func (pm *ProcessManager) readPosture() (string, error) {
	relPath := filepath.Join(constants.PidDirname, constants.OperatorPostureFilename)
	postureData, err := pm.fileSvc.ReadFile(context.Background(), relPath)
	if err != nil {
		if errors.Is(err, constants.ErrNotFound) {
			return "", nil
		}
		return "", fmt.Errorf("%w: %w", constants.ErrPostureReadFailed, err)
	}
	posture := string(postureData)
	// Validate posture is one of the allowed values
	if posture != "" && posture != constants.PostureDoctrine && posture != constants.PostureConsensus && posture != constants.PostureNotary {
		return "", fmt.Errorf("%w: invalid value '%s': must be %s, %s, or %s", constants.ErrInvalidPosture, posture, constants.PostureDoctrine, constants.PostureConsensus, constants.PostureNotary)
	}
	return posture, nil
}

func (pm *ProcessManager) deletePosture() error {
	relPath := filepath.Join(constants.PidDirname, constants.OperatorPostureFilename)
	return pm.fileSvc.Remove(context.Background(), relPath)
}

func (pm *ProcessManager) ReadPosture() (string, error) {
	return pm.readPosture()
}

// operatorBinaryName returns the canonical filename for the copied operator
// binary in .g8e/bin, accounting for the platform-specific extension.
func operatorBinaryName() string {
	if runtime.GOOS == "windows" {
		return constants.BinaryImageNameWindows
	}
	return constants.BinaryImageName
}

// copyBinaryToBinDir copies the currently running executable into .g8e/bin
// and returns the absolute path to the copy. This gives the re-executed
// gateway process a stable binary location that survives rebuilds or moves
// of the original executable.
//
// If the destination already matches the source (same size and modtime), the
// copy is skipped.
func (pm *ProcessManager) copyBinaryToBinDir() (string, error) {
	exePath, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("%w: %w", constants.ErrBinaryResolveFailed, err)
	}

	srcInfo, err := os.Stat(exePath)
	if err != nil {
		return "", fmt.Errorf("%w: %w", constants.ErrBinaryResolveFailed, err)
	}

	relPath := filepath.Join(constants.BinDirname, operatorBinaryName())
	destPath := pm.fileSvc.Resolve(relPath)

	// Skip copy if destination already matches source.
	if destInfo, statErr := os.Stat(destPath); statErr == nil {
		if destInfo.Size() == srcInfo.Size() && destInfo.ModTime().Equal(srcInfo.ModTime()) {
			return destPath, nil
		}
	}

	if err := pm.fileSvc.MkdirAll(context.Background(), constants.BinDirname, constants.PermDirStandard); err != nil {
		return "", fmt.Errorf("%w: %w", constants.ErrDirCreateFailed, err)
	}

	src, err := os.Open(exePath)
	if err != nil {
		return "", fmt.Errorf("%w: %w", constants.ErrBinaryCopyFailed, err)
	}
	defer src.Close()

	dest, err := os.OpenFile(destPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, constants.PermFileExecutable)
	if err != nil {
		return "", fmt.Errorf("%w: %w", constants.ErrBinaryCopyFailed, err)
	}

	if _, err := io.Copy(dest, src); err != nil {
		_ = dest.Close()
		return "", fmt.Errorf("%w: %w", constants.ErrBinaryCopyFailed, err)
	}

	if err := dest.Close(); err != nil {
		return "", fmt.Errorf("%w: %w", constants.ErrBinaryCopyFailed, err)
	}

	// Preserve modtime so the skip-if-same check works on subsequent starts.
	if err := os.Chtimes(destPath, srcInfo.ModTime(), srcInfo.ModTime()); err != nil {
		return "", fmt.Errorf("%w: %w", constants.ErrBinaryCopyFailed, err)
	}

	return destPath, nil
}

func (pm *ProcessManager) BuildReExecArgs(opts OperatorStartOptions) ([]string, error) {
	args := []string{
		"gw", "start", "--follow",
		"--posture", string(opts.Posture),
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

	if opts.CertIdentityMode != "" {
		args = append(args, "--cert-mode", opts.CertIdentityMode)
	}
	if opts.ConsensusID != "" {
		args = append(args, "--consensus-id", opts.ConsensusID)
	}
	if opts.ConsensusURL != "" {
		args = append(args, "--consensus-url", opts.ConsensusURL)
	}
	if opts.ConsensusBootstrap != "" {
		args = append(args, "--consensus-bootstrap", opts.ConsensusBootstrap)
	}
	if opts.MCPDownstreamURL != "" {
		args = append(args, "--mcp-downstream-url", opts.MCPDownstreamURL)
	}
	if opts.A2ADownstreamURL != "" {
		args = append(args, "--a2a-downstream-url", opts.A2ADownstreamURL)
	}
	if opts.PublicBaseURL != "" {
		args = append(args, "--public-base-url", opts.PublicBaseURL)
	}
	for _, origin := range opts.AllowedOrigins {
		args = append(args, "--cors-origin", origin)
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
	if opts.DoctrineDir != "" {
		args = append(args, "--doctrine-dir", opts.DoctrineDir)
	}

	return args, nil
}

func (pm *ProcessManager) StartOperator(opts OperatorStartOptions) error {
	if err := pm.CreateDirectories(); err != nil {
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
		effectiveDataDir = pm.fileSvc.Resolve(constants.DataDirname)
	}
	if effectivePKIDir == "" {
		effectivePKIDir = pm.fileSvc.Resolve(constants.PkiDirname)
	}
	if effectiveSecretsDir == "" {
		effectiveSecretsDir = pm.fileSvc.Resolve(constants.SecretsDirname)
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

	binPath, err := pm.copyBinaryToBinDir()
	if err != nil {
		return err
	}

	logPath := pm.fileSvc.Resolve(filepath.Join(constants.LogDirname, constants.OperatorLogFilename))
	logHandle, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, constants.PermFilePrivate)
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

	if err := pm.writePosture(string(opts.Posture)); err != nil {
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

	healthURL := fmt.Sprintf("http://%s:%d%s", constants.LocalhostIP, availableHTTPPort, constants.APIPaths.Health)
	client := &http.Client{Timeout: HealthCheckInterval}
	for i := 0; i < MaxHealthChecks; i++ {
		if !pm.isProcessRunning(cmd.Process.Pid) {
			_ = pm.deletePID(constants.OperatorPIDFilename)
			return fmt.Errorf("%w: check %s", constants.ErrProcessStartFailed, logPath)
		}
		resp, err := client.Get(healthURL)
		if err == nil {
			_ = resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return nil
			}
		}
		time.Sleep(HealthCheckInterval)
	}

	_ = pm.deletePID(constants.OperatorPIDFilename)
	return fmt.Errorf("%w: gateway did not become healthy, check %s", constants.ErrProcessStartFailed, logPath)
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
	return pm.fileSvc.Resolve(filepath.Join(constants.LogDirname, constants.OperatorLogFilename))
}

func (pm *ProcessManager) Clean() error {
	if err := pm.StopOperator(); err != nil {
		return fmt.Errorf("%w: %v", constants.ErrProcessStopFailed, err)
	}

	if err := pm.fileSvc.RemoveAll(context.Background(), ""); err != nil {
		return fmt.Errorf("%w: runtime directory: %w", constants.ErrPathValidation, err)
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
