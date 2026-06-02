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
func (t *SysEnvVarsTool) InputSchema() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"pattern": map[string]interface{}{
				"type":        "string",
				"description": "Optional pattern to filter variable names (e.g., 'G8E_*' for g8e-specific vars)",
			},
			"redact_secrets": map[string]interface{}{
				"type":        "boolean",
				"description": "Whether to redact sensitive values (default true)",
			},
		},
	}
}

// Execute implements the tool logic.
func (t *SysEnvVarsTool) Execute(ctx context.Context, args json.RawMessage) (CallToolResult, error) {
	var req struct {
		Pattern       string `json:"pattern,omitempty"`
		RedactSecrets bool   `json:"redact_secrets,omitempty"`
	}
	if err := json.Unmarshal(args, &req); err != nil {
		return CallToolResult{}, fmt.Errorf("invalid arguments: %w", err)
	}

	if req.RedactSecrets == false {
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

	result := map[string]interface{}{
		"count":     len(filteredVars),
		"variables": filteredVars,
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
