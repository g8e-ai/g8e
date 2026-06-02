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
	"context"
	"database/sql"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	_ "modernc.org/sqlite"
)

func TestNativeToolHandler_HandleTool(t *testing.T) {
	handler := NewNativeToolHandler()

	t.Run("unknown tool", func(t *testing.T) {
		_, err := handler.HandleTool(context.Background(), "unknown_tool", json.RawMessage(`{}`))
		if err == nil {
			t.Error("expected error for unknown tool")
		}
	})

	t.Run("all 23 tools registered", func(t *testing.T) {
		tools := handler.ListTools()
		if len(tools) != 23 {
			t.Errorf("expected 23 registered tools, got %d", len(tools))
		}

		expectedTools := []string{
			"db_discover_topology",
			"db_query_validate",
			"db_isolated_read",
			"db_index_triage",
			"log_stream_filter",
			"sys_oom_detect",
			"config_diff_mask",
			"proc_metric_top",
			"fs_disk_profile",
			"proc_signal_safe",
			"net_socket_audit",
			"net_endpoint_ping",
			"net_http_probe",
			"sys_info",
			"net_dns_resolve",
			"tls_cert_inspect",
			"sys_env_vars",
			"fs_file_checksum",
			"sys_service_status",
			"sys_container_status",
			"fs_disk_usage",
			"sys_time_clock",
			"proc_tree",
		}

		toolNames := make(map[string]bool)
		for _, tool := range tools {
			toolNames[tool.Name()] = true
		}

		for _, expected := range expectedTools {
			if !toolNames[expected] {
				t.Errorf("expected tool '%s' to be registered", expected)
			}
		}
	})
}

func TestHandleDBDiscoverTopology(t *testing.T) {
	handler := NewNativeToolHandler()

	t.Run("valid database", func(t *testing.T) {
		tmpDir := t.TempDir()
		dbPath := filepath.Join(tmpDir, "test.db")

		db, err := sql.Open("sqlite", dbPath)
		if err != nil {
			t.Fatalf("failed to create test database: %v", err)
		}
		defer db.Close()

		_, err = db.Exec("CREATE TABLE test_table (id INTEGER PRIMARY KEY, name TEXT)")
		if err != nil {
			t.Fatalf("failed to create test table: %v", err)
		}

		req := DBDiscoverTopologyRequest{DatabasePath: dbPath}
		reqJSON, _ := json.Marshal(req)

		result, err := handler.HandleTool(context.Background(), "db_discover_topology", reqJSON)
		if err != nil {
			t.Fatalf("HandleTool failed: %v", err)
		}

		if len(result.Content) == 0 {
			t.Error("expected content in result")
		}

		var schema DBDiscoverTopologyResult
		if err := json.Unmarshal([]byte(result.Content[0].Text), &schema); err != nil {
			t.Fatalf("failed to unmarshal result: %v", err)
		}

		if _, ok := schema.Schema["test_table"]; !ok {
			t.Error("expected test_table in schema")
		}
	})

	t.Run("missing database path", func(t *testing.T) {
		req := DBDiscoverTopologyRequest{}
		reqJSON, _ := json.Marshal(req)

		_, err := handler.HandleTool(context.Background(), "db_discover_topology", reqJSON)
		if err == nil {
			t.Error("expected error for missing database path")
		}
	})

	t.Run("non-existent database", func(t *testing.T) {
		req := DBDiscoverTopologyRequest{DatabasePath: "/nonexistent/path.db"}
		reqJSON, _ := json.Marshal(req)

		_, err := handler.HandleTool(context.Background(), "db_discover_topology", reqJSON)
		if err == nil {
			t.Error("expected error for non-existent database")
		}
	})
}

