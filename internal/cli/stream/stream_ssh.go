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

package stream

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"os/exec"
	"strings"
	"time"

	sshlib "golang.org/x/crypto/ssh"

	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/g8e-ai/g8e/internal/pkg/ssh"
)

// streamResult is emitted by streamToHost for each host attempt.
type streamResult struct {
	Host      string
	Status    constants.StreamStatus
	SizeBytes int64
	Error     error
	Elapsed   time.Duration
}

// dialResult is the result of an asynchronous SSH dial attempt.
type dialResult struct {
	client *sshlib.Client
	err    error
}

// isTransientError checks if an error is transient and worth retrying.
func isTransientError(err error) bool {
	if err == nil {
		return false
	}
	errStr := strings.ToLower(err.Error())
	for _, pattern := range constants.TransientNetworkErrorPatterns {
		if strings.Contains(errStr, pattern) {
			return true
		}
	}
	return false
}

// dialSSH establishes an SSH connection to the given address, supporting both
// direct TCP and ProxyCommand connections. It sends exactly one dialResult to
// the returned channel.
func dialSSH(ctx context.Context, r ssh.HostConfig, clientConfig *sshlib.ClientConfig, addr string) <-chan dialResult {
	ch := make(chan dialResult, 1)
	go func() {
		var conn net.Conn
		var err error

		// Use ProxyCommand if specified
		if r.ProxyCommand != "" {
			// Execute proxy command and use its stdin/stdout as the connection
			// Replace %h with hostname, %p with port
			proxyCmd := strings.ReplaceAll(r.ProxyCommand, "%h", r.Hostname)
			proxyCmd = strings.ReplaceAll(proxyCmd, "%p", r.Port)

			cmd := exec.CommandContext(ctx, constants.PathBinSh, "-c", proxyCmd)
			stdin, err := cmd.StdinPipe()
			if err != nil {
				ch <- dialResult{nil, fmt.Errorf("ssh: proxy stdin pipe: %w", err)}
				return
			}
			stdout, err := cmd.StdoutPipe()
			if err != nil {
				ch <- dialResult{nil, fmt.Errorf("ssh: proxy stdout pipe: %w", err)}
				return
			}
			if err := cmd.Start(); err != nil {
				ch <- dialResult{nil, fmt.Errorf("ssh: start proxy: %w", err)}
				return
			}

			// Create a net.Conn wrapper around the proxy command pipes
			conn = &proxyConn{
				stdin:  stdin,
				stdout: stdout,
				cmd:    cmd,
				addr:   addr,
			}
		} else {
			// Direct TCP connection with keepalive
			conn, err = net.DialTimeout(string(constants.NetworkProtocolTCP), addr, clientConfig.Timeout)
			if err != nil {
				ch <- dialResult{nil, fmt.Errorf("ssh: dial: %w", err)}
				return
			}
			// Enable TCP keepalive on the connection
			if tcpConn, ok := conn.(*net.TCPConn); ok {
				if err := tcpConn.SetKeepAlive(true); err != nil {
					conn.Close()
					ch <- dialResult{nil, fmt.Errorf("ssh: set keepalive: %w", err)}
					return
				}
				if err := tcpConn.SetKeepAlivePeriod(constants.SSHKeepaliveInterval); err != nil {
					conn.Close()
					ch <- dialResult{nil, fmt.Errorf("ssh: set keepalive period: %w", err)}
					return
				}
			}
		}

		// Establish SSH client over the connection
		sshConn, chans, reqs, err := sshlib.NewClientConn(conn, addr, clientConfig)
		if err != nil {
			conn.Close()
			ch <- dialResult{nil, fmt.Errorf("ssh: client connection: %w", err)}
			return
		}
		client := sshlib.NewClient(sshConn, chans, reqs)
		ch <- dialResult{client, nil}
	}()
	return ch
}

// preFlightCheck validates SSH connectivity and authentication before binary transfer.
// Returns nil if the host is reachable and auth works, error otherwise.
func preFlightCheck(ctx context.Context, r ssh.HostConfig, sshAuthSock, sshPassphrase, knownHostsPath string, dialTimeout time.Duration) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("ssh: preflight: %w", err)
	}
	authMethods, err := ssh.BuildAuthMethods(r, sshAuthSock, sshPassphrase)
	if err != nil {
		return fmt.Errorf("ssh: build auth: %w", err)
	}
	if len(authMethods) == 0 {
		return fmt.Errorf("ssh: preflight: %w", constants.ErrMCPRunShellCommandNoAuth)
	}

	hostKeyCallback, cbErr := ssh.BuildHostKeyCallback(knownHostsPath)
	if cbErr != nil {
		return fmt.Errorf("ssh: host key callback: %w", cbErr)
	}

	clientConfig := &sshlib.ClientConfig{
		User:            r.User,
		Auth:            authMethods,
		HostKeyCallback: hostKeyCallback,
		Timeout:         dialTimeout,
	}

	addr := net.JoinHostPort(r.Hostname, r.Port)

	select {
	case <-ctx.Done():
		return fmt.Errorf("ssh: preflight: %w", ctx.Err())
	case result := <-dialSSH(ctx, r, clientConfig, addr):
		if result.err != nil {
			return result.err
		}
		defer result.client.Close()

		// Run a simple command to verify the session works
		session, err := result.client.NewSession()
		if err != nil {
			return fmt.Errorf("ssh: session: %w", err)
		}
		defer session.Close()

		// Run 'true' command - minimal check that remote shell works
		if err := session.Run(constants.SSHPreflightVerifyCommand); err != nil {
			return fmt.Errorf("ssh: verify: %w", err)
		}
		return nil
	}
}

