package operatorcapability

import (
	"testing"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/models"
	"github.com/stretchr/testify/require"
)

func TestBlendedOperatorKeepsEveryCapability(t *testing.T) {
	cfg := &models.RuntimeConfig{Roles: constants.OperatorRoles{constants.OperatorRoleData}, InferenceEnabled: true, ProvenanceOperatorEnabled: true, ProviderBoundaryObserverEnabled: true}
	op := models.OperatorDocumentGo{ID: "blend", OperatorSessionID: "session", OperatorType: constants.OperatorTypeRemote, Status: constants.OperatorStatusActive, RuntimeConfig: cfg}
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
			cfg := &models.RuntimeConfig{Roles: roles}
			kind := constants.OperatorTypeRemote
			if roles.Has(constants.OperatorRoleEmbedded) {
				kind = constants.OperatorTypeEmbedded
			}
			op := models.OperatorDocumentGo{ID: "blend", OperatorSessionID: "session", OperatorType: kind, Status: constants.OperatorStatusActive, RuntimeConfig: cfg}
			require.Equal(t, roles, GetOperatorRoles(op))
			for _, role := range all {
				err := ValidateOperatorRoleCapabilities(op, role)
				if roles.Has(role) {
					require.NoError(t, err)
				} else {
					require.ErrorIs(t, err, constants.ErrWitnessCommandNotCapable)
				}
			}
			require.Equal(t, roles.Has(constants.OperatorRoleData), IsDataOperator(op))
			err := ValidateWitnessCommand(cfg, "pwd")
			if roles.Has(constants.OperatorRoleData) || roles.Has(constants.OperatorRoleInference) {
				require.NoError(t, err)
			} else {
				require.ErrorIs(t, err, constants.ErrWitnessCommandNotCapable)
			}
		})
	}
}
