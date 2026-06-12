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

package mcp

import (
	"bytes"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	_ "modernc.org/sqlite"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/g8e-ai/g8e/internal/constants"
	commonv1 "github.com/g8e-ai/g8e/protocol/proto/g8e/common/v1"
	operatorv1 "github.com/g8e-ai/g8e/protocol/proto/g8e/operator/v1"
)

// TestNativeToolsIntegration_DatabaseTools tests database native tools
// with real SQLite databases and audit vault persistence.
// Requires: ./g8e gw start running with mTLS
func TestNativeToolsIntegration_DatabaseTools(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	operatorURL := os.Getenv("OPERATOR_URL")
	if operatorURL == "" {
		operatorURL = fmt.Sprintf("https://localhost:%d", constants.Ports.OperatorHttps)
	}

	// Check if Operator is reachable
	insecureClient := &http.Client{
		Timeout: 5 * time.Second,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		},
	}
	if resp, err := insecureClient.Get(operatorURL + constants.APIPaths.Health); err != nil {
		t.Skipf("Operator not reachable at %s: %v. Run './g8e gw start' to enable.", operatorURL, err)
	} else {
		resp.Body.Close()
	}

	// Setup mTLS client (requires ./g8e login)
	mtlsClient, sessionID, err := setupMTLSClient(t, operatorURL)
	require.NoError(t, err, "failed to setup mTLS client. Run './g8e login'")

	t.Run("db_discover_topology", func(t *testing.T) {
		tmpDir := t.TempDir()
		dbPath := filepath.Join(tmpDir, "test.db")

		// Create test database with schema
		db, err := sql.Open("sqlite", dbPath)
		require.NoError(t, err)
		defer db.Close()

		_, err = db.Exec(`CREATE TABLE users (
			id INTEGER PRIMARY KEY,
			name TEXT NOT NULL,
			email TEXT UNIQUE,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		)`)
		require.NoError(t, err)

		_, err = db.Exec(`CREATE INDEX idx_users_email ON users(email)`)
		require.NoError(t, err)

		// Call native tool through governance envelope
		req := DBDiscoverTopologyRequest{DatabasePath: dbPath}
		reqJSON, _ := json.Marshal(req)

		receipt := callNativeToolViaEnvelope(t, mtlsClient, operatorURL, sessionID, "db_discover_topology", reqJSON)
		require.Equal(t, operatorv1.ExecutionStatus_EXECUTION_STATUS_COMPLETED, receipt.Status)

		// Verify result structure
		var result DBDiscoverTopologyResult
		err = json.Unmarshal([]byte(receipt.ResultSummary), &result)
		require.NoError(t, err)

		require.Contains(t, result.Schema, "users")
		require.Equal(t, "INTEGER", result.Schema["users"]["id"])
		require.Equal(t, "TEXT", result.Schema["users"]["name"])

		// Verify audit vault persistence
		verifyAuditVaultPersistence(t, receipt.TransactionId, sessionID)
	})

	t.Run("db_query_validate_full_scan_rejection", func(t *testing.T) {
		tmpDir := t.TempDir()
		dbPath := filepath.Join(tmpDir, "test.db")

		db, err := sql.Open("sqlite", dbPath)
		require.NoError(t, err)
		defer db.Close()

		_, err = db.Exec("CREATE TABLE test_table (id INTEGER PRIMARY KEY, data TEXT)")
		require.NoError(t, err)

		// Query that will trigger full table scan
		req := DBQueryValidateRequest{
			DatabasePath: dbPath,
			Query:        "SELECT * FROM test_table",
		}
		reqJSON, _ := json.Marshal(req)

		receipt := callNativeToolViaEnvelope(t, mtlsClient, operatorURL, sessionID, "db_query_validate", reqJSON)
		require.Equal(t, operatorv1.ExecutionStatus_EXECUTION_STATUS_COMPLETED, receipt.Status)

		var result DBQueryValidateResult
		err = json.Unmarshal([]byte(receipt.ResultSummary), &result)
		require.NoError(t, err)

		require.True(t, result.Rejected, "full table scan should be rejected")
		require.Contains(t, strings.ToLower(result.Reason), "scan")
	})

	t.Run("db_isolated_read_only_select", func(t *testing.T) {
		tmpDir := t.TempDir()
		dbPath := filepath.Join(tmpDir, "test.db")

		db, err := sql.Open("sqlite", dbPath)
		require.NoError(t, err)
		defer db.Close()

		_, err = db.Exec("CREATE TABLE test_table (id INTEGER PRIMARY KEY, value TEXT)")
		require.NoError(t, err)
		_, err = db.Exec("INSERT INTO test_table (id, value) VALUES (1, 'test_value')")
		require.NoError(t, err)

		req := DBIsolatedReadRequest{
			DatabasePath: dbPath,
			Query:        "SELECT * FROM test_table WHERE id = 1",
		}
		reqJSON, _ := json.Marshal(req)

		receipt := callNativeToolViaEnvelope(t, mtlsClient, operatorURL, sessionID, "db_isolated_read", reqJSON)
		require.Equal(t, operatorv1.ExecutionStatus_EXECUTION_STATUS_COMPLETED, receipt.Status)

		var result DBIsolatedReadResult
		err = json.Unmarshal([]byte(receipt.ResultSummary), &result)
		require.NoError(t, err)

		require.Len(t, result.Rows, 1)
		require.Equal(t, "test_value", result.Rows[0]["value"])
	})

	t.Run("db_isolated_read_rejects_mutation", func(t *testing.T) {
		tmpDir := t.TempDir()
		dbPath := filepath.Join(tmpDir, "test.db")

		req := DBIsolatedReadRequest{
			DatabasePath: dbPath,
			Query:        "DROP TABLE test_table",
		}
		reqJSON, _ := json.Marshal(req)

		receipt := callNativeToolViaEnvelope(t, mtlsClient, operatorURL, sessionID, "db_isolated_read", reqJSON)
		// Tool should complete but return error
		require.Equal(t, operatorv1.ExecutionStatus_EXECUTION_STATUS_COMPLETED, receipt.Status)

		require.Contains(t, strings.ToLower(receipt.ResultSummary), "error")
	})
}

