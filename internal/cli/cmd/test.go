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
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/g8e-ai/g8e/internal/cli/config"
	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/spf13/cobra"
	_ "modernc.org/sqlite"
)

func testCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "test",
		Short: "Run test suites",
		Long:  `Run test suites. Use 'test ci' to mirror GitHub Actions CI exactly.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load("")
			if err != nil {
				return fmt.Errorf("failed to load config: %w", err)
			}

			cmd.Println("Running all tests (unit + integration)...")
			cmd.Println("\n=== Unit Tests ===")
			goCmd := exec.Command("go", "test", "-race", "-timeout", "180s", "./cmd/...", "./internal/...", "./pkg/...", "./test/...")
			goCmd.Stdout = os.Stdout
			goCmd.Stderr = os.Stderr
			goCmd.Dir = cfg.ProjectRoot
			if err := goCmd.Run(); err != nil {
				return fmt.Errorf("unit tests failed: %w", err)
			}

			cmd.Println("\n=== Integration Tests ===")
			integrationCmd := exec.Command("go", "test", "-tags=integration", "-v", "-run", "TestScenarios", "./test/scenario/...")
			integrationCmd.Stdout = os.Stdout
			integrationCmd.Stderr = os.Stderr
			integrationCmd.Dir = cfg.ProjectRoot
			if err := integrationCmd.Run(); err != nil {
				return fmt.Errorf("integration tests failed: %w", err)
			}

			cmd.Println("\nAll tests passed")
			return nil
		},
	}

	cmd.AddCommand(
		testCiCmd(),
		testUnitCmd(),
		testIntegrationCmd(),
		testChaosCmd(),
		testScenarioCmd(),
		testReviewCmd(),
		testSummaryCmd(),
	)

	return cmd
}

func testCiCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "ci",
		Short: "Run CI pipeline locally (mirrors GitHub Actions)",
		Long:  `Run the full CI pipeline locally: proto generation, linting, vulncheck, and platform tests with platform start/stop and coverage enforcement.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load("")
			if err != nil {
				return fmt.Errorf("failed to load config: %w", err)
			}

			cmd.Println("=== Running CI pipeline locally ===")
			cmd.Println("\n=== Proto generation ===")
			protoCmd := exec.Command("make", "proto")
			protoCmd.Stdout = os.Stdout
			protoCmd.Stderr = os.Stderr
			protoCmd.Dir = cfg.ProjectRoot
			if err := protoCmd.Run(); err != nil {
				return fmt.Errorf("proto generation failed: %w", err)
			}

			cmd.Println("\n=== Linting ===")
			lintCmd := exec.Command("make", "lint")
			lintCmd.Stdout = os.Stdout
			lintCmd.Stderr = os.Stderr
			lintCmd.Dir = cfg.ProjectRoot
			if err := lintCmd.Run(); err != nil {
				return fmt.Errorf("linting failed: %w", err)
			}

			cmd.Println("\n=== Vulncheck ===")
			vulnCmd := exec.Command("make", "vulncheck")
			vulnCmd.Stdout = os.Stdout
			vulnCmd.Stderr = os.Stderr
			vulnCmd.Dir = cfg.ProjectRoot
			if err := vulnCmd.Run(); err != nil {
				return fmt.Errorf("vulncheck failed: %w", err)
			}

			cmd.Println("\n=== Platform tests ===")
			// Set G8E_STRICT_CONSTANTS_LINT for CI parity
			os.Setenv("G8E_STRICT_CONSTANTS_LINT", "1")

			// Start platform
			startCmd := exec.Command("./bin/g8e", "platform", "start")
			startCmd.Stdout = os.Stdout
			startCmd.Stderr = os.Stderr
			startCmd.Dir = cfg.ProjectRoot
			if err := startCmd.Run(); err != nil {
				return fmt.Errorf("platform start failed: %w", err)
			}

			// Run tests with same package filtering as CI
			packages := "$(go list ./... | grep -v mocks | grep -v \"^github.com/g8e-ai/g8e/cmd/\" | grep -v \"^github.com/g8e-ai/g8e/internal/testutil/\" | grep -v \"^github.com/g8e-ai/g8e/test/\" | grep -v \"^github.com/g8e-ai/g8e/internal/protocol/proto/\")"
			testCmd := exec.Command("bash", "-c", fmt.Sprintf("go test -race -timeout 180s -coverprofile=coverage.out -covermode=atomic %s", packages))
			testCmd.Stdout = os.Stdout
			testCmd.Stderr = os.Stderr
			testCmd.Dir = cfg.ProjectRoot
			testFailed := false
			if err := testCmd.Run(); err != nil {
				testFailed = true
				cmd.Printf("Tests failed: %v\n", err)
			}

			// Stop platform
			stopCmd := exec.Command("./bin/g8e", "platform", "stop")
			stopCmd.Stdout = os.Stdout
			stopCmd.Stderr = os.Stderr
			stopCmd.Dir = cfg.ProjectRoot
			if err := stopCmd.Run(); err != nil {
				return fmt.Errorf("platform stop failed: %w", err)
			}

			if testFailed {
				return fmt.Errorf("platform tests failed")
			}

			// Check coverage threshold
			cmd.Println("\n=== Coverage check ===")
			coverageCmd := exec.Command("bash", "-c", "COVERAGE=$(go tool cover -func=coverage.out | grep -v \"internal/protocol/proto\" | grep -v mocks | grep -v \"^github.com/g8e-ai/g8e/cmd/\" | grep -v \"^github.com/g8e-ai/g8e/internal/testutil/\" | grep -v \"^github.com/g8e-ai/g8e/test/\" | tail -1 | awk '{print $3}' | sed 's/%//'); if [ $(echo \"$COVERAGE < 60\" | bc -l) -eq 1 ]; then echo \"Coverage $COVERAGE% is below 60% threshold\"; exit 1; fi; echo \"Coverage $COVERAGE% meets 60% threshold\"")
			coverageCmd.Stdout = os.Stdout
			coverageCmd.Stderr = os.Stderr
			coverageCmd.Dir = cfg.ProjectRoot
			if err := coverageCmd.Run(); err != nil {
				return fmt.Errorf("coverage check failed: %w", err)
			}

			cmd.Println("\n=== CI pipeline complete ===")
			return nil
		},
	}

	return cmd
}

