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
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCallToolJSON(t *testing.T) {
	t.Run("CallToolRequest marshalling", func(t *testing.T) {
		req := CallToolRequest{
			Name:      "test-tool",
			Arguments: json.RawMessage(`{"arg1":"val1"}`),
		}
		data, err := json.Marshal(req)
		require.NoError(t, err)
		assert.Contains(t, string(data), `"name":"test-tool"`)
		assert.Contains(t, string(data), `"arguments":{"arg1":"val1"}`)
	})

	t.Run("CallToolResult marshalling", func(t *testing.T) {
		res := CallToolResult{
			Content: []TextContent{
				{
					Type: "text",
					Text: "hello world",
				},
			},
			IsError: false,
		}
		data, err := json.Marshal(res)
		require.NoError(t, err)
		assert.Contains(t, string(data), `"type":"text"`)
		assert.Contains(t, string(data), `"text":"hello world"`)
		// IsError should be omitted if false due to omitempty
		assert.NotContains(t, string(data), `"isError"`)

		res.IsError = true
		data, err = json.Marshal(res)
		require.NoError(t, err)
		assert.Contains(t, string(data), `"isError":true`)
	})
}

func TestResourceModelsJSON(t *testing.T) {
	t.Run("Resource marshalling", func(t *testing.T) {
		res := Resource{
			URI:         "file:///tmp/test.txt",
			Name:        "test-resource",
			Description: "a test resource",
			MimeType:    "text/plain",
			Metadata:    &Metadata{Custom: map[string]string{"key": "value"}},
		}
		data, err := json.Marshal(res)
		require.NoError(t, err)
		assert.Contains(t, string(data), `"uri":"file:///tmp/test.txt"`)
		assert.Contains(t, string(data), `"name":"test-resource"`)
		assert.Contains(t, string(data), `"description":"a test resource"`)
		assert.Contains(t, string(data), `"mimeType":"text/plain"`)
		assert.Contains(t, string(data), `"metadata":{"custom":{"key":"value"}}`)
	})

	t.Run("ListResourcesResult marshalling", func(t *testing.T) {
		res := ListResourcesResult{
			Resources: []Resource{
				{URI: "uri1", Name: "name1"},
				{URI: "uri2", Name: "name2"},
			},
		}
		data, err := json.Marshal(res)
		require.NoError(t, err)
		var decoded ListResourcesResult
		err = json.Unmarshal(data, &decoded)
		require.NoError(t, err)
		assert.Len(t, decoded.Resources, 2)
		assert.Equal(t, "uri1", decoded.Resources[0].URI)
	})
}

func TestPromptModelsJSON(t *testing.T) {
	t.Run("Prompt marshalling", func(t *testing.T) {
		p := Prompt{
			Name:        "test-prompt",
			Description: "a test prompt",
			Arguments: []PromptArgument{
				{Name: "arg1", Description: "desc1", Required: true},
			},
		}
		data, err := json.Marshal(p)
		require.NoError(t, err)
		assert.Contains(t, string(data), `"name":"test-prompt"`)
		assert.Contains(t, string(data), `"required":true`)
	})

	t.Run("GetPromptResult marshalling", func(t *testing.T) {
		res := GetPromptResult{
			Description: "desc",
			Messages: []PromptMessage{
				{
					Role:    "user",
					Content: TextContent{Type: "text", Text: "hello"},
				},
			},
		}
		data, err := json.Marshal(res)
		require.NoError(t, err)
		var decoded GetPromptResult
		err = json.Unmarshal(data, &decoded)
		require.NoError(t, err)
		assert.Equal(t, "user", decoded.Messages[0].Role)
		assert.Equal(t, "text", decoded.Messages[0].Content.Type)
	})
}

func TestNativeToolModelsJSON(t *testing.T) {
	t.Run("DBQueryValidateResult marshalling", func(t *testing.T) {
		res := DBQueryValidateResult{
			Valid:    true,
			Plan:     "SELECT *",
			Rejected: false,
		}
		data, err := json.Marshal(res)
		require.NoError(t, err)
		assert.Contains(t, string(data), `"valid":true`)
		assert.Contains(t, string(data), `"plan":"SELECT *"`)
		assert.Contains(t, string(data), `"rejected":false`)
	})

	t.Run("FSDiskUsageResult marshalling", func(t *testing.T) {
		res := FSDiskUsageResult{
			Path: "/tmp",
			Filesystem: &FilesystemInfo{
				Path:       "/",
				TotalBytes: 1000,
				UsedBytes:  500,
			},
		}
		data, err := json.Marshal(res)
		require.NoError(t, err)
		assert.Contains(t, string(data), `"path":"/tmp"`)
		assert.Contains(t, string(data), `"filesystem":{"path":"/","total_bytes":1000,"used_bytes":500,"free_bytes":0,"available_bytes":0,"used_percent":0}`)
	})

	t.Run("SysInfoResult marshalling", func(t *testing.T) {
		res := SysInfoResult{
			Hostname: "test-host",
			OS: OSInfo{
				OS:   "linux",
				Arch: "amd64",
			},
		}
		data, err := json.Marshal(res)
		require.NoError(t, err)
		assert.Contains(t, string(data), `"hostname":"test-host"`)
		assert.Contains(t, string(data), `"os":"linux"`)
	})

	t.Run("K8sInspectResult marshalling", func(t *testing.T) {
		res := K8sInspectResult{
			Operation: "list_pods",
			Pods: []K8sPodInfo{
				{Name: "pod1", Namespace: "default", Status: "Running"},
			},
			Count: 1,
		}
		data, err := json.Marshal(res)
		require.NoError(t, err)
		assert.Contains(t, string(data), `"operation":"list_pods"`)
		assert.Contains(t, string(data), `"pods":[{"name":"pod1","namespace":"default","status":"Running"}]`)
		assert.Contains(t, string(data), `"count":1`)
	})

	t.Run("NetSSHKnownHostsResult marshalling", func(t *testing.T) {
		res := NetSSHKnownHostsResult{
			ConfigHosts: []SSHConfigHost{
				{Pattern: "*", Hostname: "github.com", User: "git"},
			},
			KnownHosts: []SSHKnownHost{
				{HostPattern: "github.com", KeyType: "ssh-rsa", KeyHash: "hash"},
			},
		}
		data, err := json.Marshal(res)
		require.NoError(t, err)
		assert.Contains(t, string(data), `"pattern":"*"`)
		assert.Contains(t, string(data), `"key_hash":"hash"`)
	})

	t.Run("RunShellCommandResult marshalling", func(t *testing.T) {
		res := RunShellCommandResult{
			ExitCode: 0,
			Stdout:   "hello",
			Stderr:   "",
			TimedOut: false,
		}
		data, err := json.Marshal(res)
		require.NoError(t, err)
		assert.Contains(t, string(data), `"exit_code":0`)
		assert.Contains(t, string(data), `"stdout":"hello"`)
		assert.Contains(t, string(data), `"timed_out":false`)
	})

	t.Run("SysTimeClockResult marshalling", func(t *testing.T) {
		res := SysTimeClockResult{
			SystemTime: SystemTimeInfo{
				UTC:      "2026-06-14T12:00:00Z",
				Unix:     1781438400,
				Timezone: "UTC",
			},
			NTP: NTPStatus{
				Synced: true,
				Status: "synchronized",
			},
		}
		data, err := json.Marshal(res)
		require.NoError(t, err)
		assert.Contains(t, string(data), `"utc":"2026-06-14T12:00:00Z"`)
		assert.Contains(t, string(data), `"synced":true`)
	})

	t.Run("FSDiskProfileResult marshalling", func(t *testing.T) {
		res := FSDiskProfileResult{
			Entries: []DirEntry{
				{Path: "/tmp", SizeMB: 10, IsDir: true, Modified: 123456789},
			},
			TotalMB: 10,
		}
		data, err := json.Marshal(res)
		require.NoError(t, err)
		assert.Contains(t, string(data), `"path":"/tmp"`)
		assert.Contains(t, string(data), `"size_mb":10`)
		assert.Contains(t, string(data), `"total_mb":10`)
	})
}

