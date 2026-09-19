// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

//go:build integration

// These tests exercise StartOperator's failed-start cleanup paths. They
// require real process management (copyBinaryToBinDir, cmd.Start, Kill, Wait,
// PID file I/O) and therefore belong in Tier 2 (integration). The test binary
// is re-executed by StartOperator as a subprocess; TestMain (in
// testmain_helper_test.go) detects the re-execution via os.Args[1] == "gw"
// and exits immediately, so the subprocess never becomes healthy. This lets
// the tests exercise the PID-write-failure and process-death-during-health-
// check cleanup paths without a real Gateway binary.

package platform

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/v2/internal/cli/serve"
	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/services/fs"
	"github.com/g8e-ai/g8e/v2/internal/testutil"
)

// newStartFailureFileSvc creates a temp-rooted RuntimeFileService with the
// runtime tree already created. Each test gets its own isolated directory.
func newStartFailureFileSvc(t *testing.T) fs.RuntimeFileService {
	t.Helper()
	baseDir := testutil.TempDir(t)
	fileSvc, err := fs.NewRuntimeFileService(baseDir, slog.Default())
	require.NoError(t, err)
	require.NoError(t, fileSvc.CreateRuntimeTree(context.Background()))
	return fileSvc
}

// pidRelPath returns the relative path to the operator PID file, constructed
// from path constants.
func pidRelPath() string {
	return filepath.Join(constants.PidDirname, constants.OperatorPIDFilename)
}

// minimalStartOpts returns OperatorStartOptions with a minimal valid config
// suitable for exercising the start-failure paths. The actual gateway never
// becomes healthy (TestMain exits immediately), so the config values do not
// matter beyond passing initial validation.
func minimalStartOpts() *OperatorStartOptions {
	return &OperatorStartOptions{
		GatewayConfig: serve.GatewayConfig{
			Posture:  "doctrine",
			LogLevel: "info",
		},
	}
}

// assertNoLiveChildAndNoPIDFile verifies that StartOperator's failure cleanup
// left no running child process and no stale PID file behind. OperatorStatus
// reads the PID file; if it returns running=false and pid=0, the PID file is
// gone (or never written). FileExists provides an independent check.
func assertNoLiveChildAndNoPIDFile(t *testing.T, pm *ProcessManager, fileSvc fs.RuntimeFileService) {
	t.Helper()
	running, pid, err := pm.OperatorStatus()
	require.NoError(t, err, "OperatorStatus should not error after cleanup")
	assert.False(t, running, "no Gateway process should be running after failed start")
	assert.Zero(t, pid, "no PID should be recorded after failed start")

	exists, err := fileSvc.FileExists(context.Background(), pidRelPath())
	require.NoError(t, err)
	assert.False(t, exists, "no stale PID file should remain after failed start")
}

// TestStartOperator_PIDWriteFailureTerminatesChildAndLeavesNoPIDFile
// verifies that when writePID fails after cmd.Start() succeeds, stopFailedStart
// terminates the child process and no stale PID file is left behind. The
// .g8e/pids/ directory is made read-only so writePID fails, but
// copyBinaryToBinDir and cmd.Start() succeed because .g8e/bin/ and .g8e/logs/
// remain writable.
func TestStartOperator_PIDWriteFailureTerminatesChildAndLeavesNoPIDFile(t *testing.T) {
	fileSvc := newStartFailureFileSvc(t)

	// Make .g8e/pids/ read-only so writePID fails after cmd.Start() succeeds.
	pidsAbsPath := fileSvc.Resolve(constants.PidDirname)
	require.NoError(t, os.Chmod(pidsAbsPath, 0500))
	t.Cleanup(func() { _ = os.Chmod(pidsAbsPath, constants.PermDirStandard) })

	pm, err := NewProcessManager(fileSvc)
	require.NoError(t, err)

	startErr := pm.StartOperator(minimalStartOpts())
	require.Error(t, startErr, "StartOperator should fail when writePID fails")
	assert.ErrorIs(t, startErr, constants.ErrPIDWriteFailed,
		"error should wrap ErrPIDWriteFailed, got: %v", startErr)

	assertNoLiveChildAndNoPIDFile(t, pm, fileSvc)
}

// TestStartOperator_ProcessDeathDuringHealthCheckRemovesPIDFile verifies that
// when the child process exits before becoming healthy (detected by
// isProcessRunning on the first health-check iteration), stopFailedStart
// terminates/reaps the child and deletePID removes the PID file. The test
// binary exits immediately when re-executed (TestMain detects os.Args[1] ==
// "gw"), so the subprocess dies within the first HealthCheckInterval.
func TestStartOperator_ProcessDeathDuringHealthCheckRemovesPIDFile(t *testing.T) {
	fileSvc := newStartFailureFileSvc(t)

	pm, err := NewProcessManager(fileSvc)
	require.NoError(t, err)

	startErr := pm.StartOperator(minimalStartOpts())
	require.Error(t, startErr, "StartOperator should fail when the process never becomes healthy")
	assert.ErrorIs(t, startErr, constants.ErrProcessStartFailed,
		"error should wrap ErrProcessStartFailed, got: %v", startErr)

	assertNoLiveChildAndNoPIDFile(t, pm, fileSvc)
}