func testUnitCmd() *cobra.Command {
	var race bool
	var verbose bool
	var run string
	var coverage bool

	cmd := &cobra.Command{
		Use:   "unit",
		Short: "Run unit tests",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load("")
			if err != nil {
				return fmt.Errorf("failed to load config: %w", err)
			}

			var goArgs []string
			goArgs = []string{"test", "-race", "-timeout", "180s", "./cmd/...", "./internal/...", "./pkg/...", "./test/..."}

			if run != "" {
				goArgs = append(goArgs, "-run", run)
			}
			if verbose {
				goArgs = append(goArgs, "-v")
			}
			if coverage {
				goArgs = append(goArgs, "-coverprofile=coverage.out", "-covermode=atomic")
			}

			cmd.Printf("Running unit tests...\n")
			goCmd := exec.Command("go", goArgs...)
			goCmd.Stdout = os.Stdout
			goCmd.Stderr = os.Stderr
			goCmd.Dir = cfg.ProjectRoot
			return goCmd.Run()
		},
	}

	cmd.Flags().BoolVar(&race, "race", true, "Enable race detector")
	cmd.Flags().BoolVar(&verbose, "v", false, "Verbose output")
	cmd.Flags().StringVar(&run, "run", "", "Run specific test (regex)")
	cmd.Flags().BoolVar(&coverage, "coverage", false, "Generate coverage report")

	return cmd
}

func testIntegrationCmd() *cobra.Command {
	var run string

	cmd := &cobra.Command{
		Use:   "integration",
		Short: "Run integration tests",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load("")
			if err != nil {
				return fmt.Errorf("failed to load config: %w", err)
			}

			var goArgs []string
			goArgs = []string{"test", "-tags=integration", "-v", "-run", "TestScenarios", "./test/scenario/..."}

			if run != "" {
				goArgs = append(goArgs, "-run", "TestScenarios/"+run)
			}

			cmd.Printf("Running integration tests...\n")
			goCmd := exec.Command("go", goArgs...)
			goCmd.Stdout = os.Stdout
			goCmd.Stderr = os.Stderr
			goCmd.Dir = cfg.ProjectRoot
			return goCmd.Run()
		},
	}

	cmd.Flags().StringVar(&run, "run", "", "Run specific scenario (e.g., forge_signature)")

	return cmd
}