// TestNativeToolsIntegration_LogTools tests log filtering with scrubbing
func TestNativeToolsIntegration_LogTools(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	operatorURL := os.Getenv("OPERATOR_URL")
	if operatorURL == "" {
		operatorURL = fmt.Sprintf("https://localhost:%d", constants.Ports.OperatorHttps)
	}

	insecureClient := &http.Client{
		Timeout: 5 * time.Second,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		},
	}
	if resp, err := insecureClient.Get(operatorURL + constants.APIPaths.Health); err != nil {
		t.Skipf("Operator not reachable at %s: %v", operatorURL, err)
	} else {
		resp.Body.Close()
	}

	mtlsClient, sessionID, err := setupMTLSClient(t, operatorURL)
	require.NoError(t, err)

	t.Run("log_stream_filter_with_scrubbing", func(t *testing.T) {
		tmpDir := t.TempDir()
		logPath := filepath.Join(tmpDir, "test.log")

		// Create log with sensitive data
		logContent := `INFO: user login successful
ERROR: authentication failed for user=admin password=secret123
DEBUG: api_key=sk-12345 processed request
INFO: token=abc123def456 session started
WARN: database connection failed`
		err := os.WriteFile(logPath, []byte(logContent), 0644)
		require.NoError(t, err)

		req := LogStreamFilterRequest{
			LogPath: logPath,
			Pattern: "ERROR|WARN",
			Limit:   10,
		}
		reqJSON, _ := json.Marshal(req)

		receipt := callNativeToolViaEnvelope(t, mtlsClient, operatorURL, sessionID, "log_stream_filter", reqJSON)
		require.Equal(t, operatorv1.ExecutionStatus_EXECUTION_STATUS_COMPLETED, receipt.Status)

		var result LogStreamFilterResult
		err = json.Unmarshal([]byte(receipt.ResultSummary), &result)
		require.NoError(t, err)

		require.Positive(t, result.Count)

		// Verify scrubbing - sensitive patterns should be redacted
		for _, line := range result.Lines {
			require.NotContains(t, line, "secret123", "password should be redacted")
			require.NotContains(t, line, "sk-12345", "api key should be redacted")
			require.NotContains(t, line, "abc123def456", "token should be redacted")
		}
	})
}

