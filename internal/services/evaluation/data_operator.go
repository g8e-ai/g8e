// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package evaluation

import (
	"fmt"

	"github.com/g8e-ai/g8e/v2/internal/models"
	"github.com/g8e-ai/g8e/v2/internal/services/operatorcapability"
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
	return activeGovernedDataOperators(operators)
}

// ActiveCampaignDataOperators returns active governed tool Operators that are
// not inference-capable, provider-boundary observers, or provenance witnesses.
func ActiveCampaignDataOperators(operators []models.OperatorDocumentGo) []DataOperatorStatus {
	return activeGovernedDataOperators(operators)
}

func activeGovernedDataOperators(operators []models.OperatorDocumentGo) []DataOperatorStatus {
	matches := make([]DataOperatorStatus, 0)
	for _, op := range operators {
		if !operatorcapability.IsGovernedDataOperator(op) {
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
//
// Callers that know the target hardware should use
// SelectCampaignDataOperatorForHardware so that an operator session cannot be
// selected merely because it has the right capability.
func SelectCampaignDataOperator(operators []models.OperatorDocumentGo, sessionID string) (*DataOperatorStatus, error) {
	return SelectCampaignDataOperatorForHardware(operators, sessionID, "")
}

// SelectCampaignDataOperatorForHardware resolves one campaign data Operator,
// optionally constrained to the canonical OperatorDocument system fingerprint.
// An explicit session ID remains the strongest binding, but it must still be
// an active campaign data operator on the requested hardware.
func SelectCampaignDataOperatorForHardware(
	operators []models.OperatorDocumentGo,
	sessionID string,
	systemFingerprint string,
) (*DataOperatorStatus, error) {
	matches := ActiveCampaignDataOperators(operators)
	if systemFingerprint != "" {
		fingerprintMatches := make([]DataOperatorStatus, 0, len(matches))
		for _, op := range operators {
			if op.SystemFingerprint != systemFingerprint {
				continue
			}
			for _, match := range matches {
				if match.OperatorSessionID == op.OperatorSessionID {
					fingerprintMatches = append(fingerprintMatches, match)
					break
				}
			}
		}
		matches = fingerprintMatches
	}
	if sessionID != "" {
		for _, match := range matches {
			if match.OperatorSessionID == sessionID {
				selected := match
				return &selected, nil
			}
		}
		if systemFingerprint != "" {
			return nil, fmt.Errorf("evaluation: campaign data operator session %q not found among active tool operators on system fingerprint %q", sessionID, systemFingerprint)
		}
		return nil, fmt.Errorf("evaluation: campaign data operator session %q not found among active tool operators", sessionID)
	}
	switch len(matches) {
	case 0:
		if systemFingerprint != "" {
			return nil, fmt.Errorf("evaluation: no active campaign data operator found on system fingerprint %q", systemFingerprint)
		}
		return nil, fmt.Errorf("evaluation: no active campaign data operator found")
	case 1:
		selected := matches[0]
		return &selected, nil
	default:
		if systemFingerprint != "" {
			return nil, fmt.Errorf("evaluation: multiple active campaign data operators found on system fingerprint %q; pin one with --data-session", systemFingerprint)
		}
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
