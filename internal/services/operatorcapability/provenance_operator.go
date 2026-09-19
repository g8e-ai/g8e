// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package operatorcapability

import (
	"fmt"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/models"
)

// ProvenanceOperatorStatus summarizes one active remote provenance operator
// session discovered through the operator registry.
type ProvenanceOperatorStatus struct {
	OperatorID        string
	OperatorSessionID string
	Status            string
	ProvenanceEnabled bool
	ModelStorageRoot  string
	Platform          string
}

// ActiveProvenanceOperators returns every active remote operator with
// runtime_config.provenance_operator_enabled set.
func ActiveProvenanceOperators(operators []models.OperatorDocumentGo) []ProvenanceOperatorStatus {
	matches := make([]ProvenanceOperatorStatus, 0)
	for _, op := range operators {
		if op.Status != constants.OperatorStatusActive || op.OperatorType != constants.OperatorTypeRemote {
			continue
		}
		if op.RuntimeConfig == nil || !op.RuntimeConfig.ProvenanceOperatorEnabled {
			continue
		}
		if op.OperatorSessionID == "" {
			continue
		}
		matches = append(matches, ProvenanceOperatorStatus{
			OperatorID:        op.ID,
			OperatorSessionID: op.OperatorSessionID,
			Status:            string(op.Status),
			ProvenanceEnabled: true,
			ModelStorageRoot:  op.RuntimeConfig.ProvenanceOperatorModelStorageRoot,
			Platform:          op.RuntimeConfig.Platform,
		})
	}
	return matches
}

// SelectProvenanceOperator resolves exactly one provenance operator. When
// sessionID is non-empty it must match an active provenance operator;
// otherwise exactly one active provenance operator must exist.
func SelectProvenanceOperator(operators []models.OperatorDocumentGo, sessionID string) (*ProvenanceOperatorStatus, error) {
	matches := ActiveProvenanceOperators(operators)
	if sessionID != "" {
		for _, match := range matches {
			if match.OperatorSessionID == sessionID {
				selected := match
				return &selected, nil
			}
		}
		return nil, fmt.Errorf("%w: session %s", constants.ErrProvenanceOperatorNotCapable, sessionID)
	}
	switch len(matches) {
	case 0:
		return nil, constants.ErrProvenanceOperatorNotFound
	case 1:
		selected := matches[0]
		return &selected, nil
	default:
		return nil, constants.ErrProvenanceOperatorAmbiguous
	}
}
