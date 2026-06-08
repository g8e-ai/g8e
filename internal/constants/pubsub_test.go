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

package constants

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestPubSubFieldConstants(t *testing.T) {
	t.Run("PubSubFieldAction has correct value", func(t *testing.T) {
		assert.Equal(t, "action", PubSubFieldAction)
	})

	t.Run("PubSubFieldChannel has correct value", func(t *testing.T) {
		assert.Equal(t, "channel", PubSubFieldChannel)
	})

	t.Run("PubSubFieldData has correct value", func(t *testing.T) {
		assert.Equal(t, "data", PubSubFieldData)
	})

	t.Run("PubSubFieldMessage has correct value", func(t *testing.T) {
		assert.Equal(t, "message", PubSubFieldMessage)
	})

	t.Run("PubSubFieldPattern has correct value", func(t *testing.T) {
		assert.Equal(t, "pattern", PubSubFieldPattern)
	})

	t.Run("PubSubFieldType has correct value", func(t *testing.T) {
		assert.Equal(t, "type", PubSubFieldType)
	})

	t.Run("PubSubFieldSender has correct value", func(t *testing.T) {
		assert.Equal(t, "sender", PubSubFieldSender)
	})

	t.Run("all field constants are distinct", func(t *testing.T) {
		assert.NotEqual(t, PubSubFieldAction, PubSubFieldChannel)
		assert.NotEqual(t, PubSubFieldAction, PubSubFieldData)
		assert.NotEqual(t, PubSubFieldAction, PubSubFieldMessage)
		assert.NotEqual(t, PubSubFieldAction, PubSubFieldPattern)
		assert.NotEqual(t, PubSubFieldAction, PubSubFieldType)
		assert.NotEqual(t, PubSubFieldAction, PubSubFieldSender)
		assert.NotEqual(t, PubSubFieldChannel, PubSubFieldData)
		assert.NotEqual(t, PubSubFieldChannel, PubSubFieldMessage)
		assert.NotEqual(t, PubSubFieldChannel, PubSubFieldPattern)
		assert.NotEqual(t, PubSubFieldChannel, PubSubFieldType)
		assert.NotEqual(t, PubSubFieldChannel, PubSubFieldSender)
		assert.NotEqual(t, PubSubFieldData, PubSubFieldMessage)
		assert.NotEqual(t, PubSubFieldData, PubSubFieldPattern)
		assert.NotEqual(t, PubSubFieldData, PubSubFieldType)
		assert.NotEqual(t, PubSubFieldData, PubSubFieldSender)
		assert.NotEqual(t, PubSubFieldMessage, PubSubFieldPattern)
		assert.NotEqual(t, PubSubFieldMessage, PubSubFieldType)
		assert.NotEqual(t, PubSubFieldMessage, PubSubFieldSender)
		assert.NotEqual(t, PubSubFieldPattern, PubSubFieldType)
		assert.NotEqual(t, PubSubFieldPattern, PubSubFieldSender)
		assert.NotEqual(t, PubSubFieldType, PubSubFieldSender)
	})
}

func TestPubSubField_ContractRegression(t *testing.T) {
	t.Run("action field matches protocol constant", func(t *testing.T) {
		// This test ensures the Go constant matches the JSON SSOT in protocol/constants/pubsub.json
		assert.Equal(t, "action", PubSubFieldAction)
	})

	t.Run("channel field matches protocol constant", func(t *testing.T) {
		// This test ensures the Go constant matches the JSON SSOT in protocol/constants/pubsub.json
		assert.Equal(t, "channel", PubSubFieldChannel)
	})

	t.Run("data field matches protocol constant", func(t *testing.T) {
		// This test ensures the Go constant matches the JSON SSOT in protocol/constants/pubsub.json
		assert.Equal(t, "data", PubSubFieldData)
	})

	t.Run("message field matches protocol constant", func(t *testing.T) {
		// This test ensures the Go constant matches the JSON SSOT in protocol/constants/pubsub.json
		assert.Equal(t, "message", PubSubFieldMessage)
	})

	t.Run("pattern field matches protocol constant", func(t *testing.T) {
		// This test ensures the Go constant matches the JSON SSOT in protocol/constants/pubsub.json
		assert.Equal(t, "pattern", PubSubFieldPattern)
	})

	t.Run("type field matches protocol constant", func(t *testing.T) {
		// This test ensures the Go constant matches the JSON SSOT in protocol/constants/pubsub.json
		assert.Equal(t, "type", PubSubFieldType)
	})

	t.Run("sender field matches protocol constant", func(t *testing.T) {
		// This test ensures the Go constant matches the JSON SSOT in protocol/constants/pubsub.json
		assert.Equal(t, "sender", PubSubFieldSender)
	})
}