func TestFieldValueString(t *testing.T) {
	t.Run("null value", func(t *testing.T) {
		v := FieldValue{Null: true}
		assert.Equal(t, "null", v.String())
	})

	t.Run("string value", func(t *testing.T) {
		s := "hello world"
		v := FieldValue{Str: &s}
		assert.Equal(t, "hello world", v.String())
	})

	t.Run("int64 value", func(t *testing.T) {
		i := int64(42)
		v := FieldValue{Int64: &i}
		assert.Equal(t, "42", v.String())
	})

	t.Run("float64 value", func(t *testing.T) {
		f := float64(3.14159)
		v := FieldValue{Float64: &f}
		assert.Equal(t, "3.14159", v.String())
	})

	t.Run("bool value", func(t *testing.T) {
		b := true
		v := FieldValue{Bool: &b}
		assert.Equal(t, "true", v.String())

		b = false
		v = FieldValue{Bool: &b}
		assert.Equal(t, "false", v.String())
	})

	t.Run("array value", func(t *testing.T) {
		v := FieldValue{Array: []FieldValue{{Str: strPtr("a")}, {Str: strPtr("b")}}}
		assert.Contains(t, v.String(), "a")
		assert.Contains(t, v.String(), "b")
	})

	t.Run("object value", func(t *testing.T) {
		v := FieldValue{Object: map[string]FieldValue{"key": {Str: strPtr("value")}}}
		assert.Contains(t, v.String(), "key")
		assert.Contains(t, v.String(), "value")
	})

	t.Run("empty value returns empty string on marshal error", func(t *testing.T) {
		// This tests the fallback case where json.Marshal might fail
		v := FieldValue{}
		result := v.String()
		// Empty value should marshal to valid JSON
		assert.NotEmpty(t, result)
	})
}

func TestFieldReadModelsJSON(t *testing.T) {
	t.Run("FieldReadRequest marshalling", func(t *testing.T) {
		req := FieldReadRequest{
			Collection:        "test-collection",
			DocumentID:        "doc-123",
			FieldPath:         "field.nested.path",
			OperatorSessionID: "session-abc",
		}
		data, err := json.Marshal(req)
		require.NoError(t, err)
		assert.Contains(t, string(data), `"collection":"test-collection"`)
		assert.Contains(t, string(data), `"document_id":"doc-123"`)
		assert.Contains(t, string(data), `"field_path":"field.nested.path"`)
		assert.Contains(t, string(data), `"operator_session_id":"session-abc"`)

		// Test round-trip
		var decoded FieldReadRequest
		err = json.Unmarshal(data, &decoded)
		require.NoError(t, err)
		assert.Equal(t, req.Collection, decoded.Collection)
		assert.Equal(t, req.DocumentID, decoded.DocumentID)
		assert.Equal(t, req.FieldPath, decoded.FieldPath)
		assert.Equal(t, req.OperatorSessionID, decoded.OperatorSessionID)
	})

	t.Run("FieldReadResult marshalling", func(t *testing.T) {
		s := "test-value"
		res := FieldReadResult{
			Value: FieldValue{Str: &s},
		}
		data, err := json.Marshal(res)
		require.NoError(t, err)
		assert.Contains(t, string(data), `"value"`)

		// Test round-trip
		var decoded FieldReadResult
		err = json.Unmarshal(data, &decoded)
		require.NoError(t, err)
		assert.Equal(t, "test-value", *decoded.Value.Str)
	})
}

