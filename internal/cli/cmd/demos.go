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
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/spf13/cobra"
	"golang.org/x/text/cases"
	"golang.org/x/text/language"
)

// DoctrineRule represents a single doctrine rule from the JSON file
type DoctrineRule struct {
	ID          string  `json:"id"`
	Name        string  `json:"name"`
	Description string  `json:"description"`
	Category    string  `json:"category"`
	Severity    string  `json:"severity"`
	Pattern     string  `json:"pattern"`
	MitreAttack string  `json:"mitre_attack"`
	MitreTactic string  `json:"mitre_tactic"`
	Confidence  float64 `json:"confidence"`
	Enabled     bool    `json:"enabled"`
}

// DoctrineFile represents the structure of a doctrine JSON file
type DoctrineFile struct {
	Source      string         `json:"source"`
	Version     string         `json:"version"`
	LastUpdated string         `json:"last_updated"`
	License     string         `json:"license"`
	Doctrines   []DoctrineRule `json:"doctrines"`
}

// readDoctrineRule reads a doctrine file and returns a specific rule by ID
func readDoctrineRule(demoDir, doctrineFile, ruleID string) (*DoctrineRule, error) {
	doctrinePath := filepath.Join(demoDir, "doctrine", doctrineFile)
	data, err := os.ReadFile(doctrinePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read doctrine file: %w", err)
	}

	var docFile DoctrineFile
	if err := json.Unmarshal(data, &docFile); err != nil {
		return nil, fmt.Errorf("failed to parse doctrine JSON: %w", err)
	}

	for _, rule := range docFile.Doctrines {
		if rule.ID == ruleID {
			return &rule, nil
		}
	}

	return nil, fmt.Errorf("doctrine rule %q not found", ruleID)
}


