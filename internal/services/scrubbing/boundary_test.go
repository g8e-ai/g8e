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
	"crypto/ed25519"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/g8e-ai/g8e/internal/services/storage"
	"github.com/g8e-ai/g8e/internal/services/vault"
	"github.com/g8e-ai/g8e/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDefaultConfig(t *testing.T) {
	t.Parallel()
	config := DefaultConfig()

	assert.NotNil(t, config)
	assert.True(t, config.Enabled, "should be enabled by default for safety")
	assert.True(t, config.StrictMode, "strict mode should be on by default")
	assert.Equal(t, 4096, config.MaxOutputLength)
	assert.Empty(t, config.AllowedPatterns)
	assert.Empty(t, config.CustomScrubPatterns)
}

func TestNewScrubbingService(t *testing.T) {
	t.Parallel()
	logger := testutil.NewTestLogger()

	t.Run("with nil config uses defaults", func(t *testing.T) {
		t.Parallel()
		service := NewScrubbingService(nil, logger, nil)
		require.NotNil(t, service)
		assert.True(t, service.config.StrictMode)
		assert.NotEmpty(t, service.scrubbers)
	})

	t.Run("with custom config", func(t *testing.T) {
		t.Parallel()
		config := &Config{
			StrictMode:      false,
			MaxOutputLength: 1024,
		}
		service := NewScrubbingService(config, logger, nil)
		require.NotNil(t, service)
		assert.False(t, service.config.StrictMode)
		assert.Equal(t, 1024, service.config.MaxOutputLength)
	})

	t.Run("initializes scrubbers", func(t *testing.T) {
		t.Parallel()
		service := NewScrubbingService(nil, logger, nil)
		assert.Greater(t, len(service.scrubbers), 10, "should have many scrubbers")
	})
}

func TestScrubbingService_ScrubText_IPv4_Preserved(t *testing.T) {
	t.Parallel()
	logger := testutil.NewTestLogger()
	config := &Config{Enabled: true, StrictMode: false}
	service := NewScrubbingService(config, logger, nil)

	// IPs are preserved (not scrubbed) - the AI needs them for troubleshooting
	tests := []struct {
		input    string
		expected string
	}{
		{"Server at 192.168.1.1", "Server at 192.168.1.1"},
		{"Connect to 10.0.0.1:5432", "Connect to 10.0.0.1:5432"},
		{"IPs: 172.16.0.1 and 8.8.8.8", "IPs: 172.16.0.1 and 8.8.8.8"},
		{"No IP here", "No IP here"},
	}

	for _, tt := range tests {
		result := service.ScrubText(tt.input)
		assert.Equal(t, tt.expected, result, "Input: %s", tt.input)
	}
}

func TestScrubbingService_ScrubText_Email(t *testing.T) {
	t.Parallel()
	logger := testutil.NewTestLogger()
	config := &Config{Enabled: true, StrictMode: false}
	service := NewScrubbingService(config, logger, nil)

	tests := []struct {
		input    string
		expected string
	}{
		{"Contact admin@example.com", "Contact [EMAIL]"},
		{"Email: user.name+tag@domain.org", "Email: [EMAIL]"},
		{"Multiple: a@b.com and c@d.net", "Multiple: [EMAIL] and [EMAIL]"},
	}

	for _, tt := range tests {
		result := service.ScrubText(tt.input)
		assert.Equal(t, tt.expected, result, "Input: %s", tt.input)
	}
}

func TestScrubbingService_ScrubText_UUID_Preserved(t *testing.T) {
	t.Parallel()
	logger := testutil.NewTestLogger()
	config := &Config{Enabled: true, StrictMode: false}
	service := NewScrubbingService(config, logger, nil)

	// UUIDs are preserved (not scrubbed) - the AI needs them for log/resource correlation
	tests := []struct {
		input    string
		expected string
	}{
		{"ID: 550e8400-e29b-41d4-a716-446655440000", "ID: 550e8400-e29b-41d4-a716-446655440000"},
		{"User 123e4567-e89b-12d3-a456-426614174000 created", "User 123e4567-e89b-12d3-a456-426614174000 created"},
	}

	for _, tt := range tests {
		result := service.ScrubText(tt.input)
		assert.Equal(t, tt.expected, result, "Input: %s", tt.input)
	}
}

func TestScrubbingService_ScrubText_FilePaths_Preserved(t *testing.T) {
	t.Parallel()
	logger := testutil.NewTestLogger()
	config := &Config{Enabled: true, StrictMode: false}
	service := NewScrubbingService(config, logger, nil)

	// File paths are preserved (not scrubbed) - the AI needs them for troubleshooting
	tests := []struct {
		input    string
		expected string
	}{
		{"File at /home/user/secret.txt", "File at /home/user/secret.txt"},
		{"Reading /etc/passwd", "Reading /etc/passwd"},
		{"Log: /var/log/app/debug.log", "Log: /var/log/app/debug.log"},
	}

	for _, tt := range tests {
		result := service.ScrubText(tt.input)
		assert.Equal(t, tt.expected, result, "Input: %s", tt.input)
	}
}

