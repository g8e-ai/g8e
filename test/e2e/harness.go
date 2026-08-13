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
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/internal/constants"
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

// setupSharedE2EFixture performs the Docker Compose setup without requiring
// a *testing.T, enabling use from TestMain. It allocates ports, builds and
// starts the stack, waits for health, and returns the fixture. On failure it
// tears down any partially-started stack before returning the error.
func setupSharedE2EFixture(composeFile string) (*DockerE2EFixture, error) {
	// Resolve repository root via go list -m
	repoCmd := exec.Command("go", "list", "-m", "-f", "{{.Dir}}")
	repoOutput, err := repoCmd.Output()
	if err != nil {
		return nil, fmt.Errorf("resolve repo root: %w", err)
	}
	repoRoot := filepath.Clean(strings.TrimSpace(string(repoOutput)))
	if repoRoot == "" {
		return nil, fmt.Errorf("go list -m returned empty directory")
	}

	// Build absolute path to compose file
	var composePath string
	if filepath.IsAbs(composeFile) {
		composePath = composeFile
	} else {
		composePath = filepath.Join(repoRoot, composeFile)
	}

	// Verify compose file exists
	if _, err := os.Stat(composePath); os.IsNotExist(err) {
		return nil, fmt.Errorf("compose file not found: %s", composePath)
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
			return nil, fmt.Errorf("no available port pair found in range 8080-9080")
		}
		ln.Close()
	}
	containerPrefix := fmt.Sprintf("g8e-%d", httpPort)
	projectName := containerPrefix

	log.Printf("E2E: Allocated ports HTTP=%d HTTPS=%d (prefix=%s)", httpPort, httpsPort, containerPrefix)

	// Build env for docker-compose (overrides defaults in compose file)
	composeEnv := []string{
		"DOCKER_BUILDKIT=1",
		fmt.Sprintf("G8E_HTTP_PORT=%d", httpPort),
		fmt.Sprintf("G8E_HTTPS_PORT=%d", httpsPort),
		fmt.Sprintf("G8E_PREFIX=%s", containerPrefix),
	}

	// Spin up docker-compose with unique project name and env
	log.Printf("E2E: Starting docker-compose (project: %s)", projectName)
	upCmd := exec.Command("docker", "compose", "-p", projectName, "-f", composePath, "up", "-d", "--build")
	upCmd.Dir = repoRoot
	upCmd.Env = append(os.Environ(), composeEnv...)
	upOutput, err := upCmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("docker compose up failed: %w\nOutput: %s", err, string(upOutput))
	}
	log.Printf("E2E: Docker compose started: %s", string(upOutput))

	httpURL := fmt.Sprintf("http://localhost:%d", httpPort)
	httpsURL := fmt.Sprintf("https://localhost:%d", httpsPort)

	fixture := &DockerE2EFixture{
		GatewayHTTPURL:  httpURL,
		GatewayHTTPSURL: httpsURL,
		ComposeFile:     composePath,
		ProjectDir:      repoRoot,
		ProjectName:     projectName,
		ContainerPrefix: containerPrefix,
		HTTPPort:        httpPort,
		HTTPSPort:       httpsPort,
	}

	// Wait for health endpoint
	log.Printf("E2E: Waiting for gateway health at %s...", httpURL)
	client := &http.Client{Timeout: 2 * time.Second}
	deadline := time.Now().Add(120 * time.Second)
	healthy := false
	for time.Now().Before(deadline) {
		resp, err := client.Get(httpURL + "/api/v1/health")
		if err == nil {
			ok := resp.StatusCode == http.StatusOK
			resp.Body.Close()
			if ok {
				healthy = true
				break
			}
		}
		time.Sleep(2 * time.Second)
	}
	if !healthy {
		if err := fixture.teardown(); err != nil {
			log.Printf("E2E: teardown after health-wait failure also failed: %v", err)
		}
		return nil, fmt.Errorf("gateway did not become healthy within 120s")
	}
	log.Printf("E2E: Gateway is healthy")

	return fixture, nil
}

// teardown stops the Docker Compose stack and removes volumes and orphans.
func (f *DockerE2EFixture) teardown() error {
	log.Printf("E2E: Stopping docker-compose (project: %s)...", f.ProjectName)
	composeEnv := []string{
		fmt.Sprintf("G8E_HTTP_PORT=%d", f.HTTPPort),
		fmt.Sprintf("G8E_HTTPS_PORT=%d", f.HTTPSPort),
		fmt.Sprintf("G8E_PREFIX=%s", f.ContainerPrefix),
	}
	downCmd := exec.Command("docker", "compose", "-p", f.ProjectName, "-f", f.ComposeFile, "down", "-v", "--remove-orphans")
	downCmd.Dir = f.ProjectDir
	downCmd.Env = append(os.Environ(), composeEnv...)
	output, err := downCmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("docker compose down failed: %w: %s", err, string(output))
	}
	log.Printf("E2E: Docker compose stopped")
	return nil
}

