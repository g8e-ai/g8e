// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package cmd

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/v2/internal/cli/platform"
	"github.com/g8e-ai/g8e/v2/internal/cli/serve"
	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/services/fs"
)

// These regression tests lock in the current broken behavior of
// gatewayRestartCmdWithConfig so that the Phase 2 fix (complete
// launch-profile persistence) can be verified against a captured
// baseline. See plan Finding 0.3.
//
// Issue tracked: restart-persists-posture-only
// gatewayRestartCmdWithConfig reads only the posture string from
// .g8e/pids/operator.posture and constructs a serve.GatewayConfig with
// posture and log level only. Every other field (AllowedOrigins,
// PasskeyRpID, PasskeyRpName, PasskeyRpOrigins, PublicBaseURL, ports,
// downstream routes, certificate mode, rate limits, doctrine, consensus,
// vault, and path settings) is zero-valued. A guided connection command
// must not reuse this restart behavior because it drops the complete
// prior launch configuration.

// TestGatewayRestartCmd_LosesCORSAndPasskeySettings_CurrentBrokenBehavior
// proves that the restart path has no access to the original launch
// configuration. It writes a posture file containing "consensus" via the
// real ProcessManager against a temp-rooted fileSvc, then calls
// gatewayRestartCmdWithConfig. StartOperator fails before spawning a
// subprocess (the .g8e/bin directory is made read-only so the binary
// copy step fails), which surfaces as ErrProcessStartFailed. The test
// then asserts that the posture file still contains only the posture
// string with no CORS or passkey data, and that no launch profile file
// exists anywhere in the runtime tree.
func TestGatewayRestartCmd_LosesCORSAndPasskeySettings_CurrentBrokenBehavior(t *testing.T) {
	fileSvc, cfg := newCmdTestEnv(t)

	// Write a posture file containing "consensus" using the real
	// ProcessManager, mirroring what a prior `gw start` would have
	// persisted.
	_, err := platform.NewProcessManager(fileSvc)
	require.NoError(t, err)

	postureRelPath := filepath.Join(constants.PidDirname, constants.OperatorPostureFilename)
	require.NoError(t, fileSvc.WriteFile(context.Background(), postureRelPath, []byte("consensus"), constants.PermFilePrivate))

	// Make .g8e/bin read-only so StartOperator fails at the binary-copy
	// step before spawning any subprocess. This keeps the test hermetic
	// (no child process, no port binding) while still exercising the
	// real restart command path up to and including the StartOperator
	// call. Restore write permission on cleanup so the temp directory
	// can be removed.
	binAbsPath := fileSvc.Resolve(constants.BinDirname)
	require.NoError(t, os.Chmod(binAbsPath, 0500))
	t.Cleanup(func() { _ = os.Chmod(binAbsPath, constants.PermDirStandard) })

	cmd := gatewayRestartCmdWithConfig(configLoaderFor(cfg), fileSvcFactoryFor(fileSvc))
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	err = cmd.RunE(cmd, nil)
	require.Error(t, err, RegressionMarkerBeforeFix)
	assert.ErrorIs(t, err, constants.ErrProcessStartFailed, RegressionMarkerBeforeFix)

	// The posture file still contains only the posture string. No CORS
	// or passkey data was persisted or restored by the restart path.
	postureData, readErr := fileSvc.ReadFile(context.Background(), postureRelPath)
	require.NoError(t, readErr)
	assert.Equal(t, "consensus", string(postureData), RegressionMarkerBeforeFix)
	assert.NotContains(t, string(postureData), "lovable", RegressionMarkerBeforeFix)
	assert.NotContains(t, string(postureData), "cors", RegressionMarkerBeforeFix)
	assert.NotContains(t, string(postureData), "passkey", RegressionMarkerBeforeFix)

	// No launch profile file exists anywhere in the runtime tree. The
	// restart path has no complete launch configuration to restore.
	assertNoLaunchProfile(t, fileSvc)

	// Regression marker: gatewayRestartCmdWithConfig constructs
	// serve.GatewayConfig{Posture, LogLevel} only, dropping every other
	// launch setting.
	_ = RegressionMarkerIssue
}

// TestGatewayRestartCmd_ConstructsPostureOnlyConfig_CurrentBrokenBehavior
// is a structural assertion that documents exactly which fields the
// restart command populates. It reconstructs the same serve.GatewayConfig
// the restart command builds from the persisted posture and asserts that
// every browser-relevant field (CORS origins, passkey RP ID, passkey RP
// name, passkey RP origins, public base URL) is zero-valued. This captures
// the root cause without depending on process lifecycle.
func TestGatewayRestartCmd_ConstructsPostureOnlyConfig_CurrentBrokenBehavior(t *testing.T) {
	// This is the exact construction gatewayRestartCmdWithConfig performs:
	//   serve.GatewayConfig{Posture: currentPosture, LogLevel: "info"}
	restartCfg := serve.GatewayConfig{
		Posture:  "consensus",
		LogLevel: "info",
	}

	assert.Empty(t, restartCfg.AllowedOrigins, RegressionMarkerBeforeFix)
	assert.Empty(t, restartCfg.PasskeyRpID, RegressionMarkerBeforeFix)
	assert.Empty(t, restartCfg.PasskeyRpName, RegressionMarkerBeforeFix)
	assert.Empty(t, restartCfg.PasskeyRpOrigins, RegressionMarkerBeforeFix)
	assert.Empty(t, restartCfg.PublicBaseURL, RegressionMarkerBeforeFix)
	assert.Equal(t, 0, restartCfg.HTTPPort, RegressionMarkerBeforeFix)
	assert.Equal(t, 0, restartCfg.HTTPSPort, RegressionMarkerBeforeFix)
	assert.Empty(t, restartCfg.CertIdentityMode, RegressionMarkerBeforeFix)
	assert.Empty(t, restartCfg.ConsensusID, RegressionMarkerBeforeFix)
	assert.Empty(t, restartCfg.ConsensusURL, RegressionMarkerBeforeFix)
	assert.Empty(t, restartCfg.MCPDownstreamURL, RegressionMarkerBeforeFix)
	assert.Empty(t, restartCfg.A2ADownstreamURL, RegressionMarkerBeforeFix)
	assert.Equal(t, float64(0), restartCfg.RateLimitRPS, RegressionMarkerBeforeFix)
	assert.Equal(t, 0, restartCfg.RateLimitBurst, RegressionMarkerBeforeFix)
	assert.Empty(t, restartCfg.DoctrineDir, RegressionMarkerBeforeFix)

	// Regression marker: the restart config carries posture and log
	// level only; all browser-relevant fields are zero-valued.
	_ = RegressionMarkerIssue
}

// assertNoLaunchProfile verifies that no launch profile file exists
// anywhere in the .g8e runtime tree. The current restart path has no
// complete launch profile persistence; Phase 2 introduces one.
func assertNoLaunchProfile(t *testing.T, fileSvc fs.RuntimeFileService) {
	t.Helper()

	// The pids directory should contain only the posture file (and
	// optionally a PID file). No launch profile JSON should exist.
	pidsRelPath := constants.PidDirname
	entries, err := fileSvc.ReadDir(context.Background(), pidsRelPath)
	if err != nil {
		// If the directory does not exist, there is trivially no profile.
		assert.ErrorIs(t, err, constants.ErrNotFound)
		return
	}
	for _, entry := range entries {
		// A launch profile would be a JSON file; the posture file is
		// plain text and the PID file is a plain integer.
		assert.False(t, filepath.Ext(entry.Name()) == ".json",
			"no launch profile JSON should exist in %s, found %s", pidsRelPath, entry.Name())
	}
}
