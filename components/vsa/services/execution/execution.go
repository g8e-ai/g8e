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

package execution

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/g8e-ai/g8e/components/vsa/config"
	"github.com/g8e-ai/g8e/components/vsa/constants"
	"github.com/g8e-ai/g8e/components/vsa/models"
	system "github.com/g8e-ai/g8e/components/vsa/services/system"
)

// ExecutionService handles command execution with security controls.
// The VSA Operator should be run with sudo/root privileges for full system access.
// Commands are executed directly without sandboxing or user isolation.
type ExecutionService struct {
	config *config.Config
	logger *slog.Logger

	// Configuration from bootstrap
	maxConcurrentTasks int
	maxMemoryMB        int

	// Active executions tracking
	activeExecutions map[string]*ExecutionContext
	executionsMutex  sync.RWMutex
	semaphore        chan struct{} // Concurrency control
}

func (es *ExecutionService) BuildCommandString(command string, args []string) string {
	if len(args) == 0 {
		return command
	}
	return fmt.Sprintf("%s %s", command, strings.Join(args, " "))
}

// ExecutionContext tracks an active command execution
type ExecutionContext struct {
	Request   *models.ExecutionRequestPayload
	StartTime time.Time
	Cancel    context.CancelFunc
	Process   *os.Process
	Result    *models.ExecutionResultsPayload
	mu        sync.Mutex // Protects Result field from concurrent access
}

// streamingWriter wraps an io.Writer and logs each line as it's written.
// It enforces a maximum buffer size to prevent OOM from massive command output.
type streamingWriter struct {
	buffer      *bytes.Buffer
	lineBuffer  bytes.Buffer
	logger      *slog.Logger
	prefix      string
	executionID string
	mu          sync.Mutex
	maxSize     int
	truncated   bool
}

// newStreamingWriter creates a new streaming writer for real-time output logging
func newStreamingWriter(buffer *bytes.Buffer, logger *slog.Logger, prefix string, executionID string, maxSize int) *streamingWriter {
	return &streamingWriter{
		buffer:      buffer,
		logger:      logger,
		prefix:      prefix,
		executionID: executionID,
		maxSize:     maxSize,
	}
}

// Write implements io.Writer and logs each complete line
func (sw *streamingWriter) Write(p []byte) (n int, err error) {
	sw.mu.Lock()
	defer sw.mu.Unlock()

	n = len(p)

	// If already truncated, just log and return
	if sw.truncated {
		sw.logLines(p)
		return n, nil
	}

	// Check if this write would exceed maxSize
	if sw.maxSize > 0 && sw.buffer.Len()+len(p) > sw.maxSize {
		remaining := sw.maxSize - sw.buffer.Len()
		if remaining > 0 {
			sw.buffer.Write(p[:remaining])
		}
		sw.truncated = true
		sw.buffer.WriteString("\n... [OUTPUT TRUNCATED BY OPERATOR FOR SAFETY] ...\n")
		sw.logger.Warn("Command output truncated - limit reached", "prefix", sw.prefix, "execution_id", sw.executionID, "limit", sw.maxSize)
		sw.logLines(p)
		return n, nil
	}

	// Write to buffer for final result
	_, err = sw.buffer.Write(p)
	if err != nil {
		return 0, err
	}

	// Process the data line by line for real-time logging
	sw.logLines(p)

	return n, nil
}

func (sw *streamingWriter) logLines(p []byte) {
	sw.lineBuffer.Write(p)

	for {
		line, err := sw.lineBuffer.ReadString('\n')
		if err != nil {
			// No complete line yet, put the partial line back
			sw.lineBuffer.WriteString(line)
			break
		}

		// Trim the newline and log the complete line
		line = strings.TrimRight(line, "\n\r")
		if line != "" {
			sw.logger.Info(line, "execution_id", sw.executionID, "stream", sw.prefix)
		}
	}
}

// Flush logs any remaining partial line
func (sw *streamingWriter) Flush() {
	sw.mu.Lock()
	defer sw.mu.Unlock()

	if sw.lineBuffer.Len() > 0 {
		line := strings.TrimSpace(sw.lineBuffer.String())
		if line != "" {
			sw.logger.Info(line, "execution_id", sw.executionID, "stream", sw.prefix)
		}
		sw.lineBuffer.Reset()
	}
}

