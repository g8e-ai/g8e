// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

//go:build integration

// These tests exercise the gw connect process-lifecycle paths that require a
// real StartOperator success: profile-write failure cleanup, restart rollback,
// and resolved-port persistence. They re-execute the test binary as a Gateway
// subprocess with G8E_TEST_REEXEC=serve so the child starts a minimal HTTP
// health server and becomes healthy. They perform file I/O, network listeners,
// and real process management and therefore belong in Tier 2 (integration).

package cmd

import (
	"bytes"
	"context"
	"crypto/x509"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/v2/internal/cli/auth"
	"github.com/g8e-ai/g8e/v2/internal/cli/browserorigin"
	"github.com/g8e-ai/g8e/v2/internal/cli/frontendverify"
	"github.com/g8e-ai/g8e/v2/internal/cli/platform"
	"github.com/g8e-ai/g8e/v2/internal/cli/serve"
	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/services/fs"
)

// launchProfileWriteFailingFileSvc wraps a RuntimeFileService and fails
// WriteFile calls that target the launch profile path. All other operations
// delegate to the underlying service. This lets process-lifecycle tests
// exercise the profile-write-failure cleanup path after StartOperator succeeds
// (PID file write, binary copy, and health check all use the real service).
type launchProfileWriteFailingFileSvc struct {
	fs.RuntimeFileService
	failErr error
}

func (w *launchProfileWriteFailingFileSvc) WriteFile(ctx context.Context, relPath string, data []byte, mode os.FileMode) error {
	launchProfileRelPath := filepath.Join(constants.PidDirname, constants.OperatorLaunchProfileFilename)
	if relPath == launchProfileRelPath {
		return w.failErr
	}
	return w.RuntimeFileService.WriteFile(ctx, relPath, data, mode)
}

// withServeReExec sets G8E_TEST_REEXEC=serve so the re-executed test binary
// starts a minimal HTTP health server instead of exiting immediately. The
// env var is restored via t.Cleanup. It also registers a cleanup that reaps
// any remaining child process and removes stale PID files, because the test
// process is the parent of the re-executed subprocess and must call waitpid
// to reap zombies (in production the CLI exits after starting the Gateway,
// so init adopts and reaps it).
func withServeReExec(t *testing.T, fileSvc fs.RuntimeFileService) {
	t.Helper()
	old, hadOld := os.LookupEnv("G8E_TEST_REEXEC")
	require.NoError(t, os.Setenv("G8E_TEST_REEXEC", "serve"))
	t.Cleanup(func() {
		if hadOld {
			_ = os.Setenv("G8E_TEST_REEXEC", old)
		} else {
			_ = os.Unsetenv("G8E_TEST_REEXEC")
		}
	})
	t.Cleanup(func() {
		reapRemainingChild(t, fileSvc)
	})
}

// startZombieReaper starts a background goroutine that polls for and reaps
// any exited child processes (zombies) using Wait4 with WNOHANG. This is
// necessary because the test process is the parent of all re-executed
// subprocesses; in production the CLI exits after starting the Gateway and
// init reaps it, but in tests the parent must explicitly wait. Without this,
// StopOperator's isProcessRunning check sees the zombie and reports the
// process as still running, causing StopOperator to fail.
//
// The goroutine is stopped via t.Cleanup.
func startZombieReaper(t *testing.T) {
	t.Helper()
	stop := make(chan struct{})
	go func() {
		for {
			select {
			case <-stop:
				return
			default:
				var ws syscall.WaitStatus
				pid, err := syscall.Wait4(-1, &ws, syscall.WNOHANG, nil)
				if err != nil || pid == 0 {
					time.Sleep(5 * time.Millisecond)
				}
			}
		}
	}()
	t.Cleanup(func() { close(stop) })
}

// reapRemainingChild reads the operator PID file, sends SIGKILL to the process
// if it still exists, reaps the zombie via waitpid, and removes the PID file.
// This is necessary because the test process is the parent of the re-executed
// subprocess; in production the CLI exits after starting the Gateway and init
// reaps it, but in tests the parent must explicitly wait.
func reapRemainingChild(t *testing.T, fileSvc fs.RuntimeFileService) {
	t.Helper()
	pidRel := filepath.Join(constants.PidDirname, constants.OperatorPIDFilename)
	data, err := fileSvc.ReadFile(context.Background(), pidRel)
	if err != nil {
		return // PID file already gone — nothing to reap.
	}
	pid, err := strconv.Atoi(string(data))
	if err != nil || pid == 0 {
		return
	}
	_ = syscall.Kill(pid, syscall.SIGKILL)
	var ws syscall.WaitStatus
	_, _ = syscall.Wait4(pid, &ws, 0, nil)
	_ = fileSvc.Remove(context.Background(), pidRel)
}