func TestScrubbingService_ScrubText_Credentials(t *testing.T) {
	t.Parallel()
	logger := testutil.NewTestLogger()
	config := &Config{Enabled: true, StrictMode: false}
	service := NewScrubbingService(config, logger, nil)

	tests := []struct {
		input       string
		shouldMatch string
	}{
		{"password=secretvalue123", "[CREDENTIAL_REFERENCE]"},
		{"api_key: xoxb-123456789012-1234567890123-abcdef", "[CREDENTIAL_REFERENCE]"},
		{"token=eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9", "[CREDENTIAL_REFERENCE]"},
	}

	for _, tt := range tests {
		result := service.ScrubText(tt.input)
		assert.Contains(t, result, tt.shouldMatch, "Input: %s", tt.input)
		assert.NotContains(t, result, "secretvalue", "Should not contain secret")
	}
}

func TestScrubbingService_ScrubText_PII(t *testing.T) {
	t.Parallel()
	logger := testutil.NewTestLogger()
	config := &Config{Enabled: true, StrictMode: false}
	service := NewScrubbingService(config, logger, nil)

	tests := []struct {
		name     string
		input    string
		contains string
	}{
		{"SSN", "SSN: 123-45-6789", "[PII]"},
		{"Credit Card", "Card: 4111-1111-1111-1111", "[PII]"},
		{"Phone", "Call 555-123-4567", "[PHONE]"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			result := service.ScrubText(tt.input)
			assert.Contains(t, result, tt.contains)
		})
	}
}

func TestScrubbingService_ScrubText_AWSResources_Preserved(t *testing.T) {
	t.Parallel()
	logger := testutil.NewTestLogger()
	config := &Config{Enabled: true, StrictMode: false}
	service := NewScrubbingService(config, logger, nil)

	// AWS ARNs and account IDs are preserved (not scrubbed) - the AI needs them for cloud troubleshooting
	tests := []struct {
		input    string
		expected string
	}{
		{"arn:aws:s3:::my-bucket/path", "arn:aws:s3:::my-bucket/path"},
		{"arn:aws:iam::123456789012:role/MyRole", "arn:aws:iam::123456789012:role/MyRole"},
		{"Account 123456789012 created", "Account 123456789012 created"},
	}

	for _, tt := range tests {
		result := service.ScrubText(tt.input)
		assert.Equal(t, tt.expected, result, "Input: %s", tt.input)
	}
}

func TestScrubbingService_ScrubText_ConnectionStrings(t *testing.T) {
	t.Parallel()
	logger := testutil.NewTestLogger()
	config := &Config{Enabled: true, StrictMode: false}
	service := NewScrubbingService(config, logger, nil)

	tests := []struct {
		input    string
		contains string
	}{
		{"mysql://user:pass@host:3306/db", "[CONN_STRING]"},
		{"postgres://admin:secret@localhost/mydb", "[CONN_STRING]"},
		{"mongodb://cluster.example.com:27017", "[CONN_STRING]"},
		{"redis://default:password@redis.io:6379", "[CONN_STRING]"},
	}

	for _, tt := range tests {
		result := service.ScrubText(tt.input)
		assert.Contains(t, result, tt.contains, "Input: %s", tt.input)
		assert.NotContains(t, result, "password", "Should not contain password")
	}
}

func TestScrubbingService_ScrubText_PrivateKeys(t *testing.T) {
	t.Parallel()
	logger := testutil.NewTestLogger()
	config := &Config{Enabled: true, StrictMode: false}
	service := NewScrubbingService(config, logger, nil)

	input := `-----BEGIN RSA PRIVATE KEY-----
MIIEpAIBAAKCAQEA0Z3VS5JJcds3xfn/ygWyF8PbnGy...
-----END RSA PRIVATE KEY-----`

	result := service.ScrubText(input)
	assert.Equal(t, "[PRIVATE_KEY]", result)
	assert.NotContains(t, result, "MIIEpAIBAAKCAQEA")
}

func TestScrubbingService_ScrubText_Disabled(t *testing.T) {
	t.Parallel()
	logger := testutil.NewTestLogger()
	config := &Config{Enabled: false}
	service := NewScrubbingService(config, logger, nil)

	result := service.ScrubText("Sensitive data: 192.168.1.1")
	assert.Equal(t, "[OUTPUT_SUPPRESSED]", result)
}