// streamToHost injects the binary into one remote host via SSH and optionally
// starts the operator. It sends exactly one streamResult to resultCh.
func streamToHost(
	ctx context.Context,
	target string,
	binaryData []byte,
	operatorArgs string,
	sshConfigPath string,
	sshKnownHostsPath string,
	dialTimeout time.Duration,
	sshAuthSock string,
	username string,
	sshIdentityFile string,
	sshUser string,
	sshPassphrase string,
	enablePreFlightCheck bool,
	resultCh chan<- streamResult,
) {
	start := time.Now()

	emit := func(status constants.StreamStatus, err error) {
		resultCh <- streamResult{
			Host:      target,
			Status:    status,
			SizeBytes: int64(len(binaryData)),
			Error:     err,
			Elapsed:   time.Since(start),
		}
	}

	select {
	case <-ctx.Done():
		emit(constants.StreamStatusCancelled, constants.ErrSSHContextCancelled)
		return
	default:
	}

	r, err := ssh.ResolveHost(target, sshConfigPath, username, sshIdentityFile, sshUser)
	if err != nil {
		emit(constants.StreamStatusFailed, fmt.Errorf("ssh: resolve host: %w", err))
		return
	}

	// Pre-flight check if enabled
	if enablePreFlightCheck {
		if err := preFlightCheck(ctx, r, sshAuthSock, sshPassphrase, sshKnownHostsPath, dialTimeout); err != nil {
			emit(constants.StreamStatusFailed, err)
			return
		}
	}

	authMethods, err := ssh.BuildAuthMethods(r, sshAuthSock, sshPassphrase)
	if err != nil {
		emit(constants.StreamStatusFailed, fmt.Errorf("ssh: build auth: %w", err))
		return
	}
	if len(authMethods) == 0 {
		emit(constants.StreamStatusFailed, fmt.Errorf("ssh: %w", constants.ErrMCPRunShellCommandNoAuth))
		return
	}

	hostKeyCallback, cbErr := ssh.BuildHostKeyCallback(sshKnownHostsPath)
	if cbErr != nil {
		emit(constants.StreamStatusFailed, fmt.Errorf("ssh: host key callback: %w", cbErr))
		return
	}

	clientConfig := &sshlib.ClientConfig{
		User:            r.User,
		Auth:            authMethods,
		HostKeyCallback: hostKeyCallback,
		Timeout:         dialTimeout,
	}

	addr := net.JoinHostPort(r.Hostname, r.Port)

	// Retry logic with exponential backoff for transient errors
	var lastErr error
	var retryCount int

	for retryCount = 0; retryCount <= constants.SSHMaxRetries; retryCount++ {
		if retryCount > 0 {
			backoff := time.Duration(1<<uint(retryCount-1)) * time.Second
			select {
			case <-time.After(backoff):
			case <-ctx.Done():
				emit(constants.StreamStatusCancelled, constants.ErrSSHRetryBackoffCancelled)
				return
			}
		}

		var client *sshlib.Client
		var session *sshlib.Session
		keepaliveDone := make(chan struct{})

		cleanup := func() {
			close(keepaliveDone)
			if session != nil {
				_ = session.Close()
			}
			if client != nil {
				_ = client.Close()
			}
		}

		select {
		case <-ctx.Done():
			emit(constants.StreamStatusCancelled, constants.ErrSSHContextCancelled)
			return
		case result := <-dialSSH(ctx, r, clientConfig, addr):
			if result.err != nil {
				lastErr = result.err
				if retryCount < constants.SSHMaxRetries && isTransientError(result.err) {
					continue
				}
				emit(constants.StreamStatusFailed, fmt.Errorf("ssh: dial: %s (after %d retries): %w", addr, retryCount, result.err))
				return
			}
			client = result.client
		}

		go func() {
			ticker := time.NewTicker(constants.SSHKeepaliveInterval)
			defer ticker.Stop()
			missedCount := 0
			for {
				select {
				case <-ctx.Done():
					return
				case <-keepaliveDone:
					return
				case <-ticker.C:
					_, _, err := client.SendRequest(constants.SSHKeepaliveRequestType, true, nil)
					if err != nil {
						missedCount++
						if missedCount >= constants.SSHKeepaliveMaxMissed {
							_ = client.Close()
							return
						}
					} else {
						missedCount = 0
					}
				}
			}
		}()

		session, err = client.NewSession()
		if err != nil {
			lastErr = err
			if retryCount < constants.SSHMaxRetries && isTransientError(err) {
				cleanup()
				continue
			}
			cleanup()
			emit(constants.StreamStatusFailed, fmt.Errorf("ssh: session (after %d retries): %w", retryCount, err))
			return
		}

		// Wire binary data as the remote stdin.
		session.Stdin = bytes.NewReader(binaryData)

		// Capture stdout+stderr (bounded)
		stderrBuf := &boundedBuffer{limit: constants.SSHCaptureMaxBytes}
		stdoutBuf := &boundedBuffer{limit: constants.SSHCaptureMaxBytes}
		session.Stderr = stderrBuf
		session.Stdout = stdoutBuf

		var remoteCmd string
		if operatorArgs != "" {
			remoteCmd = fmt.Sprintf(constants.RemoteEphemeralScriptTemplate, operatorArgs)
		} else {
			msg := fmt.Sprintf(constants.RemoteInjectedBinaryMessage, "$B", "$B")
			remoteCmd = fmt.Sprintf(constants.RemoteInjectedScriptMinimal, msg)
		}

		runDone := make(chan struct{})
		go func() {
			select {
			case <-ctx.Done():
				_ = session.Signal(sshlib.SIGHUP)
				_ = session.Close()
			case <-runDone:
			}
		}()

		err = session.Run(remoteCmd)
		close(runDone)
		if err != nil {
			var exitErr *sshlib.ExitError
			if errors.As(err, &exitErr) {
				msg := fmt.Errorf("ssh: exit code %d", exitErr.ExitStatus())
				if tail := strings.TrimSpace(stderrBuf.String()); tail != "" {
					msg = fmt.Errorf("%w: %s", msg, tail)
				} else if tail := strings.TrimSpace(stdoutBuf.String()); tail != "" {
					msg = fmt.Errorf("%w: %s", msg, tail)
				}
				cleanup()
				emit(constants.StreamStatusExited, msg)
				return
			}
			lastErr = err
			if retryCount < constants.SSHMaxRetries && isTransientError(err) {
				cleanup()
				continue
			}
			msg := fmt.Errorf("ssh: run: %w", err)
			if tail := strings.TrimSpace(stderrBuf.String()); tail != "" {
				msg = fmt.Errorf("%w: %s", msg, tail)
			}
			cleanup()
			emit(constants.StreamStatusFailed, msg)
			return
		}

		cleanup()
		emit(constants.StreamStatusCompleted, nil)
		return
	}

	// If we exhausted retries
	emit(constants.StreamStatusFailed, fmt.Errorf("ssh: exhausted retries: %d retries: last error: %w", constants.SSHMaxRetries, lastErr))
}

