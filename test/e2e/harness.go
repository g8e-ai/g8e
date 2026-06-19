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

//go:build e2e

package e2e

import (
	"encoding/json"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// DockerE2EFixture spins up docker-compose, waits for health, and tears down on cleanup.
// It tests ONLY what is observable from outside the container — HTTP health, CA bundle
// discovery, and port reachability. mTLS protocol tests belong in tier 2.
type DockerE2EFixture struct {
	GatewayHTTPURL  string // http://localhost:8080
	GatewayHTTPSURL string // https://localhost:8443 (no client cert for these tests)
	ComposeFile     string
	ProjectDir      string
}

// NewDockerE2EFixture creates a Docker E2E fixture for testing.
// It spins up docker-compose, waits for health, and registers cleanup.
func NewDockerE2EFixture(t *testing.T, composeFile string) *DockerE2EFixture {
	t.Helper()

	// Check G8E_E2E_SKIP_DOCKER=1 to skip if no Docker available
	if os.Getenv("G8E_E2E_SKIP_DOCKER") == "1" {
		t.Skip("Skipping Docker E2E tests (G8E_E2E_SKIP_DOCKER=1)")
	}

	// Resolve repository root
	repoRoot := resolveRepoRoot(t)

	// Build absolute path to compose file
	var composePath string
	if filepath.IsAbs(composeFile) {
		composePath = composeFile
	} else {
		composePath = filepath.Join(repoRoot, composeFile)
	}

	// Verify compose file exists
	if _, err := os.Stat(composePath); os.IsNotExist(err) {
		t.Fatalf("Compose file not found: %s", composePath)
	}

	// Spin up docker-compose
	t.Logf("Starting docker-compose with file: %s", composePath)
	cmd := exec.Command("docker", "compose", "-f", composePath, "up", "-d", "--build")
	cmd.Dir = repoRoot
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("Failed to start docker-compose: %v\nOutput: %s", err, string(output))
	}
	t.Logf("Docker compose started: %s", string(output))

	// Wait for health endpoint
	httpURL := "http://localhost:8080"
	httpsURL := "https://localhost:8443"
	t.Logf("Waiting for gateway health at %s...", httpURL)
	client := &http.Client{Timeout: 2 * time.Second}
	require.Eventually(t, func() bool {
		resp, err := client.Get(httpURL + "/api/v1/health")
		if err != nil {
			return false
		}
		defer resp.Body.Close()
		return resp.StatusCode == http.StatusOK
	}, 120*time.Second, 2*time.Second, "Gateway did not become healthy within 120s")

	// Register cleanup
	t.Cleanup(func() {
		t.Logf("Stopping docker-compose...")
		downCmd := exec.Command("docker", "compose", "-f", composePath, "down", "-v", "--remove-orphans")
		downCmd.Dir = repoRoot
		downOutput, err := downCmd.CombinedOutput()
		if err != nil {
			t.Logf("Warning: failed to stop docker-compose: %v\nOutput: %s", err, string(downOutput))
		} else {
			t.Logf("Docker compose stopped")
		}
	})

	return &DockerE2EFixture{
		GatewayHTTPURL:  httpURL,
		GatewayHTTPSURL: httpsURL,
		ComposeFile:     composePath,
		ProjectDir:      repoRoot,
	}
}

// GetHealth returns the health status from the gateway.
func (f *DockerE2EFixture) GetHealth(t *testing.T) map[string]interface{} {
	t.Helper()

	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get(f.GatewayHTTPURL + "/api/v1/health")
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var health map[string]interface{}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&health))
	return health
}

// GetCABundle returns the CA bundle from the gateway.
func (f *DockerE2EFixture) GetCABundle(t *testing.T) string {
	t.Helper()

	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get(f.GatewayHTTPURL + "/api/v1/.well-known/pki/ca-bundle")
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	bundle, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	return string(bundle)
}

// CheckOperatorContainer checks if the operator container is running and has connection success in logs.
func (f *DockerE2EFixture) CheckOperatorContainer(t *testing.T) {
	t.Helper()

	// Check container status
	cmd := exec.Command("docker", "inspect", "-f", "{{.State.Status}}", "g8e-operator")
	output, err := cmd.CombinedOutput()
	require.NoError(t, err, "Failed to inspect operator container")
	status := strings.TrimSpace(string(output))
	require.Equal(t, "running", status, "Operator container is not running")

	// Check logs for connection success marker
	logsCmd := exec.Command("docker", "logs", "g8e-operator")
	logsOutput, err := logsCmd.CombinedOutput()
	require.NoError(t, err, "Failed to get operator logs")
	logs := string(logsOutput)
	require.Contains(t, logs, "connected", "Operator logs do not contain connection success marker")
}

// resolveRepoRoot finds the repository root using runtime.Caller.
func resolveRepoRoot(t *testing.T) string {
	t.Helper()
	// Get the directory of this file using runtime.Caller
	_, filename, _, _ := runtime.Caller(0)
	testDir := filepath.Dir(filename)

	// Navigate to repository root (test/e2e -> repository root)
	repoRoot := filepath.Join(testDir, "..", "..")
	repoRoot = filepath.Clean(repoRoot)

	// Verify go.mod exists at repoRoot
	goModPath := filepath.Join(repoRoot, "go.mod")
	if _, err := os.Stat(goModPath); os.IsNotExist(err) {
		t.Fatalf("go.mod not found at %s", goModPath)
	}

	return repoRoot
}
