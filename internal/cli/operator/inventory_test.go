// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package operator

import (
	"testing"

	operatorv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/operator/v1"

	"github.com/stretchr/testify/assert"

	"github.com/g8e-ai/g8e/v2/internal/constants"
)

func TestIsConnected(t *testing.T) {
	tests := []struct {
		name string
		op   *operatorv1.OperatorDocument
		want bool
	}{
		{"active", operatorv1.OperatorDocument{Status: string(constants.OperatorStatusActive)}, true},
		{"bound", operatorv1.OperatorDocument{Status: string(constants.OperatorStatusBound)}, true},
		{"stale", operatorv1.OperatorDocument{Status: string(constants.OperatorStatusStale)}, true},
		{"offline", operatorv1.OperatorDocument{Status: string(constants.OperatorStatusOffline)}, false},
		{"unclaimed slot", operatorv1.OperatorDocument{IsSlot: true, Status: constants.OperatorStatusActive}, false},
		{"claimed slot", operatorv1.OperatorDocument{IsSlot: true, Claimed: true, Status: constants.OperatorStatusActive}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, IsConnected(tt.op))
		})
	}
}
