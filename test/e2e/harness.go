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
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/test"
)

// DockerE2EFixture spins up docker-compose, waits for health, and tears down on cleanup.
// It tests ONLY what is observable from outside the container — HTTP health, CA bundle
// discovery, and port reachability. mTLS protocol tests belong in tier 2.
type DockerE2EFixture struct {
	GatewayHTTPURL  string // http://localhost:<httpPort>
	GatewayHTTPSURL string // https://localhost:<httpsPort> (no client cert for these tests)
	ComposeFile     string
	ProjectDir      string
	ProjectName     string // unique docker compose project name
	ContainerPrefix string // unique container name prefix
	HTTPPort        int    // allocated host HTTP port
	HTTPSPort       int    // allocated host HTTPS port
}

// NewDockerE2EFixture creates a Docker E2E fixture for testing.
// It spins up docker-compose, waits for health, and registers cleanup.
// Ports are allocated sequentially starting from 8080/8443 to avoid
// conflicts with other running instances.
func NewDockerE2EFixture(t *testing.T, composeFile string) *DockerE2EFixture {
	t.Helper()

	// Check G8E_E2E_SKIP_DOCKER=1 to skip if no Docker available
	if os.Getenv("G8E_E2E_SKIP_DOCKER") == "1" {
		t.Skip("Skipping Docker E2E tests (G8E_E2E_SKIP_DOCKER=1)")
	}

	// Resolve repository root
	repoRoot := tests.ResolveRepoRootFromTestDir(t)

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

	// Allocate available ports sequentially starting from 8080/8443
	httpPort, httpsPort := 8080, 8443
	for offset := 0; offset < 1000; offset++ {
		candidateHTTP := 8080 + offset
		candidateHTTPS := 8443 + offset
		lnHTTP, errHTTP := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", candidateHTTP))
		if errHTTP == nil {
			lnHTTP.Close()
			lnHTTPS, errHTTPS := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", candidateHTTPS))
			if errHTTPS == nil {
				lnHTTPS.Close()
				httpPort, httpsPort = candidateHTTP, candidateHTTPS
				break
			}
		}
	}
	if httpPort == 8080 {
		ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", httpPort))
		if err != nil {
			t.Fatalf("No available port pair found in range 8080-9080")
		}
		ln.Close()
	}
	containerPrefix := fmt.Sprintf("g8e-%d", httpPort)
	projectName := containerPrefix

	t.Logf("Allocated ports: HTTP=%d HTTPS=%d (prefix=%s)", httpPort, httpsPort, containerPrefix)

	// Build env for docker-compose (overrides defaults in compose file)
	composeEnv := []string{
		fmt.Sprintf("G8E_HTTP_PORT=%d", httpPort),
		fmt.Sprintf("G8E_HTTPS_PORT=%d", httpsPort),
		fmt.Sprintf("G8E_PREFIX=%s", containerPrefix),
	}

	// Spin up docker-compose with unique project name and env
	t.Logf("Starting docker-compose with file: %s (project: %s)", composePath, projectName)
	cmd := exec.Command("docker", "compose", "-p", projectName, "-f", composePath, "up", "-d", "--build")
	cmd.Dir = repoRoot
	cmd.Env = append(os.Environ(), composeEnv...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("Failed to start docker-compose: %v\nOutput: %s", err, string(output))
	}
	t.Logf("Docker compose started: %s", string(output))

	// Wait for health endpoint
	httpURL := fmt.Sprintf("http://localhost:%d", httpPort)
	httpsURL := fmt.Sprintf("https://localhost:%d", httpsPort)
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
		t.Logf("Stopping docker-compose (project: %s)...", projectName)
		downCmd := exec.Command("docker", "compose", "-p", projectName, "-f", composePath, "down", "-v", "--remove-orphans")
		downCmd.Dir = repoRoot
		downCmd.Env = append(os.Environ(), composeEnv...)
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
		ProjectName:     projectName,
		ContainerPrefix: containerPrefix,
		HTTPPort:        httpPort,
		HTTPSPort:       httpsPort,
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
	resp, err := client.Get(f.GatewayHTTPURL + "/.well-known/g8e/pki/ca-bundle")
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

	opContainerName := f.ContainerPrefix + "-operator"

	// Check container status
	cmd := exec.Command("docker", "inspect", "-f", "{{.State.Status}}", opContainerName)
	output, err := cmd.CombinedOutput()
	require.NoError(t, err, "Failed to inspect operator container")
	status := strings.TrimSpace(string(output))
	require.Equal(t, "running", status, "Operator container is not running")

	// Check logs for connection success marker
	logs := f.OperatorLogs(t)
	require.Contains(t, logs, "Authentication successful", "Operator logs do not contain authentication success marker")
}

// OperatorLogs returns the combined stdout/stderr logs of the operator container.
// This is a black-box observation helper — it only reads container logs, never
// accesses files inside the container or opens mTLS connections from the test process.
func (f *DockerE2EFixture) OperatorLogs(t *testing.T) string {
	t.Helper()

	opContainerName := f.ContainerPrefix + "-operator"
	logsCmd := exec.Command("docker", "logs", opContainerName)
	logsOutput, err := logsCmd.CombinedOutput()
	require.NoError(t, err, "Failed to get operator logs")
	return string(logsOutput)
}

// RestartOperator restarts the operator container and waits for it to become
// healthy again by polling the gateway health endpoint. This is a black-box
// helper — it uses `docker restart` and HTTP health checks only, no in-container
// file access or mTLS from the test process.
func (f *DockerE2EFixture) RestartOperator(t *testing.T) {
	t.Helper()

	opContainerName := f.ContainerPrefix + "-operator"

	t.Logf("Restarting operator container: %s", opContainerName)
	restartCmd := exec.Command("docker", "restart", opContainerName)
	restartOutput, err := restartCmd.CombinedOutput()
	require.NoError(t, err, "Failed to restart operator container: %s", string(restartOutput))

	// Wait for operator to re-authenticate by checking logs for the auth success marker
	client := &http.Client{Timeout: 2 * time.Second}
	require.Eventually(t, func() bool {
		// Verify gateway is still healthy
		resp, err := client.Get(f.GatewayHTTPURL + "/api/v1/health")
		if err != nil {
			return false
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			return false
		}

		// Check operator logs for re-authentication
		logsCmd := exec.Command("docker", "logs", "--since", "10s", opContainerName)
		logsOutput, err := logsCmd.CombinedOutput()
		if err != nil {
			return false
		}
		return strings.Contains(string(logsOutput), "Authentication successful")
	}, 120*time.Second, 2*time.Second, "Operator did not re-authenticate within 120s after restart")
}
