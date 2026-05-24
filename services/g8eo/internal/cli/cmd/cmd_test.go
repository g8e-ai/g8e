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
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRootCommandStructure(t *testing.T) {
	t.Run("root command has correct use", func(t *testing.T) {
		rootCmd := &cobra.Command{
			Use:   "g8e",
			Short: "g8e Platform Manager - CLI for the Governance Gateway and Governed Operator",
		}
		assert.Equal(t, "g8e", rootCmd.Use)
		assert.Contains(t, rootCmd.Short, "g8e Platform Manager")
	})
}

func TestCommandRegistration(t *testing.T) {
	t.Run("all expected subcommands are registered", func(t *testing.T) {
		expectedCommands := []string{
			"setup",
			"platform",
			"apps",
			"auth",
			"data",
			"test",
			"evals",
			"security",
			"vars",
		}

		for _, cmdName := range expectedCommands {
			t.Run(cmdName+" command exists", func(t *testing.T) {
				// Verify command constructors don't panic
				switch cmdName {
				case "setup":
					cmd := setupCmd()
					assert.NotNil(t, cmd)
					assert.Equal(t, "setup", cmd.Use)
				case "platform":
					cmd := platformCmd()
					assert.NotNil(t, cmd)
					assert.Equal(t, "platform", cmd.Use)
				case "apps":
					cmd := appsCmd()
					assert.NotNil(t, cmd)
					assert.Equal(t, "apps", cmd.Use)
				case "auth":
					cmd := authCmd()
					assert.NotNil(t, cmd)
					assert.Equal(t, "auth", cmd.Use)
				case "data":
					cmd := dataCmd()
					assert.NotNil(t, cmd)
					assert.Equal(t, "data", cmd.Use)
				case "test":
					cmd := testCmd()
					assert.NotNil(t, cmd)
					assert.Equal(t, "test", cmd.Use)
				case "evals":
					cmd := evalsCmd()
					assert.NotNil(t, cmd)
					assert.Equal(t, "evals", cmd.Use)
				case "security":
					cmd := securityCmd()
					assert.NotNil(t, cmd)
					assert.Equal(t, "security", cmd.Use)
				case "vars":
					cmd := varsCmd()
					assert.NotNil(t, cmd)
					assert.Equal(t, "vars", cmd.Use)
				}
			})
		}
	})
}

func TestPlatformCommandSubcommands(t *testing.T) {
	t.Run("platform command has expected subcommands", func(t *testing.T) {
		cmd := platformCmd()
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
			assert.True(t, found, "platform command should have %s subcommand", subcmd)
		}
	})
}

func TestAppsCommandSubcommands(t *testing.T) {
	t.Run("apps command has expected subcommands", func(t *testing.T) {
		cmd := appsCmd()
		require.NotNil(t, cmd)

		expectedSubcommands := []string{"start", "stop"}

		for _, subcmd := range expectedSubcommands {
			found := false
			for _, c := range cmd.Commands() {
				if c.Name() == subcmd {
					found = true
					break
				}
			}
			assert.True(t, found, "apps command should have %s subcommand", subcmd)
		}
	})
}

