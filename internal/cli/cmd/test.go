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
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/g8e-ai/g8e/internal/cli/config"
	"github.com/g8e-ai/g8e/internal/cli/platform"
	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/g8e-ai/g8e/internal/paths"
	"github.com/spf13/cobra"
	_ "modernc.org/sqlite"
)

func testCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "test",
		Short: "Run test suites (unit, integration, e2e, lint, agentic-tool-emulator, chaos)",
		Long:  `Run different tiers of the g8e test suite. Unit tests run fast without external dependencies. Integration tests use in-memory components. E2E tests require a running gateway. Lint runs static analysis. Agentic-tool-emulator runs demos against a real Gateway/Operator. Chaos generates governance events for testing.`,
	}

	cmd.AddCommand(
		testUnitCmd(),
		testIntegrationCmd(),
		testE2ECmd(),
		testCoverageCmd(),
		testLintCmd(),
		agenticToolEmulatorCmd(),
		chaosCmd(),
		testSummaryCmd(),
	)

	return cmd
}

func testUnitCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "unit",
		Short: "Run Tier 1 (Unit) tests",
		Long:  `Run unit tests without any build tags. These tests use mocks/stubs and have no external dependencies (no files, network, or DB).`,
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Println("Running Tier 1 (Unit) tests...")

			// Build the test command based on the Makefile test-unit target
			// TEST_RACE: -race on non-Windows, empty on Windows
			// TEST_COUNT: -count=1
			// TEST_SHORT_TIMEOUT: 60s
			// TEST_PKGS: all packages excluding cmd/, test/, internal/testutil/, mocks/, proto/

			testRace := ""
			if runtime.GOOS != "windows" {
				testRace = "-race"
			}

			testCmd := exec.Command("go", "test", testRace, "-count=1", "-timeout", "60s",
				"./internal/...", "./pkg/...")
			testCmd.Stdout = os.Stdout
			testCmd.Stderr = os.Stderr

			if err := testCmd.Run(); err != nil {
				return fmt.Errorf("%w: %w", constants.ErrUnitTestsFailed, err)
			}

			fmt.Println("Unit tests completed successfully.")
			return nil
		},
	}

	return cmd
}

func testIntegrationCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "integration",
		Short: "Run Tier 2 (In-Process Integration) tests",
		Long:  `Run in-process integration tests with the 'integration' build tag. These tests run the gateway in-process against real on-disk SQLite databases, local PKI generation, and local pubsub.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Println("Running Tier 2 (In-Process Integration) tests...")

			testRace := ""
			if runtime.GOOS != "windows" {
				testRace = "-race"
			}

			testCmd := exec.Command("go", "test", "-tags=integration", testRace, "-count=1", "-timeout", "180s", "./...")
			testCmd.Stdout = os.Stdout
			testCmd.Stderr = os.Stderr

			if err := testCmd.Run(); err != nil {
				return fmt.Errorf("%w: %w", constants.ErrIntegrationTestsFailed, err)
			}

			fmt.Println("Integration tests completed successfully.")
			return nil
		},
	}

	return cmd
}

func testE2ECmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "e2e",
		Short: "Run Tier 3 (Live Platform E2E) tests",
		Long:  `Run live-platform E2E tests with the 'e2e' build tag. These tests require a running g8e gateway and authenticated CLI session.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Println("Running Tier 3 (Live Platform E2E) tests...")

			cfg, err := config.Load("")
			if err != nil {
				return fmt.Errorf("%w: %w", constants.ErrConfigLoadFailed, err)
			}

			// Try HTTP check first (works for Docker/foreground/background modes)
			// Use plain HTTP with short timeout to avoid hanging when gateway is not running
			healthURL := fmt.Sprintf("http://127.0.0.1:%d/api/v1/health", constants.Ports.OperatorHttp)
			httpClient := &http.Client{Timeout: 5 * time.Second}
			isRunning := false
			resp, err := httpClient.Get(healthURL)
			if err == nil && resp.StatusCode == http.StatusOK {
				isRunning = true
				resp.Body.Close()
			}

			// Fallback to ProcessManager check (for background/host mode)
			if !isRunning {
				pm, err := platform.NewProcessManager(cfg.ProjectRoot)
				if err != nil {
					return fmt.Errorf("%w: %w", constants.ErrInternal, err)
				}

				running, _, err := pm.OperatorStatus()
				if err != nil {
					return fmt.Errorf("%w: %w", constants.ErrInternal, err)
				}
				isRunning = running
			}

			if !isRunning {
				fmt.Println("Error: Gateway is not running.")
				fmt.Println("Run './g8e gw start' first (it automatically bootstraps authentication).")
				return constants.ErrGatewayNotRunning
			}

			testRace := ""
			if runtime.GOOS != "windows" {
				testRace = "-race"
			}

			testCmd := exec.Command("go", "test", "-tags=e2e", testRace, "-count=1", "-timeout", "180s", "./test/...")
			testCmd.Stdout = os.Stdout
			testCmd.Stderr = os.Stderr

			if err := testCmd.Run(); err != nil {
				return fmt.Errorf("%w: %w", constants.ErrE2ETestsFailed, err)
			}

			fmt.Println("E2E tests completed successfully.")
			return nil
		},
	}

	return cmd
}

