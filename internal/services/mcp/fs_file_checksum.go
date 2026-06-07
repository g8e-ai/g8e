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
	"crypto/md5"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
)

// FSFileChecksumTool computes SHA256/MD5 checksums for file integrity verification.
type FSFileChecksumTool struct{}

// Name returns the tool identifier.
func (t *FSFileChecksumTool) Name() string {
	return "fs_file_checksum"
}

// Description returns a human-readable description.
func (t *FSFileChecksumTool) Description() string {
	return "Computes SHA256/MD5 checksums for file integrity verification."
}

// InputSchema returns the JSON Schema for tool validation.
func (t *FSFileChecksumTool) InputSchema() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"file_path": map[string]interface{}{
				"type":        "string",
				"description": "Path to the file to checksum",
			},
			"algorithm": map[string]interface{}{
				"type":        "string",
				"description": "Checksum algorithm (sha256 or md5, default sha256)",
				"enum":        []string{"sha256", "md5"},
			},
		},
		"required": []string{"file_path"},
	}
}

// Execute implements the tool logic.
func (t *FSFileChecksumTool) Execute(ctx context.Context, args json.RawMessage) (CallToolResult, error) {
	var req struct {
		FilePath  string `json:"file_path"`
		Algorithm string `json:"algorithm,omitempty"`
	}
	if err := json.Unmarshal(args, &req); err != nil {
		return CallToolResult{}, fmt.Errorf("invalid arguments: %w", err)
	}

	if req.FilePath == "" {
		return CallToolResult{}, fmt.Errorf("file_path required")
	}

	if err := validateFilePath(req.FilePath); err != nil {
		return CallToolResult{}, fmt.Errorf("invalid file path: %w", err)
	}

	algorithm := req.Algorithm
	if algorithm == "" {
		algorithm = "sha256"
	}

	data, err := os.ReadFile(req.FilePath)
	if err != nil {
		return CallToolResult{}, fmt.Errorf("failed to read file: %w", err)
	}

	var checksum string
	switch algorithm {
	case "sha256":
		hash := sha256.Sum256(data)
		checksum = hex.EncodeToString(hash[:])
	case "md5":
		hash := md5.Sum(data)
		checksum = hex.EncodeToString(hash[:])
	default:
		return CallToolResult{}, fmt.Errorf("unsupported algorithm: %s", algorithm)
	}

	fileInfo, err := os.Stat(req.FilePath)
	if err != nil {
		return CallToolResult{}, fmt.Errorf("failed to stat file: %w", err)
	}

	result := map[string]interface{}{
		"file_path":  req.FilePath,
		"algorithm":  algorithm,
		"checksum":   checksum,
		"size_bytes": fileInfo.Size(),
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