func TestScrubbingService_DetermineStatus(t *testing.T) {
	t.Parallel()
	logger := testutil.NewTestLogger()
	service := NewScrubbingService(nil, logger, nil)

	tests := []struct {
		exitCode int
		expected constants.SentinelStatus
	}{
		{0, constants.SentinelStatusSuccess},
		{1, constants.SentinelStatusFailure},
		{2, constants.SentinelStatusMisuse},
		{126, constants.SentinelStatusNotExecutable},
		{127, constants.SentinelStatusNotFound},
		{130, constants.SentinelStatusInterrupted},
		{137, constants.SentinelStatusKilled},
		{143, constants.SentinelStatusTerminated},
		{139, constants.SentinelStatus("signal_11")}, // SIGSEGV
	}

	for _, tt := range tests {
		result := service.determineStatus(tt.exitCode)
		assert.Equal(t, tt.expected, result, "Exit code: %d", tt.exitCode)
	}
}

func TestScrubbingService_CategorizeError(t *testing.T) {
	t.Parallel()
	logger := testutil.NewTestLogger()
	service := NewScrubbingService(nil, logger, nil)

	tests := []struct {
		stderr   string
		exitCode int
		expected string
	}{
		{"", 0, ""},
		{"Permission denied", 1, "permission_denied"},
		{"No such file or directory", 1, "not_found"},
		{"Connection refused", 1, "connection_refused"},
		{"Connection timed out", 1, "timeout"},
		{"Out of memory", 1, "out_of_memory"},
		{"No space left on device", 1, "disk_full"},
		{"Authentication failed", 1, "authentication_failed"},
		{"Syntax error near", 1, "syntax_error"},
		{"Invalid argument", 1, "invalid_input"},
		{"File already exists", 1, "already_exists"},
		{"Resource busy", 1, "resource_busy"},
		{"Quota exceeded", 1, "quota_exceeded"},
		{"Unknown error occurred", 1, "unknown_error"},
	}

	for _, tt := range tests {
		result := service.categorizeError(tt.stderr, tt.exitCode)
		assert.Equal(t, tt.expected, result, "Stderr: %s", tt.stderr)
	}
}

func TestScrubbingService_ScrubCommandResult(t *testing.T) {
	t.Parallel()
	logger := testutil.NewTestLogger()
	service := NewScrubbingService(nil, logger, nil)

	t.Run("successful command", func(t *testing.T) {
		t.Parallel()
		result := &CommandResult{
			Command:    "SELECT * FROM users",
			ExitCode:   0,
			Stdout:     "id,name,email\n1,John,john@example.com\n2,Jane,jane@test.org",
			Stderr:     "",
			DurationMs: 150,
		}

		scrubbed := service.ScrubCommandResult(result)

		assert.Equal(t, constants.SentinelStatusSuccess, scrubbed.Status)
		assert.Equal(t, 0, scrubbed.ExitCode)
		assert.Equal(t, int64(150), scrubbed.DurationMs)
		assert.Greater(t, scrubbed.OutputLines, 0)
		assert.NotNil(t, scrubbed.RowCount)
		assert.Empty(t, scrubbed.ErrorType)
		assert.NotContains(t, scrubbed.Summary, "john@example.com")
	})

	t.Run("failed command", func(t *testing.T) {
		t.Parallel()
		result := &CommandResult{
			Command:    "cat /etc/shadow",
			ExitCode:   1,
			Stdout:     "",
			Stderr:     "Permission denied",
			DurationMs: 5,
		}

		scrubbed := service.ScrubCommandResult(result)

		assert.Equal(t, constants.SentinelStatusFailure, scrubbed.Status)
		assert.Equal(t, 1, scrubbed.ExitCode)
		assert.Equal(t, "permission_denied", scrubbed.ErrorType)
	})

	t.Run("command with warnings", func(t *testing.T) {
		t.Parallel()
		result := &CommandResult{
			Command:    "npm install",
			ExitCode:   0,
			Stdout:     "installed 100 packages",
			Stderr:     "npm WARN deprecated package@1.0.0\nnpm WARN insecure connection",
			DurationMs: 5000,
		}

		scrubbed := service.ScrubCommandResult(result)

		assert.Equal(t, constants.SentinelStatusSuccess, scrubbed.Status)
		assert.NotEmpty(t, scrubbed.Warnings)
		assert.Contains(t, scrubbed.Warnings, "deprecation_warning")
		assert.Contains(t, scrubbed.Warnings, "security_warning")
	})
}

func TestScrubbingService_ExtractStructureHints(t *testing.T) {
	t.Parallel()
	logger := testutil.NewTestLogger()
	service := NewScrubbingService(nil, logger, nil)

	t.Run("JSON object", func(t *testing.T) {
		t.Parallel()
		hints := service.extractStructureHints(`{"key": "value"}`)
		assert.Contains(t, hints, "format: json_object")
	})

	t.Run("JSON array", func(t *testing.T) {
		t.Parallel()
		hints := service.extractStructureHints(`[1, 2, 3]`)
		assert.Contains(t, hints, "format: json_array")
	})

	t.Run("tabular data with pipes", func(t *testing.T) {
		t.Parallel()
		hints := service.extractStructureHints("id | name | email\n1 | John | j@x.com")
		found := false
		for _, h := range hints {
			if strings.HasPrefix(h, "columns:") {
				found = true
			}
		}
		assert.True(t, found, "Should detect columns")
	})

	t.Run("size categories", func(t *testing.T) {
		t.Parallel()
		tests := []struct {
			size     int
			expected string
		}{
			{50, "size: minimal"},
			{500, "size: small"},
			{5000, "size: medium"},
			{50000, "size: large"},
			{500000, "size: very_large"},
		}

		for _, tt := range tests {
			data := strings.Repeat("x", tt.size)
			hints := service.extractStructureHints(data)
			assert.Contains(t, hints, tt.expected)
		}
	})
}

