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
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"

	"github.com/spf13/cobra"
)

var (
	demoOrg string
)

func demosCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "demos",
		Short: "Manage g8e demo environments",
		Long: `Manage Docker Compose demo environments for org-specific g8e deployments.
Each org environment is hermetically sealed with no shared state, volumes, or cross-org dependencies.`,
	}

	cmd.AddCommand(
		demosListCmd(),
		demosStartCmd(),
		demosStopCmd(),
		demosStatusCmd(),
		demosCleanCmd(),
		demosResetCmd(),
		demosRunCmd(),
	)

	return cmd
}

func demosListCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List available demo environments",
		RunE:  runDemosList,
	}

	return cmd
}

func runDemosList(cmd *cobra.Command, args []string) error {
	demosDir := filepath.Join(getProjectRoot(), "demos")
	entries, err := os.ReadDir(demosDir)
	if err != nil {
		return fmt.Errorf("failed to read demos directory: %w", err)
	}

	fmt.Println("Available demo environments:")
	for _, entry := range entries {
		if entry.IsDir() && entry.Name() != "bin" {
			composePath := filepath.Join(demosDir, entry.Name(), "compose.yml")
			if _, err := os.Stat(composePath); err == nil {
				fmt.Printf("  - %s\n", entry.Name())
			}
		}
	}

	return nil
}

func demosStartCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "start <org>",
		Short: "Start a demo environment",
		Args:  cobra.ExactArgs(1),
		RunE:  runDemosStart,
	}

	return cmd
}

func runDemosStart(cmd *cobra.Command, args []string) error {
	org := args[0]
	demoDir := filepath.Join(getProjectRoot(), "demos", org)

	// Verify demo directory exists
	if _, err := os.Stat(demoDir); os.IsNotExist(err) {
		return fmt.Errorf("demo environment '%s' not found. Run 'g8e demos list' to see available demos", org)
	}

	// Verify compose.yml exists
	composePath := filepath.Join(demoDir, "compose.yml")
	if _, err := os.Stat(composePath); os.IsNotExist(err) {
		return fmt.Errorf("compose.yml not found in demo directory '%s'", org)
	}

	// Check if g8e binary exists in demos/bin
	binPath := filepath.Join(getProjectRoot(), "demos", "bin", "g8e")
	if runtime.GOOS == "windows" {
		binPath += ".exe"
	}
	if _, err := os.Stat(binPath); os.IsNotExist(err) {
		fmt.Printf("Warning: g8e binary not found at %s\n", binPath)
		fmt.Println("Run 'make build && cp g8e demos/bin/g8e' from the repository root to build it.")
	}

	// Start the demo environment
	fmt.Printf("Starting demo environment: %s\n", org)
	dockerComposeCmd := exec.Command("docker", "compose", "-f", composePath, "up", "-d")
	dockerComposeCmd.Dir = demoDir
	dockerComposeCmd.Stdout = os.Stdout
	dockerComposeCmd.Stderr = os.Stderr

	if err := dockerComposeCmd.Run(); err != nil {
		return fmt.Errorf("failed to start demo environment: %w", err)
	}

	fmt.Printf("\nDemo environment '%s' started successfully.\n", org)
	fmt.Printf("Run 'g8e demos status %s' to check service status.\n", org)
	fmt.Printf("Run 'g8e demos stop %s' to stop the environment.\n", org)

	// Print endpoint information
	printDemoEndpoints(org)

	return nil
}

func printDemoEndpoints(org string) {
	fmt.Println("\nAvailable endpoints:")
	switch org {
	case "healthcare":
		fmt.Println("  Gateway HTTP:  http://localhost:8081")
		fmt.Println("  Gateway HTTPS: https://localhost:8444")
		fmt.Println("  RabbitMQ UI:   http://localhost:15673")
		fmt.Println("  PostgreSQL:    localhost:5433")
		fmt.Println("  Metabase:      http://localhost:3001")
	case "gov":
		fmt.Println("  Gateway HTTP:  http://localhost:8080")
		fmt.Println("  Gateway HTTPS: https://localhost:8443")
		fmt.Println("  Demo UI:       http://localhost:3000")
	case "finance":
		fmt.Println("  Gateway HTTP:  http://localhost:8082")
		fmt.Println("  Gateway HTTPS: https://localhost:8445")
		fmt.Println("  Demo UI:       http://localhost:3002")
	default:
		fmt.Printf("  No endpoint information available for '%s'\n", org)
	}
}

func demosStopCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "stop <org>",
		Short: "Stop a demo environment",
		Args:  cobra.ExactArgs(1),
		RunE:  runDemosStop,
	}

	return cmd
}