// TestNativeToolsIntegration_ProcessTools tests process metrics and signal safety
func TestNativeToolsIntegration_ProcessTools(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	operatorURL := os.Getenv("OPERATOR_URL")
	if operatorURL == "" {
		operatorURL = fmt.Sprintf("https://localhost:%d", constants.Ports.OperatorHttps)
	}

	insecureClient := &http.Client{
		Timeout: 5 * time.Second,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		},
	}
	if resp, err := insecureClient.Get(operatorURL + constants.APIPaths.Health); err != nil {
		t.Skipf("Operator not reachable at %s: %v", operatorURL, err)
	} else {
		resp.Body.Close()
	}

	mtlsClient, sessionID, err := setupMTLSClient(t, operatorURL)
	require.NoError(t, err)

	t.Run("proc_metric_top", func(t *testing.T) {
		req := ProcMetricTopRequest{Limit: 5}
		reqJSON, _ := json.Marshal(req)

		receipt := callNativeToolViaEnvelope(t, mtlsClient, operatorURL, sessionID, "proc_metric_top", reqJSON)
		require.Equal(t, operatorv1.ExecutionStatus_EXECUTION_STATUS_COMPLETED, receipt.Status)

		var result ProcMetricTopResult
		err = json.Unmarshal([]byte(receipt.ResultSummary), &result)
		require.NoError(t, err)

		require.NotEmpty(t, result.Processes)
		for _, proc := range result.Processes {
			require.Positive(t, proc.PID)
			require.NotEmpty(t, proc.Name)
		}
	})

	t.Run("proc_signal_safe_denylist_enforcement", func(t *testing.T) {
		// Test that protected PIDs (1, 2) are rejected
		for _, protectedPID := range []int{1, 2} {
			req := ProcSignalSafeRequest{
				PID:    protectedPID,
				Signal: "SIGTERM",
			}
			reqJSON, _ := json.Marshal(req)

			receipt := callNativeToolViaEnvelope(t, mtlsClient, operatorURL, sessionID, "proc_signal_safe", reqJSON)
			require.Equal(t, operatorv1.ExecutionStatus_EXECUTION_STATUS_COMPLETED, receipt.Status)

			var result ProcSignalSafeResult
			err = json.Unmarshal([]byte(receipt.ResultSummary), &result)
			require.NoError(t, err)

			require.False(t, result.Sent, "protected PID should be rejected")
			require.Contains(t, strings.ToLower(result.Error), "denylist")
		}
	})

	t.Run("proc_signal_safe_unsupported_signal", func(t *testing.T) {
		req := ProcSignalSafeRequest{
			PID:    9999,
			Signal: "INVALID_SIGNAL",
		}
		reqJSON, _ := json.Marshal(req)

		receipt := callNativeToolViaEnvelope(t, mtlsClient, operatorURL, sessionID, "proc_signal_safe", reqJSON)
		require.Equal(t, operatorv1.ExecutionStatus_EXECUTION_STATUS_COMPLETED, receipt.Status)

		var result ProcSignalSafeResult
		err = json.Unmarshal([]byte(receipt.ResultSummary), &result)
		require.NoError(t, err)

		require.False(t, result.Sent)
		require.Contains(t, strings.ToLower(result.Error), "unsupported")
	})
}