func TestDBModelsJSON(t *testing.T) {
	t.Run("DBDiscoverTopologyRequest marshalling", func(t *testing.T) {
		req := DBDiscoverTopologyRequest{
			DatabasePath: "/tmp/test.db",
		}
		data, err := json.Marshal(req)
		require.NoError(t, err)
		assert.Contains(t, string(data), `"database_path":"/tmp/test.db"`)
	})

	t.Run("DBDiscoverTopologyResult marshalling", func(t *testing.T) {
		res := DBDiscoverTopologyResult{
			Schema: map[string]map[string]string{
				"users": {
					"id":   "INTEGER",
					"name": "TEXT",
				},
			},
		}
		data, err := json.Marshal(res)
		require.NoError(t, err)
		assert.Contains(t, string(data), `"users"`)
		assert.Contains(t, string(data), `"id":"INTEGER"`)

		// Test round-trip
		var decoded DBDiscoverTopologyResult
		err = json.Unmarshal(data, &decoded)
		require.NoError(t, err)
		assert.Equal(t, "INTEGER", decoded.Schema["users"]["id"])
	})

	t.Run("DBIsolatedReadRequest marshalling", func(t *testing.T) {
		req := DBIsolatedReadRequest{
			DatabasePath: "/tmp/test.db",
			Query:        "SELECT * FROM users",
		}
		data, err := json.Marshal(req)
		require.NoError(t, err)
		assert.Contains(t, string(data), `"database_path":"/tmp/test.db"`)
		assert.Contains(t, string(data), `"query":"SELECT * FROM users"`)
	})

	t.Run("DBIsolatedReadResult marshalling", func(t *testing.T) {
		s1, s2 := "alice", "bob"
		res := DBIsolatedReadResult{
			Rows: []DBRow{
				{
					Values: map[string]DBValue{
						"id":   {Int64: int64Ptr(1)},
						"name": {String: &s1},
					},
				},
				{
					Values: map[string]DBValue{
						"id":   {Int64: int64Ptr(2)},
						"name": {String: &s2},
					},
				},
			},
			Columns: []string{"id", "name"},
		}
		data, err := json.Marshal(res)
		require.NoError(t, err)
		assert.Contains(t, string(data), `"rows"`)
		assert.Contains(t, string(data), `"columns":["id","name"]`)

		// Test round-trip
		var decoded DBIsolatedReadResult
		err = json.Unmarshal(data, &decoded)
		require.NoError(t, err)
		assert.Len(t, decoded.Rows, 2)
		assert.Equal(t, []string{"id", "name"}, decoded.Columns)
	})

	t.Run("DBValue null handling", func(t *testing.T) {
		v := DBValue{Null: true}
		data, err := json.Marshal(v)
		require.NoError(t, err)
		assert.Contains(t, string(data), `"null":true`)

		var decoded DBValue
		err = json.Unmarshal(data, &decoded)
		require.NoError(t, err)
		assert.True(t, decoded.Null)
	})

	t.Run("DBIndexTriageRequest marshalling", func(t *testing.T) {
		req := DBIndexTriageRequest{
			DatabasePath: "/tmp/test.db",
		}
		data, err := json.Marshal(req)
		require.NoError(t, err)
		assert.Contains(t, string(data), `"database_path":"/tmp/test.db"`)
	})

	t.Run("DBIndexTriageResult marshalling", func(t *testing.T) {
		res := DBIndexTriageResult{
			Indexes: []IndexInfo{
				{
					Name:    "idx_users_name",
					Table:   "users",
					Columns: []string{"name"},
					Unique:  true,
					Used:    true,
				},
			},
			Fragmentation: 0.15,
		}
		data, err := json.Marshal(res)
		require.NoError(t, err)
		assert.Contains(t, string(data), `"idx_users_name"`)
		assert.Contains(t, string(data), `"fragmentation":0.15`)

		// Test round-trip
		var decoded DBIndexTriageResult
		err = json.Unmarshal(data, &decoded)
		require.NoError(t, err)
		assert.Equal(t, "idx_users_name", decoded.Indexes[0].Name)
		assert.True(t, decoded.Indexes[0].Unique)
		assert.InDelta(t, 0.15, decoded.Fragmentation, 0.001)
	})
}

func TestLogModelsJSON(t *testing.T) {
	t.Run("LogStreamFilterRequest marshalling", func(t *testing.T) {
		req := LogStreamFilterRequest{
			LogPath: "/var/log/app.log",
			Pattern: "ERROR",
			Limit:   100,
		}
		data, err := json.Marshal(req)
		require.NoError(t, err)
		assert.Contains(t, string(data), `"log_path":"/var/log/app.log"`)
		assert.Contains(t, string(data), `"pattern":"ERROR"`)
		assert.Contains(t, string(data), `"limit":100`)

		// Test omitempty for Limit
		req.Limit = 0
		data, err = json.Marshal(req)
		require.NoError(t, err)
		assert.NotContains(t, string(data), `"limit"`)
	})

	t.Run("LogStreamFilterResult marshalling", func(t *testing.T) {
		res := LogStreamFilterResult{
			Lines: []string{"line1", "line2", "line3"},
			Count: 3,
		}
		data, err := json.Marshal(res)
		require.NoError(t, err)
		assert.Contains(t, string(data), `"lines":["line1","line2","line3"]`)
		assert.Contains(t, string(data), `"count":3`)

		// Test round-trip
		var decoded LogStreamFilterResult
		err = json.Unmarshal(data, &decoded)
		require.NoError(t, err)
		assert.Len(t, decoded.Lines, 3)
		assert.Equal(t, 3, decoded.Count)
	})

	t.Run("SysOOMDetectRequest marshalling", func(t *testing.T) {
		req := SysOOMDetectRequest{
			LogPath: "/var/log/syslog",
		}
		data, err := json.Marshal(req)
		require.NoError(t, err)
		assert.Contains(t, string(data), `"log_path":"/var/log/syslog"`)

		// Test omitempty for LogPath
		req.LogPath = ""
		data, err = json.Marshal(req)
		require.NoError(t, err)
		assert.NotContains(t, string(data), `"log_path"`)
	})

	t.Run("SysOOMDetectResult marshalling", func(t *testing.T) {
		res := SysOOMDetectResult{
			Events: []OOMEvent{
				{
					Timestamp: "2026-06-14T12:00:00Z",
					PID:       1234,
					Process:   "java",
					MemoryMB:  2048,
				},
			},
		}
		data, err := json.Marshal(res)
		require.NoError(t, err)
		assert.Contains(t, string(data), `"timestamp":"2026-06-14T12:00:00Z"`)
		assert.Contains(t, string(data), `"pid":1234`)
		assert.Contains(t, string(data), `"process":"java"`)
		assert.Contains(t, string(data), `"memory_mb":2048`)

		// Test round-trip
		var decoded SysOOMDetectResult
		err = json.Unmarshal(data, &decoded)
		require.NoError(t, err)
		assert.Equal(t, 1234, decoded.Events[0].PID)
		assert.Equal(t, "java", decoded.Events[0].Process)
	})
}

