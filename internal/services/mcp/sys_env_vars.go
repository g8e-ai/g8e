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
	"os"
	"strings"
)

// SysEnvVarsTool reads environment variables for configuration debugging.
type SysEnvVarsTool struct{}

// Name returns the tool identifier.
func (t *SysEnvVarsTool) Name() string {
	return "sys_env_vars"
}

// Description returns a human-readable description.
func (t *SysEnvVarsTool) Description() string {
	return "Reads environment variables for configuration debugging."
}

// InputSchema returns the JSON Schema for tool validation.
func (t *SysEnvVarsTool) InputSchema() *InputSchema {
	return &InputSchema{
		Type: "object",
		Properties: map[string]*PropertySchema{
			"pattern": {
				Type:        "string",
				Description: "Optional pattern to filter variable names (e.g., 'G8E_*' for g8e-specific vars)",
			},
			"redact_secrets": {
				Type:        "boolean",
				Description: "Whether to redact sensitive values (default true)",
			},
		},
	}
}

// Execute implements the tool logic.
func (t *SysEnvVarsTool) Execute(ctx context.Context, args json.RawMessage) (CallToolResult, error) {
	var req SysEnvVarsRequest
	if err := json.Unmarshal(args, &req); err != nil {
		return CallToolResult{}, fmt.Errorf("invalid arguments: %w", err)
	}

	if !req.RedactSecrets {
		req.RedactSecrets = true
	}

	envVars := os.Environ()
	filteredVars := make(map[string]string)

	for _, env := range envVars {
		parts := strings.SplitN(env, "=", 2)
		if len(parts) != 2 {
			continue
		}
		key := parts[0]
		value := parts[1]

		if req.Pattern != "" {
			matched, err := matchPattern(key, req.Pattern)
			if err != nil || !matched {
				continue
			}
		}

		if req.RedactSecrets {
			value = redactEnvValue(key, value)
		}

		filteredVars[key] = value
	}

	result := SysEnvVarsResult{
		Count:     len(filteredVars),
		Variables: filteredVars,
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

func matchPattern(key, pattern string) (bool, error) {
	if pattern == "" {
		return true, nil
	}

	lowerKey := strings.ToLower(key)
	lowerPattern := strings.ToLower(pattern)

	if strings.Contains(lowerPattern, "*") {
		prefix := strings.Split(lowerPattern, "*")[0]
		suffix := ""
		if strings.Count(lowerPattern, "*") > 1 {
			parts := strings.Split(lowerPattern, "*")
			if len(parts) > 1 {
				suffix = parts[len(parts)-1]
			}
		}
		return strings.HasPrefix(lowerKey, prefix) && strings.HasSuffix(lowerKey, suffix), nil
	}

	return strings.EqualFold(key, pattern), nil
}

func redactEnvValue(key, value string) string {
	lowerKey := strings.ToLower(key)

	sensitiveKeywords := []string{
		"password", "passwd", "secret", "token", "key", "api_key",
		"private_key", "auth", "credential", "cert", "certificate",
	}

	for _, keyword := range sensitiveKeywords {
		if strings.Contains(lowerKey, keyword) {
			if len(value) > 0 {
				return "REDACTED"
			}
		}
	}

	return value
}
