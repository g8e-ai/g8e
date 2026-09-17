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

// DataOperatorStatus summarizes one active governed tool Operator session.
type DataOperatorStatus struct {
	OperatorID        string
	OperatorSessionID string
	Status            string
}

// ActiveDataOperators returns every active remote Operator that is not
// inference-capable and therefore serves the governed tool boundary.
func ActiveDataOperators(operators []models.OperatorDocumentGo) []DataOperatorStatus {
	matches := make([]DataOperatorStatus, 0)
	for _, op := range operators {
		if op.Status != constants.OperatorStatusActive || op.OperatorType != constants.OperatorTypeRemote {
			continue
		}
		if op.RuntimeConfig != nil && op.RuntimeConfig.InferenceEnabled {
			continue
		}
		if op.OperatorSessionID == "" {
			continue
		}
		matches = append(matches, DataOperatorStatus{
			OperatorID:        op.ID,
			OperatorSessionID: op.OperatorSessionID,
			Status:            string(op.Status),
		})
	}
	return matches
}

// ActiveCampaignDataOperators returns active governed tool Operators that are
// not inference-capable or provider-boundary observers.
func ActiveCampaignDataOperators(operators []models.OperatorDocumentGo) []DataOperatorStatus {
	matches := make([]DataOperatorStatus, 0)
	for _, op := range operators {
		if op.Status != constants.OperatorStatusActive || op.OperatorType != constants.OperatorTypeRemote {
			continue
		}
		if op.RuntimeConfig != nil && (op.RuntimeConfig.InferenceEnabled || op.RuntimeConfig.ProviderBoundaryObserverEnabled) {
			continue
		}
		if op.OperatorSessionID == "" {
			continue
		}
		matches = append(matches, DataOperatorStatus{
			OperatorID:        op.ID,
			OperatorSessionID: op.OperatorSessionID,
			Status:            string(op.Status),
		})
	}
	return matches
}

// SelectCampaignDataOperator resolves exactly one campaign data Operator.
func SelectCampaignDataOperator(operators []models.OperatorDocumentGo, sessionID string) (*DataOperatorStatus, error) {
	matches := ActiveCampaignDataOperators(operators)
	if sessionID != "" {
		for _, match := range matches {
			if match.OperatorSessionID == sessionID {
				selected := match
				return &selected, nil
			}
		}
		return nil, fmt.Errorf("evaluation: campaign data operator session %q not found among active tool operators", sessionID)
	}
	switch len(matches) {
	case 0:
		return nil, fmt.Errorf("evaluation: no active campaign data operator found")
	case 1:
		selected := matches[0]
		return &selected, nil
	default:
		return nil, fmt.Errorf("evaluation: multiple active campaign data operators found; pin one with --data-session")
	}
}

// SelectDataOperator resolves exactly one governed tool Operator.
func SelectDataOperator(operators []models.OperatorDocumentGo, sessionID string) (*DataOperatorStatus, error) {
	matches := ActiveDataOperators(operators)
	if sessionID != "" {
		for _, match := range matches {
			if match.OperatorSessionID == sessionID {
				selected := match
				return &selected, nil
			}
		}
		return nil, fmt.Errorf("evaluation: data operator session %q not found among active tool operators", sessionID)
	}
	switch len(matches) {
	case 0:
		return nil, fmt.Errorf("evaluation: no active data operator found")
	case 1:
		selected := matches[0]
		return &selected, nil
	default:
		return nil, fmt.Errorf("evaluation: multiple active data operators found; pin one with --data-operator-session")
	}
}
