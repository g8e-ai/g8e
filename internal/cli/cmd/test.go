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
	"time"

	"github.com/g8e-ai/g8e/internal/cli/config"
	"github.com/spf13/cobra"
)

func testCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "test",
		Short: "Run test suites",
		Long:  `Orchestrate test execution for g8eo (Gateway).`,
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
		testUnitCmd(),
		testIntegrationCmd(),
		testG8eoCmd(),
		testCICmd(),
		testChaosCmd(),
		testScenarioCmd(),
		testReviewCmd(),
	)

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

func testCICmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "ci",
		Short: "Run CI test suite (g8eo)",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load("")
			if err != nil {
				return fmt.Errorf("failed to load config: %w", err)
			}

			cmd.Println("Running CI test suite...")
			cmd.Println("\n=== Testing g8eo ===")
			goCmd := exec.Command("go", "test", "-race", "-timeout", "180s", "./cmd/...", "./internal/...", "./pkg/...", "./test/...")
			goCmd.Stdout = os.Stdout
			goCmd.Stderr = os.Stderr
			goCmd.Dir = cfg.ProjectRoot
			if err := goCmd.Run(); err != nil {
				return fmt.Errorf("g8eo tests failed: %w", err)
			}

			cmd.Println("\n=== Testing scenario integration ===")
			scenarioCmd := exec.Command("go", "test", "-tags=integration", "-v", "-run", "TestScenarios", "./test/scenario/...")
			scenarioCmd.Stdout = os.Stdout
			scenarioCmd.Stderr = os.Stderr
			scenarioCmd.Dir = cfg.ProjectRoot
			if err := scenarioCmd.Run(); err != nil {
				return fmt.Errorf("scenario tests failed: %w", err)
			}

			cmd.Println("\nCI test suite passed")
			return nil
		},
	}
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
			chaosPath := filepath.Join(cfg.ProjectRoot, "cmd", "chaos_tester")
			goArgs := []string{"run", chaosPath}
			if count > 0 {
				goArgs = append(goArgs, "--count", fmt.Sprintf("%d", count))
			}
			goCmd := exec.Command("go", goArgs...)
			goCmd.Stdout = os.Stdout
			goCmd.Stderr = os.Stderr
			goCmd.Dir = cfg.ProjectRoot
			return goCmd.Run()
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

	cmd := &cobra.Command{
		Use:   "review",
		Short: "Review integration test vault results",
		Long:  `Inspect and manage persistent test vaults from integration test runs.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load("")
			if err != nil {
				return fmt.Errorf("failed to load config: %w", err)
			}

			vaultDir := filepath.Join(cfg.ProjectRoot, ".g8e", "test-vault")

			if clean {
				if cleanOld > 0 {
					return cleanOldVaults(vaultDir, cleanOld, cmd)
				}
				return cleanAllVaults(vaultDir, cmd)
			}

			if list {
				return listVaults(vaultDir, cmd)
			}

			if query != "" {
				if vaultPath == "" {
					return fmt.Errorf("--vault-path required when using --query")
				}
				return queryVault(vaultPath, query, cmd)
			}

			if vaultPath != "" {
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

	cmd.Printf("Found %d test vault(s):\n\n", len(entries))
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
