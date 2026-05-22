// Copyright (c) 2026 Lateralus Labs, LLC.
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/g8e-ai/g8e/services/g8eo/internal/constants"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// Load
// ---------------------------------------------------------------------------

func TestLoad_Defaults(t *testing.T) {
	wantWorkDir := FindProjectRoot()
	if wantWorkDir == "" {
		var err error
		wantWorkDir, err = os.Getwd()
		require.NoError(t, err)
	}

	cfg, err := Load(LoadOptions{
		APIKey:           "test-key",
		OperatorEndpoint: constants.DefaultEndpoint,
	})
	require.NoError(t, err)
	require.NotNil(t, cfg)

	assert.Equal(t, constants.Status.ComponentName.G8EO, cfg.ComponentName)
	assert.Equal(t, "g8e", cfg.ProjectID)
	assert.Equal(t, 25, cfg.MaxConcurrentTasks)
	assert.Equal(t, 2048, cfg.MaxMemoryMB)
	assert.Equal(t, 30*time.Second, cfg.HeartbeatInterval)
	assert.Equal(t, int64(1024), cfg.LocalStoreMaxSizeMB)
	assert.Equal(t, 30, cfg.LocalStoreRetentionDays)
	assert.Equal(t, constants.Ports.OperatorHttps, cfg.HTTPPort)

	// WorkDir defaults to the project root when --working-dir is not supplied
	assert.Equal(t, wantWorkDir, cfg.WorkDir)
	// LocalStoreDBPath is anchored to WorkDir
	assert.Equal(t, filepath.Join(wantWorkDir, ".g8e", "local_state.db"), cfg.LocalStoreDBPath)
	assert.True(t, filepath.IsAbs(cfg.LocalStoreDBPath))
}

func TestLoad_WorkDir_Flag(t *testing.T) {
	tmpDir := t.TempDir()

	cfg, err := Load(LoadOptions{
		APIKey:           "test-key",
		OperatorEndpoint: constants.DefaultEndpoint,
		WorkDir:          tmpDir,
	})
	require.NoError(t, err)

	assert.Equal(t, tmpDir, cfg.WorkDir)
	assert.Equal(t, filepath.Join(tmpDir, ".g8e", "local_state.db"), cfg.LocalStoreDBPath)
	assert.True(t, strings.HasPrefix(cfg.LocalStoreDBPath, tmpDir))
}

func TestLoad_FieldPassthrough(t *testing.T) {
	cfg, err := Load(LoadOptions{
		APIKey:           "my-key",
		OperatorEndpoint: constants.DefaultEndpoint,
	})
	require.NoError(t, err)

	assert.Equal(t, "my-key", cfg.APIKey)
	assert.Equal(t, constants.DefaultEndpoint, cfg.Endpoint)
}

func TestLoad_HTTPPortOverride(t *testing.T) {
	cfg, err := Load(LoadOptions{
		APIKey:           "k",
		OperatorEndpoint: constants.DefaultEndpoint,
		HTTPPort:         constants.Ports.G8eeHttps,
	})
	require.NoError(t, err)
	assert.Equal(t, constants.Ports.G8eeHttps, cfg.HTTPPort)
}

func TestLoad_TLSServerName(t *testing.T) {
	tests := []struct {
		name           string
		endpoint       string
		wantServerName string
	}{
		{
			name:           "hostname clears TLSServerName",
			endpoint:       constants.DefaultEndpoint,
			wantServerName: "",
		},
		{
			name:           "IPv4 sets TLSServerName",
			endpoint:       "10.0.1.42",
			wantServerName: constants.DefaultEndpoint,
		},
		{
			name:           "IPv6 sets TLSServerName",
			endpoint:       "::1",
			wantServerName: constants.DefaultEndpoint,
		},
		{
			name:           "full IPv4 address sets TLSServerName",
			endpoint:       "192.168.100.200",
			wantServerName: constants.DefaultEndpoint,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg, err := Load(LoadOptions{
				APIKey:           "k",
				OperatorEndpoint: tt.endpoint,
			})
			require.NoError(t, err)
			assert.Equal(t, tt.wantServerName, cfg.TLSServerName)
		})
	}
}

func TestLoad_ValidationErrors(t *testing.T) {
	tests := []struct {
		name        string
		opts        LoadOptions
		errContains string
	}{
		{
			name: "missing api key",
			opts: LoadOptions{
				OperatorEndpoint: constants.DefaultEndpoint,
			},
			errContains: "APIKey is required",
		},
		{
			name: "missing operator endpoint",
			opts: LoadOptions{
				APIKey: "k",
			},
			errContains: "OperatorEndpoint is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg, err := Load(tt.opts)
			require.Error(t, err)
			assert.Nil(t, cfg)
			assert.Contains(t, err.Error(), tt.errContains)
		})
	}
}

// ---------------------------------------------------------------------------
// LoadGateway
// ---------------------------------------------------------------------------