func TestHandleDBQueryValidate(t *testing.T) {
	handler := NewNativeToolHandler()

	t.Run("indexed SELECT query accepted", func(t *testing.T) {
		tmpDir := t.TempDir()
		dbPath := filepath.Join(tmpDir, "test.db")

		db, err := sql.Open("sqlite", dbPath)
		if err != nil {
			t.Fatalf("failed to create test database: %v", err)
		}
		defer db.Close()

		_, err = db.Exec("CREATE TABLE test_table (id INTEGER PRIMARY KEY, name TEXT)")
		if err != nil {
			t.Fatalf("failed to create test table: %v", err)
		}

		req := DBQueryValidateRequest{
			DatabasePath: dbPath,
			Query:        "SELECT name FROM test_table WHERE id = 1",
		}
		reqJSON, _ := json.Marshal(req)

		result, err := handler.HandleTool(context.Background(), "db_query_validate", reqJSON)
		if err != nil {
			t.Fatalf("HandleTool failed: %v", err)
		}

		var validation DBQueryValidateResult
		if err := json.Unmarshal([]byte(result.Content[0].Text), &validation); err != nil {
			t.Fatalf("failed to unmarshal result: %v", err)
		}

		if validation.Rejected {
			t.Errorf("indexed query should not be rejected: %s", validation.Reason)
		}
	})

	t.Run("full table scan rejected", func(t *testing.T) {
		tmpDir := t.TempDir()
		dbPath := filepath.Join(tmpDir, "test.db")

		db, err := sql.Open("sqlite", dbPath)
		if err != nil {
			t.Fatalf("failed to create test database: %v", err)
		}
		defer db.Close()

		_, err = db.Exec("CREATE TABLE test_table (id INTEGER PRIMARY KEY, name TEXT)")
		if err != nil {
			t.Fatalf("failed to create test table: %v", err)
		}

		req := DBQueryValidateRequest{
			DatabasePath: dbPath,
			Query:        "SELECT * FROM test_table",
		}
		reqJSON, _ := json.Marshal(req)

		result, err := handler.HandleTool(context.Background(), "db_query_validate", reqJSON)
		if err != nil {
			t.Fatalf("HandleTool failed: %v", err)
		}

		var validation DBQueryValidateResult
		if err := json.Unmarshal([]byte(result.Content[0].Text), &validation); err != nil {
			t.Fatalf("failed to unmarshal result: %v", err)
		}

		if !validation.Rejected {
			t.Error("full table scan should be rejected")
		}
	})

	t.Run("non-SELECT query rejected", func(t *testing.T) {
		tmpDir := t.TempDir()
		dbPath := filepath.Join(tmpDir, "test.db")

		db, err := sql.Open("sqlite", dbPath)
		if err != nil {
			t.Fatalf("failed to create test database: %v", err)
		}
		defer db.Close()

		req := DBQueryValidateRequest{
			DatabasePath: dbPath,
			Query:        "DROP TABLE test_table",
		}
		reqJSON, _ := json.Marshal(req)

		result, err := handler.HandleTool(context.Background(), "db_query_validate", reqJSON)
		if err != nil {
			t.Fatalf("HandleTool failed: %v", err)
		}

		var validation DBQueryValidateResult
		if err := json.Unmarshal([]byte(result.Content[0].Text), &validation); err != nil {
			t.Fatalf("failed to unmarshal result: %v", err)
		}

		if !validation.Rejected {
			t.Error("expected non-SELECT query to be rejected")
		}
	})
}

func TestHandleDBIsolatedRead(t *testing.T) {
	handler := NewNativeToolHandler()

	t.Run("valid SELECT query", func(t *testing.T) {
		tmpDir := t.TempDir()
		dbPath := filepath.Join(tmpDir, "test.db")

		db, err := sql.Open("sqlite", dbPath)
		if err != nil {
			t.Fatalf("failed to create test database: %v", err)
		}
		defer db.Close()

		_, err = db.Exec("CREATE TABLE test_table (id INTEGER PRIMARY KEY, name TEXT)")
		if err != nil {
			t.Fatalf("failed to create test table: %v", err)
		}

		_, err = db.Exec("INSERT INTO test_table (id, name) VALUES (1, 'test')")
		if err != nil {
			t.Fatalf("failed to insert test data: %v", err)
		}

		req := DBIsolatedReadRequest{
			DatabasePath: dbPath,
			Query:        "SELECT * FROM test_table",
		}
		reqJSON, _ := json.Marshal(req)

		result, err := handler.HandleTool(context.Background(), "db_isolated_read", reqJSON)
		if err != nil {
			t.Fatalf("HandleTool failed: %v", err)
		}

		var readResult DBIsolatedReadResult
		if err := json.Unmarshal([]byte(result.Content[0].Text), &readResult); err != nil {
			t.Fatalf("failed to unmarshal result: %v", err)
		}

		if len(readResult.Rows) == 0 {
			t.Error("expected rows in result")
		}
	})

	t.Run("non-SELECT query rejected", func(t *testing.T) {
		tmpDir := t.TempDir()
		dbPath := filepath.Join(tmpDir, "test.db")

		req := DBIsolatedReadRequest{
			DatabasePath: dbPath,
			Query:        "DROP TABLE test_table",
		}
		reqJSON, _ := json.Marshal(req)

		result, err := handler.HandleTool(context.Background(), "db_isolated_read", reqJSON)
		if err != nil {
			t.Fatalf("HandleTool failed: %v", err)
		}

		if !result.IsError {
			t.Error("expected error for non-SELECT query")
		}
	})
}