// NewExecutionService creates a new execution service
func NewExecutionService(cfg *config.Config, logger *slog.Logger) *ExecutionService {
	es := &ExecutionService{
		config:             cfg,
		logger:             logger,
		maxConcurrentTasks: cfg.MaxConcurrentTasks,
		maxMemoryMB:        cfg.MaxMemoryMB,
		activeExecutions:   make(map[string]*ExecutionContext),
		semaphore:          make(chan struct{}, cfg.MaxConcurrentTasks),
	}

	es.logger.Info("Execution service initialized", "max_concurrent_tasks", es.maxConcurrentTasks)

	return es
}

// cloudCLICommands lists commands that require --cloud flag to execute
var cloudCLICommands = map[string]bool{
	// Cloud provider CLIs
	constants.Status.CloudSubtype.AWS: true,
	"gcloud":                          true,
	"az":                              true,
	"gsutil":                          true,
	"bq":                              true, // BigQuery CLI (part of gcloud)
	"cbt":                             true, // Cloud Bigtable CLI
	"azcopy":                          true, // Azure storage copy tool
	// Infrastructure as Code tools
	"terraform": true,
	"kubectl":   true,
	"helm":      true,
	"pulumi":    true,
	"ansible":   true,
	"eksctl":    true, // EKS CLI
	"sam":       true, // AWS SAM CLI
	"cdk":       true, // AWS CDK CLI
}

// isCloudCLICommand checks if a command or its arguments invoke cloud CLI tools
func isCloudCLICommand(command string, args []string) (bool, string) {
	// Check base command
	baseCmd := command
	if idx := strings.LastIndex(command, "/"); idx >= 0 {
		baseCmd = command[idx+1:]
	}
	if cloudCLICommands[baseCmd] {
		return true, baseCmd
	}

	// Check if cloud CLI is invoked via shell (e.g., sh -c "aws s3 ls")
	if (command == "sh" || command == "/bin/sh" || command == "bash" || command == "/bin/bash") && len(args) >= 2 && args[0] == "-c" {
		shellCmd := strings.Join(args[1:], " ")
		for cloudCmd := range cloudCLICommands {
			// Check if cloud command appears at start or after common prefixes
			if strings.HasPrefix(shellCmd, cloudCmd+" ") ||
				strings.HasPrefix(shellCmd, cloudCmd+"\t") ||
				strings.Contains(shellCmd, " "+cloudCmd+" ") ||
				strings.Contains(shellCmd, ";"+cloudCmd+" ") ||
				strings.Contains(shellCmd, "|"+cloudCmd+" ") ||
				strings.Contains(shellCmd, "&&"+cloudCmd+" ") ||
				strings.Contains(shellCmd, "|| "+cloudCmd+" ") {
				return true, cloudCmd
			}
		}
	}

	return false, ""
}

