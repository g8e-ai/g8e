// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package operator

import (
	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/models"
)

// IsConnected reports whether an Operator document from GET
// APIPaths.Operators represents a live connection: an unclaimed slot never
// does, and a claimed Operator does while it is active, bound, or stale.
func IsConnected(op models.OperatorDocumentGo) bool {
	if op.IsSlot && !op.Claimed {
		return false
	}
	switch op.Status {
	case constants.OperatorStatusActive, constants.OperatorStatusBound, constants.OperatorStatusStale:
		return true
	default:
		return false
	}
}