func TestLoadGateway_Defaults(t *testing.T) {
	wantWorkDir := FindProjectRoot()
	if wantWorkDir == "" {
		var err error
		wantWorkDir, err = os.Getwd()
		require.NoError(t, err)
	}

	cfg, err := LoadGateway(GatewayOptions{AllowTestPortZero: true})
	require.NoError(t, err)
	require.NotNil(t, cfg)

	assert.True(t, cfg.Gateway.Enabled)

	assert.Equal(t, 0, cfg.Gateway.HTTPPort)
	assert.Equal(t, filepath.Join(wantWorkDir, ".g8e", "data"), cfg.Gateway.DataDir)
	assert.True(t, filepath.IsAbs(cfg.Gateway.DataDir))
	assert.Equal(t, constants.Status.ComponentName.G8EOGateway, cfg.ComponentName)
}

func TestLoadGateway_ExplicitValues(t *testing.T) {
	cfg, err := LoadGateway(GatewayOptions{
		HTTPPort:          443,
		BootstrapPort:     80,
		PublicPort:        8443,
		DataDir:           "/var/data",
		PKIDir:            "/var/pki",
		SecretsDir:        "/var/secrets",
		PasskeyRpID:       "example.com",
		PasskeyRpName:     "Example RP",
		AllowTestPortZero: true,
	})
	require.NoError(t, err)

	assert.Equal(t, 443, cfg.Gateway.HTTPPort)
	assert.Equal(t, 80, cfg.Gateway.BootstrapPort)
	assert.Equal(t, 8443, cfg.Gateway.PublicPort)
	assert.Equal(t, "/var/data", cfg.Gateway.DataDir)
	assert.Equal(t, "/var/pki", cfg.Gateway.PKIDir)
	assert.Equal(t, "/var/secrets", cfg.Gateway.SecretsDir)
}

func TestLoadGateway_PartialDefaults(t *testing.T) {

	t.Run("only data dir overridden", func(t *testing.T) {
		cfg, err := LoadGateway(GatewayOptions{DataDir: "/custom/data", AllowTestPortZero: true})
		require.NoError(t, err)

		assert.Equal(t, "/custom/data", cfg.Gateway.DataDir)
	})

	t.Run("no operator fields set", func(t *testing.T) {
		cfg, err := LoadGateway(GatewayOptions{AllowTestPortZero: true})
		require.NoError(t, err)
		assert.Empty(t, cfg.APIKey)
		assert.Empty(t, cfg.Endpoint)
		assert.Empty(t, cfg.PubSubURL)
	})
}

func TestLoadGateway_SucceedsWithAllDefaults(t *testing.T) {
	_, err := LoadGateway(GatewayOptions{AllowTestPortZero: true})
	require.NoError(t, err)
}