func testG8eoCmd() *cobra.Command {
	var race bool
	var verbose bool
	var run string
	var coverage bool

	cmd := &cobra.Command{
		Use:   "g8eo",
		Short: "Run Gateway (g8eo) tests",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load("")
			if err != nil {
				return fmt.Errorf("failed to load config: %w", err)
			}

			var goArgs []string
			goArgs = []string{"test", "./cmd/...", "./internal/...", "./pkg/...", "./test/..."}

			if run != "" {
				goArgs = append(goArgs, "-run", run)
			}
			if race {
				goArgs = append(goArgs, "-race")
			}
			if verbose {
				goArgs = append(goArgs, "-v")
			}
			if coverage {
				goArgs = append(goArgs, "-coverprofile=coverage.out", "-covermode=atomic")
			}

			cmd.Printf("Running g8eo tests...\n")
			goCmd := exec.Command("go", goArgs...)
			goCmd.Stdout = os.Stdout
			goCmd.Stderr = os.Stderr
			goCmd.Dir = cfg.ProjectRoot
			return goCmd.Run()
		},
	}

	cmd.Flags().BoolVar(&race, "race", true, "Enable race detector")
	cmd.Flags().BoolVar(&verbose, "v", false, "Verbose output")
	cmd.Flags().StringVar(&run, "run", "", "Run specific test (regex)")
	cmd.Flags().BoolVar(&coverage, "coverage", false, "Generate coverage report")

	return cmd
}

func testChaosCmd() *cobra.Command {
	var count int

	cmd := &cobra.Command{
		Use:   "chaos",
		Short: "Run chaos engineering tests",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load("")
			if err != nil {
				return fmt.Errorf("failed to load config: %w", err)
			}

			cmd.Println("Running chaos tests...")
			chaosArgs := []string{"chaos"}
			if count > 0 {
				chaosArgs = append(chaosArgs, "--count", fmt.Sprintf("%d", count))
			}
			g8ePath := filepath.Join(cfg.ProjectRoot, "bin", "g8e")
			if _, err := os.Stat(g8ePath); os.IsNotExist(err) {
				g8ePath = filepath.Join(cfg.ProjectRoot, "g8e")
			}
			chaosCmd := exec.Command(g8ePath, chaosArgs...)
			chaosCmd.Stdout = os.Stdout
			chaosCmd.Stderr = os.Stderr
			chaosCmd.Dir = cfg.ProjectRoot
			return chaosCmd.Run()
		},
	}

	cmd.Flags().IntVar(&count, "count", 0, "Number of payloads to fire (default: 100)")
	return cmd
}

func testScenarioCmd() *cobra.Command {
	var run string
	var verbose bool

	cmd := &cobra.Command{
		Use:   "scenario",
		Short: "Run scenario integration tests",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load("")
			if err != nil {
				return fmt.Errorf("failed to load config: %w", err)
			}

			var goArgs []string
			goArgs = []string{"test", "-tags=integration", "-run", "TestScenarios", "./test/scenario/..."}

			if verbose {
				goArgs = append(goArgs, "-v")
			}

			if run != "" {
				goArgs = append(goArgs, "-run", "TestScenarios/"+run)
			}

			cmd.Printf("Running scenario tests...\n")
			goCmd := exec.Command("go", goArgs...)
			goCmd.Stdout = os.Stdout
			goCmd.Stderr = os.Stderr
			goCmd.Dir = cfg.ProjectRoot

			return goCmd.Run()
		},
	}

	cmd.Flags().StringVar(&run, "run", "", "Run specific scenario (e.g., forge_signature)")
	cmd.Flags().BoolVarP(&verbose, "verbose", "v", false, "Verbose output")

	return cmd
}

