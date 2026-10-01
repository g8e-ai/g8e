// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package mcp

import (
	"context"
	"log/slog"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/v2/internal/cli/serve"
	g8econfig "github.com/g8e-ai/g8e/v2/internal/config"
	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/services/fs"
	"github.com/g8e-ai/g8e/v2/internal/testutil"
)

func newPostureTestFileSvc(t *testing.T) fs.RuntimeFileService {
	t.Helper()
	fileSvc, err := fs.NewRuntimeFileService(testutil.TempDir(t), slog.Default())
	require.NoError(t, err)
	require.NoError(t, fileSvc.CreateRuntimeTree(context.Background()))
	return fileSvc
}

func writeProfile(t *testing.T, fileSvc fs.RuntimeFileService, cfg serve.GatewayConfig) {
	t.Helper()
	require.NoError(t, serve.WriteLaunchProfile(fileSvc, cfg))
}

func TestValidateRequestedPosture(t *testing.T) {
	for _, ok := range []string{"", constants.PostureDoctrine, constants.PostureConsensus, constants.PostureRatify, constants.PostureNotary} {
		require.NoError(t, validateRequestedPosture(ok), "%q", ok)
	}
	for _, bad := range []string{"strict", "DOCTRINE", "notary "} {
		require.ErrorIs(t, validateRequestedPosture(bad), constants.ErrInvalidPosture, "%q", bad)
	}
}

func TestStartGatewayIfNeeded_RejectsInvalidPostureBeforeAnyWork(t *testing.T) {
	err := startGatewayIfNeeded(func(string, *slog.Logger) (fs.RuntimeFileService, error) {
		t.Fatal("file service must not be built for an invalid posture")
		return nil, nil
	}, "strict")
	require.ErrorIs(t, err, constants.ErrInvalidPosture)
}

func TestResolveGatewayLaunchConfig(t *testing.T) {
	existing := serve.GatewayConfig{
		Posture:          g8econfig.PostureConsensus,
		HTTPPort:         8080,
		HTTPSPort:        8443,
		LogLevel:         "debug",
		CertIdentityMode: "full",
		AllowedOrigins:   []string{"https://console.example"},
	}

	tests := []struct {
		name        string
		profile     *serve.GatewayConfig
		requested   string
		wantPosture g8econfig.GatewayPosture
		wantBase    bool // whether the profile's other settings must carry over
	}{
		{name: "no profile, no request defaults to doctrine", wantPosture: g8econfig.PostureDoctrine},
		{name: "no profile, explicit request", requested: constants.PostureNotary, wantPosture: g8econfig.PostureNotary},
		{name: "profile posture is kept when unset", profile: &existing, wantPosture: g8econfig.PostureConsensus, wantBase: true},
		{name: "explicit request overrides profile posture", profile: &existing, requested: constants.PostureRatify, wantPosture: g8econfig.PostureRatify, wantBase: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fileSvc := newPostureTestFileSvc(t)
			if tt.profile != nil {
				writeProfile(t, fileSvc, *tt.profile)
			}

			cfg, err := resolveGatewayLaunchConfig(fileSvc, tt.requested)
			require.NoError(t, err)
			assert.Equal(t, tt.wantPosture, cfg.Posture)
			if tt.wantBase {
				assert.Equal(t, 8443, cfg.HTTPSPort, "the previous gateway's ports must survive")
				assert.Equal(t, []string{"https://console.example"}, cfg.AllowedOrigins)
			} else {
				assert.Equal(t, "localhost", cfg.CertIdentityMode)
			}
		})
	}
}

func TestResolveGatewayLaunchConfig_CorruptProfileFailsClosed(t *testing.T) {
	fileSvc := newPostureTestFileSvc(t)
	require.NoError(t, fileSvc.WriteFile(context.Background(), filepath.Join(constants.PidDirname, constants.OperatorLaunchProfileFilename), []byte(`{not json`), constants.PermFilePrivate))

	_, err := resolveGatewayLaunchConfig(fileSvc, constants.PostureNotary)
	require.ErrorIs(t, err, constants.ErrLaunchProfileCorrupted)
}

func TestConfirmRunningGatewayPosture(t *testing.T) {
	t.Run("no explicit request never checks", func(t *testing.T) {
		require.NoError(t, confirmRunningGatewayPosture(newPostureTestFileSvc(t), ""))
	})

	t.Run("matching profile passes", func(t *testing.T) {
		fileSvc := newPostureTestFileSvc(t)
		writeProfile(t, fileSvc, serve.GatewayConfig{Posture: g8econfig.PostureNotary, LogLevel: "info"})
		require.NoError(t, confirmRunningGatewayPosture(fileSvc, constants.PostureNotary))
	})

	t.Run("different posture fails closed", func(t *testing.T) {
		fileSvc := newPostureTestFileSvc(t)
		writeProfile(t, fileSvc, serve.GatewayConfig{Posture: g8econfig.PostureDoctrine, LogLevel: "info"})
		err := confirmRunningGatewayPosture(fileSvc, constants.PostureNotary)
		require.ErrorIs(t, err, constants.ErrGatewayPostureMismatch)
	})

	t.Run("unverifiable posture fails closed", func(t *testing.T) {
		err := confirmRunningGatewayPosture(newPostureTestFileSvc(t), constants.PostureNotary)
		require.ErrorIs(t, err, constants.ErrGatewayPostureMismatch)
		require.ErrorIs(t, err, constants.ErrLaunchProfileMissing)
	})
}

func TestAgentRunCmd_ExposesPostureFlagWithNoHardcodedDefault(t *testing.T) {
	flag := agentRunCmd().Flags().Lookup("posture")
	require.NotNil(t, flag)
	assert.Empty(t, flag.DefValue, "the default comes from the previous gateway or the platform default, not the flag")
}
