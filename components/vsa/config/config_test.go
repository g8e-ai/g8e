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
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/g8e-ai/g8e/components/vsa/constants"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// Load
// ---------------------------------------------------------------------------

func TestLoad_Defaults(t *testing.T) {
	cwd, err := os.Getwd()
	require.NoError(t, err)

	cfg, err := Load(LoadOptions{
		APIKey:           "test-key",
		OperatorEndpoint: constants.DefaultEndpoint,
	})
	require.NoError(t, err)
	require.NotNil(t, cfg)

	assert.Equal(t, constants.Status.ComponentName.VSA, cfg.ServiceName)
	assert.Equal(t, constants.Status.AuthMode.APIKey, cfg.AuthMode)
	assert.Equal(t, "g8e", cfg.ProjectID)
	assert.Equal(t, 25, cfg.MaxConcurrentTasks)
	assert.Equal(t, 2048, cfg.MaxMemoryMB)
	assert.Equal(t, 30*time.Second, cfg.HeartbeatInterval)
	assert.Equal(t, int64(1024), cfg.LocalStoreMaxSizeMB)
	assert.Equal(t, 30, cfg.LocalStoreRetentionDays)
	assert.Equal(t, 443, cfg.HTTPPort)

	// WorkDir defaults to the process cwd when --working-dir is not supplied
	assert.Equal(t, cwd, cfg.WorkDir)
	// LocalStoreDBPath is anchored to WorkDir
	assert.Equal(t, filepath.Join(cwd, ".g8e", "local_state.db"), cfg.LocalStoreDBPath)
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

func TestLoad_PubSubURLFormats(t *testing.T) {
	tests := []struct {
		name       string
		endpoint   string
		wssPort    int
		wantPubSub string
	}{
		{
			name:       "hostname default port",
			endpoint:   constants.DefaultEndpoint,
			wssPort:    0,
			wantPubSub: "wss://" + constants.DefaultEndpoint + ":443",
		},
		{
			name:       "hostname custom port",
			endpoint:   constants.DefaultEndpoint,
			wssPort:    443,
			wantPubSub: "wss://" + constants.DefaultEndpoint,
		},
		{
			name:       "IPv4 default port",
			endpoint:   "10.0.1.42",
			wssPort:    0,
			wantPubSub: "wss://10.0.1.42:443",
		},
		{
			name:       "IPv4 custom port",
			endpoint:   "192.168.1.5",
			wssPort:    8443,
			wantPubSub: "wss://192.168.1.5:8443",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg, err := Load(LoadOptions{
				APIKey:           "k",
				OperatorEndpoint: tt.endpoint,
				WSSPort:          tt.wssPort,
			})
			require.NoError(t, err)
			assert.Equal(t, tt.wantPubSub, cfg.PubSubURL)
		})
	}
}

