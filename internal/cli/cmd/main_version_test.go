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
// See the License for the specific language and governing permissions and
// limitations under the License.

package cmd

import (
	"bytes"
	"context"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/internal/cli/config"
	"github.com/g8e-ai/g8e/internal/cli/serve"
)

func TestVersionInfoFromCmd_WithVersionInfo(t *testing.T) {
	vi := serve.VersionInfo{
		Version:   "1.2.3",
		BuildID:   "build-abc",
		BuildTime: "2026-07-10",
		Platform:  "linux/amd64",
	}

	cmd := &cobra.Command{}
	ctx := context.WithValue(context.Background(), versionInfoKey{}, vi)
	cmd.SetContext(ctx)

	got := versionInfoFromCmd(cmd)
	assert.Equal(t, "1.2.3", got.Version)
	assert.Equal(t, "build-abc", got.BuildID)
	assert.Equal(t, "2026-07-10", got.BuildTime)
	assert.Equal(t, "linux/amd64", got.Platform)
}

func TestVersionInfoFromCmd_WithoutVersionInfo(t *testing.T) {
	cmd := &cobra.Command{}
	cmd.SetContext(context.Background())
	got := versionInfoFromCmd(cmd)
	assert.Equal(t, serve.VersionInfo{}, got)
	assert.Empty(t, got.Version)
	assert.Empty(t, got.BuildID)
}

func TestNewRootCmd_PassportVersionInfoIntoContext(t *testing.T) {
	vi := serve.VersionInfo{
		Version:   "2.0.0",
		BuildID:   "test-build",
		BuildTime: "2026-01-01",
		Platform:  "test",
	}

	rootCmd := NewRootCmd("2.0.0", vi)
	got := versionInfoFromCmd(rootCmd)
	assert.Equal(t, "2.0.0", got.Version)
	assert.Equal(t, "test-build", got.BuildID)
}

func TestNewRootCmd_PersistentFlagsExist(t *testing.T) {
	rootCmd := NewRootCmd("dev", serve.VersionInfo{})

	endpointFlag := rootCmd.PersistentFlags().Lookup("endpoint")
	require.NotNil(t, endpointFlag)
	assert.Equal(t, "e", endpointFlag.Shorthand)
	assert.Equal(t, "", endpointFlag.DefValue)

	portFlag := rootCmd.PersistentFlags().Lookup("port")
	require.NotNil(t, portFlag)
	assert.Equal(t, "p", portFlag.Shorthand)
	assert.Equal(t, "0", portFlag.DefValue)
}

func TestNewRootCmd_PersistentPreRunE_SetsEndpointOverride(t *testing.T) {
	t.Run("endpoint flag sets override", func(t *testing.T) {
		config.SetEndpointOverride("")
		t.Cleanup(func() { config.SetEndpointOverride("") })

		rootCmd := NewRootCmd("dev", serve.VersionInfo{})
		rootCmd.SetArgs([]string{"--endpoint", "remote.example.com", "gw", "status"})

		var buf bytes.Buffer
		rootCmd.SetOut(&buf)
		rootCmd.SetErr(&buf)

		_ = rootCmd.Execute()

		cfg, err := config.Load("")
		require.NoError(t, err)
		assert.Contains(t, cfg.OperatorDiscoveryURL(), "remote.example.com")
		assert.Contains(t, cfg.OperatorPublicURL(), "remote.example.com")
	})

	t.Run("endpoint and port flags set override with port", func(t *testing.T) {
		config.SetEndpointOverride("")
		t.Cleanup(func() { config.SetEndpointOverride("") })

		rootCmd := NewRootCmd("dev", serve.VersionInfo{})
		rootCmd.SetArgs([]string{"--endpoint", "remote.example.com", "--port", "9999", "gw", "status"})

		var buf bytes.Buffer
		rootCmd.SetOut(&buf)
		rootCmd.SetErr(&buf)

		_ = rootCmd.Execute()

		cfg, err := config.Load("")
		require.NoError(t, err)
		assert.Contains(t, cfg.OperatorDiscoveryURL(), "remote.example.com")
		assert.NotContains(t, cfg.OperatorDiscoveryURL(), "9999")
		assert.Contains(t, cfg.OperatorPublicURL(), "remote.example.com")
		assert.Contains(t, cfg.OperatorPublicURL(), "9999")
	})

	t.Run("endpoint with port and --port sets split overrides", func(t *testing.T) {
		config.SetEndpointOverride("")
		t.Cleanup(func() { config.SetEndpointOverride("") })

		rootCmd := NewRootCmd("dev", serve.VersionInfo{})
		rootCmd.SetArgs([]string{"--endpoint", "remote.example.com:8085", "--port", "9999", "gw", "status"})

		var buf bytes.Buffer
		rootCmd.SetOut(&buf)
		rootCmd.SetErr(&buf)

		_ = rootCmd.Execute()

		cfg, err := config.Load("")
		require.NoError(t, err)
		assert.Contains(t, cfg.OperatorDiscoveryURL(), "remote.example.com:8085")
		assert.NotContains(t, cfg.OperatorDiscoveryURL(), "9999")
		assert.Contains(t, cfg.OperatorPublicURL(), "remote.example.com:9999")
		assert.NotContains(t, cfg.OperatorPublicURL(), "8085")
	})

	t.Run("port-only flag defaults to localhost", func(t *testing.T) {
		config.SetEndpointOverride("")
		t.Cleanup(func() { config.SetEndpointOverride("") })

		rootCmd := NewRootCmd("dev", serve.VersionInfo{})
		rootCmd.SetArgs([]string{"--port", "9090", "gw", "status"})

		var buf bytes.Buffer
		rootCmd.SetOut(&buf)
		rootCmd.SetErr(&buf)

		_ = rootCmd.Execute()

		cfg, err := config.Load("")
		require.NoError(t, err)
		assert.Contains(t, cfg.OperatorDiscoveryURL(), "localhost")
		assert.NotContains(t, cfg.OperatorDiscoveryURL(), "9090")
		assert.Contains(t, cfg.OperatorPublicURL(), "localhost")
		assert.Contains(t, cfg.OperatorPublicURL(), "9090")
	})
}
