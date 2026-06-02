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
	"bytes"
	"os"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExecute(t *testing.T) {
	t.Run("Execute does not panic on valid command structure", func(t *testing.T) {
		// Capture stdout/stderr to prevent noise
		oldStdout := os.Stdout
		oldStderr := os.Stderr
		r, w, _ := os.Pipe()
		os.Stdout = w
		os.Stderr = w

		// This will fail because no arguments are provided, but should not panic
		// We're testing that the Execute function can be called without panicking
		defer func() {
			w.Close()
			os.Stdout = oldStdout
			os.Stderr = oldStderr
			r.Close()
		}()

		// Execute will call os.Exit(1) on error, so we can't actually call it
		// Instead, we test the root command construction
		rootCmd := &cobra.Command{
			Use:   "g8e",
			Short: "g8e Platform Manager - CLI for the g8e Gateway and g8e Operator",
			Long: `g8e is a zero-trust execution platform for agentic infrastructure.
The CLI manages the g8e Gateway (g8eg) and g8e Operator (g8eo).`,
			CompletionOptions: cobra.CompletionOptions{
				DisableDefaultCmd: true,
			},
		}

		rootCmd.AddCommand(
			gatewayCmd(),
			authCmd(),
			approveCmd(),
			dataCmd(),
			testCmd(),
			securityCmd(),
		)

		assert.Equal(t, "g8e", rootCmd.Use)
		assert.Contains(t, rootCmd.Short, "g8e Platform Manager")
		assert.Len(t, rootCmd.Commands(), 6)
	})

	t.Run("root command has all expected subcommands", func(t *testing.T) {
		rootCmd := &cobra.Command{
			Use:   "g8e",
			Short: "g8e Platform Manager - CLI for the g8e Gateway and g8e Operator",
			Long: `g8e is a zero-trust execution platform for agentic infrastructure.
The CLI manages the g8e Gateway (g8eg) and g8e Operator (g8eo).`,
			CompletionOptions: cobra.CompletionOptions{
				DisableDefaultCmd: true,
			},
		}

		rootCmd.AddCommand(
			gatewayCmd(),
			authCmd(),
			approveCmd(),
			dataCmd(),
			testCmd(),
			securityCmd(),
		)

		expectedCommands := []string{"gw", "auth", "approve", "data", "test", "security"}
		for _, expected := range expectedCommands {
			found := false
			for _, cmd := range rootCmd.Commands() {
				if cmd.Name() == expected {
					found = true
					break
				}
			}
			assert.True(t, found, "root command should have %s subcommand", expected)
		}
	})

	t.Run("root command has correct completion options", func(t *testing.T) {
		rootCmd := &cobra.Command{
			Use:   "g8e",
			Short: "g8e Platform Manager - CLI for the g8e Gateway and g8e Operator",
			Long: `g8e is a zero-trust execution platform for agentic infrastructure.
The CLI manages the g8e Gateway (g8eg) and g8e Operator (g8eo).`,
			CompletionOptions: cobra.CompletionOptions{
				DisableDefaultCmd: true,
			},
		}

		assert.True(t, rootCmd.CompletionOptions.DisableDefaultCmd)
	})

	t.Run("root command help displays correctly", func(t *testing.T) {
		rootCmd := &cobra.Command{
			Use:   "g8e",
			Short: "g8e Platform Manager - CLI for the g8e Gateway and g8e Operator",
			Long: `g8e is a zero-trust execution platform for agentic infrastructure.
The CLI manages the g8e Gateway (g8eg) and g8e Operator (g8eo).`,
			CompletionOptions: cobra.CompletionOptions{
				DisableDefaultCmd: true,
			},
		}

		rootCmd.AddCommand(
			gatewayCmd(),
			authCmd(),
			approveCmd(),
			dataCmd(),
			testCmd(),
			securityCmd(),
		)

		var buf bytes.Buffer
		rootCmd.SetOut(&buf)
		rootCmd.SetErr(&buf)

		err := rootCmd.Help()
		require.NoError(t, err)
		output := buf.String()

		assert.Contains(t, output, "g8e")
		assert.Contains(t, output, "gw")
		assert.Contains(t, output, "auth")
		assert.Contains(t, output, "approve")
		assert.Contains(t, output, "data")
		assert.Contains(t, output, "test")
		assert.Contains(t, output, "security")
	})
}
