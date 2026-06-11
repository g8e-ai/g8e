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
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/g8e-ai/g8e/internal/cli/config"
	"github.com/g8e-ai/g8e/internal/cli/platform"
	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/spf13/cobra"
	_ "modernc.org/sqlite"
)

func testCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "test",
		Short: "Run test suites (unit, integration, e2e, scenario, emulator, chaos)",
		Long:  `Run different tiers of the g8e test suite. Unit tests run fast without external dependencies. Integration tests use in-memory components. E2E tests require a running gateway. Emulator runs scenarios against a real Gateway/Operator. Chaos generates governance events for testing.`,
	}

	cmd.AddCommand(
		testUnitCmd(),
		testIntegrationCmd(),
		testE2ECmd(),
		testScenarioCmd(),
		emulatorCmd(),
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
				return fmt.Errorf("unit tests failed: %w", err)
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
		Short: "Run Tier 2 (In-Memory Integration) tests",
		Long:  `Run in-memory integration tests with the 'integration' build tag. These tests use SQLite in-memory databases, local PKI generation, and local pubsub.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Println("Running Tier 2 (In-Memory Integration) tests...")

			testRace := ""
			if runtime.GOOS != "windows" {
				testRace = "-race"
			}

			testCmd := exec.Command("go", "test", "-tags=integration", testRace, "-count=1", "-timeout", "180s", "./...")
			testCmd.Stdout = os.Stdout
			testCmd.Stderr = os.Stderr

			if err := testCmd.Run(); err != nil {
				return fmt.Errorf("integration tests failed: %w", err)
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
			fmt.Println("Note: This requires the gateway to be running and authenticated.")
			fmt.Println("Run './g8e gw start' and './g8e auth login' first.")

			testRace := ""
			if runtime.GOOS != "windows" {
				testRace = "-race"
			}

			testCmd := exec.Command("go", "test", "-tags=e2e", testRace, "-count=1", "-timeout", "180s", "./test/...")
			testCmd.Stdout = os.Stdout
			testCmd.Stderr = os.Stderr

			if err := testCmd.Run(); err != nil {
				return fmt.Errorf("e2e tests failed: %w", err)
			}

			fmt.Println("E2E tests completed successfully.")
			return nil
		},
	}

	return cmd
}

func testScenarioCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "scenario",
		Short: "Run Tier 3 (Scenario) tests",
		Long:  `Run scenario-specific E2E tests with the 'e2e' build tag. These tests require a running g8e gateway and authenticated CLI session.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Println("Running Tier 3 (Scenario) tests...")

			// Check if gateway is running
			cfg, err := config.Load("")
			if err != nil {
				return fmt.Errorf("failed to load config: %w", err)
			}

			pm, err := platform.NewProcessManager(cfg.ProjectRoot)
			if err != nil {
				return fmt.Errorf("failed to create process manager: %w", err)
			}

			running, _, err := pm.OperatorStatus()
			if err != nil {
				return fmt.Errorf("failed to check Operator status: %w", err)
			}

			if !running {
				fmt.Println("Error: Gateway is not running.")
				fmt.Println("Run './g8e gw start' first (it automatically bootstraps authentication).")
				return fmt.Errorf("gateway not running")
			}

			testRace := ""
			if runtime.GOOS != "windows" {
				testRace = "-race"
			}

			testCmd := exec.Command("go", "test", "-tags=e2e", testRace, "-count=1", "-timeout", "180s", "./test/scenario/...")
			testCmd.Stdout = os.Stdout
			testCmd.Stderr = os.Stderr

			if err := testCmd.Run(); err != nil {
				return fmt.Errorf("scenario tests failed: %w", err)
			}

			fmt.Println("Scenario tests completed successfully.")
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
			if err := constants.InitPaths(); err != nil {
				return fmt.Errorf("failed to initialize paths: %w", err)
			}

			testVaultDir := constants.Paths.Infra.TestVaultDir
			if _, err := os.Stat(testVaultDir); os.IsNotExist(err) {
				cmd.Printf("Test vault directory not found at %s\n", testVaultDir)
				cmd.Println("Run './g8e test chaos' first to generate test data.")
				return nil
			}

			// Find all test run directories
			entries, err := os.ReadDir(testVaultDir)
			if err != nil {
				return fmt.Errorf("failed to read test vault directory: %w", err)
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
			dbPath := filepath.Join(latestRun, "g8e.db")

			if _, err := os.Stat(dbPath); os.IsNotExist(err) {
				return fmt.Errorf("chaos test database not found at %s", dbPath)
			}

			// Query chaos_events table
			query := "SELECT category, outcome, COUNT(*) FROM chaos_events GROUP BY category, outcome"
			db, err := sql.Open("sqlite", dbPath)
			if err != nil {
				return fmt.Errorf("failed to open database: %w", err)
			}
			defer db.Close()

			rows, err := db.Query(query)
			if err != nil {
				return fmt.Errorf("failed to query chaos events: %w", err)
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
					return fmt.Errorf("failed to scan row: %w", err)
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