func TestLoadGateway_RejectsPortZeroInProduction(t *testing.T) {

	t.Run("reject httpPort 0", func(t *testing.T) {
		_, err := LoadGateway(GatewayOptions{
			HTTPPort:          0,
			BootstrapPort:     constants.Ports.OperatorBootstrapHttps,
			PublicPort:        constants.Ports.OperatorPublicHttps,
			AllowTestPortZero: false,
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "httpPort cannot be 0 in production")
	})

	t.Run("reject bootstrapPort 0", func(t *testing.T) {
		_, err := LoadGateway(GatewayOptions{
			HTTPPort:          constants.Ports.OperatorHttps,
			BootstrapPort:     0,
			PublicPort:        constants.Ports.OperatorPublicHttps,
			AllowTestPortZero: false,
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "bootstrapPort cannot be 0 in production")
	})

	t.Run("reject publicPort 0", func(t *testing.T) {
		_, err := LoadGateway(GatewayOptions{
			HTTPPort:          constants.Ports.OperatorHttps,
			BootstrapPort:     constants.Ports.OperatorBootstrapHttps,
			PublicPort:        0,
			AllowTestPortZero: false,
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "publicPort cannot be 0 in production")
	})

	t.Run("accept all non-zero ports in production", func(t *testing.T) {
		_, err := LoadGateway(GatewayOptions{
			HTTPPort:          constants.Ports.OperatorHttps,
			BootstrapPort:     constants.Ports.OperatorBootstrapHttps,
			PublicPort:        constants.Ports.OperatorPublicHttps,
			AllowTestPortZero: false,
		})
		require.NoError(t, err)
	})
}

// ---------------------------------------------------------------------------
// LoadOpenClaw
// ---------------------------------------------------------------------------

func TestLoadOpenClaw_Valid(t *testing.T) {
	gatewayURL := fmt.Sprintf("wss://gateway.example.com:%d", constants.Ports.OpenclawGateway)
	cfg, err := LoadOpenClaw(OpenClawOptions{
		GatewayURL:  gatewayURL,
		Token:       "token123",
		NodeID:      "node-1",
		DisplayName: "My Node",
		LogLevel:    "debug",
	})
	require.NoError(t, err)
	require.NotNil(t, cfg)

	assert.Equal(t, gatewayURL, cfg.GatewayURL)
	assert.Equal(t, "token123", cfg.Token)
	assert.Equal(t, "node-1", cfg.NodeID)
	assert.Equal(t, "My Node", cfg.DisplayName)
	assert.Equal(t, "debug", cfg.LogLevel)
}

func TestLoadOpenClaw_LogLevelDefaultsToInfo(t *testing.T) {
	cfg, err := LoadOpenClaw(OpenClawOptions{GatewayURL: "wss://gateway.example.com"})
	require.NoError(t, err)
	assert.Equal(t, "info", cfg.LogLevel)
}

func TestLoadOpenClaw_MissingGatewayURL(t *testing.T) {
	cfg, err := LoadOpenClaw(OpenClawOptions{
		Token:       "tok",
		NodeID:      "node",
		DisplayName: "label",
		LogLevel:    "info",
	})
	require.Error(t, err)
	assert.Nil(t, cfg)
	assert.Contains(t, err.Error(), "--openclaw-url")
}

func TestLoadOpenClaw_OptionalFieldsEmpty(t *testing.T) {
	gatewayURL := fmt.Sprintf("ws://gateway:%d", constants.Ports.OpenclawGateway)
	cfg, err := LoadOpenClaw(OpenClawOptions{GatewayURL: gatewayURL})
	require.NoError(t, err)

	assert.Empty(t, cfg.Token)
	assert.Empty(t, cfg.NodeID)
	assert.Empty(t, cfg.DisplayName)
}

// ---------------------------------------------------------------------------
// HeartbeatInterval via Load
// ---------------------------------------------------------------------------

func TestLoad_HeartbeatIntervalDefault(t *testing.T) {
	cfg, err := Load(LoadOptions{
		APIKey:           "k",
		OperatorEndpoint: constants.DefaultEndpoint,
	})
	require.NoError(t, err)
	assert.Equal(t, 30*time.Second, cfg.HeartbeatInterval)
}

func TestLoad_HeartbeatIntervalOverride(t *testing.T) {
	cfg, err := Load(LoadOptions{
		APIKey:            "k",
		OperatorEndpoint:  constants.DefaultEndpoint,
		HeartbeatInterval: 90 * time.Second,
	})
	require.NoError(t, err)
	assert.Equal(t, 90*time.Second, cfg.HeartbeatInterval)
}

func TestLoad_HeartbeatIntervalZeroUsesDefault(t *testing.T) {
	cfg, err := Load(LoadOptions{
		APIKey:            "k",
		OperatorEndpoint:  constants.DefaultEndpoint,
		HeartbeatInterval: 0,
	})
	require.NoError(t, err)
	assert.Equal(t, 30*time.Second, cfg.HeartbeatInterval)
}

// ---------------------------------------------------------------------------
// heartbeatIntervalOrDefault
// ---------------------------------------------------------------------------

func TestHeartbeatIntervalOrDefault(t *testing.T) {
	tests := []struct {
		input time.Duration
		want  time.Duration
	}{
		{0, 30 * time.Second},
		{-1 * time.Second, 30 * time.Second},
		{10 * time.Second, 10 * time.Second},
		{60 * time.Second, 60 * time.Second},
		{2 * time.Minute, 2 * time.Minute},
	}

	for _, tt := range tests {
		assert.Equal(t, tt.want, heartbeatIntervalOrDefault(tt.input), "input=%v", tt.input)
	}
}

// ---------------------------------------------------------------------------
// httpPortOrDefault
// ---------------------------------------------------------------------------

func TestHTTPPortOrDefault(t *testing.T) {
	tests := []struct {
		input int
		want  int
	}{
		{0, constants.Ports.OperatorHttps},
		{-1, constants.Ports.OperatorHttps},
		{1, 1},
		{constants.Ports.OperatorHttps, constants.Ports.OperatorHttps},
		{constants.Ports.OperatorHttps, constants.Ports.OperatorHttps},
	}

	for _, tt := range tests {
		assert.Equal(t, tt.want, httpPortOrDefault(tt.input), "input=%d", tt.input)
	}
}

// ---------------------------------------------------------------------------
// tlsServerName
// ---------------------------------------------------------------------------

func TestTLSServerName(t *testing.T) {
	tests := []struct {
		name     string
		endpoint string
		want     string
	}{
		{"hostname returns empty", constants.DefaultEndpoint, ""},
		{"plain hostname returns empty", "example.com", ""},
		{"IPv4 returns localhost", "10.0.0.1", constants.DefaultEndpoint},
		{"IPv4 loopback returns localhost", "127.0.0.1", constants.DefaultEndpoint},
		{"IPv6 loopback returns localhost", "::1", constants.DefaultEndpoint},
		{"IPv6 full returns localhost", "2001:db8::1", constants.DefaultEndpoint},
		{"IPv4-mapped IPv6 returns localhost", "::ffff:192.0.2.1", constants.DefaultEndpoint},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, tlsServerName(tt.endpoint))
		})
	}
}
