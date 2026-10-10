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

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	sshlib "golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/knownhosts"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/pkg/ssh"
	"github.com/g8e-ai/g8e/v2/internal/testutil"
)

// startExecSSHServer starts an SSH server that accepts any client and answers
// every exec request through respond. With rejectSessions it refuses session
// channels, which makes client.NewSession fail.
func startExecSSHServer(t *testing.T, rejectSessions bool, respond func(command string) execReply) *execSSHServer {
	t.Helper()
	signer := newHostSigner(t)
	config := &sshlib.ServerConfig{NoClientAuth: true}
	config.AddHostKey(signer)

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	t.Cleanup(func() { _ = listener.Close() })

	server := &execSSHServer{addr: listener.Addr().String(), hostKey: signer.PublicKey()}

	go func() {
		for {
			raw, err := listener.Accept()
			if err != nil {
				return
			}
			go server.serve(raw, config, rejectSessions, respond)
		}
	}()
	return server
}

func TestStreamToHost_DeliversBinaryViaSFTPAndExecutesDirectly(t *testing.T) {
	var executedCommand string
	var remoteUploadedPath string
	var remoteUploadedMode os.FileMode
	var remoteUploadedContent []byte

	server := startExecSSHServer(t, false, func(command string) execReply {
		executedCommand = command
		parts := strings.SplitN(command, " ", 2)
		if len(parts) > 0 {
			remoteUploadedPath = parts[0]
			if info, err := os.Stat(remoteUploadedPath); err == nil {
				remoteUploadedMode = info.Mode().Perm()
				remoteUploadedContent, _ = os.ReadFile(remoteUploadedPath)
			}
		}
		return execReply{status: 0}
	})
	fixture := newStreamFixture(t, server)
	binary := []byte("\x7fELF-not-really-a-binary\x00\x01\x02")

	res := fixture.stream(context.Background(), fixture.target(), binary)

	require.NoError(t, res.Error)
	assert.Equal(t, constants.StreamStatusCompleted, res.Status)
	assert.Equal(t, fixture.target(), res.Host)
	assert.Equal(t, int64(len(binary)), res.SizeBytes)
	assert.Positive(t, res.Elapsed)

	// Verify the binary was uploaded via SFTP with 0755 permissions and expected content
	assert.True(t, strings.HasPrefix(remoteUploadedPath, "/tmp/"+constants.StreamTempBinaryPrefix),
		"uploaded path %q should start with /tmp/.g8e-stream-", remoteUploadedPath)
	assert.Equal(t, os.FileMode(0755), remoteUploadedMode, "remote binary must have 0755 permissions")
	assert.Equal(t, binary, remoteUploadedContent, "remote uploaded content must match binary")

	// Verify remote binary was cleaned up on exit
	_, err := os.Stat(remoteUploadedPath)
	assert.True(t, os.IsNotExist(err), "remote binary must be cleaned up on session exit, but still exists: %v", err)

	// Verify command format: "<remotePath> operator start --endpoint 127.0.0.1 --gateway-http-port <p1> --gateway-https-port <p2>"
	fwdPorts := server.getForwardedPorts()
	require.Len(t, fwdPorts, 2, "must have forwarded HTTP and HTTPS ports")
	wantCommand := fmt.Sprintf("%s operator start --endpoint 127.0.0.1 --gateway-http-port %d --gateway-https-port %d",
		remoteUploadedPath, fwdPorts[0], fwdPorts[1])
	assert.Equal(t, wantCommand, executedCommand)

	// Verify stdin is empty (binary is not sent over stdin)
	commands, stdins := server.snapshot()
	require.Len(t, commands, 1)
	assert.Empty(t, stdins[0], "stdin must be empty during direct execution")
}

