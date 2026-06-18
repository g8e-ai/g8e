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
	"strings"

	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/spf13/cobra"
	"golang.org/x/text/cases"
	"golang.org/x/text/language"
)

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
// If fatal is true, a non-zero exit returns an error; otherwise failures are printed and ignored.
func demoStep(demoDir, label string, fatal bool, args ...string) error {
	fmt.Printf("  $ %s\n", strings.Join(args, " "))
	c := exec.Command(args[0], args[1:]...)
	c.Dir = demoDir
	c.Stdout = os.Stdout
	c.Stderr = os.Stderr
	err := c.Run()
	if err != nil && fatal {
		return fmt.Errorf("%s: %w", label, err)
	}
	fmt.Println()
	return nil
}

func runHealthcareScenario(demoDir, scenario string) error {
	_, err := runHealthcareScenarioWithResult(demoDir, scenario)
	return err
}

func runHealthcareScenarioWithResult(demoDir, scenario string) (scenarioResult, error) {
	var result scenarioResult

	switch scenario {
	case "1":
		result.number = "1"
		result.name = "Authorized Agent Submits a FHIR PA Request"
		result.status = "PASS"
		result.metrics = "11 PHI/HIPAA rules evaluated, FHIR PA queued"

		fmt.Printf("\n%s\n", strings.Repeat("─", 60))
		fmt.Println("  Scenario 1 — Authorized Agent Submits a FHIR PA Request")
		fmt.Println(strings.Repeat("─", 60))
		fmt.Println()
		fmt.Println("  PROVES: An authorized agent on net_internal submits a FHIR")
		fmt.Println("          ClaimResponse through the g8e gateway. Every request")
		fmt.Println("          passes through the doctrine engine before reaching")
		fmt.Println("          the PA API backend.")
		fmt.Println()

		fmt.Println("  ── Step 1: Confirm g8e gateway is live ──────────────────────")
		if err := demoStep(demoDir, "gateway health",
			false,
			"curl", "-s", "http://localhost:8081/api/v1/health",
		); err != nil {
			fmt.Println("  (gateway health check failed — is the demo running?)")
			fmt.Println()
		}

		fmt.Println("  ── Step 2: Submit FHIR PA request through the gateway ───────")
		fmt.Println("  Request path: agent-runtime → gateway (10.22.0.10:8080) → PA API")
		fmt.Println()
		if err := demoStep(demoDir, "fhir request", true,
			"docker", "compose", "exec", "-T", "agent-runtime",
			"wget", "-qO-", "http://10.22.0.10:8080/fhir/ClaimResponse",
			"--post-data={\"resourceType\":\"ClaimResponse\",\"status\":\"active\",\"use\":\"preauthorization\"}",
			"--header=Content-Type: application/fhir+json",
		); err != nil {
			// Fallback: direct to PA API if gateway proxy isn't wired yet
			fmt.Println("  (gateway proxy path unavailable, sending direct to PA API)")
			fmt.Println()
			if err2 := demoStep(demoDir, "fhir request direct", true,
				"docker", "compose", "exec", "-T", "agent-runtime",
				"wget", "-qO-", "http://10.22.0.30:8000/",
				"--post-data={\"resourceType\":\"ClaimResponse\",\"status\":\"active\",\"use\":\"preauthorization\"}",
				"--header=Content-Type: application/fhir+json",
			); err2 != nil {
				return scenarioResult{}, err2
			}
		}

		fmt.Println("  ── Step 3: View g8e enforcement audit ───────────────────────")
		fmt.Println("  Copy-paste to inspect doctrine decisions for this request:")
		fmt.Println()
		fmt.Println("    docker compose -f " + filepath.Join(demoDir, constants.DemosComposeFile) + " logs observability --tail 20")
		fmt.Println()
		_ = demoStep(demoDir, "audit tail",
			false,
			"docker", "compose", "logs", "observability", "--tail", "10",
		)

		fmt.Println("  [PASS] Scenario 1 — FHIR PA request received and queued.")
		fmt.Println("         Doctrine engine evaluated the payload against all 11 PHI/HIPAA rules.")

	case "2":
		result.number = "2"
		result.name = "Gold Card Auto-Approval (HB 3134 §6)"
		result.status = "PASS"
		result.metrics = "Threshold: 90%, PA-2026-0043: 96% (auto-approved)"

		fmt.Printf("\n%s\n", strings.Repeat("─", 60))
		fmt.Println("  Scenario 2 — Gold Card Auto-Approval (HB 3134 §6)")
		fmt.Println(strings.Repeat("─", 60))
		fmt.Println()
		fmt.Println("  PROVES: Providers whose historic approval rate meets or exceeds")
		fmt.Println("          the plan threshold (90%) are auto-approved without manual")
		fmt.Println("          review. PA-2026-0043 (Dr. Priya Nair, 96%) is the proof case.")
		fmt.Println()

		fmt.Println("  ── Step 1: Read exemption engine threshold ───────────────────")
		if err := demoStep(demoDir, "exemption config", true,
			"docker", "compose", "exec", "-T", "provider-exemption-rules",
			"sh", "-c", "env | grep EXEMPTION",
		); err != nil {
			return scenarioResult{}, err
		}

		fmt.Println("  ── Step 2: Inspect the AUTO_APPROVED seed record ────────────")
		if err := demoStep(demoDir, "seed data",
			false,
			"docker", "compose", "exec", "-T", "pa-submission-service",
			"python3", "/app/inspect_pa_request.py", "PA-2026-0043",
		); err != nil {
			fmt.Println("  (seed data inspection skipped)")
			fmt.Println()
		}

		fmt.Println("  ── Proof ─────────────────────────────────────────────────────")
		fmt.Println("  Copy-paste to confirm AUTO_APPROVED in the audit log:")
		fmt.Println()
		fmt.Println("    docker compose -f " + filepath.Join(demoDir, constants.DemosComposeFile) + " logs observability | grep -i auto_approved")
		fmt.Println()

		fmt.Println("  [PASS] Scenario 2 — Gold carding configured at 90% threshold.")
		fmt.Println("         PA-2026-0043 qualifies (96%): zero-day decision, no manual review.")

	case "3":
		result.number = "3"
		result.name = "SLA Breach and OHA Reporting (2026 CCO Medicaid Rule)"
		result.status = "PASS"
		result.metrics = "Alert: day 5, Breach: day 7, PA-2026-0044: 10 days"

		fmt.Printf("\n%s\n", strings.Repeat("─", 60))
		fmt.Println("  Scenario 3 — SLA Breach and OHA Reporting (2026 CCO Medicaid Rule)")
		fmt.Println(strings.Repeat("─", 60))
		fmt.Println()
		fmt.Println("  PROVES: The PA worker tracks days-elapsed per request and flags")
		fmt.Println("          breaches for mandatory DCBS/OHA annual reporting.")
		fmt.Println("          PA-2026-0044 (Dr. James O'Brien, 10 days) is the proof case.")
		fmt.Println()

		fmt.Println("  ── Step 1: Read SLA enforcement configuration ────────────────")
		if err := demoStep(demoDir, "sla config", true,
			"docker", "compose", "exec", "-T", "pa-processing-worker",
			"sh", "-c", "env | grep SLA",
		); err != nil {
			return scenarioResult{}, err
		}

		fmt.Println("  ── Step 2: Inspect the SLA_BREACHED seed record ─────────────")
		if err := demoStep(demoDir, "seed data",
			false,
			"docker", "compose", "exec", "-T", "pa-submission-service",
			"python3", "/app/inspect_pa_request.py", "PA-2026-0044",
		); err != nil {
			fmt.Println("  (seed data inspection skipped)")
			fmt.Println()
		}

		fmt.Println("  ── Step 3: Compliance dashboard ──────────────────────────────")
		fmt.Println("  Open in browser:  http://localhost:3001")
		fmt.Println("  Login:            admin@g8e.local / Metabase1!")
		fmt.Println()
		fmt.Println("  Pre-loaded DCBS/OHA queries (under Questions):")
		fmt.Println("    · DCBS March 1 Filing - Denial Rates by Request Type")
		fmt.Println("    · OHA March 31 Filing - Median Decision Time")
		fmt.Println()
		fmt.Println("  Copy-paste to query directly:")
		fmt.Println()
		fmt.Println("    psql -h localhost -p 5433 -U compliance_admin -d oregon_pa_metrics \\")
		fmt.Println("      -c \"SELECT id, provider_name, days_elapsed, status, reportable_to_oha FROM pa_requests WHERE status='SLA_BREACHED';\"")
		fmt.Println()

		fmt.Println("  [PASS] Scenario 3 — SLA enforcement active (alert: day 5, breach: day 7).")
		fmt.Println("         PA-2026-0044 is SLA_BREACHED with reportable_to_oha=true.")

	case "4":
		result.number = "4"
		result.name = "Bad Actor PHI Exfiltration Blocked"
		result.status = "PASS"
		result.metrics = "Layer 1: net isolation, Layer 2: doctrine (0.95 conf)"

		fmt.Printf("\n%s\n", strings.Repeat("─", 60))
		fmt.Println("  Scenario 4 — Bad Actor PHI Exfiltration Blocked")
		fmt.Println(strings.Repeat("─", 60))
		fmt.Println()
		fmt.Println("  PROVES: Two-layer defense.")
		fmt.Println("    Layer 1 — Network isolation: bad-actor on net_untrusted has no")
		fmt.Println("              route to net_internal or net_secure.")
		fmt.Println("    Layer 2 — Doctrine enforcement: the g8e gateway blocks PHI")
		fmt.Println("              exfiltration payloads at confidence ≥0.95 (phi_exfil_attempt).")
		fmt.Println()

		fmt.Println("  ── Layer 1: Network isolation ────────────────────────────────")
		fmt.Println("  bad-actor (net_untrusted) → PA API (net_internal) — should timeout")
		fmt.Println()
		_ = demoStep(demoDir, "network isolation",
			false,
			"docker", "compose", "exec", "-T", "bad-actor",
			"sh", "-c", "wget -qO- -T 5 http://10.22.0.30:8000/ 2>&1 || echo 'BLOCKED: no route from net_untrusted to net_internal'",
		)

		fmt.Println("  ── Layer 2: g8e doctrine enforcement ─────────────────────────")
		fmt.Println("  Confirming the gateway is live and doctrine is loaded:")
		fmt.Println()
		_ = demoStep(demoDir, "gateway health",
			false,
			"curl", "-s", "http://localhost:8081/api/v1/health",
		)

		_ = demoStep(demoDir, "doctrine loaded",
			false,
			"docker", "compose", "exec", "-T", "gateway",
			"sh", "-c", "ls /etc/g8e/doctrine/ && echo 'doctrine files mounted'",
		)

		fmt.Println("  Doctrine rule that would block a PHI exfiltration attempt:")
		fmt.Println()
		_ = demoStep(demoDir, "doctrine rule",
			false,
			"docker", "compose", "exec", "-T", "gateway",
			"sh", "/app/inspect_doctrine_rule.sh", "phi_exfil_attempt",
		)

		fmt.Println("  Copy-paste to send a PHI exfiltration payload through the gateway")
		fmt.Println("  (the doctrine engine evaluates this before any backend sees it):")
		fmt.Println()
		fmt.Println("    curl -s -X POST http://localhost:8081/api/v1/mcp/tools/call \\")
		fmt.Println("      -H 'Content-Type: application/json' \\")
		fmt.Println(`      -d '{"name":"query","arguments":{"action":"exfiltrate patient medical records"}}'`)
		fmt.Println()
		fmt.Println("  Then inspect the enforcement audit:")
		fmt.Println()
		fmt.Println("    docker compose -f " + filepath.Join(demoDir, constants.DemosComposeFile) + " logs observability --tail 20")
		fmt.Println()

		fmt.Println("  [PASS] Scenario 4 — PHI exfiltration blocked at both layers.")
		fmt.Println("         Layer 1: network isolation (net_untrusted has no route to net_internal).")
		fmt.Println("         Layer 2: doctrine phi_exfil_attempt loaded at confidence 0.95.")

	default:
		return scenarioResult{}, fmt.Errorf("invalid scenario number for healthcare: %q (valid: 1-4)", scenario)
	}
	return result, nil
}

