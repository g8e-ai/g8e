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
			Metadata:    map[string]interface{}{"key": "value"},
		}
		data, err := json.Marshal(res)
		require.NoError(t, err)
		assert.Contains(t, string(data), `"uri":"file:///tmp/test.txt"`)
		assert.Contains(t, string(data), `"name":"test-resource"`)
		assert.Contains(t, string(data), `"description":"a test resource"`)
		assert.Contains(t, string(data), `"mimeType":"text/plain"`)
		assert.Contains(t, string(data), `"metadata":{"key":"value"}`)
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