// ExecuteCommand executes a command with security controls and resource limits
func (es *ExecutionService) ExecuteCommand(ctx context.Context, request *models.ExecutionRequestPayload) (*models.ExecutionResultsPayload, error) {
	es.logger.Info("Executing command",
		"execution_id", request.ExecutionID,
		"case_id", request.CaseID,
		"command", request.Command,
		"args", request.Args)

	// SECURITY: Block cloud CLI commands unless --cloud flag is set
	if !es.config.CloudMode {
		if isCloud, cloudCmd := isCloudCLICommand(request.Command, request.Args); isCloud {
			es.logger.Warn("Cloud CLI command blocked - Operator not started with --cloud flag",
				"execution_id", request.ExecutionID,
				"command", request.Command,
				"blocked_tool", cloudCmd,
				"cloud_mode", es.config.CloudMode)

			now := time.Now().UTC()
			return &models.ExecutionResultsPayload{
				ExecutionID:     request.ExecutionID,
				CaseID:          request.CaseID,
				TaskID:          request.TaskID,
				InvestigationID: request.InvestigationID,
				Command:         request.Command,
				Args:            request.Args,
				Status:          constants.ExecutionStatusFailed,
				StartTime:       &now,
				ReturnCode:      system.IntPtr(126), // Command invoked cannot execute
				Stdout:          "",
				Stderr:          fmt.Sprintf("Cloud CLI command '%s' is not available. This Operator was not started with --cloud flag.", cloudCmd),
				ErrorMessage:    system.StringPtr(fmt.Sprintf("cloud CLI '%s' blocked: Operator requires --cloud flag", cloudCmd)),
				ErrorType:       system.StringPtr("cloud_cli_blocked"),
				DurationSeconds: 0,
			}, nil
		}
	}

	// Log execution context
	es.logger.Info("Execution context details",
		"timeout_seconds", request.TimeoutSeconds,
		"working_dir", system.StringPtrValue(request.WorkingDirectory))

	// Acquire semaphore for concurrency control
	select {
	case es.semaphore <- struct{}{}:
		defer func() { <-es.semaphore }()
	case <-ctx.Done():
		return nil, fmt.Errorf("execution cancelled while waiting for available slot")
	}

	// Create execution context
	execCtx := &ExecutionContext{
		Request:   request,
		StartTime: time.Now().UTC(),
	}

	// Initialize result
	result := &models.ExecutionResultsPayload{
		ExecutionID:     request.ExecutionID,
		CaseID:          request.CaseID,
		TaskID:          request.TaskID,
		InvestigationID: request.InvestigationID,
		Command:         request.Command,
		Args:            request.Args,
		Status:          constants.ExecutionStatusExecuting,
		StartTime:       &execCtx.StartTime,
	}

	execCtx.Result = result

	// Track active execution
	es.executionsMutex.Lock()
	es.activeExecutions[request.ExecutionID] = execCtx
	es.executionsMutex.Unlock()

	// Cleanup function
	defer func() {
		es.executionsMutex.Lock()
		delete(es.activeExecutions, request.ExecutionID)
		es.executionsMutex.Unlock()
	}()

	// Create timeout context - use exactly what was requested
	timeoutDuration := time.Duration(request.TimeoutSeconds) * time.Second
	cmdCtx, cancel := context.WithTimeout(ctx, timeoutDuration)
	defer cancel()
	execCtx.mu.Lock()
	execCtx.Cancel = cancel
	execCtx.mu.Unlock()

	// Execute the command
	err := es.executeCommandInternal(cmdCtx, execCtx)
	if err != nil {
		execCtx.mu.Lock()
		if result.Status == constants.ExecutionStatusExecuting {
			result.Status = constants.ExecutionStatusFailed
			result.ErrorMessage = system.StringPtr(err.Error())
			result.ErrorType = system.StringPtr("execution_error")
		}
		execCtx.mu.Unlock()
	}

	// Finalize result
	es.finalizeResult(result)

	// Create output preview for logging
	stdoutPreview := result.Stdout
	if len(stdoutPreview) > 300 {
		stdoutPreview = stdoutPreview[:300] + "..."
	}
	stderrPreview := result.Stderr
	if len(stderrPreview) > 300 {
		stderrPreview = stderrPreview[:300] + "..."
	}

	es.logger.Info("Command execution completed",
		"execution_id", request.ExecutionID,
		"case_id", request.CaseID,
		"command", request.Command,
		"status", result.Status,
		"duration_seconds", result.DurationSeconds,
		"return_code", system.IntPtrValue(result.ReturnCode),
		"stdout_preview", stdoutPreview,
		"stderr_preview", stderrPreview)

	return result, nil
}