func testReviewCmd() *cobra.Command {
	var list bool
	var query string
	var vaultPath string
	var clean bool
	var cleanOld int
	var showLedger bool
	var showReceipts bool
	var aggregate bool
	var scenario string
	var mode string

	cmd := &cobra.Command{
		Use:   "review",
		Short: "Review integration test vault results",
		Long:  `Inspect and manage persistent test vaults from integration test runs.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load("")
			if err != nil {
				return fmt.Errorf("failed to load config: %w", err)
			}

			vaultDir := filepath.Join(cfg.ProjectRoot, constants.Paths.Infra.TestVaultDir)

			if clean {
				if cleanOld > 0 {
					return cleanOldVaults(vaultDir, cleanOld, cmd)
				}
				return cleanAllVaults(vaultDir, cmd)
			}

			if list {
				return listVaults(vaultDir, cmd)
			}

			if aggregate {
				if vaultPath == "" {
					return fmt.Errorf("--vault-path required when using --aggregate")
				}
				return aggregateVaultResults(vaultPath, scenario, mode, cmd)
			}

			if query != "" {
				if vaultPath == "" {
					return fmt.Errorf("--vault-path required when using --query")
				}
				return queryVault(vaultPath, query, cmd)
			}

			if vaultPath != "" {
				if showLedger {
					return showVaultLedger(vaultPath, cmd)
				}
				if showReceipts {
					return showVaultReceipts(vaultPath, cmd)
				}
				return inspectVault(vaultPath, cmd)
			}

			return cmd.Help()
		},
	}

	cmd.Flags().BoolVar(&list, "list", false, "List all test vaults")
	cmd.Flags().StringVar(&query, "query", "", "SQL query to execute on vault database")
	cmd.Flags().StringVar(&vaultPath, "vault-path", "", "Path to specific vault directory")
	cmd.Flags().BoolVar(&clean, "clean", false, "Clean vaults")
	cmd.Flags().IntVar(&cleanOld, "clean-old", 0, "Clean vaults older than N days")
	cmd.Flags().BoolVar(&showLedger, "ledger", false, "Show git ledger for vault")
	cmd.Flags().BoolVar(&showReceipts, "receipts", false, "Show action receipts from vault")
	cmd.Flags().BoolVar(&aggregate, "aggregate", false, "Aggregate results across all scenarios")
	cmd.Flags().StringVar(&scenario, "scenario", "", "Filter by scenario name (e.g., all_valid)")
	cmd.Flags().StringVar(&mode, "mode", "", "Filter by mode (doctrine, consensus, notary)")

	return cmd
}

func listVaults(vaultDir string, cmd *cobra.Command) error {
	entries, err := os.ReadDir(vaultDir)
	if err != nil {
		if os.IsNotExist(err) {
			cmd.Println("No test vaults found")
			return nil
		}
		return fmt.Errorf("failed to read vault directory: %w", err)
	}

	if len(entries) == 0 {
		cmd.Println("No test vaults found")
		return nil
	}

	cmd.Printf("Found %d test vault run(s):\n\n", len(entries))
	for _, entry := range entries {
		if entry.IsDir() && entry.Name() != "README.md" {
			vaultPath := filepath.Join(vaultDir, entry.Name())
			info, err := entry.Info()
			if err != nil {
				cmd.Printf("  %s (error reading info: %v)\n", entry.Name(), err)
				continue
			}
			cmd.Printf("  %s (modified: %s)\n", entry.Name(), info.ModTime().Format("2006-01-02 15:04:05"))
			cmd.Printf("    Path: %s\n", vaultPath)

			// Count scenario vaults
			scenarioCount := 0
			scenarioEntries, _ := os.ReadDir(vaultPath)
			for _, se := range scenarioEntries {
				if se.IsDir() {
					scenarioCount++
				}
			}
			cmd.Printf("    Scenarios: %d\n", scenarioCount)
			cmd.Println()
		}
	}

	return nil
}

func inspectVault(vaultPath string, cmd *cobra.Command) error {
	dbPath := filepath.Join(vaultPath, "audit_vault.db")
	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		return fmt.Errorf("vault database not found at %s", dbPath)
	}

	cmd.Printf("Inspecting vault: %s\n\n", vaultPath)

	sqliteCmd := exec.Command("sqlite3", dbPath, ".tables")
	sqliteCmd.Stdout = os.Stdout
	sqliteCmd.Stderr = os.Stderr
	if err := sqliteCmd.Run(); err != nil {
		return fmt.Errorf("failed to list tables: %w", err)
	}

	cmd.Println("\nTo query this vault, use:")
	cmd.Printf("  ./g8e test review --vault-path %s --query \"SELECT * FROM action_receipts;\"\n", vaultPath)

	return nil
}

func queryVault(vaultPath, query string, cmd *cobra.Command) error {
	dbPath := filepath.Join(vaultPath, "audit_vault.db")
	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		return fmt.Errorf("vault database not found at %s", dbPath)
	}

	cmd.Printf("Executing query on %s:\n  %s\n\n", vaultPath, query)

	sqliteCmd := exec.Command("sqlite3", dbPath, query)
	sqliteCmd.Stdout = os.Stdout
	sqliteCmd.Stderr = os.Stderr
	if err := sqliteCmd.Run(); err != nil {
		return fmt.Errorf("query failed: %w", err)
	}

	return nil
}

func cleanAllVaults(vaultDir string, cmd *cobra.Command) error {
	entries, err := os.ReadDir(vaultDir)
	if err != nil {
		if os.IsNotExist(err) {
			cmd.Println("No test vaults found")
			return nil
		}
		return fmt.Errorf("failed to read vault directory: %w", err)
	}

	if len(entries) == 0 {
		cmd.Println("No test vaults found")
		return nil
	}

	removed := 0
	for _, entry := range entries {
		if entry.IsDir() && entry.Name() != "README.md" {
			vaultPath := filepath.Join(vaultDir, entry.Name())
			if err := os.RemoveAll(vaultPath); err != nil {
				cmd.Printf("Failed to remove %s: %v\n", entry.Name(), err)
			} else {
				cmd.Printf("Removed: %s\n", entry.Name())
				removed++
			}
		}
	}

	cmd.Printf("\nRemoved %d vault(s)\n", removed)
	return nil
}

func cleanOldVaults(vaultDir string, days int, cmd *cobra.Command) error {
	entries, err := os.ReadDir(vaultDir)
	if err != nil {
		if os.IsNotExist(err) {
			cmd.Println("No test vaults found")
			return nil
		}
		return fmt.Errorf("failed to read vault directory: %w", err)
	}

	if len(entries) == 0 {
		cmd.Println("No test vaults found")
		return nil
	}

	cutoff := time.Now().AddDate(0, 0, -days)
	removed := 0

	for _, entry := range entries {
		if entry.IsDir() && entry.Name() != "README.md" {
			info, err := entry.Info()
			if err != nil {
				cmd.Printf("Failed to read info for %s: %v\n", entry.Name(), err)
				continue
			}

			if info.ModTime().Before(cutoff) {
				vaultPath := filepath.Join(vaultDir, entry.Name())
				if err := os.RemoveAll(vaultPath); err != nil {
					cmd.Printf("Failed to remove %s: %v\n", entry.Name(), err)
				} else {
					cmd.Printf("Removed: %s (older than %d days)\n", entry.Name(), days)
					removed++
				}
			}
		}
	}

	cmd.Printf("\nRemoved %d vault(s) older than %d days\n", removed, days)
	return nil
}

func testSummaryCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   string(constants.StreamStatusSummary),
		Short: "Show summary of all integration test results",
		Long:  `Aggregate and display test results from all test vaults in ` + constants.Paths.Infra.TestVaultDir + `/`,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load("")
			if err != nil {
				return fmt.Errorf("failed to load config: %w", err)
			}

			vaultDir := filepath.Join(cfg.ProjectRoot, constants.Paths.Infra.TestVaultDir)
			entries, err := os.ReadDir(vaultDir)
			if err != nil {
				if os.IsNotExist(err) {
					cmd.Println("No test vaults found")
					return nil
				}
				return fmt.Errorf("failed to read vault directory: %w", err)
			}

			if len(entries) == 0 {
				cmd.Println("No test vaults found")
				return nil
			}

			cmd.Printf("Found %d test vault(s):\n\n", len(entries))

			totalChaosEvents := 0
			totalAuditEvents := 0

			for _, entry := range entries {
				if entry.IsDir() && entry.Name() != "README.md" {
					vaultPath := filepath.Join(vaultDir, entry.Name())
					dbPath := filepath.Join(vaultPath, "audit_vault.db")

					if _, err := os.Stat(dbPath); os.IsNotExist(err) {
						continue
					}

					info, err := entry.Info()
					if err != nil {
						cmd.Printf("  %s (error reading info: %v)\n", entry.Name(), err)
						continue
					}

					cmd.Printf("  %s (modified: %s)\n", entry.Name(), info.ModTime().Format("2006-01-02 15:04:05"))

					// Query chaos_events table
					chaosCount, err := countTableRows(dbPath, "chaos_events")
					if err == nil && chaosCount > 0 {
						cmd.Printf("    Chaos events: %d\n", chaosCount)
						totalChaosEvents += chaosCount
					}

					// Query events table
					auditCount, err := countTableRows(dbPath, "events")
					if err == nil && auditCount > 0 {
						cmd.Printf("    Audit events: %d\n", auditCount)
						totalAuditEvents += auditCount
					}

					cmd.Printf("    Path: %s\n", vaultPath)
					cmd.Println()
				}
			}

			cmd.Printf("Summary:\n")
			cmd.Printf("  Total chaos events: %d\n", totalChaosEvents)
			cmd.Printf("  Total audit events: %d\n", totalAuditEvents)
			cmd.Printf("\nUse './g8e test review --list' to see all vaults.\n")
			cmd.Printf("Use './g8e test review --vault-path <path> --query \"SELECT * FROM chaos_events\"' to query specific vault.\n")

			return nil
		},
	}

	return cmd
}

func countTableRows(dbPath, table string) (int, error) {
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return 0, err
	}
	defer db.Close()

	var count int
	err = db.QueryRow(fmt.Sprintf("SELECT COUNT(*) FROM %s", table)).Scan(&count)
	if err != nil {
		return 0, err
	}
	return count, nil
}

func showVaultLedger(vaultPath string, cmd *cobra.Command) error {
	// Check if this is a scenario test vault (has scenario subdirectories)
	entries, err := os.ReadDir(vaultPath)
	if err != nil {
		return fmt.Errorf("failed to read vault directory: %w", err)
	}

	// Detect scenario vault structure by looking for scenario subdirectories
	hasScenarios := false
	for _, entry := range entries {
		if entry.IsDir() && entry.Name() != "ledger" && entry.Name() != "README.md" {
			// Check if this subdirectory has mode subdirectories (doctrine/consensus/notary)
			subPath := filepath.Join(vaultPath, entry.Name())
			subEntries, subErr := os.ReadDir(subPath)
			if subErr == nil {
				for _, subEntry := range subEntries {
					if subEntry.IsDir() && (subEntry.Name() == "doctrine" || subEntry.Name() == "consensus" || subEntry.Name() == "notary") {
						hasScenarios = true
						break
					}
				}
			}
			if hasScenarios {
				break
			}
		}
	}

	if hasScenarios {
		// Scenario test vault: show all available scenario/mode ledgers
		cmd.Printf("Scenario test vault: %s\n\n", vaultPath)
		cmd.Println("Available ledgers (scenario/mode):")

		for _, entry := range entries {
			if !entry.IsDir() || entry.Name() == "ledger" || entry.Name() == "README.md" {
				continue
			}
			scenarioPath := filepath.Join(vaultPath, entry.Name())
			modes := []string{"doctrine", "consensus", "notary"}

			for _, mode := range modes {
				ledgerDir := filepath.Join(scenarioPath, mode, "ledger")
				if _, err := os.Stat(filepath.Join(ledgerDir, ".git")); err == nil {
					cmd.Printf("  %s/%s/ledger\n", entry.Name(), mode)
				}
			}
		}

		cmd.Println("\nTo view a specific ledger, use the full path:")
		cmd.Printf("  git -C %s/all_valid/doctrine/ledger log --oneline --all\n", vaultPath)
		return nil
	}

	// Standard vault: look for top-level ledger directory
	ledgerDir := filepath.Join(vaultPath, "ledger")
	if _, err := os.Stat(ledgerDir); os.IsNotExist(err) {
		return fmt.Errorf("ledger directory not found at %s", ledgerDir)
	}

	cmd.Printf("Git ledger for vault: %s\n\n", vaultPath)

	gitCmd := exec.Command("git", "-C", ledgerDir, "log", "--oneline", "--all")
	gitCmd.Stdout = os.Stdout
	gitCmd.Stderr = os.Stderr
	if err := gitCmd.Run(); err != nil {
		return fmt.Errorf("failed to show git log: %w", err)
	}

	return nil
}

func showVaultReceipts(vaultPath string, cmd *cobra.Command) error {
	// Check if this is a scenario test vault (has scenario subdirectories)
	entries, err := os.ReadDir(vaultPath)
	if err != nil {
		return fmt.Errorf("failed to read vault directory: %w", err)
	}

	// Detect scenario vault structure by looking for scenario subdirectories
	hasScenarios := false
	for _, entry := range entries {
		if entry.IsDir() && entry.Name() != "ledger" && entry.Name() != "README.md" {
			// Check if this subdirectory has mode subdirectories (doctrine/consensus/notary)
			subPath := filepath.Join(vaultPath, entry.Name())
			subEntries, subErr := os.ReadDir(subPath)
			if subErr == nil {
				for _, subEntry := range subEntries {
					if subEntry.IsDir() && (subEntry.Name() == "doctrine" || subEntry.Name() == "consensus" || subEntry.Name() == "notary") {
						hasScenarios = true
						break
					}
				}
			}
			if hasScenarios {
				break
			}
		}
	}

	if hasScenarios {
		// Scenario test vault: aggregate receipts from all scenario/mode databases
		cmd.Printf("Scenario test vault: %s\n\n", vaultPath)
		cmd.Println("Aggregating receipts from all scenarios/modes:")

		// Load test results if available
		type testResult struct {
			Status string `json:"status"`
			Label  string `json:"label"`
		}
		testResults := make(map[string]map[string]*testResult)
		testResultsPath := filepath.Join(vaultPath, "test_results.json")
		if data, err := os.ReadFile(testResultsPath); err == nil {
			if err := json.Unmarshal(data, &testResults); err == nil {
				cmd.Printf("Test verdicts loaded from %s\n\n", testResultsPath)
			}
		}

		query := `SELECT transaction_id, transaction_hash, status, result_summary, 
		          state_root_before, state_root_after, action_type, signer_key_id, 
		          executed_at_ms 
		          FROM receipts ORDER BY executed_at_ms DESC`

		cmd.Printf("%-20s %-20s %-12s %-12s %-30s %-20s\n", "Transaction ID", "Scenario/Mode", "Test Status", "Receipt Status", "Summary", "Action Type")
		cmd.Println(strings.Repeat("-", 130))

		totalCount := 0
		for _, entry := range entries {
			if !entry.IsDir() || entry.Name() == "ledger" || entry.Name() == "README.md" {
				continue
			}
			scenarioPath := filepath.Join(vaultPath, entry.Name())
			modes := []string{"doctrine", "consensus", "notary"}

			for _, mode := range modes {
				dbPath := filepath.Join(scenarioPath, mode, "audit_vault.db")
				if _, err := os.Stat(dbPath); err != nil {
					continue
				}

				db, err := sql.Open("sqlite", dbPath)
				if err != nil {
					cmd.Printf("Warning: failed to open %s/%s: %v\n", entry.Name(), mode, err)
					continue
				}

				rows, err := db.Query(query)
				if err != nil {
					db.Close()
					cmd.Printf("Warning: failed to query %s/%s: %v\n", entry.Name(), mode, err)
					continue
				}

				for rows.Next() {
					var txID, txHash, summary, stateRootBefore, stateRootAfter, actionType, signerKeyID string
					var status int
					var executedAt int64

					if err := rows.Scan(&txID, &txHash, &status, &summary, &stateRootBefore, &stateRootAfter, &actionType, &signerKeyID, &executedAt); err != nil {
						rows.Close()
						db.Close()
						cmd.Printf("Warning: failed to scan row in %s/%s: %v\n", entry.Name(), mode, err)
						continue
					}

					// Convert status int to string (matches protobuf ExecutionStatus enum)
					statusStr := "UNKNOWN"
					switch status {
					case 0:
						statusStr = "UNSPECIFIED"
					case 1:
						statusStr = "EXECUTING"
					case 2:
						statusStr = "COMPLETED"
					case 3:
						statusStr = "FAILED"
					case 4:
						statusStr = "CANCELLED"
					case 5:
						statusStr = "TIMEOUT"
					}

					// Get test verdict from test_results.json
					testStatus := "N/A"
					if modeResults, ok := testResults[entry.Name()]; ok {
						if modeResult, ok := modeResults[mode]; ok {
							testStatus = modeResult.Status
						}
					}

					// Truncate fields for display
					if len(txID) > 20 {
						txID = txID[:17] + "..."
					}
					scenarioMode := fmt.Sprintf("%s/%s", entry.Name(), mode)
					if len(scenarioMode) > 20 {
						scenarioMode = scenarioMode[:17] + "..."
					}
					if len(summary) > 30 {
						summary = summary[:27] + "..."
					}
					if len(actionType) > 20 {
						actionType = actionType[:17] + "..."
					}
					if len(signerKeyID) > 20 {
						signerKeyID = signerKeyID[:17] + "..."
					}

					// Display test status (no transformation needed)

					cmd.Printf("%-20s %-20s %-12s %-12s %-30s %-20s\n", txID, scenarioMode, testStatus, statusStr, summary, actionType)
					totalCount++
				}
				rows.Close()
				db.Close()
			}
		}

		if totalCount == 0 {
			cmd.Println("No action receipts found")
		} else {
			cmd.Printf("\nTotal receipts: %d\n", totalCount)
		}

		return nil
	}

	// Standard vault: look for top-level audit_vault.db
	dbPath := filepath.Join(vaultPath, "audit_vault.db")
	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		return fmt.Errorf("vault database not found at %s", dbPath)
	}

	cmd.Printf("Action receipts for vault: %s\n\n", vaultPath)

	query := `SELECT transaction_id, transaction_hash, status, result_summary, 
	          state_root_before, state_root_after, action_type, signer_key_id, 
	          executed_at_ms 
	          FROM receipts ORDER BY executed_at_ms DESC`

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return fmt.Errorf("failed to open database: %w", err)
	}
	defer db.Close()

	rows, err := db.Query(query)
	if err != nil {
		return fmt.Errorf("failed to query receipts: %w", err)
	}
	defer rows.Close()

	cmd.Printf("%-20s %-12s %-30s %-20s %-20s\n", "Transaction ID", "Status", "Summary", "Action Type", "Signer Key ID")
	cmd.Println(strings.Repeat("-", 110))

	count := 0
	for rows.Next() {
		var txID, txHash, summary, stateRootBefore, stateRootAfter, actionType, signerKeyID string
		var status int
		var executedAt int64

		if err := rows.Scan(&txID, &txHash, &status, &summary, &stateRootBefore, &stateRootAfter, &actionType, &signerKeyID, &executedAt); err != nil {
			return fmt.Errorf("failed to scan row: %w", err)
		}

		// Convert status int to string (matches protobuf ExecutionStatus enum)
		statusStr := "UNKNOWN"
		switch status {
		case 0:
			statusStr = "UNSPECIFIED"
		case 1:
			statusStr = "EXECUTING"
		case 2:
			statusStr = "COMPLETED"
		case 3:
			statusStr = "FAILED"
		case 4:
			statusStr = "CANCELLED"
		case 5:
			statusStr = "TIMEOUT"
		}

		// Truncate fields for display
		if len(txID) > 20 {
			txID = txID[:17] + "..."
		}
		if len(summary) > 30 {
			summary = summary[:27] + "..."
		}
		if len(actionType) > 20 {
			actionType = actionType[:17] + "..."
		}
		if len(signerKeyID) > 20 {
			signerKeyID = signerKeyID[:17] + "..."
		}

		cmd.Printf("%-20s %-12s %-30s %-20s %-20s\n", txID, statusStr, summary, actionType, signerKeyID)
		count++
	}

	if count == 0 {
		cmd.Println("No action receipts found")
	} else {
		cmd.Printf("\nTotal receipts: %d\n", count)
	}

	return nil
}

func aggregateVaultResults(vaultPath, scenarioFilter, modeFilter string, cmd *cobra.Command) error {
	entries, err := os.ReadDir(vaultPath)
	if err != nil {
		return fmt.Errorf("failed to read vault directory: %w", err)
	}

	cmd.Printf("Aggregating results from: %s\n\n", vaultPath)

	totalReceipts := 0
	totalEvents := 0

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		scenarioName := entry.Name()
		if scenarioFilter != "" && scenarioName != scenarioFilter {
			continue
		}

		scenarioPath := filepath.Join(vaultPath, scenarioName)
		modes := []string{"doctrine", "consensus", "notary"}

		for _, modeName := range modes {
			if modeFilter != "" && modeName != modeFilter {
				continue
			}

			modePath := filepath.Join(scenarioPath, modeName)
			dbPath := filepath.Join(modePath, "audit_vault.db")

			if _, err := os.Stat(dbPath); os.IsNotExist(err) {
				continue
			}

			// Count receipts
			receiptCount, err := countTableRows(dbPath, "receipts")
			if err == nil && receiptCount > 0 {
				cmd.Printf("%s/%s: %d receipts\n", scenarioName, modeName, receiptCount)
				totalReceipts += receiptCount
			}

			// Count events
			eventCount, err := countTableRows(dbPath, "events")
			if err == nil && eventCount > 0 {
				totalEvents += eventCount
			}
		}
	}

	cmd.Printf("\nSummary:\n")
	cmd.Printf("  Total receipts: %d\n", totalReceipts)
	cmd.Printf("  Total events: %d\n", totalEvents)

	if totalReceipts == 0 && totalEvents == 0 {
		cmd.Println("\nNo data found. Use --list to see available scenarios.")
	}

	return nil
}
