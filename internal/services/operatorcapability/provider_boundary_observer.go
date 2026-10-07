// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package operatorcapability

import (
	"fmt"

	operatorv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/operator/v1"

	"github.com/g8e-ai/g8e/v2/internal/constants"
)

// ProviderBoundaryObserverStatus summarizes one active remote provider-boundary
// observer operator session discovered through the operator registry.
type ProviderBoundaryObserverStatus struct {
	OperatorID        string
	OperatorSessionID string
	Status            string
	ObserverEnabled   bool
	Platform          string
}

// ActiveProviderBoundaryObservers returns every active remote operator with
// runtime_config.provider_boundary_observer_enabled set.
func ActiveProviderBoundaryObservers(operators []*operatorv1.OperatorDocument) []ProviderBoundaryObserverStatus {
	matches := make([]ProviderBoundaryObserverStatus, 0)
	for _, op := range operators {
		if op.RuntimeConfig == nil || !HasActiveRole(op, constants.OperatorRoleObserver) {
			continue
		}
		matches = append(matches, ProviderBoundaryObserverStatus{
			OperatorID:        op.Id,
			OperatorSessionID: op.OperatorSessionId,
			Status:            op.Status,
			ObserverEnabled:   true,
			Platform:          op.RuntimeConfig.Platform,
		})
	}
	return matches
}

// SelectProviderBoundaryObserver resolves exactly one provider-boundary
// observer operator. When sessionID is non-empty it must match an active
// observer; otherwise exactly one active observer must exist.
func SelectProviderBoundaryObserver(operators []*operatorv1.OperatorDocument, sessionID string) (*ProviderBoundaryObserverStatus, error) {
	return SelectProviderBoundaryObserverForHardware(operators, sessionID, "")
}

// SelectProviderBoundaryObserverForHardware resolves an observer using the
// exact OperatorDocument system fingerprint when one is available.
func SelectProviderBoundaryObserverForHardware(
	operators []*operatorv1.OperatorDocument,
	sessionID string,
	systemFingerprint string,
) (*ProviderBoundaryObserverStatus, error) {
	matches := ActiveProviderBoundaryObservers(operators)
	if systemFingerprint != "" {
		filtered := make([]ProviderBoundaryObserverStatus, 0, len(matches))
		for _, op := range operators {
			if op.SystemFingerprint != systemFingerprint {
				continue
			}
			for _, match := range matches {
				if match.OperatorSessionID == op.OperatorSessionId {
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
