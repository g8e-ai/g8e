// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package ssh

import (
	"context"
	"fmt"
	"io"
	"net"
	"os/exec"
	"strings"
	"sync"
	"time"

	sshlib "golang.org/x/crypto/ssh"

	"github.com/g8e-ai/g8e/v2/internal/constants"
)

// Dialer connects to a remote SSH server using the specified HostConfig and ClientConfig.
type Dialer func(ctx context.Context, r HostConfig, clientConfig *sshlib.ClientConfig, addr string) (*sshlib.Client, error)

// DialSSH connects to the target SSH server using the specified HostConfig and
// ClientConfig. It supports both direct TCP connections (with keepalive enabled)
// and ProxyCommand connections. DialSSH respects context cancellation and
// clientConfig.Timeout.
func DialSSH(ctx context.Context, r HostConfig, clientConfig *sshlib.ClientConfig, addr string) (*sshlib.Client, error) {
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("ssh: dial: %w", err)
	}
	if clientConfig == nil {
		return nil, fmt.Errorf("ssh: dial: %w", constants.ErrSSHNilClientConfig)
	}

	if addr == "" {
		port := r.Port
		if port == "" {
			port = "22"
		}
		addr = net.JoinHostPort(r.Hostname, port)
	}

	dialCtx := ctx
	if clientConfig.Timeout > 0 {
		var cancel context.CancelFunc
		dialCtx, cancel = context.WithTimeout(ctx, clientConfig.Timeout)
		defer cancel()
	}

	var conn net.Conn
	var err error

	if r.ProxyCommand != "" {
		proxyCmd := strings.ReplaceAll(r.ProxyCommand, "%h", r.Hostname)
		proxyCmd = strings.ReplaceAll(proxyCmd, "%p", r.Port)

		cmd := ProxyCommand(dialCtx, proxyCmd)
		stdin, err := cmd.StdinPipe()
		if err != nil {
			return nil, fmt.Errorf("ssh: proxy stdin pipe: %w", err)
		}
		stdout, err := cmd.StdoutPipe()
		if err != nil {
			_ = stdin.Close()
			return nil, fmt.Errorf("ssh: proxy stdout pipe: %w", err)
		}
		if err := cmd.Start(); err != nil {
			_ = stdin.Close()
			_ = stdout.Close()
			return nil, fmt.Errorf("ssh: start proxy: %w", err)
		}

		conn = NewProxyConn(cmd, stdin, stdout, addr)
	} else {
		var d net.Dialer
		if clientConfig.Timeout > 0 {
			d.Timeout = clientConfig.Timeout
		}
		conn, err = d.DialContext(dialCtx, string(constants.NetworkProtocolTCP), addr)
		if err != nil {
			return nil, fmt.Errorf("ssh: dial: %w", err)
		}
		if tcpConn, ok := conn.(*net.TCPConn); ok {
			if err := tcpConn.SetKeepAlive(true); err != nil {
				_ = conn.Close()
				return nil, fmt.Errorf("ssh: set keepalive: %w", err)
			}
			if err := tcpConn.SetKeepAlivePeriod(constants.SSHKeepaliveInterval); err != nil {
				_ = conn.Close()
				return nil, fmt.Errorf("ssh: set keepalive period: %w", err)
			}
		}
	}

	handshakeDone := make(chan struct{})
	defer close(handshakeDone)

	go func() {
		select {
		case <-handshakeDone:
		case <-dialCtx.Done():
			_ = conn.Close()
		}
	}()

	sshConn, chans, reqs, err := sshlib.NewClientConn(conn, addr, clientConfig)
	if err != nil {
		_ = conn.Close()
		if dialCtx.Err() != nil {
			return nil, fmt.Errorf("ssh: client connection: %w", dialCtx.Err())
		}
		return nil, fmt.Errorf("ssh: client connection: %w", err)
	}

	return sshlib.NewClient(sshConn, chans, reqs), nil
}

// StartKeepalive initiates background SSH keepalive requests at regular intervals.
// If maxMissed consecutive requests fail, the client is closed.
// It returns a stop function that safely stops the background goroutine.
func StartKeepalive(ctx context.Context, client *sshlib.Client, interval time.Duration, maxMissed int) func() {
	if interval <= 0 {
		interval = constants.SSHKeepaliveInterval
	}
	if maxMissed <= 0 {
		maxMissed = constants.SSHKeepaliveMaxMissed
	}

	done := make(chan struct{})
	var once sync.Once
	stop := func() {
		once.Do(func() {
			close(done)
		})
	}

	if client == nil {
		return stop
	}

	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		missedCount := 0
		for {
			select {
			case <-ctx.Done():
				return
			case <-done:
				return
			case <-ticker.C:
				_, _, err := client.SendRequest(constants.SSHKeepaliveRequestType, true, nil)
				if err != nil {
					missedCount++
					if missedCount >= maxMissed {
						_ = client.Close()
						return
					}
				} else {
					missedCount = 0
				}
			}
		}
	}()

	return stop
}

// ProxyConn wraps a proxy command's stdin/stdout as a net.Conn.
type ProxyConn struct {
	stdin  io.WriteCloser
	stdout io.Reader
	cmd    *exec.Cmd
	addr   string

	// golang.org/x/crypto/ssh closes the net.Conn from more than one goroutine
	// (connection teardown and the key-exchange loop), and exec.Cmd.Wait must
	// run exactly once, so Close is idempotent and safe for concurrent use.
	closeOnce sync.Once
	closeErr  error
}

// NewProxyConn creates a net.Conn implementation backed by a proxy command.
func NewProxyConn(cmd *exec.Cmd, stdin io.WriteCloser, stdout io.Reader, addr string) *ProxyConn {
	return &ProxyConn{
		stdin:  stdin,
		stdout: stdout,
		cmd:    cmd,
		addr:   addr,
	}
}

func (c *ProxyConn) Read(b []byte) (int, error) {
	return c.stdout.Read(b)
}

func (c *ProxyConn) Write(b []byte) (int, error) {
	return c.stdin.Write(b)
}

func (c *ProxyConn) Close() error {
	c.closeOnce.Do(func() {
		if c.stdin != nil {
			_ = c.stdin.Close()
		}
		if c.cmd != nil {
			if err := c.cmd.Wait(); err != nil {
				c.closeErr = fmt.Errorf("ssh: proxy command wait: %w", err)
			}
		}
	})
	return c.closeErr
}

func (c *ProxyConn) LocalAddr() net.Addr {
	return &proxyAddr{addr: constants.SSHProxyAddrLabel}
}

func (c *ProxyConn) RemoteAddr() net.Addr {
	return &proxyAddr{addr: c.addr}
}

func (c *ProxyConn) SetDeadline(t time.Time) error {
	return nil
}

func (c *ProxyConn) SetReadDeadline(t time.Time) error {
	return nil
}

func (c *ProxyConn) SetWriteDeadline(t time.Time) error {
	return nil
}

type proxyAddr struct {
	addr string
}

func (a *proxyAddr) Network() string { return string(constants.NetworkProtocolTCP) }
func (a *proxyAddr) String() string  { return a.addr }
