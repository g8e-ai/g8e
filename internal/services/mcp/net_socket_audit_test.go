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
	"encoding/json"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockFileOpener is a mock implementation of fileOpener for testing.
type mockFileOpener struct {
	openFunc func(name string) (*os.File, error)
}

func (m *mockFileOpener) Open(name string) (*os.File, error) {
	if m.openFunc != nil {
		return m.openFunc(name)
	}
	return nil, errors.New("file not found")
}

func TestNetSocketAuditTool_Name(t *testing.T) {
	t.Parallel()

	tool := &NetSocketAuditTool{}
	assert.Equal(t, "net_socket_audit", tool.Name())
}

func TestNetSocketAuditTool_Description(t *testing.T) {
	t.Parallel()

	tool := &NetSocketAuditTool{}
	assert.Equal(t, "Inspects active network sockets (TCP/UDP) from /proc/net.", tool.Description())
}

func TestNetSocketAuditTool_InputSchema(t *testing.T) {
	t.Parallel()

	tool := &NetSocketAuditTool{}
	schema := tool.InputSchema()

	assert.NotNil(t, schema)
	assert.Equal(t, "object", schema.Type)
	assert.Contains(t, schema.Properties, "protocol")
	assert.Equal(t, "string", schema.Properties["protocol"].Type)
	assert.Equal(t, "Protocol filter (tcp, udp, or empty for both)", schema.Properties["protocol"].Description)
}

func TestNetSocketAuditTool_Execute_InvalidJSON(t *testing.T) {
	t.Parallel()

	tool := &NetSocketAuditTool{}
	ctx := context.Background()

	_, err := tool.Execute(ctx, json.RawMessage("{invalid json"))
	assert.Error(t, err)
	assert.True(t, errors.Is(err, constants.ErrMCPUnmarshalArguments))
}

func TestNetSocketAuditTool_Execute_InvalidProtocol(t *testing.T) {
	t.Parallel()

	tool := &NetSocketAuditTool{}
	ctx := context.Background()

	req := NetSocketAuditRequest{Protocol: "invalid"}
	args, err := json.Marshal(req)
	require.NoError(t, err)

	_, err = tool.Execute(ctx, args)
	assert.Error(t, err)
	assert.True(t, errors.Is(err, constants.ErrMCPValidateProcNetInvalidProtocol))
}

func TestNetSocketAuditTool_Execute_ProtocolCaseInsensitivity(t *testing.T) {
	t.Parallel()

	tool := &NetSocketAuditTool{}
	ctx := context.Background()

	// Uppercase protocol should be converted to lowercase and validated
	req := NetSocketAuditRequest{Protocol: "TCP"}
	args, err := json.Marshal(req)
	require.NoError(t, err)

	// Should succeed because the code converts to lowercase before validation
	result, err := tool.Execute(ctx, args)
	assert.NoError(t, err)
	assert.NotNil(t, result)

	// Since /proc/net/tcp doesn't exist on Windows, expect empty result
	var resultData NetSocketAuditResult
	err = json.Unmarshal([]byte(result.Content[0].Text), &resultData)
	assert.NoError(t, err)
	assert.Empty(t, resultData.Sockets)
}

func TestNetSocketAuditTool_Execute_FileNotFound(t *testing.T) {
	t.Parallel()

	tool := &NetSocketAuditTool{
		fileOpener: &mockFileOpener{
			openFunc: func(name string) (*os.File, error) {
				return nil, os.ErrNotExist
			},
		},
	}
	ctx := context.Background()

	req := NetSocketAuditRequest{Protocol: "tcp"}
	args, err := json.Marshal(req)
	require.NoError(t, err)

	result, err := tool.Execute(ctx, args)
	assert.NoError(t, err)
	assert.NotNil(t, result)

	var resultData NetSocketAuditResult
	err = json.Unmarshal([]byte(result.Content[0].Text), &resultData)
	assert.NoError(t, err)
	assert.Empty(t, resultData.Sockets)
}

