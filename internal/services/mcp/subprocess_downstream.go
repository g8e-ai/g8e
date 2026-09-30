// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os/exec"
	"sync"
	"sync/atomic"

	"github.com/g8e-ai/g8e/v2/internal/constants"
)

// SubprocessDownstream manages an MCP downstream subprocess communicating over stdio pipes.
type SubprocessDownstream struct {
	command string
	args    []string
	logger  *slog.Logger

	mu      sync.Mutex
	cmd     *exec.Cmd
	stdin   io.WriteCloser
	scanner *bufio.Scanner
	started bool
	closed  bool
	reqID   atomic.Int64
}

// NewSubprocessDownstream creates a new SubprocessDownstream manager.
func NewSubprocessDownstream(command string, args []string, logger *slog.Logger) *SubprocessDownstream {
	if logger == nil {
		logger = slog.Default()
	}
	return &SubprocessDownstream{
		command: command,
		args:    args,
		logger:  logger,
	}
}

// Start launches the downstream subprocess.
func (s *SubprocessDownstream) Start() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.startLocked()
}

func (s *SubprocessDownstream) startLocked() error {
	if s.closed {
		return fmt.Errorf("subprocess downstream: closed")
	}
	if s.started && s.cmd != nil && s.cmd.Process != nil {
		return nil
	}

	cmd := exec.Command(s.command, s.args...) //nolint:gosec
	setSubprocessSysProcAttr(cmd)

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return fmt.Errorf("%w: stdin pipe: %w", constants.ErrProcessStartFailed, err)
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		_ = stdin.Close()
		return fmt.Errorf("%w: stdout pipe: %w", constants.ErrProcessStartFailed, err)
	}

	scanner := bufio.NewScanner(stdout)
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024)

	if err := cmd.Start(); err != nil {
		_ = stdin.Close()
		return fmt.Errorf("%w: %w", constants.ErrProcessStartFailed, err)
	}

	s.cmd = cmd
	s.stdin = stdin
	s.scanner = scanner
	s.started = true

	s.logger.Info("Subprocess downstream started",
		"command", s.command,
		"args", s.args,
		"pid", cmd.Process.Pid,
	)

	return nil
}

// Call sends a JSON-RPC request to the downstream subprocess and awaits its JSON-RPC response.
func (s *SubprocessDownstream) Call(ctx context.Context, method string, params any) (json.RawMessage, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return nil, fmt.Errorf("subprocess downstream: closed")
	}

	// Lazy start if not started or if process exited
	if !s.started || s.cmd == nil || s.cmd.Process == nil {
		if err := s.startLocked(); err != nil {
			return nil, err
		}
	}

	id := s.reqID.Add(1)
	req := downstreamJSONRPCRequest{
		JSONRPC: "2.0",
		ID:      int(id),
		Method:  method,
		Params:  params,
	}

	reqBytes, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("subprocess downstream: marshal request: %w", err)
	}

	// Write request line to stdin
	if _, err := fmt.Fprintf(s.stdin, "%s\n", reqBytes); err != nil {
		s.cleanupProcessLocked()
		return nil, fmt.Errorf("subprocess downstream: write stdin: %w", err)
	}

	// Read response line from stdout with context support
	type scanResult struct {
		line []byte
		err  error
	}

	resultChan := make(chan scanResult, 1)
	go func() {
		if s.scanner != nil && s.scanner.Scan() {
			b := s.scanner.Bytes()
			line := make([]byte, len(b))
			copy(line, b)
			resultChan <- scanResult{line: line}
		} else {
			var scanErr error
			if s.scanner != nil {
				scanErr = s.scanner.Err()
			}
			if scanErr == nil {
				scanErr = io.EOF
			}
			resultChan <- scanResult{err: scanErr}
		}
	}()

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case res := <-resultChan:
		if res.err != nil {
			s.cleanupProcessLocked()
			return nil, fmt.Errorf("subprocess downstream: read stdout: %w", res.err)
		}

		var resp struct {
			JSONRPC string          `json:"jsonrpc"`
			ID      interface{}     `json:"id"`
			Result  json.RawMessage `json:"result,omitempty"`
			Error   *struct {
				Code    int    `json:"code"`
				Message string `json:"message"`
			} `json:"error,omitempty"`
		}

		if err := json.Unmarshal(res.line, &resp); err != nil {
			return nil, fmt.Errorf("%w: invalid JSON response from subprocess: %w", constants.ErrInvalidJSONResponse, err)
		}

		if resp.Error != nil {
			return nil, fmt.Errorf("%w: code %d: %s", constants.ErrGatewayMCPError, resp.Error.Code, resp.Error.Message)
		}

		return resp.Result, nil
	}
}

// Close terminates the downstream subprocess.
func (s *SubprocessDownstream) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.closed = true
	s.cleanupProcessLocked()
	return nil
}

func (s *SubprocessDownstream) cleanupProcessLocked() {
	if s.stdin != nil {
		_ = s.stdin.Close()
		s.stdin = nil
	}
	if s.cmd != nil && s.cmd.Process != nil {
		_ = s.cmd.Process.Kill()
		_ = s.cmd.Wait()
		s.cmd = nil
	}
	s.scanner = nil
	s.started = false
}

// Command returns the configured downstream command.
func (s *SubprocessDownstream) Command() string {
	return s.command
}

// Args returns the configured downstream arguments.
func (s *SubprocessDownstream) Args() []string {
	return s.args
}

// IsStarted reports whether the subprocess is currently running.
func (s *SubprocessDownstream) IsStarted() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.started && s.cmd != nil && s.cmd.Process != nil
}
