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

// ProvenanceOperatorStatus summarizes one active remote provenance operator.
type ProvenanceOperatorStatus = operatorcapability.ProvenanceOperatorStatus

// ActiveProvenanceOperators returns every active remote provenance operator.
func ActiveProvenanceOperators(operators []models.OperatorDocumentGo) []ProvenanceOperatorStatus {
	return operatorcapability.ActiveProvenanceOperators(operators)
}

// SelectProvenanceOperator resolves exactly one provenance operator.
func SelectProvenanceOperator(operators []models.OperatorDocumentGo, sessionID string) (*ProvenanceOperatorStatus, error) {
	return operatorcapability.SelectProvenanceOperator(operators, sessionID)
}
