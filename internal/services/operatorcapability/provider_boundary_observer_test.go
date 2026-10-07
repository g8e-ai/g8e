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

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/v2/internal/constants"
)

func TestSelectProviderBoundaryObserver(t *testing.T) {
	operators := []*operatorv1.OperatorDocument{
		{Id: "obs-1", OperatorSessionId: "sess-obs-1", Status: string(constants.OperatorStatusActive), OperatorType: string(constants.OperatorTypeRemote), RuntimeConfig: &operatorv1.OperatorRuntimeConfig{ProviderBoundaryObserverEnabled: true}},
		{Id: "data-1", OperatorSessionId: "sess-data-1", Status: string(constants.OperatorStatusActive), OperatorType: string(constants.OperatorTypeRemote), RuntimeConfig: &operatorv1.OperatorRuntimeConfig{InferenceEnabled: true}},
	}
	selected, err := SelectProviderBoundaryObserver(operators, "")
	require.NoError(t, err)
	assert.Equal(t, "sess-obs-1", selected.OperatorSessionID)
}

func TestActiveProviderBoundaryObserversFiltersAndProjectsOperators(t *testing.T) {
	t.Parallel()

	operators := []*operatorv1.OperatorDocument{
		{Id: "inactive", OperatorSessionId: "sess-inactive", Status: string(constants.OperatorStatusAvailable), OperatorType: string(constants.OperatorTypeRemote), RuntimeConfig: &operatorv1.OperatorRuntimeConfig{ProviderBoundaryObserverEnabled: true}},
		{Id: "local", OperatorSessionId: "sess-local", Status: string(constants.OperatorStatusActive), OperatorType: string(constants.OperatorTypeEmbedded), RuntimeConfig: &operatorv1.OperatorRuntimeConfig{ProviderBoundaryObserverEnabled: true}},
		{Id: "disabled", OperatorSessionId: "sess-disabled", Status: string(constants.OperatorStatusActive), OperatorType: string(constants.OperatorTypeRemote), RuntimeConfig: &operatorv1.OperatorRuntimeConfig{ProviderBoundaryObserverEnabled: false}},
		{Id: "missing-config", OperatorSessionId: "sess-missing-config", Status: string(constants.OperatorStatusActive), OperatorType: string(constants.OperatorTypeRemote)},
		{Id: "missing-session", Status: string(constants.OperatorStatusActive), OperatorType: string(constants.OperatorTypeRemote), RuntimeConfig: &operatorv1.OperatorRuntimeConfig{ProviderBoundaryObserverEnabled: true}},
		{Id: "observer-1", OperatorSessionId: "sess-observer-1", Status: string(constants.OperatorStatusActive), OperatorType: string(constants.OperatorTypeRemote), RuntimeConfig: &operatorv1.OperatorRuntimeConfig{ProviderBoundaryObserverEnabled: true, Platform: "linux"}},
	}

	matches := ActiveProviderBoundaryObservers(operators)
	require.Len(t, matches, 2)
	assert.Equal(t, ProviderBoundaryObserverStatus{OperatorID: "observer-1", OperatorSessionID: "sess-observer-1", Status: string(constants.OperatorStatusActive), ObserverEnabled: true, Platform: "linux"}, matches[1])
}

func TestSelectProviderBoundaryObserverBySessionRejectsUnknownSession(t *testing.T) {
	t.Parallel()

	operators := []*operatorv1.OperatorDocument{{
		Id: "obs-1", OperatorSessionId: "sess-obs-1", Status: string(constants.OperatorStatusActive), OperatorType: string(constants.OperatorTypeRemote),
		RuntimeConfig: &operatorv1.OperatorRuntimeConfig{ProviderBoundaryObserverEnabled: true},
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
	operators := []*operatorv1.OperatorDocument{
		{Id: "obs-1", OperatorSessionId: "sess-obs-1", Status: string(constants.OperatorStatusActive), OperatorType: string(constants.OperatorTypeRemote), RuntimeConfig: &operatorv1.OperatorRuntimeConfig{ProviderBoundaryObserverEnabled: true}},
		{Id: "obs-2", OperatorSessionId: "sess-obs-2", Status: string(constants.OperatorStatusActive), OperatorType: string(constants.OperatorTypeRemote), RuntimeConfig: &operatorv1.OperatorRuntimeConfig{ProviderBoundaryObserverEnabled: true}},
	}
	_, err := SelectProviderBoundaryObserver(operators, "")
	assert.ErrorIs(t, err, constants.ErrProviderBoundaryObserverAmbiguous)
}

func TestSelectProviderBoundaryObserverForHardware(t *testing.T) {
	t.Parallel()
	operators := []*operatorv1.OperatorDocument{
		{
			Id: "observer-other", OperatorSessionId: "sess-other",
			SystemFingerprint: "hardware-other",
			Status:            string(constants.OperatorStatusActive), OperatorType: string(constants.OperatorTypeRemote),
			RuntimeConfig: &operatorv1.OperatorRuntimeConfig{ProviderBoundaryObserverEnabled: true},
		},
		{
			Id: "observer-target", OperatorSessionId: "sess-target",
			SystemFingerprint: "hardware-target",
			Status:            string(constants.OperatorStatusActive), OperatorType: string(constants.OperatorTypeRemote),
			RuntimeConfig: &operatorv1.OperatorRuntimeConfig{ProviderBoundaryObserverEnabled: true},
		},
	}

	selected, err := SelectProviderBoundaryObserverForHardware(operators, "", "hardware-target")
	require.NoError(t, err)
	assert.Equal(t, "sess-target", selected.OperatorSessionID)
}