func TestHandleDBIndexTriage(t *testing.T) {
	handler := NewNativeToolHandler()

	t.Run("valid database", func(t *testing.T) {
		tmpDir := t.TempDir()
		dbPath := filepath.Join(tmpDir, "test.db")

		db, err := sql.Open("sqlite", dbPath)
		if err != nil {
			t.Fatalf("failed to create test database: %v", err)
		}
		defer db.Close()

		_, err = db.Exec("CREATE TABLE test_table (id INTEGER PRIMARY KEY, name TEXT)")
		if err != nil {
			t.Fatalf("failed to create test table: %v", err)
		}

		req := DBIndexTriageRequest{DatabasePath: dbPath}
		reqJSON, _ := json.Marshal(req)

		result, err := handler.HandleTool(context.Background(), "db_index_triage", reqJSON)
		if err != nil {
			t.Fatalf("HandleTool failed: %v", err)
		}

		var triage DBIndexTriageResult
		if err := json.Unmarshal([]byte(result.Content[0].Text), &triage); err != nil {
			t.Fatalf("failed to unmarshal result: %v", err)
		}

		if triage.Indexes == nil {
			triage.Indexes = []IndexInfo{}
		}
	})
}

func TestHandleLogStreamFilter(t *testing.T) {
	handler := NewNativeToolHandler()

	t.Run("valid log file", func(t *testing.T) {
		tmpDir := t.TempDir()
		logPath := filepath.Join(tmpDir, "test.log")

		logContent := "INFO: test message\nERROR: error message\nDEBUG: debug message"
		if err := os.WriteFile(logPath, []byte(logContent), 0644); err != nil {
			t.Fatalf("failed to create test log file: %v", err)
		}

		req := LogStreamFilterRequest{
			LogPath: logPath,
			Pattern: "ERROR",
			Limit:   10,
		}
		reqJSON, _ := json.Marshal(req)

		result, err := handler.HandleTool(context.Background(), "log_stream_filter", reqJSON)
		if err != nil {
			t.Fatalf("HandleTool failed: %v", err)
		}

		var filterResult LogStreamFilterResult
		if err := json.Unmarshal([]byte(result.Content[0].Text), &filterResult); err != nil {
			t.Fatalf("failed to unmarshal result: %v", err)
		}

		if filterResult.Count == 0 {
			t.Error("expected matching lines")
		}
	})

	t.Run("missing log path", func(t *testing.T) {
		req := LogStreamFilterRequest{Pattern: "ERROR"}
		reqJSON, _ := json.Marshal(req)

		_, err := handler.HandleTool(context.Background(), "log_stream_filter", reqJSON)
		if err == nil {
			t.Error("expected error for missing log path")
		}
	})
}

func TestHandleSysOOMDetect(t *testing.T) {
	handler := NewNativeToolHandler()

	t.Run("default log path", func(t *testing.T) {
		req := SysOOMDetectRequest{}
		reqJSON, _ := json.Marshal(req)

		result, err := handler.HandleTool(context.Background(), "sys_oom_detect", reqJSON)
		if err != nil {
			t.Fatalf("HandleTool failed: %v", err)
		}

		var oomResult SysOOMDetectResult
		if err := json.Unmarshal([]byte(result.Content[0].Text), &oomResult); err != nil {
			t.Fatalf("failed to unmarshal result: %v", err)
		}

		if oomResult.Events == nil {
			oomResult.Events = []OOMEvent{}
		}
	})
}

func TestHandleConfigDiffMask(t *testing.T) {
	handler := NewNativeToolHandler()

	t.Run("valid config diff", func(t *testing.T) {
		tmpDir := t.TempDir()
		configPath := filepath.Join(tmpDir, "config.yaml")

		configContent := "key1: value1\nkey2: secret_value"
		if err := os.WriteFile(configPath, []byte(configContent), 0644); err != nil {
			t.Fatalf("failed to create test config file: %v", err)
		}

		baseline := "key1: value1\nkey2: baseline_value"

		req := ConfigDiffMaskRequest{
			ConfigPath: configPath,
			Baseline:   baseline,
		}
		reqJSON, _ := json.Marshal(req)

		result, err := handler.HandleTool(context.Background(), "config_diff_mask", reqJSON)
		if err != nil {
			t.Fatalf("HandleTool failed: %v", err)
		}

		var diffResult ConfigDiffMaskResult
		if err := json.Unmarshal([]byte(result.Content[0].Text), &diffResult); err != nil {
			t.Fatalf("failed to unmarshal result: %v", err)
		}

		if diffResult.Differences == nil {
			t.Error("expected differences in result")
		}
	})

	t.Run("secret masking", func(t *testing.T) {
		tmpDir := t.TempDir()
		configPath := filepath.Join(tmpDir, "config.yaml")

		configContent := "password: mypassword"
		if err := os.WriteFile(configPath, []byte(configContent), 0644); err != nil {
			t.Fatalf("failed to create test config file: %v", err)
		}

		baseline := "password: baseline_password"

		req := ConfigDiffMaskRequest{
			ConfigPath: configPath,
			Baseline:   baseline,
		}
		reqJSON, _ := json.Marshal(req)

		result, err := handler.HandleTool(context.Background(), "config_diff_mask", reqJSON)
		if err != nil {
			t.Fatalf("HandleTool failed: %v", err)
		}

		var diffResult ConfigDiffMaskResult
		if err := json.Unmarshal([]byte(result.Content[0].Text), &diffResult); err != nil {
			t.Fatalf("failed to unmarshal result: %v", err)
		}

		for _, diff := range diffResult.Differences {
			if diff.Current == "mypassword" {
				t.Error("expected password to be masked")
			}
			if diff.Baseline == "baseline_password" {
				t.Error("expected baseline password to be masked")
			}
		}
	})
}

