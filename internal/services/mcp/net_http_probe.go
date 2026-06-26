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
	"net"
	"net/http"
	"time"

	"github.com/g8e-ai/g8e/internal/constants"
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
func (t *NetHTTPProbeTool) InputSchema() *InputSchema {
	return &InputSchema{
		Type: "object",
		Properties: map[string]*PropertySchema{
			"url": {
				Type:        "string",
				Description: "URL to probe",
			},
			"method": {
				Type:        "string",
				Description: "HTTP method (default HEAD)",
			},
		},
		Required: []string{"url"},
	}
}

// Execute implements the tool logic.
func (t *NetHTTPProbeTool) Execute(ctx context.Context, args json.RawMessage) (CallToolResult, error) {
	var req NetHTTPProbeRequest
	if err := json.Unmarshal(args, &req); err != nil {
		return CallToolResult{}, fmt.Errorf("net_http_probe: invalid arguments: %w", err)
	}

	if req.URL == "" {
		return CallToolResult{}, fmt.Errorf("net_http_probe: url required")
	}

	parsedURL, err := validateHTTPRequestURL(req.URL)
	if err != nil {
		result := NetHTTPProbeResult{
			Error: fmt.Sprintf("URL validation failed: %v", err),
		}
		resultJSON, _ := json.Marshal(result)
		return CallToolResult{
			Content: []TextContent{
				{
					Type: "text",
					Text: string(resultJSON),
				},
			},
		}, nil
	}

	method := req.Method
	if method == "" {
		method = "HEAD"
	}

	start := time.Now()
	httpReq, err := http.NewRequestWithContext(ctx, method, parsedURL.String(), nil)
	if err != nil {
		return CallToolResult{}, fmt.Errorf("net_http_probe: failed to create request: %w", err)
	}

	timeout := constants.DefaultHTTPTimeout
	if deadline, ok := ctx.Deadline(); ok {
		timeout = time.Until(deadline)
		if timeout <= 0 {
			timeout = constants.DefaultHTTPTimeout
		}
	}

	transport := &http.Transport{
		DialContext: func(dialCtx context.Context, network, addr string) (net.Conn, error) {
			host, _, err := net.SplitHostPort(addr)
			if err != nil {
				host = addr
			}
			ip := net.ParseIP(host)
			if ip != nil && (ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast()) {
				if !isIPAllowed(ip) {
					return nil, fmt.Errorf("blocked address: %s", host)
				}
			}
			d := net.Dialer{}
			return d.DialContext(dialCtx, network, addr)
		},
	}
	client := &http.Client{Timeout: timeout, Transport: transport}
	// parsedURL is validated by validateHTTPRequestURL to satisfy CodeQL uncontrolled-data-in-network-request rule.
	resp, err := client.Do(httpReq)
	latency := time.Since(start).Seconds() * 1000

	if err != nil {
		result := NetHTTPProbeResult{
			Error:     err.Error(),
			LatencyMs: latency,
		}
		resultJSON, err := json.Marshal(result)
		if err != nil {
			return CallToolResult{}, fmt.Errorf("net_http_probe: failed to marshal result: %w", err)
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

	result := NetHTTPProbeResult{
		StatusCode: resp.StatusCode,
		Headers:    headers,
		LatencyMs:  latency,
	}
	resultJSON, err := json.Marshal(result)
	if err != nil {
		return CallToolResult{}, fmt.Errorf("net_http_probe: failed to marshal result: %w", err)
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
