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

package fixtures

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// DockerOperatorFixture manages a Docker-based operator for testing.
// It provides lifecycle management (start/stop) for operator containers,
// enabling true multi-operator testing scenarios.
type DockerOperatorFixture struct {
	ContainerID string
	ImageName   string
	Hostname    string
	GatewayURL  string
	NetworkName string
	Cleanup     func()
}

// DockerOperatorOptions configures the Docker operator fixture.
type DockerOperatorOptions struct {
	ImageName   string // Docker image name (e.g., "g8e-operator:test")
	Hostname    string // Operator hostname
	GatewayURL  string // Gateway URL for operator to connect to
	NetworkName string // Docker network name (optional)
	EnvVars     map[string]string
	AutoRemove  bool // Whether to automatically remove container on exit
}

// NewDockerOperatorFixture creates and starts a Docker-based operator.
// It builds the operator image if needed, creates a container with the
// specified configuration, and returns a fixture for managing it.
//
// The returned fixture includes a Cleanup function that should be called
// in a defer statement to stop and remove the container.
func NewDockerOperatorFixture(t *testing.T, opts DockerOperatorOptions) *DockerOperatorFixture {
	t.Helper()

	// Set defaults
	if opts.ImageName == "" {
		opts.ImageName = "g8e-operator:test"
	}
	if opts.Hostname == "" {
		opts.Hostname = fmt.Sprintf("test-operator-%d", time.Now().UnixNano())
	}
	if opts.GatewayURL == "" {
		opts.GatewayURL = "https://host.docker.internal:8443"
	}
	if opts.AutoRemove {
		// Default to auto-remove for easier cleanup
		opts.AutoRemove = true
	}

	// Build the operator image if it doesn't exist
	if !dockerImageExists(t, opts.ImageName) {
		buildDockerOperatorImage(t, opts.ImageName)
	}

	// Create Docker network if specified
	if opts.NetworkName != "" {
		createDockerNetwork(t, opts.NetworkName)
	}

	// Prepare docker run command
	args := []string{"run", "-d"}
	if opts.AutoRemove {
		args = append(args, "--rm")
	}
	args = append(args, "--name", opts.Hostname)
	args = append(args, "--hostname", opts.Hostname)

	// Add network if specified
	if opts.NetworkName != "" {
		args = append(args, "--network", opts.NetworkName)
	}

	// Add environment variables
	args = append(args, "-e", fmt.Sprintf("G8E_GATEWAY_URL=%s", opts.GatewayURL))
	for k, v := range opts.EnvVars {
		args = append(args, "-e", fmt.Sprintf("%s=%s", k, v))
	}

	// Add image name
	args = append(args, opts.ImageName)

	// Run the container
	cmd := exec.Command("docker", args...)
	output, err := cmd.CombinedOutput()
	require.NoError(t, err, "failed to start docker container: %s", string(output))

	containerID := strings.TrimSpace(string(output))
	t.Logf("Started Docker operator container: %s (hostname: %s)", containerID, opts.Hostname)

	// Wait for container to be running
	require.Eventually(t, func() bool {
		statusCmd := exec.Command("docker", "inspect", "-f", "{{.State.Status}}", containerID)
		statusOutput, err := statusCmd.CombinedOutput()
		if err != nil {
			return false
		}
		return strings.TrimSpace(string(statusOutput)) == "running"
	}, 30*time.Second, 1*time.Second, "container did not reach running state")

	// Create cleanup function
	cleanup := func() {
		if opts.AutoRemove {
			// Container will be auto-removed on exit, but stop it gracefully
			stopCmd := exec.Command("docker", "stop", containerID)
			_ = stopCmd.Run()
		} else {
			// Stop and remove the container
			stopCmd := exec.Command("docker", "stop", containerID)
			_ = stopCmd.Run()
			rmCmd := exec.Command("docker", "rm", containerID)
			_ = rmCmd.Run()
		}
	}

	return &DockerOperatorFixture{
		ContainerID: containerID,
		ImageName:   opts.ImageName,
		Hostname:    opts.Hostname,
		GatewayURL:  opts.GatewayURL,
		NetworkName: opts.NetworkName,
		Cleanup:     cleanup,
	}
}