func TestScrubbingService_ExtractSafeMetrics(t *testing.T) {
	t.Parallel()
	logger := testutil.NewTestLogger()
	service := NewScrubbingService(nil, logger, nil)

	t.Run("row counts", func(t *testing.T) {
		t.Parallel()
		metrics := service.ExtractSafeMetrics("Query returned 42 rows")
		assert.Equal(t, 42, metrics["row_count"])
	})

	t.Run("multiple metrics", func(t *testing.T) {
		t.Parallel()
		output := "Processed 100 files, 5 errors, 10 warnings"
		metrics := service.ExtractSafeMetrics(output)
		assert.Equal(t, 100, metrics["file_count"])
		assert.Equal(t, 5, metrics["error_count"])
		assert.Equal(t, 10, metrics["warning_count"])
	})
}

func TestScrubbingService_ValidateNoLeakage(t *testing.T) {
	t.Parallel()
	logger := testutil.NewTestLogger()
	service := NewScrubbingService(nil, logger, nil)

	t.Run("clean text passes", func(t *testing.T) {
		t.Parallel()
		ok, violations := service.ValidateNoLeakage("Status: success, 10 rows processed")
		assert.True(t, ok)
		assert.Empty(t, violations)
	})

	t.Run("IP address is allowed (preserved for troubleshooting)", func(t *testing.T) {
		t.Parallel()
		ok, violations := service.ValidateNoLeakage("Server at 192.168.1.1")
		assert.True(t, ok)
		assert.Empty(t, violations)
	})

	t.Run("email detected", func(t *testing.T) {
		t.Parallel()
		ok, violations := service.ValidateNoLeakage("Contact: user@example.com")
		assert.False(t, ok)
		assert.Contains(t, violations, "email")
	})

	t.Run("email placeholder is allowed", func(t *testing.T) {
		t.Parallel()
		ok, violations := service.ValidateNoLeakage("Contact: [EMAIL]")
		assert.True(t, ok)
		assert.Empty(t, violations)
	})

	t.Run("UUID is allowed (preserved for troubleshooting)", func(t *testing.T) {
		t.Parallel()
		ok, violations := service.ValidateNoLeakage("Resource 550e8400-e29b-41d4-a716-446655440000")
		assert.True(t, ok)
		assert.Empty(t, violations)
	})

	t.Run("private key detected", func(t *testing.T) {
		t.Parallel()
		ok, violations := service.ValidateNoLeakage("-----BEGIN RSA PRIVATE KEY-----")
		assert.False(t, ok)
		assert.Contains(t, violations, "private_key")
	})
}

func TestScrubbingService_ScrubMap(t *testing.T) {
	t.Parallel()
	logger := testutil.NewTestLogger()

	t.Run("preserves IPs and scrubs emails in non-strict mode", func(t *testing.T) {
		t.Parallel()
		config := &Config{Enabled: true, StrictMode: false}
		service := NewScrubbingService(config, logger, nil)

		data := map[string]interface{}{
			"ip":    "192.168.1.1",
			"email": "user@test.com",
			"count": 42,
		}

		scrubbed := service.ScrubMap(data)
		assert.Equal(t, "192.168.1.1", scrubbed["ip"].(string))
		assert.Contains(t, scrubbed["email"].(string), "[EMAIL]")
		assert.Equal(t, 42, scrubbed["count"])
	})

	t.Run("preserves IPs in nested maps in non-strict mode", func(t *testing.T) {
		t.Parallel()
		config := &Config{Enabled: true, StrictMode: false}
		service := NewScrubbingService(config, logger, nil)

		data := map[string]interface{}{
			"server": map[string]interface{}{
				"host": "192.168.1.1",
				"port": 5432,
			},
		}

		scrubbed := service.ScrubMap(data)
		nested := scrubbed["server"].(map[string]interface{})
		assert.Equal(t, "192.168.1.1", nested["host"].(string))
		assert.Equal(t, 5432, nested["port"])
	})

	t.Run("scrubs sensitive keys in strict mode", func(t *testing.T) {
		t.Parallel()
		service := NewScrubbingService(nil, logger, nil)

		data := map[string]interface{}{
			"password": "secret123",
			"balance":  1000,
		}

		scrubbed := service.ScrubMap(data)
		assert.Equal(t, "[VALUE]", scrubbed["balance"])
	})
}

