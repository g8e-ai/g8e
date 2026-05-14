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

package models

// FsGrepMatch represents a single grep match result
type FsGrepMatch struct {
	Path       string   `json:"path"`
	LineNumber int      `json:"line_number"`
	Content    string   `json:"content"`
	Before     []string `json:"before,omitempty"`
	After      []string `json:"after,omitempty"`
}

// RuntimeConfig captures the CLI flags and env var overrides active when the operator was started.
// Sent to g8ed at bootstrap and stored in operator_document.runtime_config.
type RuntimeConfig struct {
	CloudMode           bool   `json:"cloud_mode"`
	CloudProvider       string `json:"cloud_provider,omitempty"`
	LocalStorageEnabled bool   `json:"local_storage_enabled"`
	NoGit               bool   `json:"no_git"`
	LogLevel            string `json:"log_level"`
	WSSPort             int    `json:"wss_port"`
	HTTPPort            int    `json:"http_port"`
}
