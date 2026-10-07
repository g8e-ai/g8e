// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package evaluation

import (
	"testing"

	operatorv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/operator/v1"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/models"
)

func TestSelectInferenceOperator_RequiresInferenceCapableSession(t *testing.T) {
	t.Parallel()
	operators := []*operatorv1.OperatorDocument{
		{Id: "inf-1", OperatorSessionId: "sess-inf-1", Status: string(constants.OperatorStatusActive), OperatorType: string(constants.OperatorTypeRemote), RuntimeConfig: &operatorv1.OperatorRuntimeConfig{InferenceEnabled: true}},
		{Id: "data-1", OperatorSessionId: "sess-data-1", Status: string(constants.OperatorStatusActive), OperatorType: string(constants.OperatorTypeRemote), RuntimeConfig: &operatorv1.OperatorRuntimeConfig{InferenceEnabled: false}},
	}
	selected, err := SelectInferenceOperator(operators, "sess-inf-1")
	require.NoError(t, err)
	assert.Equal(t, "sess-inf-1", selected.OperatorSessionID)

	_, err = SelectInferenceOperator(operators, "sess-data-1")
	require.ErrorIs(t, err, constants.ErrInferenceOperatorNotCapable)
}

func TestGovernedInferenceOllamaEndpoint_UsesExactOperatorRuntimeConfig(t *testing.T) {
	t.Parallel()
	operators := []*operatorv1.OperatorDocument{
		{
			Id:                "inf-1",
			OperatorSessionId: "sess-inf-1",
			Status:            string(constants.OperatorStatusActive),
			OperatorType:      string(constants.OperatorTypeRemote),
			RuntimeConfig: &operatorv1.OperatorRuntimeConfig{
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
	operators := []*operatorv1.OperatorDocument{
		{
			Id:                "inf-1",
			OperatorSessionId: "sess-inf-1",
			Status:            string(constants.OperatorStatusActive),
			OperatorType:      string(constants.OperatorTypeRemote),
			RuntimeConfig:     &operatorv1.OperatorRuntimeConfig{InferenceEnabled: true},
		},
	}
	_, err := GovernedInferenceOllamaEndpoint(operators, "sess-inf-1")
	require.ErrorIs(t, err, constants.ErrInferenceEndpointInvalid)
}

func TestSelectInferenceOperator_FailsClosedOnAmbiguity(t *testing.T) {
	t.Parallel()
	operators := []*operatorv1.OperatorDocument{
		{Id: "inf-1", OperatorSessionId: "sess-inf-1", Status: string(constants.OperatorStatusActive), OperatorType: string(constants.OperatorTypeRemote), RuntimeConfig: &operatorv1.OperatorRuntimeConfig{InferenceEnabled: true}},
		{Id: "inf-2", OperatorSessionId: "sess-inf-2", Status: string(constants.OperatorStatusActive), OperatorType: string(constants.OperatorTypeRemote), RuntimeConfig: &operatorv1.OperatorRuntimeConfig{InferenceEnabled: true}},
	}
	_, err := SelectInferenceOperator(operators, "")
	require.ErrorIs(t, err, constants.ErrInferenceOperatorAmbiguous)
}

func TestSelectInferenceOperatorForHardware(t *testing.T) {
	t.Parallel()
	operators := []*operatorv1.OperatorDocument{
		{
			Id:                "inf-host1",
			OperatorSessionId: "sess-inf-host1",
			Status:            string(constants.OperatorStatusActive),
			OperatorType:      string(constants.OperatorTypeRemote),
			SystemFingerprint: "fp-host1",
			RuntimeConfig:     &operatorv1.OperatorRuntimeConfig{InferenceEnabled: true},
		},
		{
			Id:                "inf-host2",
			OperatorSessionId: "sess-inf-host2",
			Status:            string(constants.OperatorStatusActive),
			OperatorType:      string(constants.OperatorTypeRemote),
			SystemFingerprint: "fp-host2",
			RuntimeConfig:     &operatorv1.OperatorRuntimeConfig{InferenceEnabled: true},
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
			op := &operatorv1.OperatorDocument{
				Id: "blend", OperatorSessionId: "blend-session", Status: string(constants.OperatorStatusActive),
				OperatorType: string(kind), SystemFingerprint: "blend-hardware",
				RuntimeConfig: &operatorv1.OperatorRuntimeConfig{
					Roles:                   models.OperatorRolesToProto(constants.OperatorRoles{constants.OperatorRoleData, constants.OperatorRoleInference, constants.OperatorRoleProvenance, constants.OperatorRoleObserver}),
					InferenceOllamaEndpoint: "http://127.0.0.1:11434",
				},
			}
			operators := []*operatorv1.OperatorDocument{op}
			selected, err := SelectInferenceOperatorForHardware(operators, op.OperatorSessionId, op.SystemFingerprint)
			require.NoError(t, err)
			assert.Equal(t, op.OperatorSessionId, selected.OperatorSessionID)
			endpoint, err := GovernedInferenceOllamaEndpoint(operators, op.OperatorSessionId)
			require.NoError(t, err)
			assert.Equal(t, op.RuntimeConfig.InferenceOllamaEndpoint, endpoint)
			_, err = SelectInferenceOperatorForHardware(operators, op.OperatorSessionId, "other-hardware")
			require.ErrorIs(t, err, constants.ErrInferenceOperatorNotCapable)
			_, err = SelectInferenceOperator(append(operators, &operatorv1.OperatorDocument{
				Id: "other", OperatorSessionId: "other-session", Status: string(constants.OperatorStatusActive),
				OperatorType: string(constants.OperatorTypeRemote), RuntimeConfig: &operatorv1.OperatorRuntimeConfig{InferenceEnabled: true},
			}), "")
			require.ErrorIs(t, err, constants.ErrInferenceOperatorAmbiguous)
		})
	}
}

func TestActiveInferenceOperators_RejectsUnavailableOrIncapableSessions(t *testing.T) {
	capable := &operatorv1.OperatorRuntimeConfig{Roles: models.OperatorRolesToProto(constants.OperatorRoles{constants.OperatorRoleInference})}
	operators := []*operatorv1.OperatorDocument{
		{Id: "offline", OperatorSessionId: "offline", Status: string(constants.OperatorStatusOffline), OperatorType: string(constants.OperatorTypeRemote), RuntimeConfig: capable},
		{Id: "unknown-type", OperatorSessionId: "unknown", Status: string(constants.OperatorStatusActive), OperatorType: "unknown", RuntimeConfig: capable},
		{Id: "missing-session", Status: string(constants.OperatorStatusActive), OperatorType: string(constants.OperatorTypeRemote), RuntimeConfig: capable},
		{Id: "missing-config", OperatorSessionId: "missing-config", Status: string(constants.OperatorStatusActive), OperatorType: string(constants.OperatorTypeEmbedded), OperatorRoles: models.OperatorRolesToProto(constants.OperatorRoles{constants.OperatorRoleInference})},
		{Id: "data", OperatorSessionId: "data", Status: string(constants.OperatorStatusActive), OperatorType: string(constants.OperatorTypeRemote), RuntimeConfig: &operatorv1.OperatorRuntimeConfig{Roles: models.OperatorRolesToProto(constants.OperatorRoles{constants.OperatorRoleData})}},
	}
	assert.Empty(t, ActiveInferenceOperators(operators))
}
