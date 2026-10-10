// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package ssh

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"errors"
	"net"
	"os/exec"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	sshlib "golang.org/x/crypto/ssh"

	"github.com/g8e-ai/g8e/v2/internal/constants"
)

func newTestSigner(t *testing.T) sshlib.Signer {
	t.Helper()
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	signer, err := sshlib.NewSignerFromKey(priv)
	require.NoError(t, err)
	return signer
}

// startTestSSHServer creates an in-process SSH server listening on a loopback port.
func startTestSSHServer(t *testing.T, signer sshlib.Signer) (string, func()) {
	t.Helper()
	config := &sshlib.ServerConfig{
		NoClientAuth: true,
	}
	config.AddHostKey(signer)

	l, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)

	stopCh := make(chan struct{})
	var wg sync.WaitGroup

	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			conn, err := l.Accept()
			if err != nil {
				select {
				case <-stopCh:
					return
				default:
					return
				}
			}
			go func(c net.Conn) {
				sConn, chans, reqs, err := sshlib.NewServerConn(c, config)
				if err != nil {
					_ = c.Close()
					return
				}
				defer sConn.Close()

				go func() {
					for req := range reqs {
						if req.Type == constants.SSHKeepaliveRequestType && req.WantReply {
							_ = req.Reply(true, nil)
						} else if req.WantReply {
							_ = req.Reply(false, nil)
						}
					}
				}()

				for newChan := range chans {
					_ = newChan.Reject(sshlib.Prohibited, "sessions not supported in test server")
				}
			}(conn)
		}
	}()

	cleanup := func() {
		close(stopCh)
		_ = l.Close()
		wg.Wait()
	}

	return l.Addr().String(), cleanup
}

func startTestProxyConn(t *testing.T, shellCommand string) *ProxyConn {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	t.Cleanup(cancel)

	cmd := ProxyCommand(ctx, shellCommand)
	stdin, err := cmd.StdinPipe()
	require.NoError(t, err)
	stdout, err := cmd.StdoutPipe()
	require.NoError(t, err)
	require.NoError(t, cmd.Start())
	return NewProxyConn(cmd, stdin, stdout, "proxied:22")
}

func TestProxyConn(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "cat")
	stdin, err := cmd.StdinPipe()
	require.NoError(t, err)
	stdout, err := cmd.StdoutPipe()
	require.NoError(t, err)
	require.NoError(t, cmd.Start())

	conn := NewProxyConn(cmd, stdin, stdout, "test:22")

	// Test Write
	_, err = conn.Write([]byte("test"))
	require.NoError(t, err)

	// Test Read
	buf := make([]byte, 4)
	n, err := conn.Read(buf)
	require.NoError(t, err)
	assert.Equal(t, 4, n)
	assert.Equal(t, []byte("test"), buf)

	// Test Close
	require.NoError(t, conn.Close())

	// Test addresses
	assert.Equal(t, string(constants.NetworkProtocolTCP), conn.LocalAddr().Network())
	assert.Equal(t, constants.SSHProxyAddrLabel, conn.LocalAddr().String())
	assert.Equal(t, string(constants.NetworkProtocolTCP), conn.RemoteAddr().Network())
	assert.Equal(t, "test:22", conn.RemoteAddr().String())

	// Test deadline methods (should be no-ops)
	assert.NoError(t, conn.SetDeadline(time.Now()))
	assert.NoError(t, conn.SetReadDeadline(time.Now()))
	assert.NoError(t, conn.SetWriteDeadline(time.Now()))
}

func TestProxyConn_CloseIsSafeForConcurrentCallers(t *testing.T) {
	tests := []struct {
		name         string
		shellCommand string
		wantErr      bool
	}{
		{name: "proxy command exits cleanly once stdin closes", shellCommand: "cat", wantErr: false},
		{name: "proxy command exits non-zero", shellCommand: "exit 3", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			conn := startTestProxyConn(t, tt.shellCommand)

			const callers = 8
			errs := make([]error, callers)
			var wg sync.WaitGroup
			for i := range errs {
				wg.Add(1)
				go func() {
					defer wg.Done()
					errs[i] = conn.Close()
				}()
			}
			joined := make(chan struct{})
			go func() {
				wg.Wait()
				close(joined)
			}()
			select {
			case <-joined:
			case <-time.After(10 * time.Second):
				t.Fatal("concurrent Close calls did not all return; Close must be idempotent")
			}

			for i, err := range errs {
				if !tt.wantErr {
					assert.NoError(t, err, "caller %d", i)
					continue
				}
				require.Error(t, err, "caller %d", i)
				assert.Contains(t, err.Error(), "ssh: proxy command wait")
				assert.Same(t, errs[0], err, "every caller observes the first close's result")
			}
			assert.Equal(t, errs[0], conn.Close(), "a later Close repeats the original result")
		})
	}
}

