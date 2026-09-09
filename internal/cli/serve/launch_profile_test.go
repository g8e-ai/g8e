// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package serve

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	g8econfig "github.com/g8e-ai/g8e/v2/internal/config"
	"github.com/g8e-ai/g8e/v2/internal/constants"
)

// validTestGatewayConfig returns a GatewayConfig with every field populated
// to non-default values so round-trip tests can detect field loss.
func validTestGatewayConfig() GatewayConfig {
	return GatewayConfig{
		Posture:             g8econfig.PostureConsensus,
		HTTPPort:            8080,
		HTTPSPort:           8443,
		DataDir:             "/data",
		PKIDir:              "/pki",
		SecretsDir:          "/secrets",
		VaultDir:            "/vault",
		VaultKeyPath:        "/vault/key",
		PasskeyRpID:         "your-app.lovable.app",
		PasskeyRpName:       "g8e",
		PasskeyRpOrigins:    []string{"https://your-app.lovable.app"},
		RateLimitRPS:        5.0,
		RateLimitBurst:      10,
		LogLevel:            "info",
		CertIdentityMode:    "full",
		NetworkIdentityFile: "/tmp/ephemeral-identity.json",
		ConsensusID:         "trib-001",
		ConsensusURL:        "https://localhost:8443/consensus/v1/deliberate",
		ConsensusBootstrap:  "/etc/g8e/consensus-bootstrap.json",
		MCPDownstreamURL:    "http://downstream:3000/mcp",
		A2ADownstreamURL:    "http://downstream:3001/a2a",
		PublicBaseURL:       "https://your-app.lovable.app",
		AllowedOrigins:      []string{"https://your-app.lovable.app"},
		DoctrineDir:         "/etc/g8e/doctrine",
	}
}

func writeTestLaunchProfile(t *testing.T, fileSvc interface {
	WriteFile(context.Context, string, []byte, os.FileMode) error
}, cfg GatewayConfig) {
	t.Helper()
	cfg.NetworkIdentityFile = ""
	data, err := json.Marshal(GatewayLaunchProfile{Version: LaunchProfileVersion, Config: cfg})
	require.NoError(t, err)
	relPath := filepath.Join(constants.PidDirname, constants.OperatorLaunchProfileFilename)
	require.NoError(t, fileSvc.WriteFile(context.Background(), relPath, data, constants.PermFilePrivate))
}

func TestWriteLaunchProfile_RoundTripPreservesAllFields(t *testing.T) {
	fileSvc := newTestFileSvc(t)
	cfg := validTestGatewayConfig()

	require.NoError(t, WriteLaunchProfile(fileSvc, cfg))

	profile, err := ReadLaunchProfile(fileSvc)
	require.NoError(t, err)
	assert.Equal(t, LaunchProfileVersion, profile.Version)

	// NetworkIdentityFile is ephemeral — it must be cleared on write
	// and empty on read.
	assert.Empty(t, profile.Config.NetworkIdentityFile)

	// Every other field must round-trip exactly.
	assert.Equal(t, cfg.Posture, profile.Config.Posture)
	assert.Equal(t, cfg.HTTPPort, profile.Config.HTTPPort)
	assert.Equal(t, cfg.HTTPSPort, profile.Config.HTTPSPort)
	assert.Equal(t, cfg.DataDir, profile.Config.DataDir)
	assert.Equal(t, cfg.PKIDir, profile.Config.PKIDir)
	assert.Equal(t, cfg.SecretsDir, profile.Config.SecretsDir)
	assert.Equal(t, cfg.VaultDir, profile.Config.VaultDir)
	assert.Equal(t, cfg.VaultKeyPath, profile.Config.VaultKeyPath)
	assert.Equal(t, cfg.PasskeyRpID, profile.Config.PasskeyRpID)
	assert.Equal(t, cfg.PasskeyRpName, profile.Config.PasskeyRpName)
	assert.Equal(t, cfg.PasskeyRpOrigins, profile.Config.PasskeyRpOrigins)
	assert.Equal(t, cfg.RateLimitRPS, profile.Config.RateLimitRPS)
	assert.Equal(t, cfg.RateLimitBurst, profile.Config.RateLimitBurst)
	assert.Equal(t, cfg.LogLevel, profile.Config.LogLevel)
	assert.Equal(t, cfg.CertIdentityMode, profile.Config.CertIdentityMode)
	assert.Equal(t, cfg.ConsensusID, profile.Config.ConsensusID)
	assert.Equal(t, cfg.ConsensusURL, profile.Config.ConsensusURL)
	assert.Equal(t, cfg.ConsensusBootstrap, profile.Config.ConsensusBootstrap)
	assert.Equal(t, cfg.MCPDownstreamURL, profile.Config.MCPDownstreamURL)
	assert.Equal(t, cfg.A2ADownstreamURL, profile.Config.A2ADownstreamURL)
	assert.Equal(t, cfg.PublicBaseURL, profile.Config.PublicBaseURL)
	assert.Equal(t, cfg.AllowedOrigins, profile.Config.AllowedOrigins)
	assert.Equal(t, cfg.DoctrineDir, profile.Config.DoctrineDir)
}