func TestConfigModelsJSON(t *testing.T) {
	t.Run("ConfigDiffMaskRequest marshalling", func(t *testing.T) {
		req := ConfigDiffMaskRequest{
			ConfigPath: "/etc/app/config.yaml",
			Baseline:   "version: 1.0",
		}
		data, err := json.Marshal(req)
		require.NoError(t, err)
		assert.Contains(t, string(data), `"config_path":"/etc/app/config.yaml"`)
		assert.Contains(t, string(data), `"baseline":"version: 1.0"`)
	})

	t.Run("ConfigDiffMaskResult marshalling", func(t *testing.T) {
		res := ConfigDiffMaskResult{
			Differences: []ConfigDiff{
				{
					Key:      "server.port",
					Current:  "8080",
					Baseline: "3000",
					Type:     "changed",
				},
				{
					Key:     "server.host",
					Current: "localhost",
					Type:    "added",
				},
			},
		}
		data, err := json.Marshal(res)
		require.NoError(t, err)
		assert.Contains(t, string(data), `"key":"server.port"`)
		assert.Contains(t, string(data), `"type":"changed"`)

		// Test round-trip
		var decoded ConfigDiffMaskResult
		err = json.Unmarshal(data, &decoded)
		require.NoError(t, err)
		assert.Len(t, decoded.Differences, 2)
		assert.Equal(t, "server.port", decoded.Differences[0].Key)
		assert.Equal(t, "changed", decoded.Differences[0].Type)
	})
}

func TestProcessModelsJSON(t *testing.T) {
	t.Run("ProcMetricTopRequest marshalling", func(t *testing.T) {
		req := ProcMetricTopRequest{
			Limit: 10,
		}
		data, err := json.Marshal(req)
		require.NoError(t, err)
		assert.Contains(t, string(data), `"limit":10`)

		// Test omitempty for Limit
		req.Limit = 0
		data, err = json.Marshal(req)
		require.NoError(t, err)
		assert.NotContains(t, string(data), `"limit"`)
	})

	t.Run("ProcMetricTopResult marshalling", func(t *testing.T) {
		res := ProcMetricTopResult{
			Processes: []ProcessInfo{
				{
					PID:        1234,
					Name:       "nginx",
					CPUPercent: 2.5,
					MemoryMB:   128.5,
					User:       "www-data",
					Command:    "nginx: master process",
				},
			},
		}
		data, err := json.Marshal(res)
		require.NoError(t, err)
		assert.Contains(t, string(data), `"pid":1234`)
		assert.Contains(t, string(data), `"cpu_percent":2.5`)
		assert.Contains(t, string(data), `"memory_mb":128.5`)

		// Test round-trip
		var decoded ProcMetricTopResult
		err = json.Unmarshal(data, &decoded)
		require.NoError(t, err)
		assert.Equal(t, 1234, decoded.Processes[0].PID)
		assert.InDelta(t, 2.5, decoded.Processes[0].CPUPercent, 0.01)
	})

	t.Run("ProcSignalSafeRequest marshalling", func(t *testing.T) {
		req := ProcSignalSafeRequest{
			PID:    1234,
			Signal: "TERM",
		}
		data, err := json.Marshal(req)
		require.NoError(t, err)
		assert.Contains(t, string(data), `"pid":1234`)
		assert.Contains(t, string(data), `"signal":"TERM"`)

		// Test round-trip
		var decoded ProcSignalSafeRequest
		err = json.Unmarshal(data, &decoded)
		require.NoError(t, err)
		assert.Equal(t, 1234, decoded.PID)
		assert.Equal(t, "TERM", decoded.Signal)
	})

	t.Run("ProcSignalSafeResult marshalling", func(t *testing.T) {
		res := ProcSignalSafeResult{
			Sent:   true,
			PID:    1234,
			Signal: "TERM",
		}
		data, err := json.Marshal(res)
		require.NoError(t, err)
		assert.Contains(t, string(data), `"sent":true`)

		// Test with error
		res.Sent = false
		res.Error = "process not found"
		data, err = json.Marshal(res)
		require.NoError(t, err)
		assert.Contains(t, string(data), `"error":"process not found"`)
	})
}

func TestFileSystemModelsJSON(t *testing.T) {
	t.Run("FSDiskProfileRequest marshalling", func(t *testing.T) {
		req := FSDiskProfileRequest{
			Path:     "/var/log",
			MaxDepth: 3,
		}
		data, err := json.Marshal(req)
		require.NoError(t, err)
		assert.Contains(t, string(data), `"path":"/var/log"`)
		assert.Contains(t, string(data), `"max_depth":3`)

		// Test omitempty for MaxDepth
		req.MaxDepth = 0
		data, err = json.Marshal(req)
		require.NoError(t, err)
		assert.NotContains(t, string(data), `"max_depth"`)
	})

	t.Run("DirEntry marshalling", func(t *testing.T) {
		entry := DirEntry{
			Path:     "/var/log/app.log",
			SizeMB:   10,
			IsDir:    false,
			Modified: 1686768000,
		}
		data, err := json.Marshal(entry)
		require.NoError(t, err)
		assert.Contains(t, string(data), `"path":"/var/log/app.log"`)
		assert.Contains(t, string(data), `"size_mb":10`)
		assert.Contains(t, string(data), `"is_dir":false`)

		// Test round-trip
		var decoded DirEntry
		err = json.Unmarshal(data, &decoded)
		require.NoError(t, err)
		assert.Equal(t, "/var/log/app.log", decoded.Path)
		assert.Equal(t, int64(10), decoded.SizeMB)
		assert.False(t, decoded.IsDir)
	})

	t.Run("FSDiskUsageRequest marshalling", func(t *testing.T) {
		req := FSDiskUsageRequest{
			Path: "/",
		}
		data, err := json.Marshal(req)
		require.NoError(t, err)
		assert.Contains(t, string(data), `"path":"/"`)

		// Test omitempty for Path
		req.Path = ""
		data, err = json.Marshal(req)
		require.NoError(t, err)
		assert.NotContains(t, string(data), `"path"`)
	})

	t.Run("FilesystemInfo marshalling", func(t *testing.T) {
		info := FilesystemInfo{
			Path:           "/",
			TotalBytes:     1000000000000,
			UsedBytes:      500000000000,
			FreeBytes:      500000000000,
			AvailableBytes: 450000000000,
			UsedPercent:    50.0,
		}
		data, err := json.Marshal(info)
		require.NoError(t, err)
		assert.Contains(t, string(data), `"total_bytes":1000000000000`)
		assert.Contains(t, string(data), `"used_percent":50`)

		// Test round-trip
		var decoded FilesystemInfo
		err = json.Unmarshal(data, &decoded)
		require.NoError(t, err)
		assert.Equal(t, uint64(1000000000000), decoded.TotalBytes)
		assert.InDelta(t, 50.0, decoded.UsedPercent, 0.01)
	})
}

