// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package stream

import (
	"bytes"
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/pkg/sftp"
	sshlib "golang.org/x/crypto/ssh"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/pkg/ssh"
)

// StreamHostOptions defines configuration for streaming the binary to a host.
type StreamHostOptions struct {
	BinaryData        []byte
	NoGit             bool
	GatewayHTTPPort   int
	GatewayHTTPSPort  int
	SSHConfigPath     string
	SSHKnownHostsPath string
	DialTimeout       time.Duration
	SSHAuthSock       string
	Username          string
	SSHIdentityFile   string
	SSHUser           string
	SSHPassphrase     string
	PreFlightCheck    bool
}

// streamResult is emitted by streamToHost for each host attempt.
type streamResult struct {
	Host      string
	Status    constants.StreamStatus
	SizeBytes int64
	Error     error
	Elapsed   time.Duration
}

// isTransientError checks if an error is transient and worth retrying.
func isTransientError(err error) bool {
	if err == nil {
		return false
	}
	if isTransientSocketError(err) {
		return true
	}
	var networkError net.Error
	if errors.As(err, &networkError) && networkError.Timeout() {
		return true
	}
	errStr := strings.ToLower(err.Error())
	for _, pattern := range constants.TransientNetworkErrorPatterns {
		if strings.Contains(errStr, pattern) {
			return true
		}
	}
	return false
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

	client, err := ssh.DialSSH(ctx, r, clientConfig, addr)
	if err != nil {
		return err
	}
	defer client.Close()

	// Run a simple command to verify the session works
	session, err := client.NewSession()
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

func generateRemotePath() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generate remote path: %w", err)
	}
	return fmt.Sprintf("%s/%s%x", constants.PathTmp, constants.StreamTempBinaryPrefix, b), nil
}

func uploadBinaryViaSFTP(client *sshlib.Client, binaryData []byte, remotePath string) error {
	sftpClient, err := sftp.NewClient(client)
	if err != nil {
		return fmt.Errorf("ssh: sftp client: %w", err)
	}
	defer sftpClient.Close()

	dstFile, err := sftpClient.Create(remotePath)
	if err != nil {
		return fmt.Errorf("ssh: sftp create: %w", err)
	}

	if _, err := io.Copy(dstFile, bytes.NewReader(binaryData)); err != nil {
		_ = dstFile.Close()
		return fmt.Errorf("ssh: sftp write: %w", err)
	}

	if err := dstFile.Close(); err != nil {
		return fmt.Errorf("ssh: sftp close: %w", err)
	}

	if err := sftpClient.Chmod(remotePath, constants.PermFileExecutable); err != nil {
		return fmt.Errorf("ssh: sftp chmod: %w", err)
	}

	return nil
}

func removeRemoteBinary(client *sshlib.Client, remotePath string) {
	sftpClient, err := sftp.NewClient(client)
	if err != nil {
		return
	}
	defer sftpClient.Close()
	_ = sftpClient.Remove(remotePath)
}

func getListenerPort(listener net.Listener) (int, error) {
	addr := listener.Addr()
	if tcpAddr, ok := addr.(*net.TCPAddr); ok {
		return tcpAddr.Port, nil
	}
	_, portStr, err := net.SplitHostPort(addr.String())
	if err != nil {
		return 0, fmt.Errorf("ssh: parse listener addr %s: %w", addr.String(), err)
	}
	var port int
	if _, err := fmt.Sscanf(portStr, "%d", &port); err != nil {
		return 0, fmt.Errorf("ssh: parse listener port %s: %w", portStr, err)
	}
	return port, nil
}

type tunnelForwarder struct {
	ctx          context.Context
	cancel       context.CancelFunc
	listener     net.Listener
	localAddr    string
	assignedPort int
	wg           sync.WaitGroup
}

func startTunnelForwarder(ctx context.Context, client *sshlib.Client, localPort int) (*tunnelForwarder, error) {
	remoteListenAddr := net.JoinHostPort(constants.LocalhostIP, "0")
	listener, err := client.Listen("tcp", remoteListenAddr)
	if err != nil {
		return nil, fmt.Errorf("ssh: remote forward %s: %w", remoteListenAddr, err)
	}

	assignedPort, err := getListenerPort(listener)
	if err != nil {
		_ = listener.Close()
		return nil, err
	}

	fwdCtx, cancel := context.WithCancel(ctx)
	tf := &tunnelForwarder{
		ctx:          fwdCtx,
		cancel:       cancel,
		listener:     listener,
		localAddr:    net.JoinHostPort(constants.LocalhostIP, fmt.Sprintf("%d", localPort)),
		assignedPort: assignedPort,
	}

	tf.wg.Add(1)
	go tf.runAcceptLoop()

	return tf, nil
}