// boundedBuffer is an io.Writer that retains at most `limit` bytes, dropping
// any overflow silently. It is used to capture remote stderr/stdout from an
// SSH session without risking unbounded memory growth for chatty operators.
type boundedBuffer struct {
	buf   bytes.Buffer
	limit int
}

func (b *boundedBuffer) Write(p []byte) (int, error) {
	if b.limit <= 0 {
		return len(p), nil
	}
	remaining := b.limit - b.buf.Len()
	if remaining <= 0 {
		return len(p), nil
	}
	if len(p) > remaining {
		b.buf.Write(p[:remaining])
		return len(p), nil
	}
	b.buf.Write(p)
	return len(p), nil
}

func (b *boundedBuffer) String() string { return b.buf.String() }

// proxyConn wraps a proxy command's stdin/stdout as a net.Conn.
type proxyConn struct {
	stdin  io.WriteCloser
	stdout io.Reader
	cmd    *exec.Cmd
	addr   string
}

func (c *proxyConn) Read(b []byte) (int, error) {
	return c.stdout.Read(b)
}

func (c *proxyConn) Write(b []byte) (int, error) {
	return c.stdin.Write(b)
}

func (c *proxyConn) Close() error {
	_ = c.stdin.Close()
	if err := c.cmd.Wait(); err != nil {
		return fmt.Errorf("ssh: proxy command wait: %w", err)
	}
	return nil
}

func (c *proxyConn) LocalAddr() net.Addr {
	return &proxyAddr{addr: constants.SSHProxyAddrLabel}
}

func (c *proxyConn) RemoteAddr() net.Addr {
	return &proxyAddr{addr: c.addr}
}

func (c *proxyConn) SetDeadline(t time.Time) error {
	return nil
}

func (c *proxyConn) SetReadDeadline(t time.Time) error {
	return nil
}

func (c *proxyConn) SetWriteDeadline(t time.Time) error {
	return nil
}

type proxyAddr struct {
	addr string
}

func (a *proxyAddr) Network() string { return string(constants.NetworkProtocolTCP) }
func (a *proxyAddr) String() string  { return a.addr }
