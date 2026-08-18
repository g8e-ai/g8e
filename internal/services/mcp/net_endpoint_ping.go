// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

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

// marshalResult converts a result struct to a CallToolResult with JSON text content.
func marshalResult(result interface{}) (CallToolResult, error) {
	resultJSON, err := json.Marshal(result)
	if err != nil {
		return CallToolResult{}, fmt.Errorf("net_endpoint_ping: failed to marshal result: %w", err)
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
		return CallToolResult{}, fmt.Errorf("net_endpoint_ping: invalid arguments: %w", err)
	}

	if req.Host == "" || req.Port <= 0 {
		return CallToolResult{}, fmt.Errorf("net_endpoint_ping: %w", constants.ErrMCPHostPortRequired)
	}

	address := net.JoinHostPort(req.Host, strconv.Itoa(req.Port))
	start := time.Now()

	dialer := &net.Dialer{Timeout: constants.DefaultNetworkTimeout}
	if deadline, ok := ctx.Deadline(); ok {
		dialer.Timeout = time.Until(deadline)
		if dialer.Timeout <= 0 {
			dialer.Timeout = constants.DefaultNetworkTimeout
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
		return marshalResult(result)
	}
	defer conn.Close()

	result := NetEndpointPingResult{
		Reachable: true,
		LatencyMs: latency,
	}
	return marshalResult(result)
}
