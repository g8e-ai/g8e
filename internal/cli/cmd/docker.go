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
	"path/filepath"
	"text/template"

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

	fmt.Printf("[g8e] Generating docker-compose.yml for %d gateway(s) and %d operator(s)...\n", numGateways, numOperators)
	dockerComposePath := filepath.Join("docker", "docker-compose.yml")
	if err := generateDockerCompose(dockerComposePath, numGateways, numOperators); err != nil {
		return fmt.Errorf("failed to generate docker-compose.yml: %w", err)
	}

	fmt.Printf("[g8e] Building Docker images...\n")
	if err := buildDockerImages(); err != nil {
		return fmt.Errorf("failed to build Docker images: %w", err)
	}

	fmt.Printf("[g8e] Starting containers (gateways first, then operators)...\n")
	if err := startContainers(); err != nil {
		return fmt.Errorf("failed to start containers: %w", err)
	}

	printContainerInfo(numGateways, numOperators)
	return nil
}

func dockerStart(numGateways, numOperators int) error {
	fmt.Printf("[g8e] Starting %d gateway(s) and %d operator(s)...\n", numGateways, numOperators)
	if err := startContainers(); err != nil {
		return fmt.Errorf("failed to start containers: %w", err)
	}
	printContainerInfo(numGateways, numOperators)
	return nil
}

func dockerStop() error {
	fmt.Println("[g8e] Stopping all containers...")
	cmd := exec.Command("docker-compose", "-f", "docker/docker-compose.yml", "stop")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to stop containers: %w", err)
	}
	fmt.Println("[g8e] Containers stopped.")
	return nil
}

func dockerDown() error {
	fmt.Println("[g8e] Stopping and removing all containers and volumes...")
	cmd := exec.Command("docker-compose", "-f", "docker/docker-compose.yml", "down", "-v")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to remove containers: %w", err)
	}
	fmt.Println("[g8e] Containers and volumes removed.")
	return nil
}

func buildBinary() error {
	// Copy the existing binary to docker/ directory for Dockerfile to pick up
	src := "bin/g8e-linux-amd64"
	dst := "docker/g8e-linux-amd64"

	// Check if source exists
	if _, err := os.Stat(src); os.IsNotExist(err) {
		return fmt.Errorf("binary not found at %s - run 'make build' first", src)
	}

	// Remove existing binary if it exists (ignore errors, file may be in use)
	os.Remove(dst)

	if err := copyFile(src, dst); err != nil {
		return fmt.Errorf("failed to copy binary: %w", err)
	}

	return nil
}

func copyFile(src, dst string) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	return os.WriteFile(dst, data, 0755)
}