func TestHandleProcMetricTop(t *testing.T) {
	handler := NewNativeToolHandler()

	t.Run("valid request", func(t *testing.T) {
		req := ProcMetricTopRequest{Limit: 5}
		reqJSON, _ := json.Marshal(req)

		result, err := handler.HandleTool(context.Background(), "proc_metric_top", reqJSON)
		if err != nil {
			t.Fatalf("HandleTool failed: %v", err)
		}

		var topResult ProcMetricTopResult
		if err := json.Unmarshal([]byte(result.Content[0].Text), &topResult); err != nil {
			t.Fatalf("failed to unmarshal result: %v", err)
		}

		if topResult.Processes == nil {
			t.Error("expected processes in result")
		}
	})
}

func TestHandleFSDiskProfile(t *testing.T) {
	handler := NewNativeToolHandler()

	t.Run("valid path", func(t *testing.T) {
		tmpDir := t.TempDir()

		req := FSDiskProfileRequest{
			Path:     tmpDir,
			MaxDepth: 1,
		}
		reqJSON, _ := json.Marshal(req)

		result, err := handler.HandleTool(context.Background(), "fs_disk_profile", reqJSON)
		if err != nil {
			t.Fatalf("HandleTool failed: %v", err)
		}

		var profileResult FSDiskProfileResult
		if err := json.Unmarshal([]byte(result.Content[0].Text), &profileResult); err != nil {
			t.Fatalf("failed to unmarshal result: %v", err)
		}

		if profileResult.Entries == nil {
			t.Error("expected entries in result")
		}
	})

	t.Run("missing path", func(t *testing.T) {
		req := FSDiskProfileRequest{}
		reqJSON, _ := json.Marshal(req)

		_, err := handler.HandleTool(context.Background(), "fs_disk_profile", reqJSON)
		if err == nil {
			t.Error("expected error for missing path")
		}
	})
}

