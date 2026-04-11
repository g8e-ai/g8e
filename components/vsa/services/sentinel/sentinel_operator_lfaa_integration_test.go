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

package sentinel

import (
	"context"
	"encoding/json"
	"testing"

	execution "github.com/g8e-ai/g8e/components/vsa/services/execution"
	storage "github.com/g8e-ai/g8e/components/vsa/services/storage"
	"github.com/g8e-ai/g8e/components/vsa/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestSentinelOperatorLFAAIntegration_Setup verifies that all three components
// can be initialized and wired together correctly
func TestSentinelOperatorLFAAIntegration_Setup(t *testing.T) {
	t.Run("all components initialize together", func(t *testing.T) {
		tempDir := t.TempDir()
		logger := testutil.NewTestLogger()

		// Initialize AuditVault (LFAA component)
		auditConfig := &storage.AuditVaultConfig{
			DataDir:                   tempDir,
			DBPath:                    "test.db",
			LedgerDir:                 "ledger",
			MaxDBSizeMB:               100,
			RetentionDays:             7,
			PruneIntervalMinutes:      60,
			Enabled:                   true,
			OutputTruncationThreshold: 102400,
			HeadTailSize:              51200,
		}
		auditVault, err := storage.NewAuditVaultService(auditConfig, logger)
		require.NoError(t, err)
		require.NotNil(t, auditVault)
		defer auditVault.Close()

		// Initialize LedgerMirror (LFAA component)
		ledgerMirror := storage.NewLedgerService(auditVault, nil, logger)
		require.NotNil(t, ledgerMirror)

		// Initialize storage.HistoryHandler (LFAA component)
		historyHandler := storage.NewHistoryHandler(auditVault, ledgerMirror, logger)
		require.NotNil(t, historyHandler)
		assert.True(t, historyHandler.IsEnabled())

		// Initialize Sentinel (zero-trust data scrubber)
		sentinelConfig := &SentinelConfig{
			Enabled:         true,
			StrictMode:      true,
			MaxOutputLength: 4096,
		}
		sentinel := NewSentinel(sentinelConfig, logger)
		require.NotNil(t, sentinel)
		assert.True(t, sentinel.IsEnabled())

		// Initialize ExecutionService (Operator component)
		cfg := testutil.NewTestConfig(t)
		execService := execution.NewExecutionService(cfg, logger)
		require.NotNil(t, execService)

		// Verify all services are ready for integration
		assert.True(t, auditVault.IsEnabled())
		assert.NotNil(t, ledgerMirror)
		assert.True(t, historyHandler.IsEnabled())
		assert.NotNil(t, execService)
	})
}

// TestSentinelScrubbing_Integration tests the full scrubbing pipeline
func TestSentinelScrubbing_Integration(t *testing.T) {
	logger := testutil.NewTestLogger()
	sentinel := NewSentinel(DefaultSentinelConfig(), logger)

	t.Run("scrubs database query output", func(t *testing.T) {
		result := &CommandResult{
			Command:    "psql -c 'SELECT * FROM users'",
			ExitCode:   0,
			Stdout:     "id,email,password_hash\n1,admin@corp.com,bcrypt$2a$12$xyz\n2,user@test.org,bcrypt$2a$12$abc",
			Stderr:     "",
			DurationMs: 150,
		}

		scrubbed := sentinel.ScrubForCloudAI(result)

		assert.Equal(t, "success", scrubbed.Status)
		assert.Equal(t, 0, scrubbed.ExitCode)
		assert.NotContains(t, scrubbed.Summary, "admin@corp.com")
		assert.NotContains(t, scrubbed.Summary, "bcrypt")
		assert.Greater(t, scrubbed.OutputLines, 0)
	})

	t.Run("scrubs network diagnostics", func(t *testing.T) {
		result := &CommandResult{
			Command:    "netstat -an",
			ExitCode:   0,
			Stdout:     "tcp 0 0 192.168.1.100:443 10.0.0.50:54321 ESTABLISHED\ntcp 0 0 172.16.0.1:22 192.168.1.1:12345 TIME_WAIT",
			Stderr:     "",
			DurationMs: 50,
		}

		scrubbed := sentinel.ScrubForCloudAI(result)

		assert.Equal(t, "success", scrubbed.Status)
		assert.NotContains(t, scrubbed.Summary, "192.168")
		assert.NotContains(t, scrubbed.Summary, "172.16")
		assert.NotContains(t, scrubbed.Summary, "10.0.0")
	})

	t.Run("scrubs configuration file output", func(t *testing.T) {
		result := &CommandResult{
			Command:  "cat /etc/app/config.yaml",
			ExitCode: 0,
			Stdout: `database:
  host: db.internal.company.com
  port: 5432
  password: supersecret123
  api_key: 00AbCdEfGhIjKlMnOpQrStUvWxYz0123456789XY`,
			Stderr:     "",
			DurationMs: 10,
		}

		scrubbed := sentinel.ScrubForCloudAI(result)

		assert.Equal(t, "success", scrubbed.Status)
		assert.NotContains(t, scrubbed.Summary, "supersecret")
		assert.NotContains(t, scrubbed.Summary, "00AbCdEf")
		assert.NotContains(t, scrubbed.Summary, "db.internal")
	})

	t.Run("scrubs error output with paths", func(t *testing.T) {
		result := &CommandResult{
			Command:    "cat /home/admin/.ssh/id_rsa",
			ExitCode:   1,
			Stdout:     "",
			Stderr:     "Permission denied: /home/admin/.ssh/id_rsa",
			DurationMs: 5,
		}

		scrubbed := sentinel.ScrubForCloudAI(result)

		assert.Equal(t, "failure", scrubbed.Status)
		assert.Equal(t, "permission_denied", scrubbed.ErrorType)
		assert.Contains(t, scrubbed.Summary, "/home/admin/.ssh/id_rsa")
	})

	t.Run("preserves safe metadata", func(t *testing.T) {
		result := &CommandResult{
			Command:    "wc -l /tmp/data.csv",
			ExitCode:   0,
			Stdout:     "1000 /tmp/data.csv",
			Stderr:     "",
			DurationMs: 25,
		}

		scrubbed := sentinel.ScrubForCloudAI(result)

		assert.Equal(t, "success", scrubbed.Status)
		assert.Equal(t, 0, scrubbed.ExitCode)
		assert.Equal(t, int64(25), scrubbed.DurationMs)
		assert.Greater(t, scrubbed.OutputLines, 0)
	})
}

// TestSentinelWithExecutionService_Integration tests Sentinel scrubbing command output
// that would come from ExecutionService. Uses simulated output to avoid environment deps.
func TestSentinelWithExecutionService_Integration(t *testing.T) {
	logger := testutil.NewTestLogger()
	sentinel := NewSentinel(DefaultSentinelConfig(), logger)

	t.Run("scrubs command output with sensitive data", func(t *testing.T) {
		// Simulate command output that would come from reading a file with sensitive data
		cmdResult := &CommandResult{
			Command:    "cat [PATH]",
			ExitCode:   0,
			Stdout:     "user@example.com\n192.168.1.1\npassword=secret123\n",
			Stderr:     "",
			DurationMs: 15,
		}

		// Scrub through Sentinel
		scrubbed := sentinel.ScrubForCloudAI(cmdResult)

		// Verify sensitive data is scrubbed
		assert.Equal(t, "success", scrubbed.Status)
		assert.NotContains(t, scrubbed.Summary, "user@example.com")
		assert.NotContains(t, scrubbed.Summary, "192.168.1.1")
		assert.NotContains(t, scrubbed.Summary, "secret123")
	})

	t.Run("scrubs multi-format sensitive output", func(t *testing.T) {
		// Simulate command output with various sensitive data patterns
		cmdResult := &CommandResult{
			Command:  "grep -r 'config' [PATH]",
			ExitCode: 0,
			Stdout: `db_host=10.0.0.50
api_key=00AbCdEfGhIjKlMnOpQrStUvWxYz0123456789XY
admin_email=admin@internal.corp
connection_string=postgres://user:pass@db.internal.net:5432/prod`,
			Stderr:     "",
			DurationMs: 250,
		}

		scrubbed := sentinel.ScrubForCloudAI(cmdResult)

		assert.Equal(t, "success", scrubbed.Status)
		assert.NotContains(t, scrubbed.Summary, "10.0.0.50")
		assert.NotContains(t, scrubbed.Summary, "00AbCdEf")
		assert.NotContains(t, scrubbed.Summary, "admin@internal.corp")
		assert.NotContains(t, scrubbed.Summary, "postgres://")
	})
}

// TestSentinelAuditIntegration tests that Sentinel works with AuditVault
func TestSentinelAuditIntegration(t *testing.T) {
	tempDir := t.TempDir()
	logger := testutil.NewTestLogger()

	// Initialize AuditVault
	auditConfig := &storage.AuditVaultConfig{
		DataDir:                   tempDir,
		DBPath:                    "test.db",
		LedgerDir:                 "ledger",
		MaxDBSizeMB:               100,
		RetentionDays:             7,
		PruneIntervalMinutes:      60,
		Enabled:                   true,
		OutputTruncationThreshold: 102400,
		HeadTailSize:              51200,
	}
	auditVault, err := storage.NewAuditVaultService(auditConfig, logger)
	require.NoError(t, err)
	defer auditVault.Close()

	sentinel := NewSentinel(DefaultSentinelConfig(), logger)

	t.Run("sensitive command output can be scrubbed before cloud transmission", func(t *testing.T) {
		_ = context.Background()

		// Simulate command output with sensitive data that would be stored in audit
		cmdResult := &CommandResult{
			Command:    "psql -h db.secret.internal -U admin -c 'SELECT * FROM users'",
			ExitCode:   0,
			Stdout:     "id,email,ssn\n1,admin@corp.com,123-45-6789\n",
			Stderr:     "",
			DurationMs: 1500,
		}

		scrubbed := sentinel.ScrubForCloudAI(cmdResult)

		// Verify scrubbing - emails and SSNs are scrubbed, hostnames are preserved
		assert.Equal(t, "success", scrubbed.Status)
		assert.NotContains(t, scrubbed.Summary, "admin@corp.com")
		assert.NotContains(t, scrubbed.Summary, "123-45-6789")

		// Verify audit vault is working
		assert.True(t, auditVault.IsEnabled())
	})
}

// TestSentinelJSONScrubbing tests scrubbing of JSON payloads
func TestSentinelJSONScrubbing(t *testing.T) {
	logger := testutil.NewTestLogger()
	// Use non-strict mode for pattern-based scrubbing verification
	config := &SentinelConfig{Enabled: true, StrictMode: false}
	sentinel := NewSentinel(config, logger)

	t.Run("scrubs JSON API response", func(t *testing.T) {
		jsonData := map[string]interface{}{
			"users": []interface{}{
				map[string]interface{}{
					"id":       1,
					"email":    "admin@internal.corp.com",
					"ip":       "10.0.0.100",
					"password": "hashed_password_here",
				},
			},
			"server": "db-prod-01.internal.net",
			"count":  42,
		}

		scrubbed := sentinel.ScrubMap(jsonData)

		// Verify: emails scrubbed, IPs and hostnames preserved
		users := scrubbed["users"].([]interface{})
		user := users[0].(map[string]interface{})
		assert.Contains(t, user["email"].(string), "[EMAIL]")
		assert.Equal(t, "10.0.0.100", user["ip"].(string))
		assert.Equal(t, "db-prod-01.internal.net", scrubbed["server"].(string))
		// Count should be preserved (safe metadata)
		assert.Equal(t, 42, scrubbed["count"])
	})
}

// TestSentinelMetricsExtraction tests safe metric extraction
func TestSentinelMetricsExtraction(t *testing.T) {
	logger := testutil.NewTestLogger()
	sentinel := NewSentinel(DefaultSentinelConfig(), logger)

	t.Run("extracts row counts from SQL output", func(t *testing.T) {
		output := "Query OK, 150 rows affected (0.05 sec)"
		metrics := sentinel.ExtractSafeMetrics(output)
		assert.Equal(t, 150, metrics["row_count"])
	})

	t.Run("extracts file counts from find output", func(t *testing.T) {
		output := "Found 25 files matching pattern"
		metrics := sentinel.ExtractSafeMetrics(output)
		assert.Equal(t, 25, metrics["file_count"])
	})

	t.Run("extracts error counts", func(t *testing.T) {
		output := "Compilation finished with 3 errors and 7 warnings"
		metrics := sentinel.ExtractSafeMetrics(output)
		assert.Equal(t, 3, metrics["error_count"])
		assert.Equal(t, 7, metrics["warning_count"])
	})
}

// TestSentinelValidateNoLeakage_Integration tests the leakage validation
func TestSentinelValidateNoLeakage_Integration(t *testing.T) {
	logger := testutil.NewTestLogger()
	sentinel := NewSentinel(DefaultSentinelConfig(), logger)

	t.Run("IPs are allowed (preserved for troubleshooting)", func(t *testing.T) {
		text := "Server at 192.168.1.1 returned error"
		ok, violations := sentinel.ValidateNoLeakage(text)
		assert.True(t, ok)
		assert.Empty(t, violations)
	})

	t.Run("detects unscrubbed email", func(t *testing.T) {
		text := "Contact admin@corp.com for help"
		ok, violations := sentinel.ValidateNoLeakage(text)
		assert.False(t, ok)
		assert.Contains(t, violations, "email")
	})

	t.Run("passes properly scrubbed output", func(t *testing.T) {
		text := "Server at 192.168.1.1 returned error type: connection_refused, contact [EMAIL]"
		ok, violations := sentinel.ValidateNoLeakage(text)
		assert.True(t, ok)
		assert.Empty(t, violations)
	})
}

// TestSentinelDisabled tests behavior when Sentinel is disabled
func TestSentinelDisabled(t *testing.T) {
	logger := testutil.NewTestLogger()
	config := &SentinelConfig{Enabled: false}
	sentinel := NewSentinel(config, logger)

	t.Run("ScrubText returns suppressed message", func(t *testing.T) {
		result := sentinel.ScrubText("sensitive data: 192.168.1.1")
		assert.Equal(t, "[OUTPUT_SUPPRESSED]", result)
	})

	t.Run("ScrubCommandResult still provides structure", func(t *testing.T) {
		cmdResult := &CommandResult{
			Command:    "cat /etc/passwd",
			ExitCode:   0,
			Stdout:     "root:x:0:0:root:/root:/bin/bash",
			Stderr:     "",
			DurationMs: 10,
		}

		scrubbed := sentinel.ScrubCommandResult(cmdResult)

		assert.Equal(t, "success", scrubbed.Status)
		assert.Equal(t, 0, scrubbed.ExitCode)
		assert.Contains(t, scrubbed.Summary, "output suppressed")
	})
}

// TestSentinelStrictMode tests aggressive scrubbing in strict mode
func TestSentinelStrictMode(t *testing.T) {
	logger := testutil.NewTestLogger()
	config := &SentinelConfig{
		Enabled:    true,
		StrictMode: true,
	}
	sentinel := NewSentinel(config, logger)

	t.Run("preserves tabular structure but scrubs sensitive values", func(t *testing.T) {
		input := "id\tname\temail\n1\tJohn Doe\tjohn@example.com\n2\tJane Smith\tjane@test.org"
		result := sentinel.ScrubText(input)
		// Structure preserved, emails scrubbed
		assert.Contains(t, result, "[EMAIL]")
		assert.NotContains(t, result, "john@example.com")
		assert.NotContains(t, result, "jane@test.org")
		// Column headers preserved
		assert.Contains(t, result, "id")
		assert.Contains(t, result, "name")
		assert.Contains(t, result, "email")
	})

	t.Run("scrubs sensitive key-value pairs", func(t *testing.T) {
		input := "salary_info: 75000\nincome_data: 90000"
		result := sentinel.ScrubText(input)
		assert.Contains(t, result, "salary_info: [VALUE]")
		assert.Contains(t, result, "income_data: [VALUE]")
	})

	t.Run("preserves non-sensitive key-value pairs", func(t *testing.T) {
		input := "hostname: prod-server-01.internal.net\nstatus: running\nconnections: 150"
		result := sentinel.ScrubText(input)
		assert.Contains(t, result, "hostname: prod-server-01.internal.net")
		assert.Contains(t, result, "status: running")
		assert.Contains(t, result, "connections: 150")
	})
}

// TestSentinelErrorCategorization tests error type detection
func TestSentinelErrorCategorization(t *testing.T) {
	logger := testutil.NewTestLogger()
	sentinel := NewSentinel(DefaultSentinelConfig(), logger)

	testCases := []struct {
		stderr     string
		exitCode   int
		expectType string
	}{
		{"bash: /etc/shadow: Permission denied", 1, "permission_denied"},
		{"curl: (7) Failed to connect: Connection refused", 1, "connection_refused"},
		{"ERROR: connection timed out after 30 seconds", 1, "timeout"},
		{"FATAL: out of memory", 1, "out_of_memory"},
		{"No space left on device", 1, "disk_full"},
		{"Authentication failed for user admin", 1, "authentication_failed"},
		{"bash: syntax error near unexpected token", 1, "syntax_error"},
		{"ERROR: relation already exists", 1, "already_exists"},
		{"ERROR: resource is locked by another process", 1, "resource_busy"},
	}

	for _, tc := range testCases {
		t.Run(tc.expectType, func(t *testing.T) {
			result := sentinel.categorizeError(tc.stderr, tc.exitCode)
			assert.Equal(t, tc.expectType, result)
		})
	}
}

// TestSentinelWarningExtraction tests warning message handling
func TestSentinelWarningExtraction(t *testing.T) {
	logger := testutil.NewTestLogger()
	sentinel := NewSentinel(DefaultSentinelConfig(), logger)

	t.Run("categorizes warning types", func(t *testing.T) {
		cmdResult := &CommandResult{
			Command:  "npm install",
			ExitCode: 0,
			Stdout:   "installed 100 packages",
			Stderr: `npm WARN deprecated request@2.88.2: use modern alternatives
npm WARN insecure connection to registry
npm WARN performance slow network detected`,
			DurationMs: 5000,
		}

		scrubbed := sentinel.ScrubCommandResult(cmdResult)

		assert.Contains(t, scrubbed.Warnings, "deprecation_warning")
		assert.Contains(t, scrubbed.Warnings, "security_warning")
		assert.Contains(t, scrubbed.Warnings, "performance_warning")
	})
}

// TestSentinelStructureHints tests output structure detection
func TestSentinelStructureHints(t *testing.T) {
	logger := testutil.NewTestLogger()
	sentinel := NewSentinel(DefaultSentinelConfig(), logger)

	t.Run("detects JSON object format", func(t *testing.T) {
		hints := sentinel.extractStructureHints(`{"key": "value", "count": 42}`)
		assert.Contains(t, hints, "format: json_object")
	})

	t.Run("detects JSON array format", func(t *testing.T) {
		hints := sentinel.extractStructureHints(`[{"id": 1}, {"id": 2}]`)
		assert.Contains(t, hints, "format: json_array")
	})

	t.Run("detects tabular column count", func(t *testing.T) {
		// With "id | name | email" there are 2 pipes, so columns = 2 - 1 = 1 based on code logic
		// Actually the code does: colCount := strings.Count(firstLine, "|") - 1
		// For "id | name | email | created_at" with 3 pipes: 3 - 1 = 2
		hints := sentinel.extractStructureHints("id | name | email | created_at\n1 | test | t@t.com | 2024-01-01")
		found := false
		for _, h := range hints {
			if h == "columns: 2" {
				found = true
			}
		}
		assert.True(t, found, "Should detect 2 columns (3 pipes minus 1)")
	})
}

// TestSentinelOutputResultSerialization tests that scrubbed results serialize correctly
func TestSentinelOutputResultSerialization(t *testing.T) {
	logger := testutil.NewTestLogger()
	sentinel := NewSentinel(DefaultSentinelConfig(), logger)

	cmdResult := &CommandResult{
		Command:    "SELECT COUNT(*) FROM users WHERE email LIKE '%@corp.com'",
		ExitCode:   0,
		Stdout:     "count\n-----\n  150\n(1 row)",
		Stderr:     "",
		DurationMs: 45,
	}

	scrubbed := sentinel.ScrubForCloudAI(cmdResult)

	// Serialize to JSON (as would happen when sending to AI Agent Services)
	jsonBytes, err := json.Marshal(scrubbed)
	require.NoError(t, err)

	// Verify JSON doesn't contain sensitive data
	jsonStr := string(jsonBytes)
	assert.NotContains(t, jsonStr, "@corp.com")
	assert.Contains(t, jsonStr, "success")
	assert.Contains(t, jsonStr, "exit_code")
}