func (tf *tunnelForwarder) Close() {
	tf.cancel()
	_ = tf.listener.Close()
	tf.wg.Wait()
}

func (tf *tunnelForwarder) runAcceptLoop() {
	defer tf.wg.Done()
	var dialer net.Dialer

	for {
		remoteConn, err := tf.listener.Accept()
		if err != nil {
			return
		}

		tf.wg.Add(1)
		go func(rc net.Conn) {
			defer tf.wg.Done()
			defer rc.Close()

			localConn, err := dialer.DialContext(tf.ctx, "tcp", tf.localAddr)
			if err != nil {
				return
			}
			defer localConn.Close()

			tf.pipe(rc, localConn)
		}(remoteConn)
	}
}

func (tf *tunnelForwarder) pipe(c1, c2 net.Conn) {
	var once sync.Once
	closeBoth := func() {
		once.Do(func() {
			_ = c1.Close()
			_ = c2.Close()
		})
	}
	defer closeBoth()

	copyDone := make(chan struct{})
	watcherDone := make(chan struct{})

	go func() {
		defer close(watcherDone)
		select {
		case <-tf.ctx.Done():
			closeBoth()
		case <-copyDone:
		}
	}()

	var copyWg sync.WaitGroup
	copyWg.Add(2)

	go func() {
		defer copyWg.Done()
		defer closeBoth()
		_, _ = io.Copy(c1, c2)
	}()

	go func() {
		defer copyWg.Done()
		defer closeBoth()
		_, _ = io.Copy(c2, c1)
	}()

	copyWg.Wait()
	close(copyDone)
	<-watcherDone
}

