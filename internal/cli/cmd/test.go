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

	"github.com/g8e-ai/g8e/internal/cli/config"
	"github.com/spf13/cobra"
)

func testCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "test",
		Short: "Run test suites",
		Long:  `Orchestrate test execution for g8eo (Gateway).`,
	}

	cmd.AddCommand(
		testG8eoCmd(),
		testCICmd(),
		testChaosCmd(),
	)

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
			goArgs = []string{"test", "./..."}

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
			goCmd := exec.Command("go", "test", "-race", "-timeout", "180s", "./...")
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