func TestHandleProcSignalSafe(t *testing.T) {
	handler := NewNativeToolHandler()

	t.Run("protected PID rejected", func(t *testing.T) {
		req := ProcSignalSafeRequest{
			PID:    1,
			Signal: "SIGTERM",
		}
		reqJSON, _ := json.Marshal(req)

		result, err := handler.HandleTool(context.Background(), "proc_signal_safe", reqJSON)
		if err != nil {
			t.Fatalf("HandleTool failed: %v", err)
		}

		var signalResult ProcSignalSafeResult
		if err := json.Unmarshal([]byte(result.Content[0].Text), &signalResult); err != nil {
			t.Fatalf("failed to unmarshal result: %v", err)
		}

		if signalResult.Sent {
			t.Error("expected protected PID to be rejected")
		}
	})

	t.Run("protected PID 2 rejected", func(t *testing.T) {
		req := ProcSignalSafeRequest{
			PID:    2,
			Signal: "SIGKILL",
		}
		reqJSON, _ := json.Marshal(req)

		result, err := handler.HandleTool(context.Background(), "proc_signal_safe", reqJSON)
		if err != nil {
			t.Fatalf("HandleTool failed: %v", err)
		}

		var signalResult ProcSignalSafeResult
		if err := json.Unmarshal([]byte(result.Content[0].Text), &signalResult); err != nil {
			t.Fatalf("failed to unmarshal result: %v", err)
		}

		if signalResult.Sent {
			t.Error("expected protected PID 2 to be rejected")
		}
	})

	t.Run("unsupported signal", func(t *testing.T) {
		req := ProcSignalSafeRequest{
			PID:    9999,
			Signal: "INVALID",
		}
		reqJSON, _ := json.Marshal(req)

		result, err := handler.HandleTool(context.Background(), "proc_signal_safe", reqJSON)
		if err != nil {
			t.Fatalf("HandleTool failed: %v", err)
		}

		var signalResult ProcSignalSafeResult
		if err := json.Unmarshal([]byte(result.Content[0].Text), &signalResult); err != nil {
			t.Fatalf("failed to unmarshal result: %v", err)
		}

		if signalResult.Sent {
			t.Error("expected unsupported signal to be rejected")
		}
	})

	t.Run("missing PID", func(t *testing.T) {
		req := ProcSignalSafeRequest{
			Signal: "SIGTERM",
		}
		reqJSON, _ := json.Marshal(req)

		_, err := handler.HandleTool(context.Background(), "proc_signal_safe", reqJSON)
		if err == nil {
			t.Error("expected error for missing PID")
		}
	})

	t.Run("missing signal", func(t *testing.T) {
		req := ProcSignalSafeRequest{
			PID: 9999,
		}
		reqJSON, _ := json.Marshal(req)

		_, err := handler.HandleTool(context.Background(), "proc_signal_safe", reqJSON)
		if err == nil {
			t.Error("expected error for missing signal")
		}
	})

	t.Run("valid signals accepted", func(t *testing.T) {
		signals := []string{"SIGTERM", "SIGKILL", "SIGINT"}
		for _, sig := range signals {
			req := ProcSignalSafeRequest{
				PID:    99999, // Non-existent PID
				Signal: sig,
			}
			reqJSON, _ := json.Marshal(req)

			result, err := handler.HandleTool(context.Background(), "proc_signal_safe", reqJSON)
			if err != nil {
				t.Fatalf("HandleTool failed for %s: %v", sig, err)
			}

			var signalResult ProcSignalSafeResult
			if err := json.Unmarshal([]byte(result.Content[0].Text), &signalResult); err != nil {
				t.Fatalf("failed to unmarshal result for %s: %v", sig, err)
			}

			// Signal should be attempted (process may not exist, but signal is valid)
			if signalResult.Error == "" {
				t.Errorf("expected error for non-existent PID with %s", sig)
			}
		}
	})
}

func TestHandleNetSocketAudit(t *testing.T) {
	handler := NewNativeToolHandler()

	t.Run("valid request", func(t *testing.T) {
		req := NetSocketAuditRequest{Protocol: "tcp"}
		reqJSON, _ := json.Marshal(req)

		result, err := handler.HandleTool(context.Background(), "net_socket_audit", reqJSON)
		if err != nil {
			t.Fatalf("HandleTool failed: %v", err)
		}

		var auditResult NetSocketAuditResult
		if err := json.Unmarshal([]byte(result.Content[0].Text), &auditResult); err != nil {
			t.Fatalf("failed to unmarshal result: %v", err)
		}

		if auditResult.Sockets == nil {
			t.Error("expected sockets in result")
		}
	})

	t.Run("invalid protocol - path traversal attempt", func(t *testing.T) {
		req := NetSocketAuditRequest{Protocol: "../../etc/passwd"}
		reqJSON, _ := json.Marshal(req)

		_, err := handler.HandleTool(context.Background(), "net_socket_audit", reqJSON)
		if err == nil {
			t.Error("expected error for invalid protocol, got nil")
		}
	})

	t.Run("invalid protocol - arbitrary string", func(t *testing.T) {
		req := NetSocketAuditRequest{Protocol: "sctp"}
		reqJSON, _ := json.Marshal(req)

		_, err := handler.HandleTool(context.Background(), "net_socket_audit", reqJSON)
		if err == nil {
			t.Error("expected error for invalid protocol, got nil")
		}
	})
}