func TestNetSocketAuditTool_Execute_ContextCancellation(t *testing.T) {
	t.Parallel()

	// Create a temporary file with valid content
	tmpFile, err := os.CreateTemp("", "proc-net-tcp")
	require.NoError(t, err)
	defer os.Remove(tmpFile.Name())

	// Write valid TCP data
	content := `  sl  local_address rem_address   st tx_queue rx_queue tr tm->when retrnsmt   uid  timeout inode
   0: 0100007F:1F90 00000000:0000 0A 00000000:00000000 00:00000000 00000000     0        0 12345 1 0000000000000000 100 0 0 10 0`
	_, err = tmpFile.WriteString(content)
	require.NoError(t, err)
	tmpFile.Close()

	tool := &NetSocketAuditTool{
		fileOpener: &mockFileOpener{
			openFunc: func(name string) (*os.File, error) {
				return os.Open(tmpFile.Name())
			},
		},
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	req := NetSocketAuditRequest{Protocol: "tcp"}
	args, err := json.Marshal(req)
	require.NoError(t, err)

	_, err = tool.Execute(ctx, args)
	assert.NoError(t, err) // Should return gracefully even with cancelled context
}

func TestNetSocketAuditTool_Execute_ValidTCPData(t *testing.T) {
	t.Parallel()

	// Create a temporary file with valid TCP data
	tmpFile, err := os.CreateTemp("", "proc-net-tcp")
	require.NoError(t, err)
	defer os.Remove(tmpFile.Name())

	// Write valid TCP data with multiple entries
	// 127.0.0.1:8080 (0100007F:1F90) listening (0A)
	// 0.0.0.0:443 (00000000:01BB) listening (0A)
	content := `  sl  local_address rem_address   st tx_queue rx_queue tr tm->when retrnsmt   uid  timeout inode
   0: 0100007F:1F90 00000000:0000 0A 00000000:00000000 00:00000000 00000000     0        0 12345 1 0000000000000000 100 0 0 10 0
   1: 00000000:01BB 00000000:0000 0A 00000000:00000000 00:00000000 00000000     0        0 12346 1 0000000000000000 100 0 0 10 0`
	_, err = tmpFile.WriteString(content)
	require.NoError(t, err)
	tmpFile.Close()

	tool := &NetSocketAuditTool{
		fileOpener: &mockFileOpener{
			openFunc: func(name string) (*os.File, error) {
				return os.Open(tmpFile.Name())
			},
		},
	}
	ctx := context.Background()

	req := NetSocketAuditRequest{Protocol: "tcp"}
	args, err := json.Marshal(req)
	require.NoError(t, err)

	result, err := tool.Execute(ctx, args)
	assert.NoError(t, err)
	assert.NotNil(t, result)

	var resultData NetSocketAuditResult
	err = json.Unmarshal([]byte(result.Content[0].Text), &resultData)
	assert.NoError(t, err)
	assert.Len(t, resultData.Sockets, 2)

	// Verify first socket (127.0.0.1:8080 in little-endian: 0100007F:1F90)
	assert.Equal(t, "tcp", resultData.Sockets[0].Protocol)
	assert.Equal(t, "127.0.0.1", resultData.Sockets[0].LocalAddr)
	assert.Equal(t, 8080, resultData.Sockets[0].LocalPort)
	assert.Equal(t, "0.0.0.0", resultData.Sockets[0].RemoteAddr)
	assert.Equal(t, 0, resultData.Sockets[0].RemotePort)
	assert.Equal(t, "0A", resultData.Sockets[0].State)

	// Verify second socket (0.0.0.0:443 in little-endian: 00000000:01BB)
	assert.Equal(t, "tcp", resultData.Sockets[1].Protocol)
	assert.Equal(t, "0.0.0.0", resultData.Sockets[1].LocalAddr)
	assert.Equal(t, 443, resultData.Sockets[1].LocalPort)
}

func TestNetSocketAuditTool_Execute_ValidUDPData(t *testing.T) {
	t.Parallel()

	// Create a temporary file with valid UDP data
	tmpFile, err := os.CreateTemp("", "proc-net-udp")
	require.NoError(t, err)
	defer os.Remove(tmpFile.Name())

	// Write valid UDP data
	// 127.0.0.1:53 (0100007F:0035)
	content := `  sl  local_address rem_address   st tx_queue rx_queue tr tm->when retrnsmt   uid  timeout inode
   0: 0100007F:0035 00000000:0000 07 00000000:00000000 00:00000000 00000000     0        0 12345 1 0000000000000000 100 0 0 10 0`
	_, err = tmpFile.WriteString(content)
	require.NoError(t, err)
	tmpFile.Close()

	tool := &NetSocketAuditTool{
		fileOpener: &mockFileOpener{
			openFunc: func(name string) (*os.File, error) {
				return os.Open(tmpFile.Name())
			},
		},
	}
	ctx := context.Background()

	req := NetSocketAuditRequest{Protocol: "udp"}
	args, err := json.Marshal(req)
	require.NoError(t, err)

	result, err := tool.Execute(ctx, args)
	assert.NoError(t, err)
	assert.NotNil(t, result)

	var resultData NetSocketAuditResult
	err = json.Unmarshal([]byte(result.Content[0].Text), &resultData)
	assert.NoError(t, err)
	assert.Len(t, resultData.Sockets, 1)

	// Verify UDP socket (127.0.0.1:53 in little-endian: 0100007F:0035)
	assert.Equal(t, "udp", resultData.Sockets[0].Protocol)
	assert.Equal(t, "127.0.0.1", resultData.Sockets[0].LocalAddr)
	assert.Equal(t, 53, resultData.Sockets[0].LocalPort)
}

func TestNetSocketAuditTool_Execute_EmptyProtocol(t *testing.T) {
	t.Parallel()

	// Create temporary files for both TCP and UDP
	tcpFile, err := os.CreateTemp("", "proc-net-tcp")
	require.NoError(t, err)
	defer os.Remove(tcpFile.Name())

	udpFile, err := os.CreateTemp("", "proc-net-udp")
	require.NoError(t, err)
	defer os.Remove(udpFile.Name())

	// Write TCP data
	tcpContent := `  sl  local_address rem_address   st tx_queue rx_queue tr tm->when retrnsmt   uid  timeout inode
   0: 0100007F:1F90 00000000:0000 0A 00000000:00000000 00:00000000 00000000     0        0 12345 1 0000000000000000 100 0 0 10 0`
	_, err = tcpFile.WriteString(tcpContent)
	require.NoError(t, err)
	tcpFile.Close()

	// Write UDP data
	udpContent := `  sl  local_address rem_address   st tx_queue rx_queue tr tm->when retrnsmt   uid  timeout inode
   0: 0100007F:0035 00000000:0000 07 00000000:00000000 00:00000000 00000000     0        0 12346 1 0000000000000000 100 0 0 10 0`
	_, err = udpFile.WriteString(udpContent)
	require.NoError(t, err)
	udpFile.Close()

	openCount := 0
	tool := &NetSocketAuditTool{
		fileOpener: &mockFileOpener{
			openFunc: func(name string) (*os.File, error) {
				openCount++
				if strings.Contains(name, "tcp") {
					return os.Open(tcpFile.Name())
				}
				if strings.Contains(name, "udp") {
					return os.Open(udpFile.Name())
				}
				return nil, os.ErrNotExist
			},
		},
	}
	ctx := context.Background()

	req := NetSocketAuditRequest{Protocol: ""}
	args, err := json.Marshal(req)
	require.NoError(t, err)

	result, err := tool.Execute(ctx, args)
	assert.NoError(t, err)
	assert.NotNil(t, result)

	var resultData NetSocketAuditResult
	err = json.Unmarshal([]byte(result.Content[0].Text), &resultData)
	assert.NoError(t, err)
	assert.Len(t, resultData.Sockets, 2) // One TCP, one UDP

	// Should have opened both TCP and UDP
	assert.Equal(t, 2, openCount)
}

func TestNetSocketAuditTool_Execute_MalformedFile(t *testing.T) {
	t.Parallel()

	// Create a temporary file with malformed data
	tmpFile, err := os.CreateTemp("", "proc-net-tcp")
	require.NoError(t, err)
	defer os.Remove(tmpFile.Name())

	// Write malformed data (insufficient fields)
	content := `  sl  local_address rem_address
   0: 0100007F`
	_, err = tmpFile.WriteString(content)
	require.NoError(t, err)
	tmpFile.Close()

	tool := &NetSocketAuditTool{
		fileOpener: &mockFileOpener{
			openFunc: func(name string) (*os.File, error) {
				return os.Open(tmpFile.Name())
			},
		},
	}
	ctx := context.Background()

	req := NetSocketAuditRequest{Protocol: "tcp"}
	args, err := json.Marshal(req)
	require.NoError(t, err)

	result, err := tool.Execute(ctx, args)
	assert.NoError(t, err) // Should handle malformed data gracefully
	assert.NotNil(t, result)

	var resultData NetSocketAuditResult
	err = json.Unmarshal([]byte(result.Content[0].Text), &resultData)
	assert.NoError(t, err)
	assert.Empty(t, resultData.Sockets) // No valid sockets parsed
}

func TestNetSocketAuditTool_Execute_EmptyFile(t *testing.T) {
	t.Parallel()

	// Create a temporary empty file
	tmpFile, err := os.CreateTemp("", "proc-net-tcp")
	require.NoError(t, err)
	defer os.Remove(tmpFile.Name())
	tmpFile.Close()

	tool := &NetSocketAuditTool{
		fileOpener: &mockFileOpener{
			openFunc: func(name string) (*os.File, error) {
				return os.Open(tmpFile.Name())
			},
		},
	}
	ctx := context.Background()

	req := NetSocketAuditRequest{Protocol: "tcp"}
	args, err := json.Marshal(req)
	require.NoError(t, err)

	result, err := tool.Execute(ctx, args)
	assert.NoError(t, err)
	assert.NotNil(t, result)

	var resultData NetSocketAuditResult
	err = json.Unmarshal([]byte(result.Content[0].Text), &resultData)
	assert.NoError(t, err)
	assert.Empty(t, resultData.Sockets)
}

func TestNetSocketAuditTool_Execute_OnlyHeader(t *testing.T) {
	t.Parallel()

	// Create a temporary file with only header
	tmpFile, err := os.CreateTemp("", "proc-net-tcp")
	require.NoError(t, err)
	defer os.Remove(tmpFile.Name())

	content := `  sl  local_address rem_address   st tx_queue rx_queue tr tm->when retrnsmt   uid  timeout inode`
	_, err = tmpFile.WriteString(content)
	require.NoError(t, err)
	tmpFile.Close()

	tool := &NetSocketAuditTool{
		fileOpener: &mockFileOpener{
			openFunc: func(name string) (*os.File, error) {
				return os.Open(tmpFile.Name())
			},
		},
	}
	ctx := context.Background()

	req := NetSocketAuditRequest{Protocol: "tcp"}
	args, err := json.Marshal(req)
	require.NoError(t, err)

	result, err := tool.Execute(ctx, args)
	assert.NoError(t, err)
	assert.NotNil(t, result)

	var resultData NetSocketAuditResult
	err = json.Unmarshal([]byte(result.Content[0].Text), &resultData)
	assert.NoError(t, err)
	assert.Empty(t, resultData.Sockets)
}

func TestNetSocketAuditTool_Execute_InvalidAddress(t *testing.T) {
	t.Parallel()

	// Create a temporary file with invalid address format
	tmpFile, err := os.CreateTemp("", "proc-net-tcp")
	require.NoError(t, err)
	defer os.Remove(tmpFile.Name())

	// Write data with invalid hex address (8 chars but invalid hex)
	// "ZZZZZZZZ" will fail to parse and the entry will be skipped
	content := `  sl  local_address rem_address   st tx_queue rx_queue tr tm->when retrnsmt   uid  timeout inode
   0: ZZZZZZZZ:0000 00000000:0000 0A 00000000:00000000 00:00000000 00000000     0        0 12345 1 0000000000000000 100 0 0 10 0`
	_, err = tmpFile.WriteString(content)
	require.NoError(t, err)
	tmpFile.Close()

	tool := &NetSocketAuditTool{
		fileOpener: &mockFileOpener{
			openFunc: func(name string) (*os.File, error) {
				return os.Open(tmpFile.Name())
			},
		},
	}
	ctx := context.Background()

	req := NetSocketAuditRequest{Protocol: "tcp"}
	args, err := json.Marshal(req)
	require.NoError(t, err)

	result, err := tool.Execute(ctx, args)
	assert.NoError(t, err) // Should handle invalid addresses gracefully
	assert.NotNil(t, result)

	var resultData NetSocketAuditResult
	err = json.Unmarshal([]byte(result.Content[0].Text), &resultData)
	assert.NoError(t, err)
	// Invalid hex addresses are skipped entirely
	assert.Empty(t, resultData.Sockets)
}

func TestGetProcNetPath(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		protocol string
		expected string
	}{
		{
			name:     "tcp",
			protocol: "tcp",
			expected: "/proc/net/tcp",
		},
		{
			name:     "udp",
			protocol: "udp",
			expected: "/proc/net/udp",
		},
		{
			name:     "tcp6",
			protocol: "tcp6",
			expected: "/proc/net/tcp6",
		},
		{
			name:     "udp6",
			protocol: "udp6",
			expected: "/proc/net/udp6",
		},
		{
			name:     "raw",
			protocol: "raw",
			expected: "/proc/net/raw",
		},
		{
			name:     "unknown protocol",
			protocol: "unknown",
			expected: "/proc/net/unknown",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			result := getProcNetPath(tt.protocol)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestParseProcNetFile_ContextCancellation(t *testing.T) {
	t.Parallel()

	// Create a temporary file
	tmpFile, err := os.CreateTemp("", "proc-net-tcp")
	require.NoError(t, err)
	defer os.Remove(tmpFile.Name())

	// Write valid data
	content := `  sl  local_address rem_address   st tx_queue rx_queue tr tm->when retrnsmt   uid  timeout inode
   0: 0100007F:1F90 00000000:0000 0A 00000000:00000000 00:00000000 00000000     0        0 12345 1 0000000000000000 100 0 0 10 0`
	_, err = tmpFile.WriteString(content)
	require.NoError(t, err)
	tmpFile.Close()

	file, err := os.Open(tmpFile.Name())
	require.NoError(t, err)
	defer file.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	sockets, err := parseProcNetFile(ctx, file, "tcp")
	assert.Error(t, err)
	assert.Equal(t, context.Canceled, err)
	assert.Empty(t, sockets)
}

func TestParseProcNetFile_ValidData(t *testing.T) {
	t.Parallel()

	// Create a temporary file with valid data
	tmpFile, err := os.CreateTemp("", "proc-net-tcp")
	require.NoError(t, err)
	defer os.Remove(tmpFile.Name())

	// Write valid TCP data with established connection
	// 192.168.1.1:12345 -> 10.0.0.1:443, state 01 (ESTABLISHED)
	// Little-endian hex: 192.168.1.1=0101A8C0, 10.0.0.1=0100000A
	content := `  sl  local_address rem_address   st tx_queue rx_queue tr tm->when retrnsmt   uid  timeout inode
   0: 0101A8C0:3039 0100000A:01BB 01 00000000:00000000 00:00000000 00000000     0        0 12345 1 0000000000000000 100 0 0 10 0`
	_, err = tmpFile.WriteString(content)
	require.NoError(t, err)
	tmpFile.Close()

	file, err := os.Open(tmpFile.Name())
	require.NoError(t, err)
	defer file.Close()

	ctx := context.Background()
	sockets, err := parseProcNetFile(ctx, file, "tcp")
	assert.NoError(t, err)
	assert.Len(t, sockets, 1)

	assert.Equal(t, "tcp", sockets[0].Protocol)
	assert.Equal(t, "192.168.1.1", sockets[0].LocalAddr)
	assert.Equal(t, 12345, sockets[0].LocalPort)
	assert.Equal(t, "10.0.0.1", sockets[0].RemoteAddr)
	assert.Equal(t, 443, sockets[0].RemotePort)
	assert.Equal(t, "01", sockets[0].State)
}

func TestParseProcNetFile_MultipleEntries(t *testing.T) {
	t.Parallel()

	// Create a temporary file with multiple entries
	tmpFile, err := os.CreateTemp("", "proc-net-tcp")
	require.NoError(t, err)
	defer os.Remove(tmpFile.Name())

	content := `  sl  local_address rem_address   st tx_queue rx_queue tr tm->when retrnsmt   uid  timeout inode
   0: 0100007F:1F90 00000000:0000 0A 00000000:00000000 00:00000000 00000000     0        0 12345 1 0000000000000000 100 0 0 10 0
   1: 00000000:01BB 00000000:0000 0A 00000000:00000000 00:00000000 00000000     0        0 12346 1 0000000000000000 100 0 0 10 0
   2: 0100007F:0050 0100007F:3039 01 00000000:00000000 00:00000000 00000000     0        0 12347 1 0000000000000000 100 0 0 10 0`
	_, err = tmpFile.WriteString(content)
	require.NoError(t, err)
	tmpFile.Close()

	file, err := os.Open(tmpFile.Name())
	require.NoError(t, err)
	defer file.Close()

	ctx := context.Background()
	sockets, err := parseProcNetFile(ctx, file, "tcp")
	assert.NoError(t, err)
	assert.Len(t, sockets, 3)
}

func TestParseProcNetFile_SkipInvalidLines(t *testing.T) {
	t.Parallel()

	// Create a temporary file with mixed valid and invalid data
	tmpFile, err := os.CreateTemp("", "proc-net-tcp")
	require.NoError(t, err)
	defer os.Remove(tmpFile.Name())

	content := `  sl  local_address rem_address   st tx_queue rx_queue tr tm->when retrnsmt   uid  timeout inode
   0: 0100007F:1F90 00000000:0000 0A 00000000:00000000 00:00000000 00000000     0        0 12345 1 0000000000000000 100 0 0 10 0
   1: INVALID 00000000:0000 0A 00000000:00000000 00:00000000 00000000     0        0 12346 1 0000000000000000 100 0 0 10 0
   2: 00000000:01BB 00000000:0000 0A 00000000:00000000 00:00000000 00000000     0        0 12347 1 0000000000000000 100 0 0 10 0`
	_, err = tmpFile.WriteString(content)
	require.NoError(t, err)
	tmpFile.Close()

	file, err := os.Open(tmpFile.Name())
	require.NoError(t, err)
	defer file.Close()

	ctx := context.Background()
	sockets, err := parseProcNetFile(ctx, file, "tcp")
	assert.NoError(t, err)
	assert.Len(t, sockets, 2) // Only valid entries
}

func TestParseProcNetFile_InsufficientFields(t *testing.T) {
	t.Parallel()

	// Create a temporary file with insufficient fields
	tmpFile, err := os.CreateTemp("", "proc-net-tcp")
	require.NoError(t, err)
	defer os.Remove(tmpFile.Name())

	content := `  sl  local_address rem_address
   0: 0100007F:1F90 00000000:0000`
	_, err = tmpFile.WriteString(content)
	require.NoError(t, err)
	tmpFile.Close()

	file, err := os.Open(tmpFile.Name())
	require.NoError(t, err)
	defer file.Close()

	ctx := context.Background()
	sockets, err := parseProcNetFile(ctx, file, "tcp")
	assert.NoError(t, err)
	assert.Empty(t, sockets) // Lines with insufficient fields are skipped
}

func TestParseProcNetFile_EmptyFile(t *testing.T) {
	t.Parallel()

	// Create a temporary empty file
	tmpFile, err := os.CreateTemp("", "proc-net-tcp")
	require.NoError(t, err)
	defer os.Remove(tmpFile.Name())
	tmpFile.Close()

	file, err := os.Open(tmpFile.Name())
	require.NoError(t, err)
	defer file.Close()

	ctx := context.Background()
	sockets, err := parseProcNetFile(ctx, file, "tcp")
	assert.NoError(t, err)
	assert.Empty(t, sockets)
}

func TestNetSocketAuditTool_Execute_NilFileOpener(t *testing.T) {
	t.Parallel()

	// Test that nil fileOpener uses default osFileOpener
	tool := &NetSocketAuditTool{fileOpener: nil}
	ctx := context.Background()

	req := NetSocketAuditRequest{Protocol: "tcp"}
	args, err := json.Marshal(req)
	require.NoError(t, err)

	// This will fail to open /proc/net/tcp on non-Linux systems, but should not panic
	result, err := tool.Execute(ctx, args)
	assert.NoError(t, err) // Should handle missing file gracefully
	assert.NotNil(t, result)
}