// assertGatewayStopped asserts that OperatorStatus reports no running Gateway
// and no PID file remains.
func assertGatewayStopped(t *testing.T, pm *platform.ProcessManager, fileSvc fs.RuntimeFileService) {
	t.Helper()
	running, pid, err := pm.OperatorStatus()
	require.NoError(t, err, "OperatorStatus should not error after cleanup")
	assert.False(t, running, "Gateway should not be running after cleanup")
	assert.Zero(t, pid, "no PID should remain after cleanup")

	pidRel := filepath.Join(constants.PidDirname, constants.OperatorPIDFilename)
	exists, err := fileSvc.FileExists(context.Background(), pidRel)
	require.NoError(t, err)
	assert.False(t, exists, "no PID file should remain after cleanup")
}

// TestConnectStoppedGateway_ProfileWriteFailureStopsGateway verifies that
// connectStoppedGateway stops the untracked Gateway when WriteLaunchProfile
// fails after StartOperator succeeds. The test uses a wrapper fileSvc that
// fails WriteFile for the launch profile path while allowing PID file writes
// and binary copy to succeed. The re-executed test binary serves health checks
// so StartOperator completes successfully. A zombie reaper goroutine reaps
// killed children so StopOperator succeeds within its poll window. The test
// asserts the error wraps ErrInternal, mentions the profile write failure,
// and the Gateway is stopped with no stale PID file.
func TestConnectStoppedGateway_ProfileWriteFailureStopsGateway(t *testing.T) {
	fileSvc, cfg := newCmdTestEnv(t)
	withServeReExec(t, fileSvc)
	startZombieReaper(t)

	failingFileSvc := &launchProfileWriteFailingFileSvc{
		RuntimeFileService: fileSvc,
		failErr:            constants.ErrLaunchProfileInvalid,
	}

	deps := connectDeps{
		trustInstaller: &stubTrustInstaller{},
		discoveryFetcher: func(context.Context, string, func() time.Time) (auth.TrustDiscoveryResult, error) {
			panic("discoveryFetcher should not be called when profile write fails")
		},
	}

	cmd := gatewayConnectCmdWithConfig(configLoaderFor(cfg), fileSvcFactoryFor(failingFileSvc), deps)
	cmd.SetContext(t.Context())
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	err := cmd.RunE(cmd, []string{"https://your-app.lovable.app"})
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrInternal, "error should wrap ErrInternal, got: %v", err)
	assert.Contains(t, err.Error(), "persist launch profile",
		"error should mention the profile write failure, got: %v", err)

	pm, pmErr := platform.NewProcessManager(fileSvc)
	require.NoError(t, pmErr)
	assertGatewayStopped(t, pm, fileSvc)
}

// TestConnectRestartGateway_ProfileWriteFailureRollsBackToPreviousConfig
// verifies that connectRestartGateway attempts rollback to the previous
// configuration when WriteLaunchProfile fails after StartOperator succeeds
// with the new configuration. The test starts a real Gateway subprocess with
// an old configuration, writes a matching launch profile, then calls gw connect
// with a different origin and a wrapper fileSvc that fails the profile write.
// The restart flow stops the old process, starts with the new config (succeeds),
// fails to persist the profile, stops the new process, and rolls back to the
// old config. The test asserts the error wraps ErrInternal, mentions the
// profile write failure, and that a Gateway process is running after rollback
// (the rollback start succeeded).
func TestConnectRestartGateway_ProfileWriteFailureRollsBackToPreviousConfig(t *testing.T) {
	fileSvc, cfg := newCmdTestEnv(t)
	withServeReExec(t, fileSvc)
	startZombieReaper(t)

	// Start a real Gateway subprocess with the old configuration.
	pm, err := platform.NewProcessManager(fileSvc)
	require.NoError(t, err)

	oldOrigin, err := browserorigin.Parse("https://old-app.lovable.app")
	require.NoError(t, err)

	oldCfg := serve.GatewayConfig{
		Posture:          "doctrine",
		LogLevel:         "info",
		CertIdentityMode: "localhost",
		AllowedOrigins:   []string{oldOrigin.URL},
		PasskeyRpOrigins: []string{oldOrigin.URL},
		PasskeyRpID:      oldOrigin.RPID,
		PasskeyRpName:    "g8e",
	}
	oldStartOpts := platform.OperatorStartOptions{GatewayConfig: oldCfg}
	require.NoError(t, pm.StartOperator(&oldStartOpts))

	// Persist the old launch profile so connectRestartGateway can read it.
	require.NoError(t, serve.WriteLaunchProfile(fileSvc, oldCfg))

	// Wrap the fileSvc so the new profile write fails but other I/O succeeds.
	failingFileSvc := &launchProfileWriteFailingFileSvc{
		RuntimeFileService: fileSvc,
		failErr:            constants.ErrLaunchProfileInvalid,
	}

	deps := connectDeps{
		trustInstaller: &stubTrustInstaller{},
		discoveryFetcher: func(context.Context, string, func() time.Time) (auth.TrustDiscoveryResult, error) {
			panic("discoveryFetcher should not be called when profile write fails")
		},
	}

	cmd := gatewayConnectCmdWithConfig(configLoaderFor(cfg), fileSvcFactoryFor(failingFileSvc), deps)
	require.NoError(t, cmd.Flags().Set("yes", "true"))
	cmd.SetContext(t.Context())
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	err = cmd.RunE(cmd, []string{"https://your-app.lovable.app"})
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrInternal, "error should wrap ErrInternal, got: %v", err)
	assert.Contains(t, err.Error(), "persist launch profile",
		"error should mention the profile write failure, got: %v", err)

	// After rollback, a Gateway process should be running with the old config.
	// The PID file should exist (written by the rollback StartOperator).
	runPM, runErr := platform.NewProcessManager(fileSvc)
	require.NoError(t, runErr)
	running, pid, statusErr := runPM.OperatorStatus()
	require.NoError(t, statusErr)
	assert.True(t, running, "Gateway should be running after rollback")
	assert.NotZero(t, pid, "rollback Gateway should have a PID")
}

