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

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTestCmd(t *testing.T) {
	t.Run("test command has correct use and description", func(t *testing.T) {
		cmd := testCmd()
		assert.Equal(t, "test", cmd.Use)
		assert.Contains(t, cmd.Short, "Run test suites")
		assert.Contains(t, cmd.Long, "Unit tests")
	})

	t.Run("test command has expected subcommands", func(t *testing.T) {
		cmd := testCmd()
		require.NotNil(t, cmd)

		expectedSubcommands := []string{"unit", "integration", "e2e", "coverage", "lint", "agent-harness", "chaos", "summary"}
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
	t.Run("unit command has correct use and description", func(t *testing.T) {
		cmd := testUnitCmd()
		assert.Equal(t, "unit", cmd.Use)
		assert.Contains(t, cmd.Short, "Tier 1")
		assert.Contains(t, cmd.Long, "mocks")
		assert.NotNil(t, cmd.RunE)
	})
}

func TestTestIntegrationCmd(t *testing.T) {
	t.Run("integration command has correct use and description", func(t *testing.T) {
		cmd := testIntegrationCmd()
		assert.Equal(t, "integration", cmd.Use)
		assert.Contains(t, cmd.Short, "Tier 2")
		assert.Contains(t, cmd.Long, "integration")
		assert.NotNil(t, cmd.RunE)
	})
}

func TestTestE2ECmd(t *testing.T) {
	t.Run("e2e command has correct use and description", func(t *testing.T) {
		cmd := testE2ECmd()
		assert.Equal(t, "e2e", cmd.Use)
		assert.Contains(t, cmd.Short, "Tier 3")
		assert.Contains(t, cmd.Long, "running g8e gateway")
		assert.NotNil(t, cmd.RunE)
	})
}

func TestTestCoverageCmd(t *testing.T) {
	t.Run("coverage command has correct use and description", func(t *testing.T) {
		cmd := testCoverageCmd()
		assert.Equal(t, "coverage", cmd.Use)
		assert.Contains(t, cmd.Short, "coverage")
		assert.Contains(t, cmd.Long, "70%")
		assert.NotNil(t, cmd.RunE)
	})

	t.Run("coverage command has pkg flag", func(t *testing.T) {
		cmd := testCoverageCmd()
		flag := cmd.Flags().Lookup("pkg")
		assert.NotNil(t, flag)
		assert.Equal(t, "", flag.DefValue)
	})

	t.Run("coverage command has verbose flag", func(t *testing.T) {
		cmd := testCoverageCmd()
		flag := cmd.Flags().Lookup("verbose")
		assert.NotNil(t, flag)
		assert.Equal(t, "false", flag.DefValue)
	})
}

func TestTestLintCmd(t *testing.T) {
	t.Run("lint command has correct use and description", func(t *testing.T) {
		cmd := testLintCmd()
		assert.Equal(t, "lint", cmd.Use)
		assert.Contains(t, cmd.Short, "linting")
		assert.Contains(t, cmd.Long, "golangci-lint")
		assert.NotNil(t, cmd.RunE)
	})
}

func TestTestSummaryCmd(t *testing.T) {
	t.Run("summary command has correct use and description", func(t *testing.T) {
		cmd := testSummaryCmd()
		assert.Equal(t, "summary", cmd.Use)
		assert.Contains(t, cmd.Short, "chaos test summary")
		assert.Contains(t, cmd.Long, "test vault")
		assert.NotNil(t, cmd.RunE)
	})
}