// toDockerPath converts a filepath to a Docker-compatible path format.
// On Windows, Docker expects forward slashes even though the OS uses backslashes.
func toDockerPath(path string) string {
	if runtime.GOOS == "windows" {
		return filepath.ToSlash(path)
	}
	return path
}

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
		demosAuditCmd(),
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
	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("demos: failed to get working directory: %w", err)
	}
	demosDir := filepath.Join(cwd, constants.DemosDirname)
	entries, err := os.ReadDir(demosDir)
	if err != nil {
		return fmt.Errorf("failed to read demos directory: %w", err)
	}

	fmt.Println("Available demo environments:")
	for _, entry := range entries {
		if entry.IsDir() && entry.Name() != constants.DemosBinDirname {
			composePath := filepath.Join(demosDir, entry.Name(), constants.DemosComposeFile)
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
	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("demos: failed to get working directory: %w", err)
	}
	demoDir := filepath.Join(cwd, constants.DemosDirname, org)

	// Verify demo directory exists
	if _, err := os.Stat(demoDir); os.IsNotExist(err) {
		return fmt.Errorf("demo environment '%s' not found. Run 'g8e demos list' to see available demos", org)
	}

	// Verify compose.yml exists
	composePath := filepath.Join(demoDir, constants.DemosComposeFile)
	if _, err := os.Stat(composePath); os.IsNotExist(err) {
		return fmt.Errorf("compose.yml not found in demo directory '%s'", org)
	}

	// Check if g8e binary exists in demos/bin
	binPath := filepath.Join(cwd, constants.DemosDirname, constants.DemosBinDirname, constants.DemosBinaryName)
	if runtime.GOOS == "windows" {
		binPath += ".exe"
	}
	if _, err := os.Stat(binPath); os.IsNotExist(err) {
		fmt.Printf("Warning: g8e binary not found at %s\n", binPath)
		fmt.Printf("Run 'make build && cp g8e %s/%s/%s' from the repository root to build it.\n", constants.DemosDirname, constants.DemosBinDirname, constants.DemosBinaryName)
	}

	// Start the demo environment
	fmt.Printf("Starting demo environment: %s\n", org)
	dockerPath := toDockerPath(composePath)
	fmt.Printf("Debug: composePath=%s, dockerPath=%s\n", composePath, dockerPath)
	dockerComposeCmd := exec.Command("docker", "compose", "-f", dockerPath, "up", "-d")
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
	case "secure-data":
		fmt.Println("  Gateway HTTP:  http://localhost:8083")
		fmt.Println("  Gateway HTTPS: https://localhost:8446")
		fmt.Println("  Demo UI:       http://localhost:3003")
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
	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("demos: failed to get working directory: %w", err)
	}
	demoDir := filepath.Join(cwd, constants.DemosDirname, org)

	// Verify demo directory exists
	if _, err := os.Stat(demoDir); os.IsNotExist(err) {
		return fmt.Errorf("demo environment '%s' not found. Run 'g8e demos list' to see available demos", org)
	}

	// Verify compose.yml exists
	composePath := filepath.Join(demoDir, constants.DemosComposeFile)
	if _, err := os.Stat(composePath); os.IsNotExist(err) {
		return fmt.Errorf("compose.yml not found in demo directory '%s'", org)
	}

	// Stop the demo environment
	fmt.Printf("Stopping demo environment: %s\n", org)
	dockerComposeCmd := exec.Command("docker", "compose", "-f", toDockerPath(composePath), "down")
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
	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("demos: failed to get working directory: %w", err)
	}
	demoDir := filepath.Join(cwd, constants.DemosDirname, org)

	// Verify demo directory exists
	if _, err := os.Stat(demoDir); os.IsNotExist(err) {
		return fmt.Errorf("demo environment '%s' not found. Run 'g8e demos list' to see available demos", org)
	}

	// Verify compose.yml exists
	composePath := filepath.Join(demoDir, constants.DemosComposeFile)
	if _, err := os.Stat(composePath); os.IsNotExist(err) {
		return fmt.Errorf("compose.yml not found in demo directory '%s'", org)
	}

	// Show status
	dockerComposeCmd := exec.Command("docker", "compose", "-f", toDockerPath(composePath), "ps")
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
	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("demos: failed to get working directory: %w", err)
	}
	demoDir := filepath.Join(cwd, constants.DemosDirname, org)

	// Verify demo directory exists
	if _, err := os.Stat(demoDir); os.IsNotExist(err) {
		return fmt.Errorf("demo environment '%s' not found. Run 'g8e demos list' to see available demos", org)
	}

	// Verify compose.yml exists
	composePath := filepath.Join(demoDir, constants.DemosComposeFile)
	if _, err := os.Stat(composePath); os.IsNotExist(err) {
		return fmt.Errorf("compose.yml not found in demo directory '%s'", org)
	}

	// Clean the demo environment (remove containers, volumes, and networks)
	fmt.Printf("Cleaning demo environment: %s\n", org)
	dockerComposeCmd := exec.Command("docker", "compose", "-f", toDockerPath(composePath), "down", "-v")
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

// scenarioCounts maps each org to the number of defined scenarios.
var scenarioCounts = map[string]int{
	"healthcare":  4,
	"gov":         1,
	"finance":     1,
	"secure-data": 3,
}

func demosRunCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "run <org> [scenario]",
		Short: "Run demo scenarios",
		Long: `Run one or all scenarios for a demo environment.
Omit the scenario number to run all scenarios in sequence.

Available scenarios:
  healthcare: 1-4
    1 - Authorized Agent Submits a FHIR PA Request
    2 - Gold Card Auto-Approval
    3 - SLA Breach and OHA Reporting
    4 - Bad Actor PHI Exfiltration Blocked
  gov: 1
    1 - CUI Exfiltration Attempt Blocked
  finance: 1
    1 - Unauthorized Trade Blocked
  secure-data: 1-3
    1 - Governed Migration with Chain-of-Custody Receipts
    2 - Connector Bypass Attempt Blocked
    3 - Cross-Tenant Leak Doctrine Triggered`,
		Args: cobra.RangeArgs(1, 2),
		RunE: runDemosRun,
	}

	return cmd
}

