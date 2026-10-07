// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package operatorcapability

import (
	"testing"

	operatorv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/operator/v1"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/models"
	"github.com/stretchr/testify/require"
)

func TestBlendedOperatorKeepsEveryCapability(t *testing.T) {
	cfg := &operatorv1.OperatorRuntimeConfig{Roles: models.OperatorRolesToProto(constants.OperatorRoles{constants.OperatorRoleData}), InferenceEnabled: true, ProvenanceOperatorEnabled: true, ProviderBoundaryObserverEnabled: true}
	op := &operatorv1.OperatorDocument{Id: "blend", OperatorSessionId: "session", OperatorType: string(constants.OperatorTypeRemote), Status: string(constants.OperatorStatusActive), RuntimeConfig: cfg}
	for _, role := range []constants.OperatorRole{constants.OperatorRoleData, constants.OperatorRoleInference, constants.OperatorRoleProvenance, constants.OperatorRoleObserver} {
		require.NoError(t, ValidateOperatorRoleCapabilities(op, role))
	}
	require.True(t, IsDataOperator(op))
	require.NoError(t, ValidateWitnessCommand(cfg, "pwd"))
}

func TestEveryOperatorRoleCombinationPreservesMembership(t *testing.T) {
	all := []constants.OperatorRole{constants.OperatorRoleEmbedded, constants.OperatorRoleData, constants.OperatorRoleInference, constants.OperatorRoleProvenance, constants.OperatorRoleObserver}
	for mask := 1; mask < 1<<len(all); mask++ {
		roles := constants.OperatorRoles{}
		for i, role := range all {
			if mask&(1<<i) != 0 {
				roles = append(roles, role)
			}
		}
		t.Run(roles.String(), func(t *testing.T) {
			cfg := &operatorv1.OperatorRuntimeConfig{Roles: models.OperatorRolesToProto(roles)}
			kind := constants.OperatorTypeRemote
			if roles.Has(constants.OperatorRoleEmbedded) {
				kind = constants.OperatorTypeEmbedded
			}
			op := &operatorv1.OperatorDocument{Id: "blend", OperatorSessionId: "session", OperatorType: string(kind), Status: string(constants.OperatorStatusActive), RuntimeConfig: cfg}
			effective := roles.Effective()
			require.Equal(t, effective, GetOperatorRoles(op))
			for _, role := range all {
				err := ValidateOperatorRoleCapabilities(op, role)
				if effective.Has(role) {
					require.NoError(t, err)
				} else {
					require.ErrorIs(t, err, constants.ErrWitnessCommandNotCapable)
				}
			}
			require.Equal(t, effective.Has(constants.OperatorRoleData), IsDataOperator(op))
			err := ValidateWitnessCommand(cfg, "pwd")
			if effective.Has(constants.OperatorRoleData) || effective.Has(constants.OperatorRoleInference) {
				require.NoError(t, err)
			} else {
				require.ErrorIs(t, err, constants.ErrWitnessCommandNotCapable)
			}
		})
	}
}
