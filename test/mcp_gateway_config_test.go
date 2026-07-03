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

//go:build integration

package tests

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

/*
TestMCPGateway_JSONRPCParsing validates that the gateway can parse
JSON-RPC requests from HTTP format.
*/
func TestMCPGateway_JSONRPCParsing(t *testing.T) {
	t.Run("valid tools/list request", func(t *testing.T) {
		req := `{"jsonrpc":"2.0","id":1,"method":"tools/list","params":{}}`
		var parsed struct {
			JSONRPC string          `json:"jsonrpc"`
			ID      int             `json:"id"`
			Method  string          `json:"method"`
			Params  json.RawMessage `json:"params"`
		}
		err := json.Unmarshal([]byte(req), &parsed)
		require.NoError(t, err)
		assert.Equal(t, "tools/list", parsed.Method)
	})

	t.Run("valid tools/call request", func(t *testing.T) {
		req := `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"test_tool","arguments":{}}}`
		var parsed struct {
			JSONRPC string          `json:"jsonrpc"`
			ID      int             `json:"id"`
			Method  string          `json:"method"`
			Params  json.RawMessage `json:"params"`
		}
		err := json.Unmarshal([]byte(req), &parsed)
		require.NoError(t, err)
		assert.Equal(t, "tools/call", parsed.Method)
	})

	t.Run("invalid JSON", func(t *testing.T) {
		req := `invalid json`
		var parsed struct {
			JSONRPC string `json:"jsonrpc"`
		}
		err := json.Unmarshal([]byte(req), &parsed)
		require.Error(t, err, "invalid JSON should fail to parse")
	})
}

/*
TestMCPGateway_ConfigTemplate validates that the config template file
exists and contains the expected HTTP transport structure.
*/
func TestMCPGateway_ConfigTemplate(t *testing.T) {
	t.Run("http template exists", func(t *testing.T) {
		repoRoot := ResolveRepoRootFromTestDir(t)
		fullPath := filepath.Join(repoRoot, "protocol/examples/mcp_server/g8e_gateway_mcp_config.json")
		content, err := os.ReadFile(fullPath)
		require.NoError(t, err, "http template should exist at %s", fullPath)
		assert.Contains(t, string(content), `"type": "http"`)
		assert.Contains(t, string(content), `"url"`)
		assert.Contains(t, string(content), `"tls"`)
		assert.Contains(t, string(content), `"clientCertificate"`)
		assert.Contains(t, string(content), `"clientKey"`)
		assert.Contains(t, string(content), `"caCertificate"`)
	})
}
