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
	"net"
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

	authMethods := ssh.BuildAuthMethods(r, sshAuthSock)
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

	// Respect context cancellation during dial
	dialDone := make(chan struct {
		client *sshlib.Client
		err    error
	}, 1)
	go func() {
		client, err := sshlib.Dial(string(constants.NetworkProtocolTCP), addr, clientConfig)
		dialDone <- struct {
			client *sshlib.Client
			err    error
		}{client, err}
	}()

	var client *sshlib.Client
	select {
	case <-ctx.Done():
		emit(constants.StreamStatusCancelled, "context cancelled")
		return
	case result := <-dialDone:
		if result.err != nil {
			emit(constants.StreamStatusFailed, fmt.Sprintf("dial %s: %v", addr, result.err))
			return
		}
		client = result.client
	}
	defer func() {
		_ = client.Close()
	}()

	session, err := client.NewSession()
	if err != nil {
		emit(constants.StreamStatusFailed, fmt.Sprintf("new session: %v", err))
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
		msg := fmt.Sprintf("run: %v", err)
		if tail := strings.TrimSpace(stderrBuf.String()); tail != "" {
			msg = fmt.Sprintf("%s: %s", msg, tail)
		}
		emit(constants.StreamStatusFailed, msg)
		return
	}

	emit(constants.StreamStatusCompleted, "")
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