func TestScrubbingService_ScrubText_G8EAPIKey(t *testing.T) {
	t.Parallel()
	logger := testutil.NewTestLogger()
	config := &Config{Enabled: true, StrictMode: false}
	service := NewScrubbingService(config, logger, nil)

	t.Run("standalone key output", func(t *testing.T) {
		t.Parallel()
		input := "g8e_cm1241_0889f747327ff462500fba691894edbc415e81d145869757e9c2e75647defbf1"
		result := service.ScrubText(input)
		assert.Contains(t, result, "[REDACTED_API_KEY]")
		assert.NotContains(t, result, "g8e_cm1241_0889f747")
	})

	t.Run("key embedded in text", func(t *testing.T) {
		t.Parallel()
		input := "Your key is g8e_test99_aabbccdd00112233445566778899aabbccddeeff00112233445566778899aabb end"
		result := service.ScrubText(input)
		assert.Contains(t, result, "[REDACTED_API_KEY]")
		assert.NotContains(t, result, "aabbccdd0011")
	})

	t.Run("key in env var echo", func(t *testing.T) {
		t.Parallel()
		input := "G8E_OPERATOR_API_KEY=g8e_op5_0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
		result := service.ScrubText(input)
		assert.NotContains(t, result, "0123456789abcdef")
	})
}

func TestScrubbingService_ScrubText_CloudCredentials(t *testing.T) {
	t.Parallel()
	logger := testutil.NewTestLogger()
	config := &Config{Enabled: true, StrictMode: false}
	service := NewScrubbingService(config, logger, nil)

	t.Run("AWS Access Key ID", func(t *testing.T) {
		t.Parallel()
		tests := []struct {
			input    string
			contains string
		}{
			{"Key: AKIAIOSFODNN7EXAMPLE", "[AWS_KEY]"},
			{"ASIA1234567890ABCDEF", "[AWS_KEY]"},
			{"export AWS_ACCESS_KEY_ID=AKIA1234567890ABCDEF", "[AWS_KEY]"},
		}
		for _, tt := range tests {
			result := service.ScrubText(tt.input)
			assert.Contains(t, result, tt.contains, "Input: %s", tt.input)
		}
	})

	t.Run("GCP API Key", func(t *testing.T) {
		t.Parallel()
		input := "gcp_key=AIzaSyDaGmWKa4JsXZ-HjGw7ISLn_3namBGewQe"
		result := service.ScrubText(input)
		assert.Contains(t, result, "[GCP_API_KEY]")
		assert.NotContains(t, result, "AIzaSy")
	})

	t.Run("Azure Secret in config", func(t *testing.T) {
		t.Parallel()
		input := `azure_client_secret="abc123def456ghi789jkl012mno345pqr678"`
		result := service.ScrubText(input)
		assert.Contains(t, result, "[AZURE_SECRET]")
	})
}

func TestScrubbingService_ScrubText_JWT(t *testing.T) {
	t.Parallel()
	logger := testutil.NewTestLogger()
	config := &Config{Enabled: true, StrictMode: false}
	service := NewScrubbingService(config, logger, nil)

	input := "Authorization: Bearer eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiIxMjM0NTY3ODkwIiwibmFtZSI6IkpvaG4gRG9lIiwiaWF0IjoxNTE2MjM5MDIyfQ.SflKxwRJSMeKKF2QT4fwpMeJf36POk6yJV_adQssw5c"
	result := service.ScrubText(input)
	assert.Contains(t, result, "[JWT]")
	assert.NotContains(t, result, "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9")
}

func TestScrubbingService_TokenPersistence(t *testing.T) {
	t.Parallel()
	logger := testutil.NewTestLogger()
	tempDir := t.TempDir()

	// Create vault
	_, privKey, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)
	vaultDir := filepath.Join(tempDir, "vault")
	require.NoError(t, os.MkdirAll(vaultDir, 0700))
	vHeader, _, err := vault.NewVaultHeader(privKey)
	require.NoError(t, err)
	require.NoError(t, vHeader.Save(vaultDir))
	testVault, err := vault.NewVault(&vault.VaultConfig{DataDir: vaultDir, Logger: logger})
	require.NoError(t, err)
	require.NoError(t, testVault.Unlock(privKey))
	defer testVault.Close()

	// Create TokenStore
	storageConfig := &storage.TokenStoreConfig{
		DBPath:        filepath.Join(tempDir, "test_tokens.db"),
		RetentionDays: 30,
	}
	tokenStore, err := storage.NewTokenStoreService(storageConfig, logger, testVault)
	require.NoError(t, err)
	require.NotNil(t, tokenStore)
	defer tokenStore.Close()

	// Create ScrubbingService with persistence
	config := &Config{
		StrictMode:         false,
		RequirePersistence: true,
	}
	service := NewScrubbingService(config, logger, tokenStore)

	// Test token creation and persistence
	sensitiveValue := "my-secret-api-key-12345"
	token := service.GetTokenForValue(sensitiveValue)
	assert.NotEmpty(t, token)
	assert.Contains(t, token, "{{UEI_")

	// Verify token is in memory
	rehydrated := service.RehydrateText(token)
	assert.Equal(t, sensitiveValue, rehydrated)

	// Verify token is persisted in TokenStore
	key := fmt.Sprintf("sentinel_token_%s", token)
	storedValue, err := tokenStore.KVGet(context.Background(), key)
	require.NoError(t, err)
	assert.Equal(t, sensitiveValue, storedValue)

	// Test loading persisted tokens on new SovereigntyService instance
	service2 := NewScrubbingService(config, logger, tokenStore)
	rehydrated2 := service2.RehydrateText(token)
	assert.Equal(t, sensitiveValue, rehydrated2, "Should rehydrate from persisted storage")
}

