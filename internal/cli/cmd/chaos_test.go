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

func TestChaosCmd(t *testing.T) {
	t.Run("chaos command has correct use and description", func(t *testing.T) {
		cmd := chaosCmd()
		assert.Equal(t, "chaos", cmd.Use)
		assert.Contains(t, cmd.Short, "Generate realistic governance events")
		assert.Contains(t, cmd.Long, "realistic distribution")
	})

	t.Run("chaos command has count flag", func(t *testing.T) {
		cmd := chaosCmd()
		require.NotNil(t, cmd)

		flag := cmd.Flags().Lookup("count")
		assert.NotNil(t, flag, "chaos command should have --count flag")
		assert.Equal(t, "100", flag.DefValue)
	})

	t.Run("chaos command has data-dir flag", func(t *testing.T) {
		cmd := chaosCmd()
		require.NotNil(t, cmd)

		flag := cmd.Flags().Lookup("data-dir")
		assert.NotNil(t, flag, "chaos command should have --data-dir flag")
	})

	t.Run("chaos command has pki-dir flag", func(t *testing.T) {
		cmd := chaosCmd()
		require.NotNil(t, cmd)

		flag := cmd.Flags().Lookup("pki-dir")
		assert.NotNil(t, flag, "chaos command should have --pki-dir flag")
	})

	t.Run("chaos long description mentions distribution", func(t *testing.T) {
		cmd := chaosCmd()
		assert.Contains(t, cmd.Long, "70%")
		assert.Contains(t, cmd.Long, "Good Actor")
		assert.Contains(t, cmd.Long, "20%")
		assert.Contains(t, cmd.Long, "Prompt Inj")
		assert.Contains(t, cmd.Long, "10%")
		assert.Contains(t, cmd.Long, "MitM")
	})
}