func TestWriteLaunchProfile_PrivatePermissions(t *testing.T) {
	fileSvc := newTestFileSvc(t)
	cfg := validTestGatewayConfig()

	require.NoError(t, WriteLaunchProfile(fileSvc, cfg))

	profilePath := fileSvc.Resolve(filepath.Join(constants.PidDirname, constants.OperatorLaunchProfileFilename))
	info, err := os.Stat(profilePath)
	require.NoError(t, err)

	if runtime.GOOS != "windows" {
		assert.Equal(t, os.FileMode(constants.PermFilePrivate), info.Mode().Perm(),
			"launch profile must have private permissions (0600)")
	}
}

func TestReadLaunchProfile_MissingReturnsErrLaunchProfileMissing(t *testing.T) {
	fileSvc := newTestFileSvc(t)

	_, err := ReadLaunchProfile(fileSvc)
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrLaunchProfileMissing)
}

func TestReadLaunchProfile_CorruptedJSONReturnsErrLaunchProfileCorrupted(t *testing.T) {
	fileSvc := newTestFileSvc(t)

	relPath := filepath.Join(constants.PidDirname, constants.OperatorLaunchProfileFilename)
	require.NoError(t, fileSvc.WriteFile(context.Background(), relPath, []byte("{not valid json"), constants.PermFilePrivate))

	_, err := ReadLaunchProfile(fileSvc)
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrLaunchProfileCorrupted)
}

func TestReadLaunchProfile_UnknownFieldReturnsErrLaunchProfileCorrupted(t *testing.T) {
	fileSvc := newTestFileSvc(t)

	relPath := filepath.Join(constants.PidDirname, constants.OperatorLaunchProfileFilename)
	data := []byte(`{"version":1,"config":{"Posture":"doctrine"},"unexpected":true}`)
	require.NoError(t, fileSvc.WriteFile(context.Background(), relPath, data, constants.PermFilePrivate))

	_, err := ReadLaunchProfile(fileSvc)
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrLaunchProfileCorrupted)
}

func TestReadLaunchProfile_UnknownVersionReturnsErrLaunchProfileVersionUnsupported(t *testing.T) {
	fileSvc := newTestFileSvc(t)

	profile := GatewayLaunchProfile{
		Version: 999,
		Config:  validTestGatewayConfig(),
	}
	data, err := json.Marshal(profile)
	require.NoError(t, err)

	relPath := filepath.Join(constants.PidDirname, constants.OperatorLaunchProfileFilename)
	require.NoError(t, fileSvc.WriteFile(context.Background(), relPath, data, constants.PermFilePrivate))

	_, err = ReadLaunchProfile(fileSvc)
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrLaunchProfileVersionUnsupported)
}

func TestReadLaunchProfile_InvalidPostureReturnsErrLaunchProfileInvalid(t *testing.T) {
	fileSvc := newTestFileSvc(t)

	cfg := validTestGatewayConfig()
	cfg.Posture = g8econfig.GatewayPosture("bogus")
	writeTestLaunchProfile(t, fileSvc, cfg)

	_, err := ReadLaunchProfile(fileSvc)
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrLaunchProfileInvalid)
}

func TestReadLaunchProfile_EmptyPostureReturnsErrLaunchProfileInvalid(t *testing.T) {
	fileSvc := newTestFileSvc(t)

	cfg := validTestGatewayConfig()
	cfg.Posture = ""
	writeTestLaunchProfile(t, fileSvc, cfg)

	_, err := ReadLaunchProfile(fileSvc)
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrLaunchProfileInvalid)
}

func TestReadLaunchProfile_NegativeHTTPPortReturnsErrLaunchProfileInvalid(t *testing.T) {
	fileSvc := newTestFileSvc(t)

	cfg := validTestGatewayConfig()
	cfg.HTTPPort = -1
	writeTestLaunchProfile(t, fileSvc, cfg)

	_, err := ReadLaunchProfile(fileSvc)
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrLaunchProfileInvalid)
}