func runDemosRun(cmd *cobra.Command, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("demos: requires demo environment name")
	}
	if len(args) > 2 {
		return fmt.Errorf("demos: accepts at most 2 arguments (demo environment and optional scenario name)")
	}

	org := args[0]
	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("demos: failed to get working directory: %w", err)
	}
	demoDir := filepath.Join(cwd, constants.DemosDirname, org)

	if _, err := os.Stat(demoDir); os.IsNotExist(err) {
		return fmt.Errorf("demo environment '%s' not found. Run 'g8e demos list' to see available demos", org)
	}

	composePath := filepath.Join(demoDir, constants.DemosComposeFile)
	if _, err := os.Stat(composePath); os.IsNotExist(err) {
		return fmt.Errorf("compose.yml not found in demo directory '%s'", org)
	}

	// Check if demo is running, start if not
	if !isDemoRunning(demoDir, composePath) {
		fmt.Printf("Demo environment '%s' is not running. Starting it now...\n", org)
		if err := runDemosStart(cmd, args); err != nil {
			return fmt.Errorf("failed to start demo environment: %w", err)
		}
	}

	if len(args) >= 2 {
		return runScenario(org, demoDir, args[1]) //nolint:gosec // length checked above
	}

	return runAllScenarios(org, demoDir)
}

func isDemoRunning(demoDir, composePath string) bool {
	dockerComposeCmd := exec.Command("docker", "compose", "-f", toDockerPath(composePath), "ps", "-q")
	dockerComposeCmd.Dir = demoDir
	output, err := dockerComposeCmd.Output()
	if err != nil {
		return false
	}
	return len(strings.TrimSpace(string(output))) > 0
}

func runAllScenarios(org, demoDir string) error {
	count, ok := scenarioCounts[org]
	if !ok {
		return fmt.Errorf("no scenarios defined for demo environment '%s'", org)
	}

	fmt.Printf("\n%s\n  Running all %s demo scenarios\n%s\n",
		strings.Repeat("═", 60), org, strings.Repeat("═", 60))

	results := make([]scenarioResult, 0, count)

	for i := 1; i <= count; i++ {
		scenarioNum := fmt.Sprintf("%d", i)
		result, err := runScenarioWithResult(org, demoDir, scenarioNum)
		if err != nil {
			return err
		}
		results = append(results, result)
	}

	printResultsTable(org, results)

	fmt.Printf("\n%s\n  All %s scenarios passed.\n%s\n",
		strings.Repeat("═", 60), org, strings.Repeat("═", 60))
	return nil
}

type scenarioResult struct {
	number  string
	name    string
	status  string
	metrics string
}

func runScenario(org, demoDir, scenario string) error {
	_, err := runScenarioWithResult(org, demoDir, scenario)
	return err
}

func runScenarioWithResult(org, demoDir, scenario string) (scenarioResult, error) {
	switch org {
	case "healthcare":
		return runHealthcareScenarioWithResult(demoDir, scenario)
	case "gov":
		return runGovScenarioWithResult(demoDir, scenario)
	case "finance":
		return runFinanceScenarioWithResult(demoDir, scenario)
	case "secure-data":
		return runSecureDataScenarioWithResult(demoDir, scenario)
	default:
		return scenarioResult{}, fmt.Errorf("no scenarios defined for demo environment '%s'", org)
	}
}

func printResultsTable(org string, results []scenarioResult) {
	fmt.Printf("\n%s\n  %s Scenario Results Summary\n%s\n",
		strings.Repeat("═", 60), cases.Title(language.English).String(org), strings.Repeat("═", 60))
	fmt.Println()

	// Print header
	fmt.Printf("%-10s\t%-50s\t%-12s\t%s\n",
		"Scenario", "Name", "Status", "Key Metrics")
	fmt.Println(strings.Repeat("─", 120))

	// Print rows
	for _, r := range results {
		fmt.Printf("%-10s\t%-50s\t%-12s\t%s\n",
			r.number, r.name, r.status, r.metrics)
	}
	fmt.Println()
}

