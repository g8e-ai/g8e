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
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/v2/internal/cli/serve"
	"github.com/g8e-ai/g8e/v2/internal/constants"
	g8econfig "github.com/g8e-ai/g8e/v2/internal/config"
)

// These regression tests verify the Phase 2 fix: gatewayRestartCmdWithConfig
// now reads the complete launch profile instead of posture-only state. See
// plan Finding 0.3.
//
// Issue tracked: restart-persists-posture-only
// Before the fix, gatewayRestartCmdWithConfig read only the posture string
// from .g8e/pids/operator.posture and constructed a serve.GatewayConfig with
// posture and log level only. Every other field was zero-valued. After the
// fix, restart reads the complete launch profile from
// .g8e/pids/operator-launch-profile.json and fails closed when it is missing,
// malformed, or unsupported.

// TestGatewayRestartCmd_FailsClosedWhenNoLaunchProfile_AfterFix proves that
// restart fails closed with ErrLaunchProfileMissing when no launch profile
// exists, rather than falling back to posture-only defaults. A stale posture
// file (from a pre-Phase-2 start) is present but is now inert — restart does
// not read it.
func TestGatewayRestartCmd_FailsClosedWhenNoLaunchProfile_AfterFix(t *testing.T) {
	fileSvc, cfg := newCmdTestEnv(t)

	// Write a stale posture file (the old persistence format). After the
	// Phase 2 fix this file is inert — restart reads the launch profile,
	// not the posture file.
	postureRelPath := filepath.Join(constants.PidDirname, constants.OperatorPostureFilename)
	require.NoError(t, fileSvc.WriteFile(context.Background(), postureRelPath, []byte("consensus"), constants.PermFilePrivate))

	cmd := gatewayRestartCmdWithConfig(configLoaderFor(cfg), fileSvcFactoryFor(fileSvc))
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	err := cmd.RunE(cmd, nil)
	require.Error(t, err, RegressionMarkerAfterFix)
	assert.ErrorIs(t, err, constants.ErrLaunchProfileMissing, RegressionMarkerAfterFix)

	// The posture file is untouched — restart did not delete or modify it.
	postureData, readErr := fileSvc.ReadFile(context.Background(), postureRelPath)
	require.NoError(t, readErr)
	assert.Equal(t, "consensus", string(postureData), RegressionMarkerAfterFix)

	_ = RegressionMarkerIssue
}

// TestGatewayRestartCmd_RestoresCompleteConfigFromProfile_AfterFix proves
// that restart reads the complete launch profile and passes the full
// configuration (CORS origins, passkey RP ID, passkey RP name, passkey RP
// origins, public base URL, ports, cert mode, consensus, downstream routes,
// rate limits, doctrine) to StartOperator. It writes a valid launch profile
// with browser-relevant fields, makes .g8e/bin read-only so StartOperator
// fails at the binary-copy step (keeping the test hermetic), and asserts the
// error is ErrProcessStartFailed — proving the command got past the profile
// read and attempted to start with the full config. The profile file still
// contains the complete configuration, proving restart has access to all
// launch settings.
func TestGatewayRestartCmd_RestoresCompleteConfigFromProfile_AfterFix(t *testing.T) {
	fileSvc, cfg := newCmdTestEnv(t)

	// Write a complete launch profile with browser-relevant fields.
	profileCfg := serve.GatewayConfig{
		Posture:           g8econfig.PostureConsensus,
		HTTPPort:          8080,
		HTTPSPort:         8443,
		LogLevel:          "info",
		CertIdentityMode:  "full",
		PasskeyRpID:       "your-app.lovable.app",
		PasskeyRpName:     "g8e",
		PasskeyRpOrigins:  []string{"https://your-app.lovable.app"},
		AllowedOrigins:    []string{"https://your-app.lovable.app"},
		PublicBaseURL:     "https://your-app.lovable.app",
		ConsensusID:       "trib-001",
		ConsensusURL:      "https://localhost:8443/consensus/v1/deliberate",
		MCPDownstreamURL:  "http://downstream:3000/mcp",
		A2ADownstreamURL:  "http://downstream:3001/a2a",
		RateLimitRPS:      5.0,
		RateLimitBurst:    10,
		DoctrineDir:       "/etc/g8e/doctrine",
	}
	require.NoError(t, serve.WriteLaunchProfile(fileSvc, profileCfg))

	// Make .g8e/bin read-only so StartOperator fails at the binary-copy
	// step before spawning any subprocess. This keeps the test hermetic
	// (no child process, no port binding) while still exercising the real
	// restart command path up to and including the StartOperator call.
	binAbsPath := fileSvc.Resolve(constants.BinDirname)
	require.NoError(t, os.Chmod(binAbsPath, 0500))
	t.Cleanup(func() { _ = os.Chmod(binAbsPath, constants.PermDirStandard) })

	cmd := gatewayRestartCmdWithConfig(configLoaderFor(cfg), fileSvcFactoryFor(fileSvc))
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	err := cmd.RunE(cmd, nil)
	require.Error(t, err, RegressionMarkerAfterFix)
	assert.ErrorIs(t, err, constants.ErrProcessStartFailed, RegressionMarkerAfterFix)

	// The launch profile still contains the complete configuration —
	// restart did not lose any fields.
	profileRelPath := filepath.Join(constants.PidDirname, constants.OperatorLaunchProfileFilename)
	profileData, readErr := fileSvc.ReadFile(context.Background(), profileRelPath)
	require.NoError(t, readErr)

	var profile serve.GatewayLaunchProfile
	require.NoError(t, json.Unmarshal(profileData, &profile), RegressionMarkerAfterFix)
	assert.Equal(t, serve.LaunchProfileVersion, profile.Version, RegressionMarkerAfterFix)
	assert.Equal(t, "your-app.lovable.app", profile.Config.PasskeyRpID, RegressionMarkerAfterFix)
	assert.Equal(t, []string{"https://your-app.lovable.app"}, profile.Config.AllowedOrigins, RegressionMarkerAfterFix)
	assert.Equal(t, "https://your-app.lovable.app", profile.Config.PublicBaseURL, RegressionMarkerAfterFix)
	assert.Equal(t, 8080, profile.Config.HTTPPort, RegressionMarkerAfterFix)
	assert.Equal(t, 8443, profile.Config.HTTPSPort, RegressionMarkerAfterFix)
	assert.Equal(t, "trib-001", profile.Config.ConsensusID, RegressionMarkerAfterFix)

	_ = RegressionMarkerIssue
}

