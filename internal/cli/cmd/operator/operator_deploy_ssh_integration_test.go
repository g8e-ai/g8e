// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

//go:build integration

package operatorcmd

// Hermetic tests for the `operator deploy` SSH transport against an in-process
// SSH server. The server serves the SFTP subsystem for real and answers each
// `<agent> operator deploy-host` exec by running ExecuteDeployHost in process,
// so the copy, the agent command string, and the JSON contract are exercised
// end to end without mocking any internal service.

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/json"
	"io"
	"net"
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/pkg/sftp"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	sshlib "golang.org/x/crypto/ssh"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/models"
	"github.com/g8e-ai/g8e/v2/internal/services/fs"
	"github.com/g8e-ai/g8e/v2/internal/testutil"
)

const deployAgentSuffix = " operator deploy-host"

// deploySSHServer is an in-process SSH server that records every exec command.
type deploySSHServer struct {
	mu       sync.Mutex
	commands []string
}

func (s *deploySSHServer) recordedCommands() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.commands...)
}

// startDeploySSHServer listens on loopback and returns the address and host key
// clients must pin.
func startDeploySSHServer(t *testing.T) (string, sshlib.PublicKey, *deploySSHServer) {
	t.Helper()
	hostSigner := newDeployHostSigner(t)

	config := &sshlib.ServerConfig{NoClientAuth: true}
	config.AddHostKey(hostSigner)

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	t.Cleanup(func() { _ = ln.Close() })

	srv := &deploySSHServer{}
	go func() {
		for {
			raw, err := ln.Accept()
			if err != nil {
				return
			}
			go srv.serve(raw, config)
		}
	}()
	return ln.Addr().String(), hostSigner.PublicKey(), srv
}

func newDeployHostSigner(t *testing.T) sshlib.Signer {
	t.Helper()
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	signer, err := sshlib.NewSignerFromKey(priv)
	require.NoError(t, err)
	return signer
}

func (s *deploySSHServer) serve(raw net.Conn, config *sshlib.ServerConfig) {
	conn, chans, reqs, err := sshlib.NewServerConn(raw, config)
	if err != nil {
		_ = raw.Close()
		return
	}
	defer conn.Close()
	go sshlib.DiscardRequests(reqs)

	for newChannel := range chans {
		if newChannel.ChannelType() != "session" {
			_ = newChannel.Reject(sshlib.UnknownChannelType, "only session channels are supported")
			continue
		}
		channel, requests, err := newChannel.Accept()
		if err != nil {
			return
		}
		go s.handleSession(channel, requests)
	}
}

func (s *deploySSHServer) handleSession(channel sshlib.Channel, requests <-chan *sshlib.Request) {
	defer channel.Close()
	for req := range requests {
		switch req.Type {
		case "subsystem":
			var payload struct{ Subsystem string }
			if err := sshlib.Unmarshal(req.Payload, &payload); err != nil || payload.Subsystem != "sftp" {
				if req.WantReply {
					_ = req.Reply(false, nil)
				}
				return
			}
			_ = req.Reply(true, nil)
			server, err := sftp.NewServer(channel)
			if err == nil {
				_ = server.Serve()
			}
			return

		case "exec":
			var payload struct{ Command string }
			if err := sshlib.Unmarshal(req.Payload, &payload); err != nil {
				if req.WantReply {
					_ = req.Reply(false, nil)
				}
				return
			}
			_ = req.Reply(true, nil)
			stdin, _ := io.ReadAll(channel)
			s.mu.Lock()
			s.commands = append(s.commands, payload.Command)
			s.mu.Unlock()

			status := runDeployHostExec(payload.Command, stdin, channel, channel.Stderr())
			_, _ = channel.SendRequest("exit-status", false, sshlib.Marshal(struct{ Status uint32 }{status}))
			return

		default:
			if req.WantReply {
				_ = req.Reply(false, nil)
			}
		}
	}
}

// runDeployHostExec mirrors the `operator deploy-host` command: it decodes the
// request from stdin, runs the real agent, and writes the response to stdout.
// Failures go to stderr with a non-zero exit status, as the cobra command does.
func runDeployHostExec(command string, stdin []byte, stdout, stderr io.Writer) uint32 {
	if !strings.HasSuffix(command, deployAgentSuffix) {
		_, _ = io.WriteString(stderr, "unexpected command: "+command)
		return 127
	}
	var req models.DeployHostRequest
	if err := json.Unmarshal(stdin, &req); err != nil {
		_, _ = io.WriteString(stderr, "decode deploy-host request: "+err.Error())
		return 1
	}
	resp, err := ExecuteDeployHost(context.Background(), req, fs.NewRuntimeFileService)
	if err != nil {
		_, _ = io.WriteString(stderr, err.Error())
		return 1
	}
	resp.Success = true
	if err := json.NewEncoder(stdout).Encode(resp); err != nil {
		_, _ = io.WriteString(stderr, "encode deploy-host response: "+err.Error())
		return 1
	}
	return 0
}