func TestStreamToHost_ExecutesDirectCommandWithNoGitFlag(t *testing.T) {
	server := startExecSSHServer(t, false, exitReply(0, "", ""))
	fixture := newStreamFixture(t, server)

	res := fixture.stream(context.Background(), fixture.target(), []byte("bin"), func(o *StreamHostOptions) {
		o.NoGit = true
	})

	require.NoError(t, res.Error)
	assert.Equal(t, constants.StreamStatusCompleted, res.Status)
	commands, _ := server.snapshot()
	require.Len(t, commands, 1)
	fwdPorts := server.getForwardedPorts()
	require.Len(t, fwdPorts, 2)
	assert.True(t, strings.HasSuffix(commands[0], " --no-git"), "command %q should end with --no-git", commands[0])
	assert.Contains(t, commands[0], "operator start --endpoint 127.0.0.1")
	assert.Contains(t, commands[0], fmt.Sprintf("--gateway-http-port %d", fwdPorts[0]))
	assert.Contains(t, commands[0], fmt.Sprintf("--gateway-https-port %d", fwdPorts[1]))
}

func TestStreamToHost_ReportsRemoteExitCodeWithBestAvailableOutputTail(t *testing.T) {
	tests := []struct {
		name    string
		stdout  string
		stderr  string
		wantMsg string
	}{
		{name: "stderr tail is preferred", stdout: "from stdout", stderr: "  boom: disk full \n", wantMsg: "ssh: exit code 3: boom: disk full"},
		{name: "stdout tail is used when stderr is empty", stdout: "only stdout\n", stderr: "", wantMsg: "ssh: exit code 3: only stdout"},
		{name: "bare exit code when the remote printed nothing", stdout: "", stderr: "  \n", wantMsg: "ssh: exit code 3"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := startExecSSHServer(t, false, exitReply(3, tt.stdout, tt.stderr))
			fixture := newStreamFixture(t, server)

			res := fixture.stream(context.Background(), fixture.target(), []byte("bin"))

			assert.Equal(t, constants.StreamStatusExited, res.Status, "a remote exit is distinct from a transport failure")
			require.Error(t, res.Error)
			assert.EqualError(t, res.Error, tt.wantMsg)
		})
	}
}

func TestStreamToHost_PreFlightRunsVerifyCommandBeforeStreamingBinary(t *testing.T) {
	server := startExecSSHServer(t, false, exitReply(0, "", ""))
	fixture := newStreamFixture(t, server)
	binary := []byte("payload")

	res := fixture.stream(context.Background(), fixture.target(), binary, func(o *StreamHostOptions) {
		o.PreFlightCheck = true
	})

	require.NoError(t, res.Error)
	assert.Equal(t, constants.StreamStatusCompleted, res.Status)
	commands, stdins := server.snapshot()
	require.Len(t, commands, 2)
	assert.Equal(t, constants.SSHPreflightVerifyCommand, commands[0])
	assert.Empty(t, stdins[0], "the pre-flight probe must not carry the binary")
	assert.Empty(t, stdins[1], "stdin is empty during direct execution")
	assert.Contains(t, commands[1], "operator start --endpoint 127.0.0.1")
}

func TestStreamToHost_FailedPreFlightPreventsBinaryTransfer(t *testing.T) {
	server := startExecSSHServer(t, false, func(command string) execReply {
		if command == constants.SSHPreflightVerifyCommand {
			return execReply{status: 1}
		}
		return execReply{}
	})
	fixture := newStreamFixture(t, server)

	res := fixture.stream(context.Background(), fixture.target(), []byte("payload"), func(o *StreamHostOptions) {
		o.PreFlightCheck = true
	})

	assert.Equal(t, constants.StreamStatusFailed, res.Status)
	require.Error(t, res.Error)
	assert.Contains(t, res.Error.Error(), "ssh: verify")
	commands, _ := server.snapshot()
	assert.Equal(t, []string{constants.SSHPreflightVerifyCommand}, commands, "the binary must never be streamed after a failed probe")
}

func TestStreamToHost_FailsWhenServerRefusesSessions(t *testing.T) {
	server := startExecSSHServer(t, true, exitReply(0, "", ""))
	fixture := newStreamFixture(t, server)

	res := fixture.stream(context.Background(), fixture.target(), []byte("bin"))

	assert.Equal(t, constants.StreamStatusFailed, res.Status)
	require.Error(t, res.Error)
	assert.Contains(t, res.Error.Error(), "ssh: upload binary (after 0 retries)")
}

