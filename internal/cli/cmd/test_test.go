// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

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

		expectedSubcommands := []string{"unit", "integration", "e2e", "coverage", "lint", "chaos", "summary"}
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
		assert.Contains(t, cmd.Long, "75%")
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
