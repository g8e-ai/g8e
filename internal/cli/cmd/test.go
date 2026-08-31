// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package cmd

import (
	"context"
	"database/sql"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/spf13/cobra"
	_ "modernc.org/sqlite"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/paths"
)

func testCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "test",
		Short: "Run test suites (unit, integration, e2e, e2e-full, lint, chaos)",
		Long:  `Run different tiers of the g8e test suite. Unit tests run fast without external dependencies. Integration tests use in-memory components. E2E tests require a running gateway. Lint runs static analysis. Chaos generates governance events for testing.`,
	}

	cmd.AddCommand(
		testUnitCmd(),
		testIntegrationCmd(),
		testE2ECmd(),
		testE2EFullCmd(),
		testCoverageCmd(),
		testLintCmd(),
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
				"./internal/...", "./protocol/...")
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

// e2eCommandRunner abstracts exec.CommandContext so unit tests can verify
// argument propagation and failure wrapping without starting platform tests.
type e2eCommandRunner func(ctx context.Context, name string, args ...string) (int, error)

// realE2ERunner runs the Go test binary as a child process, streaming stdout
// and stderr to the parent. It returns the child exit code and any start/run
// error. Cancellation via ctx signals the child process.
func realE2ERunner(stdout, stderr io.Writer) e2eCommandRunner {
	return func(ctx context.Context, name string, args ...string) (int, error) {
		c := exec.CommandContext(ctx, name, args...)
		c.Stdout = stdout
		c.Stderr = stderr
		if err := c.Run(); err != nil {
			if exitErr, ok := err.(*exec.ExitError); ok {
				return exitErr.ExitCode(), err
			}
			return -1, err
		}
		return 0, nil
	}
}

func testE2ECmd() *cobra.Command {
	return testE2ECmdWithRunner(realE2ERunner(os.Stdout, os.Stderr))
}

// testE2ECmdWithRunner constructs the canonical Tier 3 executor. It runs only
// ./test/e2e/... with the e2e build tag, race detection on non-Windows
// platforms, -count=1, -parallel=1 as a backstop against accidental
// t.Parallel() use, and an optional --run regexp for scenario selection. The
// external scenario runner owns Docker lifecycle; this command performs network
// requests and assertions only.
func testE2ECmdWithRunner(runner e2eCommandRunner) *cobra.Command {
	var runRegexp string

	cmd := &cobra.Command{
		Use:   "e2e",
		Short: "Run Tier 3 (Live Platform E2E) tests",
		Long: `Run Tier 3 (Live Platform E2E) tests against a running production platform.

Start the platform first (docker compose up or ./g8e gw start), then run this
command. The test binary connects to the running platform and fails fast if it
is not reachable. Supports an optional --run regexp to select specific tests.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Println("Running Tier 3 (Live Platform E2E) tests...")

			testArgs := []string{"test", "-tags=e2e", "-count=1", "-parallel=1", "-timeout", "300s"}
			if runtime.GOOS != "windows" {
				testArgs = append(testArgs, "-race")
			}
			if runRegexp != "" {
				testArgs = append(testArgs, "-run", runRegexp)
			}
			testArgs = append(testArgs, "./test/e2e/...")

			code, err := runner(cmd.Context(), "go", testArgs...)
			if err != nil {
				return fmt.Errorf("%w: %w", constants.ErrE2ETestsFailed, err)
			}
			if code != 0 {
				return fmt.Errorf("%w: exit code %d", constants.ErrE2ETestsFailed, code)
			}

			fmt.Println("E2E tests completed successfully.")
			return nil
		},
	}

	cmd.Flags().StringVar(&runRegexp, "run", "", "Regular expression selecting which E2E tests to run (passed to go test -run)")

	return cmd
}

func testE2EFullCmd() *cobra.Command {
	return testE2EFullCmdWithRunner(realE2ERunner(os.Stdout, os.Stderr))
}

// testE2EFullCmdWithRunner constructs the full lifecycle Tier 3 executor.
// It starts the unified Docker Compose stack, waits for health, runs the E2E tests,
// and tears down on completion or failure.
func testE2EFullCmdWithRunner(runner e2eCommandRunner) *cobra.Command {
	var runRegexp string

	cmd := &cobra.Command{
		Use:   "e2e-full",
		Short: "Run full lifecycle Tier 3 E2E tests (start compose, test, tear down)",
		Long: `Start the unified Docker Compose stack (--profile bootstrapped), wait for
services to become healthy, run the Tier 3 E2E test suite, and tear down
(docker compose down -v) on completion or failure.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Println("Starting unified Docker Compose stack (--profile bootstrapped)...")
			if err := runDockerCompose([]string{"up", "-d"}, "bootstrapped"); err != nil {
				return fmt.Errorf("%w: docker compose up failed: %w", constants.ErrE2ETestsFailed, err)
			}
			defer func() {
				fmt.Println("Tearing down Docker Compose stack...")
				_ = runDockerCompose([]string{"down", "-v"}, "")
			}()

			fmt.Println("Waiting for platform services to become healthy...")
			waitCtx, waitCancel := context.WithTimeout(cmd.Context(), 60*time.Second)
			defer waitCancel()
			if err := waitForStackHealthy(waitCtx); err != nil {
				return fmt.Errorf("%w: platform health check failed: %w", constants.ErrE2ETestsFailed, err)
			}

			fmt.Println("Running Tier 3 (Live Platform E2E) tests...")
			testArgs := []string{"test", "-tags=e2e", "-count=1", "-parallel=1", "-timeout", "300s"}
			if runtime.GOOS != "windows" {
				testArgs = append(testArgs, "-race")
			}
			if runRegexp != "" {
				testArgs = append(testArgs, "-run", runRegexp)
			}
			testArgs = append(testArgs, "./test/e2e/...")

			code, err := runner(cmd.Context(), "go", testArgs...)
			if err != nil {
				return fmt.Errorf("%w: %w", constants.ErrE2ETestsFailed, err)
			}
			if code != 0 {
				return fmt.Errorf("%w: exit code %d", constants.ErrE2ETestsFailed, code)
			}

			fmt.Println("E2E tests completed successfully.")
			return nil
		},
	}

	cmd.Flags().StringVar(&runRegexp, "run", "", "Regular expression selecting which E2E tests to run (passed to go test -run)")
	return cmd
}