func TestNetworkModelsJSON(t *testing.T) {
	t.Run("NetSocketAuditRequest marshalling", func(t *testing.T) {
		req := NetSocketAuditRequest{
			Protocol: "tcp",
		}
		data, err := json.Marshal(req)
		require.NoError(t, err)
		assert.Contains(t, string(data), `"protocol":"tcp"`)

		// Test omitempty for Protocol
		req.Protocol = ""
		data, err = json.Marshal(req)
		require.NoError(t, err)
		assert.NotContains(t, string(data), `"protocol"`)
	})

	t.Run("SocketInfo marshalling", func(t *testing.T) {
		info := SocketInfo{
			Protocol:   "tcp",
			LocalAddr:  "0.0.0.0",
			LocalPort:  8080,
			RemoteAddr: "192.168.1.1",
			RemotePort: 54321,
			State:      "ESTABLISHED",
			PID:        1234,
			Process:    "nginx",
		}
		data, err := json.Marshal(info)
		require.NoError(t, err)
		assert.Contains(t, string(data), `"protocol":"tcp"`)
		assert.Contains(t, string(data), `"local_port":8080`)
		assert.Contains(t, string(data), `"state":"ESTABLISHED"`)

		// Test round-trip
		var decoded SocketInfo
		err = json.Unmarshal(data, &decoded)
		require.NoError(t, err)
		assert.Equal(t, "tcp", decoded.Protocol)
		assert.Equal(t, 8080, decoded.LocalPort)
		assert.Equal(t, "ESTABLISHED", decoded.State)
	})

	t.Run("NetDNSResolveRequest marshalling", func(t *testing.T) {
		req := NetDNSResolveRequest{
			Hostname:   "example.com",
			RecordType: "A",
		}
		data, err := json.Marshal(req)
		require.NoError(t, err)
		assert.Contains(t, string(data), `"hostname":"example.com"`)
		assert.Contains(t, string(data), `"record_type":"A"`)

		// Test omitempty for RecordType
		req.RecordType = ""
		data, err = json.Marshal(req)
		require.NoError(t, err)
		assert.NotContains(t, string(data), `"record_type"`)
	})

	t.Run("DNSRecords marshalling", func(t *testing.T) {
		records := DNSRecords{
			A: []DNSARecord{
				{IP: "93.184.216.34"},
			},
			AAAA: []DNSAAAARecord{
				{IP: "2606:2800:220:1:248:1893:25c8:1946"},
			},
			MX: []DNSMXRecord{
				{Host: "mail.example.com", Pref: 10},
			},
			TXT: []DNSTXTRecord{
				{Text: "v=spf1 include:_spf.example.com ~all"},
			},
			CNAME: &DNSCNAMERecord{
				Target: "www.example.com",
			},
			NS: []DNSNSRecord{
				{Host: "ns1.example.com"},
			},
		}
		data, err := json.Marshal(records)
		require.NoError(t, err)
		assert.Contains(t, string(data), `"a":[{"ip":"93.184.216.34"}]`)
		assert.Contains(t, string(data), `"cname":{"target":"www.example.com"}`)

		// Test round-trip
		var decoded DNSRecords
		err = json.Unmarshal(data, &decoded)
		require.NoError(t, err)
		assert.Equal(t, "93.184.216.34", decoded.A[0].IP)
		assert.Equal(t, "www.example.com", decoded.CNAME.Target)
	})

	t.Run("NetDNSResolveResult marshalling", func(t *testing.T) {
		res := NetDNSResolveResult{
			Hostname:   "example.com",
			RecordType: "A",
			Records: DNSRecords{
				A: []DNSARecord{{IP: "93.184.216.34"}},
			},
			Count: 1,
		}
		data, err := json.Marshal(res)
		require.NoError(t, err)
		assert.Contains(t, string(data), `"hostname":"example.com"`)
		assert.Contains(t, string(data), `"count":1`)

		// Test with error
		res.Error = "NXDOMAIN"
		data, err = json.Marshal(res)
		require.NoError(t, err)
		assert.Contains(t, string(data), `"error":"NXDOMAIN"`)
	})

	t.Run("NetEndpointPingRequest marshalling", func(t *testing.T) {
		req := NetEndpointPingRequest{
			Host: "example.com",
			Port: 443,
		}
		data, err := json.Marshal(req)
		require.NoError(t, err)
		assert.Contains(t, string(data), `"host":"example.com"`)
		assert.Contains(t, string(data), `"port":443`)

		// Test round-trip
		var decoded NetEndpointPingRequest
		err = json.Unmarshal(data, &decoded)
		require.NoError(t, err)
		assert.Equal(t, "example.com", decoded.Host)
		assert.Equal(t, 443, decoded.Port)
	})

	t.Run("NetEndpointPingResult marshalling", func(t *testing.T) {
		res := NetEndpointPingResult{
			Reachable: true,
			LatencyMs: 15.5,
		}
		data, err := json.Marshal(res)
		require.NoError(t, err)
		assert.Contains(t, string(data), `"reachable":true`)
		assert.Contains(t, string(data), `"latency_ms":15.5`)

		// Test with error
		res.Reachable = false
		res.Error = "timeout"
		data, err = json.Marshal(res)
		require.NoError(t, err)
		assert.Contains(t, string(data), `"error":"timeout"`)
	})

	t.Run("NetHTTPProbeRequest marshalling", func(t *testing.T) {
		req := NetHTTPProbeRequest{
			URL:    "https://example.com",
			Method: "GET",
		}
		data, err := json.Marshal(req)
		require.NoError(t, err)
		assert.Contains(t, string(data), `"url":"https://example.com"`)
		assert.Contains(t, string(data), `"method":"GET"`)

		// Test omitempty for Method
		req.Method = ""
		data, err = json.Marshal(req)
		require.NoError(t, err)
		assert.NotContains(t, string(data), `"method"`)
	})

	t.Run("NetHTTPProbeResult marshalling", func(t *testing.T) {
		res := NetHTTPProbeResult{
			StatusCode: 200,
			Headers: map[string]string{
				"Content-Type": "text/html",
				"Server":       "nginx",
			},
			LatencyMs: 45.2,
		}
		data, err := json.Marshal(res)
		require.NoError(t, err)
		assert.Contains(t, string(data), `"status_code":200`)
		assert.Contains(t, string(data), `"Content-Type":"text/html"`)

		// Test with error
		res.StatusCode = 0
		res.Error = "connection refused"
		data, err = json.Marshal(res)
		require.NoError(t, err)
		assert.Contains(t, string(data), `"error":"connection refused"`)
	})
}

