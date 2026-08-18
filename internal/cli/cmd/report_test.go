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

func TestReportCmd(t *testing.T) {
	t.Run("report command has correct use and description", func(t *testing.T) {
		cmd := reportCmd()
		assert.Equal(t, "report", cmd.Use)
		assert.Contains(t, cmd.Short, "CSV evidence reports")
		assert.Contains(t, cmd.Long, "deterministic CSV")
	})

	t.Run("report command has expected subcommands", func(t *testing.T) {
		cmd := reportCmd()
		require.NotNil(t, cmd)

		expectedSubcommands := []string{"all", "verify"}
		for _, subcmd := range expectedSubcommands {
			found := false
			for _, c := range cmd.Commands() {
				if c.Name() == subcmd {
					found = true
					break
				}
			}
			assert.True(t, found, "report command should have %s subcommand", subcmd)
		}
	})
}

func TestReportAllCmd(t *testing.T) {
	t.Run("all command has correct use and description", func(t *testing.T) {
		cmd := reportAllCmd()
		assert.Equal(t, "all", cmd.Use)
		assert.Contains(t, cmd.Short, "Export all stores")
		assert.NotNil(t, cmd.RunE)
	})

	t.Run("all command has expected flags", func(t *testing.T) {
		cmd := reportAllCmd()
		expectedFlags := []string{"out", "data-dir", "runtime-dir", "ledger-dir"}
		for _, flagName := range expectedFlags {
			flag := cmd.Flags().Lookup(flagName)
			assert.NotNil(t, flag, "report all should have --%s flag", flagName)
		}
	})
}

func TestReportVerifyCmd(t *testing.T) {
	t.Run("verify command has correct use and description", func(t *testing.T) {
		cmd := reportVerifyCmd()
		assert.Equal(t, "verify", cmd.Use)
		assert.Contains(t, cmd.Short, "verification")
		assert.NotNil(t, cmd.RunE)
	})

	t.Run("verify command has expected flags", func(t *testing.T) {
		cmd := reportVerifyCmd()
		expectedFlags := []string{"out", "data-dir", "runtime-dir", "ledger-dir"}
		for _, flagName := range expectedFlags {
			flag := cmd.Flags().Lookup(flagName)
			assert.NotNil(t, flag, "report verify should have --%s flag", flagName)
		}
	})
}
