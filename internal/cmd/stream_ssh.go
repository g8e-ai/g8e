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

package cmd

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net"
	"os/exec"
	"strings"
	"time"

	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/g8e-ai/g8e/internal/pkg/ssh"
	sshlib "golang.org/x/crypto/ssh"
)

// streamResult is emitted by streamToHost for each host attempt.
type streamResult struct {
	Host      string
	Status    constants.StreamStatus
	SizeBytes int64
	Error     string
	Elapsed   time.Duration
}

// isTransientError checks if an error is transient and worth retrying.
func isTransientError(err error) bool {
	if err == nil {
		return false
	}
	errStr := err.Error()
	// Network timeouts, connection refused, temporary failures
	transientPatterns := []string{
		"timeout",
		"connection refused",
		"temporary failure",
		"network is unreachable",
		"no route to host",
		"i/o timeout",
		"broken pipe",
		"connection reset",
	}
	for _, pattern := range transientPatterns {
		if strings.Contains(strings.ToLower(errStr), pattern) {
			return true
		}
	}
	return false
}

// preFlightCheck validates SSH connectivity and authentication before binary transfer.
// Returns nil if the host is reachable and auth works, error otherwise.
func preFlightCheck(ctx context.Context, r ssh.HostConfig, sshAuthSock, sshPassphrase string, dialTimeout time.Duration) error {
	authMethods := ssh.BuildAuthMethods(r, sshAuthSock, sshPassphrase)
	if len(authMethods) == 0 {
		return fmt.Errorf("no SSH auth methods available")
	}

	hostKeyCallback, cbErr := ssh.BuildHostKeyCallback()
	if cbErr != nil {
		return fmt.Errorf("host key callback: %w", cbErr)
	}

	clientConfig := &sshlib.ClientConfig{
		User:            r.User,
		Auth:            authMethods,
		HostKeyCallback: hostKeyCallback,
		Timeout:         dialTimeout,
	}

	addr := net.JoinHostPort(r.Hostname, r.Port)

	// Try to establish a connection and run a simple command
	dialDone := make(chan struct {
		client *sshlib.Client
		err    error
	}, 1)
	go func() {
		var conn net.Conn
		var err error

		// Use ProxyCommand if specified
		if r.ProxyCommand != "" {
			proxyCmd := strings.ReplaceAll(r.ProxyCommand, "%h", r.Hostname)
			proxyCmd = strings.ReplaceAll(proxyCmd, "%p", r.Port)

			cmd := exec.CommandContext(ctx, "sh", "-c", proxyCmd)
			stdin, err := cmd.StdinPipe()
			if err != nil {
				dialDone <- struct {
					client *sshlib.Client
					err    error
				}{nil, fmt.Errorf("proxy command stdin pipe: %w", err)}
				return
			}
			stdout, err := cmd.StdoutPipe()
			if err != nil {
				dialDone <- struct {
					client *sshlib.Client
					err    error
				}{nil, fmt.Errorf("proxy command stdout pipe: %w", err)}
				return
			}
			if err := cmd.Start(); err != nil {
				dialDone <- struct {
					client *sshlib.Client
					err    error
				}{nil, fmt.Errorf("proxy command start: %w", err)}
				return
			}

			conn = &proxyConn{
				stdin:  stdin,
				stdout: stdout,
				cmd:    cmd,
				addr:   addr,
			}
		} else {
			conn, err = net.DialTimeout(string(constants.NetworkProtocolTCP), addr, dialTimeout)
			if err != nil {
				dialDone <- struct {
					client *sshlib.Client
					err    error
				}{nil, err}
				return
			}
			if tcpConn, ok := conn.(*net.TCPConn); ok {
				_ = tcpConn.SetKeepAlive(true)
				_ = tcpConn.SetKeepAlivePeriod(15 * time.Second)
			}
		}

		sshConn, chans, reqs, err := sshlib.NewClientConn(conn, addr, clientConfig)
		if err != nil {
			conn.Close()
			dialDone <- struct {
				client *sshlib.Client
				err    error
			}{nil, err}
			return
		}
		client := sshlib.NewClient(sshConn, chans, reqs)
		dialDone <- struct {
			client *sshlib.Client
			err    error
		}{client, nil}
	}()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case result := <-dialDone:
		if result.err != nil {
			return result.err
		}
		defer result.client.Close()

		// Run a simple command to verify the session works
		session, err := result.client.NewSession()
		if err != nil {
			return fmt.Errorf("new session: %w", err)
		}
		defer session.Close()

		// Run 'true' command - minimal check that remote shell works
		err = session.Run("true")
		if err != nil {
			return fmt.Errorf("remote command failed: %w", err)
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
	r := ssh.HostConfig{}

	emit := func(status constants.StreamStatus, errMsg string) {
		resultCh <- streamResult{
			Host:      target,
			Status:    status,
			SizeBytes: int64(len(binaryData)),
			Error:     errMsg,
			Elapsed:   time.Since(start),
		}
	}

	select {
	case <-ctx.Done():
		emit(constants.StreamStatusCancelled, "context cancelled")
		return
	default:
	}

	r = ssh.ResolveHost(target, sshConfigPath, username, sshIdentityFile, sshUser)

	// Pre-flight check if enabled
	if enablePreFlightCheck {
		if err := preFlightCheck(ctx, r, sshAuthSock, sshPassphrase, dialTimeout); err != nil {
			emit(constants.StreamStatusFailed, fmt.Sprintf("pre-flight check failed: %v", err))
			return
		}
	}

	authMethods := ssh.BuildAuthMethods(r, sshAuthSock, sshPassphrase)
	if len(authMethods) == 0 {
		emit(constants.StreamStatusFailed, "no SSH auth methods available (no keys found, no agent)")
		return
	}

	hostKeyCallback, cbErr := ssh.BuildHostKeyCallback()
	if cbErr != nil {
		emit(constants.StreamStatusFailed, cbErr.Error())
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
	const maxRetries = 3
	var lastErr error
	var retryCount int

	for retryCount = 0; retryCount <= maxRetries; retryCount++ {
		if retryCount > 0 {
			// Exponential backoff: 1s, 2s, 4s
			backoff := time.Duration(1<<uint(retryCount-1)) * time.Second
			select {
			case <-time.After(backoff):
			case <-ctx.Done():
				emit(constants.StreamStatusCancelled, "context cancelled during retry backoff")
				return
			}
		}

		// Respect context cancellation during dial
		dialDone := make(chan struct {
			client *sshlib.Client
			err    error
		}, 1)
		go func() {
			var conn net.Conn
			var err error

			// Use ProxyCommand if specified
			if r.ProxyCommand != "" {
				// Execute proxy command and use its stdin/stdout as the connection
				// Replace %h with hostname, %p with port
				proxyCmd := strings.ReplaceAll(r.ProxyCommand, "%h", r.Hostname)
				proxyCmd = strings.ReplaceAll(proxyCmd, "%p", r.Port)

				cmd := exec.CommandContext(ctx, "sh", "-c", proxyCmd)
				stdin, err := cmd.StdinPipe()
				if err != nil {
					dialDone <- struct {
						client *sshlib.Client
						err    error
					}{nil, fmt.Errorf("proxy command stdin pipe: %w", err)}
					return
				}
				stdout, err := cmd.StdoutPipe()
				if err != nil {
					dialDone <- struct {
						client *sshlib.Client
						err    error
					}{nil, fmt.Errorf("proxy command stdout pipe: %w", err)}
					return
				}
				if err := cmd.Start(); err != nil {
					dialDone <- struct {
						client *sshlib.Client
						err    error
					}{nil, fmt.Errorf("proxy command start: %w", err)}
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
				conn, err = net.DialTimeout(string(constants.NetworkProtocolTCP), addr, dialTimeout)
				if err != nil {
					dialDone <- struct {
						client *sshlib.Client
						err    error
					}{nil, err}
					return
				}
				// Enable TCP keepalive on the connection
				if tcpConn, ok := conn.(*net.TCPConn); ok {
					_ = tcpConn.SetKeepAlive(true)
					_ = tcpConn.SetKeepAlivePeriod(15 * time.Second)
				}
			}

			// Establish SSH client over the connection
			sshConn, chans, reqs, err := sshlib.NewClientConn(conn, addr, clientConfig)
			if err != nil {
				conn.Close()
				dialDone <- struct {
					client *sshlib.Client
					err    error
				}{nil, err}
				return
			}
			client := sshlib.NewClient(sshConn, chans, reqs)
			dialDone <- struct {
				client *sshlib.Client
				err    error
			}{client, nil}
		}()

		var client *sshlib.Client
		select {
		case <-ctx.Done():
			emit(constants.StreamStatusCancelled, "context cancelled")
			return
		case result := <-dialDone:
			if result.err != nil {
				lastErr = result.err
				if retryCount < maxRetries && isTransientError(result.err) {
					continue // Retry transient errors
				}
				emit(constants.StreamStatusFailed, fmt.Sprintf("dial %s: %v (after %d retries)", addr, result.err, retryCount))
				return
			}
			client = result.client
		}
		defer func() {
			_ = client.Close()
		}()
		// Send keepalive requests every 15s, fail after 3 missed responses (45s total)
		go func() {
			ticker := time.NewTicker(15 * time.Second)
			defer ticker.Stop()
			missedCount := 0
			for {
				select {
				case <-ctx.Done():
					return
				case <-ticker.C:
					_, _, err := client.SendRequest("keepalive@g8e", true, nil)
					if err != nil {
						missedCount++
						if missedCount >= 3 {
							// Close client on persistent keepalive failure
							client.Close()
							return
						}
					} else {
						missedCount = 0
					}
				}
			}
		}()

		session, err := client.NewSession()
		if err != nil {
			lastErr = err
			if retryCount < maxRetries && isTransientError(err) {
				continue // Retry transient errors
			}
			emit(constants.StreamStatusFailed, fmt.Sprintf("new session: %v (after %d retries)", err, retryCount))
			return
		}
		defer func() {
			_ = session.Close()
		}()

		// Wire binary data as the remote stdin.
		session.Stdin = bytes.NewReader(binaryData)

		// Capture stdout+stderr (bounded) so the caller can surface the remote
		// operator's output when it exits non-zero. Without this, the deployment
		// tool silently drops every remote log line and a failing operator is
		// indistinguishable from a generic SSH exit - see g8eo review notes.
		const maxCapturedBytes = 64 * 1024
		stderrBuf := &boundedBuffer{limit: maxCapturedBytes}
		stdoutBuf := &boundedBuffer{limit: maxCapturedBytes}
		session.Stderr = stderrBuf
		session.Stdout = stdoutBuf

		// Build the remote ephemeral script inline.
		//
		// Critical: when the local stream is cancelled (Ctrl-C, ctx cancel) we
		// must guarantee the remote operator dies with the session. Without a
		// PTY, sshd does not automatically HUP the remote process group, and a
		// plain `& wait $!` pattern leaves the backgrounded operator orphaned to
		// init. We therefore:
		//   1. Install a trap on HUP/INT/TERM that forwards the signal to the
		//      operator's PID and the whole process group, then waits briefly
		//      for graceful exit before SIGKILL.
		//   2. Run the operator in its own process group (setsid) so we can
		//      signal the group, covering any children it spawned.
		//   3. `wait "$PID"` is interruptible by trapped signals, so the trap
		//      fires promptly rather than after the operator exits on its own.
		var remoteCmd string
		if operatorArgs != "" {
			remoteCmd = fmt.Sprintf(
				`set -e
B=$(mktemp)
cat > "$B"
chmod +x "$B"
cleanup() {
  sig=${1:-TERM}
  if [ -n "${PID:-}" ] && kill -0 "$PID" 2>/dev/null; then
    kill -"$sig" "-$PID" 2>/dev/null || kill -"$sig" "$PID" 2>/dev/null || true
    for _ in 1 2 3 4 5 6 7 8 9 10; do
      kill -0 "$PID" 2>/dev/null || break
      sleep 0.2
    done
    kill -0 "$PID" 2>/dev/null && { kill -KILL "-$PID" 2>/dev/null || kill -KILL "$PID" 2>/dev/null || true; }
  fi
  rm -f "$B"
}
trap 'cleanup TERM; exit 143' HUP INT TERM
trap 'rm -f "$B"' EXIT
setsid "$B" %s < /dev/null &
PID=$!
wait "$PID"`,
				operatorArgs,
			)
		} else {
			remoteCmd = `set -e; B=$(mktemp); cat > "$B"; chmod +x "$B"; trap 'rm -f "$B"' EXIT; echo "[g8e] Binary injected into $B -- run it manually: $B -e <endpoint> [options]"`
		}

		// Check for context cancellation before running
		select {
		case <-ctx.Done():
			emit(constants.StreamStatusCancelled, "context cancelled before run")
			return
		default:
		}

		// Watcher: on ctx cancellation, send SIGHUP to the remote shell and
		// close the session so sshd tears down the channel. Our remote trap
		// will fire and kill the operator process group.
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
			// SSH exit status non-zero is surfaced as *ssh.ExitError - treat operator
			// exit as a normal end of session, not a hard failure, but attach the
			// captured remote stderr (and last-resort stdout) so the caller can
			// tell a real auth/registration failure apart from a clean exit.
			var exitErr *sshlib.ExitError
			if isSSHExitError(err, &exitErr) {
				msg := fmt.Sprintf("exit code %d", exitErr.ExitStatus())
				if tail := strings.TrimSpace(stderrBuf.String()); tail != "" {
					msg = fmt.Sprintf("%s: %s", msg, tail)
				} else if tail := strings.TrimSpace(stdoutBuf.String()); tail != "" {
					msg = fmt.Sprintf("%s: %s", msg, tail)
				}
				emit(constants.StreamStatusExited, msg)
				return
			}
			lastErr = err
			if retryCount < maxRetries && isTransientError(err) {
				continue // Retry transient errors
			}
			msg := fmt.Sprintf("run: %v", err)
			if tail := strings.TrimSpace(stderrBuf.String()); tail != "" {
				msg = fmt.Sprintf("%s: %s", msg, tail)
			}
			emit(constants.StreamStatusFailed, msg)
			return
		}

		emit(constants.StreamStatusCompleted, "")
		return
	}

	// If we exhausted retries
	emit(constants.StreamStatusFailed, fmt.Sprintf("failed after %d retries, last error: %v", maxRetries, lastErr))
}

// isSSHExitError checks whether err is an *ssh.ExitError and sets target.
func isSSHExitError(err error, target **sshlib.ExitError) bool {
	if e, ok := err.(*sshlib.ExitError); ok {
		*target = e
		return true
	}
	return false
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

func (c *proxyConn) Read(b []byte) (n int, err error) {
	return c.stdout.Read(b)
}

func (c *proxyConn) Write(b []byte) (n int, err error) {
	return c.stdin.Write(b)
}

func (c *proxyConn) Close() error {
	_ = c.stdin.Close()
	_ = c.cmd.Wait()
	return nil
}

func (c *proxyConn) LocalAddr() net.Addr {
	return &proxyAddr{addr: "proxy"}
}

func (c *proxyConn) RemoteAddr() net.Addr {
	return &proxyAddr{addr: c.addr}
}

func (c *proxyConn) SetDeadline(t time.Time) error {
	return nil // Not supported for proxy connections
}

func (c *proxyConn) SetReadDeadline(t time.Time) error {
	return nil // Not supported for proxy connections
}

func (c *proxyConn) SetWriteDeadline(t time.Time) error {
	return nil // Not supported for proxy connections
}

type proxyAddr struct {
	addr string
}

func (a *proxyAddr) Network() string { return "tcp" }
func (a *proxyAddr) String() string  { return a.addr }
