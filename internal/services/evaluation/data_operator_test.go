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

func TestActiveCampaignDataOperators_ExcludesSpecializedOperators(t *testing.T) {
	t.Parallel()
	operators := []models.OperatorDocumentGo{
		{
			ID:                "data-1",
			OperatorSessionID: "sess-data-1",
			Status:            constants.OperatorStatusActive,
			OperatorType:      constants.OperatorTypeRemote,
		},
		{
			ID:                "infer-1",
			OperatorSessionID: "sess-infer-1",
			Status:            constants.OperatorStatusActive,
			OperatorType:      constants.OperatorTypeRemote,
			RuntimeConfig:     &models.RuntimeConfig{InferenceEnabled: true},
		},
		{
			ID:                "observer-1",
			OperatorSessionID: "sess-observer-1",
			Status:            constants.OperatorStatusActive,
			OperatorType:      constants.OperatorTypeRemote,
			RuntimeConfig:     &models.RuntimeConfig{ProviderBoundaryObserverEnabled: true},
		},
		{
			ID:                "prov-1",
			OperatorSessionID: "sess-prov-1",
			Status:            constants.OperatorStatusActive,
			OperatorType:      constants.OperatorTypeRemote,
			RuntimeConfig:     &models.RuntimeConfig{ProvenanceOperatorEnabled: true},
		},
	}

	matches := ActiveCampaignDataOperators(operators)
	require.Len(t, matches, 1)
	assert.Equal(t, "sess-data-1", matches[0].OperatorSessionID)
}

func TestActiveDataOperators_ExcludesSpecializedOperators(t *testing.T) {
	t.Parallel()
	operators := []models.OperatorDocumentGo{
		{
			ID:                "prov-1",
			OperatorSessionID: "sess-prov-1",
			Status:            constants.OperatorStatusActive,
			OperatorType:      constants.OperatorTypeRemote,
			RuntimeConfig:     &models.RuntimeConfig{ProvenanceOperatorEnabled: true},
		},
		{
			ID:                "data-1",
			OperatorSessionID: "sess-data-1",
			Status:            constants.OperatorStatusActive,
			OperatorType:      constants.OperatorTypeRemote,
		},
	}

	matches := ActiveDataOperators(operators)
	require.Len(t, matches, 1)
	assert.Equal(t, "sess-data-1", matches[0].OperatorSessionID)
}

func TestSelectCampaignDataOperator_ResolvesBySessionID(t *testing.T) {
	t.Parallel()
	operators := []models.OperatorDocumentGo{
		{
			ID:                "data-1",
			OperatorSessionID: "sess-data-1",
			Status:            constants.OperatorStatusActive,
			OperatorType:      constants.OperatorTypeRemote,
		},
		{
			ID:                "data-2",
			OperatorSessionID: "sess-data-2",
			Status:            constants.OperatorStatusActive,
			OperatorType:      constants.OperatorTypeRemote,
		},
	}
	selected, err := SelectCampaignDataOperator(operators, "sess-data-2")
	require.NoError(t, err)
	assert.Equal(t, "sess-data-2", selected.OperatorSessionID)
}
