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

package cmd

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/g8e-ai/g8e/internal/cli/auth"
	"github.com/g8e-ai/g8e/internal/cli/config"
	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/stretchr/testify/assert"
)

func TestDetectToolBinary(t *testing.T) {
	tests := []struct {
		name        string
		toolName    string
		wantErr     bool
		errContains string
	}{
		{
			name:        "unknown tool",
			toolName:    "unknown-tool",
			wantErr:     true,
			errContains: "supported tools",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			binary, err := detectToolBinary(tt.toolName, defaultToolPaths)
			if tt.wantErr {
				assert.Error(t, err)
				if tt.errContains != "" && err != nil {
					assert.Contains(t, err.Error(), tt.errContains)
				}
				assert.Empty(t, binary)
			} else {
				assert.NoError(t, err)
				assert.NotEmpty(t, binary)
			}
		})
	}
}

func TestDetectToolBinary_WithMockBinary(t *testing.T) {
	tempDir := t.TempDir()
	testBinary := filepath.Join(tempDir, "test-tool")

	err := os.WriteFile(testBinary, []byte("#!/bin/sh\necho test"), 0755)
	assert.NoError(t, err)

	mockToolPaths := map[string][]string{
		"test-tool": {testBinary},
	}

	binary, err := detectToolBinary("test-tool", mockToolPaths)
	assert.NoError(t, err)
	assert.Equal(t, testBinary, binary)
}

func TestPrepareAgentEnvironment(t *testing.T) {
	tempDir := t.TempDir()
	constants.InitPathsWithBase(tempDir)
	cfg := &config.Config{
		ProjectRoot: tempDir,
		Paths: &config.PathsConfig{
			Infra: struct {
				AppCertDir           string `json:"app_cert_dir"`
				CACertPath           string `json:"ca_cert_path"`
				DBPath               string `json:"db_path"`
				DocsDir              string `json:"docs_dir"`
				PKIDir               string `json:"pki_dir"`
				ProtocolConstantsDir string `json:"protocol_constants_dir"`
				ProtocolDir          string `json:"protocol_dir"`
				ProtocolModelsDir    string `json:"protocol_models_dir"`
				SecretsDir           string `json:"secrets_dir"`
				SSHConfigPath        string `json:"ssh_config_path"`
			VaultDir             string `json:"vault_dir"`
			VaultKeyPath         string `json:"vault_key_path"`
			}{
				CACertPath: constants.Paths.Infra.CaCertPath,
			},
		},
	}

	creds := &auth.Credentials{
		OperatorSessionID: "test-operator-session-id",
		UserID:            "test-user-id",
	}

	env := prepareAgentEnvironment(cfg, creds)

	envMap := make(map[string]string)
	for _, e := range env {
		for i := 0; i < len(e); i++ {
			if e[i] == '=' {
				key := e[:i]
				value := e[i+1:]
				envMap[key] = value
				break
			}
		}
	}

	assert.Contains(t, envMap, "G8E_MCP_CONFIG")
	assert.Contains(t, envMap, "G8E_GATEWAY_URL")
	assert.Contains(t, envMap, "G8E_CLIENT_CERT")
	assert.Contains(t, envMap, "G8E_CLIENT_KEY")
	assert.Contains(t, envMap, "G8E_CA_BUNDLE")
	assert.Contains(t, envMap, "G8E_OPERATOR_SESSION_ID")
	assert.Contains(t, envMap, "G8E_USER_ID")

	assert.Equal(t, "test-operator-session-id", envMap["G8E_OPERATOR_SESSION_ID"])
	assert.Equal(t, "test-user-id", envMap["G8E_USER_ID"])
}

func TestGenerateMCPConfigForStdio(t *testing.T) {
	tempDir := t.TempDir()
	cfg := &config.Config{
		ProjectRoot: tempDir,
	}

	config := generateMCPConfigForStdio(cfg)

	assert.Contains(t, config, "mcpServers")
	assert.Contains(t, config, "g8e-gateway")
	assert.Contains(t, config, "stdio-proxy")
	assert.Contains(t, config, tempDir)
}

func TestCheckGatewayStatus(t *testing.T) {
	tempDir := t.TempDir()
	cfg := &config.Config{
		ProjectRoot: tempDir,
	}

	running, pid, err := checkGatewayStatus(cfg)
	assert.NoError(t, err)
	if running {
		assert.Greater(t, pid, 0)
	} else {
		assert.Equal(t, 0, pid)
	}
}

func TestAgentCmd(t *testing.T) {
	cmd := agentCmd()
	assert.NotNil(t, cmd)
	assert.Contains(t, cmd.Use, "agent")
	assert.Equal(t, "Wrap agentic coding tools with g8e zero-trust gateway", cmd.Short)
}

func TestAgentClaudeCmd(t *testing.T) {
	cmd := agentClaudeCmd()
	assert.NotNil(t, cmd)
	assert.Contains(t, cmd.Use, "claude")
	assert.Equal(t, "Execute Claude Code proxied through g8e gateway", cmd.Short)
}

func TestExecuteTool(t *testing.T) {
	t.Run("executes real subprocess successfully", func(t *testing.T) {
		err := executeTool("true", []string{}, os.Environ())
		assert.NoError(t, err)
	})

	t.Run("returns error on failed subprocess", func(t *testing.T) {
		err := executeTool("false", []string{}, os.Environ())
		assert.Error(t, err)
	})

	t.Run("returns error for nonexistent binary", func(t *testing.T) {
		err := executeTool("/nonexistent/binary/that/does/not/exist", []string{}, os.Environ())
		assert.Error(t, err)
	})

	t.Run("passes arguments to subprocess", func(t *testing.T) {
		// sh -c "exit 0" verifies args are passed through
		err := executeTool("sh", []string{"-c", "exit 0"}, os.Environ())
		assert.NoError(t, err)
	})

	t.Run("passes environment to subprocess", func(t *testing.T) {
		env := append(os.Environ(), "G8E_TEST_VAR=integration-test-value")
		// Use sh to verify env var is set
		err := executeTool("sh", []string{"-c", "test \"$G8E_TEST_VAR\" = \"integration-test-value\""}, env)
		assert.NoError(t, err)
	})
}
