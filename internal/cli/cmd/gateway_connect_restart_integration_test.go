// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

//go:build integration

package cmd

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/v2/internal/cli/auth"
	"github.com/g8e-ai/g8e/v2/internal/cli/serve"
	"github.com/g8e-ai/g8e/v2/internal/constants"
)

// TestGatewayConnectCmd_RunningGatewayWithDifferingConfigYesFlagAttemptsRestart
// is a Tier 2 integration test that verifies --yes approves the restart
// non-interactively. The test spawns a dummy subprocess (sleep) and writes its
// PID to the PID file so OperatorStatus reports the Gateway as running.
// connectRestartGateway calls StopOperator which kills the dummy subprocess
// (not the test process), then StartOperator which fails at the binary-copy
// step because .g8e/bin is read-only. This proves the command got past the
// consent gate, stopped the old process, and attempted to start the new one.
//
// This test cannot run in Tier 1 because StopOperator sends SIGTERM to the
// process identified by the PID file — writing the test process's own PID
// would kill the test. Spawning a real subprocess is an external dependency
// that places this test in Tier 2.
func TestGatewayConnectCmd_RunningGatewayWithDifferingConfigYesFlagAttemptsRestart(t *testing.T) {
	fileSvc, cfg := newCmdTestEnv(t)

	// Spawn a dummy subprocess whose PID will be written to the PID file.
	// StopOperator will kill this process, not the test process.
	dummyCmd := exec.Command("sleep", "300")
	require.NoError(t, dummyCmd.Start())
	t.Cleanup(func() {
		_ = dummyCmd.Process.Kill()
		_, _ = dummyCmd.Process.Wait()
	})

	// Write the dummy subprocess's PID to the PID file.
	pidRelPath := filepath.Join(constants.PidDirname, constants.OperatorPIDFilename)
	require.NoError(t, fileSvc.WriteFile(context.Background(), pidRelPath,
		[]byte(strconv.Itoa(dummyCmd.Process.Pid)), constants.PermFilePrivate))

	// Write a non-matching launch profile (different CORS origin).
	profileCfg := serve.GatewayConfig{
		Posture:          "doctrine",
		LogLevel:         "info",
		CertIdentityMode: "localhost",
		AllowedOrigins:   []string{"https://old-app.lovable.app"},
		PasskeyRpOrigins: []string{"https://old-app.lovable.app"},
		PasskeyRpID:      "old-app.lovable.app",
		PasskeyRpName:    "g8e",
	}
	require.NoError(t, serve.WriteLaunchProfile(fileSvc, profileCfg))

	// Make .g8e/bin read-only so StartOperator fails at the binary-copy
	// step after StopOperator kills the dummy subprocess.
	binAbsPath := fileSvc.Resolve(constants.BinDirname)
	require.NoError(t, os.MkdirAll(binAbsPath, constants.PermDirStandard))
	require.NoError(t, os.Chmod(binAbsPath, 0500))
	t.Cleanup(func() { _ = os.Chmod(binAbsPath, constants.PermDirStandard) })

	deps := connectDeps{
		trustInstaller: &stubTrustInstaller{},
		discoveryFetcher: func(context.Context, string, func() time.Time) (auth.TrustDiscoveryResult, error) {
			panic("discoveryFetcher should not be called when StartOperator fails")
		},
		confirm: func(string) bool { panic("confirm should not be called with --yes") },
	}

	cmd := gatewayConnectCmdWithConfig(configLoaderFor(cfg), fileSvcFactoryFor(fileSvc), deps)
	require.NoError(t, cmd.Flags().Set("yes", "true"))
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	err := cmd.RunE(cmd, []string{"https://your-app.lovable.app"})
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrProcessStartFailed)
	assert.Contains(t, buf.String(), "Stopping g8e Gateway")
}