// TestNativeToolsIntegration_NetworkTools tests network auditing and probing
func TestNativeToolsIntegration_NetworkTools(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	operatorURL := os.Getenv("OPERATOR_URL")
	if operatorURL == "" {
		operatorURL = fmt.Sprintf("https://localhost:%d", constants.Ports.OperatorHttps)
	}

	insecureClient := &http.Client{
		Timeout: 5 * time.Second,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		},
	}
	if resp, err := insecureClient.Get(operatorURL + constants.APIPaths.Health); err != nil {
		t.Skipf("Operator not reachable at %s: %v", operatorURL, err)
	} else {
		resp.Body.Close()
	}

	mtlsClient, sessionID, err := setupMTLSClient(t, operatorURL)
	require.NoError(t, err)

	t.Run("net_socket_audit", func(t *testing.T) {
		req := NetSocketAuditRequest{Protocol: "tcp"}
		reqJSON, _ := json.Marshal(req)

		receipt := callNativeToolViaEnvelope(t, mtlsClient, operatorURL, sessionID, "net_socket_audit", reqJSON)
		require.Equal(t, operatorv1.ExecutionStatus_EXECUTION_STATUS_COMPLETED, receipt.Status)

		var result NetSocketAuditResult
		err = json.Unmarshal([]byte(receipt.ResultSummary), &result)
		require.NoError(t, err)

		require.NotNil(t, result.Sockets)
	})

	t.Run("net_endpoint_ping_unreachable", func(t *testing.T) {
		req := NetEndpointPingRequest{
			Host: "invalid-host-that-does-not-exist-12345.com",
			Port: 9999,
		}
		reqJSON, _ := json.Marshal(req)

		receipt := callNativeToolViaEnvelope(t, mtlsClient, operatorURL, sessionID, "net_endpoint_ping", reqJSON)
		require.Equal(t, operatorv1.ExecutionStatus_EXECUTION_STATUS_COMPLETED, receipt.Status)

		var result NetEndpointPingResult
		err = json.Unmarshal([]byte(receipt.ResultSummary), &result)
		require.NoError(t, err)

		require.False(t, result.Reachable)
		require.NotEmpty(t, result.Error)
	})

	t.Run("net_http_probe_local_operator", func(t *testing.T) {
		// Probe the local Operator health endpoint
		req := NetHTTPProbeRequest{
			URL:    operatorURL + constants.APIPaths.Health,
			Method: "HEAD",
		}
		reqJSON, _ := json.Marshal(req)

		receipt := callNativeToolViaEnvelope(t, mtlsClient, operatorURL, sessionID, "net_http_probe", reqJSON)
		require.Equal(t, operatorv1.ExecutionStatus_EXECUTION_STATUS_COMPLETED, receipt.Status)

		var result NetHTTPProbeResult
		err = json.Unmarshal([]byte(receipt.ResultSummary), &result)
		require.NoError(t, err)

		require.Equal(t, http.StatusOK, result.StatusCode)
		require.Greater(t, result.LatencyMs, 0.0)
	})
}

