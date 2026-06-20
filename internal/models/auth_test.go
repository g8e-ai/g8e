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

	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOperatorDocumentGo(t *testing.T) {
	t.Run("marshals JSON with default Operator type", func(t *testing.T) {
		doc := &OperatorDocumentGo{
			ID:        "operator-123",
			UserID:    "user-123",
			Component: constants.ComponentNameG8EO,
			Status:    constants.OperatorStatusActive,
			IsSlot:    true,
			Claimed:   true,
			CreatedAt: time.Now().UTC(),
			UpdatedAt: time.Now().UTC(),
		}

		data, err := json.Marshal(doc)
		require.NoError(t, err)
		assert.Contains(t, string(data), constants.OperatorTypeSystem)
	})

	t.Run("marshals JSON with explicit Operator type", func(t *testing.T) {
		doc := &OperatorDocumentGo{
			ID:           "operator-123",
			UserID:       "user-123",
			Component:    constants.ComponentNameG8EO,
			Status:       constants.OperatorStatusActive,
			OperatorType: constants.OperatorTypeSystem,
			IsSlot:       true,
			Claimed:      true,
			CreatedAt:    time.Now().UTC(),
			UpdatedAt:    time.Now().UTC(),
		}

		data, err := json.Marshal(doc)
		require.NoError(t, err)
		assert.Contains(t, string(data), constants.OperatorTypeSystem)
	})
}

func TestUser(t *testing.T) {
	t.Run("active user with empty status", func(t *testing.T) {
		user := &User{
			ID:     "user-123",
			Status: "",
		}

		assert.True(t, user.IsActive())
	})

	t.Run("inactive user", func(t *testing.T) {
		user := &User{
			ID:     "user-123",
			Status: constants.UserStatusDisabled,
		}

		assert.False(t, user.IsActive())
	})

	t.Run("nil user is inactive", func(t *testing.T) {
		var user *User
		assert.False(t, user.IsActive())
	})
}
