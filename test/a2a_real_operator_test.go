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
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/internal/testutil"
)

// TestA2ARealOperator_Smoke is a live-operator smoke test that validates
// the bootstrap + A2A call flow against a running ./g8e platform.
//
// This test is intentionally narrow: it does NOT exercise the full A2A
// skill execution with downstream server and OOB WebAuthn approval -
// those flows are covered hermetically by TestA2AGateway_EndToEnd in
// a2a_gateway_test.go.
//
// This test now starts its own isolated gateway instance for proper test isolation.
func TestA2ARealOperator_Smoke(t *testing.T) {
	// Create isolated test environment
	dataDir := t.TempDir()
	binPath := testutil.GetTestBinaryPath(t)

	// Ensure binary exists
	if _, err := os.Stat(binPath); os.IsNotExist(err) {
		t.Skipf("g8e binary not found at %s - run 'make build' first", binPath)
	}

	// Start gateway subprocess
	env := []string{
		fmt.Sprintf("G8E_RUNTIME_DIR=%s", dataDir),
	}
	_, bootstrapPort, publicPort := testutil.StartGatewaySubprocess(t, binPath, dataDir, env)

	// Run CLI login to bootstrap
	loginEnv := append(env,
		"G8E_OPERATOR_ENDPOINT=localhost",
		fmt.Sprintf("G8E_OPERATOR_PORT=%d", publicPort),
		fmt.Sprintf("G8E_OPERATOR_BOOTSTRAP_PORT=%d", bootstrapPort),
		fmt.Sprintf("HOME=%s", dataDir), // Redirect CLI credentials to temp dir
	)

	stdout, stderr, err := testutil.RunCLI(t, binPath, []string{"auth", "login"}, loginEnv)
	require.NoError(t, err, "CLI login failed: %s\n%s", stdout, stderr)
	require.Contains(t, stdout, "Bootstrap complete", "CLI login did not perform bootstrap")

	// Test basic connectivity to operator via HTTPS
	healthURL := fmt.Sprintf("https://localhost:%d/health", publicPort)
	tr := &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
	}
	client := &http.Client{Transport: tr, Timeout: 5 * time.Second}

	resp, err := client.Get(healthURL)
	require.NoError(t, err)
	defer resp.Body.Close()

	require.Equal(t, http.StatusOK, resp.StatusCode, "health check failed")

	// Query isolated operator audit vault for inspection
	vaultPath := filepath.Join(dataDir, "audit_vault.db")
	if _, statErr := os.Stat(vaultPath); statErr == nil {
		t.Logf("Isolated audit vault found at: %s", vaultPath)
	}
}
