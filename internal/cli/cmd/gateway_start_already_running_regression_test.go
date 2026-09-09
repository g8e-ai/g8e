// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version  2.0.

package cmd

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/v2/internal/constants"
)

// These regression tests lock in the current broken behavior of
// gatewayStartCmdWithConfig when a Gateway is already running so that
// the Phase 4 fix (gw connect state machine) can be verified against a
// captured baseline. See plan Finding 0.4.
//
// Issue tracked: start-no-op-when-running-ignores-new-flags
// gatewayStartCmdWithConfig calls pm.OperatorStatus(); when running is
// true it prints "g8e Gateway is already running (PID: %d)" and returns
// nil without comparing the supplied --cors-origin/--passkey-rp-* flags
// against the running process configuration. A syntactically correct
// command appears successful while the old configuration remains active.

// TestGatewayStartCmd_ReturnsSuccessWhenAlreadyRunning_IgnoresNewBrowserFlags_CurrentBrokenBehavior
// proves that gw start returns success when a Gateway is already running
// even when new browser flags are supplied that do not match the running
// configuration. It pre-writes a PID file pointing at the current test
// process PID (which is alive), so OperatorStatus reports running. It
// then invokes gatewayStartCmdWithConfig with --cors-origin set to a new
// origin and asserts the command returns nil and prints "already
// running", proving the new origin is silently ignored.
func TestGatewayStartCmd_ReturnsSuccessWhenAlreadyRunning_IgnoresNewBrowserFlags_CurrentBrokenBehavior(t *testing.T) {
	fileSvc, cfg := newCmdTestEnv(t)

	// Write a PID file pointing at the current test process PID, which
	// is alive. OperatorStatus reads this file and checks whether the
	// PID is running; since the test process is alive, it reports
	// running=true.
	currentPID := os.Getpid()
	pidRelPath := filepath.Join(constants.PidDirname, constants.OperatorPIDFilename)
	require.NoError(t, fileSvc.WriteFile(context.Background(), pidRelPath, []byte(strconv.Itoa(currentPID)), constants.PermFilePrivate))

	cmd := gatewayStartCmdWithConfig(configLoaderFor(cfg), fileSvcFactoryFor(fileSvc), nil)
	cmd.SetArgs([]string{"--cors-origin", "https://new-origin.lovable.app"})
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	err := cmd.Execute()
	require.NoError(t, err, RegressionMarkerBeforeFix)

	output := buf.String()
	assert.Contains(t, output, "already running", RegressionMarkerBeforeFix)

	// The new CORS origin must NOT appear in the output as an applied
	// configuration. The command silently ignores it.
	assert.NotContains(t, output, "new-origin.lovable.app", RegressionMarkerBeforeFix)

	// Regression marker: gatewayStartCmdWithConfig returns nil when
	// already running without applying or acknowledging new browser flags.
	_ = RegressionMarkerIssue
}

// TestGatewayStartCmd_AlreadyRunningDoesNotRestartOrReconfigure_CurrentBrokenBehavior
// is a companion assertion proving that no PID file rewrite or posture
// file change occurs when the command takes the already-running path.
// The PID file still points at the original PID, confirming no restart
// was attempted.
func TestGatewayStartCmd_AlreadyRunningDoesNotRestartOrReconfigure_CurrentBrokenBehavior(t *testing.T) {
	fileSvc, cfg := newCmdTestEnv(t)

	originalPID := os.Getpid()
	pidRelPath := filepath.Join(constants.PidDirname, constants.OperatorPIDFilename)
	require.NoError(t, fileSvc.WriteFile(context.Background(), pidRelPath, []byte(strconv.Itoa(originalPID)), constants.PermFilePrivate))

	cmd := gatewayStartCmdWithConfig(configLoaderFor(cfg), fileSvcFactoryFor(fileSvc), nil)
	cmd.SetArgs([]string{"--cors-origin", "https://changed-origin.lovable.app"})
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	require.NoError(t, cmd.Execute(), RegressionMarkerBeforeFix)

	// The PID file is unchanged — no restart occurred.
	pidData, err := fileSvc.ReadFile(context.Background(), pidRelPath)
	require.NoError(t, err)
	assert.Equal(t, strconv.Itoa(originalPID), string(pidData), RegressionMarkerBeforeFix)

	// Regression marker: the already-running path does not rewrite the
	// PID file or attempt a restart.
	_ = RegressionMarkerIssue
}
