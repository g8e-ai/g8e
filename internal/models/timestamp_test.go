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
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestNowUTC(t *testing.T) {
	t.Run("returns current UTC time", func(t *testing.T) {
		before := time.Now().UTC()
		result := NowUTC()
		after := time.Now().UTC()

		assert.True(t, result.After(before) || result.Equal(before))
		assert.True(t, result.Before(after) || result.Equal(after))
	})
}

func TestFormatTimestamp(t *testing.T) {
	t.Run("formats time as RFC3339Nano", func(t *testing.T) {
		tm := time.Date(2026, 1, 2, 15, 4, 5, 123456789, time.UTC)
		result := FormatTimestamp(tm)

		assert.Equal(t, "2026-01-02T15:04:05.123456789Z", result)
	})

	t.Run("always produces Z-suffixed string", func(t *testing.T) {
		tm := time.Date(2026, 1, 2, 15, 4, 5, 0, time.UTC)
		result := FormatTimestamp(tm)

		assert.Contains(t, result, "Z")
	})
}

func TestNowTimestamp(t *testing.T) {
	t.Run("returns current UTC time as string", func(t *testing.T) {
		result := NowTimestamp()

		assert.NotEmpty(t, result)
		assert.Contains(t, result, "Z")
	})
}

func TestParseTimestamp(t *testing.T) {
	t.Run("parses valid RFC3339Nano timestamp", func(t *testing.T) {
		input := "2026-01-02T15:04:05.123456789Z"
		result, err := ParseTimestamp(input)

		assert.NoError(t, err)
		assert.Equal(t, 2026, result.Year())
		assert.Equal(t, time.January, result.Month())
		assert.Equal(t, 2, result.Day())
		assert.True(t, result.Location() == time.UTC)
	})

	t.Run("rejects empty timestamp", func(t *testing.T) {
		_, err := ParseTimestamp("")

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "empty timestamp")
	})

	t.Run("rejects invalid format", func(t *testing.T) {
		_, err := ParseTimestamp("invalid-timestamp")

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "unrecognized timestamp format")
	})

	t.Run("converts non-UTC timestamp to UTC", func(t *testing.T) {
		result, err := ParseTimestamp("2026-01-02T15:04:05+08:00")

		assert.NoError(t, err)
		assert.Equal(t, 2026, result.Year())
		assert.Equal(t, time.January, result.Month())
		assert.Equal(t, 2, result.Day())
		assert.Equal(t, 7, result.Hour()) // 15:04:05+08:00 = 07:04:05 UTC
		assert.True(t, result.Location() == time.UTC)
	})
}