func runDemosStop(cmd *cobra.Command, args []string) error {
	org := args[0]
	demoDir := filepath.Join(getProjectRoot(), "demos", org)

	// Verify demo directory exists
	if _, err := os.Stat(demoDir); os.IsNotExist(err) {
		return fmt.Errorf("demo environment '%s' not found. Run 'g8e demos list' to see available demos", org)
	}

	// Verify compose.yml exists
	composePath := filepath.Join(demoDir, "compose.yml")
	if _, err := os.Stat(composePath); os.IsNotExist(err) {
		return fmt.Errorf("compose.yml not found in demo directory '%s'", org)
	}

	// Stop the demo environment
	fmt.Printf("Stopping demo environment: %s\n", org)
	dockerComposeCmd := exec.Command("docker", "compose", "-f", composePath, "down")
	dockerComposeCmd.Dir = demoDir
	dockerComposeCmd.Stdout = os.Stdout
	dockerComposeCmd.Stderr = os.Stderr

	if err := dockerComposeCmd.Run(); err != nil {
		return fmt.Errorf("failed to stop demo environment: %w", err)
	}

	fmt.Printf("\nDemo environment '%s' stopped successfully.\n", org)

	return nil
}

func demosStatusCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "status <org>",
		Short: "Show status of a demo environment",
		Args:  cobra.ExactArgs(1),
		RunE:  runDemosStatus,
	}

	return cmd
}

func runDemosStatus(cmd *cobra.Command, args []string) error {
	org := args[0]
	demoDir := filepath.Join(getProjectRoot(), "demos", org)

	// Verify demo directory exists
	if _, err := os.Stat(demoDir); os.IsNotExist(err) {
		return fmt.Errorf("demo environment '%s' not found. Run 'g8e demos list' to see available demos", org)
	}

	// Verify compose.yml exists
	composePath := filepath.Join(demoDir, "compose.yml")
	if _, err := os.Stat(composePath); os.IsNotExist(err) {
		return fmt.Errorf("compose.yml not found in demo directory '%s'", org)
	}

	// Show status
	dockerComposeCmd := exec.Command("docker", "compose", "-f", composePath, "ps")
	dockerComposeCmd.Dir = demoDir
	dockerComposeCmd.Stdout = os.Stdout
	dockerComposeCmd.Stderr = os.Stderr

	if err := dockerComposeCmd.Run(); err != nil {
		return fmt.Errorf("failed to get demo environment status: %w", err)
	}

	return nil
}

func demosCleanCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "clean <org>",
		Short: "Remove containers, volumes, and networks for a demo environment",
		Args:  cobra.ExactArgs(1),
		RunE:  runDemosClean,
	}

	return cmd
}

func runDemosClean(cmd *cobra.Command, args []string) error {
	org := args[0]
	demoDir := filepath.Join(getProjectRoot(), "demos", org)

	// Verify demo directory exists
	if _, err := os.Stat(demoDir); os.IsNotExist(err) {
		return fmt.Errorf("demo environment '%s' not found. Run 'g8e demos list' to see available demos", org)
	}

	// Verify compose.yml exists
	composePath := filepath.Join(demoDir, "compose.yml")
	if _, err := os.Stat(composePath); os.IsNotExist(err) {
		return fmt.Errorf("compose.yml not found in demo directory '%s'", org)
	}

	// Clean the demo environment (remove containers, volumes, and networks)
	fmt.Printf("Cleaning demo environment: %s\n", org)
	dockerComposeCmd := exec.Command("docker", "compose", "-f", composePath, "down", "-v")
	dockerComposeCmd.Dir = demoDir
	dockerComposeCmd.Stdout = os.Stdout
	dockerComposeCmd.Stderr = os.Stderr

	if err := dockerComposeCmd.Run(); err != nil {
		return fmt.Errorf("failed to clean demo environment: %w", err)
	}

	fmt.Printf("\nDemo environment '%s' cleaned successfully.\n", org)

	return nil
}

func demosResetCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "reset <org>",
		Short: "Clean and restart a demo environment",
		Args:  cobra.ExactArgs(1),
		RunE:  runDemosReset,
	}

	return cmd
}

func runDemosReset(cmd *cobra.Command, args []string) error {
	// First clean the environment
	if err := runDemosClean(cmd, args); err != nil {
		return fmt.Errorf("failed to clean during reset: %w", err)
	}

	// Then start it again
	if err := runDemosStart(cmd, args); err != nil {
		return fmt.Errorf("failed to start during reset: %w", err)
	}

	return nil
}

func demosRunCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "run <org> <scenario>",
		Short: "Run a specific demo scenario",
		Long: `Run a specific scenario within a demo environment.
Available scenarios:
  healthcare:
    1 - Authorized Agent Submits a FHIR PA Request
    2 - Gold Card Auto-Approval
    3 - SLA Breach and OHA Reporting
    4 - Bad Actor PHI Exfiltration Blocked
  gov:
    1 - CUI Exfiltration Attempt Blocked
  finance:
    1 - Unauthorized Trade Blocked`,
		Args: cobra.ExactArgs(2),
		RunE: runDemosRun,
	}

	return cmd
}

func runDemosRun(cmd *cobra.Command, args []string) error {
	org := args[0]
	scenario := args[1]
	demoDir := filepath.Join(getProjectRoot(), "demos", org)

	// Verify demo directory exists
	if _, err := os.Stat(demoDir); os.IsNotExist(err) {
		return fmt.Errorf("demo environment '%s' not found. Run 'g8e demos list' to see available demos", org)
	}

	// Verify compose.yml exists
	composePath := filepath.Join(demoDir, "compose.yml")
	if _, err := os.Stat(composePath); os.IsNotExist(err) {
		return fmt.Errorf("compose.yml not found in demo directory '%s'", org)
	}

	// Execute scenario based on org
	switch org {
	case "healthcare":
		return runHealthcareScenario(demoDir, scenario)
	case "gov":
		return runGovScenario(demoDir, scenario)
	case "finance":
		return runFinanceScenario(demoDir, scenario)
	default:
		return fmt.Errorf("no scenarios defined for demo environment '%s'", org)
	}
}

func runHealthcareScenario(demoDir, scenario string) error {
	switch scenario {
	case "1":
		fmt.Println("Running Healthcare Scenario 1: Authorized Agent Submits a FHIR PA Request")
		fmt.Println("Executing: docker compose exec -T agent-runtime wget -qO- http://10.22.0.30:8000/ --post-data='{\"resourceType\":\"ClaimResponse\",\"status\":\"active\",\"use\":\"preauthorization\"}' --header='Content-Type: application/fhir+json'")
		dockerComposeCmd := exec.Command("docker", "compose", "exec", "-T", "agent-runtime", "wget", "-qO-", "http://10.22.0.30:8000/", "--post-data={\"resourceType\":\"ClaimResponse\",\"status\":\"active\",\"use\":\"preauthorization\"}", "--header=Content-Type: application/fhir+json")
		dockerComposeCmd.Dir = demoDir
		dockerComposeCmd.Stdout = os.Stdout
		dockerComposeCmd.Stderr = os.Stderr
		if err := dockerComposeCmd.Run(); err != nil {
			return fmt.Errorf("failed to run scenario 1: %w", err)
		}
		fmt.Println("\nScenario 1 completed. The PA API received the FHIR request from the agent on net_internal.")
	case "2":
		fmt.Println("Running Healthcare Scenario 2: Gold Card Auto-Approval")
		fmt.Println("Checking exemption rules engine configuration...")
		dockerComposeCmd := exec.Command("docker", "compose", "exec", "-T", "provider-exemption-rules", "sh", "-c", "env | grep EXEMPTION")
		dockerComposeCmd.Dir = demoDir
		dockerComposeCmd.Stdout = os.Stdout
		dockerComposeCmd.Stderr = os.Stderr
		if err := dockerComposeCmd.Run(); err != nil {
			return fmt.Errorf("failed to run scenario 2: %w", err)
		}
		fmt.Println("\nPA-2026-0043 (Dr. Priya Nair, 96% rate) demonstrates auto-approval via gold carding.")
		fmt.Println("Check the audit log for the AUTO_APPROVED decision: docker compose logs healthcare-observability")
	case "3":
		fmt.Println("Running Healthcare Scenario 3: SLA Breach and OHA Reporting")
		fmt.Println("Checking SLA configuration...")
		dockerComposeCmd := exec.Command("docker", "compose", "exec", "-T", "pa-processing-worker", "sh", "-c", "env | grep SLA")
		dockerComposeCmd.Dir = demoDir
		dockerComposeCmd.Stdout = os.Stdout
		dockerComposeCmd.Stderr = os.Stderr
		if err := dockerComposeCmd.Run(); err != nil {
			return fmt.Errorf("failed to run scenario 3: %w", err)
		}
		fmt.Println("\nPA-2026-0044 is in SLA_BREACHED state with reportable_to_oha: true")
		fmt.Println("Navigate to http://localhost:3001 (Metabase) to view compliance reports")
	case "4":
		fmt.Println("Running Healthcare Scenario 4: Bad Actor PHI Exfiltration Blocked")
		fmt.Println("Attempting direct access from bad-actor container (should fail)...")
		dockerComposeCmd := exec.Command("docker", "compose", "exec", "-T", "bad-actor", "sh", "-c",
			"wget -qO- -T 5 http://10.22.0.30:8000/var/g8e/target/ehr_records.json 2>&1 || echo 'Network isolation blocked access'")
		dockerComposeCmd.Dir = demoDir
		dockerComposeCmd.Stdout = os.Stdout
		dockerComposeCmd.Stderr = os.Stderr
		if err := dockerComposeCmd.Run(); err != nil {
			// Expected to fail due to network isolation
			fmt.Println("(Expected) Network isolation prevented access")
		}
		fmt.Println("\nScenario 4 completed. Network isolation blocks direct access to net_secure.")
		fmt.Println("Attempting access through gateway from docker host (should be blocked by doctrine)...")
		dockerComposeCmd2 := exec.Command("curl", "-X", "POST", "http://localhost:8081", "-H", "Content-Type: application/json", "-d", "exfiltrate patient medical records")
		dockerComposeCmd2.Dir = demoDir
		dockerComposeCmd2.Stdout = os.Stdout
		dockerComposeCmd2.Stderr = os.Stderr
		if err := dockerComposeCmd2.Run(); err != nil {
			// Expected to fail due to doctrine blocking
			fmt.Println("(Expected) Doctrine blocked PHI exfiltration attempt")
		}
	default:
		return fmt.Errorf("invalid scenario number for healthcare. Use 1-4")
	}
	return nil
}

