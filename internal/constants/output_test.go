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
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestTruncatedOutputFormat(t *testing.T) {
	t.Run("formats correctly with all components", func(t *testing.T) {
		head := "first line"
		tail := "last line"
		skipped := 100
		result := fmt.Sprintf(TruncatedOutputFormat, head, skipped, tail)
		assert.Contains(t, result, head)
		assert.Contains(t, result, tail)
		assert.Contains(t, result, "100 bytes skipped")
	})

	t.Run("handles empty head", func(t *testing.T) {
		head := ""
		tail := "last line"
		skipped := 50
		result := fmt.Sprintf(TruncatedOutputFormat, head, skipped, tail)
		assert.Contains(t, result, tail)
		assert.Contains(t, result, "50 bytes skipped")
	})

	t.Run("handles empty tail", func(t *testing.T) {
		head := "first line"
		tail := ""
		skipped := 75
		result := fmt.Sprintf(TruncatedOutputFormat, head, skipped, tail)
		assert.Contains(t, result, head)
		assert.Contains(t, result, "75 bytes skipped")
	})

	t.Run("handles zero bytes skipped", func(t *testing.T) {
		head := "first line"
		tail := "last line"
		skipped := 0
		result := fmt.Sprintf(TruncatedOutputFormat, head, skipped, tail)
		assert.Contains(t, result, head)
		assert.Contains(t, result, tail)
		assert.Contains(t, result, "0 bytes skipped")
	})

	t.Run("constant is not empty", func(t *testing.T) {
		assert.NotEmpty(t, TruncatedOutputFormat)
	})

	t.Run("contains expected format markers", func(t *testing.T) {
		assert.Contains(t, TruncatedOutputFormat, "%s")
		assert.Contains(t, TruncatedOutputFormat, "%d")
		assert.Contains(t, TruncatedOutputFormat, "TRUNCATED")
		assert.Contains(t, TruncatedOutputFormat, "bytes skipped")
	})
}
