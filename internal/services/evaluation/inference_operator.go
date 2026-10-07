// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package evaluation

import (
	"fmt"
	"strings"

	operatorv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/operator/v1"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/services/operatorcapability"
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

// ActiveInferenceOperators returns every active remote or configured embedded
// Operator whose runtime roles include Inference.
func ActiveInferenceOperators(operators []*operatorv1.OperatorDocument) []InferenceOperatorStatus {
	matches := make([]InferenceOperatorStatus, 0)
	for _, op := range operators {
		if op.RuntimeConfig == nil || !operatorcapability.HasActiveRole(op, constants.OperatorRoleInference) {
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
func SelectInferenceOperator(operators []*operatorv1.OperatorDocument, sessionID string) (*InferenceOperatorStatus, error) {
	return SelectInferenceOperatorForHardware(operators, sessionID, "")
}

// SelectInferenceOperatorForHardware resolves an inference operator using the
// exact OperatorDocument system fingerprint when one is available.
func SelectInferenceOperatorForHardware(
	operators []*operatorv1.OperatorDocument,
	sessionID string,
	systemFingerprint string,
) (*InferenceOperatorStatus, error) {
	matches := ActiveInferenceOperators(operators)
	if systemFingerprint != "" {
		filtered := make([]InferenceOperatorStatus, 0, len(matches))
		for _, op := range operators {
			if op.SystemFingerprint != systemFingerprint {
				continue
			}
			for _, match := range matches {
				if match.OperatorSessionID == op.OperatorSessionID {
					filtered = append(filtered, match)
					break
				}
			}
		}
		matches = filtered
	}
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

func inferenceOperatorOllamaEndpoint(op *operatorv1.OperatorDocument) string {
	if op.RuntimeConfig == nil {
		return ""
	}
	return strings.TrimSpace(op.RuntimeConfig.InferenceOllamaEndpoint)
}

// GovernedInferenceOllamaEndpoint returns the approved provider endpoint from
// the exact Inference Operator runtime_config. Campaign-host environment
// overrides are intentionally rejected.
func GovernedInferenceOllamaEndpoint(operators []*operatorv1.OperatorDocument, inferenceSessionID string) (string, error) {
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
