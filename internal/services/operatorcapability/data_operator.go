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
