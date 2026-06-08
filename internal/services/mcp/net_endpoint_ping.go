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
	"strconv"
	"time"

	"github.com/g8e-ai/g8e/internal/constants"
)

// NetEndpointPingTool performs TCP handshake to verify connectivity.
type NetEndpointPingTool struct{}

// Name returns the tool identifier
func (t *NetEndpointPingTool) Name() string {
	return "net_endpoint_ping"
}

// Description returns a human-readable description
func (t *NetEndpointPingTool) Description() string {
	return "Performs TCP handshake to verify network endpoint connectivity and measure latency"
}

// InputSchema returns the JSON Schema for tool validation
func (t *NetEndpointPingTool) InputSchema() *InputSchema {
	return &InputSchema{
		Type: "object",
		Properties: map[string]*PropertySchema{
			"host": {
				Type:        "string",
				Description: "Hostname or IP address",
			},
			"port": {
				Type:        "integer",
				Description: "Port number",
			},
		},
		Required: []string{"host", "port"},
	}
}

// Execute implements the tool logic
func (t *NetEndpointPingTool) Execute(ctx context.Context, args json.RawMessage) (CallToolResult, error) {
	var req NetEndpointPingRequest
	if err := json.Unmarshal(args, &req); err != nil {
		return CallToolResult{}, fmt.Errorf("invalid arguments: %w", err)
	}

	if req.Host == "" || req.Port <= 0 {
		return CallToolResult{}, fmt.Errorf("host and port required")
	}

	address := net.JoinHostPort(req.Host, strconv.Itoa(req.Port))
	start := time.Now()

	dialer := &net.Dialer{Timeout: defaultNetworkTimeout}
	if deadline, ok := ctx.Deadline(); ok {
		dialer.Timeout = time.Until(deadline)
		if dialer.Timeout <= 0 {
			dialer.Timeout = defaultNetworkTimeout
		}
	}

	conn, err := dialer.DialContext(ctx, string(constants.NetworkProtocolTCP), address)
	latency := time.Since(start).Seconds() * 1000

	if err != nil {
		result := NetEndpointPingResult{
			Reachable: false,
			LatencyMs: latency,
			Error:     err.Error(),
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
	defer conn.Close()

	result := NetEndpointPingResult{
		Reachable: true,
		LatencyMs: latency,
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