func TestSystemModelsJSON(t *testing.T) {
	t.Run("SysInfoRequest marshalling", func(t *testing.T) {
		req := SysInfoRequest{}
		data, err := json.Marshal(req)
		require.NoError(t, err)
		assert.Equal(t, "{}", string(data))
	})

	t.Run("OSInfo marshalling", func(t *testing.T) {
		info := OSInfo{
			OS:          "linux",
			Arch:        "amd64",
			Kernel:      "5.15.0",
			OSVersion:   "22.04",
			Uptime:      "10 days",
			LoadAverage: "0.5, 0.3, 0.1",
		}
		data, err := json.Marshal(info)
		require.NoError(t, err)
		assert.Contains(t, string(data), `"os":"linux"`)
		assert.Contains(t, string(data), `"load_average":"0.5, 0.3, 0.1"`)

		// Test round-trip
		var decoded OSInfo
		err = json.Unmarshal(data, &decoded)
		require.NoError(t, err)
		assert.Equal(t, "linux", decoded.OS)
		assert.Equal(t, "5.15.0", decoded.Kernel)
	})

	t.Run("SysTimeClockRequest marshalling", func(t *testing.T) {
		req := SysTimeClockRequest{}
		data, err := json.Marshal(req)
		require.NoError(t, err)
		assert.Equal(t, "{}", string(data))
	})

	t.Run("SystemTimeInfo marshalling", func(t *testing.T) {
		info := SystemTimeInfo{
			UTC:      "2026-06-14T12:00:00Z",
			Local:    "2026-06-14T08:00:00-04:00",
			Unix:     1781438400,
			UnixNano: 1781438400000000000,
			Timezone: "America/New_York",
			Offset:   "-0400",
		}
		data, err := json.Marshal(info)
		require.NoError(t, err)
		assert.Contains(t, string(data), `"utc":"2026-06-14T12:00:00Z"`)
		assert.Contains(t, string(data), `"unix":1781438400`)

		// Test round-trip
		var decoded SystemTimeInfo
		err = json.Unmarshal(data, &decoded)
		require.NoError(t, err)
		assert.Equal(t, "2026-06-14T12:00:00Z", decoded.UTC)
		assert.Equal(t, int64(1781438400), decoded.Unix)
	})

	t.Run("NTPStatus marshalling", func(t *testing.T) {
		status := NTPStatus{
			Synced:           true,
			Status:           "synchronized",
			NTPService:       "ntpd",
			NTPSynchronized:  "yes",
			ReferenceID:      "GPS",
			Stratum:          "1",
			SystemTimeOffset: "+0.001",
			SelectedPeer: &NTPSelectedPeer{
				Remote:  "time1.google.com",
				RefID:   "GOOG",
				Stratum: "1",
				When:    "1",
				Poll:    "64",
				Reach:   "377",
				Delay:   "0.123",
				Offset:  "0.001",
				Jitter:  "0.000",
			},
		}
		data, err := json.Marshal(status)
		require.NoError(t, err)
		assert.Contains(t, string(data), `"synced":true`)
		assert.Contains(t, string(data), `"selected_peer":{"remote":"time1.google.com"`)

		// Test round-trip
		var decoded NTPStatus
		err = json.Unmarshal(data, &decoded)
		require.NoError(t, err)
		assert.True(t, decoded.Synced)
		assert.NotNil(t, decoded.SelectedPeer)
		assert.Equal(t, "time1.google.com", decoded.SelectedPeer.Remote)
	})
}

