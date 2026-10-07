// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package evaluation

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/models"
)

func TestSelectInferenceOperator_RequiresInferenceCapableSession(t *testing.T) {
	t.Parallel()
	operators := []models.OperatorDocumentGo{
		{ID: "inf-1", OperatorSessionID: "sess-inf-1", Status: constants.OperatorStatusActive, OperatorType: constants.OperatorTypeRemote, RuntimeConfig: &models.RuntimeConfig{InferenceEnabled: true}},
		{ID: "data-1", OperatorSessionID: "sess-data-1", Status: constants.OperatorStatusActive, OperatorType: constants.OperatorTypeRemote, RuntimeConfig: &models.RuntimeConfig{InferenceEnabled: false}},
	}
	selected, err := SelectInferenceOperator(operators, "sess-inf-1")
	require.NoError(t, err)
	assert.Equal(t, "sess-inf-1", selected.OperatorSessionID)

	_, err = SelectInferenceOperator(operators, "sess-data-1")
	require.ErrorIs(t, err, constants.ErrInferenceOperatorNotCapable)
}

func TestGovernedInferenceOllamaEndpoint_UsesExactOperatorRuntimeConfig(t *testing.T) {
	t.Parallel()
	operators := []models.OperatorDocumentGo{
		{
			ID:                "inf-1",
			OperatorSessionID: "sess-inf-1",
			Status:            constants.OperatorStatusActive,
			OperatorType:      constants.OperatorTypeRemote,
			RuntimeConfig: &models.RuntimeConfig{
				InferenceEnabled:        true,
				InferenceOllamaEndpoint: "http://192.168.1.2:11434",
			},
		},
	}
	endpoint, err := GovernedInferenceOllamaEndpoint(operators, "sess-inf-1")
	require.NoError(t, err)
	assert.Equal(t, "http://192.168.1.2:11434", endpoint)
}

func TestGovernedInferenceOllamaEndpoint_RejectsMissingOperatorEndpoint(t *testing.T) {
	t.Parallel()
	operators := []models.OperatorDocumentGo{
		{
			ID:                "inf-1",
			OperatorSessionID: "sess-inf-1",
			Status:            constants.OperatorStatusActive,
			OperatorType:      constants.OperatorTypeRemote,
			RuntimeConfig:     &models.RuntimeConfig{InferenceEnabled: true},
		},
	}
	_, err := GovernedInferenceOllamaEndpoint(operators, "sess-inf-1")
	require.ErrorIs(t, err, constants.ErrInferenceEndpointInvalid)
}

func TestSelectInferenceOperator_FailsClosedOnAmbiguity(t *testing.T) {
	t.Parallel()
	operators := []models.OperatorDocumentGo{
		{ID: "inf-1", OperatorSessionID: "sess-inf-1", Status: constants.OperatorStatusActive, OperatorType: constants.OperatorTypeRemote, RuntimeConfig: &models.RuntimeConfig{InferenceEnabled: true}},
		{ID: "inf-2", OperatorSessionID: "sess-inf-2", Status: constants.OperatorStatusActive, OperatorType: constants.OperatorTypeRemote, RuntimeConfig: &models.RuntimeConfig{InferenceEnabled: true}},
	}
	_, err := SelectInferenceOperator(operators, "")
	require.ErrorIs(t, err, constants.ErrInferenceOperatorAmbiguous)
}

func TestSelectInferenceOperatorForHardware(t *testing.T) {
	t.Parallel()
	operators := []models.OperatorDocumentGo{
		{
			ID:                "inf-host1",
			OperatorSessionID: "sess-inf-host1",
			Status:            constants.OperatorStatusActive,
			OperatorType:      constants.OperatorTypeRemote,
			SystemFingerprint: "fp-host1",
			RuntimeConfig:     &models.RuntimeConfig{InferenceEnabled: true},
		},
		{
			ID:                "inf-host2",
			OperatorSessionID: "sess-inf-host2",
			Status:            constants.OperatorStatusActive,
			OperatorType:      constants.OperatorTypeRemote,
			SystemFingerprint: "fp-host2",
			RuntimeConfig:     &models.RuntimeConfig{InferenceEnabled: true},
		},
	}

	t.Run("resolves by system fingerprint", func(t *testing.T) {
		selected, err := SelectInferenceOperatorForHardware(operators, "", "fp-host2")
		require.NoError(t, err)
		assert.Equal(t, "inf-host2", selected.OperatorID)
	})

	t.Run("returns not found for non-matching fingerprint", func(t *testing.T) {
		_, err := SelectInferenceOperatorForHardware(operators, "", "fp-nonexistent")
		require.ErrorIs(t, err, constants.ErrInferenceOperatorNotFound)
	})
}