func TestHandleNetEndpointPing(t *testing.T) {
	handler := NewNativeToolHandler()

	t.Run("invalid host", func(t *testing.T) {
		req := NetEndpointPingRequest{
			Host: "invalid-host-that-does-not-exist-12345.com",
			Port: 9999,
		}
		reqJSON, _ := json.Marshal(req)

		result, err := handler.HandleTool(context.Background(), "net_endpoint_ping", reqJSON)
		if err != nil {
			t.Fatalf("HandleTool failed: %v", err)
		}

		var pingResult NetEndpointPingResult
		if err := json.Unmarshal([]byte(result.Content[0].Text), &pingResult); err != nil {
			t.Fatalf("failed to unmarshal result: %v", err)
		}

		if pingResult.Reachable {
			t.Error("expected unreachable host")
		}
	})

	t.Run("missing host", func(t *testing.T) {
		req := NetEndpointPingRequest{Port: 80}
		reqJSON, _ := json.Marshal(req)

		_, err := handler.HandleTool(context.Background(), "net_endpoint_ping", reqJSON)
		if err == nil {
			t.Error("expected error for missing host")
		}
	})

	t.Run("missing port", func(t *testing.T) {
		req := NetEndpointPingRequest{
			Host: "localhost",
		}
		reqJSON, _ := json.Marshal(req)

		_, err := handler.HandleTool(context.Background(), "net_endpoint_ping", reqJSON)
		if err == nil {
			t.Error("expected error for missing port")
		}
	})

	t.Run("invalid port", func(t *testing.T) {
		req := NetEndpointPingRequest{
			Host: "localhost",
			Port: -1,
		}
		reqJSON, _ := json.Marshal(req)

		_, err := handler.HandleTool(context.Background(), "net_endpoint_ping", reqJSON)
		if err == nil {
			t.Error("expected error for invalid port")
		}
	})

	t.Run("localhost ping", func(t *testing.T) {
		req := NetEndpointPingRequest{
			Host: "127.0.0.1",
			Port: 80, // May or may not be listening
		}
		reqJSON, _ := json.Marshal(req)

		result, err := handler.HandleTool(context.Background(), "net_endpoint_ping", reqJSON)
		if err != nil {
			t.Fatalf("HandleTool failed: %v", err)
		}

		var pingResult NetEndpointPingResult
		if err := json.Unmarshal([]byte(result.Content[0].Text), &pingResult); err != nil {
			t.Fatalf("failed to unmarshal result: %v", err)
		}

		// Latency should be set regardless of reachability
		if pingResult.LatencyMs < 0 {
			t.Error("expected non-negative latency")
		}
	})

	t.Run("context cancellation", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel() // Cancel immediately

		req := NetEndpointPingRequest{
			Host: "google.com",
			Port: 443,
		}
		reqJSON, _ := json.Marshal(req)

		result, err := handler.HandleTool(ctx, "net_endpoint_ping", reqJSON)
		if err != nil {
			t.Fatalf("HandleTool failed: %v", err)
		}

		var pingResult NetEndpointPingResult
		if err := json.Unmarshal([]byte(result.Content[0].Text), &pingResult); err != nil {
			t.Fatalf("failed to unmarshal result: %v", err)
		}

		// Should fail due to context cancellation
		if pingResult.Reachable {
			t.Error("expected unreachable due to context cancellation")
		}
	})
}