func TestScrubbingService_TokenPersistence_FailClosed(t *testing.T) {
	t.Parallel()
	logger := testutil.NewTestLogger()

	// Create ScrubbingService with persistence required but no TokenStore
	config := &Config{
		StrictMode:         false,
		RequirePersistence: true,
	}
	service := NewScrubbingService(config, logger, nil)

	// Should fail closed - return empty token
	sensitiveValue := "my-secret-api-key-12345"
	token := service.GetTokenForValue(sensitiveValue)
	assert.Empty(t, token, "Should return empty token when persistence required but unavailable")
}

func TestScrubbingService_TokenPersistence_TTL(t *testing.T) {
	t.Parallel()
	logger := testutil.NewTestLogger()
	tempDir := t.TempDir()

	// Create vault
	_, privKey, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)
	vaultDir := filepath.Join(tempDir, "vault")
	require.NoError(t, os.MkdirAll(vaultDir, 0700))
	vHeader, _, err := vault.NewVaultHeader(privKey)
	require.NoError(t, err)
	require.NoError(t, vHeader.Save(vaultDir))
	testVault, err := vault.NewVault(&vault.VaultConfig{DataDir: vaultDir, Logger: logger})
	require.NoError(t, err)
	require.NoError(t, testVault.Unlock(privKey))
	defer testVault.Close()

	// Create TokenStore
	storageConfig := &storage.TokenStoreConfig{
		DBPath:        filepath.Join(tempDir, "test_ttl.db"),
		RetentionDays: 30,
	}
	tokenStore, err := storage.NewTokenStoreService(storageConfig, logger, testVault)
	require.NoError(t, err)
	require.NotNil(t, tokenStore)
	defer tokenStore.Close()

	// Create ScrubbingService with persistence
	config := &Config{
		StrictMode:         false,
		RequirePersistence: true,
	}
	service := NewScrubbingService(config, logger, tokenStore)

	// Create a token
	sensitiveValue := "my-secret-api-key-12345"
	token := service.GetTokenForValue(sensitiveValue)
	assert.NotEmpty(t, token)

	// Manually set a very short TTL in TokenStore to test expiration
	key := fmt.Sprintf("sentinel_token_%s", token)
	err = tokenStore.KVSet(context.Background(), key, sensitiveValue, 1) // 1 second TTL
	require.NoError(t, err)

	// Wait for expiration
	time.Sleep(2 * time.Second)

	// Token should no longer be retrievable from storage
	_, err = tokenStore.KVGet(context.Background(), key)
	assert.Error(t, err, "Token should expire after TTL")
}

func TestScrubbingService_ScrubText_ServiceTokens(t *testing.T) {
	t.Parallel()
	logger := testutil.NewTestLogger()
	config := &Config{Enabled: true, StrictMode: false}
	service := NewScrubbingService(config, logger, nil)

	t.Run("GitHub Token", func(t *testing.T) {
		t.Parallel()
		tests := []string{
			"ghp_xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx1234",
			"gho_xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx5678",
			"ghu_xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx9012",
		}
		for _, input := range tests {
			result := service.ScrubText(input)
			assert.Contains(t, result, "[GITHUB_TOKEN]", "Input: %s", input)
		}
	})

	t.Run("Slack Token", func(t *testing.T) {
		t.Parallel()
		tests := []string{
			"xoxb-123456789012-1234567890123-AbCdEfGhIjKlMnOpQrStUvWx",
			"xoxp-123456789012-1234567890123-AbCdEfGhIjKlMnOpQrStUvWx",
			"xapp-1-A1B2C3D4E5F-1234567890123-abcdef1234567890abcdef1234567890abcdef1234567890abcdef1234567890ab",
		}
		for _, input := range tests {
			result := service.ScrubText(input)
			assert.Contains(t, result, "[SLACK_TOKEN]", "Input: %s", input)
		}
	})

	t.Run("Okta API Token", func(t *testing.T) {
		t.Parallel()
		tests := []string{
			"00abcDefGhIjKlMnOpQrStUvWxYz0123456789ABCD",
			"00ABCDEFGHIJKLMNOPQRSTUVWXYZ01234567890abc",
		}
		for _, input := range tests {
			result := service.ScrubText(input)
			assert.Contains(t, result, "[OKTA_TOKEN]", "Input: %s", input)
		}
	})

	t.Run("Azure AD Client Secret", func(t *testing.T) {
		t.Parallel()
		tests := []string{
			"abc8Q~defghijklmnopqrstuvwxyz1234567890AB",
			"Xyz12~ABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789",
		}
		for _, input := range tests {
			result := service.ScrubText(input)
			assert.Contains(t, result, "[AZURE_SECRET]", "Input: %s", input)
		}
	})

	t.Run("SendGrid Key", func(t *testing.T) {
		t.Parallel()
		input := "SG.xxxxxxxxxxxxxxxxxxxxxx.xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx"
		result := service.ScrubText(input)
		assert.Contains(t, result, "[SENDGRID_KEY]")
	})

	t.Run("Twilio SID", func(t *testing.T) {
		t.Parallel()
		input := "Account SID: AC12345678901234567890123456789012"
		result := service.ScrubText(input)
		assert.Contains(t, result, "[TWILIO_SID]")
	})

	t.Run("NPM Token", func(t *testing.T) {
		t.Parallel()
		input := "Using npm_xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx for auth"
		result := service.ScrubText(input)
		assert.Contains(t, result, "[NPM_TOKEN]")
	})
}