// freeTCPPort returns a currently free loopback TCP port.
func freeTCPPort(t *testing.T) int {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	port := ln.Addr().(*net.TCPAddr).Port
	require.NoError(t, ln.Close())
	return port
}

// bindForeignHealthServer binds a minimal health endpoint on port that
// reports a real but wrong PID, emulating a colliding process that answers
// g8e-shaped health checks (for example a re-executed test binary from
// another package that claimed the same free port).
func bindForeignHealthServer(t *testing.T, port int) {
	t.Helper()
	ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", port))
	require.NoError(t, err)
	mux := http.NewServeMux()
	mux.HandleFunc(constants.APIPaths.Health, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = fmt.Fprintf(w, `{"status":"ok","mode":"gateway","posture":"doctrine","pid":%d}`, os.Getpid())
	})
	srv := &http.Server{Handler: mux}
	go func() { _ = srv.Serve(ln) }()
	t.Cleanup(func() { _ = srv.Close() })
}

// bindForeignHealthServerAfterPIDWrite waits until StartOperator has
// completed its port scan and written the PID file, then claims the selected
// port with a foreign health server. The PID file is written after
// findAvailablePort returns, so the scan result is fixed at that point. The
// goroutine gives up silently once StartOperator returns or the deadline
// passes.
func bindForeignHealthServerAfterPIDWrite(t *testing.T, fileSvc fs.RuntimeFileService, port int, done <-chan struct{}) {
	t.Helper()
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		select {
		case <-done:
			return
		default:
		}
		exists, err := fileSvc.FileExists(context.Background(), pidRelPath())
		if err != nil {
			return
		}
		if exists {
			bindForeignHealthServer(t, port)
			return
		}
		time.Sleep(2 * time.Millisecond)
	}
}

// startOptsOnFreePorts returns OperatorStartOptions pinned to currently free
// ports so the test knows in advance which port StartOperator will select.
func startOptsOnFreePorts(t *testing.T) *OperatorStartOptions {
	t.Helper()
	opts := minimalStartOpts()
	opts.HTTPPort = freeTCPPort(t)
	opts.HTTPSPort = freeTCPPort(t)
	return opts
}

// TestStartOperator_ForeignHealthServerAfterChildExitIsRejected verifies that
// a foreign process answering g8e-shaped health checks on the selected port is
// not mistaken for the started child. The re-executed child exits immediately
// without binding the port, so a colliding listener can claim it; the health
// check must still fail closed because the responder is not the child.
func TestStartOperator_ForeignHealthServerAfterChildExitIsRejected(t *testing.T) {
	fileSvc := newStartFailureFileSvc(t)
	opts := startOptsOnFreePorts(t)
	httpPort := opts.HTTPPort

	pm, err := NewProcessManager(fileSvc)
	require.NoError(t, err)

	done := make(chan struct{})
	defer close(done)
	startErrCh := make(chan error, 1)
	go func() { startErrCh <- pm.StartOperator(opts) }()
	go bindForeignHealthServerAfterPIDWrite(t, fileSvc, httpPort, done)

	select {
	case startErr := <-startErrCh:
		require.Error(t, startErr, "StartOperator must not accept a foreign health server as the started child")
		assert.ErrorIs(t, startErr, constants.ErrProcessStartFailed,
			"error should wrap ErrProcessStartFailed, got: %v", startErr)
	case <-time.After(60 * time.Second):
		t.Fatal("StartOperator did not return")
	}

	assertNoLiveChildAndNoPIDFile(t, pm, fileSvc)
}

// TestStartOperator_ForeignHealthServerWhileChildAliveIsRejected verifies the
// same fail-closed behavior while the child is still running: the re-executed
// child in "hold" mode stays alive without binding the selected port, a
// foreign server claims it, and its health response must not satisfy the
// start health check.
func TestStartOperator_ForeignHealthServerWhileChildAliveIsRejected(t *testing.T) {
	t.Setenv("G8E_TEST_REEXEC", "hold")
	fileSvc := newStartFailureFileSvc(t)
	opts := startOptsOnFreePorts(t)
	httpPort := opts.HTTPPort

	pm, err := NewProcessManager(fileSvc)
	require.NoError(t, err)

	done := make(chan struct{})
	defer close(done)
	startErrCh := make(chan error, 1)
	go func() { startErrCh <- pm.StartOperator(opts) }()
	go bindForeignHealthServerAfterPIDWrite(t, fileSvc, httpPort, done)

	select {
	case startErr := <-startErrCh:
		require.Error(t, startErr, "StartOperator must not accept a foreign health server as the started child")
		assert.ErrorIs(t, startErr, constants.ErrProcessStartFailed,
			"error should wrap ErrProcessStartFailed, got: %v", startErr)
	case <-time.After(60 * time.Second):
		t.Fatal("StartOperator did not return")
	}

	assertNoLiveChildAndNoPIDFile(t, pm, fileSvc)
}