// dialDeploySSHClient builds the production remote client shape over an
// in-process connection. The caller owns Close.
func dialDeploySSHClient(t *testing.T, addr string, hostKey sshlib.PublicKey) *sshRemoteDeployClient {
	t.Helper()
	clientConfig := &sshlib.ClientConfig{
		User:            "deploy-test",
		HostKeyCallback: sshlib.FixedHostKey(hostKey),
		Timeout:         5 * time.Second,
	}
	client, err := sshlib.Dial("tcp", addr, clientConfig)
	require.NoError(t, err)

	sftpClient, err := sftp.NewClient(client)
	if err != nil {
		_ = client.Close()
		require.NoError(t, err)
	}
	wd, err := sftpClient.Getwd()
	require.NoError(t, err)

	return &sshRemoteDeployClient{
		client:     client,
		sftpClient: sftpClient,
		host:       addr,
		isWSL:      isWindowsPath(wd),
		workingDir: wd,
	}
}

func writeDeploySourceBinary(t *testing.T) (string, []byte) {
	t.Helper()
	content := []byte("fake-g8e-binary-for-transport-test")
	src := filepath.Join(testutil.TempDir(t), "g8e")
	require.NoError(t, os.WriteFile(src, content, constants.PermFilePrivate))
	return src, content
}

func TestDeploySSH_UploadBinaryAndAgentRoundTrip(t *testing.T) {
	addr, hostKey, srv := startDeploySSHServer(t)
	client := dialDeploySSHClient(t, addr, hostKey)
	t.Cleanup(func() { _ = client.Close() })

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	src, content := writeDeploySourceBinary(t)
	remoteDir := testutil.TempDir(t)

	binDir, err := client.UploadBinary(ctx, src, remoteDir)
	require.NoError(t, err)
	require.Equal(t, path.Join(remoteDir, constants.DeployBinDirname), binDir)

	installed := filepath.Join(binDir, "g8e")
	got, err := os.ReadFile(installed)
	require.NoError(t, err)
	assert.Equal(t, content, got, "uploaded bytes must match the source binary")

	info, err := os.Stat(installed)
	require.NoError(t, err)
	assert.NotZero(t, info.Mode().Perm()&0o111, "installed binary must be executable")

	_, err = os.Stat(filepath.Join(binDir, "g8e.new"))
	assert.True(t, os.IsNotExist(err), "staging name must be renamed away, got %v", err)

	dest := filepath.Join(remoteDir, "op-00001")
	resp, err := client.ExecuteAgent(ctx, models.DeployHostRequest{
		Action: models.DeployHostActionPrepare,
		Dirs:   []string{dest},
	})
	require.NoError(t, err)
	require.Len(t, resp.ResolvedDirs, 1)
	assert.Equal(t, dest, resp.ResolvedDirs[0])

	commands := srv.recordedCommands()
	require.NotEmpty(t, commands)
	assert.Equal(t, installed+deployAgentSuffix, commands[len(commands)-1])
}

func TestDeploySSH_AgentErrorSurfacesToCaller(t *testing.T) {
	addr, hostKey, _ := startDeploySSHServer(t)
	client := dialDeploySSHClient(t, addr, hostKey)
	t.Cleanup(func() { _ = client.Close() })

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	src, _ := writeDeploySourceBinary(t)
	_, err := client.UploadBinary(ctx, src, testutil.TempDir(t))
	require.NoError(t, err)

	_, err = client.ExecuteAgent(ctx, models.DeployHostRequest{Action: models.DeployHostAction("bogus")})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown deploy-host action")
}

func TestDeploySSH_ExecuteDeploySSHUploadsThroughFactory(t *testing.T) {
	addr, hostKey, _ := startDeploySSHServer(t)
	client := dialDeploySSHClient(t, addr, hostKey)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	src, content := writeDeploySourceBinary(t)
	remoteDir := testutil.TempDir(t)

	factory := func(context.Context, string, int, string) (remoteDeployClient, error) {
		return client, nil
	}
	var out bytes.Buffer
	cmd := &cobra.Command{}
	cmd.SetOut(&out)
	cmd.SetErr(&out)

	deployed, failed, cleanup, err := executeDeploySSH(
		ctx, cmd, []string{"deploy-test-host"}, 0, "",
		remoteDir, src, nil, operatorDeployOptions{}, 1, factory,
	)
	t.Cleanup(cleanup)
	require.NoError(t, err)
	assert.Empty(t, deployed)
	assert.Empty(t, failed)

	got, err := os.ReadFile(filepath.Join(remoteDir, constants.DeployBinDirname, "g8e"))
	require.NoError(t, err)
	assert.Equal(t, content, got)
}
