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

func evalsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "evals",
		Short: "Run evaluation benchmarks",
		Long:  `Orchestrate AI benchmark evaluation suites against the g8e Agentic Ensemble.`,
	}

	cmd.AddCommand(
		evalsBenchCmd(),
		evalsListCmd(),
		evalsVerifyReceiptsCmd(),
	)

	return cmd
}

func evalsBenchCmd() *cobra.Command {
	var suite string
	var model string
	var provider string
	var operatorSessionID string
	var goldSet string
	var limit int

	cmd := &cobra.Command{
		Use:   "bench",
		Short: "Run an evaluation benchmark",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load("")
			if err != nil {
				return fmt.Errorf("failed to load config: %w", err)
			}

			venvPython := filepath.Join(cfg.ProjectRoot, ".venv", "bin", "python")
			if _, err := os.Stat(venvPython); os.IsNotExist(err) {
				return fmt.Errorf("g8ee venv not found at %s - run setup first", venvPython)
			}

			evalsDir := filepath.Join(cfg.ProjectRoot, "evals")
			pyArgs := []string{venvPython, "-m", "g8e_evals", "run"}

			if suite != "" {
				pyArgs = append(pyArgs, "--suite", suite)
			}
			if model != "" {
				pyArgs = append(pyArgs, "--model", model)
			}
			if provider != "" {
				pyArgs = append(pyArgs, "--provider", provider)
			}
			if operatorSessionID != "" {
				pyArgs = append(pyArgs, "--operator-session-id", operatorSessionID)
			}
			if goldSet != "" {
				pyArgs = append(pyArgs, "--gold-set", goldSet)
			}
			if limit > 0 {
				pyArgs = append(pyArgs, "--limit", fmt.Sprintf("%d", limit))
			}

			cmd.Printf("Running evals benchmark in %s...\n", evalsDir)
			pyCmd := exec.Command(pyArgs[0], pyArgs[1:]...)
			pyCmd.Stdout = os.Stdout
			pyCmd.Stderr = os.Stderr
			pyCmd.Dir = cfg.ProjectRoot
			pyCmd.Env = os.Environ()
			return pyCmd.Run()
		},
	}

	cmd.Flags().StringVar(&suite, "suite", "ifeval", "Benchmark suite (ifeval)")
	cmd.Flags().StringVar(&model, "model", "", "Model name (e.g., gpt-4o)")
	cmd.Flags().StringVar(&provider, "provider", "", "LLM provider (openai, anthropic, gemini, ollama)")
	cmd.Flags().StringVar(&operatorSessionID, "operator-session-id", "", "Operator session ID (auto-loaded from login)")
	cmd.Flags().StringVar(&goldSet, "gold-set", "", "Path to gold set JSONL file")
	cmd.Flags().IntVar(&limit, "limit", 0, "Limit number of tasks to run")

	return cmd
}

func evalsListCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List available evaluation suites",
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.Println("Available evaluation suites:")
			cmd.Println("  ifeval  - Instruction Following Evaluation")
			return nil
		},
	}
	return cmd
}

func evalsVerifyReceiptsCmd() *cobra.Command {
	var reportDir string
	var pkiDir string

	cmd := &cobra.Command{
		Use:   "verify-receipts",
		Short: "Verify ActionReceipt signatures offline",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load("")
			if err != nil {
				return fmt.Errorf("failed to load config: %w", err)
			}

			venvPython := filepath.Join(cfg.ProjectRoot, ".venv", "bin", "python")
			if _, err := os.Stat(venvPython); os.IsNotExist(err) {
				return fmt.Errorf("g8ee venv not found at %s - run setup first", venvPython)
			}

			reportDir = args[0]
			pyArgs := []string{venvPython, "-m", "g8e_evals", "verify-receipts", reportDir}

			if pkiDir != "" {
				pyArgs = append(pyArgs, "--pki-dir", pkiDir)
			}

			cmd.Printf("Verifying receipts in %s...\n", reportDir)
			pyCmd := exec.Command(pyArgs[0], pyArgs[1:]...)
			pyCmd.Stdout = os.Stdout
			pyCmd.Stderr = os.Stderr
			pyCmd.Dir = cfg.ProjectRoot
			pyCmd.Env = os.Environ()
			return pyCmd.Run()
		},
	}

	cmd.Flags().StringVar(&pkiDir, "pki-dir", "", "PKI directory (default: .g8e/pki)")

	return cmd
}
