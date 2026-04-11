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
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/g8e-ai/g8e/components/vsa/constants"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/ssh"
)

// mockSSHServer is a minimal SSH server for testing streamToHost.
type mockSSHServer struct {
	listener net.Listener
	config   *ssh.ServerConfig
	addr     string
}

func newMockSSHServer(t *testing.T, handler func(ssh.Conn, <-chan ssh.NewChannel, <-chan *ssh.Request)) *mockSSHServer {
	t.Helper()
	key, err := ssh.ParsePrivateKey(testutil_GenerateRSAPrivateKey(t))
	require.NoError(t, err)

	config := &ssh.ServerConfig{
		NoClientAuth: true,
	}
	config.AddHostKey(key)

	l, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)

	s := &mockSSHServer{
		listener: l,
		config:   config,
		addr:     l.Addr().String(),
	}

	go func() {
		for {
			nConn, err := l.Accept()
			if err != nil {
				return
			}
			serverConn, chans, reqs, err := ssh.NewServerConn(nConn, s.config)
			if err != nil {
				continue
			}
			go handler(serverConn, chans, reqs)
		}
	}()

	t.Cleanup(func() { l.Close() })
	return s
}

func testutil_GenerateRSAPrivateKey(t *testing.T) []byte {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)

	keyPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(key),
	})
	return keyPEM
}

func TestStreamToHost_Success(t *testing.T) {
	binaryData := []byte("fake-binary-content")
	target := "127.0.0.1"

	server := newMockSSHServer(t, func(conn ssh.Conn, chans <-chan ssh.NewChannel, reqs <-chan *ssh.Request) {
		defer conn.Close()
		go ssh.DiscardRequests(reqs)

		for newChannel := range chans {
			if newChannel.ChannelType() != "session" {
				newChannel.Reject(ssh.UnknownChannelType, "unknown channel type")
				continue
			}
			channel, requests, err := newChannel.Accept()
			require.NoError(t, err)

			go func(ch ssh.Channel, in <-chan *ssh.Request) {
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
						ch.SendRequest("exit-status", false, ssh.Marshal(struct{ Status uint32 }{0}))
						return
					default:
						req.Reply(false, nil)
					}
				}
			}(channel, requests)
		}
	})

	_, port, _ := net.SplitHostPort(server.addr)

	resultCh := make(chan streamResult, 1)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Set HOME to temp dir so resolveHost finds our key
	home := t.TempDir()
	t.Setenv("HOME", home)
	sshDir := filepath.Join(home, ".ssh")
	err := os.MkdirAll(sshDir, 0700)
	require.NoError(t, err)

	keyPath := filepath.Join(sshDir, "id_rsa")
	err = os.WriteFile(keyPath, testutil_GenerateRSAPrivateKey(t), 0600)
	require.NoError(t, err)

	// Mock SSH config to use our mock server's port
	sshConfigPath := filepath.Join(sshDir, "config")
	err = os.WriteFile(sshConfigPath, []byte(fmt.Sprintf("Host 127.0.0.1\n  Port %s\n", port)), 0600)
	require.NoError(t, err)

	streamToHost(
		ctx,
		target,
		binaryData,
		"", // no args
		sshConfigPath,
		2*time.Second,
		"", // no agent
		"testuser",
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

func TestStreamToHost_ContextCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	resultCh := make(chan streamResult, 1)
	streamToHost(
		ctx,
		"127.0.0.1",
		[]byte("data"),
		"",
		"",
		2*time.Second,
		"",
		"user",
		resultCh,
	)

	res := <-resultCh
	assert.Equal(t, constants.StreamStatusCancelled, res.Status)
}

func TestStreamToHost_DialFailure(t *testing.T) {
	// Use an unassigned port
	l, _ := net.Listen("tcp", "127.0.0.1:0")
	addr := l.Addr().String()
	l.Close()

	_, port, _ := net.SplitHostPort(addr)

	// Set HOME to temp dir so resolveHost finds our key
	home := t.TempDir()
	t.Setenv("HOME", home)
	sshDir := filepath.Join(home, ".ssh")
	err := os.MkdirAll(sshDir, 0700)
	require.NoError(t, err)

	keyPath := filepath.Join(sshDir, "id_rsa")
	err = os.WriteFile(keyPath, testutil_GenerateRSAPrivateKey(t), 0600)
	require.NoError(t, err)

	sshConfigPath := filepath.Join(sshDir, "config")
	os.WriteFile(sshConfigPath, []byte(fmt.Sprintf("Host failedhost\n  Port %s\n", port)), 0600)

	resultCh := make(chan streamResult, 1)
	streamToHost(
		context.Background(),
		"failedhost",
		[]byte("data"),
		"",
		sshConfigPath,
		500*time.Millisecond,
		"",
		"user",
		resultCh,
	)

	res := <-resultCh
	assert.Equal(t, constants.StreamStatusFailed, res.Status)
	assert.Contains(t, res.Error, "dial")
}
