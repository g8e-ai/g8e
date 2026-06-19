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
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/internal/constants"
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
		{"empty", "", constants.ErrSQLiteValidateEmptyIdentifier.Error()},
		{"leading digit", "1column", constants.ErrSQLiteValidateInvalidPattern.Error()},
		{"hyphen", "col-name", constants.ErrSQLiteValidateInvalidPattern.Error()},
		{"space", "col name", constants.ErrSQLiteValidateInvalidPattern.Error()},
		{"dot", "table.column", constants.ErrSQLiteValidateInvalidPattern.Error()},
		{"semicolon", "col;DROP TABLE", constants.ErrSQLiteValidateInvalidPattern.Error()},
		{"single quote", "col'", constants.ErrSQLiteValidateInvalidPattern.Error()},
		{"double quote", `col"`, constants.ErrSQLiteValidateInvalidPattern.Error()},
		{"parenthesis", "col(", constants.ErrSQLiteValidateInvalidPattern.Error()},
		{"asterisk", "col*", constants.ErrSQLiteValidateInvalidPattern.Error()},
		{"equals", "col=val", constants.ErrSQLiteValidateInvalidPattern.Error()},
		{"newline", "col\n", constants.ErrSQLiteValidateInvalidPattern.Error()},
		{"tab", "col\t", constants.ErrSQLiteValidateInvalidPattern.Error()},
		{"carriage return", "col\r", constants.ErrSQLiteValidateInvalidPattern.Error()},
		{"backtick", "col`", constants.ErrSQLiteValidateInvalidPattern.Error()},
		{"backslash", "col\\", constants.ErrSQLiteValidateInvalidPattern.Error()},
		{"forward slash", "col/", constants.ErrSQLiteValidateInvalidPattern.Error()},
		{"at sign", "col@", constants.ErrSQLiteValidateInvalidPattern.Error()},
		{"hash", "col#", constants.ErrSQLiteValidateInvalidPattern.Error()},
		{"dollar", "col$", constants.ErrSQLiteValidateInvalidPattern.Error()},
		{"percent", "col%", constants.ErrSQLiteValidateInvalidPattern.Error()},
		{"ampersand", "col&", constants.ErrSQLiteValidateInvalidPattern.Error()},
		{"pipe", "col|", constants.ErrSQLiteValidateInvalidPattern.Error()},
		{"tilde", "col~", constants.ErrSQLiteValidateInvalidPattern.Error()},
		{"backtick SQL injection", "`users`", constants.ErrSQLiteValidateInvalidPattern.Error()},
		{"comment start", "col--", constants.ErrSQLiteValidateInvalidPattern.Error()},
		{"comment block", "col/*", constants.ErrSQLiteValidateInvalidPattern.Error()},
		{"union SQL injection", "col UNION", constants.ErrSQLiteValidateInvalidPattern.Error()},
		{"or SQL injection", "col OR", constants.ErrSQLiteValidateInvalidPattern.Error()},
		{"and SQL injection", "col AND", constants.ErrSQLiteValidateInvalidPattern.Error()},
		{"where SQL injection", "col WHERE", constants.ErrSQLiteValidateInvalidPattern.Error()},
		{"select SQL injection", "col SELECT", constants.ErrSQLiteValidateInvalidPattern.Error()},
		{"insert SQL injection", "col INSERT", constants.ErrSQLiteValidateInvalidPattern.Error()},
		{"update SQL injection", "col UPDATE", constants.ErrSQLiteValidateInvalidPattern.Error()},
		{"delete SQL injection", "col DELETE", constants.ErrSQLiteValidateInvalidPattern.Error()},
		{"drop SQL injection", "col DROP", constants.ErrSQLiteValidateInvalidPattern.Error()},
		{"null byte", "col\x00", constants.ErrSQLiteValidateInvalidPattern.Error()},
		{"unicode", "colé", constants.ErrSQLiteValidateInvalidPattern.Error()},
		{"emoji", "col😀", constants.ErrSQLiteValidateInvalidPattern.Error()},
		{"bracket open", "col[", constants.ErrSQLiteValidateInvalidPattern.Error()},
		{"bracket close", "col]", constants.ErrSQLiteValidateInvalidPattern.Error()},
		{"brace open", "col{", constants.ErrSQLiteValidateInvalidPattern.Error()},
		{"brace close", "col}", constants.ErrSQLiteValidateInvalidPattern.Error()},
		{"angle bracket open", "col<", constants.ErrSQLiteValidateInvalidPattern.Error()},
		{"angle bracket close", "col>", constants.ErrSQLiteValidateInvalidPattern.Error()},
		{"comma", "col,", constants.ErrSQLiteValidateInvalidPattern.Error()},
		{"colon", "col:", constants.ErrSQLiteValidateInvalidPattern.Error()},
		{"question", "col?", constants.ErrSQLiteValidateInvalidPattern.Error()},
		{"exclamation", "col!", constants.ErrSQLiteValidateInvalidPattern.Error()},
		{"plus", "col+", constants.ErrSQLiteValidateInvalidPattern.Error()},
		{"minus", "col-", constants.ErrSQLiteValidateInvalidPattern.Error()},
		{"only digits", "12345", constants.ErrSQLiteValidateInvalidPattern.Error()},
		{"starts with digit", "9field", constants.ErrSQLiteValidateInvalidPattern.Error()},
		{"contains space in middle", "col umn", constants.ErrSQLiteValidateInvalidPattern.Error()},
		{"multiple spaces", "col   name", constants.ErrSQLiteValidateInvalidPattern.Error()},
		{"leading space", " column", constants.ErrSQLiteValidateInvalidPattern.Error()},
		{"trailing space", "column ", constants.ErrSQLiteValidateInvalidPattern.Error()},
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
			assert.Contains(t, err.Error(), constants.ErrSQLiteValidateInvalidPattern.Error())
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
			wantExactError: fmt.Sprintf("sqliteutil: validate identifier: %s", constants.ErrSQLiteValidateEmptyIdentifier.Error()),
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