// TestNativeToolsIntegration_Concurrency tests TOCTOU resistance with concurrent tool calls
func TestNativeToolsIntegration_Concurrency(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	operatorURL := os.Getenv("OPERATOR_URL")
	if operatorURL == "" {
		operatorURL = fmt.Sprintf("https://localhost:%d", constants.Ports.OperatorHttps)
	}

	insecureClient := &http.Client{
		Timeout: 5 * time.Second,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		},
	}
	if resp, err := insecureClient.Get(operatorURL + constants.APIPaths.Health); err != nil {
		t.Skipf("Operator not reachable at %s: %v", operatorURL, err)
	} else {
		resp.Body.Close()
	}

	mtlsClient, sessionID, err := setupMTLSClient(t, operatorURL)
	require.NoError(t, err)

	t.Run("concurrent_db_reads", func(t *testing.T) {
		tmpDir := t.TempDir()
		dbPath := filepath.Join(tmpDir, "test.db")

		db, err := sql.Open("sqlite", dbPath)
		require.NoError(t, err)
		defer db.Close()

		_, err = db.Exec("CREATE TABLE test_table (id INTEGER PRIMARY KEY, value TEXT)")
		require.NoError(t, err)
		for i := 0; i < 100; i++ {
			_, err = db.Exec("INSERT INTO test_table (id, value) VALUES (?, ?)", i, fmt.Sprintf("value_%d", i))
			require.NoError(t, err)
		}

		// Launch 10 concurrent reads
		var wg sync.WaitGroup
		errors := make(chan error, 10)

		for i := 0; i < 10; i++ {
			wg.Add(1)
			go func(iteration int) {
				defer wg.Done()

				req := DBIsolatedReadRequest{
					DatabasePath: dbPath,
					Query:        fmt.Sprintf("SELECT * FROM test_table WHERE id = %d", iteration*10),
				}
				reqJSON, _ := json.Marshal(req)

				receipt := callNativeToolViaEnvelope(t, mtlsClient, operatorURL, sessionID, "db_isolated_read", reqJSON)
				if receipt.Status != operatorv1.ExecutionStatus_EXECUTION_STATUS_COMPLETED {
					errors <- fmt.Errorf("iteration %d: unexpected status %s", iteration, receipt.Status)
					return
				}

				var result DBIsolatedReadResult
				err = json.Unmarshal([]byte(receipt.ResultSummary), &result)
				if err != nil {
					errors <- fmt.Errorf("iteration %d: failed to unmarshal result: %v", iteration, err)
					return
				}

				if len(result.Rows) == 0 {
					errors <- fmt.Errorf("iteration %d: expected rows in result", iteration)
				}
			}(i)
		}

		wg.Wait()
		close(errors)

		for err := range errors {
			t.Error(err)
		}
	})
}

