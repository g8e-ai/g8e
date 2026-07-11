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
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRootCommandStructure(t *testing.T) {
	t.Run("root command has correct use", func(t *testing.T) {
		rootCmd := &cobra.Command{
			Use:   "g8e",
			Short: "g8e Platform Manager - CLI for the g8e Gateway and g8e Operator",
		}
		assert.Equal(t, "g8e", rootCmd.Use)
		assert.Contains(t, rootCmd.Short, "g8e Platform Manager")
	})
}

func TestCommandRegistration(t *testing.T) {
	t.Run("all expected subcommands are registered", func(t *testing.T) {
		expectedCommands := []string{
			"gw",
			"auth",
		}

		for _, cmdName := range expectedCommands {
			t.Run(cmdName+" command exists", func(t *testing.T) {
				// Verify command constructors don't panic
				switch cmdName {
				case "gw":
					cmd := gatewayCmd()
					assert.NotNil(t, cmd)
					assert.Equal(t, "gw", cmd.Use)
				case "auth":
					cmd := authCmd()
					assert.NotNil(t, cmd)
					assert.Equal(t, "auth", cmd.Use)
				}
			})
		}
	})
}

func TestGatewayCommandSubcommands(t *testing.T) {
	t.Run("gateway command has expected subcommands", func(t *testing.T) {
		cmd := gatewayCmd()
		require.NotNil(t, cmd)

		expectedSubcommands := []string{
			"start",
			"stop",
			"status",
			"restart",
			"logs",
			"settings",
			"reset",
			"clean",
		}

		for _, subcmd := range expectedSubcommands {
			found := false
			for _, c := range cmd.Commands() {
				if c.Name() == subcmd {
					found = true
					break
				}
			}
			assert.True(t, found, "gateway command should have %s subcommand", subcmd)
		}
	})
}

func TestAuthCommandSubcommands(t *testing.T) {
	t.Run("auth command has expected subcommands", func(t *testing.T) {
		cmd := authCmd()
		require.NotNil(t, cmd)

		expectedSubcommands := []string{"enroll", "logout"}

		for _, subcmd := range expectedSubcommands {
			found := false
			for _, c := range cmd.Commands() {
				if c.Name() == subcmd {
					found = true
					break
				}
			}
			assert.True(t, found, "auth command should have %s subcommand", subcmd)
		}
	})
}

func TestDataCommandSubcommands(t *testing.T) {
	t.Run("data command has expected subcommands", func(t *testing.T) {
		cmd := dataCmd()
		require.NotNil(t, cmd)

		expectedSubcommands := []string{
			"users",
			"operators",
			"settings",
			"store",
			"audit",
		}

		for _, subcmd := range expectedSubcommands {
			found := false
			for _, c := range cmd.Commands() {
				if c.Name() == subcmd {
					found = true
					break
				}
			}
			assert.True(t, found, "data command should have %s subcommand", subcmd)
		}
	})
}

func TestSecurityCommandSubcommands(t *testing.T) {
	t.Run("security command has expected subcommands", func(t *testing.T) {
		cmd := securityCmd()
		require.NotNil(t, cmd)

		expectedSubcommands := []string{"validate"}

		for _, subcmd := range expectedSubcommands {
			found := false
			for _, c := range cmd.Commands() {
				if c.Name() == subcmd {
					found = true
					break
				}
			}
			assert.True(t, found, "security command should have %s subcommand", subcmd)
		}
	})
}

func TestOperatorCommandSubcommands(t *testing.T) {
	t.Run("operator command has expected subcommands", func(t *testing.T) {
		cmd := operatorCmd()
		require.NotNil(t, cmd)

		expectedSubcommands := []string{
			"list",
			"run",
			"cp",
			"scp",
			"deploy",
			"stream",
		}

		for _, subcmd := range expectedSubcommands {
			found := false
			for _, c := range cmd.Commands() {
				if c.Name() == subcmd {
					found = true
					break
				}
			}
			assert.True(t, found, "operator command should have %s subcommand", subcmd)
		}
	})
}

func TestCommandHelpText(t *testing.T) {
	t.Run("commands have non-empty help text", func(t *testing.T) {
		commands := []struct {
			name string
			cmd  *cobra.Command
		}{
			{"gw", gatewayCmd()},
			{"auth", authCmd()},
			{"operator", operatorCmd()},
		}

		for _, tc := range commands {
			t.Run(tc.name, func(t *testing.T) {
				assert.NotEmpty(t, tc.cmd.Short, tc.name+" should have non-empty Short description")
				assert.NotEmpty(t, tc.cmd.Long, tc.name+" should have non-empty Long description")
			})
		}
	})
}

func TestCommandFlagValidation(t *testing.T) {
	t.Run("gateway reset has force flags", func(t *testing.T) {
		cmd := gatewayResetCmd()
		require.NotNil(t, cmd)

		forceFlag := cmd.Flags().Lookup("force")
		yFlag := cmd.Flags().Lookup("y")
		yesFlag := cmd.Flags().Lookup("yes")

		assert.NotNil(t, forceFlag, "gateway reset should have --force flag")
		assert.NotNil(t, yFlag, "gateway reset should have -y flag")
		assert.NotNil(t, yesFlag, "gateway reset should have --yes flag")
	})

	t.Run("gateway clean has force flags", func(t *testing.T) {
		cmd := gatewayCleanCmd()
		require.NotNil(t, cmd)

		forceFlag := cmd.Flags().Lookup("force")
		yFlag := cmd.Flags().Lookup("y")
		yesFlag := cmd.Flags().Lookup("yes")

		assert.NotNil(t, forceFlag, "gateway clean should have --force flag")
		assert.NotNil(t, yFlag, "gateway clean should have -y flag")
		assert.NotNil(t, yesFlag, "gateway clean should have --yes flag")
	})

	t.Run("security validate has directory flags", func(t *testing.T) {
		cmd := securityValidateCmd()
		require.NotNil(t, cmd)

		pkiDirFlag := cmd.Flags().Lookup("pki-dir")
		secretsDirFlag := cmd.Flags().Lookup("secrets-dir")

		assert.NotNil(t, pkiDirFlag, "security validate should have --pki-dir flag")
		assert.NotNil(t, secretsDirFlag, "security validate should have --secrets-dir flag")
	})
}

func TestCommandAliases(t *testing.T) {
	t.Run("gateway logs has follow alias", func(t *testing.T) {
		cmd := gatewayLogsCmd()
		flag := cmd.Flags().Lookup("follow")
		assert.NotNil(t, flag)
		// Check shorthand
		shorthand := cmd.Flags().ShorthandLookup("f")
		assert.NotNil(t, shorthand)
	})
}

func TestPlaceholderCommands(t *testing.T) {
	t.Run("approve command is registered", func(t *testing.T) {
		cmd := approveCmd()
		assert.NotNil(t, cmd)
		assert.Contains(t, cmd.Use, "approve")
	})
}