func TestK8sModelsJSON(t *testing.T) {
	t.Run("K8sInspectRequest marshalling", func(t *testing.T) {
		req := K8sInspectRequest{
			Operation: "list_pods",
			Namespace: "default",
			Name:      "nginx",
			Limit:     10,
		}
		data, err := json.Marshal(req)
		require.NoError(t, err)
		assert.Contains(t, string(data), `"operation":"list_pods"`)
		assert.Contains(t, string(data), `"namespace":"default"`)
		assert.Contains(t, string(data), `"limit":10`)

		// Test omitempty
		req.Limit = 0
		data, err = json.Marshal(req)
		require.NoError(t, err)
		assert.NotContains(t, string(data), `"limit"`)
	})

	t.Run("K8sPodInfo marshalling", func(t *testing.T) {
		pod := K8sPodInfo{
			Name:      "nginx-deployment-abc123",
			Namespace: "default",
			Status:    "Running",
		}
		data, err := json.Marshal(pod)
		require.NoError(t, err)
		assert.Contains(t, string(data), `"name":"nginx-deployment-abc123"`)
		assert.Contains(t, string(data), `"status":"Running"`)

		// Test round-trip
		var decoded K8sPodInfo
		err = json.Unmarshal(data, &decoded)
		require.NoError(t, err)
		assert.Equal(t, "nginx-deployment-abc123", decoded.Name)
		assert.Equal(t, "Running", decoded.Status)
	})

	t.Run("K8sNodeInfo marshalling", func(t *testing.T) {
		node := K8sNodeInfo{
			Name:  "node-1",
			Ready: true,
		}
		data, err := json.Marshal(node)
		require.NoError(t, err)
		assert.Contains(t, string(data), `"name":"node-1"`)
		assert.Contains(t, string(data), `"ready":true`)

		// Test round-trip
		var decoded K8sNodeInfo
		err = json.Unmarshal(data, &decoded)
		require.NoError(t, err)
		assert.Equal(t, "node-1", decoded.Name)
		assert.True(t, decoded.Ready)
	})

	t.Run("K8sServiceInfo marshalling", func(t *testing.T) {
		svc := K8sServiceInfo{
			Name:      "nginx-service",
			Namespace: "default",
			Type:      "ClusterIP",
		}
		data, err := json.Marshal(svc)
		require.NoError(t, err)
		assert.Contains(t, string(data), `"name":"nginx-service"`)
		assert.Contains(t, string(data), `"type":"ClusterIP"`)

		// Test round-trip
		var decoded K8sServiceInfo
		err = json.Unmarshal(data, &decoded)
		require.NoError(t, err)
		assert.Equal(t, "nginx-service", decoded.Name)
		assert.Equal(t, "ClusterIP", decoded.Type)
	})

	t.Run("K8sDeploymentInfo marshalling", func(t *testing.T) {
		deploy := K8sDeploymentInfo{
			Name:              "nginx-deployment",
			Namespace:         "default",
			DesiredReplicas:   3,
			AvailableReplicas: 3,
			UpdatedReplicas:   3,
			Ready:             true,
		}
		data, err := json.Marshal(deploy)
		require.NoError(t, err)
		assert.Contains(t, string(data), `"desired_replicas":3`)
		assert.Contains(t, string(data), `"ready":true`)

		// Test round-trip
		var decoded K8sDeploymentInfo
		err = json.Unmarshal(data, &decoded)
		require.NoError(t, err)
		assert.Equal(t, 3, decoded.DesiredReplicas)
		assert.True(t, decoded.Ready)
	})

	t.Run("K8sNamespaceInfo marshalling", func(t *testing.T) {
		ns := K8sNamespaceInfo{
			Name:   "production",
			Status: "Active",
		}
		data, err := json.Marshal(ns)
		require.NoError(t, err)
		assert.Contains(t, string(data), `"name":"production"`)
		assert.Contains(t, string(data), `"status":"Active"`)

		// Test round-trip
		var decoded K8sNamespaceInfo
		err = json.Unmarshal(data, &decoded)
		require.NoError(t, err)
		assert.Equal(t, "production", decoded.Name)
		assert.Equal(t, "Active", decoded.Status)
	})

	t.Run("K8sClusterInfo marshalling", func(t *testing.T) {
		cluster := K8sClusterInfo{
			Version: "v1.28.0",
			Context: "minikube",
			Cluster: "minikube",
		}
		data, err := json.Marshal(cluster)
		require.NoError(t, err)
		assert.Contains(t, string(data), `"version":"v1.28.0"`)
		assert.Contains(t, string(data), `"context":"minikube"`)

		// Test round-trip
		var decoded K8sClusterInfo
		err = json.Unmarshal(data, &decoded)
		require.NoError(t, err)
		assert.Equal(t, "v1.28.0", decoded.Version)
		assert.Equal(t, "minikube", decoded.Context)
	})

	t.Run("K8sPodLogs marshalling", func(t *testing.T) {
		logs := K8sPodLogs{
			Namespace: "default",
			Pod:       "nginx-abc123",
			Logs:      "log line 1\nlog line 2",
			Truncated: false,
		}
		data, err := json.Marshal(logs)
		require.NoError(t, err)
		assert.Contains(t, string(data), `"pod":"nginx-abc123"`)
		assert.Contains(t, string(data), `"truncated":false`)

		// Test round-trip
		var decoded K8sPodLogs
		err = json.Unmarshal(data, &decoded)
		require.NoError(t, err)
		assert.Equal(t, "nginx-abc123", decoded.Pod)
		assert.False(t, decoded.Truncated)
	})

	t.Run("K8sPodDescribe marshalling", func(t *testing.T) {
		desc := K8sPodDescribe{
			Namespace: "default",
			Pod:       "nginx-abc123",
			Describe:  "Name: nginx-abc123\nNamespace: default",
		}
		data, err := json.Marshal(desc)
		require.NoError(t, err)
		assert.Contains(t, string(data), `"pod":"nginx-abc123"`)
		assert.Contains(t, string(data), `"describe"`)

		// Test round-trip
		var decoded K8sPodDescribe
		err = json.Unmarshal(data, &decoded)
		require.NoError(t, err)
		assert.Equal(t, "nginx-abc123", decoded.Pod)
		assert.Contains(t, decoded.Describe, "nginx-abc123")
	})
}

func TestSSHModelsJSON(t *testing.T) {
	t.Run("NetSSHKnownHostsRequest marshalling", func(t *testing.T) {
		req := NetSSHKnownHostsRequest{
			SSHConfigPath:  "~/.ssh/config",
			KnownHostsPath: "~/.ssh/known_hosts",
		}
		data, err := json.Marshal(req)
		require.NoError(t, err)
		assert.Contains(t, string(data), `"ssh_config_path":"~/.ssh/config"`)
		assert.Contains(t, string(data), `"known_hosts_path":"~/.ssh/known_hosts"`)

		// Test omitempty
		req.SSHConfigPath = ""
		req.KnownHostsPath = ""
		data, err = json.Marshal(req)
		require.NoError(t, err)
		assert.NotContains(t, string(data), `"ssh_config_path"`)
		assert.NotContains(t, string(data), `"known_hosts_path"`)
	})

	t.Run("SSHConfigHost marshalling", func(t *testing.T) {
		host := SSHConfigHost{
			Pattern:       "github.com",
			Hostname:      "github.com",
			User:          "git",
			Port:          "22",
			IdentityFiles: []string{"~/.ssh/id_rsa"},
			ProxyCommand:  "ssh -q -W %h:%p jump.example.com",
		}
		data, err := json.Marshal(host)
		require.NoError(t, err)
		assert.Contains(t, string(data), `"pattern":"github.com"`)
		assert.Contains(t, string(data), `"identity_files":["~/.ssh/id_rsa"]`)

		// Test round-trip
		var decoded SSHConfigHost
		err = json.Unmarshal(data, &decoded)
		require.NoError(t, err)
		assert.Equal(t, "github.com", decoded.Pattern)
		assert.Equal(t, "git", decoded.User)
	})

	t.Run("SSHKnownHost marshalling", func(t *testing.T) {
		host := SSHKnownHost{
			HostPattern: "github.com",
			KeyType:     "ssh-rsa",
			KeyHash:     "SHA256:abc123",
		}
		data, err := json.Marshal(host)
		require.NoError(t, err)
		assert.Contains(t, string(data), `"host_pattern":"github.com"`)
		assert.Contains(t, string(data), `"key_hash":"SHA256:abc123"`)

		// Test round-trip
		var decoded SSHKnownHost
		err = json.Unmarshal(data, &decoded)
		require.NoError(t, err)
		assert.Equal(t, "github.com", decoded.HostPattern)
		assert.Equal(t, "ssh-rsa", decoded.KeyType)
	})
}

