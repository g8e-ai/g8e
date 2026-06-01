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
	"fmt"
	"net/http"
	"time"
)

// NetHTTPProbeTool performs lightweight HTTP requests.
type NetHTTPProbeTool struct{}

// Name returns the tool identifier.
func (t *NetHTTPProbeTool) Name() string {
	return "net_http_probe"
}

// Description returns a human-readable description.
func (t *NetHTTPProbeTool) Description() string {
	return "Performs lightweight HTTP requests to probe web endpoints."
}

// InputSchema returns the JSON Schema for tool validation.
func (t *NetHTTPProbeTool) InputSchema() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"url": map[string]interface{}{
				"type":        "string",
				"description": "URL to probe",
			},
			"method": map[string]interface{}{
				"type":        "string",
				"description": "HTTP method (default HEAD)",
			},
		},
		"required": []string{"url"},
	}
}

// Execute implements the tool logic.
func (t *NetHTTPProbeTool) Execute(ctx context.Context, args json.RawMessage) (CallToolResult, error) {
	var req struct {
		URL    string `json:"url"`
		Method string `json:"method,omitempty"`
	}
	if err := json.Unmarshal(args, &req); err != nil {
		return CallToolResult{}, fmt.Errorf("invalid arguments: %w", err)
	}

	if req.URL == "" {
		return CallToolResult{}, fmt.Errorf("url required")
	}

	method := req.Method
	if method == "" {
		method = "HEAD"
	}

	start := time.Now()
	httpReq, err := http.NewRequestWithContext(ctx, method, req.URL, nil)
	if err != nil {
		return CallToolResult{}, fmt.Errorf("failed to create request: %w", err)
	}

	timeout := defaultHTTPTimeout
	if deadline, ok := ctx.Deadline(); ok {
		timeout = time.Until(deadline)
		if timeout <= 0 {
			timeout = defaultHTTPTimeout
		}
	}

	client := &http.Client{Timeout: timeout}
	resp, err := client.Do(httpReq)
	latency := time.Since(start).Seconds() * 1000

	if err != nil {
		result := map[string]interface{}{
			"error":      err.Error(),
			"latency_ms": latency,
		}
		resultJSON, err := json.Marshal(result)
		if err != nil {
			return CallToolResult{}, fmt.Errorf("failed to marshal result: %w", err)
		}
		return CallToolResult{
			Content: []TextContent{
				{
					Type: "text",
					Text: string(resultJSON),
				},
			},
		}, nil
	}
	defer resp.Body.Close()

	headers := make(map[string]string)
	for k, v := range resp.Header {
		if len(v) > 0 {
			headers[k] = v[0]
		}
	}

	result := map[string]interface{}{
		"status_code": resp.StatusCode,
		"headers":     headers,
		"latency_ms":  latency,
	}
	resultJSON, err := json.Marshal(result)
	if err != nil {
		return CallToolResult{}, fmt.Errorf("failed to marshal result: %w", err)
	}

	return CallToolResult{
		Content: []TextContent{
			{
				Type: "text",
				Text: string(resultJSON),
			},
		},
	}, nil
}
