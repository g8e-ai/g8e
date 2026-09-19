// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package evaluation

import (
	"github.com/g8e-ai/g8e/v2/internal/models"
	"github.com/g8e-ai/g8e/v2/internal/services/operatorcapability"
)

// ProviderBoundaryObserverStatus summarizes one active remote provider-boundary
// observer operator session discovered through the operator registry.
type ProviderBoundaryObserverStatus = operatorcapability.ProviderBoundaryObserverStatus

// ActiveProviderBoundaryObservers returns every active remote operator with
// runtime_config.provider_boundary_observer_enabled set.
func ActiveProviderBoundaryObservers(operators []models.OperatorDocumentGo) []ProviderBoundaryObserverStatus {
	return operatorcapability.ActiveProviderBoundaryObservers(operators)
}

// SelectProviderBoundaryObserver resolves exactly one provider-boundary
// observer operator. When sessionID is non-empty it must match an active
// observer; otherwise exactly one active observer must exist.
func SelectProviderBoundaryObserver(operators []models.OperatorDocumentGo, sessionID string) (*ProviderBoundaryObserverStatus, error) {
	return operatorcapability.SelectProviderBoundaryObserver(operators, sessionID)
}