func TestStreamToHost_RejectsHostWhoseKeyDiffersFromKnownHosts(t *testing.T) {
	server := startExecSSHServer(t, false, exitReply(0, "", ""))
	fixture := newStreamFixture(t, server)

	impostor := newHostSigner(t).PublicKey()
	line := knownhosts.Line([]string{knownhosts.Normalize(server.addr)}, impostor)
	require.NoError(t, os.WriteFile(fixture.knownHostsPath, []byte(line+"\n"), 0o600))

	res := fixture.stream(context.Background(), fixture.target(), []byte("bin"))

	assert.Equal(t, constants.StreamStatusFailed, res.Status)
	require.Error(t, res.Error)
	assert.Contains(t, res.Error.Error(), "ssh: dial")
	assert.Contains(t, res.Error.Error(), "key mismatch")
	commands, _ := server.snapshot()
	assert.Empty(t, commands, "nothing may be executed on a host that fails key verification")
}

func TestStreamToHost_FailsWhenKnownHostsFileIsMissing(t *testing.T) {
	server := startExecSSHServer(t, false, exitReply(0, "", ""))
	fixture := newStreamFixture(t, server)
	require.NoError(t, os.Remove(fixture.knownHostsPath))

	res := fixture.stream(context.Background(), fixture.target(), []byte("bin"))

	assert.Equal(t, constants.StreamStatusFailed, res.Status)
	require.Error(t, res.Error)
	assert.Contains(t, res.Error.Error(), "host key callback")
}

func TestStreamToHost_FailsWithoutAnyAuthMethod(t *testing.T) {
	testHome := testutil.TempDir(t)
	t.Setenv("HOME", testHome)
	t.Setenv("USERPROFILE", testHome)
	server := startExecSSHServer(t, false, exitReply(0, "", ""))
	fixture := newStreamFixture(t, server)
	fixture.identityFile = ""

	res := fixture.stream(context.Background(), fixture.target(), []byte("bin"))

	assert.Equal(t, constants.StreamStatusFailed, res.Status)
	require.ErrorIs(t, res.Error, constants.ErrMCPRunShellCommandNoAuth)
}

func TestStreamToHost_FailsWhenAgentSocketIsUnreachable(t *testing.T) {
	server := startExecSSHServer(t, false, exitReply(0, "", ""))
	fixture := newStreamFixture(t, server)
	resultCh := make(chan streamResult, 1)

	streamToHost(context.Background(), fixture.target(), StreamHostOptions{
		BinaryData:        []byte("bin"),
		SSHConfigPath:     fixture.sshConfigPath,
		SSHKnownHostsPath: fixture.knownHostsPath,
		DialTimeout:       2 * time.Second,
		SSHAuthSock:       filepath.Join(testutil.TempDir(t), "missing-agent.sock"),
		Username:          "testuser",
		SSHIdentityFile:   fixture.identityFile,
	}, resultCh)

	res := <-resultCh
	assert.Equal(t, constants.StreamStatusFailed, res.Status)
	require.Error(t, res.Error)
	assert.Contains(t, res.Error.Error(), "ssh: build auth")
}

func TestStreamToHost_FailsWhenSSHConfigCannotBeRead(t *testing.T) {
	server := startExecSSHServer(t, false, exitReply(0, "", ""))
	fixture := newStreamFixture(t, server)
	// A directory is not a readable config file; a merely missing one is tolerated.
	fixture.sshConfigPath = testutil.TempDir(t)

	res := fixture.stream(context.Background(), fixture.target(), []byte("bin"))

	assert.Equal(t, constants.StreamStatusFailed, res.Status)
	require.Error(t, res.Error)
	assert.Contains(t, res.Error.Error(), "ssh: resolve host")
}

func TestStreamToHost_CancelsDuringRetryBackoffAfterTransientDialFailure(t *testing.T) {
	// Reserve then release a port so the dial is refused, which is transient.
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	refusedAddr := listener.Addr().String()
	require.NoError(t, listener.Close())

	server := startExecSSHServer(t, false, exitReply(0, "", ""))
	fixture := newStreamFixture(t, server)

	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	t.Cleanup(cancel)

	res := fixture.stream(ctx, "testuser@"+refusedAddr, []byte("bin"))

	assert.Equal(t, constants.StreamStatusCancelled, res.Status)
	require.ErrorIs(t, res.Error, constants.ErrSSHRetryBackoffCancelled)
}

