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
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestNetHTTPProbeTool_Metadata(t *testing.T) {
	tool := &NetHTTPProbeTool{}
	require.Equal(t, "net_http_probe", tool.Name())
	require.NotEmpty(t, tool.Description())
	require.Contains(t, tool.Description(), "HTTP")
}

func TestNetHTTPProbeTool_InputSchema(t *testing.T) {
	tool := &NetHTTPProbeTool{}
	schema := tool.InputSchema()

	require.Equal(t, "object", schema.Type)
	require.NotNil(t, schema.Properties)

	urlProp, ok := schema.Properties["url"]
	require.True(t, ok)
	require.Equal(t, "string", urlProp.Type)

	methodProp, ok := schema.Properties["method"]
	require.True(t, ok)
	require.Equal(t, "string", methodProp.Type)

	require.Contains(t, schema.Required, "url")
}

func TestNetHTTPProbeTool_Execute_InvalidArgs(t *testing.T) {
	tool := &NetHTTPProbeTool{}
	ctx := context.Background()

	// Invalid JSON
	_, err := tool.Execute(ctx, json.RawMessage(`{invalid`))
	require.Error(t, err)
	require.Contains(t, err.Error(), "invalid arguments")

	// Missing URL
	_, err = tool.Execute(ctx, json.RawMessage(`{}`))
	require.Error(t, err)
	require.Contains(t, err.Error(), "url required")
}

func TestNetHTTPProbeTool_Execute_BlockedURLs(t *testing.T) {
	tool := &NetHTTPProbeTool{}
	ctx := context.Background()

	tests := []struct {
		name string
		url  string
	}{
		{"localhost", "http://localhost"},
		{"loopback ip", "http://127.0.0.1"},
		{"private ip 10.x", "http://10.0.0.1"},
		{"private ip 192.168.x", "http://192.168.1.1"},
		{"link-local", "http://169.254.1.1"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			args := json.RawMessage(`{"url": "` + tt.url + `"}`)
			result, err := tool.Execute(ctx, args)
			require.NoError(t, err)
			require.Len(t, result.Content, 1)
			require.Equal(t, "text", result.Content[0].Type)

			var probeResult NetHTTPProbeResult
			err = json.Unmarshal([]byte(result.Content[0].Text), &probeResult)
			require.NoError(t, err)
			require.Contains(t, probeResult.Error, "URL validation failed")
		})
	}
}

func TestNetHTTPProbeTool_Execute_NetworkError(t *testing.T) {
	tool := &NetHTTPProbeTool{}
	ctx := context.Background()

	// Use a non-existent domain that won't resolve but passes validation
	args := json.RawMessage(`{"url": "http://non-existent-domain-12345.example.com"}`)
	result, err := tool.Execute(ctx, args)
	require.NoError(t, err)
	require.Len(t, result.Content, 1)

	var probeResult NetHTTPProbeResult
	err = json.Unmarshal([]byte(result.Content[0].Text), &probeResult)
	require.NoError(t, err)
	require.NotEmpty(t, probeResult.Error)
	require.NotContains(t, probeResult.Error, "URL validation failed")
	require.GreaterOrEqual(t, probeResult.LatencyMs, 0.0)
}

func TestNetHTTPProbeTool_Execute_Timeout(t *testing.T) {
	tool := &NetHTTPProbeTool{}
	
	// Create a context that is already canceled or has a very short timeout
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Microsecond)
	defer cancel()
	
	// Sleep a bit to ensure timeout
	time.Sleep(10 * time.Millisecond)

	args := json.RawMessage(`{"url": "http://example.com"}`)
	result, err := tool.Execute(ctx, args)
	require.NoError(t, err)
	require.Len(t, result.Content, 1)

	var probeResult NetHTTPProbeResult
	err = json.Unmarshal([]byte(result.Content[0].Text), &probeResult)
	require.NoError(t, err)
	require.NotEmpty(t, probeResult.Error)
	// It should be a context deadline exceeded error or similar
	require.Contains(t, strings.ToLower(probeResult.Error), "context")
}

func TestNetHTTPProbeTool_Execute_MethodHandling(t *testing.T) {
	// Since we can't easily hit a real server, we'll just test that different methods 
	// don't crash and pass through the validation.
	tool := &NetHTTPProbeTool{}
	ctx := context.Background()

	args := json.RawMessage(`{"url": "http://example.com", "method": "GET"}`)
	result, err := tool.Execute(ctx, args)
	require.NoError(t, err)
	require.Len(t, result.Content, 1)
	
	var probeResult NetHTTPProbeResult
	err = json.Unmarshal([]byte(result.Content[0].Text), &probeResult)
	require.NoError(t, err)
	// Error should be a network error, not a validation error
	require.NotContains(t, probeResult.Error, "URL validation failed")
}
