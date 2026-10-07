// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package operator

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/models"
)

func TestIsConnected(t *testing.T) {
	tests := []struct {
		name string
		op   models.OperatorDocumentGo
		want bool
	}{
		{"active", models.OperatorDocumentGo{Status: constants.OperatorStatusActive}, true},
		{"bound", models.OperatorDocumentGo{Status: constants.OperatorStatusBound}, true},
		{"stale", models.OperatorDocumentGo{Status: constants.OperatorStatusStale}, true},
		{"offline", models.OperatorDocumentGo{Status: constants.OperatorStatusOffline}, false},
		{"unclaimed slot", models.OperatorDocumentGo{IsSlot: true, Status: constants.OperatorStatusActive}, false},
		{"claimed slot", models.OperatorDocumentGo{IsSlot: true, Claimed: true, Status: constants.OperatorStatusActive}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, IsConnected(tt.op))
		})
	}
}
