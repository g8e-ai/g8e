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
	"fmt"
	"time"
)

const (
	TimestampFormat = time.RFC3339Nano
)

func FormatTimestamp(t time.Time) string {
	return t.UTC().Format(TimestampFormat)
}

func NowTimestamp() string {
	return FormatTimestamp(time.Now())
}

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
