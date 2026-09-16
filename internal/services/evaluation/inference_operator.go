// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package evaluation

import (
	"fmt"

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