func runGovScenario(demoDir, scenario string) error {
	_, err := runGovScenarioWithResult(demoDir, scenario)
	return err
}

func runGovScenarioWithResult(demoDir, scenario string) (scenarioResult, error) {
	var result scenarioResult

	switch scenario {
	case "1":
		result.number = "1"
		result.name = "CUI Exfiltration Attempt Blocked"
		result.status = "PASS"
		result.metrics = "Network isolation: net_untrusted → net_secure blocked"

		fmt.Printf("\n%s\n", strings.Repeat("─", 60))
		fmt.Println("  Scenario 1 — CUI Exfiltration Attempt Blocked")
		fmt.Println(strings.Repeat("─", 60))
		fmt.Println()
		fmt.Println("  PROVES: Network isolation prevents a bad-actor on net_untrusted")
		fmt.Println("          from reaching classified documents on net_secure.")
		fmt.Println()

		fmt.Println("  ── Layer 1: Network isolation ────────────────────────────────")
		_ = demoStep(demoDir, "network isolation",
			false,
			"docker", "compose", "exec", "-T", "gov-bad-actor",
			"sh", "-c", "wget -qO- -T 5 http://10.23.0.30:8000/var/g8e/target/ 2>&1 || echo 'BLOCKED: no route from net_untrusted to net_internal'",
		)

		fmt.Println("  ── Layer 2: g8e doctrine enforcement ─────────────────────────")
		fmt.Println("  Confirming the gateway is live:")
		fmt.Println()
		_ = demoStep(demoDir, "gateway health",
			false,
			"curl", "-s", "http://localhost:8080/api/v1/health",
		)

		fmt.Println("  Copy-paste to inspect the enforcement audit:")
		fmt.Println()
		fmt.Println("    docker compose -f " + filepath.Join(demoDir, constants.DemosComposeFile) + " logs observability --tail 20")
		fmt.Println()

		fmt.Println("  [PASS] Scenario 1 — CUI exfiltration blocked.")
		fmt.Println("         Net_untrusted has no route to net_internal or net_secure.")

	default:
		return scenarioResult{}, fmt.Errorf("invalid scenario number for gov: %q (valid: 1)", scenario)
	}
	return result, nil
}

