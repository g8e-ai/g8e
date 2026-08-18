// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package models

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHeartbeatCapabilityFlags(t *testing.T) {
	t.Run("marshals with correct JSON tags", func(t *testing.T) {
		flags := &HeartbeatCapabilityFlags{
			ExecutionVaultEnabled: true,
			GitAvailable:          true,
			LedgerMirrorEnabled:   false,
		}

		data, err := json.Marshal(flags)
		require.NoError(t, err)

		var raw map[string]interface{}
		err = json.Unmarshal(data, &raw)
		require.NoError(t, err)

		assert.Contains(t, raw, "execution_vault_enabled")
		assert.Contains(t, raw, "git_available")
		assert.Contains(t, raw, "ledger_enabled")
		assert.True(t, raw["execution_vault_enabled"].(bool))
		assert.True(t, raw["git_available"].(bool))
		assert.False(t, raw["ledger_enabled"].(bool))
	})
}