func TestReadLaunchProfile_NegativeHTTPSPortReturnsErrLaunchProfileInvalid(t *testing.T) {
	fileSvc := newTestFileSvc(t)

	cfg := validTestGatewayConfig()
	cfg.HTTPSPort = -1
	writeTestLaunchProfile(t, fileSvc, cfg)

	_, err := ReadLaunchProfile(fileSvc)
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrLaunchProfileInvalid)
}

func TestReadLaunchProfile_NegativeRateLimitRPSReturnsErrLaunchProfileInvalid(t *testing.T) {
	fileSvc := newTestFileSvc(t)

	cfg := validTestGatewayConfig()
	cfg.RateLimitRPS = -1.0
	writeTestLaunchProfile(t, fileSvc, cfg)

	_, err := ReadLaunchProfile(fileSvc)
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrLaunchProfileInvalid)
}

func TestReadLaunchProfile_NegativeRateLimitBurstReturnsErrLaunchProfileInvalid(t *testing.T) {
	fileSvc := newTestFileSvc(t)

	cfg := validTestGatewayConfig()
	cfg.RateLimitBurst = -1
	writeTestLaunchProfile(t, fileSvc, cfg)

	_, err := ReadLaunchProfile(fileSvc)
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrLaunchProfileInvalid)
}

func TestReadLaunchProfile_StaleNetworkIdentityFileReturnsErrLaunchProfileInvalid(t *testing.T) {
	fileSvc := newTestFileSvc(t)

	// Manually write a profile with a non-empty NetworkIdentityFile,
	// simulating tampering or an incompatible version. WriteLaunchProfile
	// clears this field, so a non-empty value in a persisted profile is
	// evidence of tampering.
	profile := GatewayLaunchProfile{
		Version: LaunchProfileVersion,
		Config:  validTestGatewayConfig(),
	}
	profile.Config.NetworkIdentityFile = "/tmp/stale-identity.json"
	data, err := json.Marshal(profile)
	require.NoError(t, err)

	relPath := filepath.Join(constants.PidDirname, constants.OperatorLaunchProfileFilename)
	require.NoError(t, fileSvc.WriteFile(context.Background(), relPath, data, constants.PermFilePrivate))

	_, err = ReadLaunchProfile(fileSvc)
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrLaunchProfileInvalid)
}

func TestDeleteLaunchProfile_RemovesFile(t *testing.T) {
	fileSvc := newTestFileSvc(t)
	cfg := validTestGatewayConfig()

	require.NoError(t, WriteLaunchProfile(fileSvc, cfg))

	exists, err := LaunchProfileExists(context.Background(), fileSvc)
	require.NoError(t, err)
	assert.True(t, exists)

	require.NoError(t, DeleteLaunchProfile(fileSvc))

	exists, err = LaunchProfileExists(context.Background(), fileSvc)
	require.NoError(t, err)
	assert.False(t, exists)
}

func TestDeleteLaunchProfile_NoOpWhenMissing(t *testing.T) {
	fileSvc := newTestFileSvc(t)

	err := DeleteLaunchProfile(fileSvc)
	assert.NoError(t, err, "deleting a non-existent profile should not error")
}

func TestLaunchProfileExists_FalseWhenNoProfile(t *testing.T) {
	fileSvc := newTestFileSvc(t)

	exists, err := LaunchProfileExists(context.Background(), fileSvc)
	require.NoError(t, err)
	assert.False(t, exists)
}

func TestValidateLaunchProfile_AcceptsAllValidPostures(t *testing.T) {
	validPostures := []g8econfig.GatewayPosture{
		g8econfig.PostureDoctrine,
		g8econfig.PostureConsensus,
		g8econfig.PostureRatify,
		g8econfig.PostureNotary,
	}
	for _, p := range validPostures {
		cfg := validTestGatewayConfig()
		cfg.Posture = p
		// ValidateLaunchProfile validates a persisted profile, which
		// always has NetworkIdentityFile cleared by WriteLaunchProfile.
		cfg.NetworkIdentityFile = ""
		profile := GatewayLaunchProfile{Version: LaunchProfileVersion, Config: cfg}
		assert.NoError(t, ValidateLaunchProfile(profile), "posture %s should be valid", p)
	}
}

func TestValidateLaunchProfile_AcceptsZeroPorts(t *testing.T) {
	fileSvc := newTestFileSvc(t)
	cfg := validTestGatewayConfig()
	cfg.HTTPPort = 0
	cfg.HTTPSPort = 0
	cfg.RateLimitRPS = 0
	cfg.RateLimitBurst = 0
	require.NoError(t, WriteLaunchProfile(fileSvc, cfg))

	_, err := ReadLaunchProfile(fileSvc)
	require.NoError(t, err, "zero ports and rate limits are valid (meaning 'use defaults')")
}
