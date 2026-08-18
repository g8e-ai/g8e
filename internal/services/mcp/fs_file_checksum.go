// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package mcp

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
)

// FSFileChecksumRequest represents the input for the file checksum tool.
type FSFileChecksumRequest struct {
	FilePath string `json:"file_path"`
}

// FSFileChecksumResult represents the output of the file checksum tool.
type FSFileChecksumResult struct {
	FilePath  string `json:"file_path"`
	Algorithm string `json:"algorithm"`
	Checksum  string `json:"checksum"`
	SizeBytes int64  `json:"size_bytes"`
}

// FSFileChecksumTool computes SHA256 checksums for file integrity verification.
type FSFileChecksumTool struct{}

// Name returns the tool identifier.
func (t *FSFileChecksumTool) Name() string {
	return "fs_file_checksum"
}

// Description returns a human-readable description.
func (t *FSFileChecksumTool) Description() string {
	return "Computes SHA256 checksums for file integrity verification."
}

// InputSchema returns the JSON Schema for tool validation.
func (t *FSFileChecksumTool) InputSchema() *InputSchema {
	return &InputSchema{
		Type: "object",
		Properties: map[string]*PropertySchema{
			"file_path": {
				Type:        "string",
				Description: "Path to the file to checksum",
			},
		},
		Required: []string{"file_path"},
	}
}

// Execute implements the tool logic.
func (t *FSFileChecksumTool) Execute(ctx context.Context, args json.RawMessage) (CallToolResult, error) {
	var req FSFileChecksumRequest
	if err := json.Unmarshal(args, &req); err != nil {
		return CallToolResult{}, fmt.Errorf("fs_file_checksum: unmarshal arguments: %w", err)
	}

	if req.FilePath == "" {
		return CallToolResult{}, fmt.Errorf("fs_file_checksum: file_path required")
	}

	if err := validateFilePath(req.FilePath); err != nil {
		return CallToolResult{}, fmt.Errorf("fs_file_checksum: invalid file path: %w", err)
	}

	data, err := os.ReadFile(req.FilePath)
	if err != nil {
		return CallToolResult{}, fmt.Errorf("fs_file_checksum: failed to read file: %w", err)
	}

	hash := sha256.Sum256(data)
	checksum := hex.EncodeToString(hash[:])

	fileInfo, err := os.Stat(req.FilePath)
	if err != nil {
		return CallToolResult{}, fmt.Errorf("fs_file_checksum: failed to stat file: %w", err)
	}

	result := FSFileChecksumResult{
		FilePath:  req.FilePath,
		Algorithm: "sha256",
		Checksum:  checksum,
		SizeBytes: fileInfo.Size(),
	}

	resultJSON, err := json.Marshal(result)
	if err != nil {
		return CallToolResult{}, fmt.Errorf("fs_file_checksum: failed to marshal result: %w", err)
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
