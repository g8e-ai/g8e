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

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidateIdentifier(t *testing.T) {
	t.Parallel()

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
		"__double_underscore",
		"snake_case_identifier",
		"PascalCase",
		"lowercase",
		"UPPERCASE",
		"mixed_Case_123",
		"_leading_underscore",
		"trailing_underscore_",
		"a1b2c3",
		"x",
		"X",
		"_0",
	}

	for _, name := range valid {
		name := name
		t.Run("valid/"+name, func(t *testing.T) {
			t.Parallel()
			err := ValidateIdentifier(name)
			require.NoError(t, err)
		})
	}

	invalid := []struct {
		name            string
		input           string
		wantErrContains string
	}{
		{"empty", "", "empty identifier"},
		{"leading digit", "1column", "invalid identifier"},
		{"hyphen", "col-name", "invalid identifier"},
		{"space", "col name", "invalid identifier"},
		{"dot", "table.column", "invalid identifier"},
		{"semicolon", "col;DROP TABLE", "invalid identifier"},
		{"single quote", "col'", "invalid identifier"},
		{"double quote", `col"`, "invalid identifier"},
		{"parenthesis", "col(", "invalid identifier"},
		{"asterisk", "col*", "invalid identifier"},
		{"equals", "col=val", "invalid identifier"},
		{"newline", "col\n", "invalid identifier"},
		{"tab", "col\t", "invalid identifier"},
		{"carriage return", "col\r", "invalid identifier"},
		{"backtick", "col`", "invalid identifier"},
		{"backslash", "col\\", "invalid identifier"},
		{"forward slash", "col/", "invalid identifier"},
		{"at sign", "col@", "invalid identifier"},
		{"hash", "col#", "invalid identifier"},
		{"dollar", "col$", "invalid identifier"},
		{"percent", "col%", "invalid identifier"},
		{"ampersand", "col&", "invalid identifier"},
		{"pipe", "col|", "invalid identifier"},
		{"tilde", "col~", "invalid identifier"},
		{"backtick SQL injection", "`users`", "invalid identifier"},
		{"comment start", "col--", "invalid identifier"},
		{"comment block", "col/*", "invalid identifier"},
		{"union SQL injection", "col UNION", "invalid identifier"},
		{"or SQL injection", "col OR", "invalid identifier"},
		{"and SQL injection", "col AND", "invalid identifier"},
		{"where SQL injection", "col WHERE", "invalid identifier"},
		{"select SQL injection", "col SELECT", "invalid identifier"},
		{"insert SQL injection", "col INSERT", "invalid identifier"},
		{"update SQL injection", "col UPDATE", "invalid identifier"},
		{"delete SQL injection", "col DELETE", "invalid identifier"},
		{"drop SQL injection", "col DROP", "invalid identifier"},
		{"null byte", "col\x00", "invalid identifier"},
		{"unicode", "colé", "invalid identifier"},
		{"emoji", "col😀", "invalid identifier"},
		{"bracket open", "col[", "invalid identifier"},
		{"bracket close", "col]", "invalid identifier"},
		{"brace open", "col{", "invalid identifier"},
		{"brace close", "col}", "invalid identifier"},
		{"angle bracket open", "col<", "invalid identifier"},
		{"angle bracket close", "col>", "invalid identifier"},
		{"comma", "col,", "invalid identifier"},
		{"colon", "col:", "invalid identifier"},
		{"question", "col?", "invalid identifier"},
		{"exclamation", "col!", "invalid identifier"},
		{"plus", "col+", "invalid identifier"},
		{"minus", "col-", "invalid identifier"},
		{"only digits", "12345", "invalid identifier"},
		{"starts with digit", "9field", "invalid identifier"},
		{"contains space in middle", "col umn", "invalid identifier"},
		{"multiple spaces", "col   name", "invalid identifier"},
		{"leading space", " column", "invalid identifier"},
		{"trailing space", "column ", "invalid identifier"},
	}

	for _, tc := range invalid {
		tc := tc
		t.Run("invalid/"+tc.name, func(t *testing.T) {
			t.Parallel()
			err := ValidateIdentifier(tc.input)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.wantErrContains)
		})
	}
}

