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

func TestSelectInferenceOperator_RequiresExactSessionWhenPinned(t *testing.T) {
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