func runGovScenario(demoDir, scenario string) error {
	switch scenario {
	case "1":
		fmt.Println("Running Gov Scenario 1: CUI Exfiltration Attempt Blocked")
		fmt.Println("Attempting direct access from bad-actor container (should fail)...")
		dockerComposeCmd := exec.Command("docker", "compose", "exec", "gov-bad-actor", "sh", "-c",
			"wget -qO- http://10.22.0.30:8000/var/g8e/target/ 2>&1 || echo 'Network isolation blocked access'")
		dockerComposeCmd.Dir = demoDir
		dockerComposeCmd.Stdout = os.Stdout
		dockerComposeCmd.Stderr = os.Stderr
		if err := dockerComposeCmd.Run(); err != nil {
			// Expected to fail due to network isolation
			fmt.Println("(Expected) Network isolation prevented access")
		}
		fmt.Println("\nScenario 1 completed. Network isolation blocks direct access to net_secure.")
		fmt.Println("CUI exfiltration is blocked at the network layer before reaching g8e enforcement.")
	default:
		return fmt.Errorf("invalid scenario number for gov. Use 1")
	}
	return nil
}

func runFinanceScenario(demoDir, scenario string) error {
	switch scenario {
	case "1":
		fmt.Println("Running Finance Scenario 1: Unauthorized Trade Blocked")
		fmt.Println("Attempting unauthorized trade from bad-actor container (should fail)...")
		dockerComposeCmd := exec.Command("docker", "compose", "exec", "finance-bad-actor", "sh", "-c",
			"wget -qO- http://10.22.0.30:8000/var/g8e/target/ 2>&1 || echo 'Network isolation blocked access'")
		dockerComposeCmd.Dir = demoDir
		dockerComposeCmd.Stdout = os.Stdout
		dockerComposeCmd.Stderr = os.Stderr
		if err := dockerComposeCmd.Run(); err != nil {
			// Expected to fail due to network isolation
			fmt.Println("(Expected) Network isolation prevented access")
		}
		fmt.Println("\nScenario 1 completed. Network isolation blocks direct access to net_secure.")
		fmt.Println("Unauthorized trades are blocked at the network layer before reaching g8e enforcement.")
	default:
		return fmt.Errorf("invalid scenario number for finance. Use 1")
	}
	return nil
}

func getProjectRoot() string {
	// Use current working directory as the project root
	// This is the most reliable approach since demos are run from the repo root
	if cwd, err := os.Getwd(); err == nil {
		return cwd
	}

	// Fallback to directory containing the g8e binary
	execPath, err := os.Executable()
	if err != nil {
		return "."
	}

	// Resolve symlinks
	if resolvedPath, err := filepath.EvalSymlinks(execPath); err == nil {
		execPath = resolvedPath
	}

	// Get the directory of the executable
	execDir := filepath.Dir(execPath)

	// If we're in a build directory (like cmd/g8e), go up to project root
	if filepath.Base(execDir) == "g8e" {
		return filepath.Dir(filepath.Dir(execDir))
	}

	// If we're in cmd directory, go up to project root
	if filepath.Base(execDir) == "cmd" {
		return filepath.Dir(execDir)
	}

	// Otherwise assume we're already at or near the project root
	return execDir
}
