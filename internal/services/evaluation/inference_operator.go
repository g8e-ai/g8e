// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package evaluation

import (
	"fmt"
	"os"
	"strings"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/models"
)

// InferenceOperatorStatus summarizes one active inference-capable Operator
// session discovered through the owner-authenticated operator registry.
type InferenceOperatorStatus struct {
	OperatorID        string
	OperatorSessionID string
	Status            string
	InferenceEnabled  bool
	OllamaEndpoint    string
}

// ActiveInferenceOperators returns every active remote Operator with
// runtime_config.inference_enabled set.
func ActiveInferenceOperators(operators []models.OperatorDocumentGo) []InferenceOperatorStatus {
	matches := make([]InferenceOperatorStatus, 0)
	for _, op := range operators {
		if op.Status != constants.OperatorStatusActive || op.OperatorType != constants.OperatorTypeRemote {
			continue
		}
		if op.RuntimeConfig == nil || !op.RuntimeConfig.InferenceEnabled {
			continue
		}
		if op.OperatorSessionID == "" {
			continue
		}
		matches = append(matches, InferenceOperatorStatus{
			OperatorID:        op.ID,
			OperatorSessionID: op.OperatorSessionID,
			Status:            string(op.Status),
			InferenceEnabled:  true,
			OllamaEndpoint:    inferenceOperatorOllamaEndpoint(op),
		})
	}
	return matches
}

// SelectInferenceOperator resolves exactly one inference-capable Operator.
// When sessionID is non-empty it must match an active inference Operator;
// otherwise exactly one active inference Operator must exist.
func SelectInferenceOperator(operators []models.OperatorDocumentGo, sessionID string) (*InferenceOperatorStatus, error) {
	matches := ActiveInferenceOperators(operators)
	if sessionID != "" {
		for _, match := range matches {
			if match.OperatorSessionID == sessionID {
				selected := match
				return &selected, nil
			}
		}
		return nil, fmt.Errorf("%w: session %s", constants.ErrInferenceOperatorNotCapable, sessionID)
	}
	switch len(matches) {
	case 0:
		return nil, constants.ErrInferenceOperatorNotFound
	case 1:
		selected := matches[0]
		return &selected, nil
	default:
		return nil, constants.ErrInferenceOperatorAmbiguous
	}
}

func inferenceOperatorOllamaEndpoint(op models.OperatorDocumentGo) string {
	if op.RuntimeConfig == nil {
		return ""
	}
	return strings.TrimSpace(op.RuntimeConfig.InferenceOllamaEndpoint)
}

// GovernedInferenceOllamaEndpoint returns the approved provider endpoint from
// the exact Inference Operator runtime_config. Campaign-host environment
// overrides are intentionally rejected.
func GovernedInferenceOllamaEndpoint(operators []models.OperatorDocumentGo, inferenceSessionID string) (string, error) {
	if inferenceSessionID == "" {
		return "", fmt.Errorf("evaluation: governed inference endpoint: %w", constants.ErrMissingRequiredField)
	}
	selected, err := SelectInferenceOperator(operators, inferenceSessionID)
	if err != nil {
		return "", fmt.Errorf("evaluation: governed inference endpoint: %w", err)
	}
	endpoint := strings.TrimSpace(selected.OllamaEndpoint)
	if endpoint == "" {
		return "", fmt.Errorf("evaluation: governed inference endpoint: %w", constants.ErrInferenceEndpointInvalid)
	}
	return endpoint, nil
}

// ResolveInferenceOllamaEndpoint selects the Ollama provider URL for campaign
// model maintenance and residency checking. Resolution order: explicit flag override, process
// environment, active inference operator runtime_config, then loopback default.
func ResolveInferenceOllamaEndpoint(flag string, operators []models.OperatorDocumentGo, inferenceSessionID string) (string, error) {
	if endpoint := strings.TrimSpace(flag); endpoint != "" {
		return endpoint, nil
	}
	if endpoint := strings.TrimSpace(os.Getenv("G8E_OLLAMA_ENDPOINT")); endpoint != "" {
		return endpoint, nil
	}
	selected, err := SelectInferenceOperator(operators, inferenceSessionID)
	if err == nil {
		if endpoint := strings.TrimSpace(selected.OllamaEndpoint); endpoint != "" {
			return endpoint, nil
		}
	}
	return fmt.Sprintf("http://127.0.0.1:%d", constants.InferenceOllamaDefaultPort), nil
}
