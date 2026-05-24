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
	"time"

	"github.com/stretchr/testify/assert"
)

func TestSuspendedTransaction(t *testing.T) {
	t.Run("creates valid suspended transaction", func(t *testing.T) {
		now := time.Now().UTC()
		expiresAt := now.Add(1 * time.Hour)
		envelope := json.RawMessage(`{"type":"test"}`)
		toolArgs := json.RawMessage(`{"arg":"value"}`)

		tx := &SuspendedTransaction{
			TransactionHash: "hash-123",
			Envelope:        envelope,
			CreatedAt:       now,
			ExpiresAt:       expiresAt,
			ToolName:        "execute_bash",
			ToolArguments:   toolArgs,
			UserID:          "user-123",
			OperatorID:      "operator-123",
		}

		assert.Equal(t, "hash-123", tx.TransactionHash)
		assert.Equal(t, "execute_bash", tx.ToolName)
		assert.Equal(t, "user-123", tx.UserID)
		assert.Equal(t, "operator-123", tx.OperatorID)
		assert.NotNil(t, tx.Envelope)
		assert.NotNil(t, tx.ToolArguments)
	})
}