// demoStep prints a labeled command and runs it, streaming output inline.
// Always returns error if command fails, but only stops execution if fatal is true.
func demoStep(demoDir, label string, fatal bool, args ...string) error {
	fmt.Printf("  $ %s\n", strings.Join(args, " "))
	c := exec.Command(args[0], args[1:]...)
	c.Dir = demoDir
	c.Stdout = os.Stdout
	c.Stderr = os.Stderr
	err := c.Run()
	fmt.Println()
	if err != nil {
		if fatal {
			return fmt.Errorf("%s: %w", label, err)
		}
		// Return error but don't stop execution
		return fmt.Errorf("%s failed (non-fatal): %w", label, err)
	}
	return nil
}


func demosAuditCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "audit <org> [action]",
		Short: "View audit logs and ledger history for a demo environment",
		Long: `View audit logs and ledger history for a demo environment.
Without an action, it prints a summary of available audit resources.

Actions:
  logs              Tail the observability logs
  gateway-db        Open the gateway audit database (SQLite)
  operator-db       Open the operator audit database (SQLite)
  ledger-log        View the git ledger log
  ledger-files      List all files in the git ledger
  ledger-history <file> View git history for a specific file
  ledger-show <hash> View a specific git commit diff
  vault             Open the execution vault database (SQLite)`,
		Args: cobra.MinimumNArgs(1),
		RunE: runDemosAudit,
	}

	return cmd
}