// Stop stops the operator container.
func (f *DockerOperatorFixture) Stop(t *testing.T) {
	t.Helper()
	cmd := exec.Command("docker", "stop", f.ContainerID)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Logf("Warning: failed to stop container %s: %v\nOutput: %s", f.ContainerID, err, string(output))
	}
}

// GetLogs retrieves the container logs.
func (f *DockerOperatorFixture) GetLogs(t *testing.T) string {
	t.Helper()
	cmd := exec.Command("docker", "logs", f.ContainerID)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Logf("Warning: failed to get logs for container %s: %v", f.ContainerID, err)
	}
	return string(output)
}

// ExecCommand executes a command inside the container.
func (f *DockerOperatorFixture) ExecCommand(t *testing.T, command string, args ...string) (string, error) {
	t.Helper()
	dockerArgs := []string{"exec", f.ContainerID, command}
	dockerArgs = append(dockerArgs, args...)
	cmd := exec.Command("docker", dockerArgs...)
	output, err := cmd.CombinedOutput()
	return string(output), err
}

// WaitForReady waits for the operator to be ready by checking logs for a readiness marker.
func (f *DockerOperatorFixture) WaitForReady(t *testing.T, timeout time.Duration, marker string) {
	t.Helper()
	require.Eventually(t, func() bool {
		logs := f.GetLogs(t)
		return strings.Contains(logs, marker)
	}, timeout, 1*time.Second, "operator did not become ready (marker '%s' not found in logs)", marker)
}

// dockerImageExists checks if a Docker image exists locally.
func dockerImageExists(t *testing.T, imageName string) bool {
	t.Helper()
	cmd := exec.Command("docker", "inspect", "--type=image", imageName)
	err := cmd.Run()
	return err == nil
}

// buildDockerOperatorImage builds the operator Docker image.
func buildDockerOperatorImage(t *testing.T, imageName string) {
	t.Helper()

	// Check if Dockerfile.operator exists
	repoRoot := ResolveRepoRootFromTestDir(t)
	dockerfilePath := repoRoot + "/Dockerfile.operator"
	if _, err := os.Stat(dockerfilePath); os.IsNotExist(err) {
		t.Fatalf("Dockerfile.operator not found at %s", dockerfilePath)
	}

	t.Logf("Building Docker operator image: %s", imageName)
	cmd := exec.Command("docker", "build", "-f", dockerfilePath, "-t", imageName, repoRoot)
	cmd.Dir = repoRoot
	output, err := cmd.CombinedOutput()
	require.NoError(t, err, "failed to build docker image: %s", string(output))
	t.Logf("Successfully built Docker operator image: %s", imageName)
}

// createDockerNetwork creates a Docker network if it doesn't exist.
func createDockerNetwork(t *testing.T, networkName string) {
	t.Helper()

	// Check if network exists
	cmd := exec.Command("docker", "network", "inspect", networkName)
	if cmd.Run() == nil {
		// Network already exists
		return
	}

	// Create the network
	createCmd := exec.Command("docker", "network", "create", networkName)
	output, err := createCmd.CombinedOutput()
	require.NoError(t, err, "failed to create docker network: %s", string(output))
	t.Logf("Created Docker network: %s", networkName)
}

// ResolveRepoRootFromTestDir finds the repository root using go list -m.
// This is duplicated from integration_helper.go to avoid circular dependencies
// in the fixtures package.
func ResolveRepoRootFromTestDir(t *testing.T) string {
	t.Helper()
	cmd := exec.Command("go", "list", "-m", "-f", "{{.Dir}}")
	output, err := cmd.Output()
	require.NoError(t, err, "failed to run go list -m to find repository root")

	repoRoot := strings.TrimSpace(string(output))
	require.NotEmpty(t, repoRoot, "go list -m returned empty directory")

	return repoRoot
}