// TestNativeToolsIntegration_PropertyBasedTests verifies safety invariants
func TestNativeToolsIntegration_PropertyBasedTests(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	operatorURL := os.Getenv("OPERATOR_URL")
	if operatorURL == "" {
		operatorURL = fmt.Sprintf("https://localhost:%d", constants.Ports.OperatorHttps)
	}

	insecureClient := &http.Client{
		Timeout: 5 * time.Second,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		},
	}
	if resp, err := insecureClient.Get(operatorURL + constants.APIPaths.Health); err != nil {
		t.Skipf("Operator not reachable at %s: %v", operatorURL, err)
	} else {
		resp.Body.Close()
	}

	mtlsClient, sessionID, err := setupMTLSClient(t, operatorURL)
	require.NoError(t, err)

	t.Run("db_isolated_read_never_mutates", func(t *testing.T) {
		// Property: db_isolated_read must never mutate the database
		tmpDir := t.TempDir()
		dbPath := filepath.Join(tmpDir, "test.db")

		db, err := sql.Open("sqlite", dbPath)
		require.NoError(t, err)
		defer db.Close()

		_, err = db.Exec("CREATE TABLE test_table (id INTEGER PRIMARY KEY, value TEXT)")
		require.NoError(t, err)
		_, err = db.Exec("INSERT INTO test_table (id, value) VALUES (1, 'original')")
		require.NoError(t, err)

		// Get initial row count
		var initialCount int
		err = db.QueryRow("SELECT COUNT(*) FROM test_table").Scan(&initialCount)
		require.NoError(t, err)

		// Try various mutation attempts
		mutationQueries := []string{
			"DROP TABLE test_table",
			"DELETE FROM test_table",
			"INSERT INTO test_table (id, value) VALUES (2, 'new')",
			"UPDATE test_table SET value = 'modified' WHERE id = 1",
			"CREATE TABLE new_table (id INTEGER)",
		}

		for _, query := range mutationQueries {
			req := DBIsolatedReadRequest{
				DatabasePath: dbPath,
				Query:        query,
			}
			reqJSON, _ := json.Marshal(req)

			receipt := callNativeToolViaEnvelope(t, mtlsClient, operatorURL, sessionID, "db_isolated_read", reqJSON)
			require.Equal(t, operatorv1.ExecutionStatus_EXECUTION_STATUS_COMPLETED, receipt.Status)

			// Verify database is unchanged
			var currentCount int
			err = db.QueryRow("SELECT COUNT(*) FROM test_table").Scan(&currentCount)
			require.NoError(t, err)
			require.Equal(t, initialCount, currentCount, "database should not be mutated by isolated read")
		}
	})

	t.Run("proc_signal_safe_never_kills_protected", func(t *testing.T) {
		// Property: proc_signal_safe must never allow killing protected PIDs
		protectedPIDs := []int{0, 1, 2}
		signals := []string{"SIGTERM", "SIGKILL", "SIGINT"}

		for _, pid := range protectedPIDs {
			for _, sig := range signals {
				req := ProcSignalSafeRequest{
					PID:    pid,
					Signal: sig,
				}
				reqJSON, _ := json.Marshal(req)

				receipt := callNativeToolViaEnvelope(t, mtlsClient, operatorURL, sessionID, "proc_signal_safe", reqJSON)
				require.Equal(t, operatorv1.ExecutionStatus_EXECUTION_STATUS_COMPLETED, receipt.Status)

				var result ProcSignalSafeResult
				err = json.Unmarshal([]byte(receipt.ResultSummary), &result)
				require.NoError(t, err)

				require.False(t, result.Sent, "protected PID %d should never be killed with %s", pid, sig)
			}
		}
	})

	t.Run("log_stream_filter_always_scrubs_secrets", func(t *testing.T) {
		// Property: log_stream_filter must always redact sensitive patterns
		tmpDir := t.TempDir()
		logPath := filepath.Join(tmpDir, "test.log")

		sensitivePatterns := []string{
			"password=secret123",
			"api_key=sk-12345",
			"token=abc123",
			"secret=mysecret",
		}

		logContent := strings.Join(sensitivePatterns, "\n")
		err := os.WriteFile(logPath, []byte(logContent), 0644)
		require.NoError(t, err)

		req := LogStreamFilterRequest{
			LogPath: logPath,
			Pattern: ".*",
			Limit:   100,
		}
		reqJSON, _ := json.Marshal(req)

		receipt := callNativeToolViaEnvelope(t, mtlsClient, operatorURL, sessionID, "log_stream_filter", reqJSON)
		require.Equal(t, operatorv1.ExecutionStatus_EXECUTION_STATUS_COMPLETED, receipt.Status)

		var result LogStreamFilterResult
		err = json.Unmarshal([]byte(receipt.ResultSummary), &result)
		require.NoError(t, err)

		for _, line := range result.Lines {
			require.NotContains(t, line, "secret123")
			require.NotContains(t, line, "sk-12345")
			require.NotContains(t, line, "abc123")
			require.NotContains(t, line, "mysecret")
		}
	})
}