func TestScrubbingService_ScrubText_IBAN(t *testing.T) {
	t.Parallel()
	logger := testutil.NewTestLogger()
	config := &Config{Enabled: true, StrictMode: false}
	service := NewScrubbingService(config, logger, nil)

	tests := []struct {
		name  string
		input string
	}{
		{"German IBAN", "DE89370400440532013000"},
		{"UK IBAN", "GB82WEST12345698765432"},
		{"French IBAN", "FR7630006000011234567890189"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			result := service.ScrubText("Bank account: " + tt.input)
			assert.Contains(t, result, "[IBAN]")
			assert.NotContains(t, result, tt.input)
		})
	}
}

func TestScrubbingService_ScrubText_BearerToken(t *testing.T) {
	t.Parallel()
	logger := testutil.NewTestLogger()
	config := &Config{Enabled: true, StrictMode: false}
	service := NewScrubbingService(config, logger, nil)

	tests := []string{
		"Authorization: Bearer abc123def456",
		"bearer some_random_token_value",
		"BEARER MySecretTokenHere",
	}

	for _, input := range tests {
		result := service.ScrubText(input)
		assert.Contains(t, result, "[BEARER_TOKEN]", "Input: %s", input)
	}
}

func TestScrubbingService_ScrubText_OAuthSecret(t *testing.T) {
	t.Parallel()
	logger := testutil.NewTestLogger()
	config := &Config{Enabled: true, StrictMode: false}
	service := NewScrubbingService(config, logger, nil)

	tests := []string{
		"client_secret=abcdefghijklmnopqrstuvwx",
		"oauth_secret: 12345678901234567890abcd",
		"clientSecret='mysupersecretclientkey'",
	}

	for _, input := range tests {
		result := service.ScrubText(input)
		assert.Contains(t, result, "[OAUTH_SECRET]", "Input: %s", input)
	}
}

func TestScrubbingService_IsEnabled(t *testing.T) {
	t.Parallel()
	logger := testutil.NewTestLogger()

	t.Run("enabled config", func(t *testing.T) {
		t.Parallel()
		config := &Config{Enabled: true}
		service := NewScrubbingService(config, logger, nil)
		assert.True(t, service.IsEnabled())
	})

	t.Run("disabled config", func(t *testing.T) {
		t.Parallel()
		config := &Config{Enabled: false}
		service := NewScrubbingService(config, logger, nil)
		assert.False(t, service.IsEnabled())
	})
}

func TestScrubbingService_CustomScrubPatterns(t *testing.T) {
	t.Parallel()
	logger := testutil.NewTestLogger()
	config := &Config{
		Enabled:    true,
		StrictMode: false,
		CustomScrubPatterns: map[string]string{
			"internal_id": `INT-\d{6}`,
		},
	}
	service := NewScrubbingService(config, logger, nil)

	result := service.ScrubText("Processing INT-123456")
	assert.Equal(t, "Processing [INTERNAL_ID]", result)
}

