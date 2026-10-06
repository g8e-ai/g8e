package operatorcapability

import (
 "testing"
 "github.com/stretchr/testify/require"
 "github.com/g8e-ai/g8e/v2/internal/constants"
 "github.com/g8e-ai/g8e/v2/internal/models"
)

func TestBlendedOperatorKeepsEveryCapability(t *testing.T) {
 cfg := &models.RuntimeConfig{Role: constants.OperatorRoleData, InferenceEnabled: true, ProvenanceOperatorEnabled: true, ProviderBoundaryObserverEnabled: true}
 op := models.OperatorDocumentGo{ID: "blend", OperatorSessionID: "session", OperatorType: constants.OperatorTypeRemote, Status: constants.OperatorStatusActive, RuntimeConfig: cfg}
 for _, role := range []constants.OperatorRoles{constants.OperatorRoleData, constants.OperatorRoleInference, constants.OperatorRoleProvenance, constants.OperatorRoleObserver} {
  require.NoError(t, ValidateOperatorRoleCapabilities(op, role))
 }
 require.True(t, IsDataOperator(op))
 require.NoError(t, ValidateWitnessCommand(cfg, "pwd"))
}