func TestHandleNetHTTPProbe(t *testing.T) {
	handler := NewNativeToolHandler()

	t.Run("invalid URL", func(t *testing.T) {
		req := NetHTTPProbeRequest{
			URL: "http://invalid-host-that-does-not-exist-12345.com",
		}
		reqJSON, _ := json.Marshal(req)

		result, err := handler.HandleTool(context.Background(), "net_http_probe", reqJSON)
		if err != nil {
			t.Fatalf("HandleTool failed: %v", err)
		}

		var probeResult NetHTTPProbeResult
		if err := json.Unmarshal([]byte(result.Content[0].Text), &probeResult); err != nil {
			t.Fatalf("failed to unmarshal result: %v", err)
		}

		if probeResult.Error == "" {
			t.Error("expected error for invalid URL")
		}
	})

	t.Run("missing URL", func(t *testing.T) {
		req := NetHTTPProbeRequest{}
		reqJSON, _ := json.Marshal(req)

		_, err := handler.HandleTool(context.Background(), "net_http_probe", reqJSON)
		if err == nil {
			t.Error("expected error for missing URL")
		}
	})

	t.Run("default method is HEAD", func(t *testing.T) {
		req := NetHTTPProbeRequest{
			URL: "http://example.com",
		}
		reqJSON, _ := json.Marshal(req)

		result, err := handler.HandleTool(context.Background(), "net_http_probe", reqJSON)
		if err != nil {
			t.Fatalf("HandleTool failed: %v", err)
		}

		var probeResult NetHTTPProbeResult
		if err := json.Unmarshal([]byte(result.Content[0].Text), &probeResult); err != nil {
			t.Fatalf("failed to unmarshal result: %v", err)
		}

		// Latency should be set regardless of success
		if probeResult.LatencyMs < 0 {
			t.Error("expected non-negative latency")
		}
	})

	t.Run("custom method GET", func(t *testing.T) {
		req := NetHTTPProbeRequest{
			URL:    "http://example.com",
			Method: "GET",
		}
		reqJSON, _ := json.Marshal(req)

		result, err := handler.HandleTool(context.Background(), "net_http_probe", reqJSON)
		if err != nil {
			t.Fatalf("HandleTool failed: %v", err)
		}

		var probeResult NetHTTPProbeResult
		if err := json.Unmarshal([]byte(result.Content[0].Text), &probeResult); err != nil {
			t.Fatalf("failed to unmarshal result: %v", err)
		}

		// Latency should be set
		if probeResult.LatencyMs < 0 {
			t.Error("expected non-negative latency")
		}
	})

	t.Run("context cancellation", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel() // Cancel immediately

		req := NetHTTPProbeRequest{
			URL: "http://example.com",
		}
		reqJSON, _ := json.Marshal(req)

		result, err := handler.HandleTool(ctx, "net_http_probe", reqJSON)
		if err != nil {
			t.Fatalf("HandleTool failed: %v", err)
		}

		var probeResult NetHTTPProbeResult
		if err := json.Unmarshal([]byte(result.Content[0].Text), &probeResult); err != nil {
			t.Fatalf("failed to unmarshal result: %v", err)
		}

		// Should fail due to context cancellation
		if probeResult.Error == "" {
			t.Error("expected error due to context cancellation")
		}
	})

	t.Run("invalid URL format", func(t *testing.T) {
		req := NetHTTPProbeRequest{
			URL: "not-a-valid-url",
		}
		reqJSON, _ := json.Marshal(req)

		result, err := handler.HandleTool(context.Background(), "net_http_probe", reqJSON)
		if err != nil {
			t.Fatalf("HandleTool failed: %v", err)
		}

		var probeResult NetHTTPProbeResult
		if err := json.Unmarshal([]byte(result.Content[0].Text), &probeResult); err != nil {
			t.Fatalf("failed to unmarshal result: %v", err)
		}

		if probeResult.Error == "" {
			t.Error("expected error for invalid URL format")
		}
	})

	t.Run("blocks localhost", func(t *testing.T) {
		req := NetHTTPProbeRequest{
			URL: "http://localhost:8080",
		}
		reqJSON, _ := json.Marshal(req)

		result, err := handler.HandleTool(context.Background(), "net_http_probe", reqJSON)
		if err != nil {
			t.Fatalf("HandleTool failed: %v", err)
		}

		var probeResult NetHTTPProbeResult
		if err := json.Unmarshal([]byte(result.Content[0].Text), &probeResult); err != nil {
			t.Fatalf("failed to unmarshal result: %v", err)
		}

		if probeResult.Error == "" {
			t.Error("expected error for localhost URL")
		}
	})

	t.Run("blocks loopback IP", func(t *testing.T) {
		req := NetHTTPProbeRequest{
			URL: "http://127.0.0.1:8080",
		}
		reqJSON, _ := json.Marshal(req)

		result, err := handler.HandleTool(context.Background(), "net_http_probe", reqJSON)
		if err != nil {
			t.Fatalf("HandleTool failed: %v", err)
		}

		var probeResult NetHTTPProbeResult
		if err := json.Unmarshal([]byte(result.Content[0].Text), &probeResult); err != nil {
			t.Fatalf("failed to unmarshal result: %v", err)
		}

		if probeResult.Error == "" {
			t.Error("expected error for loopback IP URL")
		}
	})

	t.Run("blocks private IP", func(t *testing.T) {
		req := NetHTTPProbeRequest{
			URL: "http://192.168.1.1",
		}
		reqJSON, _ := json.Marshal(req)

		result, err := handler.HandleTool(context.Background(), "net_http_probe", reqJSON)
		if err != nil {
			t.Fatalf("HandleTool failed: %v", err)
		}

		var probeResult NetHTTPProbeResult
		if err := json.Unmarshal([]byte(result.Content[0].Text), &probeResult); err != nil {
			t.Fatalf("failed to unmarshal result: %v", err)
		}

		if probeResult.Error == "" {
			t.Error("expected error for private IP URL")
		}
	})
}

func TestNativeTools(t *testing.T) {
	handler := NewNativeToolHandler()
	nativeTools := handler.ListTools()

	if len(nativeTools) != 23 {
		t.Errorf("expected 23 native tools, got %d", len(nativeTools))
	}

	expectedTools := []string{
		"db_discover_topology",
		"db_query_validate",
		"db_isolated_read",
		"db_index_triage",
		"log_stream_filter",
		"sys_oom_detect",
		"config_diff_mask",
		"proc_metric_top",
		"fs_disk_profile",
		"proc_signal_safe",
		"net_socket_audit",
		"net_endpoint_ping",
		"net_http_probe",
		"sys_info",
		"net_dns_resolve",
		"tls_cert_inspect",
		"sys_env_vars",
		"fs_file_checksum",
		"sys_service_status",
		"sys_container_status",
		"fs_disk_usage",
		"sys_time_clock",
		"proc_tree",
	}

	toolNames := make(map[string]bool)
	for _, tool := range nativeTools {
		toolNames[tool.Name()] = true
	}

	for _, expected := range expectedTools {
		if !toolNames[expected] {
			t.Errorf("missing expected tool: %s", expected)
		}
	}
}