func runDemosAudit(cmd *cobra.Command, args []string) error {
	org := args[0]
	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("demos: failed to get working directory: %w", err)
	}
	demoDir := filepath.Join(cwd, constants.DemosDirname, org)

	// Verify demo directory exists
	if _, err := os.Stat(demoDir); os.IsNotExist(err) {
		return fmt.Errorf("demo environment '%s' not found. Run 'g8e demos list' to see available demos", org)
	}

	// Verify compose.yml exists
	composePath := filepath.Join(demoDir, constants.DemosComposeFile)
	if _, err := os.Stat(composePath); os.IsNotExist(err) {
		return fmt.Errorf("compose.yml not found in demo directory '%s'", org)
	}

	// Check if demo is running
	if !isDemoRunning(demoDir, composePath) {
		return fmt.Errorf("demo environment '%s' is not running. Run 'g8e demos start %s' first", org, org)
	}

	// Determine service names based on org
	var gatewayService, operatorService string
	switch org {
	case "healthcare", "gov", "finance":
		gatewayService = "gateway"
		operatorService = "operator"
	default:
		gatewayService = "gateway"
		operatorService = "operator"
	}

	if len(args) == 1 {
		fmt.Printf("Audit logs and ledger history for: %s\n", org)
		fmt.Println(strings.Repeat("─", 60))
		fmt.Println("Run 'g8e demos audit <org> <action>' to execute these directly.")
		fmt.Println()

		// View operator log (real-time audit stream)
		fmt.Println("1. Real-time audit log stream (operator.log):")
		fmt.Printf("   Action: logs\n")
		fmt.Printf("   Command: docker compose -f %s logs -f observability\n", composePath)
		fmt.Println()

		// View audit database
		fmt.Println("2. Audit vault database (SQLite):")
		fmt.Printf("   Action: gateway-db\n")
		fmt.Printf("   Command: docker compose -f %s exec %s sqlite3 /root/.g8e/data/audit_vault.db\n", composePath, gatewayService)
		fmt.Println()
		fmt.Printf("   Action: operator-db\n")
		fmt.Printf("   Command: docker compose -f %s exec %s sqlite3 /root/.g8e/data/audit_vault.db\n", composePath, operatorService)
		fmt.Println()
		fmt.Println("   Useful SQL queries:")
		fmt.Println("   SELECT * FROM sessions;")
		fmt.Println("   SELECT * FROM events ORDER BY id DESC LIMIT 20;")
		fmt.Println("   SELECT * FROM file_mutation_log ORDER BY id DESC LIMIT 20;")
		fmt.Println("   SELECT * FROM receipts ORDER BY id DESC LIMIT 20;")
		fmt.Println()

		// View git ledger
		fmt.Println("3. Git ledger history:")
		fmt.Printf("   Action: ledger-log\n")
		fmt.Printf("   Command: docker compose -f %s exec %s sh -c 'cd /root/.g8e/ledger/files && git log --oneline'\n", composePath, gatewayService)
		fmt.Println()
		fmt.Printf("   Action: ledger-files\n")
		fmt.Printf("   Command: docker compose -f %s exec %s sh -c 'cd /root/.g8e/ledger/files && git ls-files'\n", composePath, gatewayService)
		fmt.Println()
		fmt.Printf("   Action: ledger-history <file>\n")
		fmt.Printf("   Command: docker compose -f %s exec %s sh -c 'cd /root/.g8e/ledger/files && git log --follow -- path/to/file'\n", composePath, gatewayService)
		fmt.Println()
		fmt.Printf("   Action: ledger-show <hash>\n")
		fmt.Printf("   Command: docker compose -f %s exec %s sh -c 'cd /root/.g8e/ledger/files && git show <commit-hash>'\n", composePath, gatewayService)
		fmt.Println()

		// View execution vault
		fmt.Println("4. Execution vault (command results and file diffs):")
		fmt.Printf("   Action: vault\n")
		fmt.Printf("   Command: docker compose -f %s exec %s sqlite3 /root/.g8e/execution_vault.db\n", composePath, gatewayService)
		fmt.Println()
		fmt.Println("   Useful SQL queries:")
		fmt.Println("   SELECT * FROM execution_log ORDER BY timestamp_utc DESC LIMIT 20;")
		fmt.Println("   SELECT * FROM file_diff_log ORDER BY timestamp_utc DESC LIMIT 20;")
		fmt.Println()
		return nil
	}

	action := args[1]
	switch action {
	case "logs":
		return runDockerComposeLogs(demoDir, composePath, "observability")
	case "gateway-db":
		return runDockerComposeExec(demoDir, composePath, gatewayService, "sqlite3", "/root/.g8e/data/audit_vault.db")
	case "operator-db":
		return runDockerComposeExec(demoDir, composePath, operatorService, "sqlite3", "/root/.g8e/data/audit_vault.db")
	case "ledger-log":
		return runDockerComposeExec(demoDir, composePath, gatewayService, "sh", "-c", "cd /root/.g8e/ledger/files && git log --oneline")
	case "ledger-files":
		return runDockerComposeExec(demoDir, composePath, gatewayService, "sh", "-c", "cd /root/.g8e/ledger/files && git ls-files")
	case "ledger-history":
		if len(args) < 3 {
			return fmt.Errorf("ledger-history requires a file path")
		}
		return runDockerComposeExec(demoDir, composePath, gatewayService, "sh", "-c", "cd /root/.g8e/ledger/files && git log --follow -- \"$1\"", "--", args[2])
	case "ledger-show":
		if len(args) < 3 {
			return fmt.Errorf("ledger-show requires a commit hash")
		}
		return runDockerComposeExec(demoDir, composePath, gatewayService, "sh", "-c", "cd /root/.g8e/ledger/files && git show \"$1\"", "--", args[2])
	case "vault":
		return runDockerComposeExec(demoDir, composePath, gatewayService, "sqlite3", "/root/.g8e/execution_vault.db")
	default:
		return fmt.Errorf("unknown audit action: %s", action)
	}
}

func runDockerComposeExec(demoDir, composePath, service string, args ...string) error {
	fullArgs := []string{"compose", "-f", toDockerPath(composePath), "exec", service}
	fullArgs = append(fullArgs, args...)

	cmd := exec.Command("docker", fullArgs...)
	cmd.Dir = demoDir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin

	return cmd.Run()
}

func runDockerComposeLogs(demoDir, composePath, service string) error {
	cmd := exec.Command("docker", "compose", "-f", toDockerPath(composePath), "logs", "-f", service)
	cmd.Dir = demoDir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	return cmd.Run()
}
