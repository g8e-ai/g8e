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
	"crypto/tls"
	"fmt"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/internal/cli/config"
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
	// Load CLI config to get ports and paths
	cliCfg, err := config.Load("")
	if err != nil {
		t.Fatalf("failed to load CLI config: %v", err)
	}

	// Verify certificates exist (bootstrapped via ./g8e auth login)
	clientCertPath := cliCfg.CLICertFile()
	if _, err := os.Stat(clientCertPath); os.IsNotExist(err) {
		t.Fatalf("client cert not found at %s - run './g8e auth login' first", clientCertPath)
	}

	// Test basic connectivity to operator via HTTPS
	healthURL := fmt.Sprintf("https://localhost:%d/health", cliCfg.Paths.Ports.OperatorPublicHTTPS)
	tr := &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
	}
	client := &http.Client{Transport: tr, Timeout: 5 * time.Second}

	resp, err := client.Get(healthURL)
	require.NoError(t, err)
	defer resp.Body.Close()

	require.Equal(t, http.StatusOK, resp.StatusCode, "health check failed")
}
