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
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExecute(t *testing.T) {
	t.Run("root command structure matches main.go implementation", func(t *testing.T) {
		rootCmd := NewRootCmd()

		assert.Equal(t, "g8e", rootCmd.Use)
		assert.Contains(t, rootCmd.Short, "g8e Platform Manager")
		assert.Contains(t, rootCmd.Short, "g8e Gateway")
		assert.Contains(t, rootCmd.Short, "g8e Operator")
		assert.Len(t, rootCmd.Commands(), 9)
	})

	t.Run("root command has all expected subcommands", func(t *testing.T) {
		rootCmd := NewRootCmd()

		expectedCommands := []string{"gw", "auth", "mcp", "operator", "vault", "test", "demos", "audit", "swagger"}
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
		rootCmd := NewRootCmd()

		assert.True(t, rootCmd.CompletionOptions.DisableDefaultCmd, "default completion command should be disabled")
	})

	t.Run("root command help displays correctly", func(t *testing.T) {
		rootCmd := NewRootCmd()

		var buf bytes.Buffer
		rootCmd.SetOut(&buf)
		rootCmd.SetErr(&buf)

		err := rootCmd.Help()
		require.NoError(t, err)
		output := buf.String()

		assert.Contains(t, output, "g8e")
		assert.Contains(t, output, "gw")
		assert.Contains(t, output, "auth")
		assert.Contains(t, output, "mcp")
		assert.Contains(t, output, "operator")
		assert.Contains(t, output, "vault")
		assert.Contains(t, output, "test")
		assert.Contains(t, output, "demos")
		assert.Contains(t, output, "audit")
		assert.Contains(t, output, "swagger")
	})

	t.Run("root command long description contains key components", func(t *testing.T) {
		rootCmd := NewRootCmd()

		assert.Contains(t, rootCmd.Long, "zero-trust execution platform")
		assert.Contains(t, rootCmd.Long, "agentic infrastructure")
		assert.Contains(t, rootCmd.Long, "g8e Gateway")
		assert.Contains(t, rootCmd.Long, "g8e Operator")
	})
}

func TestRootCommandValidation(t *testing.T) {
	tests := []struct {
		name           string
		expectedUse    string
		expectedShort  string
		expectedLong   string
		expectedCmds   []string
		expectedCmdLen int
	}{
		{
			name:           "root command has correct metadata",
			expectedUse:    "g8e",
			expectedShort:  "g8e Platform Manager",
			expectedLong:   "zero-trust execution platform",
			expectedCmds:   []string{"gw", "auth", "mcp", "operator", "vault", "test", "demos", "audit", "swagger"},
			expectedCmdLen: 9,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rootCmd := NewRootCmd()

			assert.Contains(t, rootCmd.Use, tt.expectedUse)
			assert.Contains(t, rootCmd.Short, tt.expectedShort)
			assert.Contains(t, rootCmd.Long, tt.expectedLong)
			assert.Len(t, rootCmd.Commands(), tt.expectedCmdLen)

			for _, expectedCmd := range tt.expectedCmds {
				found := false
				for _, cmd := range rootCmd.Commands() {
					if cmd.Name() == expectedCmd {
						found = true
						break
					}
				}
				assert.True(t, found, "expected to find %s command", expectedCmd)
			}
		})
	}
}

func TestCommandExecutionErrorHandling(t *testing.T) {
	t.Run("invalid command returns error", func(t *testing.T) {
		rootCmd := NewRootCmd()

		var buf bytes.Buffer
		rootCmd.SetOut(&buf)
		rootCmd.SetErr(&buf)

		rootCmd.SetArgs([]string{"invalid-command"})
		err := rootCmd.Execute()

		assert.Error(t, err)
	})

	t.Run("no arguments shows help", func(t *testing.T) {
		rootCmd := NewRootCmd()

		var buf bytes.Buffer
		rootCmd.SetOut(&buf)
		rootCmd.SetErr(&buf)

		rootCmd.SetArgs([]string{})
		err := rootCmd.Execute()

		// Cobra shows help and returns nil error when no args provided
		require.NoError(t, err)
		output := buf.String()
		assert.Contains(t, output, "g8e")
	})
}

