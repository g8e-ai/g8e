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

package scrubbing

import (
	"context"
	"testing"

	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/g8e-ai/g8e/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRegexScrubber_Name(t *testing.T) {
	t.Parallel()
	s := &RegexScrubber{name: "test-scrubber"}
	assert.Equal(t, "test-scrubber", s.Name())
}

func TestScrubSlice(t *testing.T) {
	t.Parallel()
	logger := testutil.NewTestLogger()
	config := &Config{Enabled: true, StrictMode: false}
	service := mustNewScrubbingService(t, context.Background(), config, logger, nil)

	input := []interface{}{
		"password=secret123",
		map[string]interface{}{"api_key": "sk-abc123"},
		[]interface{}{"nested=secret", 42},
		12345,
	}

	result := service.scrubSlice(input)
	require.Len(t, result, 4)
	assert.NotContains(t, result[0], "secret123")
	assert.IsType(t, map[string]interface{}{}, result[1])
	assert.IsType(t, []interface{}{}, result[2])
	assert.Equal(t, 12345, result[3])
}

func TestScrubSlice_Empty(t *testing.T) {
	t.Parallel()
	logger := testutil.NewTestLogger()
	service := mustNewScrubbingService(t, context.Background(), nil, logger, nil)
	result := service.scrubSlice([]interface{}{})
	assert.Empty(t, result)
}

func TestTokenKeymapHash_Empty(t *testing.T) {
	t.Parallel()
	logger := testutil.NewTestLogger()
	service := mustNewScrubbingService(t, context.Background(), nil, logger, nil)
	hash := service.TokenKeymapHash()
	assert.Equal(t, "", hash)
}

func TestTokenKeymapHash_WithTokens(t *testing.T) {
	t.Parallel()
	logger := testutil.NewTestLogger()
	config := &Config{Enabled: true, StrictMode: false}
	service := mustNewScrubbingService(t, context.Background(), config, logger, nil)

	service.GetTokenForValue(context.Background(), "secret123")
	service.GetTokenForValue(context.Background(), "sk-abc123")

	hash1 := service.TokenKeymapHash()
	assert.NotEmpty(t, hash1)

	service.GetTokenForValue(context.Background(), "xyz789")
	hash2 := service.TokenKeymapHash()
	assert.NotEmpty(t, hash2)
	assert.NotEqual(t, hash1, hash2)
}

func TestTokenKeymapHash_Deterministic(t *testing.T) {
	t.Parallel()
	logger := testutil.NewTestLogger()
	config := &Config{Enabled: true, StrictMode: false}

	service1 := mustNewScrubbingService(t, context.Background(), config, logger, nil)
	service2 := mustNewScrubbingService(t, context.Background(), config, logger, nil)

	service1.GetTokenForValue(context.Background(), "secret123")
	service2.GetTokenForValue(context.Background(), "secret123")

	assert.Equal(t, service1.TokenKeymapHash(), service2.TokenKeymapHash())
}

func TestCategorizeWarning_AllCategories(t *testing.T) {
	t.Parallel()
	logger := testutil.NewTestLogger()
	service := mustNewScrubbingService(t, context.Background(), nil, logger, nil)

	tests := []struct {
		input    string
		expected string
	}{
		{"This feature is deprecated", "deprecation_warning"},
		{"Using insecure protocol", "security_warning"},
		{"Performance issue detected", "performance_warning"},
		{"Out of memory", "memory_warning"},
		{"Disk space low", "disk_warning"},
		{"Network timeout occurred", "network"},
		{"SSL certificate expired", "certificate_warning"},
		{"TLS handshake failed", "certificate_warning"},
		{"Version mismatch detected", "version_warning"},
		{"Unknown warning type", ""},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := service.categorizeWarning(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestExtractKey_ColonSeparator(t *testing.T) {
	t.Parallel()
	logger := testutil.NewTestLogger()
	config := &Config{Enabled: true, StrictMode: false}
	service := mustNewScrubbingService(t, context.Background(), config, logger, nil)

	key := service.extractKey("API_KEY: abc123")
	assert.NotEmpty(t, key)
	assert.NotContains(t, key, "abc123")
}

func TestExtractKey_EqualsSeparator(t *testing.T) {
	t.Parallel()
	logger := testutil.NewTestLogger()
	config := &Config{Enabled: true, StrictMode: false}
	service := mustNewScrubbingService(t, context.Background(), config, logger, nil)

	key := service.extractKey("api_key=secret_value")
	assert.NotEmpty(t, key)
	assert.NotContains(t, key, "secret_value")
}

func TestExtractKey_NoSeparator(t *testing.T) {
	t.Parallel()
	logger := testutil.NewTestLogger()
	service := mustNewScrubbingService(t, context.Background(), nil, logger, nil)

	key := service.extractKey("no separator here")
	assert.Equal(t, "[KEY]", key)
}

func TestDetermineStatus_SignalCodes(t *testing.T) {
	t.Parallel()
	logger := testutil.NewTestLogger()
	service := mustNewScrubbingService(t, context.Background(), nil, logger, nil)

	tests := []struct {
		exitCode int
		expected string
	}{
		{129, string(constants.CommandExitStatusSignal1)},
		{130, string(constants.CommandExitStatusInterrupted)},
		{131, string(constants.CommandExitStatusSignal3)},
		{134, string(constants.CommandExitStatusSignal6)},
		{137, string(constants.CommandExitStatusKilled)},
		{139, string(constants.CommandExitStatusSignal11)},
		{141, string(constants.CommandExitStatusSignal13)},
		{143, string(constants.CommandExitStatusTerminated)},
		{200, string(constants.CommandExitStatusError)},
		{50, string(constants.CommandExitStatusError)},
	}

	for _, tt := range tests {
		t.Run(string(rune(tt.exitCode)), func(t *testing.T) {
			result := service.determineStatus(tt.exitCode)
			assert.Equal(t, tt.expected, string(result))
		})
	}
}

func TestRehydrateText_Empty(t *testing.T) {
	t.Parallel()
	logger := testutil.NewTestLogger()
	service := mustNewScrubbingService(t, context.Background(), nil, logger, nil)
	assert.Equal(t, "", service.RehydrateText(context.Background(), ""))
}

func TestRehydrateText_NoTokens(t *testing.T) {
	t.Parallel()
	logger := testutil.NewTestLogger()
	service := mustNewScrubbingService(t, context.Background(), nil, logger, nil)
	input := "no tokens here"
	assert.Equal(t, input, service.RehydrateText(context.Background(), input))
}
