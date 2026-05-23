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

package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/g8e-ai/g8e/cmd/g8e/internal/config"
	"github.com/spf13/cobra"
)

func testCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "test",
		Short: "Run test suites",
		Long:  `Orchestrate test execution for g8eo (Gateway) and g8ee (Ensemble).`,
	}

	cmd.AddCommand(
		testG8eoCmd(),
		testG8eeCmd(),
		testCICmd(),
		testChaosCmd(),
	)

	return cmd
}

func testG8eoCmd() *cobra.Command {
	var race bool
	var verbose bool
	var run string

	cmd := &cobra.Command{
		Use:   "g8eo",
		Short: "Run Gateway (g8eo) tests",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load("")
			if err != nil {
				return fmt.Errorf("failed to load config: %w", err)
			}

			g8eoDir := filepath.Join(cfg.ProjectRoot, "services", "g8eo")
			makeArgs := []string{"-C", g8eoDir, "test"}

			if run != "" {
				makeArgs = append(makeArgs, "TESTFLAGS=-run="+run)
			}
			if race {
				makeArgs = append(makeArgs, "TESTFLAGS=-race")
			}
			if verbose {
				makeArgs = append(makeArgs, "TESTFLAGS=-v")
			}

			cmd.Printf("Running g8eo tests in %s...\n", g8eoDir)
			makeCmd := exec.Command("make", makeArgs...)
			makeCmd.Stdout = os.Stdout
			makeCmd.Stderr = os.Stderr
			return makeCmd.Run()
		},
	}

	cmd.Flags().BoolVar(&race, "race", true, "Enable race detector")
	cmd.Flags().BoolVar(&verbose, "v", false, "Verbose output")
	cmd.Flags().StringVar(&run, "run", "", "Run specific test (regex)")

	return cmd
}

func testG8eeCmd() *cobra.Command {
	var verbose bool
	var run string

	cmd := &cobra.Command{
		Use:   "g8ee",
		Short: "Run Ensemble (g8ee) tests",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load("")
			if err != nil {
				return fmt.Errorf("failed to load config: %w", err)
			}

			venvPython := filepath.Join(cfg.ProjectRoot, ".venv", "bin", "python")
			if _, err := os.Stat(venvPython); os.IsNotExist(err) {
				return fmt.Errorf("g8ee venv not found at %s - run setup first", venvPython)
			}

			g8eeDir := filepath.Join(cfg.ProjectRoot, "services", "g8ee")
			pytestArgs := []string{venvPython, "-m", "pytest", g8eeDir}

			if run != "" {
				pytestArgs = append(pytestArgs, "-k", run)
			}
			if verbose {
				pytestArgs = append(pytestArgs, "-v")
			}

			cmd.Printf("Running g8ee tests in %s...\n", g8eeDir)
			pytestCmd := exec.Command(pytestArgs[0], pytestArgs[1:]...)
			pytestCmd.Stdout = os.Stdout
			pytestCmd.Stderr = os.Stderr
			pytestCmd.Dir = cfg.ProjectRoot
			return pytestCmd.Run()
		},
	}

	cmd.Flags().BoolVar(&verbose, "v", false, "Verbose output")
	cmd.Flags().StringVar(&run, "run", "", "Run specific test (regex)")

	return cmd
}

func testCICmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "ci",
		Short: "Run CI test suite (g8eo + g8ee)",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load("")
			if err != nil {
				return fmt.Errorf("failed to load config: %w", err)
			}

			cmd.Println("Running CI test suite...")

			g8eoDir := filepath.Join(cfg.ProjectRoot, "services", "g8eo")
			cmd.Println("\n=== Testing g8eo ===")
			makeCmd := exec.Command("make", "-C", g8eoDir, "test")
			makeCmd.Stdout = os.Stdout
			makeCmd.Stderr = os.Stderr
			if err := makeCmd.Run(); err != nil {
				return fmt.Errorf("g8eo tests failed: %w", err)
			}

			venvPython := filepath.Join(cfg.ProjectRoot, ".venv", "bin", "python")
			if _, err := os.Stat(venvPython); err == nil {
				cmd.Println("\n=== Testing g8ee ===")
				g8eeDir := filepath.Join(cfg.ProjectRoot, "services", "g8ee")
				pytestCmd := exec.Command(venvPython, "-m", "pytest", g8eeDir)
				pytestCmd.Stdout = os.Stdout
				pytestCmd.Stderr = os.Stderr
				pytestCmd.Dir = cfg.ProjectRoot
				if err := pytestCmd.Run(); err != nil {
					return fmt.Errorf("g8ee tests failed: %w", err)
				}
			} else {
				cmd.Println("\nSkipping g8ee tests (venv not found)")
			}

			cmd.Println("\nCI test suite passed")
			return nil
		},
	}
	return cmd
}

func testChaosCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "chaos",
		Short: "Run chaos engineering tests",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load("")
			if err != nil {
				return fmt.Errorf("failed to load config: %w", err)
			}

			g8eoDir := filepath.Join(cfg.ProjectRoot, "services", "g8eo")
			cmd.Println("Running chaos tests with sudo...")
			makeCmd := exec.Command("make", "-C", g8eoDir, "test-sudo")
			makeCmd.Stdout = os.Stdout
			makeCmd.Stderr = os.Stderr
			return makeCmd.Run()
		},
	}
	return cmd
}