// streamToHost injects the binary into one remote host via SSH and starts
// the operator through SSH remote forwarded tunnels. It sends exactly one streamResult to resultCh.
func streamToHost(
	ctx context.Context,
	target string,
	opts StreamHostOptions,
	resultCh chan<- streamResult,
) {
	start := time.Now()

	emit := func(status constants.StreamStatus, err error) {
		resultCh <- streamResult{
			Host:      target,
			Status:    status,
			SizeBytes: int64(len(opts.BinaryData)),
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

	if opts.GatewayHTTPPort == 0 {
		opts.GatewayHTTPPort = constants.Ports.OperatorHttp
	}
	if opts.GatewayHTTPSPort == 0 {
		opts.GatewayHTTPSPort = constants.Ports.OperatorHttps
	}

	r, err := ssh.ResolveHost(target, opts.SSHConfigPath, opts.Username, opts.SSHIdentityFile, opts.SSHUser)
	if err != nil {
		emit(constants.StreamStatusFailed, fmt.Errorf("ssh: resolve host: %w", err))
		return
	}

	// Pre-flight check if enabled
	if opts.PreFlightCheck {
		if err := preFlightCheck(ctx, r, opts.SSHAuthSock, opts.SSHPassphrase, opts.SSHKnownHostsPath, opts.DialTimeout); err != nil {
			emit(constants.StreamStatusFailed, err)
			return
		}
	}

	authMethods, err := ssh.BuildAuthMethods(r, opts.SSHAuthSock, opts.SSHPassphrase)
	if err != nil {
		emit(constants.StreamStatusFailed, fmt.Errorf("ssh: build auth: %w", err))
		return
	}
	if len(authMethods) == 0 {
		emit(constants.StreamStatusFailed, fmt.Errorf("ssh: %w", constants.ErrMCPRunShellCommandNoAuth))
		return
	}

	hostKeyCallback, cbErr := ssh.BuildHostKeyCallback(opts.SSHKnownHostsPath)
	if cbErr != nil {
		emit(constants.StreamStatusFailed, fmt.Errorf("ssh: host key callback: %w", cbErr))
		return
	}

	clientConfig := &sshlib.ClientConfig{
		User:            r.User,
		Auth:            authMethods,
		HostKeyCallback: hostKeyCallback,
		Timeout:         opts.DialTimeout,
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

		if err := ctx.Err(); err != nil {
			emit(constants.StreamStatusCancelled, constants.ErrSSHContextCancelled)
			return
		}

		client, err := ssh.DialSSH(ctx, r, clientConfig, addr)
		if err != nil {
			if errors.Is(err, context.Canceled) || errors.Is(ctx.Err(), context.Canceled) {
				emit(constants.StreamStatusCancelled, constants.ErrSSHContextCancelled)
				return
			}
			lastErr = err
			if retryCount < constants.SSHMaxRetries && isTransientError(err) {
				continue
			}
			emit(constants.StreamStatusFailed, fmt.Errorf("ssh: dial: %s (after %d retries): %w", addr, retryCount, err))
			return
		}

		stopKeepalive := ssh.StartKeepalive(ctx, client, constants.SSHKeepaliveInterval, constants.SSHKeepaliveMaxMissed)

		remotePath, err := generateRemotePath()
		if err != nil {
			stopKeepalive()
			_ = client.Close()
			emit(constants.StreamStatusFailed, err)
			return
		}

		cleanup := func(httpFwd, httpsFwd *tunnelForwarder) {
			stopKeepalive()
			if httpFwd != nil {
				httpFwd.Close()
			}
			if httpsFwd != nil {
				httpsFwd.Close()
			}
			removeRemoteBinary(client, remotePath)
			_ = client.Close()
		}

		// Upload binary via SFTP
		if err := uploadBinaryViaSFTP(client, opts.BinaryData, remotePath); err != nil {
			lastErr = err
			cleanup(nil, nil)
			if retryCount < constants.SSHMaxRetries && isTransientError(err) {
				continue
			}
			emit(constants.StreamStatusFailed, fmt.Errorf("ssh: upload binary (after %d retries): %w", retryCount, err))
			return
		}

		// Setup tunnels for HTTP and HTTPS Gateway ports
		httpFwd, err := startTunnelForwarder(ctx, client, opts.GatewayHTTPPort)
		if err != nil {
			lastErr = err
			cleanup(nil, nil)
			if retryCount < constants.SSHMaxRetries && isTransientError(err) {
				continue
			}
			emit(constants.StreamStatusFailed, fmt.Errorf("ssh: start http tunnel: %w", err))
			return
		}

		httpsFwd, err := startTunnelForwarder(ctx, client, opts.GatewayHTTPSPort)
		if err != nil {
			lastErr = err
			cleanup(httpFwd, nil)
			if retryCount < constants.SSHMaxRetries && isTransientError(err) {
				continue
			}
			emit(constants.StreamStatusFailed, fmt.Errorf("ssh: start https tunnel: %w", err))
			return
		}

		session, err := client.NewSession()
		if err != nil {
			lastErr = err
			cleanup(httpFwd, httpsFwd)
			if retryCount < constants.SSHMaxRetries && isTransientError(err) {
				continue
			}
			emit(constants.StreamStatusFailed, fmt.Errorf("ssh: session (after %d retries): %w", retryCount, err))
			return
		}

		session.Stdin = bytes.NewReader(nil)

		// Capture stdout+stderr (bounded)
		stderrBuf := &boundedBuffer{limit: constants.SSHCaptureMaxBytes}
		stdoutBuf := &boundedBuffer{limit: constants.SSHCaptureMaxBytes}
		session.Stderr = stderrBuf
		session.Stdout = stdoutBuf

		remoteCmd := buildOperatorCommand(remotePath, httpFwd.assignedPort, httpsFwd.assignedPort, opts.NoGit)

		var runWg sync.WaitGroup
		runDone := make(chan struct{})
		runWg.Add(1)
		go func() {
			defer runWg.Done()
			select {
			case <-ctx.Done():
				_ = session.Signal(sshlib.SIGHUP)
				_ = session.Close()
			case <-runDone:
			}
		}()

		err = session.Run(remoteCmd)
		close(runDone)
		runWg.Wait()
		_ = session.Close()

		cleanup(httpFwd, httpsFwd)

		if ctx.Err() != nil {
			emit(constants.StreamStatusCancelled, constants.ErrSSHContextCancelled)
			return
		}

		if err != nil {
			var exitErr *sshlib.ExitError
			if errors.As(err, &exitErr) {
				msg := fmt.Errorf("ssh: exit code %d", exitErr.ExitStatus())
				if tail := strings.TrimSpace(stderrBuf.String()); tail != "" {
					msg = fmt.Errorf("%w: %s", msg, tail)
				} else if tail := strings.TrimSpace(stdoutBuf.String()); tail != "" {
					msg = fmt.Errorf("%w: %s", msg, tail)
				}
				emit(constants.StreamStatusExited, msg)
				return
			}
			lastErr = err
			if retryCount < constants.SSHMaxRetries && isTransientError(err) {
				continue
			}
			msg := fmt.Errorf("ssh: run: %w", err)
			if tail := strings.TrimSpace(stderrBuf.String()); tail != "" {
				msg = fmt.Errorf("%w: %s", msg, tail)
			}
			emit(constants.StreamStatusFailed, msg)
			return
		}

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
