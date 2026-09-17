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

// ProviderBoundaryObserverStatus summarizes one active remote provider-boundary
// observer operator session discovered through the operator registry.
type ProviderBoundaryObserverStatus struct {
	OperatorID        string
	OperatorSessionID string
	Status            string
	ObserverEnabled   bool
}

// ActiveProviderBoundaryObservers returns every active remote operator with
// runtime_config.provider_boundary_observer_enabled set.
func ActiveProviderBoundaryObservers(operators []models.OperatorDocumentGo) []ProviderBoundaryObserverStatus {
	matches := make([]ProviderBoundaryObserverStatus, 0)
	for _, op := range operators {
		if op.Status != constants.OperatorStatusActive || op.OperatorType != constants.OperatorTypeRemote {
			continue
		}
		if op.RuntimeConfig == nil || !op.RuntimeConfig.ProviderBoundaryObserverEnabled {
			continue
		}
		if op.OperatorSessionID == "" {
			continue
		}
		matches = append(matches, ProviderBoundaryObserverStatus{
			OperatorID:        op.ID,
			OperatorSessionID: op.OperatorSessionID,
			Status:            string(op.Status),
			ObserverEnabled:   true,
		})
	}
	return matches
}

// SelectProviderBoundaryObserver resolves exactly one provider-boundary
// observer operator. When sessionID is non-empty it must match an active
// observer; otherwise exactly one active observer must exist.
func SelectProviderBoundaryObserver(operators []models.OperatorDocumentGo, sessionID string) (*ProviderBoundaryObserverStatus, error) {
	matches := ActiveProviderBoundaryObservers(operators)
	if sessionID != "" {
		for _, match := range matches {
			if match.OperatorSessionID == sessionID {
				selected := match
				return &selected, nil
			}
		}
		return nil, fmt.Errorf("%w: session %s", constants.ErrProviderBoundaryObserverNotCapable, sessionID)
	}
	switch len(matches) {
	case 0:
		return nil, constants.ErrProviderBoundaryObserverNotFound
	case 1:
		selected := matches[0]
		return &selected, nil
	default:
		return nil, constants.ErrProviderBoundaryObserverAmbiguous
	}
}
