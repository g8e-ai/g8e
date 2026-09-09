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
	"log/slog"
	"os"
	"path/filepath"
	"testing"

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
