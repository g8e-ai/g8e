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
	"runtime"

	"github.com/spf13/cobra"
)

func testCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "test",
		Short: "Run test suites (unit, integration, e2e, scenario)",
		Long:  `Run different tiers of the g8e test suite. Unit tests run fast without external dependencies. Integration tests use in-memory components. E2E tests require a running gateway.`,
	}

	cmd.AddCommand(
		testUnitCmd(),
		testIntegrationCmd(),
		testE2ECmd(),
		testScenarioCmd(),
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
			fmt.Println("Note: This requires the gateway to be running and authenticated.")
			fmt.Println("Run './g8e gw start' and './g8e auth login' first.")
			
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
