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

func TestIsDataOperator_ExcludesOtherRoles(t *testing.T) {
	t.Parallel()

	data := models.OperatorDocumentGo{
		ID:                "data-1",
		OperatorSessionID: "sess-data-1",
		Status:            constants.OperatorStatusActive,
		OperatorType:      constants.OperatorTypeRemote,
	}
	assert.True(t, IsDataOperator(data))

	cases := []models.OperatorDocumentGo{
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
		{
			ID:                "role-1",
			OperatorSessionID: "sess-role-1",
			Status:            constants.OperatorStatusActive,
			OperatorType:      constants.OperatorTypeRemote,
			OperatorRole:      constants.OperatorRoleObserver,
		},
		{
			ID:                "offline-1",
			OperatorSessionID: "sess-offline-1",
			Status:            constants.OperatorStatusOffline,
			OperatorType:      constants.OperatorTypeRemote,
		},
		{
			ID:                "embedded-1",
			OperatorSessionID: "sess-embedded-1",
			Status:            constants.OperatorStatusActive,
			OperatorType:      constants.OperatorTypeEmbedded,
		},
	}
	for _, op := range cases {
		assert.False(t, IsDataOperator(op), op.ID)
	}
}

func stackDataOperatorDoc(id, sessionID, hostname string) models.OperatorDocumentGo {
	return models.OperatorDocumentGo{
		ID:                id,
		OperatorSessionID: sessionID,
		Status:            constants.OperatorStatusActive,
		OperatorType:      constants.OperatorTypeRemote,
		CurrentHostname:   hostname,
	}
}

func TestSelectDataOperator_ConsidersOnlyTheStackDataOperator(t *testing.T) {
	t.Parallel()
	operators := []models.OperatorDocumentGo{
		stackDataOperatorDoc("load-1", "sess-load-1", "load-host-1"),
		stackDataOperatorDoc("stack", "sess-stack", constants.DataOperatorHostname),
		stackDataOperatorDoc("load-2", "sess-load-2", "load-host-2"),
	}

	selected, err := SelectDataOperator(operators)
	require.NoError(t, err)
	assert.Equal(t, "stack", selected.OperatorID)
	assert.Equal(t, "sess-stack", selected.OperatorSessionID)
}

func TestSelectDataOperator_IgnoresOtherRolesOnTheStackHostname(t *testing.T) {
	t.Parallel()
	inference := stackDataOperatorDoc("infer", "sess-infer", constants.DataOperatorHostname)
	inference.RuntimeConfig = &models.RuntimeConfig{InferenceEnabled: true}

	_, err := SelectDataOperator([]models.OperatorDocumentGo{inference})
	require.ErrorIs(t, err, constants.ErrDataOperatorNotFound)
}

func TestSelectDataOperator_NotFoundWithoutStackDataOperator(t *testing.T) {
	t.Parallel()
	_, err := SelectDataOperator([]models.OperatorDocumentGo{stackDataOperatorDoc("load-1", "sess-load-1", "load-host-1")})
	require.ErrorIs(t, err, constants.ErrDataOperatorNotFound)

	_, err = SelectDataOperator(nil)
	require.ErrorIs(t, err, constants.ErrDataOperatorNotFound)
}

func TestSelectDataOperator_AmbiguousWhenTwoStackDataOperatorsAreActive(t *testing.T) {
	t.Parallel()
	_, err := SelectDataOperator([]models.OperatorDocumentGo{
		stackDataOperatorDoc("a", "sess-a", constants.DataOperatorHostname),
		stackDataOperatorDoc("b", "sess-b", constants.DataOperatorHostname),
	})
	require.ErrorIs(t, err, constants.ErrDataOperatorAmbiguous)
}

func TestIsStackDataOperator_RequiresDataOperatorHostname(t *testing.T) {
	t.Parallel()

	stack := models.OperatorDocumentGo{
		ID:                "data-1",
		OperatorSessionID: "sess-data-1",
		Status:            constants.OperatorStatusActive,
		OperatorType:      constants.OperatorTypeRemote,
		CurrentHostname:   constants.DataOperatorHostname,
	}
	assert.True(t, IsStackDataOperator(stack))

	other := stack
	other.CurrentHostname = "loadtest-host"
	assert.False(t, IsStackDataOperator(other))

	inference := stack
	inference.RuntimeConfig = &models.RuntimeConfig{InferenceEnabled: true}
	assert.False(t, IsStackDataOperator(inference))
}
