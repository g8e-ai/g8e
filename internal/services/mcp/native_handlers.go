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
	"regexp"
	"strconv"
	"strings"
	"time"
)

// isValidIdentifier validates SQLite identifiers to prevent SQL injection.
// SQLite identifiers must start with a letter or underscore, followed by letters, digits, or underscores.
func isValidIdentifier(name string) bool {
	if len(name) == 0 {
		return false
	}
	for i, r := range name {
		if i == 0 {
			if (r < 'a' || r > 'z') && (r < 'A' || r > 'Z') && r != '_' {
				return false
			}
		} else {
			if (r < 'a' || r > 'z') && (r < 'A' || r > 'Z') && (r < '0' || r > '9') && r != '_' {
				return false
			}
		}
	}
	return true
}

const (
	defaultLogFilterLimit   = 100
	defaultProcessLimit     = 10
	defaultDiskProfileDepth = 2
	defaultNetworkTimeout   = 5 * time.Second
	defaultHTTPTimeout      = 10 * time.Second
)

// NativeToolHandler executes native tools compiled into the Node binary.
type NativeToolHandler struct {
	registry *ToolRegistry
}

// NewNativeToolHandler creates a new native tool handler with all native tools
// explicitly registered. This avoids init()-based auto-registration and
// mutable global state.
func NewNativeToolHandler() (*NativeToolHandler, error) {
	registry := NewToolRegistry()
	if err := RegisterNativeTools(registry); err != nil {
		return nil, fmt.Errorf("native tool registration failed: %w", err)
	}
	return &NativeToolHandler{
		registry: registry,
	}, nil
}

// NewNativeToolHandlerWithRegistry creates a new native tool handler with a custom registry.
func NewNativeToolHandlerWithRegistry(registry *ToolRegistry) *NativeToolHandler {
	return &NativeToolHandler{
		registry: registry,
	}
}

// HandleTool executes a native tool by name and returns the result.
func (h *NativeToolHandler) HandleTool(ctx context.Context, toolName string, arguments json.RawMessage) (CallToolResult, error) {
	tool, ok := h.registry.Get(toolName)
	if !ok {
		return CallToolResult{}, fmt.Errorf("unknown native tool: %s", toolName)
	}
	return tool.Execute(ctx, arguments)
}

// ListTools returns all registered native tools.
func (h *NativeToolHandler) ListTools() []NativeTool {
	return h.registry.List()
}

// scrubLine redacts sensitive patterns from log lines.
func scrubLine(line string) string {
	scrubbed := line

	sensitivePatterns := []struct {
		pattern string
		repl    string
	}{
		{`password[=:]\s*\S+`, "password=REDACTED"},
		{`api[_-]?key[=:]\s*\S+`, "api_key=REDACTED"},
		{`secret[=:]\s*\S+`, "secret=REDACTED"},
		{`token[=:]\s*\S+`, "token=REDACTED"},
		{`bearer\s+\S+`, "bearer REDACTED"},
	}

	for _, sp := range sensitivePatterns {
		re := regexp.MustCompile(`(?i)` + sp.pattern)
		scrubbed = re.ReplaceAllString(scrubbed, sp.repl)
	}

	return scrubbed
}

// maskSecret redacts secret values in configuration lines.
func maskSecret(line string) string {
	if strings.Contains(strings.ToLower(line), "password") ||
		strings.Contains(strings.ToLower(line), "secret") ||
		strings.Contains(strings.ToLower(line), "token") ||
		strings.Contains(strings.ToLower(line), "key") {
		return "REDACTED"
	}
	return line
}

// parseSocketAddr parses /proc/net socket address format.
func parseSocketAddr(hexAddr string) (string, int, error) {
	if len(hexAddr) < 8 {
		return "0.0.0.0", 0, nil
	}

	portHex := hexAddr[len(hexAddr)-4:]
	ipHex := hexAddr[:len(hexAddr)-4]

	port, err := strconv.ParseInt(portHex, 16, 32)
	if err != nil {
		return "0.0.0.0", 0, fmt.Errorf("parse port: %w", err)
	}

	var ip string
	if len(ipHex) == 8 {
		p1, err := strconv.ParseInt(ipHex[6:8], 16, 32)
		if err != nil {
			return "0.0.0.0", 0, fmt.Errorf("parse ip octet 1: %w", err)
		}
		p2, err := strconv.ParseInt(ipHex[4:6], 16, 32)
		if err != nil {
			return "0.0.0.0", 0, fmt.Errorf("parse ip octet 2: %w", err)
		}
		p3, err := strconv.ParseInt(ipHex[2:4], 16, 32)
		if err != nil {
			return "0.0.0.0", 0, fmt.Errorf("parse ip octet 3: %w", err)
		}
		p4, err := strconv.ParseInt(ipHex[0:2], 16, 32)
		if err != nil {
			return "0.0.0.0", 0, fmt.Errorf("parse ip octet 4: %w", err)
		}
		ip = fmt.Sprintf("%d.%d.%d.%d", p1, p2, p3, p4)
	} else {
		ip = "unknown"
	}

	return ip, int(port), nil
}