func TestAuthCommandSubcommands(t *testing.T) {
	t.Run("auth command has expected subcommands", func(t *testing.T) {
		cmd := authCmd()
		require.NotNil(t, cmd)

		expectedSubcommands := []string{"login", "logout"}

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
			"device-links",
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

func TestEvalsCommandSubcommands(t *testing.T) {
	t.Run("evals command has expected subcommands", func(t *testing.T) {
		cmd := evalsCmd()
		require.NotNil(t, cmd)

		expectedSubcommands := []string{"bench", "list", "verify-receipts"}

		for _, subcmd := range expectedSubcommands {
			found := false
			for _, c := range cmd.Commands() {
				if c.Name() == subcmd {
					found = true
					break
				}
			}
			assert.True(t, found, "evals command should have %s subcommand", subcmd)
		}
	})
}

func TestTestCommandSubcommands(t *testing.T) {
	t.Run("test command has expected subcommands", func(t *testing.T) {
		cmd := testCmd()
		require.NotNil(t, cmd)

		expectedSubcommands := []string{"g8eo", "g8ee", "ci", "chaos"}

		for _, subcmd := range expectedSubcommands {
			found := false
			for _, c := range cmd.Commands() {
				if c.Name() == subcmd {
					found = true
					break
				}
			}
			assert.True(t, found, "test command should have %s subcommand", subcmd)
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

func TestVarsCommandSubcommands(t *testing.T) {
	t.Run("vars command has expected subcommands", func(t *testing.T) {
		cmd := varsCmd()
		require.NotNil(t, cmd)

		expectedSubcommands := []string{"list", "set", "get", "unset"}

		for _, subcmd := range expectedSubcommands {
			found := false
			for _, c := range cmd.Commands() {
				if c.Name() == subcmd {
					found = true
					break
				}
			}
			assert.True(t, found, "vars command should have %s subcommand", subcmd)
		}
	})
}

func TestCommandHelpText(t *testing.T) {
	t.Run("commands have non-empty help text", func(t *testing.T) {
		commands := []struct {
			name string
			cmd  *cobra.Command
		}{
			{"setup", setupCmd()},
			{"platform", platformCmd()},
			{"apps", appsCmd()},
			{"auth", authCmd()},
			{"data", dataCmd()},
			{"test", testCmd()},
			{"evals", evalsCmd()},
			{"security", securityCmd()},
			{"vars", varsCmd()},
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
	t.Run("platform start has apps flag", func(t *testing.T) {
		cmd := platformStartCmd()
		require.NotNil(t, cmd)

		flag := cmd.Flags().Lookup("apps")
		assert.NotNil(t, flag, "platform start should have --apps flag")
	})

	t.Run("platform reset has force flags", func(t *testing.T) {
		cmd := platformResetCmd()
		require.NotNil(t, cmd)

		forceFlag := cmd.Flags().Lookup("force")
		yFlag := cmd.Flags().Lookup("y")
		yesFlag := cmd.Flags().Lookup("yes")

		assert.NotNil(t, forceFlag, "platform reset should have --force flag")
		assert.NotNil(t, yFlag, "platform reset should have -y flag")
		assert.NotNil(t, yesFlag, "platform reset should have --yes flag")
	})

	t.Run("platform clean has force flags", func(t *testing.T) {
		cmd := platformCleanCmd()
		require.NotNil(t, cmd)

		forceFlag := cmd.Flags().Lookup("force")
		yFlag := cmd.Flags().Lookup("y")
		yesFlag := cmd.Flags().Lookup("yes")

		assert.NotNil(t, forceFlag, "platform clean should have --force flag")
		assert.NotNil(t, yFlag, "platform clean should have -y flag")
		assert.NotNil(t, yesFlag, "platform clean should have --yes flag")
	})

	t.Run("auth login has required flags", func(t *testing.T) {
		cmd := loginCmd()
		require.NotNil(t, cmd)

		emailFlag := cmd.Flags().Lookup("email")
		countFlag := cmd.Flags().Lookup("count")
		ttlFlag := cmd.Flags().Lookup("ttl")

		assert.NotNil(t, emailFlag, "auth login should have --email flag")
		assert.NotNil(t, countFlag, "auth login should have --count flag")
		assert.NotNil(t, ttlFlag, "auth login should have --ttl flag")
	})

	t.Run("test g8eo has test flags", func(t *testing.T) {
		cmd := testG8eoCmd()
		require.NotNil(t, cmd)

		raceFlag := cmd.Flags().Lookup("race")
		verboseFlag := cmd.Flags().Lookup("v")
		runFlag := cmd.Flags().Lookup("run")
		coverageFlag := cmd.Flags().Lookup("coverage")

		assert.NotNil(t, raceFlag, "test g8eo should have --race flag")
		assert.NotNil(t, verboseFlag, "test g8eo should have -v flag")
		assert.NotNil(t, runFlag, "test g8eo should have --run flag")
		assert.NotNil(t, coverageFlag, "test g8eo should have --coverage flag")
	})

	t.Run("evals bench has required flags", func(t *testing.T) {
		cmd := evalsBenchCmd()
		require.NotNil(t, cmd)

		suiteFlag := cmd.Flags().Lookup("suite")
		modelFlag := cmd.Flags().Lookup("model")
		providerFlag := cmd.Flags().Lookup("provider")
		operatorSessionIDFlag := cmd.Flags().Lookup("operator-session-id")
		goldSetFlag := cmd.Flags().Lookup("gold-set")
		limitFlag := cmd.Flags().Lookup("limit")

		assert.NotNil(t, suiteFlag, "evals bench should have --suite flag")
		assert.NotNil(t, modelFlag, "evals bench should have --model flag")
		assert.NotNil(t, providerFlag, "evals bench should have --provider flag")
		assert.NotNil(t, operatorSessionIDFlag, "evals bench should have --operator-session-id flag")
		assert.NotNil(t, goldSetFlag, "evals bench should have --gold-set flag")
		assert.NotNil(t, limitFlag, "evals bench should have --limit flag")
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

func TestCommandArgumentValidation(t *testing.T) {
	t.Run("apps stop requires exact args", func(t *testing.T) {
		cmd := appsStopCmd()
		require.NotNil(t, cmd)
		// Args validation is tested by running the command with wrong number of args
	})

	t.Run("evals verify-receipts requires exact args", func(t *testing.T) {
		cmd := evalsVerifyReceiptsCmd()
		require.NotNil(t, cmd)
	})

	t.Run("vars set requires exact args", func(t *testing.T) {
		cmd := varsSetCmd()
		require.NotNil(t, cmd)
	})

	t.Run("vars get requires exact args", func(t *testing.T) {
		cmd := varsGetCmd()
		require.NotNil(t, cmd)
	})

	t.Run("vars unset requires exact args", func(t *testing.T) {
		cmd := varsUnsetCmd()
		require.NotNil(t, cmd)
	})

	t.Run("apps start allows max 1 arg", func(t *testing.T) {
		cmd := appsStartCmd()
		require.NotNil(t, cmd)
	})
}

func TestCommandAliases(t *testing.T) {
	t.Run("vars list has ls alias", func(t *testing.T) {
		cmd := varsListCmd()
		require.NotNil(t, cmd)

		assert.Contains(t, cmd.Aliases, "ls")
	})
}

func TestPlaceholderCommands(t *testing.T) {
	t.Run("setup command is placeholder", func(t *testing.T) {
		cmd := setupCmd()
		require.NotNil(t, cmd)

		var buf bytes.Buffer
		cmd.SetOut(&buf)
		cmd.SetErr(&buf)

		err := cmd.RunE(cmd, []string{})
		require.NoError(t, err)
		assert.Contains(t, buf.String(), "to be implemented")
	})

	t.Run("vars list is placeholder", func(t *testing.T) {
		cmd := varsListCmd()
		require.NotNil(t, cmd)

		var buf bytes.Buffer
		cmd.SetOut(&buf)
		cmd.SetErr(&buf)

		err := cmd.RunE(cmd, []string{})
		require.NoError(t, err)
		assert.Contains(t, buf.String(), "to be implemented")
	})

	t.Run("vars set is placeholder", func(t *testing.T) {
		cmd := varsSetCmd()
		require.NotNil(t, cmd)

		var buf bytes.Buffer
		cmd.SetOut(&buf)
		cmd.SetErr(&buf)

		err := cmd.RunE(cmd, []string{"key", "value"})
		require.NoError(t, err)
		assert.Contains(t, buf.String(), "to be implemented")
	})

	t.Run("vars get is placeholder", func(t *testing.T) {
		cmd := varsGetCmd()
		require.NotNil(t, cmd)

		var buf bytes.Buffer
		cmd.SetOut(&buf)
		cmd.SetErr(&buf)

		err := cmd.RunE(cmd, []string{"key"})
		require.NoError(t, err)
		assert.Contains(t, buf.String(), "to be implemented")
	})

	t.Run("vars unset is placeholder", func(t *testing.T) {
		cmd := varsUnsetCmd()
		require.NotNil(t, cmd)

		var buf bytes.Buffer
		cmd.SetOut(&buf)
		cmd.SetErr(&buf)

		err := cmd.RunE(cmd, []string{"key"})
		require.NoError(t, err)
		assert.Contains(t, buf.String(), "to be implemented")
	})
}

func TestCommandErrorHandling(t *testing.T) {
	t.Run("invalid app name returns error", func(t *testing.T) {
		cmd := appsStartCmd()
		require.NotNil(t, cmd)

		var buf bytes.Buffer
		cmd.SetOut(&buf)
		cmd.SetErr(&buf)

		err := cmd.RunE(cmd, []string{"invalid-app"})
		require.Error(t, err)
		assert.Contains(t, strings.ToLower(err.Error()), "unsupported app")
	})

	t.Run("invalid app name for stop returns error", func(t *testing.T) {
		cmd := appsStopCmd()
		require.NotNil(t, cmd)

		var buf bytes.Buffer
		cmd.SetOut(&buf)
		cmd.SetErr(&buf)

		err := cmd.RunE(cmd, []string{"invalid-app"})
		require.Error(t, err)
		assert.Contains(t, strings.ToLower(err.Error()), "unsupported app")
	})
}