func TestStreamToHost_CancellationInterruptsRunningRemoteCommand(t *testing.T) {
	release := make(chan struct{})
	t.Cleanup(func() { close(release) })
	server := startExecSSHServer(t, false, func(string) execReply {
		<-release
		return execReply{}
	})
	fixture := newStreamFixture(t, server)

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	resultCh := make(chan streamResult, 1)
	go func() {
		streamToHost(ctx, fixture.target(), StreamHostOptions{
			BinaryData:        []byte("bin"),
			SSHConfigPath:     fixture.sshConfigPath,
			SSHKnownHostsPath: fixture.knownHostsPath,
			DialTimeout:       2 * time.Second,
			Username:          "testuser",
			SSHIdentityFile:   fixture.identityFile,
		}, resultCh)
	}()

	require.Eventually(t, func() bool {
		commands, _ := server.snapshot()
		return len(commands) == 1
	}, 5*time.Second, 10*time.Millisecond, "remote command never started")
	cancel()

	select {
	case res := <-resultCh:
		assert.NotEqual(t, constants.StreamStatusCompleted, res.Status)
		assert.Error(t, res.Error)
	case <-time.After(5 * time.Second):
		t.Fatal("streamToHost did not return after the context was cancelled")
	}
}

func TestPreFlightCheck_SucceedsAgainstReachableHost(t *testing.T) {
	server := startExecSSHServer(t, false, exitReply(0, "", ""))
	fixture := newStreamFixture(t, server)
	host, port, err := net.SplitHostPort(server.addr)
	require.NoError(t, err)

	err = preFlightCheck(context.Background(), ssh.HostConfig{
		Hostname: host, Port: port, User: "testuser", KeyFiles: []string{fixture.identityFile},
	}, "", "", fixture.knownHostsPath, 2*time.Second)

	require.NoError(t, err)
	commands, _ := server.snapshot()
	assert.Equal(t, []string{constants.SSHPreflightVerifyCommand}, commands)
}

func TestPreFlightCheck_ReportsSessionAndVerifyFailures(t *testing.T) {
	tests := []struct {
		name           string
		rejectSessions bool
		respond        func(string) execReply
		wantMsg        string
	}{
		{name: "server refuses the session channel", rejectSessions: true, respond: exitReply(0, "", ""), wantMsg: "ssh: session"},
		{name: "verify command exits non-zero", respond: exitReply(127, "", ""), wantMsg: "ssh: verify"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := startExecSSHServer(t, tt.rejectSessions, tt.respond)
			fixture := newStreamFixture(t, server)
			host, port, err := net.SplitHostPort(server.addr)
			require.NoError(t, err)

			err = preFlightCheck(context.Background(), ssh.HostConfig{
				Hostname: host, Port: port, User: "testuser", KeyFiles: []string{fixture.identityFile},
			}, "", "", fixture.knownHostsPath, 2*time.Second)

			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantMsg)
		})
	}
}

func TestPreFlightCheck_PropagatesDialAndHostKeyFailures(t *testing.T) {
	server := startExecSSHServer(t, false, exitReply(0, "", ""))
	fixture := newStreamFixture(t, server)
	host, port, err := net.SplitHostPort(server.addr)
	require.NoError(t, err)
	hostConfig := ssh.HostConfig{Hostname: host, Port: port, User: "testuser", KeyFiles: []string{fixture.identityFile}}

	t.Run("missing known_hosts file", func(t *testing.T) {
		err := preFlightCheck(context.Background(), hostConfig, "", "", filepath.Join(testutil.TempDir(t), "absent"), time.Second)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "host key callback")
	})

	t.Run("unreachable agent socket", func(t *testing.T) {
		err := preFlightCheck(context.Background(), hostConfig, filepath.Join(testutil.TempDir(t), "missing.sock"), "", fixture.knownHostsPath, time.Second)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "ssh: build auth")
	})

	t.Run("refused connection surfaces the dial error", func(t *testing.T) {
		listener, err := net.Listen("tcp", "127.0.0.1:0")
		require.NoError(t, err)
		_, closedPort, err := net.SplitHostPort(listener.Addr().String())
		require.NoError(t, err)
		require.NoError(t, listener.Close())

		refused := hostConfig
		refused.Port = closedPort
		err = preFlightCheck(context.Background(), refused, "", "", fixture.knownHostsPath, time.Second)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "ssh: dial")
	})
}