// executeCommandInternal performs the actual command execution
func (es *ExecutionService) executeCommandInternal(ctx context.Context, execCtx *ExecutionContext) error {
	request := execCtx.Request
	result := execCtx.Result

	// Build the full command string for shell execution
	var fullCommand string

	// Check if command is already a shell with -c flag to avoid double-wrapping
	// e.g., Command="sh", Args=["-c", "echo $VAR"] should become just "echo $VAR"
	// not "sh -c echo $VAR" which would then be wrapped again as /bin/sh -c "sh -c echo $VAR"
	isShellCommand := request.Command == "sh" || request.Command == "/bin/sh" ||
		request.Command == "bash" || request.Command == "/bin/bash"

	if isShellCommand && len(request.Args) >= 2 && request.Args[0] == "-c" {
		// Extract the script directly - it's already meant for shell execution
		// Join remaining args in case the script was split across multiple args
		fullCommand = strings.Join(request.Args[1:], " ")
	} else if len(request.Args) > 0 {
		// Command and args provided separately - combine them
		fullCommand = request.Command + " " + strings.Join(request.Args, " ")
	} else {
		fullCommand = request.Command
	}

	if strings.TrimSpace(fullCommand) == "" {
		return fmt.Errorf("empty command")
	}

	// ALWAYS use shell execution for reliable behavior
	// This ensures:
	// - Variable expansion ($HOME, $USER, etc.)
	// - Tilde expansion (~)
	// - Glob patterns (*.txt)
	// - Pipes, redirects, and all shell features
	// - Consistent behavior matching what users expect from a terminal

	// Apply memory limit via ulimit if configured
	shellCommand := fullCommand
	if es.maxMemoryMB > 0 {
		// ulimit -v is virtual memory limit in KB
		limitKB := es.maxMemoryMB * 1024
		shellCommand = fmt.Sprintf("ulimit -v %d; %s", limitKB, fullCommand)
		es.logger.Info("Applying memory limit via ulimit", "limit_mb", es.maxMemoryMB)
	}

	es.logger.Info("Executing command via shell",
		"command", shellCommand,
		"execution_type", "shell")

	cmd := exec.CommandContext(ctx, "/bin/bash", "-c", shellCommand)

	// Update result with the full command
	execCtx.mu.Lock()
	result.Command = fullCommand
	result.Args = []string{}
	execCtx.mu.Unlock()

	// Set working directory: explicit request overrides, otherwise use operator's WorkDir.
	// WorkDir is the directory the operator was launched from (or --working-dir flag value).
	if request.WorkingDirectory != nil {
		cmd.Dir = *request.WorkingDirectory
	} else {
		cmd.Dir = es.config.WorkDir
	}

	// Set environment variables - ensure PATH is included
	cmd.Env = os.Environ()

	// Add common system paths if PATH is not set
	pathSet := false
	for _, env := range cmd.Env {
		if strings.HasPrefix(env, "PATH=") {
			pathSet = true
			break
		}
	}
	if !pathSet {
		cmd.Env = append(cmd.Env, "PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin")
	}

	// Force non-interactive mode for common tools to prevent hanging
	// These environment variables tell installers/tools to run without user prompts
	cmd.Env = append(cmd.Env,
		"DEBIAN_FRONTEND=noninteractive",          // Debian/Ubuntu package managers
		"APT_KEY_DONT_WARN_ON_DANGEROUS_USAGE=1",  // apt-key warnings
		"CLOUDSDK_CORE_DISABLE_PROMPTS=1",         // Google Cloud SDK
		"CLOUDSDK_INSTALL_DIR="+es.config.WorkDir, // Google Cloud SDK install location
		"CI=true",          // Generic CI flag (many tools respect this)
		"NONINTERACTIVE=1", // Generic non-interactive flag
	)

	// Add any additional environment variables from request
	for k, v := range request.Environment {
		cmd.Env = append(cmd.Env, fmt.Sprintf("%s=%s", k, v))
	}

	// Create buffers for stdout and stderr
	var stdoutBuf, stderrBuf bytes.Buffer

	// Default max output size is 10MB per stream to prevent OOM
	maxStreamSize := 10 * 1024 * 1024

	// Create streaming writers that log output in real-time
	stdoutWriter := newStreamingWriter(&stdoutBuf, es.logger, "stdout", request.ExecutionID, maxStreamSize)
	stderrWriter := newStreamingWriter(&stderrBuf, es.logger, "stderr", request.ExecutionID, maxStreamSize)

	cmd.Stdout = stdoutWriter
	cmd.Stderr = stderrWriter

	// CRITICAL: Close stdin to prevent commands from hanging waiting for input
	// Commands that attempt to read from stdin will get EOF and fail fast
	cmd.Stdin = nil

	// Set process group so we can kill entire command tree on timeout/cancel
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.Setpgid = true

	// Start command
	startTime := time.Now().UTC()
	if err := cmd.Start(); err != nil {
		endTime := time.Now().UTC()
		duration := endTime.Sub(startTime)
		execCtx.mu.Lock()
		result.EndTime = &endTime
		result.DurationSeconds = duration.Seconds()
		result.Status = constants.ExecutionStatusFailed
		result.ErrorMessage = system.StringPtr(err.Error())
		result.ErrorType = system.StringPtr("start_error")
		result.ReturnCode = system.IntPtr(es.errorToReturnCode(err))
		execCtx.mu.Unlock()
		return err
	}

	// Store process reference for potential cancellation
	execCtx.mu.Lock()
	execCtx.Process = cmd.Process
	execCtx.mu.Unlock()

	// Monitor for context cancellation/timeout and kill process group
	done := make(chan error, 1)
	go func() {
		done <- cmd.Wait()
	}()

	var err error
	select {
	case <-ctx.Done():
		// Context cancelled or timed out - kill entire process group
		if cmd.Process != nil {
			pgid, pgidErr := syscall.Getpgid(cmd.Process.Pid)
			if pgidErr == nil {
				syscall.Kill(-pgid, syscall.SIGKILL)
			} else {
				cmd.Process.Kill()
			}
		}
		err = <-done // Wait for process to actually exit
	case err = <-done:
		// Command completed normally
	}

	endTime := time.Now().UTC()
	duration := endTime.Sub(startTime)

	// Flush any remaining partial lines
	stdoutWriter.Flush()
	stderrWriter.Flush()

	// Update result
	execCtx.mu.Lock()
	result.EndTime = &endTime
	result.DurationSeconds = duration.Seconds()
	result.Stdout = stdoutBuf.String()
	result.Stderr = stderrBuf.String()

	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			result.Status = constants.ExecutionStatusTimeout
			result.ErrorMessage = system.StringPtr("Command execution timed out")
			result.ErrorType = system.StringPtr("timeout")
			result.ReturnCode = system.IntPtr(124)
		} else if exitError, ok := err.(*exec.ExitError); ok {
			// Command ran but exited with non-zero
			if waitStatus, ok := exitError.Sys().(syscall.WaitStatus); ok {
				exitCode := waitStatus.ExitStatus()
				result.ReturnCode = system.IntPtr(exitCode)

				// Exit codes 126 and 127 may indicate shell errors, but only if stderr
				// contains the actual error message. A deliberate "exit 127" should be
				// treated as completed, not failed.
				stderr := stderrBuf.String()
				stderrLower := strings.ToLower(stderr)

				if exitCode == 126 && (strings.Contains(stderrLower, "permission denied") ||
					strings.Contains(stderrLower, "not executable") ||
					strings.Contains(stderrLower, "cannot execute")) {
					// Actual permission denied error from shell
					result.Status = constants.ExecutionStatusFailed
					result.ErrorMessage = system.StringPtr("Permission denied: command is not executable")
					result.ErrorType = system.StringPtr("permission_denied")
				} else if exitCode == 127 && (strings.Contains(stderrLower, "not found") ||
					strings.Contains(stderrLower, "no such file") ||
					strings.Contains(stderrLower, "command not found")) {
					// Actual command not found error from shell
					result.Status = constants.ExecutionStatusFailed
					result.ErrorMessage = system.StringPtr("Command not found")
					result.ErrorType = system.StringPtr("command_not_found")
				} else {
					// Other non-zero exit codes (including deliberate exit 126/127) are normal completion
					result.Status = constants.ExecutionStatusCompleted
				}
			} else {
				result.Status = constants.ExecutionStatusCompleted
			}
		} else if strings.Contains(err.Error(), "signal: killed") {
			// Process was killed (by us on timeout/cancel, or externally)
			result.Status = constants.ExecutionStatusFailed
			result.ErrorMessage = system.StringPtr("Command was terminated")
			result.ErrorType = system.StringPtr("killed")
			result.ReturnCode = system.IntPtr(137)
		} else {
			result.Status = constants.ExecutionStatusFailed
			result.ErrorMessage = system.StringPtr(err.Error())
			result.ErrorType = system.StringPtr("execution_error")
			result.ReturnCode = system.IntPtr(es.errorToReturnCode(err))
		}
	} else {
		result.Status = constants.ExecutionStatusCompleted
		returnCode := 0
		result.ReturnCode = &returnCode
	}

	// Create terminal output for UI
	result.TerminalOutput = es.createTerminalOutput(request.Command, request.Args, stdoutBuf.String(), stderrBuf.String())

	// Collect system information
	result.SystemInfo = es.collectSystemInfo()
	result.EnvironmentInfo = es.collectEnvironmentInfo()

	execCtx.mu.Unlock()

	return nil
}

