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

package sqliteutil

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFormatTimestamp_NormalizesToUTC(t *testing.T) {
	loc, err := time.LoadLocation("America/New_York")
	require.NoError(t, err)

	eastern := time.Date(2025, 6, 15, 12, 0, 0, 0, loc)
	result := FormatTimestamp(eastern)

	parsed, err := time.Parse(TimestampFormat, result)
	require.NoError(t, err)
	assert.Equal(t, time.UTC, parsed.Location())
	assert.True(t, eastern.Equal(parsed))
}

func TestFormatTimestamp_RFC3339Nano(t *testing.T) {
	ts := time.Date(2025, 1, 2, 3, 4, 5, 123456789, time.UTC)
	result := FormatTimestamp(ts)

	assert.Equal(t, "2025-01-02T03:04:05.123456789Z", result)
}

func TestFormatTimestamp_AlreadyUTC(t *testing.T) {
	ts := time.Date(2025, 3, 10, 8, 30, 0, 0, time.UTC)
	assert.Equal(t, "2025-03-10T08:30:00Z", FormatTimestamp(ts))
}

func TestNowTimestamp_IsUTCRFC3339Nano(t *testing.T) {
	before := time.Now().UTC()
	result := NowTimestamp()
	after := time.Now().UTC()

	parsed, err := time.Parse(TimestampFormat, result)
	require.NoError(t, err)
	assert.True(t, !parsed.Before(before) && !parsed.After(after))
	assert.True(t, strings.HasSuffix(result, "Z"), "expected UTC suffix 'Z', got %q", result)
}

func TestParseTimestamp_CanonicalFormat(t *testing.T) {
	ts := time.Date(2025, 6, 15, 10, 20, 30, 999000000, time.UTC)
	formatted := FormatTimestamp(ts)

	parsed, err := ParseTimestamp(formatted)
	require.NoError(t, err)
	assert.True(t, ts.Equal(parsed))
}

func TestParseTimestamp_EmptyString(t *testing.T) {
	_, err := ParseTimestamp("")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "empty timestamp string")
}

func TestParseTimestamp_UnrecognizedFormat(t *testing.T) {
	_, err := ParseTimestamp("not-a-timestamp")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unrecognized timestamp format")
}

func TestFormatParseRoundTrip(t *testing.T) {
	original := time.Date(2025, 12, 31, 23, 59, 59, 987654321, time.UTC)
	formatted := FormatTimestamp(original)

	parsed, err := ParseTimestamp(formatted)
	require.NoError(t, err)
	assert.True(t, original.Equal(parsed), "round-trip mismatch: %v != %v", original, parsed)
}

func TestFormatTimestamp_ZSuffixNotOffsetNotation(t *testing.T) {
	ts := time.Date(2026, 3, 3, 19, 5, 0, 0, time.UTC)
	result := FormatTimestamp(ts)

	assert.True(t, strings.HasSuffix(result, "Z"), "expected Z suffix, got %q", result)
	assert.NotContains(t, result, "+00:00", "must not use offset notation")
	assert.NotContains(t, result, "-00:00", "must not use offset notation")
}

func TestFormatTimestamp_WholeSectionOmitsFractional(t *testing.T) {
	ts := time.Date(2026, 3, 3, 19, 5, 0, 0, time.UTC)
	result := FormatTimestamp(ts)
	assert.Equal(t, "2026-03-03T19:05:00Z", result)
}

func TestNowTimestamp_NoOffsetNotation(t *testing.T) {
	result := NowTimestamp()
	assert.NotContains(t, result, "+", "NowTimestamp must not contain offset notation")
	assert.True(t, strings.HasSuffix(result, "Z"), "NowTimestamp must end with Z")
}

func TestNowTimestamp_LexicographicOrderIsChronological(t *testing.T) {
	a := NowTimestamp()
	b := NowTimestamp()
	assert.True(t, a <= b, "timestamps must be lexicographically non-decreasing: a=%q b=%q", a, b)
}

func TestFormatTimestamp_ParseableByStdlibRFC3339Nano(t *testing.T) {
	ts := time.Date(2026, 6, 15, 10, 20, 30, 123456789, time.UTC)
	formatted := FormatTimestamp(ts)

	parsed, err := time.Parse(time.RFC3339Nano, formatted)
	require.NoError(t, err, "output must be parseable by time.RFC3339Nano")
	assert.True(t, ts.Equal(parsed))
}

func TestParseTimestamp_AcceptsMillisecondPrecision(t *testing.T) {
	s := "2026-03-03T19:05:00.123Z"
	parsed, err := ParseTimestamp(s)
	require.NoError(t, err)
	assert.Equal(t, time.UTC, parsed.Location())
	assert.Equal(t, 2026, parsed.Year())
	assert.Equal(t, 123000000, parsed.Nanosecond())
}

func TestParseTimestamp_AcceptsMicrosecondPrecision(t *testing.T) {
	s := "2026-03-03T19:05:00.123456Z"
	parsed, err := ParseTimestamp(s)
	require.NoError(t, err)
	assert.Equal(t, time.UTC, parsed.Location())
	assert.Equal(t, 123456000, parsed.Nanosecond())
}

func TestParseTimestamp_AcceptsNanosecondPrecision(t *testing.T) {
	s := "2026-03-03T19:05:00.123456789Z"
	parsed, err := ParseTimestamp(s)
	require.NoError(t, err)
	assert.Equal(t, time.UTC, parsed.Location())
	assert.Equal(t, 123456789, parsed.Nanosecond())
}
