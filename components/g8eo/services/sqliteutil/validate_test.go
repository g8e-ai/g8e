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
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidateIdentifier(t *testing.T) {
	valid := []string{
		"column",
		"column_name",
		"_private",
		"CamelCase",
		"UPPER",
		"a",
		"_",
		"abc123",
		"field_1",
	}

	for _, name := range valid {
		t.Run("valid/"+name, func(t *testing.T) {
			err := ValidateIdentifier(name)
			require.NoError(t, err)
		})
	}

	invalid := []struct {
		name  string
		input string
	}{
		{"empty", ""},
		{"leading digit", "1column"},
		{"hyphen", "col-name"},
		{"space", "col name"},
		{"dot", "table.column"},
		{"semicolon", "col;DROP TABLE"},
		{"single quote", "col'"},
		{"double quote", `col"`},
		{"parenthesis", "col("},
		{"asterisk", "col*"},
		{"equals", "col=val"},
		{"newline", "col\n"},
	}

	for _, tc := range invalid {
		t.Run("invalid/"+tc.name, func(t *testing.T) {
			err := ValidateIdentifier(tc.input)
			require.Error(t, err)
			if tc.input == "" {
				assert.Equal(t, "empty identifier", err.Error())
			} else {
				assert.Contains(t, err.Error(), "invalid identifier")
			}
		})
	}
}
