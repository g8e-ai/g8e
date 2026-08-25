// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/g8e-ai/g8e/v2/internal/constants"
)

// fileOpener is an interface for opening files, enabling test mocking.
type fileOpener interface {
	Open(name string) (*os.File, error)
}

// osFileOpener implements fileOpener using os.Open.
type osFileOpener struct{}

func (o *osFileOpener) Open(name string) (*os.File, error) {
	return os.Open(name)
}

// NetSocketAuditTool inspects active network sockets.
type NetSocketAuditTool struct {
	fileOpener fileOpener
}

// Name returns the tool identifier.
func (t *NetSocketAuditTool) Name() string {
	return "net_socket_audit"
}

// Description returns a human-readable description.
func (t *NetSocketAuditTool) Description() string {
	return "Inspects active network sockets (TCP/UDP) from /proc/net."
}

// InputSchema returns the JSON Schema for tool validation.
func (t *NetSocketAuditTool) InputSchema() *InputSchema {
	return &InputSchema{
		Type: "object",
		Properties: map[string]*PropertySchema{
			"protocol": {
				Type:        "string",
				Description: "Protocol filter (tcp, udp, or empty for both)",
			},
		},
	}
}

// Execute implements the tool logic.
func (t *NetSocketAuditTool) Execute(ctx context.Context, args json.RawMessage) (CallToolResult, error) {
	var req NetSocketAuditRequest
	if err := json.Unmarshal(args, &req); err != nil {
		return CallToolResult{}, fmt.Errorf("%w: %v", constants.ErrMCPUnmarshalArguments, err)
	}

	// Use default file opener if not set
	opener := t.fileOpener
	if opener == nil {
		opener = &osFileOpener{}
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
		path := getProcNetPath(proto)
		// proto is validated by validateProcNetPath to satisfy CodeQL uncontrolled-data-in-path-expression rule.
		file, err := opener.Open(path)
		if err != nil {
			// Skip protocols that are not available on this system
			continue
		}

		protoSockets, err := parseProcNetFile(ctx, file, proto)
		file.Close()
		if err != nil {
			// Log and continue with other protocols
			continue
		}

		sockets = append(sockets, protoSockets...)
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

// getProcNetPath returns the /proc/net path for a given protocol.
func getProcNetPath(protocol string) string {
	switch protocol {
	case "tcp":
		return constants.PathProcNetTCP
	case "udp":
		return constants.PathProcNetUDP
	case "tcp6":
		return constants.PathProcNetTCP6
	case "udp6":
		return constants.PathProcNetUDP6
	case "raw":
		return constants.PathProcNetRaw
	default:
		return fmt.Sprintf("%s/%s", constants.PathProcNet, protocol)
	}
}

// parseProcNetFile parses a /proc/net protocol file and extracts socket information.
// Minimum field count is 10: sl local_address rem_address st tx_queue rx_queue ...
const minProcNetFields = 10

func parseProcNetFile(ctx context.Context, file *os.File, protocol string) ([]SocketInfo, error) {
	var sockets []SocketInfo
	scanner := bufio.NewScanner(file)

	// Skip header line
	if !scanner.Scan() {
		return sockets, nil
	}

	for scanner.Scan() {
		if ctx.Err() != nil {
			return sockets, ctx.Err()
		}

		line := scanner.Text()
		fields := strings.Fields(line)
		if len(fields) < minProcNetFields {
			continue
		}

		localAddr := fields[1]
		remoteAddr := fields[2]
		state := ""
		if len(fields) > 3 {
			state = fields[3]
		}

		// /proc/net format uses colon to separate IP and port (e.g., 0100007F:1F90).
		// Skip lines where either address field lacks the expected colon separator.
		if !strings.Contains(localAddr, ":") || !strings.Contains(remoteAddr, ":") {
			continue
		}
		localAddr = strings.ReplaceAll(localAddr, ":", "")
		remoteAddr = strings.ReplaceAll(remoteAddr, ":", "")

		localIP, localPort, err := parseSocketAddr(localAddr)
		if err != nil {
			continue
		}
		remoteIP, remotePort, err := parseSocketAddr(remoteAddr)
		if err != nil {
			continue
		}

		sockets = append(sockets, SocketInfo{
			Protocol:   protocol,
			LocalAddr:  localIP,
			LocalPort:  localPort,
			RemoteAddr: remoteIP,
			RemotePort: remotePort,
			State:      state,
		})
	}

	if err := scanner.Err(); err != nil {
		return sockets, fmt.Errorf("net_socket_audit: scan %s file: %w", protocol, err)
	}

	return sockets, nil
}
