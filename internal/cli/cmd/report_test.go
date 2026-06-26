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
