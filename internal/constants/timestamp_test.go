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

func TestFormatRFC3339(t *testing.T) {
	t.Run("has correct value", func(t *testing.T) {
		assert.Equal(t, "2006-01-02T15:04:05Z07:00", FormatRFC3339)
	})

	t.Run("is not empty", func(t *testing.T) {
		assert.NotEmpty(t, FormatRFC3339)
	})

	t.Run("contains expected RFC3339 components", func(t *testing.T) {
		assert.Contains(t, FormatRFC3339, "2006")
		assert.Contains(t, FormatRFC3339, "01")
		assert.Contains(t, FormatRFC3339, "02")
		assert.Contains(t, FormatRFC3339, "15")
		assert.Contains(t, FormatRFC3339, "04")
		assert.Contains(t, FormatRFC3339, "05")
		assert.Contains(t, FormatRFC3339, "Z07:00")
	})
}