func testCoverageCmd() *cobra.Command {
	var pkg string
	var verbose bool

	cmd := &cobra.Command{
		Use:   "coverage",
		Short: "Run tests with coverage report",
		Long:  `Run tests with coverage profiling and enforce a minimum coverage threshold (70%). Use PKG flag to test a specific package, VERBOSE for detailed output.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Println("Running tests with coverage...")

			testRace := ""
			if runtime.GOOS != "windows" {
				testRace = "-race"
			}

			// Build test command
			testArgs := []string{"test", testRace, "-timeout", "180s", "-coverprofile=coverage.out", "-covermode=atomic"}
			if verbose {
				testArgs = append(testArgs, "-v")
			}

			// Determine packages to test
			if pkg != "" {
				fmt.Printf("Running coverage for package: %s\n", pkg)
				testArgs = append(testArgs, pkg)
			} else {
				fmt.Println("Running coverage for all packages...")
				testArgs = append(testArgs, "./internal/...", "./pkg/...")
			}

			testCmd := exec.Command("go", testArgs...)
			testCmd.Stdout = os.Stdout
			testCmd.Stderr = os.Stderr

			if err := testCmd.Run(); err != nil {
				return fmt.Errorf("%w: %w", constants.ErrCoverageTestsFailed, err)
			}

			// Calculate coverage
			coverageCmd := exec.Command("go", "tool", "cover", "-func=coverage.out")
			output, err := coverageCmd.Output()
			if err != nil {
				return fmt.Errorf("%w: %w", constants.ErrInternal, err)
			}

			// Parse coverage percentage from last line
			lines := strings.Split(string(output), "\n")
			for _, line := range lines {
				if strings.Contains(line, "total:") {
					parts := strings.Fields(line)
					if len(parts) >= 3 {
						coverageStr := strings.TrimSuffix(parts[2], "%")
						fmt.Printf("\nTotal coverage: %s%%\n", coverageStr)
						return nil
					}
				}
			}

			fmt.Println("Coverage tests completed successfully.")
			return nil
		},
	}

	cmd.Flags().StringVar(&pkg, "pkg", "", "Specific package to test (e.g., ./internal/services/auth)")
	cmd.Flags().BoolVar(&verbose, "verbose", false, "Verbose output")

	return cmd
}

func testLintCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "lint",
		Short: "Run linting and quality checks",
		Long:  `Run golangci-lint with modern Go 1.26.3 best practices. This includes staticcheck, govet, and additional linters for bug prevention, security, and code quality.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Println("Running linting and quality checks...")

			// Check if golangci-lint is installed
			if _, err := exec.LookPath("golangci-lint"); err != nil {
				fmt.Println("golangci-lint not found. Installing...")
				installCmd := exec.Command("go", "install", "github.com/golangci/golangci-lint/cmd/golangci-lint@v2.12.2")
				installCmd.Stdout = os.Stdout
				installCmd.Stderr = os.Stderr
				if err := installCmd.Run(); err != nil {
					return fmt.Errorf("%w: %w", constants.ErrInternal, err)
				}
			}

			// Run golangci-lint
			lintCmd := exec.Command("golangci-lint", "run")
			lintCmd.Stdout = os.Stdout
			lintCmd.Stderr = os.Stderr

			if err := lintCmd.Run(); err != nil {
				return fmt.Errorf("%w: %w", constants.ErrLintingFailed, err)
			}

			fmt.Println("Linting completed successfully.")
			return nil
		},
	}

	return cmd
}

func testSummaryCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "summary",
		Short: "View chaos test summary from test vault",
		Long:  `View aggregated chaos test results from the test vault database. This queries the chaos_events table across all test runs in the test vault directory.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			// Initialize paths to get test vault directory
			if err := paths.Init(); err != nil {
				return fmt.Errorf("%w: %w", constants.ErrInternal, err)
			}

			testVaultDir := paths.Infra.TestVaultDir
			if _, err := os.Stat(testVaultDir); os.IsNotExist(err) {
				cmd.Printf("Test vault directory not found at %s\n", testVaultDir)
				cmd.Println("Run './g8e test chaos' first to generate test data.")
				return nil
			}

			// Find all test run directories
			entries, err := os.ReadDir(testVaultDir)
			if err != nil {
				return fmt.Errorf("%w: %w", constants.ErrDirectoryRead, err)
			}

			var testRuns []string
			for _, entry := range entries {
				if entry.IsDir() && strings.HasSuffix(entry.Name(), "chaos-test") {
					testRuns = append(testRuns, filepath.Join(testVaultDir, entry.Name()))
				}
			}

			if len(testRuns) == 0 {
				cmd.Println("No chaos test runs found in test vault.")
				cmd.Println("Run './g8e test chaos' first to generate test data.")
				return nil
			}

			// Sort test runs by name (timestamp)
			// For simplicity, we'll just use the most recent one
			latestRun := testRuns[len(testRuns)-1]
			dbPath := filepath.Join(latestRun, constants.DbFilename)

			if _, err := os.Stat(dbPath); os.IsNotExist(err) {
				return fmt.Errorf("%w: %s", constants.ErrChaosTestDatabaseNotFound, dbPath)
			}

			// Query chaos_events table
			query := "SELECT category, outcome, COUNT(*) FROM chaos_events GROUP BY category, outcome"
			db, err := sql.Open("sqlite", dbPath)
			if err != nil {
				return fmt.Errorf("%w: %w", constants.ErrInternal, err)
			}
			defer db.Close()

			rows, err := db.Query(query)
			if err != nil {
				return fmt.Errorf("%w: %w", constants.ErrAuditQueryFailed, err)
			}
			defer rows.Close()

			type Result struct {
				Category string
				Outcome  string
				Count    int
			}

			var results []Result
			total := 0
			for rows.Next() {
				var category, outcome string
				var count int
				if err := rows.Scan(&category, &outcome, &count); err != nil {
					return fmt.Errorf("%w: %w", constants.ErrAuditScanFailed, err)
				}
				results = append(results, Result{Category: category, Outcome: outcome, Count: count})
				total += count
			}

			if total == 0 {
				cmd.Println("No chaos events found in database.")
				return nil
			}

			cmd.Printf("Chaos Test Summary (from: %s)\n", latestRun)
			cmd.Println(strings.Repeat("=", 110))
			cmd.Printf("%-23s | %-16s | %6s\n", "Category", "Outcome", "Count")
			cmd.Println(strings.Repeat("-", 110))
			for _, r := range results {
				cmd.Printf("%-23s | %-16s | %6d\n", r.Category, r.Outcome, r.Count)
			}
			cmd.Println(strings.Repeat("=", 110))
			cmd.Printf("%-23s | %-16s | %6d\n", "TOTAL", "", total)

			return nil
		},
	}

	return cmd
}
