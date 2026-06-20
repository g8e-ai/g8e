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

func TestDocument(t *testing.T) {
	t.Run("forWire serializes document correctly", func(t *testing.T) {
		now := time.Now().UTC()
		data := map[string]json.RawMessage{
			"name": json.RawMessage(`"test"`),
		}

		doc := &Document{
			ID:         "doc-123",
			Collection: "test_collection",
			Data:       data,
			CreatedAt:  now,
			UpdatedAt:  now,
		}

		wire := doc.ForWire()

		assert.Contains(t, wire, "id")
		assert.Contains(t, wire, "created_at")
		assert.Contains(t, wire, "updated_at")
		assert.Contains(t, wire, "name")
	})
}
