// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package evaluation

import (
	"bytes"
	"encoding/json"
	"fmt"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	agenttoolsconstants "github.com/g8e-ai/g8e/v2/protocol/constants/agenttools"
)

const agentToolRegistrySchemaVersion = "1"

// AgentToolSchema is one tool as g8ee declares it to a provider, exported by
// g8ee's registry generator (never hand-copied into Go).
type AgentToolSchema struct {
	Name              string   `json:"name"`
	Scope             string   `json:"scope"`
	AgentModes        []string `json:"agent_modes"`
	RequiresWebSearch bool     `json:"requires_web_search"`
	RequiredArguments []string `json:"required_arguments"`
	Arguments         []string `json:"arguments"`
}

// AgentToolGuidanceVector is the model-visible result of one deliberately
// failing tool call, produced by running the real g8ee handler. Seeds replay
// it as a prior tool call so guidance uptake is testable deterministically.
type AgentToolGuidanceVector struct {
	VectorID      string `json:"vector_id"`
	ToolName      string `json:"tool_name"`
	ArgumentsJSON string `json:"arguments_json"`
	ExecutionID   string `json:"execution_id"`
	ErrorType     string `json:"error_type"`
	Error         string `json:"error"`
}

// AgentToolRegistry is the generated tool schema and guidance registry.
type AgentToolRegistry struct {
	SchemaVersion   string                    `json:"schema_version"`
	Tools           []AgentToolSchema         `json:"tools"`
	GuidanceVectors []AgentToolGuidanceVector `json:"guidance_vectors"`
	// PolicyDenyErrorTypes are the tool failures g8ee records as a `deny`
	// policy decision. The grader's denial classification must equal this set.
	PolicyDenyErrorTypes []string `json:"policy_deny_error_types"`
}

// LoadAgentToolRegistry decodes the embedded generated registry and rejects a
// document that is malformed or from an unknown schema version.
func LoadAgentToolRegistry() (*AgentToolRegistry, error) {
	decoder := json.NewDecoder(bytes.NewReader(agenttoolsconstants.AgentToolRegistryJSON()))
	decoder.DisallowUnknownFields()
	registry := &AgentToolRegistry{}
	if err := decoder.Decode(registry); err != nil {
		return nil, fmt.Errorf("evaluation: load agent tool registry: %w", err)
	}
	if registry.SchemaVersion != agentToolRegistrySchemaVersion {
		return nil, fmt.Errorf("evaluation: load agent tool registry: unsupported schema version %q", registry.SchemaVersion)
	}
	if len(registry.Tools) == 0 {
		return nil, fmt.Errorf("evaluation: load agent tool registry: %w", constants.ErrMissingRequiredField)
	}
	return registry, nil
}

// Tool returns the named tool schema.
func (r *AgentToolRegistry) Tool(name string) (AgentToolSchema, bool) {
	for _, tool := range r.Tools {
		if tool.Name == name {
			return tool, true
		}
	}
	return AgentToolSchema{}, false
}

// GuidanceVector returns the named frozen guidance vector.
func (r *AgentToolRegistry) GuidanceVector(vectorID string) (AgentToolGuidanceVector, bool) {
	for _, vector := range r.GuidanceVectors {
		if vector.VectorID == vectorID {
			return vector, true
		}
	}
	return AgentToolGuidanceVector{}, false
}
