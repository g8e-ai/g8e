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

// Package timesvc provides the single source of truth for all timestamp
// formatting, parsing, and UTC generation used across the platform.
//
// The canonical format produces fixed 6-digit microsecond precision so that
// timestamp strings are lexicographically ordered, which is required by SQLite
// indices and audit trail ordering.  ParseTimestamp accepts any RFC3339Nano
// value (variable fractional precision and offsets) and normalizes it to UTC.
package timesvc

import (
	"fmt"
	"time"
)

// Format is the canonical timestamp format string for RFC3339 with fixed
// microsecond precision. The fixed 6-digit fractional seconds guarantee
// lexicographic ordering for SQLite indices.
const Format = "2006-01-02T15:04:05.000000Z07:00"

// FormatRFC3339 is the canonical RFC3339 timestamp format with timezone offset.
const FormatRFC3339 = "2006-01-02T15:04:05Z07:00"

// FormatTimestamp formats a time.Time as a fixed-microsecond UTC string for wire serialization.
// Always produces a Z-suffixed string (e.g. "2026-01-02T15:04:05.123456Z").
func FormatTimestamp(t time.Time) string {
	return t.UTC().Format(Format)
}

// NowTimestamp returns the current UTC time formatted as a fixed-microsecond string.
// Use this when a wire timestamp string field must be set at the current time.
func NowTimestamp() string {
	return FormatTimestamp(time.Now())
}

// ParseTimestamp parses an RFC3339Nano timestamp string into a UTC time.Time.
// Returns an error if the string is empty or unrecognized. Fallbacks are not allowed.
func ParseTimestamp(s string) (time.Time, error) {
	if s == "" {
		return time.Time{}, fmt.Errorf("timestamp: parse: empty string")
	}

	t, err := time.Parse(time.RFC3339Nano, s)
	if err != nil {
		return time.Time{}, fmt.Errorf("timestamp: parse: unrecognized format: %q (expected %s)", s, Format)
	}

	return t.UTC(), nil
}