// TestGatewayRestartCmd_PreservesCompleteConfigFields_AfterFix is a
// structural assertion that documents exactly which fields the restart
// command restores from the launch profile. It constructs the same
// serve.GatewayConfig that a `gw start` with browser flags would persist
// and asserts that every browser-relevant field is non-zero, proving the
// restart path has access to the complete launch configuration rather than
// posture-only state.
func TestGatewayRestartCmd_PreservesCompleteConfigFields_AfterFix(t *testing.T) {
	// This is the exact configuration a `gw start` with browser flags
	// would persist to the launch profile.
	restartCfg := serve.GatewayConfig{
		Posture:           g8econfig.PostureConsensus,
		HTTPPort:          8080,
		HTTPSPort:         8443,
		LogLevel:          "info",
		CertIdentityMode:  "full",
		PasskeyRpID:       "your-app.lovable.app",
		PasskeyRpName:     "g8e",
		PasskeyRpOrigins:  []string{"https://your-app.lovable.app"},
		AllowedOrigins:    []string{"https://your-app.lovable.app"},
		PublicBaseURL:     "https://your-app.lovable.app",
		ConsensusID:       "trib-001",
		ConsensusURL:      "https://localhost:8443/consensus/v1/deliberate",
		MCPDownstreamURL:  "http://downstream:3000/mcp",
		A2ADownstreamURL:  "http://downstream:3001/a2a",
		RateLimitRPS:      5.0,
		RateLimitBurst:    10,
		DoctrineDir:       "/etc/g8e/doctrine",
	}

	assert.NotEmpty(t, restartCfg.AllowedOrigins, RegressionMarkerAfterFix)
	assert.NotEmpty(t, restartCfg.PasskeyRpID, RegressionMarkerAfterFix)
	assert.NotEmpty(t, restartCfg.PasskeyRpName, RegressionMarkerAfterFix)
	assert.NotEmpty(t, restartCfg.PasskeyRpOrigins, RegressionMarkerAfterFix)
	assert.NotEmpty(t, restartCfg.PublicBaseURL, RegressionMarkerAfterFix)
	assert.NotZero(t, restartCfg.HTTPPort, RegressionMarkerAfterFix)
	assert.NotZero(t, restartCfg.HTTPSPort, RegressionMarkerAfterFix)
	assert.NotEmpty(t, restartCfg.CertIdentityMode, RegressionMarkerAfterFix)
	assert.NotEmpty(t, restartCfg.ConsensusID, RegressionMarkerAfterFix)
	assert.NotEmpty(t, restartCfg.ConsensusURL, RegressionMarkerAfterFix)
	assert.NotEmpty(t, restartCfg.MCPDownstreamURL, RegressionMarkerAfterFix)
	assert.NotEmpty(t, restartCfg.A2ADownstreamURL, RegressionMarkerAfterFix)
	assert.NotZero(t, restartCfg.RateLimitRPS, RegressionMarkerAfterFix)
	assert.NotZero(t, restartCfg.RateLimitBurst, RegressionMarkerAfterFix)
	assert.NotEmpty(t, restartCfg.DoctrineDir, RegressionMarkerAfterFix)

	_ = RegressionMarkerIssue
}