func buildDockerImages() error {
	cmd := exec.Command("docker-compose", "-f", "docker/docker-compose.yml", "build")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func startContainers() error {
	cmd := exec.Command("docker-compose", "-f", "docker/docker-compose.yml", "up", "-d")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func generateDockerCompose(path string, numGateways, numOperators int) error {
	tmpl := `# Copyright (c) 2026 Lateralus Labs, LLC.
# Licensed under the Apache License, Version 2.0
# Auto-generated by ./g8e docker

services:
{{- range $i := .GatewayIndices }}
  gateway{{ $i }}:
    build:
      context: ..
      dockerfile: docker/Dockerfile.gateway
    image: g8e-gateway:latest
    container_name: g8e-gateway{{ $i }}
    hostname: gateway{{ $i }}
    networks:
      - g8e-network
    ports:
      - "{{ add 9000 (mul $i 10) }}:9000"  # HTTPS mTLS API
      - "{{ add 9001 (mul $i 10) }}:9001"  # Bootstrap TLS (CSR enrollment)
      - "{{ add 9002 (mul $i 10) }}:9002"  # Public browser/BYO bootstrap
    volumes:
      - gateway{{ $i }}-data:/home/g8eg/.g8e/data
      - gateway{{ $i }}-pki:/home/g8eg/.g8e/pki
      - gateway{{ $i }}-secrets:/home/g8eg/.g8e/secrets
    healthcheck:
      test: ["CMD", "/home/g8eg/g8e-linux-amd64", "gw", "status"]
      interval: 30s
      timeout: 10s
      retries: 3
      start_period: 5s
    restart: unless-stopped
{{- end }}
{{- range $i := .OperatorIndices }}
  operator{{ $i }}:
    build:
      context: ..
      dockerfile: docker/Dockerfile.operator
    image: g8e-operator:latest
    container_name: g8e-operator{{ $i }}
    hostname: operator{{ $i }}
    networks:
      - g8e-network
    ports:
      - "{{ add 9010 (mul $i 10) }}:9000"  # HTTPS mTLS API (offset to avoid conflict with gateway)
      - "{{ add 9011 (mul $i 10) }}:9001"  # Bootstrap TLS (CSR enrollment)
      - "{{ add 9012 (mul $i 10) }}:9002"  # Public browser/BYO bootstrap
    volumes:
      - operator{{ $i }}-data:/home/g8eo/.g8e/data
      - operator{{ $i }}-pki:/home/g8eo/.g8e/pki
      - operator{{ $i }}-secrets:/home/g8eo/.g8e/secrets
    depends_on:
{{- range $j := .GatewayIndices }}
      gateway{{ $j }}:
        condition: service_healthy
{{- end }}
    healthcheck:
      test: ["CMD", "/home/g8eo/g8e-linux-amd64", "--version"]
      interval: 30s
      timeout: 10s
      retries: 3
      start_period: 5s
    restart: unless-stopped
{{- end }}

networks:
  g8e-network:
    driver: bridge

volumes:
{{- range $i := .GatewayIndices }}
  gateway{{ $i }}-data:
    driver: local
  gateway{{ $i }}-pki:
    driver: local
  gateway{{ $i }}-secrets:
    driver: local
{{- end }}
{{- range $i := .OperatorIndices }}
  operator{{ $i }}-data:
    driver: local
  operator{{ $i }}-pki:
    driver: local
  operator{{ $i }}-secrets:
    driver: local
{{- end }}
`

	funcMap := map[string]interface{}{
		"add": func(a, b int) int { return a + b },
		"mul": func(a, b int) int { return a * b },
	}

	gatewayIndices := make([]int, numGateways)
	for i := 0; i < numGateways; i++ {
		gatewayIndices[i] = i
	}

	operatorIndices := make([]int, numOperators)
	for i := 0; i < numOperators; i++ {
		operatorIndices[i] = i
	}

	data := struct {
		GatewayIndices  []int
		OperatorIndices []int
	}{
		GatewayIndices:  gatewayIndices,
		OperatorIndices: operatorIndices,
	}

	t, err := template.New("docker-compose").Funcs(funcMap).Parse(tmpl)
	if err != nil {
		return fmt.Errorf("failed to parse template: %w", err)
	}

	var buf bytes.Buffer
	if err := t.Execute(&buf, data); err != nil {
		return fmt.Errorf("failed to execute template: %w", err)
	}

	if err := os.WriteFile(path, buf.Bytes(), 0644); err != nil {
		return fmt.Errorf("failed to write docker-compose.yml: %w", err)
	}

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
		basePort := 9010 + (i * 10)
		fmt.Printf(" │   - operator%d: https://localhost:%d (mTLS), %d (bootstrap), %d (public)\n", i, basePort, basePort+1, basePort+2)
	}
	fmt.Println(" └──────────────────────────────────────────────────────────────────────────┘")
	fmt.Println()
	fmt.Println("View logs: docker-compose -f docker/docker-compose.yml logs -f")
	fmt.Println("Stop all:  ./g8e docker --stop")
	fmt.Println("Clean up:  ./g8e docker --down")
}