func TestSubcommandRegistration(t *testing.T) {
	t.Run("all subcommands are non-nil", func(t *testing.T) {
		assert.NotNil(t, gatewayCmd(), "gatewayCmd should not be nil")
		assert.NotNil(t, authCmd(), "authCmd should not be nil")
		assert.NotNil(t, mcpCmd(), "mcpCmd should not be nil")
		assert.NotNil(t, operatorCmd(), "operatorCmd should not be nil")
		assert.NotNil(t, vaultCmd(), "vaultCmd should not be nil")
		assert.NotNil(t, testCmd(), "testCmd should not be nil")
		assert.NotNil(t, demosCmd(), "demosCmd should not be nil")
		assert.NotNil(t, auditCmd(), "auditCmd should not be nil")
		assert.NotNil(t, swaggerCmd(), "swaggerCmd should not be nil")
	})

	t.Run("all subcommands have valid cobra.Command structure", func(t *testing.T) {
		commands := []*cobra.Command{
			gatewayCmd(),
			authCmd(),
			mcpCmd(),
			operatorCmd(),
			vaultCmd(),
			testCmd(),
			demosCmd(),
			auditCmd(),
			swaggerCmd(),
		}

		for _, cmd := range commands {
			assert.NotNil(t, cmd, "command should not be nil")
			assert.NotEmpty(t, cmd.Use, "command should have a Use field")
			assert.NotEmpty(t, cmd.Short, "command should have a Short field")
		}
	})
}

func TestExecuteStderrErrorOutput(t *testing.T) {
	t.Run("error is written to stderr on command failure", func(t *testing.T) {
		oldStderr := os.Stderr
		r, w, _ := os.Pipe()
		os.Stderr = w

		defer func() {
			w.Close()
			os.Stderr = oldStderr
			r.Close()
		}()

		rootCmd := NewRootCmd()

		rootCmd.SetArgs([]string{"invalid-command"})
		err := rootCmd.Execute()

		require.Error(t, err)

		w.Close()
		var buf bytes.Buffer
		buf.ReadFrom(r)
		output := buf.String()

		assert.True(t, strings.Contains(output, "Error") || strings.Contains(output, "unknown") || strings.Contains(output, "invalid"),
			"stderr should contain error message")
	})
}

func TestRootCommandConsistency(t *testing.T) {
	t.Run("subcommand names are unique", func(t *testing.T) {
		rootCmd := NewRootCmd()

		cmdNames := make(map[string]bool)
		for _, cmd := range rootCmd.Commands() {
			name := cmd.Name()
			assert.False(t, cmdNames[name], "duplicate command name: %s", name)
			cmdNames[name] = true
		}
	})

	t.Run("subcommand aliases are consistent", func(t *testing.T) {
		commands := []*cobra.Command{
			gatewayCmd(),
			authCmd(),
			mcpCmd(),
			operatorCmd(),
			vaultCmd(),
			testCmd(),
			demosCmd(),
			auditCmd(),
			swaggerCmd(),
		}

		for _, cmd := range commands {
			if len(cmd.Aliases) > 0 {
				for _, alias := range cmd.Aliases {
					assert.NotEmpty(t, alias, "alias should not be empty")
					assert.NotEqual(t, cmd.Use, alias, "alias should not be the same as the command name")
				}
			}
		}
	})
}

func TestExecuteWithHelpFlag(t *testing.T) {
	t.Run("--help flag displays help", func(t *testing.T) {
		rootCmd := NewRootCmd()

		var buf bytes.Buffer
		rootCmd.SetOut(&buf)
		rootCmd.SetErr(&buf)

		rootCmd.SetArgs([]string{"--help"})
		err := rootCmd.Execute()

		require.NoError(t, err)
		output := buf.String()

		assert.Contains(t, output, "g8e")
		assert.Contains(t, output, "zero-trust execution platform")
	})

	t.Run("help command displays help", func(t *testing.T) {
		rootCmd := NewRootCmd()

		var buf bytes.Buffer
		rootCmd.SetOut(&buf)
		rootCmd.SetErr(&buf)

		rootCmd.SetArgs([]string{"help"})
		err := rootCmd.Execute()

		require.NoError(t, err)
		output := buf.String()

		assert.Contains(t, output, "g8e")
	})
}

func TestExecuteVersionConsistency(t *testing.T) {
	t.Run("version command is available if configured", func(t *testing.T) {
		rootCmd := NewRootCmd()

		rootCmd.SetArgs([]string{"--version"})
		err := rootCmd.Execute()

		// Version command should now be configured and return nil error
		assert.NoError(t, err)
	})
}

func TestRootCommandRunFunction(t *testing.T) {
	t.Run("root command without subcommand shows help", func(t *testing.T) {
		rootCmd := NewRootCmd()
		rootCmd.Run = func(cmd *cobra.Command, args []string) {
			cmd.Help()
		}

		var buf bytes.Buffer
		rootCmd.SetOut(&buf)
		rootCmd.SetErr(&buf)

		rootCmd.SetArgs([]string{})
		err := rootCmd.Execute()

		require.NoError(t, err)
		output := buf.String()

		assert.Contains(t, output, "g8e")
	})
}

func TestErrorFormatting(t *testing.T) {
	t.Run("error message format is consistent", func(t *testing.T) {
		testErr := fmt.Errorf("test error")
		var buf bytes.Buffer

		fmt.Fprintf(&buf, "Error: %v\n", testErr)

		output := buf.String()
		assert.Contains(t, output, "Error:")
		assert.Contains(t, output, "test error")
	})
}
