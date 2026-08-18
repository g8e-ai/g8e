// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package models

// FsGrepMatch represents a single grep match result
type FsGrepMatch struct {
	Path       string   `json:"path"`
	LineNumber int      `json:"line_number"`
	Content    string   `json:"content"`
	Before     []string `json:"before,omitempty"`
	After      []string `json:"after,omitempty"`
}

// RuntimeConfig captures the CLI flags and env var overrides active when the Operator was started.
// Sent to client at bootstrap and stored in operator_document.runtime_config.
type RuntimeConfig struct {
	CloudMode             bool   `json:"cloud_mode"`
	CloudProvider         string `json:"cloud_provider,omitempty"`
	ExecutionVaultEnabled bool   `json:"local_storage_enabled"`
	NoGit                 bool   `json:"no_git"`
	LogLevel              string `json:"log_level"`

	HTTPPort int `json:"http_port"`
}
