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

	// InferenceEnabled is true when the Operator started with
	// --inference-enabled, marking it as an Inference Node in the g8ellama
	// topology. The User Gateway's inference dispatch service resolves the
	// Inference Node's operator session by querying for operators with this
	// field set.
	InferenceEnabled bool `json:"inference_enabled"`

	// InferenceOllamaEndpoint is the approved remote Ollama provider URL
	// configured when the Inference Node started with --inference-ollama-endpoint.
	InferenceOllamaEndpoint string `json:"inference_ollama_endpoint,omitempty"`

	// ProviderBoundaryObserverEnabled is true when the Operator started with
	// --provider-boundary-observer-enabled, marking it as the read-only
	// remote hardware observer on the approved provider host.
	ProviderBoundaryObserverEnabled bool `json:"provider_boundary_observer_enabled"`

	// ProviderBoundaryObserverOllamaEnabled is true when the provider-boundary
	// Observer Operator started with --ollama, opting in to remote Ollama
	// service lifecycle commands (stop/start/status) on the provider host.
	ProviderBoundaryObserverOllamaEnabled bool `json:"provider_boundary_observer_ollama_enabled,omitempty"`

	// Platform is the operator host GOOS recorded at startup (for example
	// "linux" or "windows") so remote callers can choose host-appropriate
	// settle commands.
	Platform string `json:"platform,omitempty"`
}
