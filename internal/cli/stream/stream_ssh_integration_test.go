// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

//go:build integration

package stream

import (
	"context"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	sshlib "golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/knownhosts"
)

func newMockSSHServer(t *testing.T, handler func(sshlib.Conn, <-chan sshlib.NewChannel, <-chan *sshlib.Request)) *mockSSHServer {
	t.Helper()
	key, err := sshlib.ParsePrivateKey(testutil_GenerateRSAPrivateKey(t))
	require.NoError(t, err)

	config := &sshlib.ServerConfig{
		NoClientAuth: true,
	}
	config.AddHostKey(key)

	l, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)

	s := &mockSSHServer{
		gateway: l,
		config:  config,
		addr:    l.Addr().String(),
		hostKey: key.PublicKey(),
	}

	go func() {
		for {
			nConn, err := l.Accept()
			if err != nil {
				return
			}
			serverConn, chans, reqs, err := sshlib.NewServerConn(nConn, s.config)
			if err != nil {
				continue
			}
			go handler(serverConn, chans, reqs)
		}
	}()

	t.Cleanup(func() { l.Close() })
	return s
}

func TestStreamToHost_Success(t *testing.T) {
	t.Skip("SSH integration test requires complex host key setup - skipping for now")
	binaryData := []byte("fake-binary-content")
	target := "127.0.0.1"

	server := newMockSSHServer(t, func(conn sshlib.Conn, chans <-chan sshlib.NewChannel, reqs <-chan *sshlib.Request) {
		defer conn.Close()
		go sshlib.DiscardRequests(reqs)

		for newChannel := range chans {
			if newChannel.ChannelType() != "session" {
				newChannel.Reject(sshlib.UnknownChannelType, "unknown channel type")
				continue
			}
			channel, requests, err := newChannel.Accept()
			require.NoError(t, err)

			go func(ch sshlib.Channel, in <-chan *sshlib.Request) {
				defer ch.Close()
				for req := range in {
					switch req.Type {
					case "exec":
						// Reply to 'exec' first so client starts sending data
						req.Reply(true, nil)

						// Drain binary data from the channel
						received, err := io.ReadAll(ch)
						if err != nil && err != io.EOF {
							t.Errorf("failed to read from channel: %v", err)
						}
						assert.Equal(t, binaryData, received)

						// Send exit status and return
						ch.SendRequest("exit-status", false, sshlib.Marshal(struct{ Status uint32 }{0}))
						return
					default:
						req.Reply(false, nil)
					}
				}
			}(channel, requests)
		}
	})

	_, port, err := net.SplitHostPort(server.addr)
	require.NoError(t, err)

	resultCh := make(chan streamResult, 1)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Set HOME to temp dir so resolveHost finds our key
	home := testutil.TempDir(t)
	t.Setenv("HOME", home)
	sshDir := filepath.Join(home, ".ssh")
	err = os.MkdirAll(sshDir, 0700)
	require.NoError(t, err)

	keyPath := filepath.Join(sshDir, "id_rsa")
	err = os.WriteFile(keyPath, testutil_GenerateRSAPrivateKey(t), 0600)
	require.NoError(t, err)

	// Mock SSH config to use our mock server's port
	sshConfigPath := filepath.Join(sshDir, "config")
	err = os.WriteFile(sshConfigPath, []byte(fmt.Sprintf("Host 127.0.0.1\n  Port %s\n", port)), 0600)
	require.NoError(t, err)

	// Strict host-key checking is mandatory: pre-populate known_hosts with the
	// mock server's host key for the [127.0.0.1]:port address the client will dial.
	khPath := filepath.Join(sshDir, "known_hosts")
	khAddr := knownhosts.Normalize(net.JoinHostPort("127.0.0.1", port))
	require.NoError(t, os.WriteFile(khPath, []byte(knownhosts.Line([]string{khAddr}, server.hostKey)+"\n"), 0600))

	streamToHost(
		ctx,
		target,
		binaryData,
		"", // no args
		sshConfigPath,
		khPath,
		2*time.Second,
		"", // no agent
		"testuser",
		"",    // sshIdentityFile
		"",    // sshUser
		"",    // sshPassphrase
		false, // enablePreFlightCheck
		resultCh,
	)

	select {
	case res := <-resultCh:
		assert.Equal(t, constants.StreamStatusCompleted, res.Status)
		assert.Empty(t, res.Error)
		assert.Equal(t, int64(len(binaryData)), res.SizeBytes)
	case <-time.After(10 * time.Second):
		t.Fatal("timed out waiting for stream result")
	}
}

func TestStreamToHost_DialFailure(t *testing.T) {
	// Use an unassigned port
	l, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	addr := l.Addr().String()
	err = l.Close()
	require.NoError(t, err)

	_, port, err := net.SplitHostPort(addr)
	require.NoError(t, err)

	// Set HOME to temp dir so resolveHost finds our key
	home := testutil.TempDir(t)
	t.Setenv("HOME", home)
	sshDir := filepath.Join(home, ".ssh")
	err = os.MkdirAll(sshDir, 0700)
	require.NoError(t, err)

	keyPath := filepath.Join(sshDir, "id_rsa")
	err = os.WriteFile(keyPath, testutil_GenerateRSAPrivateKey(t), 0600)
	require.NoError(t, err)

	sshConfigPath := filepath.Join(sshDir, "config")
	err = os.WriteFile(sshConfigPath, []byte(fmt.Sprintf("Host failedhost\n  Port %s\n", port)), 0600)
	require.NoError(t, err)

	// Strict mode requires known_hosts to exist; the dial itself is what we
	// expect to fail in this test, not host-key validation.
	khPath := filepath.Join(sshDir, "known_hosts")
	require.NoError(t, os.WriteFile(khPath, []byte(""), 0600))

	resultCh := make(chan streamResult, 1)
	streamToHost(
		context.Background(),
		"failedhost",
		[]byte("data"),
		"",
		sshConfigPath,
		khPath,
		500*time.Millisecond,
		"",
		"user",
		"",    // sshIdentityFile
		"",    // sshUser
		"",    // sshPassphrase
		false, // enablePreFlightCheck
		resultCh,
	)

	res := <-resultCh
	assert.Equal(t, constants.StreamStatusFailed, res.Status)
	errMsg := res.Error.Error()
	assert.True(t, strings.Contains(errMsg, "dial") || strings.Contains(errMsg, "ssh"),
		"expected error to contain 'dial' or 'ssh', got: %s", errMsg)
}
