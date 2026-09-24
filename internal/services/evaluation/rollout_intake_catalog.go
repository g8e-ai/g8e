// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package evaluation

import (
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/g8e-ai/g8e/v2/internal/constants"
)

const (
	DefaultRolloutIntakeCatalogRelPath = "eval/rollout-intake-hf.json"
	defaultRolloutIntakePullTimeout    = 24 * time.Hour
	defaultRolloutIntakeConnectTimeout = 2 * time.Minute
)

// DefaultRolloutIntakePullTimeout returns the default HF pull deadline.
func DefaultRolloutIntakePullTimeout() time.Duration {
	return defaultRolloutIntakePullTimeout
}

// RolloutIntakeCatalog describes rollout-priority models and their HF/Ollama staging metadata.
type RolloutIntakeCatalog struct {
	Description               string                `json:"description"`
	OllamaEndpoint            string                `json:"ollama_endpoint"`
	PullTimeoutSeconds        int64                 `json:"pull_timeout_seconds"`
	PullConnectTimeoutSeconds int64                 `json:"pull_connect_timeout_seconds"`
	HFTokenEnv                string                `json:"hf_token_env"`
	PriorityOrder             []string              `json:"priority_order"`
	Models                    []RolloutIntakeModel  `json:"models"`
}

// RolloutIntakeModel is one catalog entry for rollout intake staging.
type RolloutIntakeModel struct {
	VariantID      string                 `json:"variant_id"`
	ServedModelTag string                 `json:"served_model_tag"`
	ModelFamily    string                 `json:"model_family"`
	ParameterCount string                 `json:"parameter_count"`
	Quantization   string                 `json:"quantization"`
	ContextLimit   uint32                 `json:"context_limit,omitempty"`
	Notes          string                 `json:"notes,omitempty"`
	Staging        RolloutIntakeStaging   `json:"staging"`
}

// RolloutIntakeStaging describes how one intake model is staged on the provider.
type RolloutIntakeStaging struct {
	Method           string `json:"method"`
	Status           string `json:"status,omitempty"`
	HFRepo           string `json:"hf_repo,omitempty"`
	HFGGUFRepo       string `json:"hf_gguf_repo,omitempty"`
	HFGGUFFile       string `json:"hf_gguf_file,omitempty"`
	OllamaPull       string `json:"ollama_pull,omitempty"`
	OllamaPullError  string `json:"ollama_pull_error,omitempty"`
	Alias            string `json:"alias,omitempty"`
}

// LoadRolloutIntakeCatalog reads the checked-in rollout intake catalog.
func LoadRolloutIntakeCatalog(path string) (*RolloutIntakeCatalog, error) {
	if path == "" {
		path = DefaultRolloutIntakeCatalogRelPath
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("evaluation: load rollout intake catalog: %w", err)
	}
	catalog := &RolloutIntakeCatalog{}
	if err := json.Unmarshal(data, catalog); err != nil {
		return nil, fmt.Errorf("evaluation: load rollout intake catalog: decode: %w", err)
	}
	if len(catalog.Models) == 0 {
		return nil, fmt.Errorf("evaluation: load rollout intake catalog: %w", constants.ErrMissingRequiredField)
	}
	return catalog, nil
}

func (catalog *RolloutIntakeCatalog) pullTimeout() time.Duration {
	if catalog == nil || catalog.PullTimeoutSeconds <= 0 {
		return defaultRolloutIntakePullTimeout
	}
	return time.Duration(catalog.PullTimeoutSeconds) * time.Second
}
