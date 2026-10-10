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
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/pkg/sftp"
	"github.com/stretchr/testify/require"
	sshlib "golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/knownhosts"

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

	mu               sync.Mutex
	commands         []string
	stdins           [][]byte
	forwardListeners map[uint32]net.Listener
	forwardedPorts   []uint32
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

func (s *execSSHServer) getForwardedPorts() []uint32 {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]uint32(nil), s.forwardedPorts...)
}

func (s *execSSHServer) closeForwardListeners() {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, ln := range s.forwardListeners {
		_ = ln.Close()
	}
	s.forwardListeners = nil
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
	defer s.closeForwardListeners()

	go s.handleGlobalRequests(conn, reqs)

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

func (s *execSSHServer) handleGlobalRequests(conn *sshlib.ServerConn, reqs <-chan *sshlib.Request) {
	for req := range reqs {
		switch req.Type {
		case "tcpip-forward":
			var payload struct {
				Addr  string
				RPort uint32
			}
			if err := sshlib.Unmarshal(req.Payload, &payload); err != nil {
				if req.WantReply {
					_ = req.Reply(false, nil)
				}
				continue
			}
			listenAddr := net.JoinHostPort(payload.Addr, fmt.Sprintf("%d", payload.RPort))
			ln, err := net.Listen("tcp", listenAddr)
			if err != nil {
				if req.WantReply {
					_ = req.Reply(false, nil)
				}
				continue
			}
			assignedPort := uint32(ln.Addr().(*net.TCPAddr).Port)
			s.mu.Lock()
			if s.forwardListeners == nil {
				s.forwardListeners = make(map[uint32]net.Listener)
			}
			s.forwardListeners[assignedPort] = ln
			s.forwardedPorts = append(s.forwardedPorts, assignedPort)
			s.mu.Unlock()

			if req.WantReply {
				_ = req.Reply(true, sshlib.Marshal(struct{ Port uint32 }{Port: assignedPort}))
			}

			go s.acceptForwardedConn(conn, ln, payload.Addr, assignedPort)

		case "cancel-tcpip-forward":
			var payload struct {
				Addr  string
				RPort uint32
			}
			if err := sshlib.Unmarshal(req.Payload, &payload); err != nil {
				if req.WantReply {
					_ = req.Reply(false, nil)
				}
				continue
			}
			s.mu.Lock()
			if ln, ok := s.forwardListeners[payload.RPort]; ok {
				_ = ln.Close()
				delete(s.forwardListeners, payload.RPort)
			}
			s.mu.Unlock()
			if req.WantReply {
				_ = req.Reply(true, nil)
			}

		default:
			if req.WantReply {
				_ = req.Reply(false, nil)
			}
		}
	}
}

func (s *execSSHServer) acceptForwardedConn(conn *sshlib.ServerConn, ln net.Listener, addr string, port uint32) {
	for {
		c, err := ln.Accept()
		if err != nil {
			return
		}
		go func(localConn net.Conn) {
			defer localConn.Close()
			originHost, originPortStr, _ := net.SplitHostPort(localConn.RemoteAddr().String())
			var originPort uint32
			_, _ = fmt.Sscanf(originPortStr, "%d", &originPort)

			type forwardedTCPPayload struct {
				Addr       string
				Port       uint32
				OriginAddr string
				OriginPort uint32
			}
			extra := sshlib.Marshal(&forwardedTCPPayload{
				Addr:       addr,
				Port:       port,
				OriginAddr: originHost,
				OriginPort: originPort,
			})
			ch, reqs, err := conn.OpenChannel("forwarded-tcpip", extra)
			if err != nil {
				return
			}
			defer ch.Close()
			go sshlib.DiscardRequests(reqs)

			var pipeWg sync.WaitGroup
			pipeWg.Add(2)
			go func() {
				defer pipeWg.Done()
				_, _ = io.Copy(ch, localConn)
				_ = ch.CloseWrite()
			}()
			go func() {
				defer pipeWg.Done()
				_, _ = io.Copy(localConn, ch)
				if tc, ok := localConn.(*net.TCPConn); ok {
					_ = tc.CloseWrite()
				}
			}()
			pipeWg.Wait()
		}(c)
	}
}

func (s *execSSHServer) handleSession(channel sshlib.Channel, requests <-chan *sshlib.Request, respond func(string) execReply) {
	defer channel.Close()
	for req := range requests {
		switch req.Type {
		case "subsystem":
			var payload struct{ Subsystem string }
			if err := sshlib.Unmarshal(req.Payload, &payload); err == nil && payload.Subsystem == "sftp" {
				_ = req.Reply(true, nil)
				server, err := sftp.NewServer(channel)
				if err == nil {
					_ = server.Serve()
				}
				return
			}
			if req.WantReply {
				_ = req.Reply(false, nil)
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
			if req.WantReply {
				_ = req.Reply(true, nil)
			}

			stdin, _ := io.ReadAll(channel)
			s.record(payload.Command, stdin)

			reply := respond(payload.Command)
			_, _ = io.WriteString(channel, reply.stdout)
			_, _ = io.WriteString(channel.Stderr(), reply.stderr)
			_, _ = channel.SendRequest("exit-status", false, sshlib.Marshal(struct{ Status uint32 }{reply.status}))
			return

		default:
			if req.WantReply {
				_ = req.Reply(false, nil)
			}
		}
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

func (f *streamFixture) stream(ctx context.Context, target string, binary []byte, opts ...func(*StreamHostOptions)) streamResult {
	f.t.Helper()
	resultCh := make(chan streamResult, 1)
	streamOpts := StreamHostOptions{
		BinaryData:        binary,
		SSHConfigPath:     f.sshConfigPath,
		SSHKnownHostsPath: f.knownHostsPath,
		DialTimeout:       2 * time.Second,
		Username:          "testuser",
		SSHIdentityFile:   f.identityFile,
	}
	for _, fn := range opts {
		fn(&streamOpts)
	}
	streamToHost(ctx, target, streamOpts, resultCh)
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

func proxyCommandViaTestBinary() string {
	executable := strings.ReplaceAll(filepath.ToSlash(os.Args[0]), "'", "'\\''")
	return fmt.Sprintf("'%s' -test.run TestSSHProxyHelperProcess -- %s %%h %%p", executable, proxyHelperMarker)
}