func TestScrubbingService_StrictModeDataRows(t *testing.T) {
	t.Parallel()
	logger := testutil.NewTestLogger()
	config := &Config{Enabled: true, StrictMode: true}
	service := NewScrubbingService(config, logger, nil)

	t.Run("tabular data preserves structure but scrubs sensitive values", func(t *testing.T) {
		t.Parallel()
		input := "id\tname\temail\n1\tJohn\tjohn@test.com\n2\tJane\tjane@test.com"
		result := service.ScrubText(input)
		// Structure preserved, but emails scrubbed
		assert.Contains(t, result, "[EMAIL]")
		assert.NotContains(t, result, "john@test.com")
		assert.NotContains(t, result, "jane@test.com")
		// Non-sensitive data preserved
		assert.Contains(t, result, "id")
		assert.Contains(t, result, "name")
	})

	t.Run("sensitive key-value pairs scrubbed", func(t *testing.T) {
		t.Parallel()
		input := "salary_info: 75000\nincome_data: 4430"
		result := service.ScrubText(input)
		assert.Contains(t, result, "salary_info: [VALUE]")
		assert.Contains(t, result, "income_data: [VALUE]")
	})

	t.Run("non-sensitive key-value pairs preserved", func(t *testing.T) {
		t.Parallel()
		input := "Version: 24.0.7\nClient: Container Engine"
		result := service.ScrubText(input)
		assert.Contains(t, result, "Version: 24.0.7")
		assert.Contains(t, result, "Client: Container Engine")
	})

	t.Run("JSON data preserves structure but scrubs sensitive values", func(t *testing.T) {
		t.Parallel()
		input := `{"user": "admin", "email": "admin@test.com"}`
		result := service.ScrubText(input)
		// Structure preserved, sensitive values scrubbed
		assert.Contains(t, result, "[EMAIL]")
		assert.NotContains(t, result, "admin@test.com")
	})
}

func TestScrubbingService_RehydrateText(t *testing.T) {
	t.Parallel()
	logger := testutil.NewTestLogger()
	config := &Config{Enabled: true, StrictMode: false}
	service := NewScrubbingService(config, logger, nil)

	t.Run("empty input returns empty", func(t *testing.T) {
		t.Parallel()
		result := service.RehydrateText("")
		assert.Equal(t, "", result)
	})

	t.Run("no tokens returns original", func(t *testing.T) {
		t.Parallel()
		input := "This is normal text"
		result := service.RehydrateText(input)
		assert.Equal(t, input, result)
	})

	t.Run("rehydrates known token", func(t *testing.T) {
		t.Parallel()
		token := service.GetTokenForValue("secret-value")
		result := service.RehydrateText("Command with " + token)
		assert.Equal(t, "Command with secret-value", result)
	})
}

func TestScrubbingService_RehydratePayload(t *testing.T) {
	t.Parallel()
	logger := testutil.NewTestLogger()
	config := &Config{Enabled: true, StrictMode: false}
	service := NewScrubbingService(config, logger, nil)

	t.Run("empty payload returns empty", func(t *testing.T) {
		t.Parallel()
		result, err := service.RehydratePayload([]byte{})
		assert.NoError(t, err)
		assert.Empty(t, result)
	})

	t.Run("non-JSON payload uses text rehydration", func(t *testing.T) {
		t.Parallel()
		token := service.GetTokenForValue("secret")
		payload := []byte("Text with " + token)
		result, err := service.RehydratePayload(payload)
		assert.NoError(t, err)
		assert.Equal(t, "Text with secret", string(result))
	})

	t.Run("JSON payload rehydrates recursively", func(t *testing.T) {
		t.Parallel()
		token := service.GetTokenForValue("secret")
		payload := []byte(`{"key": "value ` + token + `", "nested": {"data": "` + token + `"}}`)
		result, err := service.RehydratePayload(payload)
		assert.NoError(t, err)
		// Parse the result to verify rehydration
		var parsed map[string]interface{}
		err = json.Unmarshal(result, &parsed)
		assert.NoError(t, err)
		assert.Equal(t, "value secret", parsed["key"])
		nested := parsed["nested"].(map[string]interface{})
		assert.Equal(t, "secret", nested["data"])
	})
}

func TestScrubbingService_GetTokenForValue(t *testing.T) {
	t.Parallel()
	logger := testutil.NewTestLogger()
	config := &Config{Enabled: true, StrictMode: false}
	service := NewScrubbingService(config, logger, nil)

	t.Run("empty value returns empty", func(t *testing.T) {
		t.Parallel()
		token := service.GetTokenForValue("")
		assert.Empty(t, token)
	})

	t.Run("same value returns same token", func(t *testing.T) {
		t.Parallel()
		value := "my-secret"
		token1 := service.GetTokenForValue(value)
		token2 := service.GetTokenForValue(value)
		assert.Equal(t, token1, token2)
	})

	t.Run("different values return different tokens", func(t *testing.T) {
		t.Parallel()
		token1 := service.GetTokenForValue("secret1")
		token2 := service.GetTokenForValue("secret2")
		assert.NotEqual(t, token1, token2)
	})
}

func TestScrubbingService_ClearTokens(t *testing.T) {
	t.Parallel()
	logger := testutil.NewTestLogger()
	config := &Config{Enabled: true, StrictMode: false}
	service := NewScrubbingService(config, logger, nil)

	// Add some tokens
	service.GetTokenForValue("secret1")
	service.GetTokenForValue("secret2")
	assert.Greater(t, len(service.tokenMap), 0)

	// Clear tokens
	service.ClearTokens()
	assert.Empty(t, service.tokenMap)
	assert.Empty(t, service.reverseMap)
	assert.Equal(t, 0, service.tokenSequence)
}