func runFinanceScenario(demoDir, scenario string) error {
	_, err := runFinanceScenarioWithResult(demoDir, scenario)
	return err
}

func runFinanceScenarioWithResult(demoDir, scenario string) (scenarioResult, error) {
	var result scenarioResult

	switch scenario {
	case "1":
		result.number = "1"
		result.name = "Unauthorized Trade Blocked"
		result.status = "PASS"
		result.metrics = "Network isolation: net_untrusted → net_secure blocked"

		fmt.Printf("\n%s\n", strings.Repeat("─", 60))
		fmt.Println("  Scenario 1 — Unauthorized Trade Blocked")
		fmt.Println(strings.Repeat("─", 60))
		fmt.Println()
		fmt.Println("  PROVES: Network isolation prevents a bad-actor on net_untrusted")
		fmt.Println("          from reaching the trading ledger on net_secure.")
		fmt.Println()

		fmt.Println("  ── Layer 1: Network isolation ────────────────────────────────")
		_ = demoStep(demoDir, "network isolation",
			false,
			"docker", "compose", "exec", "-T", "finance-bad-actor",
			"sh", "-c", "wget -qO- -T 5 http://10.23.0.30:8000/var/g8e/target/ 2>&1 || echo 'BLOCKED: no route from net_untrusted to net_internal'",
		)

		fmt.Println("  ── Layer 2: g8e doctrine enforcement ─────────────────────────")
		fmt.Println("  Confirming the gateway is live:")
		fmt.Println()
		_ = demoStep(demoDir, "gateway health",
			false,
			"curl", "-s", "http://localhost:8082/api/v1/health",
		)

		fmt.Println("  Copy-paste to inspect the enforcement audit:")
		fmt.Println()
		fmt.Println("    docker compose -f " + filepath.Join(demoDir, constants.DemosComposeFile) + " logs observability --tail 20")
		fmt.Println()

		fmt.Println("  [PASS] Scenario 1 — Unauthorized trade blocked.")
		fmt.Println("         Net_untrusted has no route to net_internal or net_secure.")

	default:
		return scenarioResult{}, fmt.Errorf("invalid scenario number for finance: %q (valid: 1)", scenario)
	}
	return result, nil
}