// createTerminalOutput creates terminal-formatted output for UI interfaces
func (es *ExecutionService) createTerminalOutput(command string, args []string, stdout, stderr string) *models.TerminalOutput {
	// Combine command and args
	commandWithArgs := command
	if len(args) > 0 {
		commandWithArgs += " " + strings.Join(args, " ")
	}

	// Split output into lines
	stdoutLines := strings.Split(stdout, "\n")
	stderrLines := strings.Split(stderr, "\n")

	// Remove empty last line if present
	if len(stdoutLines) > 0 && stdoutLines[len(stdoutLines)-1] == "" {
		stdoutLines = stdoutLines[:len(stdoutLines)-1]
	}
	if len(stderrLines) > 0 && stderrLines[len(stderrLines)-1] == "" {
		stderrLines = stderrLines[:len(stderrLines)-1]
	}

	// Combine stdout and stderr for terminal simulation
	combinedOutput := stdout
	if stderr != "" {
		if combinedOutput != "" {
			combinedOutput += "\n"
		}
		combinedOutput += stderr
	}

	// Get last 50 lines for UI
	const maxLines = 50
	allLines := append(stdoutLines, stderrLines...)
	var lastLines []string

	if len(allLines) > maxLines {
		lastLines = allLines[len(allLines)-maxLines:]
	} else {
		lastLines = allLines
	}

	return &models.TerminalOutput{
		Command:             command,
		CommandWithArgs:     commandWithArgs,
		CombinedOutput:      combinedOutput,
		LastLines:           lastLines,
		TruncatedStdout:     len(stdoutLines) > maxLines,
		TruncatedStderr:     len(stderrLines) > maxLines,
		OriginalStdoutLines: len(stdoutLines),
		OriginalStderrLines: len(stderrLines),
		TotalOriginalLines:  len(stdoutLines) + len(stderrLines),
	}
}