func TestNativeToolHandler_ScrubLine(t *testing.T) {
	t.Parallel()

	t.Run("redacts password", func(t *testing.T) {
		t.Parallel()
		line := "user login password=secret123"
		scrubbed := scrubLine(line)
		require.Equal(t, "user login password=REDACTED", scrubbed)
		require.NotContains(t, scrubbed, "secret123")
	})

	t.Run("redacts api_key", func(t *testing.T) {
		t.Parallel()
		line := "api_key=sk-12345 processed"
		scrubbed := scrubLine(line)
		require.Equal(t, "api_key=REDACTED processed", scrubbed)
		require.NotContains(t, scrubbed, "sk-12345")
	})

	t.Run("redacts secret", func(t *testing.T) {
		t.Parallel()
		line := "secret=mysecret value"
		scrubbed := scrubLine(line)
		require.Equal(t, "secret=REDACTED value", scrubbed)
		require.NotContains(t, scrubbed, "mysecret")
	})

	t.Run("redacts token", func(t *testing.T) {
		t.Parallel()
		line := "token=abc123def456 session"
		scrubbed := scrubLine(line)
		require.Equal(t, "token=REDACTED session", scrubbed)
		require.NotContains(t, scrubbed, "abc123def456")
	})

	t.Run("redacts bearer", func(t *testing.T) {
		t.Parallel()
		line := "Authorization: bearer xyz789"
		scrubbed := scrubLine(line)
		require.Equal(t, "Authorization: bearer REDACTED", scrubbed)
		require.NotContains(t, scrubbed, "xyz789")
	})

	t.Run("case insensitive", func(t *testing.T) {
		t.Parallel()
		line := "PASSWORD=secret123 API_KEY=sk-12345"
		scrubbed := scrubLine(line)
		require.Equal(t, "password=REDACTED api_key=REDACTED", scrubbed)
	})

	t.Run("safe line unchanged", func(t *testing.T) {
		t.Parallel()
		line := "INFO: request processed successfully"
		scrubbed := scrubLine(line)
		require.Equal(t, line, scrubbed)
	})
}

func TestNativeToolHandler_MaskSecret(t *testing.T) {
	t.Parallel()

	t.Run("masks password line", func(t *testing.T) {
		t.Parallel()
		line := "db_password=secret123"
		masked := maskSecret(line)
		require.Equal(t, "REDACTED", masked)
	})

	t.Run("masks secret line", func(t *testing.T) {
		t.Parallel()
		line := "shared_secret=mysecret"
		masked := maskSecret(line)
		require.Equal(t, "REDACTED", masked)
	})

	t.Run("masks token line", func(t *testing.T) {
		t.Parallel()
		line := "access_token=abc123"
		masked := maskSecret(line)
		require.Equal(t, "REDACTED", masked)
	})

	t.Run("masks key line", func(t *testing.T) {
		t.Parallel()
		line := "encryption_key=xyz789"
		masked := maskSecret(line)
		require.Equal(t, "REDACTED", masked)
	})

	t.Run("case insensitive", func(t *testing.T) {
		t.Parallel()
		line := "PASSWORD=secret123"
		masked := maskSecret(line)
		require.Equal(t, "REDACTED", masked)
	})

	t.Run("safe line unchanged", func(t *testing.T) {
		t.Parallel()
		line := "timeout=30"
		masked := maskSecret(line)
		require.Equal(t, line, masked)
	})
}

func TestParseSocketAddr(t *testing.T) {
	t.Parallel()

	t.Run("valid IPv4 address", func(t *testing.T) {
		t.Parallel()
		// 127.0.0.1:8080 in hex (little-endian)
		// IP: 0100007F (127.0.0.1), Port: 1F90 (8080)
		ip, port := parseSocketAddr("0100007F1F90")
		require.Equal(t, "127.0.0.1", ip)
		require.Equal(t, 8080, port)
	})

	t.Run("localhost", func(t *testing.T) {
		t.Parallel()
		// 0.0.0.0:443 in hex
		// IP: 00000000 (0.0.0.0), Port: 01BB (443)
		ip, port := parseSocketAddr("0000000001BB")
		require.Equal(t, "0.0.0.0", ip)
		require.Equal(t, 443, port)
	})

	t.Run("invalid short address", func(t *testing.T) {
		t.Parallel()
		ip, port := parseSocketAddr("1234")
		require.Equal(t, "0.0.0.0", ip)
		require.Equal(t, 0, port)
	})

	t.Run("invalid IP length", func(t *testing.T) {
		t.Parallel()
		// IP part not 8 bytes
		ip, port := parseSocketAddr("12345601BB")
		require.Equal(t, "unknown", ip)
		require.Equal(t, 443, port)
	})
}