// TestNativeToolsIntegration_NegativeControls tests intentional failures
func TestNativeToolsIntegration_NegativeControls(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	operatorURL := os.Getenv("OPERATOR_URL")
	if operatorURL == "" {
		operatorURL = fmt.Sprintf("https://localhost:%d", constants.Ports.OperatorHttps)
	}

	insecureClient := &http.Client{
		Timeout: 5 * time.Second,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		},
	}
	if resp, err := insecureClient.Get(operatorURL + constants.APIPaths.Health); err != nil {
		t.Skipf("Operator not reachable at %s: %v", operatorURL, err)
	} else {
		resp.Body.Close()
	}

	mtlsClient, sessionID, err := setupMTLSClient(t, operatorURL)
	require.NoError(t, err)

	t.Run("missing_required_parameters", func(t *testing.T) {
		// Test that tools reject missing required parameters
		tmpDir := t.TempDir()
		tests := []struct {
			toolName string
			args     json.RawMessage
		}{
			{"db_discover_topology", json.RawMessage(`{}`)},
			{"db_query_validate", json.RawMessage(fmt.Sprintf(`{"database_path": "%s/test.db"}`, tmpDir))},
			{"db_isolated_read", json.RawMessage(fmt.Sprintf(`{"database_path": "%s/test.db"}`, tmpDir))},
			{"fs_disk_profile", json.RawMessage(`{}`)},
			{"net_endpoint_ping", json.RawMessage(`{"host": "localhost"}`)},
		}

		for _, tt := range tests {
			receipt := callNativeToolViaEnvelope(t, mtlsClient, operatorURL, sessionID, tt.toolName, tt.args)
			// Tool should complete but return error
			require.Equal(t, operatorv1.ExecutionStatus_EXECUTION_STATUS_COMPLETED, receipt.Status)
			require.Contains(t, strings.ToLower(receipt.ResultSummary), "error")
		}
	})

	t.Run("invalid_json_arguments", func(t *testing.T) {
		// Test that tools reject malformed JSON
		invalidJSON := json.RawMessage(`{invalid json`)

		receipt := callNativeToolViaEnvelope(t, mtlsClient, operatorURL, sessionID, "db_discover_topology", invalidJSON)
		require.Equal(t, operatorv1.ExecutionStatus_EXECUTION_STATUS_COMPLETED, receipt.Status)
		require.Contains(t, strings.ToLower(receipt.ResultSummary), "error")
	})

	t.Run("nonexistent_paths", func(t *testing.T) {
		// Test that tools handle nonexistent paths gracefully
		req := DBDiscoverTopologyRequest{DatabasePath: "/nonexistent/path/to/database.db"}
		reqJSON, _ := json.Marshal(req)

		receipt := callNativeToolViaEnvelope(t, mtlsClient, operatorURL, sessionID, "db_discover_topology", reqJSON)
		require.Equal(t, operatorv1.ExecutionStatus_EXECUTION_STATUS_COMPLETED, receipt.Status)
		require.Contains(t, strings.ToLower(receipt.ResultSummary), "error")
	})
}

// Helper functions

func setupMTLSClient(t *testing.T, operatorURL string) (*http.Client, string, error) {
	cwd, err := os.Getwd()
	require.NoError(t, err)
	repoRoot := filepath.Dir(filepath.Dir(filepath.Dir(cwd)))

	pkiDir := filepath.Join(repoRoot, constants.Paths.Infra.PkiDir)

	// Load client certificate and key
	certPath := filepath.Join(pkiDir, "client", "client.pem")
	keyPath := filepath.Join(pkiDir, "client", "client-key.pem")

	cert, err := tls.LoadX509KeyPair(certPath, keyPath)
	if err != nil {
		return nil, "", fmt.Errorf("failed to load client certificate: %w. Run './g8e login'", err)
	}

	// Load trust bundle
	trustBundlePath := filepath.Join(pkiDir, "trust", "g8eg-ca-bundle.pem")
	trustPEM, err := os.ReadFile(trustBundlePath)
	if err != nil {
		return nil, "", fmt.Errorf("failed to read trust bundle: %w", err)
	}
	rootPool := x509.NewCertPool()
	if !rootPool.AppendCertsFromPEM(trustPEM) {
		return nil, "", fmt.Errorf("failed to parse trust bundle")
	}

	// Extract session ID from certificate CN
	sessionID := cert.Leaf.Subject.CommonName

	client := &http.Client{
		Timeout: 30 * time.Second,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				RootCAs:      rootPool,
				Certificates: []tls.Certificate{cert},
			},
		},
	}

	return client, sessionID, nil
}

