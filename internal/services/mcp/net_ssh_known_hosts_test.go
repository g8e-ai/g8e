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

package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNetSSHKnownHostsTool_Metadata(t *testing.T) {
	tool := &NetSSHKnownHostsTool{}
	assert.Equal(t, "net_ssh_known_hosts", tool.Name())
	assert.NotEmpty(t, tool.Description())
	assert.NotNil(t, tool.InputSchema())
}

func TestNetSSHKnownHostsTool_Execute_Validation(t *testing.T) {
	tool := &NetSSHKnownHostsTool{}
	ctx := context.Background()

	t.Run("invalid json", func(t *testing.T) {
		_, err := tool.Execute(ctx, []byte(`{invalid}`))
		assert.Error(t, err)
		assert.True(t, errors.Is(err, constants.ErrMCPUnmarshalArguments))
	})

	t.Run("dangerous path config", func(t *testing.T) {
		args, _ := json.Marshal(NetSSHKnownHostsRequest{
			SSHConfigPath: "../dangerous",
		})
		_, err := tool.Execute(ctx, args)
		assert.Error(t, err)
		assert.True(t, errors.Is(err, constants.ErrMCPValidatePathParentDirRef))
	})

	t.Run("dangerous path known_hosts", func(t *testing.T) {
		args, _ := json.Marshal(NetSSHKnownHostsRequest{
			KnownHostsPath: "/etc/passwd\x00",
		})
		_, err := tool.Execute(ctx, args)
		assert.Error(t, err)
		assert.True(t, errors.Is(err, constants.ErrMCPValidatePathNullBytes))
	})
}

func TestNetSSHKnownHostsTool_Execute_Success(t *testing.T) {
	tool := &NetSSHKnownHostsTool{}
	ctx := context.Background()

	// Create temp files
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config")
	knownHostsPath := filepath.Join(tmpDir, "known_hosts")

	configContent := `
Host dev-server
    HostName 192.168.1.10
    User devuser
    Port 2222
    IdentityFile ~/.ssh/id_rsa

Host *.prod.example.com
    User admin
`
	err := os.WriteFile(configPath, []byte(configContent), 0600)
	require.NoError(t, err)

	knownHostsContent := `
# Comment line
192.168.1.10 ssh-rsa AAAAB3NzaC1yc2E...
github.com ssh-ed25519 AAAAC3NzaC1lZDI1NTE5...
@revoked old-host.com ssh-rsa AAAAB3...
`
	err = os.WriteFile(knownHostsPath, []byte(knownHostsContent), 0600)
	require.NoError(t, err)

	args, _ := json.Marshal(NetSSHKnownHostsRequest{
		SSHConfigPath:  configPath,
		KnownHostsPath: knownHostsPath,
	})

	result, err := tool.Execute(ctx, args)
	require.NoError(t, err)
	assert.False(t, result.IsError)
	require.Len(t, result.Content, 1)

	var hostsResult NetSSHKnownHostsResult
	err = json.Unmarshal([]byte(result.Content[0].Text), &hostsResult)
	require.NoError(t, err)

	// Verify SSH config hosts
	assert.Len(t, hostsResult.ConfigHosts, 2)
	foundDev := false
	for _, h := range hostsResult.ConfigHosts {
		if h.Pattern == "dev-server" {
			foundDev = true
			assert.Equal(t, "192.168.1.10", h.Hostname)
			assert.Equal(t, "devuser", h.User)
			assert.Equal(t, "2222", h.Port)
		}
	}
	assert.True(t, foundDev)

	// Verify known_hosts
	// The implementation skips markers but includes the host.
	// 1. 192.168.1.10 ssh-rsa
	// 2. github.com ssh-ed25519
	// 3. old-host.com ssh-rsa (marker @revoked is skipped)
	assert.Len(t, hostsResult.KnownHosts, 3)

	foundGithub := false
	foundRevoked := false
	for _, kh := range hostsResult.KnownHosts {
		if kh.HostPattern == "github.com" {
			foundGithub = true
			assert.Equal(t, "ssh-ed25519", kh.KeyType)
			assert.NotEmpty(t, kh.KeyHash)
		}
		if kh.HostPattern == "old-host.com" {
			foundRevoked = true
			assert.Equal(t, "ssh-rsa", kh.KeyType)
		}
	}
	assert.True(t, foundGithub)
	assert.True(t, foundRevoked)

	assert.Equal(t, configPath, hostsResult.ConfigPath)
	assert.Equal(t, knownHostsPath, hostsResult.KnownHostsPath)
}

func TestNetSSHKnownHostsTool_Execute_Defaults(t *testing.T) {
	tool := &NetSSHKnownHostsTool{}
	ctx := context.Background()

	// Empty request should use defaults
	args, _ := json.Marshal(NetSSHKnownHostsRequest{})

	result, err := tool.Execute(ctx, args)
	require.NoError(t, err)
	assert.False(t, result.IsError)

	var hostsResult NetSSHKnownHostsResult
	err = json.Unmarshal([]byte(result.Content[0].Text), &hostsResult)
	require.NoError(t, err)

	assert.NotEmpty(t, hostsResult.ConfigPath)
	assert.NotEmpty(t, hostsResult.KnownHostsPath)
	assert.Contains(t, hostsResult.ConfigPath, ".ssh")
	assert.Contains(t, hostsResult.KnownHostsPath, ".ssh")
}

func TestNetSSHKnownHostsTool_Execute_MissingFiles(t *testing.T) {
	tool := &NetSSHKnownHostsTool{}
	ctx := context.Background()

	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "non_existent_config")
	knownHostsPath := filepath.Join(tmpDir, "non_existent_known_hosts")

	args, _ := json.Marshal(NetSSHKnownHostsRequest{
		SSHConfigPath:  configPath,
		KnownHostsPath: knownHostsPath,
	})

	result, err := tool.Execute(ctx, args)
	require.NoError(t, err)

	var hostsResult NetSSHKnownHostsResult
	err = json.Unmarshal([]byte(result.Content[0].Text), &hostsResult)
	require.NoError(t, err)

	assert.Empty(t, hostsResult.ConfigHosts)
	assert.Empty(t, hostsResult.KnownHosts)
}
