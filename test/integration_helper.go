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

//go:build integration || e2e

package tests

import (
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

	"github.com/g8e-ai/g8e/internal/cli/api"
	"github.com/g8e-ai/g8e/internal/cli/config"
	"github.com/g8e-ai/g8e/internal/constants"
)

// NewLiveOperatorHTTPClient creates an API client configured for mTLS
// against a running g8e platform using the platform's api.NewClient.
//
// This helper requires the platform to be running via `./g8e platform start`
// and authenticated via `./g8e auth login`.
//
// Parameters:
//   - t: testing.T for assertions
//   - repoRoot: path to the repository root (typically derived from test directory)
//
// Returns:
//   - *api.Client: configured API client with mTLS and CA verification
//   - *config.Config: loaded CLI configuration (for ports and paths)
func NewLiveOperatorHTTPClient(t require.TestingT, repoRoot string) (*api.Client, *config.Config) {
	// Load CLI config to get ports and paths
	cliCfg, err := config.Load(repoRoot)
	require.NoError(t, err, "failed to load CLI config")

	// Use platform's api.NewClient with 5-second timeout
	client, err := api.NewClient(cliCfg)
	require.NoError(t, err, "failed to create API client - run './g8e auth login' first")

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
	client := &http.Client{Timeout: 5 * time.Second}

	require.Eventually(t, func() bool {
		resp, err := client.Get(healthURL)
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
	}, 5*time.Second, 500*time.Millisecond, "gateway did not become governance-ready within timeout")
}

// EnsureAuthLogin ensures the CLI has a fresh session by running './g8e auth login'.
// This is called automatically by integration tests to bootstrap credentials.
func EnsureAuthLogin(t *testing.T, repoRoot string) {
	t.Helper()

	// Build the g8e binary if needed
	g8ePath := filepath.Join(repoRoot, "g8e")
	if _, err := os.Stat(g8ePath); os.IsNotExist(err) {
		// Build the binary
		buildCmd := exec.Command("go", "build", "-o", g8ePath, "./cmd/operator")
		buildCmd.Dir = repoRoot
		if output, err := buildCmd.CombinedOutput(); err != nil {
			require.NoError(t, err, "failed to build g8e binary: %s", string(output))
		}
	}

	// Check if gateway is already running by examining status output
	// (gw status always exits 0, so we must parse output rather than checking err)
	checkCmd := exec.Command(g8ePath, "gw", "status")
	checkCmd.Dir = repoRoot
	checkOutput, _ := checkCmd.CombinedOutput()
	if strings.Contains(string(checkOutput), "STOPPED") {
		// Gateway not running, start it
		t.Logf("Gateway not running, starting it...")
		startCmd := exec.Command(g8ePath, "gw", "start")
		startCmd.Dir = repoRoot
		if output, err := startCmd.CombinedOutput(); err != nil {
			require.NoError(t, err, "failed to start gateway: %s", string(output))
		}
		// Wait for gateway to be ready
		time.Sleep(3 * time.Second)
	}

	// Skip re-enrollment if credentials are fresh (< 45 min old).
	// CLI sessions last 1 hour; re-enrolling concurrently from parallel tests
	// causes rate-limit failures and unnecessary churn.
	credsPath := filepath.Join(repoRoot, ".g8e", "credentials")
	if info, err := os.Stat(credsPath); err == nil {
		if time.Since(info.ModTime()) < 45*time.Minute {
			t.Logf("Credentials are fresh (%v old), skipping re-enrollment", time.Since(info.ModTime()).Round(time.Second))
			return
		}
	}

	// Run './g8e auth login' with explicit endpoint
	loginCmd := exec.Command(g8ePath, "auth", "login")
	loginCmd.Dir = repoRoot
	if output, err := loginCmd.CombinedOutput(); err != nil {
		require.NoError(t, err, "failed to run './g8e auth login': %s", string(output))
	}
}

// RunCLICommand executes ./g8e commands with proper error handling and output capture.
// It ensures the g8e binary is built, sets the working directory to the repo root,
// and returns the combined stdout/stderr output along with any error.
//
// Parameters:
//   - t: testing.T for assertions
//   - repoRoot: path to the repository root
//   - args: command arguments (e.g., "gw", "status")
//
// Returns:
//   - string: combined stdout/stderr output
//   - error: nil if command succeeded, otherwise the execution error
func RunCLICommand(t *testing.T, repoRoot string, args ...string) (string, error) {
	t.Helper()

	g8ePath := filepath.Join(repoRoot, "g8e")
	if _, err := os.Stat(g8ePath); os.IsNotExist(err) {
		// Build the binary if it doesn't exist
		buildCmd := exec.Command("go", "build", "-o", g8ePath, "./cmd/operator")
		buildCmd.Dir = repoRoot
		if output, err := buildCmd.CombinedOutput(); err != nil {
			require.NoError(t, err, "failed to build g8e binary: %s", string(output))
		}
	}

	cmd := exec.Command(g8ePath, args...)
	cmd.Dir = repoRoot
	output, err := cmd.CombinedOutput()
	return string(output), err
}

// RunCLICommandRequire executes a CLI command and requires it to succeed.
// If the command fails, it calls t.Fatalf with the output.
//
// This is a convenience wrapper around RunCLICommand for cases where
// the command must succeed for the test to continue.
func RunCLICommandRequire(t *testing.T, repoRoot string, args ...string) string {
	t.Helper()

	output, err := RunCLICommand(t, repoRoot, args...)
	if err != nil {
		t.Fatalf("CLI command '%v' failed: %v\nOutput: %s", args, err, output)
	}
	return output
}
