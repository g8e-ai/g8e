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

func TestActiveProviderBoundaryObserversFiltersAndProjectsOperators(t *testing.T) {
	t.Parallel()

	operators := []models.OperatorDocumentGo{
		{ID: "inactive", OperatorSessionID: "sess-inactive", Status: constants.OperatorStatusAvailable, OperatorType: constants.OperatorTypeRemote, RuntimeConfig: &models.RuntimeConfig{ProviderBoundaryObserverEnabled: true}},
		{ID: "local", OperatorSessionID: "sess-local", Status: constants.OperatorStatusActive, OperatorType: constants.OperatorTypeEmbedded, RuntimeConfig: &models.RuntimeConfig{ProviderBoundaryObserverEnabled: true}},
		{ID: "disabled", OperatorSessionID: "sess-disabled", Status: constants.OperatorStatusActive, OperatorType: constants.OperatorTypeRemote, RuntimeConfig: &models.RuntimeConfig{ProviderBoundaryObserverEnabled: false}},
		{ID: "missing-config", OperatorSessionID: "sess-missing-config", Status: constants.OperatorStatusActive, OperatorType: constants.OperatorTypeRemote},
		{ID: "missing-session", Status: constants.OperatorStatusActive, OperatorType: constants.OperatorTypeRemote, RuntimeConfig: &models.RuntimeConfig{ProviderBoundaryObserverEnabled: true}},
		{ID: "observer-1", OperatorSessionID: "sess-observer-1", Status: constants.OperatorStatusActive, OperatorType: constants.OperatorTypeRemote, RuntimeConfig: &models.RuntimeConfig{ProviderBoundaryObserverEnabled: true, Platform: "linux"}},
	}

	matches := ActiveProviderBoundaryObservers(operators)
	require.Len(t, matches, 1)
	assert.Equal(t, ProviderBoundaryObserverStatus{OperatorID: "observer-1", OperatorSessionID: "sess-observer-1", Status: string(constants.OperatorStatusActive), ObserverEnabled: true, Platform: "linux"}, matches[0])
}

func TestSelectProviderBoundaryObserverBySessionRejectsUnknownSession(t *testing.T) {
	t.Parallel()

	operators := []models.OperatorDocumentGo{{
		ID: "obs-1", OperatorSessionID: "sess-obs-1", Status: constants.OperatorStatusActive, OperatorType: constants.OperatorTypeRemote,
		RuntimeConfig: &models.RuntimeConfig{ProviderBoundaryObserverEnabled: true},
	}}
	selected, err := SelectProviderBoundaryObserver(operators, "sess-obs-1")
	require.NoError(t, err)
	assert.Equal(t, "obs-1", selected.OperatorID)

	_, err = SelectProviderBoundaryObserver(operators, "missing")
	assert.ErrorIs(t, err, constants.ErrProviderBoundaryObserverNotCapable)
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

func TestSelectProviderBoundaryObserverForHardware(t *testing.T) {
	t.Parallel()
	operators := []models.OperatorDocumentGo{
		{
			ID: "observer-other", OperatorSessionID: "sess-other",
			SystemFingerprint: "hardware-other",
			Status:            constants.OperatorStatusActive, OperatorType: constants.OperatorTypeRemote,
			RuntimeConfig: &models.RuntimeConfig{ProviderBoundaryObserverEnabled: true},
		},
		{
			ID: "observer-target", OperatorSessionID: "sess-target",
			SystemFingerprint: "hardware-target",
			Status:            constants.OperatorStatusActive, OperatorType: constants.OperatorTypeRemote,
			RuntimeConfig: &models.RuntimeConfig{ProviderBoundaryObserverEnabled: true},
		},
	}

	selected, err := SelectProviderBoundaryObserverForHardware(operators, "", "hardware-target")
	require.NoError(t, err)
	assert.Equal(t, "sess-target", selected.OperatorSessionID)
}