func waitForStackHealthy(ctx context.Context) error {
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	client := &http.Client{Timeout: 3 * time.Second}
	gatewayURL := "http://localhost:8080/api/v1/health"
	ensembleURL := "http://localhost:8000/health"

	for {
		gwOK := false
		if req, err := http.NewRequestWithContext(ctx, http.MethodGet, gatewayURL, nil); err == nil {
			if resp, err := client.Do(req); err == nil {
				resp.Body.Close()
				if resp.StatusCode == http.StatusOK {
					gwOK = true
				}
			}
		}

		ensOK := false
		if req, err := http.NewRequestWithContext(ctx, http.MethodGet, ensembleURL, nil); err == nil {
			if resp, err := client.Do(req); err == nil {
				resp.Body.Close()
				if resp.StatusCode == http.StatusOK {
					ensOK = true
				}
			}
		}

		if gwOK && ensOK {
			return nil
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

type coverageCommandRunner func(name string, args ...string) error

func realCoverageRunner(name string, args ...string) error {
	coverageCmd := exec.Command(name, args...)
	coverageCmd.Stdout = os.Stdout
	coverageCmd.Stderr = os.Stderr
	return coverageCmd.Run()
}

func testCoverageCmd() *cobra.Command {
	return testCoverageCmdWithRunner(realCoverageRunner)
}

func testCoverageCmdWithRunner(runner coverageCommandRunner) *cobra.Command {
	var pkg string
	var verbose bool

	cmd := &cobra.Command{
		Use:   "coverage",
		Short: "Run tests with coverage report",
		Long:  `Run tests with coverage profiling and enforce a minimum coverage threshold (75%). Use PKG flag to test a specific package, VERBOSE for detailed output.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Println("Running tests with coverage...")

			// Build the canonical Makefile coverage command.
			makeArgs := []string{"test-coverage"}

			// Forward package and verbosity selections as Make variables.
			if pkg != "" {
				makeArgs = append(makeArgs, "PKG="+pkg)
			}
			if verbose {
				makeArgs = append(makeArgs, "VERBOSE=1")
			}

			// Calculate coverage with the repository's canonical exclusions.
			if err := runner("make", makeArgs...); err != nil {
				return fmt.Errorf("%w: %w", constants.ErrCoverageTestsFailed, err)
			}

			// The Makefile target returns success only after enforcing the threshold.
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
