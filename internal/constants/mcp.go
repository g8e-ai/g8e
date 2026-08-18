// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package constants

import "time"

// MCP service constants
const (
	// DefaultLogFilterLimit is the default number of log lines to return
	DefaultLogFilterLimit = 100

	// DefaultProcessLimit is the default number of processes to return in metric tools
	DefaultProcessLimit = 10

	// DefaultDiskProfileDepth is the default directory depth for disk profiling
	DefaultDiskProfileDepth = 2

	// DefaultNetworkTimeout is the default timeout for network operations
	DefaultNetworkTimeout = 5 * time.Second

	// DefaultHTTPTimeout is the default timeout for HTTP operations
	DefaultHTTPTimeout = 10 * time.Second

	// SSHKeepaliveRequestType is the SSH request type for keepalive packets
	SSHKeepaliveRequestType = "keepalive@g8e"

	// SSHKeepaliveInterval is the interval between SSH keepalive packets
	SSHKeepaliveInterval = 15 * time.Second

	// SSHKeepaliveMaxMissed is the maximum number of missed keepalive responses before failure
	SSHKeepaliveMaxMissed = 3

	// SSHMaxRetries is the maximum number of retry attempts for transient SSH errors
	SSHMaxRetries = 3

	// SSHCaptureMaxBytes is the maximum number of bytes to capture from remote stdout/stderr
	SSHCaptureMaxBytes = 64 * 1024

	// SSHPreflightVerifyCommand is the minimal command run to verify remote shell works
	SSHPreflightVerifyCommand = "true"

	// SSHProxyAddrLabel is the network address label for proxy connections
	SSHProxyAddrLabel = "proxy"

	// MCPTransportStdio is the transport type string for stdio MCP transport.
	MCPTransportStdio = "stdio"

	// MCPServerNameG8E is the MCP server name key used in agent config files
	// (mcpServers map key, Goose extension name).
	MCPServerNameG8E = "g8e"

	// MCPG8EDescription is the human-readable description for the g8e MCP server
	// entry in agent extension configs.
	MCPG8EDescription = "g8e governance gateway"

	// GooseExtTimeout is the timeout in seconds for the g8e Goose extension entry.
	GooseExtTimeout = 300
)