func TestLoad_HTTPPortOverride(t *testing.T) {
	cfg, err := Load(LoadOptions{
		APIKey:           "k",
		OperatorEndpoint: constants.DefaultEndpoint,
		HTTPPort:         8080,
	})
	require.NoError(t, err)
	assert.Equal(t, 8080, cfg.HTTPPort)
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

func TestLoad_AuthModeDefaults(t *testing.T) {
	t.Run("empty AuthMode defaults to api_key", func(t *testing.T) {
		cfg, err := Load(LoadOptions{
			APIKey:           "k",
			OperatorEndpoint: constants.DefaultEndpoint,
		})
		require.NoError(t, err)
		assert.Equal(t, constants.Status.AuthMode.APIKey, cfg.AuthMode)
	})

	t.Run("explicit api_key mode accepted", func(t *testing.T) {
		cfg, err := Load(LoadOptions{
			AuthMode:         constants.Status.AuthMode.APIKey,
			APIKey:           "k",
			OperatorEndpoint: constants.DefaultEndpoint,
		})
		require.NoError(t, err)
		assert.Equal(t, constants.Status.AuthMode.APIKey, cfg.AuthMode)
	})

	t.Run("session mode stores operator session ID", func(t *testing.T) {
		cfg, err := Load(LoadOptions{
			AuthMode:          constants.Status.AuthMode.OperatorSession,
			OperatorSessionID: "sess-abc",
			OperatorEndpoint:  constants.DefaultEndpoint,
		})
		require.NoError(t, err)
		assert.Equal(t, constants.Status.AuthMode.OperatorSession, cfg.AuthMode)
		assert.Equal(t, "sess-abc", cfg.OperatorSessionId)
	})
}

func TestLoad_CloudAndLocalStorage(t *testing.T) {
	tests := []struct {
		name     string
		opts     LoadOptions
		validate func(t *testing.T, cfg *Config)
	}{
		{
			name: "cloud mode aws",
			opts: LoadOptions{
				APIKey:           "k",
				OperatorEndpoint: constants.DefaultEndpoint,
				CloudMode:        true,
				CloudProvider:    constants.Status.CloudSubtype.AWS,
			},
			validate: func(t *testing.T, cfg *Config) {
				assert.True(t, cfg.CloudMode)
				assert.Equal(t, constants.Status.CloudSubtype.AWS, cfg.CloudProvider)
			},
		},
		{
			name: "cloud mode gcp",
			opts: LoadOptions{
				APIKey:           "k",
				OperatorEndpoint: constants.DefaultEndpoint,
				CloudMode:        true,
				CloudProvider:    constants.Status.CloudSubtype.GCP,
			},
			validate: func(t *testing.T, cfg *Config) {
				assert.True(t, cfg.CloudMode)
				assert.Equal(t, constants.Status.CloudSubtype.GCP, cfg.CloudProvider)
			},
		},
		{
			name: "cloud mode azure",
			opts: LoadOptions{
				APIKey:           "k",
				OperatorEndpoint: constants.DefaultEndpoint,
				CloudMode:        true,
				CloudProvider:    constants.Status.CloudSubtype.Azure,
			},
			validate: func(t *testing.T, cfg *Config) {
				assert.True(t, cfg.CloudMode)
				assert.Equal(t, constants.Status.CloudSubtype.Azure, cfg.CloudProvider)
			},
		},
		{
			name: "cloud mode empty provider by default",
			opts: LoadOptions{
				APIKey:           "k",
				OperatorEndpoint: constants.DefaultEndpoint,
				CloudMode:        true,
			},
			validate: func(t *testing.T, cfg *Config) {
				assert.True(t, cfg.CloudMode)
				assert.Empty(t, cfg.CloudProvider)
			},
		},
		{
			name: "local storage enabled",
			opts: LoadOptions{
				APIKey:              "k",
				OperatorEndpoint:    constants.DefaultEndpoint,
				LocalStorageEnabled: true,
			},
			validate: func(t *testing.T, cfg *Config) {
				assert.True(t, cfg.LocalStoreEnabled)
			},
		},
		{
			name: "local storage disabled by default",
			opts: LoadOptions{
				APIKey:           "k",
				OperatorEndpoint: constants.DefaultEndpoint,
			},
			validate: func(t *testing.T, cfg *Config) {
				assert.False(t, cfg.LocalStoreEnabled)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg, err := Load(tt.opts)
			require.NoError(t, err)
			tt.validate(t, cfg)
		})
	}
}

func TestLoad_NoGitPassthrough(t *testing.T) {
	t.Run("no-git false by default", func(t *testing.T) {
		cfg, err := Load(LoadOptions{
			APIKey:           "k",
			OperatorEndpoint: constants.DefaultEndpoint,
		})
		require.NoError(t, err)
		assert.False(t, cfg.NoGit)
	})

	t.Run("no-git true when set", func(t *testing.T) {
		cfg, err := Load(LoadOptions{
			APIKey:           "k",
			OperatorEndpoint: constants.DefaultEndpoint,
			NoGit:            true,
		})
		require.NoError(t, err)
		assert.True(t, cfg.NoGit)
	})
}

func TestLoad_ValidationErrors(t *testing.T) {
	tests := []struct {
		name        string
		opts        LoadOptions
		errContains string
	}{
		{
			name: "missing api key in api_key mode",
			opts: LoadOptions{
				OperatorEndpoint: constants.DefaultEndpoint,
			},
			errContains: "APIKey is required",
		},
		{
			name: "missing api key when AuthMode explicit api_key",
			opts: LoadOptions{
				AuthMode:         constants.Status.AuthMode.APIKey,
				OperatorEndpoint: constants.DefaultEndpoint,
			},
			errContains: "APIKey is required",
		},
		{
			name: "missing operator session ID in session mode",
			opts: LoadOptions{
				AuthMode:         constants.Status.AuthMode.OperatorSession,
				OperatorEndpoint: constants.DefaultEndpoint,
			},
			errContains: "OperatorSessionID is required",
		},
		{
			name: "missing operator endpoint",
			opts: LoadOptions{
				APIKey: "k",
			},
			errContains: "OperatorEndpoint is required",
		},
		{
			name: "session mode missing both operator session ID and endpoint",
			opts: LoadOptions{
				AuthMode: constants.Status.AuthMode.OperatorSession,
			},
			errContains: "OperatorSessionID is required",
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
// LoadListen
// ---------------------------------------------------------------------------

func TestLoadListen_Defaults(t *testing.T) {
	cwd, err := os.Getwd()
	require.NoError(t, err)

	cfg, err := LoadListen(0, 0, "", "", "", "", "")
	require.NoError(t, err)
	require.NotNil(t, cfg)

	assert.True(t, cfg.Listen.Enabled)
	assert.Equal(t, 443, cfg.Listen.WSSPort)
	assert.Equal(t, 443, cfg.Listen.HTTPPort)
	assert.Equal(t, filepath.Join(cwd, ".g8e", "data"), cfg.Listen.DataDir)
	assert.Equal(t, filepath.Join(cwd, ".g8e", "bin"), cfg.Listen.BinaryDir)
	assert.True(t, filepath.IsAbs(cfg.Listen.DataDir))
	assert.True(t, filepath.IsAbs(cfg.Listen.BinaryDir))
	assert.Equal(t, "vsa-listen", cfg.ServiceName)
}

func TestLoadListen_ExplicitValues(t *testing.T) {
	cfg, err := LoadListen(
		9443,
		8080,
		"/var/data",
		"",
		"/opt/bin",
		"/etc/certs/tls.crt",
		"/etc/certs/tls.key",
	)
	require.NoError(t, err)

	assert.Equal(t, 9443, cfg.Listen.WSSPort)
	assert.Equal(t, 8080, cfg.Listen.HTTPPort)
	assert.Equal(t, "/var/data", cfg.Listen.DataDir)
	assert.Equal(t, "/opt/bin", cfg.Listen.BinaryDir)
	assert.Equal(t, "/etc/certs/tls.crt", cfg.Listen.TLSCertPath)
	assert.Equal(t, "/etc/certs/tls.key", cfg.Listen.TLSKeyPath)
}

func TestLoadListen_PartialDefaults(t *testing.T) {
	t.Run("only wss port overridden", func(t *testing.T) {
		cwd, err := os.Getwd()
		require.NoError(t, err)
		cfg, err := LoadListen(443, 0, "", "", "", "", "")
		require.NoError(t, err)
		assert.Equal(t, 443, cfg.Listen.WSSPort)
		assert.Equal(t, 443, cfg.Listen.HTTPPort)
		assert.Equal(t, filepath.Join(cwd, ".g8e", "data"), cfg.Listen.DataDir)
	})

	t.Run("only data dir overridden", func(t *testing.T) {
		cwd, err := os.Getwd()
		require.NoError(t, err)
		cfg, err := LoadListen(0, 0, "/custom/data", "", "", "", "")
		require.NoError(t, err)
		assert.Equal(t, 443, cfg.Listen.WSSPort)
		assert.Equal(t, "/custom/data", cfg.Listen.DataDir)
		assert.Equal(t, filepath.Join(cwd, ".g8e", "bin"), cfg.Listen.BinaryDir)
	})

	t.Run("no operator fields set", func(t *testing.T) {
		cfg, err := LoadListen(0, 0, "", "", "", "", "")
		require.NoError(t, err)
		assert.Empty(t, cfg.APIKey)
		assert.Empty(t, cfg.Endpoint)
		assert.Empty(t, cfg.PubSubURL)
	})
}

func TestLoadListen_SucceedsWithAllDefaults(t *testing.T) {
	_, err := LoadListen(0, 0, "", "", "", "", "")
	require.NoError(t, err)
}

// ---------------------------------------------------------------------------
// LoadOpenClaw
// ---------------------------------------------------------------------------

func TestLoadOpenClaw_Valid(t *testing.T) {
	cfg, err := LoadOpenClaw("wss://gateway.example.com:8080", "token123", "node-1", "My Node", "", "debug")
	require.NoError(t, err)
	require.NotNil(t, cfg)

	assert.Equal(t, "wss://gateway.example.com:8080", cfg.GatewayURL)
	assert.Equal(t, "token123", cfg.Token)
	assert.Equal(t, "node-1", cfg.NodeID)
	assert.Equal(t, "My Node", cfg.DisplayName)
	assert.Equal(t, "debug", cfg.LogLevel)
}

func TestLoadOpenClaw_LogLevelDefaultsToInfo(t *testing.T) {
	cfg, err := LoadOpenClaw("wss://gateway.example.com", "", "", "", "", "")
	require.NoError(t, err)
	assert.Equal(t, "info", cfg.LogLevel)
}

func TestLoadOpenClaw_MissingGatewayURL(t *testing.T) {
	cfg, err := LoadOpenClaw("", "tok", "node", "label", "", "info")
	require.Error(t, err)
	assert.Nil(t, cfg)
	assert.Contains(t, err.Error(), "--openclaw-url")
}

func TestLoadOpenClaw_OptionalFieldsEmpty(t *testing.T) {
	cfg, err := LoadOpenClaw("ws://gateway:8080", "", "", "", "", "")
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
// wssPortOrDefault
// ---------------------------------------------------------------------------

func TestWSSPortOrDefault(t *testing.T) {
	tests := []struct {
		input int
		want  int
	}{
		{0, 443},
		{-1, 443},
		{1, 1},
		{443, 443},
	}

	for _, tt := range tests {
		assert.Equal(t, tt.want, wssPortOrDefault(tt.input), "input=%d", tt.input)
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
		{0, 443},
		{-1, 443},
		{1, 1},
		{443, 443},
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
		{"IPv4 returns g8e.local", "10.0.0.1", constants.DefaultEndpoint},
		{"IPv4 loopback returns g8e.local", "127.0.0.1", constants.DefaultEndpoint},
		{"IPv6 loopback returns g8e.local", "::1", constants.DefaultEndpoint},
		{"IPv6 full returns g8e.local", "2001:db8::1", constants.DefaultEndpoint},
		{"IPv4-mapped IPv6 returns g8e.local", "::ffff:192.0.2.1", constants.DefaultEndpoint},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, tlsServerName(tt.endpoint))
		})
	}
}