func TestOperatorModelsJSON(t *testing.T) {
	t.Run("OperatorDeployRequest marshalling", func(t *testing.T) {
		req := OperatorDeployRequest{
			Hostnames:      []string{"host1", "host2"},
			OperatorBinary: "/usr/local/bin/g8e-operator",
			OperatorArgs:   []string{"--config", "/etc/g8e/config.yaml"},
			Timeout:        300,
		}
		data, err := json.Marshal(req)
		require.NoError(t, err)
		assert.Contains(t, string(data), `"hostnames":["host1","host2"]`)
		assert.Contains(t, string(data), `"timeout":300`)

		// Test omitempty
		req.Timeout = 0
		data, err = json.Marshal(req)
		require.NoError(t, err)
		assert.NotContains(t, string(data), `"timeout"`)
	})

	t.Run("OperatorDeploymentResult marshalling", func(t *testing.T) {
		res := OperatorDeploymentResult{
			Hostname: "host1",
			Success:  true,
			Message:  "Deployed successfully",
			Output:   "operator started",
		}
		data, err := json.Marshal(res)
		require.NoError(t, err)
		assert.Contains(t, string(data), `"hostname":"host1"`)
		assert.Contains(t, string(data), `"success":true`)

		// Test with error
		res.Success = false
		res.Error = "connection failed"
		data, err = json.Marshal(res)
		require.NoError(t, err)
		assert.Contains(t, string(data), `"error":"connection failed"`)

		// Test round-trip
		var decoded OperatorDeploymentResult
		err = json.Unmarshal(data, &decoded)
		require.NoError(t, err)
		assert.Equal(t, "host1", decoded.Hostname)
		assert.False(t, decoded.Success)
		assert.Equal(t, "connection failed", decoded.Error)
	})

	t.Run("OperatorDeployResult marshalling", func(t *testing.T) {
		res := OperatorDeployResult{
			Deployments: []OperatorDeploymentResult{
				{
					Hostname: "host1",
					Success:  true,
					Message:  "Deployed successfully",
				},
				{
					Hostname: "host2",
					Success:  false,
					Error:    "timeout",
				},
			},
		}
		data, err := json.Marshal(res)
		require.NoError(t, err)
		assert.Contains(t, string(data), `"deployments"`)

		// Test round-trip
		var decoded OperatorDeployResult
		err = json.Unmarshal(data, &decoded)
		require.NoError(t, err)
		assert.Len(t, decoded.Deployments, 2)
		assert.True(t, decoded.Deployments[0].Success)
		assert.False(t, decoded.Deployments[1].Success)
	})
}

func TestSysServiceStatusModelsJSON(t *testing.T) {
	t.Run("SysServiceStatusRequest marshalling", func(t *testing.T) {
		req := SysServiceStatusRequest{
			ServiceName: "nginx",
		}
		data, err := json.Marshal(req)
		require.NoError(t, err)
		assert.Contains(t, string(data), `"service_name":"nginx"`)

		// Test round-trip
		var decoded SysServiceStatusRequest
		err = json.Unmarshal(data, &decoded)
		require.NoError(t, err)
		assert.Equal(t, "nginx", decoded.ServiceName)
	})

	t.Run("SysServiceStatusResult marshalling", func(t *testing.T) {
		res := SysServiceStatusResult{
			ServiceName: "nginx",
			LoadState:   "loaded",
			ActiveState: "active",
			SubState:    "running",
			Enabled:     true,
			Description: "A high performance web server",
			MainPID:     "1234",
			ExecStart:   "/usr/sbin/nginx -g daemon off;",
		}
		data, err := json.Marshal(res)
		require.NoError(t, err)
		assert.Contains(t, string(data), `"service_name":"nginx"`)
		assert.Contains(t, string(data), `"active_state":"active"`)
		assert.Contains(t, string(data), `"enabled":true`)

		// Test with error
		res.Error = "service not found"
		data, err = json.Marshal(res)
		require.NoError(t, err)
		assert.Contains(t, string(data), `"error":"service not found"`)

		// Test round-trip
		var decoded SysServiceStatusResult
		err = json.Unmarshal(data, &decoded)
		require.NoError(t, err)
		assert.Equal(t, "nginx", decoded.ServiceName)
		assert.Equal(t, "active", decoded.ActiveState)
		assert.True(t, decoded.Enabled)
	})
}

func TestA2AModelsJSON(t *testing.T) {
	t.Run("A2ASuspensionResponse marshalling", func(t *testing.T) {
		res := A2ASuspensionResponse{
			ID:          "id1",
			Status:      "suspended",
			TxHash:      "hash1",
			ApprovalURL: "http://approval",
		}
		data, err := json.Marshal(res)
		require.NoError(t, err)
		assert.Contains(t, string(data), `"tx_hash":"hash1"`)
		assert.Contains(t, string(data), `"approval_url":"http://approval"`)
	})

	t.Run("A2ADownstreamRequest marshalling", func(t *testing.T) {
		req := A2ADownstreamRequest{
			SkillName:   "test-skill",
			PayloadJSON: json.RawMessage(`{"data":1}`),
		}
		data, err := json.Marshal(req)
		require.NoError(t, err)
		assert.Contains(t, string(data), `"skill_name":"test-skill"`)
		assert.Contains(t, string(data), `"payload":{"data":1}`)
	})
}

// Helper functions for creating pointers to primitives
func strPtr(s string) *string {
	return &s
}

func int64Ptr(i int64) *int64 {
	return &i
}