// TestConnectStoppedGateway_PersistsResolvedPorts verifies that
// connectStoppedGateway persists the resolved ports (not the requested ports)
// in the launch profile after StartOperator succeeds. The test selects an
// isolated port pair and binds the requested HTTP port to force StartOperator
// to shift to the next available port, then calls gw connect on a stopped
// gateway. After the command completes (it may fail at trust/verify since the
// subprocess only serves HTTP health checks, not HTTPS), the test reads the
// persisted launch profile and asserts the HTTP and HTTPS ports differ from the
// requested ports.
func TestConnectStoppedGateway_PersistsResolvedPorts(t *testing.T) {
	fileSvc, cfg := newCmdTestEnv(t)
	withServeReExec(t, fileSvc)
	startZombieReaper(t)

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	t.Cleanup(func() { _ = ln.Close() })
	requestedHTTPPort := ln.Addr().(*net.TCPAddr).Port

	httpsProbe, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	requestedHTTPSPort := httpsProbe.Addr().(*net.TCPAddr).Port
	require.NoError(t, httpsProbe.Close())

	baseCfg := defaultServeConfig()
	baseCfg.HTTPPort = requestedHTTPPort
	baseCfg.HTTPSPort = requestedHTTPSPort
	require.NoError(t, serve.WriteLaunchProfile(fileSvc, baseCfg))

	deps := connectDeps{
		trustInstaller: &stubTrustInstaller{trusted: true},
		discoveryFetcher: func(context.Context, string, func() time.Time) (auth.TrustDiscoveryResult, error) {
			return stubDiscoveryResult(t), nil
		},
		verifier: frontendverify.NewVerifier(frontendverify.VerifierDeps{
			HTTPClientFactory: func(*x509.CertPool, time.Duration) (*http.Client, error) {
				return &http.Client{Timeout: 2 * time.Second}, nil
			},
		}),
		confirm:    func(string) bool { return false },
		continueFn: func(string) bool { return true },
		now:        time.Now,
	}

	cmd := gatewayConnectCmdWithConfig(configLoaderFor(cfg), fileSvcFactoryFor(fileSvc), deps)
	cmd.SetContext(t.Context())
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	err = cmd.RunE(cmd, []string{"https://your-app.lovable.app"})
	// The command may fail at trust/verify because the subprocess doesn't
	// serve HTTPS. That's expected — the profile is already persisted.
	require.Error(t, err)

	// Read the persisted launch profile and assert the ports were resolved
	// after the requested HTTP port collision.
	profile, readErr := serve.ReadLaunchProfile(fileSvc)
	require.NoError(t, readErr, "launch profile should be persisted after successful start")

	assert.NotEqual(t, requestedHTTPPort, profile.Config.HTTPPort,
		"HTTP port should be shifted from requested port %d", requestedHTTPPort)
	assert.NotEqual(t, requestedHTTPSPort, profile.Config.HTTPSPort,
		"HTTPS port should be shifted from requested port %d", requestedHTTPSPort)

	// The offset between HTTP and HTTPS should be preserved.
	httpOffset := profile.Config.HTTPPort - requestedHTTPPort
	expectedHTTPS := requestedHTTPSPort + httpOffset
	assert.Equal(t, expectedHTTPS, profile.Config.HTTPSPort,
		"HTTPS port should maintain the same offset from the requested port as HTTP port")
}
