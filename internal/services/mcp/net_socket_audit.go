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
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/g8e-ai/g8e/internal/constants"
)

// NetSocketAuditTool inspects active network sockets.
type NetSocketAuditTool struct{}

// Name returns the tool identifier.
func (t *NetSocketAuditTool) Name() string {
	return "net_socket_audit"
}

// Description returns a human-readable description.
func (t *NetSocketAuditTool) Description() string {
	return "Inspects active network sockets (TCP/UDP) from /proc/net."
}

// InputSchema returns the JSON Schema for tool validation.
func (t *NetSocketAuditTool) InputSchema() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"protocol": map[string]interface{}{
				"type":        "string",
				"description": "Protocol filter (tcp, udp, or empty for both)",
			},
		},
	}
}

// Execute implements the tool logic.
func (t *NetSocketAuditTool) Execute(ctx context.Context, args json.RawMessage) (CallToolResult, error) {
	var req NetSocketAuditRequest
	if err := json.Unmarshal(args, &req); err != nil {
		return CallToolResult{}, fmt.Errorf("net_socket_audit: unmarshal arguments: %w", err)
	}

	var sockets []SocketInfo
	protocols := []string{string(constants.NetworkProtocolTCP), string(constants.NetworkProtocolUDP)}

	if req.Protocol != "" {
		proto := strings.ToLower(req.Protocol)
		if err := validateProcNetPath(proto); err != nil {
			return CallToolResult{}, fmt.Errorf("net_socket_audit: validate protocol: %w", err)
		}
		protocols = []string{proto}
	}

	for _, proto := range protocols {
		path := fmt.Sprintf("/proc/net/%s", proto)
		file, err := os.Open(path)
		if err != nil {
			continue
		}
		defer file.Close()

		scanner := bufio.NewScanner(file)
		scanner.Scan()

		for scanner.Scan() {
			if ctx.Err() != nil {
				return CallToolResult{}, ctx.Err()
			}

			line := scanner.Text()
			fields := strings.Fields(line)
			if len(fields) < 10 {
				continue
			}

			localAddr := fields[1]
			remoteAddr := fields[2]
			state := ""
			if len(fields) > 3 {
				state = fields[3]
			}

			localIP, localPort, err := parseSocketAddr(localAddr)
			if err != nil {
				continue
			}
			remoteIP, remotePort, err := parseSocketAddr(remoteAddr)
			if err != nil {
				continue
			}

			sockets = append(sockets, SocketInfo{
				Protocol:   proto,
				LocalAddr:  localIP,
				LocalPort:  localPort,
				RemoteAddr: remoteIP,
				RemotePort: remotePort,
				State:      state,
			})
		}

		if err := scanner.Err(); err != nil {
			continue
		}
	}

	result := NetSocketAuditResult{
		Sockets: sockets,
	}
	resultJSON, err := json.Marshal(result)
	if err != nil {
		return CallToolResult{}, fmt.Errorf("net_socket_audit: marshal result: %w", err)
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
