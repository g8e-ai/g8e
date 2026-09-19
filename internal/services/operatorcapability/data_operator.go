// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package operatorcapability

import (
	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/models"
)

// IsGovernedDataOperator reports whether op is an active remote session that
// serves the governed tool/data boundary. Inference, provider-boundary
// observer, and provenance witness sessions are excluded.
func IsGovernedDataOperator(op models.OperatorDocumentGo) bool {
	if op.Status != constants.OperatorStatusActive || op.OperatorType != constants.OperatorTypeRemote {
		return false
	}
	if op.OperatorSessionID == "" {
		return false
	}
	if op.RuntimeConfig != nil && (op.RuntimeConfig.InferenceEnabled ||
		op.RuntimeConfig.ProviderBoundaryObserverEnabled ||
		op.RuntimeConfig.ProvenanceOperatorEnabled) {
		return false
	}
	return true
}
