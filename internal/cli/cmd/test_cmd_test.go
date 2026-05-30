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
	"path/filepath"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTestCmd(t *testing.T) {
	t.Run("test command has correct use and description", func(t *testing.T) {
		cmd := testCmd()
		assert.Equal(t, "test", cmd.Use)
		assert.Contains(t, cmd.Short, "Run test suites")
		assert.Contains(t, cmd.Long, "CI")
	})

	t.Run("test command has expected subcommands", func(t *testing.T) {
		cmd := testCmd()
		expectedSubcommands := []string{"unit", "integration", "chaos", "scenario", "review", "summary"}

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

func TestTestUnitCmd(t *testing.T) {
	t.Run("unit command has correct use", func(t *testing.T) {
		cmd := testUnitCmd()
		assert.Equal(t, "unit", cmd.Use)
		assert.Contains(t, cmd.Short, "unit tests")
	})

	t.Run("unit has race flag", func(t *testing.T) {
		cmd := testUnitCmd()
		flag := cmd.Flags().Lookup("race")
		assert.NotNil(t, flag)
	})

	t.Run("unit has verbose flag", func(t *testing.T) {
		cmd := testUnitCmd()
		flag := cmd.Flags().Lookup("v")
		assert.NotNil(t, flag)
	})

	t.Run("unit has run flag", func(t *testing.T) {
		cmd := testUnitCmd()
		flag := cmd.Flags().Lookup("run")
		assert.NotNil(t, flag)
	})

	t.Run("unit has coverage flag", func(t *testing.T) {
		cmd := testUnitCmd()
		flag := cmd.Flags().Lookup("coverage")
		assert.NotNil(t, flag)
	})

	t.Run("unit race flag has default value", func(t *testing.T) {
		cmd := testUnitCmd()
		raceFlag := cmd.Flags().Lookup("race")
		assert.NotNil(t, raceFlag)
		assert.Equal(t, "true", raceFlag.DefValue)
	})
}

func TestTestIntegrationCmd(t *testing.T) {
	t.Run("integration command has correct use", func(t *testing.T) {
		cmd := testIntegrationCmd()
		assert.Equal(t, "integration", cmd.Use)
		assert.Contains(t, cmd.Short, "integration tests")
	})

	t.Run("integration has run flag", func(t *testing.T) {
		cmd := testIntegrationCmd()
		flag := cmd.Flags().Lookup("run")
		assert.NotNil(t, flag)
	})
}

func TestTestG8eoCmd(t *testing.T) {
	t.Run("g8eo command has correct use", func(t *testing.T) {
		cmd := testG8eoCmd()
		assert.Equal(t, "g8eo", cmd.Use)
		assert.Contains(t, cmd.Short, "Gateway")
		assert.Contains(t, cmd.Short, "g8eo")
	})

	t.Run("g8eo has race flag", func(t *testing.T) {
		cmd := testG8eoCmd()
		flag := cmd.Flags().Lookup("race")
		assert.NotNil(t, flag)
	})

	t.Run("g8eo has verbose flag", func(t *testing.T) {
		cmd := testG8eoCmd()
		flag := cmd.Flags().Lookup("v")
		assert.NotNil(t, flag)
	})

	t.Run("g8eo has run flag", func(t *testing.T) {
		cmd := testG8eoCmd()
		flag := cmd.Flags().Lookup("run")
		assert.NotNil(t, flag)
	})

	t.Run("g8eo has coverage flag", func(t *testing.T) {
		cmd := testG8eoCmd()
		flag := cmd.Flags().Lookup("coverage")
		assert.NotNil(t, flag)
	})

	t.Run("g8eo race flag has default value", func(t *testing.T) {
		cmd := testG8eoCmd()
		raceFlag := cmd.Flags().Lookup("race")
		assert.NotNil(t, raceFlag)
		assert.Equal(t, "true", raceFlag.DefValue)
	})
}

func TestTestChaosCmd(t *testing.T) {
	t.Run("chaos command has correct use", func(t *testing.T) {
		cmd := testChaosCmd()
		assert.Equal(t, "chaos", cmd.Use)
		assert.Contains(t, cmd.Short, "chaos")
	})

	t.Run("chaos has count flag", func(t *testing.T) {
		cmd := testChaosCmd()
		flag := cmd.Flags().Lookup("count")
		assert.NotNil(t, flag)
	})

	t.Run("chaos count flag has default value", func(t *testing.T) {
		cmd := testChaosCmd()
		countFlag := cmd.Flags().Lookup("count")
		assert.NotNil(t, countFlag)
		assert.Equal(t, "0", countFlag.DefValue)
	})
}

func TestTestScenarioCmd(t *testing.T) {
	t.Run("scenario command has correct use", func(t *testing.T) {
		cmd := testScenarioCmd()
		assert.Equal(t, "scenario", cmd.Use)
		assert.Contains(t, cmd.Short, "scenario")
	})

	t.Run("scenario has run flag", func(t *testing.T) {
		cmd := testScenarioCmd()
		flag := cmd.Flags().Lookup("run")
		assert.NotNil(t, flag)
	})

	t.Run("scenario has verbose flag", func(t *testing.T) {
		cmd := testScenarioCmd()
		flag := cmd.Flags().Lookup("verbose")
		assert.NotNil(t, flag)
	})
}

func TestTestReviewCmd(t *testing.T) {
	t.Run("review command has correct use", func(t *testing.T) {
		cmd := testReviewCmd()
		assert.Equal(t, "review", cmd.Use)
		assert.Contains(t, cmd.Short, "Review")
		assert.Contains(t, cmd.Short, "test vault")
	})

	t.Run("review has list flag", func(t *testing.T) {
		cmd := testReviewCmd()
		flag := cmd.Flags().Lookup("list")
		assert.NotNil(t, flag)
	})

	t.Run("review has query flag", func(t *testing.T) {
		cmd := testReviewCmd()
		flag := cmd.Flags().Lookup("query")
		assert.NotNil(t, flag)
	})

	t.Run("review has vault-path flag", func(t *testing.T) {
		cmd := testReviewCmd()
		flag := cmd.Flags().Lookup("vault-path")
		assert.NotNil(t, flag)
	})

	t.Run("review has clean flag", func(t *testing.T) {
		cmd := testReviewCmd()
		flag := cmd.Flags().Lookup("clean")
		assert.NotNil(t, flag)
	})

	t.Run("review has clean-old flag", func(t *testing.T) {
		cmd := testReviewCmd()
		flag := cmd.Flags().Lookup("clean-old")
		assert.NotNil(t, flag)
	})
}

func TestTestSummaryCmd(t *testing.T) {
	t.Run("summary command has correct use", func(t *testing.T) {
		cmd := testSummaryCmd()
		assert.Contains(t, cmd.Use, "summary")
		assert.Contains(t, cmd.Short, "summary")
		assert.Contains(t, cmd.Short, "test results")
	})
}

func TestListVaults(t *testing.T) {
	t.Run("list vaults with no vault directory", func(t *testing.T) {
		tmpDir := t.TempDir()
		vaultDir := filepath.Join(tmpDir, "vaults")

		cmd := &cobra.Command{}
		var buf bytes.Buffer
		cmd.SetOut(&buf)
		cmd.SetErr(&buf)

		err := listVaults(vaultDir, cmd)
		assert.NoError(t, err)
		assert.Contains(t, buf.String(), "No test vaults found")
	})

	t.Run("list vaults with empty directory", func(t *testing.T) {
		tmpDir := t.TempDir()
		vaultDir := filepath.Join(tmpDir, "vaults")
		require.NoError(t, os.MkdirAll(vaultDir, 0755))

		cmd := &cobra.Command{}
		var buf bytes.Buffer
		cmd.SetOut(&buf)
		cmd.SetErr(&buf)

		err := listVaults(vaultDir, cmd)
		assert.NoError(t, err)
		assert.Contains(t, buf.String(), "No test vaults found")
	})

	t.Run("list vaults with vaults", func(t *testing.T) {
		tmpDir := t.TempDir()
		vaultDir := filepath.Join(tmpDir, "vaults")
		require.NoError(t, os.MkdirAll(vaultDir, 0755))

		// Create a test vault
		vaultPath := filepath.Join(vaultDir, "test-vault-1")
		require.NoError(t, os.MkdirAll(vaultPath, 0755))

		cmd := &cobra.Command{}
		var buf bytes.Buffer
		cmd.SetOut(&buf)
		cmd.SetErr(&buf)

		err := listVaults(vaultDir, cmd)
		assert.NoError(t, err)
		assert.Contains(t, buf.String(), "Found 1 test vault")
		assert.Contains(t, buf.String(), "test-vault-1")
	})
}

func TestCleanAllVaults(t *testing.T) {
	t.Run("clean all vaults with no vault directory", func(t *testing.T) {
		tmpDir := t.TempDir()
		vaultDir := filepath.Join(tmpDir, "vaults")

		cmd := &cobra.Command{}
		var buf bytes.Buffer
		cmd.SetOut(&buf)
		cmd.SetErr(&buf)

		err := cleanAllVaults(vaultDir, cmd)
		assert.NoError(t, err)
		assert.Contains(t, buf.String(), "No test vaults found")
	})

	t.Run("clean all vaults with vaults", func(t *testing.T) {
		tmpDir := t.TempDir()
		vaultDir := filepath.Join(tmpDir, "vaults")
		require.NoError(t, os.MkdirAll(vaultDir, 0755))

		// Create test vaults
		vault1 := filepath.Join(vaultDir, "test-vault-1")
		vault2 := filepath.Join(vaultDir, "test-vault-2")
		require.NoError(t, os.MkdirAll(vault1, 0755))
		require.NoError(t, os.MkdirAll(vault2, 0755))

		cmd := &cobra.Command{}
		var buf bytes.Buffer
		cmd.SetOut(&buf)
		cmd.SetErr(&buf)

		err := cleanAllVaults(vaultDir, cmd)
		assert.NoError(t, err)
		assert.Contains(t, buf.String(), "Removed 2 vault")

		// Verify vaults are deleted
		_, err1 := os.Stat(vault1)
		_, err2 := os.Stat(vault2)
		assert.True(t, os.IsNotExist(err1))
		assert.True(t, os.IsNotExist(err2))
	})
}

func TestCleanOldVaults(t *testing.T) {
	t.Run("clean old vaults with no vault directory", func(t *testing.T) {
		tmpDir := t.TempDir()
		vaultDir := filepath.Join(tmpDir, "vaults")

		cmd := &cobra.Command{}
		var buf bytes.Buffer
		cmd.SetOut(&buf)
		cmd.SetErr(&buf)

		err := cleanOldVaults(vaultDir, 1, cmd)
		assert.NoError(t, err)
		assert.Contains(t, buf.String(), "No test vaults found")
	})

	t.Run("clean old vaults removes only old vaults", func(t *testing.T) {
		tmpDir := t.TempDir()
		vaultDir := filepath.Join(tmpDir, "vaults")
		require.NoError(t, os.MkdirAll(vaultDir, 0755))

		// Create an old vault (we can't easily set mod time in tests, so just test structure)
		oldVault := filepath.Join(vaultDir, "old-vault")
		require.NoError(t, os.MkdirAll(oldVault, 0755))

		cmd := &cobra.Command{}
		var buf bytes.Buffer
		cmd.SetOut(&buf)
		cmd.SetErr(&buf)

		// With a very high day count, nothing should be removed
		err := cleanOldVaults(vaultDir, 365, cmd)
		assert.NoError(t, err)
		assert.Contains(t, buf.String(), "Removed 0 vault")
	})
}

func TestCountTableRows(t *testing.T) {
	t.Run("count table rows with non-existent database", func(t *testing.T) {
		tmpDir := t.TempDir()
		dbPath := filepath.Join(tmpDir, "nonexistent.db")

		count, err := countTableRows(dbPath, "test_table")
		assert.Error(t, err)
		assert.Equal(t, 0, count)
	})
}

func TestInspectVault(t *testing.T) {
	t.Run("inspect vault with non-existent database", func(t *testing.T) {
		tmpDir := t.TempDir()
		vaultPath := filepath.Join(tmpDir, "vault")

		cmd := &cobra.Command{}
		var buf bytes.Buffer
		cmd.SetOut(&buf)
		cmd.SetErr(&buf)

		err := inspectVault(vaultPath, cmd)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "vault database not found")
	})
}

func TestQueryVault(t *testing.T) {
	t.Run("query vault with non-existent database", func(t *testing.T) {
		tmpDir := t.TempDir()
		vaultPath := filepath.Join(tmpDir, "vault")

		cmd := &cobra.Command{}
		var buf bytes.Buffer
		cmd.SetOut(&buf)
		cmd.SetErr(&buf)

		err := queryVault(vaultPath, "SELECT * FROM test", cmd)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "vault database not found")
	})
}
