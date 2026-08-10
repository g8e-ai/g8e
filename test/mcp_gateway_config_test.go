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
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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
