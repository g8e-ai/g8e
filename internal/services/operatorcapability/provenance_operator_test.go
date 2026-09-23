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

func TestSelectProvenanceOperator(t *testing.T) {
	operators := []models.OperatorDocumentGo{
		{ID: "prov-1", OperatorSessionID: "sess-prov-1", Status: constants.OperatorStatusActive, OperatorType: constants.OperatorTypeRemote, RuntimeConfig: &models.RuntimeConfig{ProvenanceOperatorEnabled: true}},
	}
	selected, err := SelectProvenanceOperator(operators, "")
	assert.NoError(t, err)
	assert.Equal(t, "sess-prov-1", selected.OperatorSessionID)
}

func TestActiveProvenanceOperatorsFiltersAndProjectsOperators(t *testing.T) {
	t.Parallel()

	operators := []models.OperatorDocumentGo{
		{ID: "inactive", OperatorSessionID: "sess-inactive", Status: constants.OperatorStatusAvailable, OperatorType: constants.OperatorTypeRemote, RuntimeConfig: &models.RuntimeConfig{ProvenanceOperatorEnabled: true}},
		{ID: "local", OperatorSessionID: "sess-local", Status: constants.OperatorStatusActive, OperatorType: constants.OperatorTypeEmbedded, RuntimeConfig: &models.RuntimeConfig{ProvenanceOperatorEnabled: true}},
		{ID: "disabled", OperatorSessionID: "sess-disabled", Status: constants.OperatorStatusActive, OperatorType: constants.OperatorTypeRemote, RuntimeConfig: &models.RuntimeConfig{ProvenanceOperatorEnabled: false}},
		{ID: "missing-config", OperatorSessionID: "sess-missing-config", Status: constants.OperatorStatusActive, OperatorType: constants.OperatorTypeRemote},
		{ID: "missing-session", Status: constants.OperatorStatusActive, OperatorType: constants.OperatorTypeRemote, RuntimeConfig: &models.RuntimeConfig{ProvenanceOperatorEnabled: true}},
		{ID: "provenance-1", OperatorSessionID: "sess-provenance-1", Status: constants.OperatorStatusActive, OperatorType: constants.OperatorTypeRemote, RuntimeConfig: &models.RuntimeConfig{ProvenanceOperatorEnabled: true, ProvenanceOperatorModelStorageRoot: "/models", Platform: "linux"}},
	}

	matches := ActiveProvenanceOperators(operators)
	require.Len(t, matches, 1)
	assert.Equal(t, ProvenanceOperatorStatus{OperatorID: "provenance-1", OperatorSessionID: "sess-provenance-1", Status: string(constants.OperatorStatusActive), ProvenanceEnabled: true, ModelStorageRoot: "/models", Platform: "linux"}, matches[0])
}

func TestSelectProvenanceOperatorBySessionRejectsUnknownSession(t *testing.T) {
	t.Parallel()

	operators := []models.OperatorDocumentGo{{
		ID: "prov-1", OperatorSessionID: "sess-prov-1", Status: constants.OperatorStatusActive, OperatorType: constants.OperatorTypeRemote,
		RuntimeConfig: &models.RuntimeConfig{ProvenanceOperatorEnabled: true},
	}}
	selected, err := SelectProvenanceOperator(operators, "sess-prov-1")
	require.NoError(t, err)
	assert.Equal(t, "prov-1", selected.OperatorID)

	_, err = SelectProvenanceOperator(operators, "missing")
	assert.ErrorIs(t, err, constants.ErrProvenanceOperatorNotCapable)
}

func TestSelectProvenanceOperator_NotFound(t *testing.T) {
	_, err := SelectProvenanceOperator(nil, "")
	assert.ErrorIs(t, err, constants.ErrProvenanceOperatorNotFound)
}

func TestSelectProvenanceOperator_Ambiguous(t *testing.T) {
	operators := []models.OperatorDocumentGo{
		{ID: "prov-1", OperatorSessionID: "sess-prov-1", Status: constants.OperatorStatusActive, OperatorType: constants.OperatorTypeRemote, RuntimeConfig: &models.RuntimeConfig{ProvenanceOperatorEnabled: true}},
		{ID: "prov-2", OperatorSessionID: "sess-prov-2", Status: constants.OperatorStatusActive, OperatorType: constants.OperatorTypeRemote, RuntimeConfig: &models.RuntimeConfig{ProvenanceOperatorEnabled: true}},
	}
	_, err := SelectProvenanceOperator(operators, "")
	assert.ErrorIs(t, err, constants.ErrProvenanceOperatorAmbiguous)
}
