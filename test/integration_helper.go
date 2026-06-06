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
	"crypto/x509"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/internal/cli/config"
	"github.com/g8e-ai/g8e/internal/constants"
)

// NewLiveOperatorHTTPClient creates an HTTP client configured for mTLS
// against a running g8e platform. It loads the canonical trust bundle,
// validates it parses correctly, loads the CLI client certificate, and
// returns a client with proper TLS configuration.
//
// This helper requires the platform to be running via `./g8e platform start`
// and authenticated via `./g8e auth login`.
//
// Parameters:
//   - t: testing.T for assertions
//   - repoRoot: path to the repository root (typically derived from test directory)
//
// Returns:
//   - *http.Client: configured HTTP client with mTLS and CA verification
//   - *config.Config: loaded CLI configuration (for ports and paths)
func NewLiveOperatorHTTPClient(t require.TestingT, repoRoot string) (*http.Client, *config.Config) {
	// Load CLI config to get ports and paths
	cliCfg, err := config.Load(repoRoot)
	require.NoError(t, err, "failed to load CLI config")

	// Verify client certificate exists (bootstrapped via ./g8e auth login)
	clientCertPath := cliCfg.CLICertFile()
	if _, err := os.Stat(clientCertPath); os.IsNotExist(err) {
		require.NoError(t, fmt.Errorf("client cert not found at %s - run './g8e auth login' first", clientCertPath))
	}

	// Load canonical trust bundle
	caCertPath := cliCfg.TrustBundlePath()
	caCert, err := os.ReadFile(caCertPath)
	require.NoError(t, err, "failed to read CA bundle from %s - run './g8e platform start && ./g8e auth login' first", caCertPath)

	// Validate trust bundle parses correctly
	caCertPool := x509.NewCertPool()
	ok := caCertPool.AppendCertsFromPEM(caCert)
	require.True(t, ok, "invalid canonical trust bundle at %s - regenerate PKI by running './g8e platform start && ./g8e auth login'", caCertPath)

	// Load client certificate for mTLS
	cert, err := tls.LoadX509KeyPair(cliCfg.CLICertFile(), cliCfg.CLIKeyFile())
	require.NoError(t, err, "failed to load client certificate")

	// Build HTTP client with proper mTLS configuration
	tr := &http.Transport{
		TLSClientConfig: &tls.Config{
			RootCAs:      caCertPool,
			Certificates: []tls.Certificate{cert},
			ServerName:   "localhost",
		},
	}
	client := &http.Client{Transport: tr, Timeout: 5 * time.Second}

	return client, cliCfg
}

// ResolveRepoRootFromTestDir finds the repository root using go list -m.
// This is more robust than directory navigation and works regardless of
// the current working directory.
func ResolveRepoRootFromTestDir(t require.TestingT) string {
	// Use go list -m to find the module directory
	cmd := exec.Command("go", "list", "-m", "-f", "{{.Dir}}")
	output, err := cmd.Output()
	require.NoError(t, err, "failed to run go list -m to find repository root")

	repoRoot := string(output)
	repoRoot = filepath.Clean(strings.TrimSpace(repoRoot))
	require.NotEmpty(t, repoRoot, "go list -m returned empty directory")

	return repoRoot
}

// EnsureGatewayReady ensures the gateway is running and governance is ready.
// It polls the health endpoint until governance_ready is true.
func EnsureGatewayReady(t *testing.T, cliCfg *config.Config) {
	t.Helper()

	healthURL := fmt.Sprintf("http://127.0.0.1:%d%s", constants.Ports.OperatorHttp, "/api/v1/health")

	require.Eventually(t, func() bool {
		resp, err := http.Get(healthURL)
		if err != nil {
			return false
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			return false
		}

		var health struct {
			Status          string `json:"status"`
			Mode            string `json:"mode"`
			Version         string `json:"version"`
			GovernanceReady bool   `json:"governance_ready"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&health); err != nil {
			return false
		}

		return health.GovernanceReady
	}, 30*time.Second, 500*time.Millisecond, "gateway did not become governance-ready within timeout")
}

// EnsureAuthLogin ensures the CLI has a fresh session by running './g8e auth login'.
// This is called automatically by integration tests to bootstrap credentials.
func EnsureAuthLogin(t *testing.T, repoRoot string) {
	t.Helper()

	// Build the g8e binary if needed
	g8ePath := filepath.Join(repoRoot, "g8e")
	if _, err := os.Stat(g8ePath); os.IsNotExist(err) {
		// Build the binary
		buildCmd := exec.Command("go", "build", "-o", g8ePath, "./cmd/g8e")
		buildCmd.Dir = repoRoot
		if output, err := buildCmd.CombinedOutput(); err != nil {
			require.NoError(t, err, "failed to build g8e binary: %s", string(output))
		}
	}

	// Run './g8e auth login'
	loginCmd := exec.Command(g8ePath, "auth", "login")
	loginCmd.Dir = repoRoot
	if output, err := loginCmd.CombinedOutput(); err != nil {
		require.NoError(t, err, "failed to run './g8e auth login': %s", string(output))
	}
}