// NewDockerE2EFixture creates a per-test Docker E2E fixture. It spins up
// docker-compose, waits for health, and registers cleanup via t.Cleanup.
// For shared fixture across all tests, prefer TestMain + sharedFixture.
func NewDockerE2EFixture(t *testing.T, composeFile string) *DockerE2EFixture {
	t.Helper()

	if os.Getenv("G8E_E2E_SKIP_DOCKER") == "1" {
		t.Skip("Skipping Docker E2E tests (G8E_E2E_SKIP_DOCKER=1)")
	}

	fixture, err := setupSharedE2EFixture(composeFile)
	if err != nil {
		t.Fatalf("Failed to set up Docker E2E fixture: %v", err)
	}

	// Teardown cleanup: stops and removes the compose stack.
	t.Cleanup(func() {
		if err := fixture.teardown(); err != nil {
			t.Logf("Warning: failed to stop docker-compose: %v", err)
		}
	})

	// Failure-capture cleanup: registered AFTER teardown so it runs FIRST
	// (t.Cleanup is LIFO), while the containers are still up. Only captures
	// when the test actually failed, avoiding diagnostic noise on success.
	t.Cleanup(func() {
		if t.Failed() {
			fixture.captureDiagnostics(t.Logf)
		}
	})

	return fixture
}

// captureDiagnostics collects gateway/operator container logs and the compose
// ps state into files under a fresh temp dir, then logs the dir path via msg.
// Containers must still be up when called — invoke before teardown. msg is
// log.Printf for the TestMain path (no *testing.T available) and t.Logf for
// the per-test path. Purely diagnostic; changes no assertions.
func (f *DockerE2EFixture) captureDiagnostics(msg func(format string, args ...any)) {
	dir, err := os.MkdirTemp("", "g8e-e2e-diag-*")
	if err != nil {
		msg("E2E: failed to create diagnostics dir: %v", err)
		return
	}

	gatewayContainer := f.ContainerPrefix + "-gateway"
	operatorContainer := f.ContainerPrefix + "-operator"

	captures := []struct {
		name string
		cmd  *exec.Cmd
	}{
		{"gateway.log", exec.Command("docker", "logs", gatewayContainer)},
		{"operator.log", exec.Command("docker", "logs", operatorContainer)},
		{"compose-ps.txt", exec.Command("docker", "compose", "-p", f.ProjectName, "-f", f.ComposeFile, "ps")},
	}

	for _, c := range captures {
		out, runErr := c.cmd.CombinedOutput()
		path := filepath.Join(dir, c.name)
		if writeErr := os.WriteFile(path, out, constants.PermFilePublic); writeErr != nil {
			msg("E2E: failed to write %s: %v", c.name, writeErr)
			continue
		}
		if runErr != nil {
			msg("E2E: captured %s (command exited with error, see file)", c.name)
		}
	}

	msg("E2E: failure diagnostics written to %s", dir)
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

// CheckOperatorContainer checks if the operator container is running and has
// authentication success in logs. It waits for the operator to complete bootstrap
// authentication before asserting, since the operator may still be enrolling
// when the gateway first becomes healthy.
func (f *DockerE2EFixture) CheckOperatorContainer(t *testing.T) {
	t.Helper()

	opContainerName := f.ContainerPrefix + "-operator"

	// Check container status
	cmd := exec.Command("docker", "inspect", "-f", "{{.State.Status}}", opContainerName)
	output, err := cmd.CombinedOutput()
	require.NoError(t, err, "Failed to inspect operator container")
	status := strings.TrimSpace(string(output))
	require.Equal(t, "running", status, "Operator container is not running")

	// Wait for operator to complete bootstrap authentication
	require.Eventually(t, func() bool {
		logsCmd := exec.Command("docker", "logs", opContainerName)
		logsOutput, err := logsCmd.CombinedOutput()
		if err != nil {
			return false
		}
		return strings.Contains(string(logsOutput), "Authentication successful")
	}, 120*time.Second, 2*time.Second, "Operator logs do not contain authentication success marker")
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
