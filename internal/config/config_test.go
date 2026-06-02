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
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/g8e-ai/g8e/internal/constants"
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
		OperatorEndpoint: constants.DefaultEndpoint,
	})
	require.NoError(t, err)
	require.NotNil(t, cfg)

	assert.Equal(t, constants.ComponentNameG8EO, cfg.ComponentName)
	assert.Equal(t, "g8e", cfg.ProjectID)
	assert.Equal(t, 25, cfg.MaxConcurrentTasks)
	// Outbound mode defaults to notary posture since L3Notary is nil
	assert.Equal(t, PostureNotary, cfg.Posture)
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
		OperatorEndpoint: constants.DefaultEndpoint,
	})
	require.NoError(t, err)

	assert.Equal(t, constants.DefaultEndpoint, cfg.Endpoint)
}

func TestLoad_HTTPPortOverride(t *testing.T) {
	cfg, err := Load(LoadOptions{
		OperatorEndpoint: constants.DefaultEndpoint,
		HTTPPort:         constants.Ports.OperatorHttps,
	})
	require.NoError(t, err)
	assert.Equal(t, constants.Ports.OperatorHttps, cfg.HTTPPort)
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
			name:        "missing Operator endpoint",
			opts:        LoadOptions{},
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

func TestResolveGatewayPorts(t *testing.T) {
	// Try to bind to a port to make it unavailable
	ln, err := net.Listen("tcp", ":0")
	require.NoError(t, err)
	defer ln.Close()

	_, portStr, _ := net.SplitHostPort(ln.Addr().String())
	takenPort, _ := strconv.Atoi(portStr)

	t.Run("resolves when port is taken", func(t *testing.T) {
		h, b, p, m := ResolveGatewayPorts(takenPort, takenPort+1, takenPort+2, takenPort+3)
		assert.NotEqual(t, takenPort, h)
		assert.True(t, h > takenPort)
		assert.Equal(t, b, h+1)
		assert.Equal(t, p, h+2)
		assert.Equal(t, m, h+3)
	})

	t.Run("resolves when all are available", func(t *testing.T) {
		// Use very high ports that are likely free
		h, b, p, m := ResolveGatewayPorts(55000, 55001, 55002, 55003)
		// Verify ports are sequential and >= requested values
		assert.True(t, h >= 55000)
		assert.True(t, b >= 55001)
		assert.True(t, p >= 55002)
		assert.True(t, m >= 55003)
		assert.Equal(t, b, h+1)
		assert.Equal(t, p, h+2)
		assert.Equal(t, m, h+3)
	})
}

func TestLoadGateway_IncrementalPorts(t *testing.T) {
	// Try to find a port to block
	basePort := 56000
	var ln net.Listener
	var err error
	for i := 0; i < 10; i++ {
		ln, err = net.Listen("tcp", fmt.Sprintf(":%d", basePort+i))
		if err == nil {
			basePort = basePort + i
			break
		}
	}
	if ln == nil {
		t.Skip("Could not find a port to block for test")
		return
	}
	defer ln.Close()

	// Now try to load gateway with that base port
	cfg, err := LoadGateway(GatewayOptions{
		HTTPPort:          basePort,
		BootstrapPort:     basePort + 10, // Far enough away
		PublicPort:        basePort + 20,
		AllowTestPortZero: false,
	})
	require.NoError(t, err)
	assert.NotEqual(t, basePort, cfg.Gateway.HTTPPort)
	assert.True(t, cfg.Gateway.HTTPPort > basePort)
}

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
	assert.Equal(t, constants.ComponentNameG8EOGateway, cfg.ComponentName)
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

	t.Run("no Operator fields set", func(t *testing.T) {
		cfg, err := LoadGateway(GatewayOptions{AllowTestPortZero: true})
		require.NoError(t, err)
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
// LoadInsecureMcp
// ---------------------------------------------------------------------------

func TestLoadInsecureMcp_Valid(t *testing.T) {
	gatewayURL := fmt.Sprintf("wss://gateway.example.com:%d", constants.Ports.InsecureMcpGateway)
	cfg, err := LoadInsecureMcp(InsecureMcpOptions{
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

func TestLoadInsecureMcp_LogLevelDefaultsToInfo(t *testing.T) {
	cfg, err := LoadInsecureMcp(InsecureMcpOptions{GatewayURL: "wss://gateway.example.com"})
	require.NoError(t, err)
	assert.Equal(t, "info", cfg.LogLevel)
}

func TestLoadInsecureMcp_MissingGatewayURL(t *testing.T) {
	cfg, err := LoadInsecureMcp(InsecureMcpOptions{
		Token:       "tok",
		NodeID:      "node",
		DisplayName: "label",
		LogLevel:    "info",
	})
	require.Error(t, err)
	assert.Nil(t, cfg)
	assert.Contains(t, err.Error(), "--insecure-url")
}

func TestLoadInsecureMcp_OptionalFieldsEmpty(t *testing.T) {
	gatewayURL := fmt.Sprintf("ws://gateway:%d", constants.Ports.InsecureMcpGateway)
	cfg, err := LoadInsecureMcp(InsecureMcpOptions{GatewayURL: gatewayURL})
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
		OperatorEndpoint: constants.DefaultEndpoint,
	})
	require.NoError(t, err)
	assert.Equal(t, 30*time.Second, cfg.HeartbeatInterval)
}

func TestLoad_HeartbeatIntervalOverride(t *testing.T) {
	cfg, err := Load(LoadOptions{
		OperatorEndpoint:  constants.DefaultEndpoint,
		HeartbeatInterval: 90 * time.Second,
	})
	require.NoError(t, err)
	assert.Equal(t, 90*time.Second, cfg.HeartbeatInterval)
}

func TestLoad_HeartbeatIntervalZeroUsesDefault(t *testing.T) {
	cfg, err := Load(LoadOptions{
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
