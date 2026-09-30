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

// DataOperatorStatus summarizes the active data-operator session discovered
// through the operator registry.
type DataOperatorStatus struct {
	OperatorID        string
	OperatorSessionID string
	Status            string
}

// IsDataOperator reports whether op is an active remote session whose role is
// data. Inference, observer, and provenance Operators have their own roles.
func IsDataOperator(op models.OperatorDocumentGo) bool {
	if op.Status != constants.OperatorStatusActive || op.OperatorType != constants.OperatorTypeRemote {
		return false
	}
	if op.OperatorSessionID == "" {
		return false
	}
	return GetOperatorRole(op) == constants.OperatorRoleData
}

// IsStackDataOperator reports whether op is the data-operator the unified
// Docker stack launches: an active data Operator whose heartbeat hostname is
// constants.DataOperatorHostname.
func IsStackDataOperator(op models.OperatorDocumentGo) bool {
	return IsDataOperator(op) && op.CurrentHostname == constants.DataOperatorHostname
}

// ActiveDataOperators returns every active stack data-operator session. Other
// enrolled data Operators are not data-operators in this sense.
func ActiveDataOperators(operators []models.OperatorDocumentGo) []DataOperatorStatus {
	matches := make([]DataOperatorStatus, 0, 1)
	for _, op := range operators {
		if !IsStackDataOperator(op) {
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

// SelectDataOperator resolves exactly one data-operator. Zero sessions return
// ErrDataOperatorNotFound and several return ErrDataOperatorAmbiguous.
func SelectDataOperator(operators []models.OperatorDocumentGo) (*DataOperatorStatus, error) {
	matches := ActiveDataOperators(operators)
	switch len(matches) {
	case 0:
		return nil, constants.ErrDataOperatorNotFound
	case 1:
		selected := matches[0]
		return &selected, nil
	default:
		return nil, fmt.Errorf("%w: %d sessions", constants.ErrDataOperatorAmbiguous, len(matches))
	}
}
