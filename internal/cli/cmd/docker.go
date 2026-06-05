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

package cmd

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"

	"github.com/spf13/cobra"
)

func dockerCmd() *cobra.Command {
	var numGateways int
	var numOperators int
	var startOnly bool
	var stop bool
	var down bool

	cmd := &cobra.Command{
		Use:   "docker",
		Short: "Manage g8e Docker containers for testing",
		Long: `Start and manage multiple g8e Gateway and Operator containers in Docker.
This command copies the pre-built g8e binary, generates a docker-compose.yml with the specified
number of gateways and operators, and orchestrates their startup with proper port allocation.
Run 'make build' first to build the binary.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if stop {
				return dockerStop()
			}
			if down {
				return dockerDown()
			}
			if startOnly {
				return dockerStart(numGateways, numOperators)
			}
			return dockerUp(numGateways, numOperators)
		},
	}

	cmd.Flags().IntVar(&numGateways, "gateways", 1, "Number of gateway instances to start")
	cmd.Flags().IntVar(&numOperators, "operators", 1, "Number of operator instances to start")
	cmd.Flags().BoolVar(&startOnly, "start-only", false, "Start existing containers without regenerating compose file")
	cmd.Flags().BoolVar(&stop, "stop", false, "Stop all running containers")
	cmd.Flags().BoolVar(&down, "down", false, "Stop and remove all containers and volumes")

	return cmd
}

func dockerUp(numGateways, numOperators int) error {
	if numGateways < 1 {
		return fmt.Errorf("at least 1 gateway is required")
	}
	if numOperators < 0 {
		return fmt.Errorf("number of operators cannot be negative")
	}

	// Check if binary exists
	if _, err := os.Stat("bin/g8e-linux-amd64"); os.IsNotExist(err) {
		return fmt.Errorf("binary not found at bin/g8e-linux-amd64 - run 'make build' first")
	}

	fmt.Printf("[g8e] Building Docker images for %d gateway(s) and %d operator(s)...\n", numGateways, numOperators)

	// Create docker network if it doesn't exist
	fmt.Println("[g8e] Ensuring docker network exists...")
	networkCmd := exec.Command("docker", "network", "create", "g8e-network")
	networkCmd.Stdout = os.Stdout
	networkCmd.Stderr = os.Stderr
	networkCmd.Run() // Ignore error if network already exists

	if err := buildDockerImages(); err != nil {
		return fmt.Errorf("failed to build Docker images: %w", err)
	}

	fmt.Printf("[g8e] Starting gateway containers...\n")
	for i := 0; i < numGateways; i++ {
		if err := startGatewayContainer(i); err != nil {
			return fmt.Errorf("failed to start gateway%d: %w", i, err)
		}
	}

	fmt.Printf("[g8e] Starting operator containers...\n")
	for i := 0; i < numOperators; i++ {
		if err := startOperatorContainer(i); err != nil {
			return fmt.Errorf("failed to start operator%d: %w", i, err)
		}
	}

	printContainerInfo(numGateways, numOperators)
	return nil
}

func dockerStart(numGateways, numOperators int) error {
	fmt.Printf("[g8e] Starting %d gateway(s) and %d operator(s)...\n", numGateways, numOperators)
	for i := 0; i < numGateways; i++ {
		if err := startGatewayContainer(i); err != nil {
			return fmt.Errorf("failed to start gateway%d: %w", i, err)
		}
	}
	for i := 0; i < numOperators; i++ {
		if err := startOperatorContainer(i); err != nil {
			return fmt.Errorf("failed to start operator%d: %w", i, err)
		}
	}
	printContainerInfo(numGateways, numOperators)
	return nil
}

func dockerStop() error {
	fmt.Println("[g8e] Stopping all g8e containers...")
	cmd := exec.Command("docker", "ps", "-q", "--filter", "name=g8e-")
	output, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("failed to list containers: %w", err)
	}

	containerIDs := bytes.TrimSpace(output)
	if len(containerIDs) > 0 {
		stopCmd := exec.Command("docker", "stop")
		stopCmd.Stdin = bytes.NewReader(containerIDs)
		stopCmd.Stdout = os.Stdout
		stopCmd.Stderr = os.Stderr
		if err := stopCmd.Run(); err != nil {
			return fmt.Errorf("failed to stop containers: %w", err)
		}
	}
	fmt.Println("[g8e] Containers stopped.")
	return nil
}

func dockerDown() error {
	fmt.Println("[g8e] Stopping and removing all g8e containers and volumes...")
	cmd := exec.Command("docker", "ps", "-q", "--filter", "name=g8e-")
	output, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("failed to list containers: %w", err)
	}

	containerIDs := bytes.TrimSpace(output)
	if len(containerIDs) > 0 {
		stopCmd := exec.Command("docker", "stop")
		stopCmd.Stdin = bytes.NewReader(containerIDs)
		stopCmd.Stdout = os.Stdout
		stopCmd.Stderr = os.Stderr
		if err := stopCmd.Run(); err != nil {
			return fmt.Errorf("failed to stop containers: %w", err)
		}

		rmCmd := exec.Command("docker", "rm")
		rmCmd.Stdin = bytes.NewReader(containerIDs)
		rmCmd.Stdout = os.Stdout
		rmCmd.Stderr = os.Stderr
		if err := rmCmd.Run(); err != nil {
			return fmt.Errorf("failed to remove containers: %w", err)
		}
	}

	// Remove volumes
	volCmd := exec.Command("docker", "volume", "ls", "-q", "--filter", "name=g8e-")
	volOutput, err := volCmd.Output()
	if err != nil {
		return fmt.Errorf("failed to list volumes: %w", err)
	}

	volumeIDs := bytes.TrimSpace(volOutput)
	if len(volumeIDs) > 0 {
		rmVolCmd := exec.Command("docker", "volume", "rm")
		rmVolCmd.Stdin = bytes.NewReader(volumeIDs)
		rmVolCmd.Stdout = os.Stdout
		rmVolCmd.Stderr = os.Stderr
		if err := rmVolCmd.Run(); err != nil {
			return fmt.Errorf("failed to remove volumes: %w", err)
		}
	}

	fmt.Println("[g8e] Containers and volumes removed.")
	return nil
}

func buildDockerImages() error {
	// Build gateway image
	fmt.Println("[g8e] Building gateway image...")
	gwCmd := exec.Command("docker", "build", "-f", "docker/Dockerfile.gateway", "-t", "g8e-gateway:latest", ".")
	gwCmd.Stdout = os.Stdout
	gwCmd.Stderr = os.Stderr
	if err := gwCmd.Run(); err != nil {
		return fmt.Errorf("failed to build gateway image: %w", err)
	}

	// Build operator image
	fmt.Println("[g8e] Building operator image...")
	opCmd := exec.Command("docker", "build", "-f", "docker/Dockerfile.operator", "-t", "g8e-operator:latest", ".")
	opCmd.Stdout = os.Stdout
	opCmd.Stderr = os.Stderr
	if err := opCmd.Run(); err != nil {
		return fmt.Errorf("failed to build operator image: %w", err)
	}

	return nil
}

func startGatewayContainer(index int) error {
	containerName := fmt.Sprintf("g8e-gateway%d", index)
	basePort := 9000 + (index * 10)

	// Check if container already exists
	checkCmd := exec.Command("docker", "ps", "-a", "-q", "--filter", fmt.Sprintf("name=%s", containerName))
	if output, _ := checkCmd.Output(); len(bytes.TrimSpace(output)) > 0 {
		// Container exists, remove it first
		rmCmd := exec.Command("docker", "rm", "-f", containerName)
		rmCmd.Stdout = os.Stdout
		rmCmd.Stderr = os.Stderr
		if err := rmCmd.Run(); err != nil {
			return fmt.Errorf("failed to remove existing container: %w", err)
		}
	}

	// Create volumes
	dataVol := fmt.Sprintf("gateway%d-data", index)
	pkiVol := fmt.Sprintf("gateway%d-pki", index)
	secretsVol := fmt.Sprintf("gateway%d-secrets", index)

	for _, vol := range []string{dataVol, pkiVol, secretsVol} {
		exec.Command("docker", "volume", "create", vol).Run()
	}

	// Start container
	cmd := exec.Command("docker", "run", "-d",
		"--name", containerName,
		"--hostname", fmt.Sprintf("gateway%d", index),
		"--network", "g8e-network",
		"-p", fmt.Sprintf("%d:9000", basePort),
		"-p", fmt.Sprintf("%d:9001", basePort+1),
		"-p", fmt.Sprintf("%d:9002", basePort+2),
		"-v", fmt.Sprintf("%s:/home/g8eg/.g8e/data", dataVol),
		"-v", fmt.Sprintf("%s:/home/g8eg/.g8e/pki", pkiVol),
		"-v", fmt.Sprintf("%s:/home/g8eg/.g8e/secrets", secretsVol),
		"g8e-gateway:latest")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to start container: %w", err)
	}

	fmt.Printf("[g8e] Started %s (ports: %d, %d, %d)\n", containerName, basePort, basePort+1, basePort+2)
	return nil
}

func startOperatorContainer(index int) error {
	containerName := fmt.Sprintf("g8e-operator%d", index)
	basePort := 9020 + (index * 10)

	// Check if container already exists
	checkCmd := exec.Command("docker", "ps", "-a", "-q", "--filter", fmt.Sprintf("name=%s", containerName))
	if output, _ := checkCmd.Output(); len(bytes.TrimSpace(output)) > 0 {
		// Container exists, remove it first
		rmCmd := exec.Command("docker", "rm", "-f", containerName)
		rmCmd.Stdout = os.Stdout
		rmCmd.Stderr = os.Stderr
		if err := rmCmd.Run(); err != nil {
			return fmt.Errorf("failed to remove existing container: %w", err)
		}
	}

	// Create volumes
	dataVol := fmt.Sprintf("operator%d-data", index)
	pkiVol := fmt.Sprintf("operator%d-pki", index)
	secretsVol := fmt.Sprintf("operator%d-secrets", index)

	for _, vol := range []string{dataVol, pkiVol, secretsVol} {
		exec.Command("docker", "volume", "create", vol).Run()
	}

	// Start container
	cmd := exec.Command("docker", "run", "-d",
		"--name", containerName,
		"--hostname", fmt.Sprintf("operator%d", index),
		"--network", "g8e-network",
		"-p", fmt.Sprintf("%d:9000", basePort),
		"-p", fmt.Sprintf("%d:9001", basePort+1),
		"-p", fmt.Sprintf("%d:9002", basePort+2),
		"-v", fmt.Sprintf("%s:/home/g8eo/.g8e/data", dataVol),
		"-v", fmt.Sprintf("%s:/home/g8eo/.g8e/pki", pkiVol),
		"-v", fmt.Sprintf("%s:/home/g8eo/.g8e/secrets", secretsVol),
		"g8e-operator:latest")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to start container: %w", err)
	}

	fmt.Printf("[g8e] Started %s (ports: %d, %d, %d)\n", containerName, basePort, basePort+1, basePort+2)
	return nil
}

func printContainerInfo(numGateways, numOperators int) {
	fmt.Println()
	fmt.Println(" ┌── Docker Containers ───────────────────────────────────────────────────┐")
	fmt.Printf(" │ ✔ Gateways: %d\n", numGateways)
	for i := 0; i < numGateways; i++ {
		basePort := 9000 + (i * 10)
		fmt.Printf(" │   - gateway%d: https://localhost:%d (mTLS), %d (bootstrap), %d (public)\n", i, basePort, basePort+1, basePort+2)
	}
	fmt.Printf(" │ ✔ Operators: %d\n", numOperators)
	for i := 0; i < numOperators; i++ {
		basePort := 9020 + (i * 10)
		fmt.Printf(" │   - operator%d: https://localhost:%d (mTLS), %d (bootstrap), %d (public)\n", i, basePort, basePort+1, basePort+2)
	}
	fmt.Println(" └──────────────────────────────────────────────────────────────────────────┘")
	fmt.Println()
	fmt.Println("View logs: docker logs -f <container-name>")
	fmt.Println("Stop all:  ./g8e docker --stop")
	fmt.Println("Clean up:  ./g8e docker --down")
}