// errorToReturnCode maps common errors to standard shell return codes
func (es *ExecutionService) errorToReturnCode(err error) int {
	if err == nil {
		return 0
	}
	errStr := err.Error()
	if strings.Contains(errStr, "executable file not found") ||
		strings.Contains(errStr, "no such file or directory") {
		return 127 // Command not found
	}
	if strings.Contains(errStr, "permission denied") {
		return 126 // Command not executable
	}
	return 1 // Generic failure
}

// collectSystemInfo collects system information
func (es *ExecutionService) collectSystemInfo() *models.ExecutionSystemInfo {
	info := &models.ExecutionSystemInfo{
		Hostname:     system.GetHostname(),
		OS:           runtime.GOOS,
		Architecture: runtime.GOARCH,
		NumCPU:       runtime.NumCPU(),
		GoVersion:    runtime.Version(),
		CurrentUser:  system.GetCurrentUser(),
	}

	es.logger.Info("System info collected",
		"hostname", info.Hostname,
		"os", info.OS,
		"architecture", info.Architecture,
		"num_cpu", info.NumCPU,
		"current_user", info.CurrentUser)

	if runtime.GOOS == constants.Status.Platform.Linux {
		es.logger.Info("Linux detected - collecting extended system metrics")

		if loadavg, err := getLoadAverage(); err == nil {
			info.LoadAverage = loadavg
			es.logger.Info("Load average collected", "load_average", loadavg)
		} else {
			es.logger.Info("Failed to collect load average", "error", err)
		}

		if memInfo, err := getMemoryInfo(); err == nil {
			info.Memory = memInfo
			es.logger.Info("Memory information collected", "memory_info", memInfo)
		} else {
			es.logger.Info("Failed to collect memory information", "error", err)
		}
	} else {
		es.logger.Info("Non-Linux OS - skipping extended system metrics", "os", runtime.GOOS)
	}

	return info
}

func (es *ExecutionService) collectEnvironmentInfo() *models.ExecutionEnvironmentInfo {
	envInfo := &models.ExecutionEnvironmentInfo{
		ServiceName: es.config.ServiceName,
		ProjectID:   es.config.ProjectID,
		MaxMemoryMB: es.maxMemoryMB,
	}

	es.logger.Info("Environment configuration collected",
		"max_memory_mb", envInfo.MaxMemoryMB)

	return envInfo
}

