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
