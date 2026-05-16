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
	"fmt"
	"time"
)

const TimestampFormat = time.RFC3339Nano

// NowUTC returns the current time in UTC. Use this for all internal *time.Time fields.
func NowUTC() time.Time {
	return time.Now().UTC()
}

// FormatTimestamp formats a time.Time as an RFC3339Nano UTC string for wire serialization.
// Always produces a Z-suffixed string (e.g. "2026-01-02T15:04:05.123456789Z").
func FormatTimestamp(t time.Time) string {
	return t.UTC().Format(TimestampFormat)
}

// NowTimestamp returns the current UTC time formatted as an RFC3339Nano string.
// Use this when a wire timestamp string field must be set at the current time.
func NowTimestamp() string {
	return FormatTimestamp(time.Now())
}

// ParseTimestamp parses an RFC3339Nano timestamp string into a UTC time.Time.
// Returns an error if the string is empty or unrecognized. Fallbacks are not allowed.
func ParseTimestamp(s string) (time.Time, error) {
	if s == "" {
		return time.Time{}, fmt.Errorf("empty timestamp string")
	}

	t, err := time.Parse(TimestampFormat, s)
	if err != nil {
		return time.Time{}, fmt.Errorf("unrecognized timestamp format: %q (expected %s)", s, TimestampFormat)
	}

	return t.UTC(), nil
}
