// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package stream

// Hermetic end-to-end tests for streamToHost, preFlightCheck and dialSSH against
// an in-process SSH server. The server records each exec request together with
// the stdin the client streamed, so assertions cover what actually crossed the
// wire rather than what the client claims to have sent.

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	sshlib "golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/knownhosts"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/pkg/ssh"
	"github.com/g8e-ai/g8e/v2/internal/testutil"
)

// proxyHelperMarker identifies the re-exec'd test binary acting as a ProxyCommand.
const proxyHelperMarker = "g8e-ssh-proxy-helper"

// execReply is what the fake server answers for one exec request.
type execReply struct {
	stdout string
	stderr string
	status uint32
}

type execSSHServer struct {
	addr    string
	hostKey sshlib.PublicKey

	mu       sync.Mutex
	commands []string
	stdins   [][]byte
}

func (s *execSSHServer) record(command string, stdin []byte) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.commands = append(s.commands, command)
	s.stdins = append(s.stdins, stdin)
}

func (s *execSSHServer) snapshot() ([]string, [][]byte) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.commands...), append([][]byte(nil), s.stdins...)
}

func newHostSigner(t *testing.T) sshlib.Signer {
	t.Helper()
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	signer, err := sshlib.NewSignerFromKey(priv)
	require.NoError(t, err)
	return signer
}

func (s *execSSHServer) serve(raw net.Conn, config *sshlib.ServerConfig, rejectSessions bool, respond func(string) execReply) {
	conn, chans, reqs, err := sshlib.NewServerConn(raw, config)
	if err != nil {
		_ = raw.Close()
		return
	}
	defer conn.Close()
	go sshlib.DiscardRequests(reqs)

	for newChannel := range chans {
		if rejectSessions || newChannel.ChannelType() != "session" {
			_ = newChannel.Reject(sshlib.Prohibited, "sessions are not permitted")
			continue
		}
		channel, requests, err := newChannel.Accept()
		if err != nil {
			return
		}
		go s.handleSession(channel, requests, respond)
	}
}

func (s *execSSHServer) handleSession(channel sshlib.Channel, requests <-chan *sshlib.Request, respond func(string) execReply) {
	defer channel.Close()
	for req := range requests {
		if req.Type != "exec" {
			if req.WantReply {
				_ = req.Reply(false, nil)
			}
			continue
		}
		var payload struct{ Command string }
		if err := sshlib.Unmarshal(req.Payload, &payload); err != nil {
			_ = req.Reply(false, nil)
			return
		}
		_ = req.Reply(true, nil)

		// The client half-closes stdin after streaming the binary.
		stdin, _ := io.ReadAll(channel)
		s.record(payload.Command, stdin)

		reply := respond(payload.Command)
		_, _ = io.WriteString(channel, reply.stdout)
		_, _ = io.WriteString(channel.Stderr(), reply.stderr)
		_, _ = channel.SendRequest("exit-status", false, sshlib.Marshal(struct{ Status uint32 }{reply.status}))
		return
	}
}

type streamFixture struct {
	t              *testing.T
	server         *execSSHServer
	sshConfigPath  string
	knownHostsPath string
	identityFile   string
}

func newStreamFixture(t *testing.T, server *execSSHServer) *streamFixture {
	t.Helper()
	dir := testutil.TempDir(t)

	identity := filepath.Join(dir, "id_rsa")
	generateTestSSHKey(t, identity)

	knownHosts := filepath.Join(dir, "known_hosts")
	line := knownhosts.Line([]string{knownhosts.Normalize(server.addr)}, server.hostKey)
	require.NoError(t, os.WriteFile(knownHosts, []byte(line+"\n"), 0o600))

	config := filepath.Join(dir, "config")
	require.NoError(t, os.WriteFile(config, nil, 0o600))

	return &streamFixture{t: t, server: server, sshConfigPath: config, knownHostsPath: knownHosts, identityFile: identity}
}

func (f *streamFixture) stream(ctx context.Context, target string, binary []byte, operatorArgs string, preflight bool) streamResult {
	f.t.Helper()
	resultCh := make(chan streamResult, 1)
	streamToHost(ctx, target, binary, operatorArgs, f.sshConfigPath, f.knownHostsPath, 2*time.Second,
		"", "testuser", f.identityFile, "", "", preflight, resultCh)
	select {
	case res := <-resultCh:
		return res
	default:
		f.t.Fatal("streamToHost returned without emitting a result")
		return streamResult{}
	}
}

func (f *streamFixture) target() string { return "testuser@" + f.server.addr }

func exitReply(status uint32, stdout, stderr string) func(string) execReply {
	return func(string) execReply { return execReply{stdout: stdout, stderr: stderr, status: status} }
}

func startProxyConn(t *testing.T, shellCommand string) *proxyConn {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	t.Cleanup(cancel)

	cmd := exec.CommandContext(ctx, constants.PathBinSh, "-c", shellCommand)
	stdin, err := cmd.StdinPipe()
	require.NoError(t, err)
	stdout, err := cmd.StdoutPipe()
	require.NoError(t, err)
	require.NoError(t, cmd.Start())
	return &proxyConn{stdin: stdin, stdout: stdout, cmd: cmd, addr: "proxied:22"}
}

func TestProxyConn_CloseIsSafeForConcurrentCallers(t *testing.T) {
	// x/crypto/ssh closes its net.Conn from several goroutines at teardown.
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
			conn := startProxyConn(t, tt.shellCommand)

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

func TestDialSSH_ProxyCommandThatExitsImmediatelyFailsHandshake(t *testing.T) {
	config := &sshlib.ClientConfig{
		User:            "testuser",
		HostKeyCallback: sshlib.FixedHostKey(newHostSigner(t).PublicKey()),
		Timeout:         time.Second,
	}

	select {
	case result := <-dialSSH(context.Background(), ssh.HostConfig{ProxyCommand: "exit 1", Hostname: "h", Port: "22"}, config, "h:22"):
		assert.Nil(t, result.client)
		require.Error(t, result.err)
		assert.Contains(t, result.err.Error(), "ssh: client connection")
	case <-time.After(5 * time.Second):
		t.Fatal("dialSSH did not report the proxy failure")
	}
}

func proxyCommandViaTestBinary() string {
	return fmt.Sprintf("%s -test.run TestSSHProxyHelperProcess -- %s %%h %%p", os.Args[0], proxyHelperMarker)
}
