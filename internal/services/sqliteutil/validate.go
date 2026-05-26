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
	"regexp"
)

// validIdentifierRe guards against SQL injection when field names must be
// interpolated into queries (e.g., json_extract paths, ORDER BY columns).
var validIdentifierRe = regexp.MustCompile(`^[a-zA-Z_][a-zA-Z0-9_]*$`)

func ValidateIdentifier(name string) error {
	if name == "" {
		return fmt.Errorf("empty identifier")
	}
	if !validIdentifierRe.MatchString(name) {
		return fmt.Errorf("invalid identifier %q: must match [a-zA-Z_][a-zA-Z0-9_]*", name)
	}
	return nil
}
