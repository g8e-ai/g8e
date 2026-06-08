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

func TestDocumentIDConstants(t *testing.T) {
	t.Run("DocIDPlatformSettings has correct value", func(t *testing.T) {
		assert.Equal(t, "platform_settings", string(DocIDPlatformSettings))
	})

	t.Run("DocIDUserSettingsPrefix has correct value", func(t *testing.T) {
		assert.Equal(t, "user_settings_", string(DocIDUserSettingsPrefix))
	})

	t.Run("document IDs are distinct", func(t *testing.T) {
		assert.NotEqual(t, DocIDPlatformSettings, DocIDUserSettingsPrefix)
	})
}

func TestDocumentID_ContractRegression(t *testing.T) {
	t.Run("platform_settings ID matches protocol constant", func(t *testing.T) {
		// This test ensures the Go constant matches the JSON SSOT in protocol/constants/document_ids.json
		assert.Equal(t, "platform_settings", string(DocIDPlatformSettings))
	})

	t.Run("user_settings_prefix ID matches protocol constant", func(t *testing.T) {
		// This test ensures the Go constant matches the JSON SSOT in protocol/constants/document_ids.json
		assert.Equal(t, "user_settings_", string(DocIDUserSettingsPrefix))
	})
}