func TestValidateIdentifier_SecurityEdgeCases(t *testing.T) {
	t.Parallel()

	// Test various SQL injection patterns that should be rejected
	sqlInjectionPatterns := []string{
		"'; DROP TABLE users; --",
		"' OR '1'='1",
		"1' OR '1'='1' --",
		"admin'--",
		"admin' #",
		"admin'/*",
		"x' OR 1=1 --",
		"x' UNION SELECT * FROM users --",
		"x'; EXEC xp_cmdshell('dir') --",
		"' AND 1=1 --",
		"' AND 1=2 --",
		"' OR 1=1 #",
		"' OR 'a'='a",
		"1' AND 1=1--",
		"1' AND 1=2--",
		"1' EXEC master..xp_cmdshell 'dir'--",
		"' UNION SELECT 1,2,3--",
		"' UNION SELECT NULL,NULL,NULL--",
		"' UNION SELECT @@version--",
		"' UNION SELECT user,password FROM users--",
	}

	for _, pattern := range sqlInjectionPatterns {
		pattern := pattern
		t.Run("sql_injection/"+pattern, func(t *testing.T) {
			t.Parallel()
			err := ValidateIdentifier(pattern)
			require.Error(t, err)
			assert.Contains(t, err.Error(), "invalid identifier")
		})
	}
}

func TestValidateIdentifier_LengthBoundaries(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{
			name:    "single character valid",
			input:   "a",
			wantErr: false,
		},
		{
			name:    "single underscore valid",
			input:   "_",
			wantErr: false,
		},
		{
			name:    "short identifier",
			input:   "abc",
			wantErr: false,
		},
		{
			name:    "long identifier valid",
			input:   strings.Repeat("a", 1000),
			wantErr: false,
		},
		{
			name:    "very long identifier valid",
			input:   strings.Repeat("x", 10000),
			wantErr: false,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := ValidateIdentifier(tt.input)
			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestValidateIdentifier_RegexConsistency(t *testing.T) {
	t.Parallel()

	// Property-based test: if ValidateIdentifier returns nil, the input must match the regex
	testCases := []string{
		"valid_name",
		"_private",
		"CamelCase",
		"abc123",
		"field_1",
		"x",
		"_",
		"__",
		"a1",
		"A1",
		"_1",
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc, func(t *testing.T) {
			t.Parallel()
			err := ValidateIdentifier(tc)
			if err == nil {
				assert.True(t, validIdentifierRe.MatchString(tc), "validated identifier must match regex")
			}
		})
	}
}

func TestValidateIdentifier_ErrorMessages(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		input          string
		wantExactError string
	}{
		{
			name:           "empty string exact error",
			input:          "",
			wantExactError: "validate: empty identifier",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := ValidateIdentifier(tt.input)
			require.Error(t, err)
			assert.Equal(t, tt.wantExactError, err.Error())
		})
	}
}

func TestValidateIdentifier_CharacterClasses(t *testing.T) {
	t.Parallel()

	// Test that only allowed character classes are accepted
	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{"lowercase letters only", "abcdefghijklmnopqrstuvwxyz", false},
		{"uppercase letters only", "ABCDEFGHIJKLMNOPQRSTUVWXYZ", false},
		{"digits only after first char", "a0123456789", false},
		{"underscores only", "_", false},
		{"mixed valid chars", "aBc_123_XyZ", false},
		{"special char minus", "a-b", true},
		{"special char plus", "a+b", true},
		{"special char at", "a@b", true},
		{"special char hash", "a#b", true},
		{"special char dollar", "a$b", true},
		{"special char percent", "a%b", true},
		{"special char caret", "a^b", true},
		{"special char ampersand", "a&b", true},
		{"special char asterisk", "a*b", true},
		{"special char paren", "a(b", true},
		{"special char pipe", "a|b", true},
		{"special char backslash", "a\\b", true},
		{"special char slash", "a/b", true},
		{"special char question", "a?b", true},
		{"special char exclamation", "a!b", true},
		{"special char tilde", "a~b", true},
		{"special char backtick", "a`b", true},
		{"special char quote single", "a'b", true},
		{"special char quote double", `a"b`, true},
		{"special char bracket square", "a[b", true},
		{"special char bracket curly", "a{b", true},
		{"special char angle", "a<b", true},
		{"special char comma", "a,b", true},
		{"special char colon", "a:b", true},
		{"special char semicolon", "a;b", true},
		{"special char period", "a.b", true},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := ValidateIdentifier(tt.input)
			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}
