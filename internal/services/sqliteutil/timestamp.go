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

	"github.com/g8e-ai/g8e/internal/constants"
)

func FormatTimestamp(t time.Time) string {
	return t.UTC().Format(constants.TimestampFormat)
}

func NowTimestamp() string {
	return FormatTimestamp(time.Now())
}

func ParseTimestamp(s string) (time.Time, error) {
	if s == "" {
		return time.Time{}, constants.ErrTimestampParseEmpty
	}

	t, err := time.Parse(constants.TimestampFormat, s)
	if err != nil {
		return time.Time{}, fmt.Errorf("%w: %q (expected %s)", constants.ErrTimestampParseInvalidFormat, s, constants.TimestampFormat)
	}

	return t.UTC(), nil
}