func TestSelectInferenceOperator_BlendedDeploymentTypes(t *testing.T) {
	for _, kind := range []constants.OperatorType{constants.OperatorTypeRemote, constants.OperatorTypeEmbedded} {
		t.Run(string(kind), func(t *testing.T) {
			op := models.OperatorDocumentGo{
				ID: "blend", OperatorSessionID: "blend-session", Status: constants.OperatorStatusActive,
				OperatorType: kind, SystemFingerprint: "blend-hardware",
				RuntimeConfig: &models.RuntimeConfig{
					Roles:                   constants.OperatorRoles{constants.OperatorRoleData, constants.OperatorRoleInference, constants.OperatorRoleProvenance, constants.OperatorRoleObserver},
					InferenceOllamaEndpoint: "http://127.0.0.1:11434",
				},
			}
			operators := []models.OperatorDocumentGo{op}
			selected, err := SelectInferenceOperatorForHardware(operators, op.OperatorSessionID, op.SystemFingerprint)
			require.NoError(t, err)
			assert.Equal(t, op.OperatorSessionID, selected.OperatorSessionID)
			endpoint, err := GovernedInferenceOllamaEndpoint(operators, op.OperatorSessionID)
			require.NoError(t, err)
			assert.Equal(t, op.RuntimeConfig.InferenceOllamaEndpoint, endpoint)
			_, err = SelectInferenceOperatorForHardware(operators, op.OperatorSessionID, "other-hardware")
			require.ErrorIs(t, err, constants.ErrInferenceOperatorNotCapable)
			_, err = SelectInferenceOperator(append(operators, models.OperatorDocumentGo{
				ID: "other", OperatorSessionID: "other-session", Status: constants.OperatorStatusActive,
				OperatorType: constants.OperatorTypeRemote, RuntimeConfig: &models.RuntimeConfig{InferenceEnabled: true},
			}), "")
			require.ErrorIs(t, err, constants.ErrInferenceOperatorAmbiguous)
		})
	}
}

func TestActiveInferenceOperators_RejectsUnavailableOrIncapableSessions(t *testing.T) {
	capable := &models.RuntimeConfig{Roles: constants.OperatorRoles{constants.OperatorRoleInference}}
	operators := []models.OperatorDocumentGo{
		{ID: "offline", OperatorSessionID: "offline", Status: constants.OperatorStatusOffline, OperatorType: constants.OperatorTypeRemote, RuntimeConfig: capable},
		{ID: "unknown-type", OperatorSessionID: "unknown", Status: constants.OperatorStatusActive, OperatorType: constants.OperatorType("unknown"), RuntimeConfig: capable},
		{ID: "missing-session", Status: constants.OperatorStatusActive, OperatorType: constants.OperatorTypeRemote, RuntimeConfig: capable},
		{ID: "missing-config", OperatorSessionID: "missing-config", Status: constants.OperatorStatusActive, OperatorType: constants.OperatorTypeEmbedded, OperatorRoles: constants.OperatorRoles{constants.OperatorRoleInference}},
		{ID: "data", OperatorSessionID: "data", Status: constants.OperatorStatusActive, OperatorType: constants.OperatorTypeRemote, RuntimeConfig: &models.RuntimeConfig{Roles: constants.OperatorRoles{constants.OperatorRoleData}}},
	}
	assert.Empty(t, ActiveInferenceOperators(operators))
}