func TestDialSSH_NilConfig(t *testing.T) {
	ctx := context.Background()
	client, err := DialSSH(ctx, HostConfig{Hostname: "127.0.0.1", Port: "22"}, nil, "127.0.0.1:22")
	assert.Nil(t, client)
	require.Error(t, err)
	assert.True(t, errors.Is(err, constants.ErrSSHNilClientConfig))
}

func TestDialSSH_ContextCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	clientConfig := &sshlib.ClientConfig{
		User:            "testuser",
		HostKeyCallback: sshlib.InsecureIgnoreHostKey(), //nolint:gosec // Unit test only
		Timeout:         time.Second,
	}

	client, err := DialSSH(ctx, HostConfig{Hostname: "127.0.0.1", Port: "22"}, clientConfig, "127.0.0.1:22")
	assert.Nil(t, client)
	require.Error(t, err)
	assert.True(t, errors.Is(err, context.Canceled))
}

func TestDialSSH_ProxyCommandThatExitsImmediatelyFailsHandshake(t *testing.T) {
	signer := newTestSigner(t)
	config := &sshlib.ClientConfig{
		User:            "testuser",
		HostKeyCallback: sshlib.FixedHostKey(signer.PublicKey()),
		Timeout:         time.Second,
	}

	client, err := DialSSH(context.Background(), HostConfig{ProxyCommand: "exit 1", Hostname: "h", Port: "22"}, config, "h:22")
	assert.Nil(t, client)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "ssh: client connection")
}

func TestDialSSH_DirectTCP_Success(t *testing.T) {
	signer := newTestSigner(t)
	serverAddr, cleanup := startTestSSHServer(t, signer)
	defer cleanup()

	host, port, err := net.SplitHostPort(serverAddr)
	require.NoError(t, err)

	clientConfig := &sshlib.ClientConfig{
		User:            "testuser",
		HostKeyCallback: sshlib.FixedHostKey(signer.PublicKey()),
		Timeout:         5 * time.Second,
	}

	r := HostConfig{
		Hostname: host,
		Port:     port,
	}

	client, err := DialSSH(context.Background(), r, clientConfig, serverAddr)
	require.NoError(t, err)
	require.NotNil(t, client)
	defer client.Close()

	// Verify keepalive request
	_, _, err = client.SendRequest(constants.SSHKeepaliveRequestType, true, nil)
	assert.NoError(t, err)
}

func TestStartKeepalive(t *testing.T) {
	signer := newTestSigner(t)
	serverAddr, cleanup := startTestSSHServer(t, signer)
	defer cleanup()

	host, port, err := net.SplitHostPort(serverAddr)
	require.NoError(t, err)

	clientConfig := &sshlib.ClientConfig{
		User:            "testuser",
		HostKeyCallback: sshlib.FixedHostKey(signer.PublicKey()),
		Timeout:         5 * time.Second,
	}

	client, err := DialSSH(context.Background(), HostConfig{Hostname: host, Port: port}, clientConfig, serverAddr)
	require.NoError(t, err)
	defer client.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	stopKeepalive := StartKeepalive(ctx, client, 50*time.Millisecond, 2)
	time.Sleep(150 * time.Millisecond)
	stopKeepalive()
	// Calling stop multiple times must be safe
	stopKeepalive()
}

func TestClientPool(t *testing.T) {
	signer := newTestSigner(t)
	serverAddr, cleanup := startTestSSHServer(t, signer)
	defer cleanup()

	host, port, err := net.SplitHostPort(serverAddr)
	require.NoError(t, err)

	clientConfig := &sshlib.ClientConfig{
		User:            "testuser",
		HostKeyCallback: sshlib.FixedHostKey(signer.PublicKey()),
		Timeout:         5 * time.Second,
	}

	r := HostConfig{
		Hostname: host,
		Port:     port,
	}

	pool := NewClientPool()
	defer func() {
		_ = pool.Close()
	}()

	client1, err := pool.Dial(context.Background(), r, clientConfig, serverAddr)
	require.NoError(t, err)
	require.NotNil(t, client1)

	// Dialing again should return the same client
	client2, err := pool.Dial(context.Background(), r, clientConfig, serverAddr)
	require.NoError(t, err)
	assert.Same(t, client1, client2)

	// Closing pool should close client
	require.NoError(t, pool.Close())
	assert.Empty(t, pool.clients)
}
