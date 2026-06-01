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
	"fmt"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestMCPRealOperator_Smoke is a live-operator smoke test that validates
// the bootstrap + MCP tools/list flow against a running ./g8e platform.
//
// This test is intentionally narrow: it does NOT exercise tools/call,
// which requires a downstream MCP server and OOB WebAuthn approval -
// those flows are covered hermetically by TestMCPGateway_EndToEnd in
// mcp_gateway_test.go.
//
// This test requires the platform to be running via `./g8e platform start`
// and authenticated via `./g8e auth login`.
func TestMCPRealOperator_Smoke(t *testing.T) {
	repoRoot := ResolveRepoRootFromTestDir(t)
	client, cliCfg := NewLiveOperatorHTTPClient(t, repoRoot)

	// Test basic connectivity to operator via HTTPS
	healthURL := fmt.Sprintf("https://localhost:%d/health", cliCfg.Paths.Ports.OperatorPublicHTTPS)

	resp, err := client.Get(healthURL)
	require.NoError(t, err)
	defer resp.Body.Close()

	require.Equal(t, http.StatusOK, resp.StatusCode, "health check failed")
}