func callNativeToolViaEnvelope(t *testing.T, client *http.Client, operatorURL, sessionID, toolName string, arguments json.RawMessage) *operatorv1.ActionReceipt {
	// Build governance envelope with native tool call
	now := time.Now()
	envelope := &commonv1.GovernanceEnvelope{
		Id:              uuid.New().String(),
		Timestamp:       timestamppb.New(now),
		ExpiresAt:       timestamppb.New(now.Add(5 * time.Minute)),
		TransactionHash: computeTransactionHash(toolName, arguments),
		ActionType:      string(constants.ActionTypeMcpCall),
		EventType:       string(constants.EventOperatorMcpCallRequested),
		Payload:         buildToolCallPayload(toolName, arguments),
		Governance: &commonv1.GovernanceMetadata{
			GatewaySigned: true,
			L2: &commonv1.L2Metadata{
				KeyId:    "gateway-local-signer",
				AgentIds: []string{"gateway-local-signer"},
			},
			L3: &commonv1.L3Metadata{
				AutoApproved: true,
			},
		},
		StateMerkleRoot: "test-state-root",
		SourceComponent: commonv1.Component_COMPONENT_CLIENT,
	}

	envelopeJSON, err := protojson.Marshal(envelope)
	assert.NoError(t, err) //nolint:testifylint,require-error // called from goroutine

	// Submit envelope to governance endpoint
	req, err := http.NewRequest(http.MethodPost, operatorURL+constants.APIPaths.GovernanceEnvelopes, bytes.NewReader(envelopeJSON))
	assert.NoError(t, err) //nolint:testifylint,require-error // called from goroutine
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+sessionID)
	req.Header.Set("X-G8E-Source-Component", "test-integration")

	resp, err := client.Do(req)
	assert.NoError(t, err) //nolint:testifylint,require-error // called from goroutine
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode, "envelope submission failed")

	body, err := io.ReadAll(resp.Body)
	assert.NoError(t, err) //nolint:testifylint,require-error // called from goroutine

	var receipt operatorv1.ActionReceipt
	err = protojson.Unmarshal(body, &receipt)
	assert.NoError(t, err) //nolint:testifylint,require-error // called from goroutine

	return &receipt
}

func buildToolCallPayload(toolName string, arguments json.RawMessage) []byte {
	mcpPayload := &operatorv1.McpCallRequested{
		ToolName:      toolName,
		ArgumentsJson: string(arguments),
		ExecutionId:   uuid.New().String(),
	}
	payloadBytes, err := proto.Marshal(mcpPayload)
	if err != nil {
		// In test context, panic is acceptable for setup failures
		panic(fmt.Sprintf("failed to marshal MCP payload: %v", err))
	}
	return payloadBytes
}

func computeTransactionHash(toolName string, arguments json.RawMessage) string {
	h := sha256.New()
	h.Write([]byte(toolName))
	h.Write(arguments)
	return hex.EncodeToString(h.Sum(nil))
}

func verifyAuditVaultPersistence(t *testing.T, transactionID, sessionID string) {
	cwd, err := os.Getwd()
	require.NoError(t, err)
	repoRoot := filepath.Dir(filepath.Dir(filepath.Dir(cwd)))

	vaultPath := filepath.Join(repoRoot, constants.Paths.Infra.AuditVaultDBPath)

	db, err := sql.Open("sqlite", vaultPath)
	require.NoError(t, err)
	defer db.Close()

	// Query audit vault for the transaction
	var count int
	err = db.QueryRow("SELECT COUNT(*) FROM console_audit WHERE transaction_id = ?", transactionID).Scan(&count)
	require.NoError(t, err)
	require.Equal(t, 1, count, "transaction should be recorded in audit vault")
}