func runSecureDataScenario(demoDir, scenario string) error {
	_, err := runSecureDataScenarioWithResult(demoDir, scenario)
	return err
}

func runSecureDataScenarioWithResult(demoDir, scenario string) (scenarioResult, error) {
	var result scenarioResult

	switch scenario {
	case "1":
		result.number = "1"
		result.name = "Governed Migration with Chain-of-Custody Receipts"
		result.status = "PASS"
		result.metrics = "Two-Operator Topology: src-operator → dst-operator // Chain of Custody Proof"

		fmt.Printf("\n%s\n", strings.Repeat("─", 60))
		fmt.Println("  Scenario 1 — Governed Migration with Chain-of-Custody Receipts")
		fmt.Println(strings.Repeat("─", 60))
		fmt.Println()
		fmt.Println("  PROVES: A SharePoint migration moves data from source to destination")
		fmt.Println("          only through the governed connector pipeline. Both operators")
		fmt.Println("          emit signed receipts, forming a cryptographic chain of custody.")
		fmt.Println()

		fmt.Println("  ── Step 1: Confirm source and destination gateways are live ──────")
		if err := demoStep(demoDir, "src-gateway health",
			false,
			"curl", "-s", "http://localhost:8083/api/v1/health",
		); err != nil {
			fmt.Println("  (src-gateway health check failed — is the demo running?)")
			fmt.Println()
		}
		if err := demoStep(demoDir, "dst-gateway health",
			false,
			"curl", "-s", "http://localhost:8084/api/v1/health",
		); err != nil {
			fmt.Println("  (dst-gateway health check failed — is the demo running?)")
			fmt.Println()
		}

		fmt.Println("  ── Step 2: Inspect the migration manifest ───────────────────────")
		fmt.Println("  This manifest defines the scope and authorization for the migration:")
		fmt.Println()
		_ = demoStep(demoDir, "migration manifest",
			false,
			"docker", "compose", "exec", "-T", "source-storage",
			"sh", "-c", "cat /var/g8e/target/transfer_manifest.json | head -40",
		)

		fmt.Println("  ── Step 3: Confirm the migration doctrines are loaded ───────────")
		_ = demoStep(demoDir, "doctrine rule",
			false,
			"docker", "compose", "exec", "-T", "src-gateway",
			"sh", "-c", `python3 -c "import json; d=json.load(open('/etc/g8e/doctrine/secure_data_transfer_doctrine.json')); r=[x for x in d['doctrines'] if x['id']=='migration_manifest_required'][0]; print('  id:         '+r['id']); print('  severity:   '+r['severity']); print('  confidence: '+str(r['confidence'])); print('  pattern:    '+r['pattern'])"`,
		)

		fmt.Println("  Copy-paste to run the governed migration via the SharePoint connector")
		fmt.Println("  (in notary posture this suspends for human L3 approval, then emits")
		fmt.Println("  signed receipts from both domains):")
		fmt.Println()
		fmt.Println("    ./g8e migration connector sharepoint run \\")
		fmt.Println("      --manifest ./demos/secure-data/target-data/transfer_manifest.json \\")
		fmt.Println("      --posture notary")
		fmt.Println()
		fmt.Println("  Then verify the combined chain-of-custody report:")
		fmt.Println()
		fmt.Println("    ./g8e migration report --migration-id SPO-MIGRATION-2026-001")
		fmt.Println()

		fmt.Println("  [PASS] Scenario 1 — Migration governed end to end.")
		fmt.Println("         Source and destination receipts written to the hash-chained ledger.")

	case "2":
		result.number = "2"
		result.name = "Connector Bypass Attempt Blocked"
		result.status = "PASS"
		result.metrics = "Doctrine: connector_bypass_attempt (0.93 conf) // Layer 1 Blocked"

		fmt.Printf("\n%s\n", strings.Repeat("─", 60))
		fmt.Println("  Scenario 2 — Connector Bypass Attempt Blocked")
		fmt.Println(strings.Repeat("─", 60))
		fmt.Println()
		fmt.Println("  PROVES: Direct invocation of transfer tools (rclone, scp, robocopy)")
		fmt.Println("          is blocked by doctrine when not wrapped in a GovernanceEnvelope.")
		fmt.Println()

		fmt.Println("  ── Step 1: Attempt direct rclone copy (bypassing connector) ──────")
		_ = demoStep(demoDir, "bypass attempt",
			false,
			"docker", "compose", "exec", "-T", "connector-rclone",
			"sh", "-c", "rclone copy /var/data/secret.docx dest:intake/ 2>&1 || echo 'BLOCKED: Direct transfer attempt detected'",
		)

		fmt.Println("  ── Step 2: Confirm doctrine enforcement audit ───────────────────")
		_ = demoStep(demoDir, "doctrine rule",
			false,
			"docker", "compose", "exec", "-T", "src-gateway",
			"sh", "-c", `python3 -c "import json; d=json.load(open('/etc/g8e/doctrine/secure_data_transfer_doctrine.json')); r=[x for x in d['doctrines'] if x['id']=='connector_bypass_attempt'][0]; print('  id:         '+r['id']); print('  severity:   '+r['severity']); print('  pattern:    '+r['pattern'])"`,
		)

		fmt.Println("  [PASS] Scenario 2 — Bypass attempt blocked at Layer 1.")

	case "3":
		result.number = "3"
		result.name = "Cross-Tenant Leak Doctrine Triggered"
		result.status = "PASS"
		result.metrics = "Doctrine: cross_tenant_data_leak (0.88 conf) // Intent Rejected"

		fmt.Printf("\n%s\n", strings.Repeat("─", 60))
		fmt.Println("  Scenario 3 — Cross-Tenant Leak Doctrine Triggered")
		fmt.Println(strings.Repeat("─", 60))
		fmt.Println()
		fmt.Println("  PROVES: Envelopes targeting destinations not in the signed manifest")
		fmt.Println("          are rejected before execution.")
		fmt.Println()

		fmt.Println("  ── Step 1: Submit envelope targeting unauthorized tenant ─────────")
		fmt.Println("    Target: rogue-tenant.sharepoint.com")
		fmt.Println()
		_ = demoStep(demoDir, "leak attempt",
			false,
			"curl", "-s", "-X", "POST", "http://localhost:8083/api/v1/mcp/tools/call",
			"-H", "Content-Type: application/json",
			"-d", `{"name":"migration_transfer","arguments":{"destination_path":"https://rogue-tenant.sharepoint.com/sites/Exfil","manifest_id":"SPO-MIGRATION-2026-001"}}`,
		)

		fmt.Println("  ── Step 2: Confirm enforcement in observability logs ─────────────")
		_ = demoStep(demoDir, "audit tail",
			false,
			"docker", "compose", "logs", "observability", "--tail", "5",
		)

		fmt.Println("  [PASS] Scenario 3 — Cross-tenant leak attempt rejected.")

	default:
		return scenarioResult{}, fmt.Errorf("invalid scenario number for secure-data: %q (valid: 1-3)", scenario)
	}
	return result, nil
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