// TestSSHProxyHelperProcess is not a real test: when the test binary is
// re-executed as a ProxyCommand it bridges stdin/stdout to a TCP address, like
// `nc %h %p`, so proxy dialing can be exercised without external tools.
func TestSSHProxyHelperProcess(t *testing.T) {
	marker := -1
	for i, arg := range os.Args {
		if arg == "--" && i+3 < len(os.Args) && os.Args[i+1] == proxyHelperMarker {
			marker = i
			break
		}
	}
	if marker < 0 {
		t.Skip("only runs when re-executed as an SSH ProxyCommand")
	}

	conn, err := net.Dial("tcp", net.JoinHostPort(os.Args[marker+2], os.Args[marker+3]))
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	copied := make(chan struct{})
	go func() {
		_, _ = io.Copy(os.Stdout, conn)
		close(copied)
	}()
	_, _ = io.Copy(conn, os.Stdin)
	_ = conn.Close()
	<-copied
	os.Exit(0)
}

func TestStreamToHost_ReachesHostThroughProxyCommandFromSSHConfig(t *testing.T) {
	server := startExecSSHServer(t, false, exitReply(0, "", ""))
	fixture := newStreamFixture(t, server)
	host, port, err := net.SplitHostPort(server.addr)
	require.NoError(t, err)

	config := fmt.Sprintf("Host bastion-only\n  Hostname %s\n  Port %s\n  ProxyCommand %s\n", host, port, proxyCommandViaTestBinary())
	require.NoError(t, os.WriteFile(fixture.sshConfigPath, []byte(config), 0o600))
	binary := []byte("tunnelled-binary")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)
	res := fixture.stream(ctx, "bastion-only", binary)

	require.NoError(t, res.Error)
	assert.Equal(t, constants.StreamStatusCompleted, res.Status)
	commands, stdins := server.snapshot()
	require.Len(t, commands, 1)
	assert.Contains(t, commands[0], "operator start --endpoint 127.0.0.1")
	assert.Empty(t, stdins[0])
}

func TestStreamToHost_ForwardsTunnelTrafficToLocalGateway(t *testing.T) {
	// Start a local TCP listener to represent the local Gateway HTTP service
	localListener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	t.Cleanup(func() { _ = localListener.Close() })
	localHTTPPort := localListener.Addr().(*net.TCPAddr).Port

	// Echo server on the local listener
	go func() {
		for {
			conn, err := localListener.Accept()
			if err != nil {
				return
			}
			go func(c net.Conn) {
				defer c.Close()
				_, _ = io.Copy(c, c)
			}(conn)
		}
	}()

	var remoteEchoResult string
	var server *execSSHServer
	server = startExecSSHServer(t, false, func(command string) execReply {
		// When command runs, remote tunnel forwards are established
		fwdPorts := server.getForwardedPorts()
		if len(fwdPorts) == 0 {
			return execReply{status: 1, stderr: "no forwarded ports available"}
		}
		// The first forwarded port is HTTP
		fwdHTTP := fwdPorts[0]
		conn, err := net.Dial("tcp", fmt.Sprintf("127.0.0.1:%d", fwdHTTP))
		if err != nil {
			return execReply{status: 1, stderr: fmt.Sprintf("dial forwarded port: %v", err)}
		}
		defer conn.Close()

		msg := []byte("ping via gateway tunnel\n")
		if _, err := conn.Write(msg); err != nil {
			return execReply{status: 1, stderr: fmt.Sprintf("write to forwarded conn: %v", err)}
		}
		buf := make([]byte, len(msg))
		if _, err := io.ReadFull(conn, buf); err != nil {
			return execReply{status: 1, stderr: fmt.Sprintf("read from forwarded conn: %v", err)}
		}
		remoteEchoResult = string(buf)
		return execReply{status: 0, stdout: "tunnel verified"}
	})
	fixture := newStreamFixture(t, server)

	res := fixture.stream(context.Background(), fixture.target(), []byte("bin"), func(o *StreamHostOptions) {
		o.GatewayHTTPPort = localHTTPPort
	})

	require.NoError(t, res.Error)
	assert.Equal(t, constants.StreamStatusCompleted, res.Status)
	assert.Equal(t, "ping via gateway tunnel\n", remoteEchoResult)
}