// finalizeResult finalizes the execution result
func (es *ExecutionService) finalizeResult(result *models.ExecutionResultsPayload) {
	if result.EndTime == nil {
		endTime := time.Now().UTC()
		result.EndTime = &endTime
	}

	if result.StartTime != nil && result.EndTime != nil {
		result.DurationSeconds = result.EndTime.Sub(*result.StartTime).Seconds()
	}
}

// Stop cancels all active executions and releases resources
func (es *ExecutionService) Stop() {
	es.executionsMutex.Lock()
	defer es.executionsMutex.Unlock()

	if len(es.activeExecutions) == 0 {
		return
	}

	es.logger.Info("Stopping execution service, cancelling active tasks", "count", len(es.activeExecutions))

	for id, execCtx := range es.activeExecutions {
		es.logger.Info("Cancelling task during shutdown", "execution_id", id)
		execCtx.mu.Lock()
		if execCtx.Cancel != nil {
			execCtx.Cancel()
		}
		if execCtx.Process != nil {
			pgid, err := syscall.Getpgid(execCtx.Process.Pid)
			if err == nil {
				syscall.Kill(-pgid, syscall.SIGKILL)
			} else {
				execCtx.Process.Kill()
			}
		}
		execCtx.mu.Unlock()
	}

	// Clear the map
	es.activeExecutions = make(map[string]*ExecutionContext)
}

// GetActiveExecutions returns the currently active executions
func (es *ExecutionService) GetActiveExecutions() map[string]*ExecutionContext {
	es.executionsMutex.RLock()
	defer es.executionsMutex.RUnlock()

	// Create a copy to avoid race conditions
	active := make(map[string]*ExecutionContext)
	for k, v := range es.activeExecutions {
		active[k] = v
	}
	return active
}

// CancelExecution cancels a running execution
func (es *ExecutionService) CancelExecution(requestID string) error {
	es.executionsMutex.RLock()
	execCtx, exists := es.activeExecutions[requestID]
	es.executionsMutex.RUnlock()

	if !exists {
		return fmt.Errorf("execution not found: %s", requestID)
	}

	es.logger.Info("Cancelling execution", "execution_id", requestID)

	execCtx.mu.Lock()
	if execCtx.Cancel != nil {
		execCtx.Cancel()
	}
	if execCtx.Process != nil {
		pgid, err := syscall.Getpgid(execCtx.Process.Pid)
		if err == nil {
			syscall.Kill(-pgid, syscall.SIGKILL)
		} else {
			execCtx.Process.Kill()
		}
	}
	execCtx.mu.Unlock()

	return nil
}

// Helper functions for system information

func getLoadAverage() ([]float64, error) {
	content, err := os.ReadFile("/proc/loadavg")
	if err != nil {
		return nil, err
	}

	fields := strings.Fields(string(content))
	if len(fields) < 3 {
		return nil, fmt.Errorf("invalid loadavg format")
	}

	var loads []float64
	for i := 0; i < 3; i++ {
		var load float64
		if _, err := fmt.Sscanf(fields[i], "%f", &load); err != nil {
			return nil, err
		}
		loads = append(loads, load)
	}

	return loads, nil
}

func getMemoryInfo() (*models.MemoryInfo, error) {
	file, err := os.Open("/proc/meminfo")
	if err != nil {
		return nil, err
	}
	defer file.Close()

	info := &models.MemoryInfo{}
	scanner := bufio.NewScanner(file)

	for scanner.Scan() {
		line := scanner.Text()
		fields := strings.Fields(line)
		if len(fields) >= 2 {
			var value int64
			if _, err := fmt.Sscanf(fields[1], "%d", &value); err == nil {
				valueMB := value / 1024
				switch fields[0] {
				case "MemTotal:":
					info.MemTotal = valueMB
				case "MemFree:":
					info.MemFree = valueMB
				case "MemAvailable:":
					info.MemAvailable = valueMB
				case "Buffers:":
					info.Buffers = valueMB
				case "Cached:":
					info.Cached = valueMB
				case "SwapTotal:":
					info.SwapTotal = valueMB
				case "SwapFree:":
					info.SwapFree = valueMB
				}
			}
		}
	}

	return info, scanner.Err()
}
