// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package operatorcapability

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/models"
)

func TestSelectProviderBoundaryObserver(t *testing.T) {
	operators := []models.OperatorDocumentGo{
		{ID: "obs-1", OperatorSessionID: "sess-obs-1", Status: constants.OperatorStatusActive, OperatorType: constants.OperatorTypeRemote, RuntimeConfig: &models.RuntimeConfig{ProviderBoundaryObserverEnabled: true}},
		{ID: "data-1", OperatorSessionID: "sess-data-1", Status: constants.OperatorStatusActive, OperatorType: constants.OperatorTypeRemote, RuntimeConfig: &models.RuntimeConfig{InferenceEnabled: true}},
	}
	selected, err := SelectProviderBoundaryObserver(operators, "")
	require.NoError(t, err)
	assert.Equal(t, "sess-obs-1", selected.OperatorSessionID)
}

func TestSelectProviderBoundaryObserver_NotFound(t *testing.T) {
	_, err := SelectProviderBoundaryObserver(nil, "")
	assert.ErrorIs(t, err, constants.ErrProviderBoundaryObserverNotFound)
}

func TestSelectProviderBoundaryObserver_Ambiguous(t *testing.T) {
	operators := []models.OperatorDocumentGo{
		{ID: "obs-1", OperatorSessionID: "sess-obs-1", Status: constants.OperatorStatusActive, OperatorType: constants.OperatorTypeRemote, RuntimeConfig: &models.RuntimeConfig{ProviderBoundaryObserverEnabled: true}},
		{ID: "obs-2", OperatorSessionID: "sess-obs-2", Status: constants.OperatorStatusActive, OperatorType: constants.OperatorTypeRemote, RuntimeConfig: &models.RuntimeConfig{ProviderBoundaryObserverEnabled: true}},
	}
	_, err := SelectProviderBoundaryObserver(operators, "")
	assert.ErrorIs(t, err, constants.ErrProviderBoundaryObserverAmbiguous)
}
