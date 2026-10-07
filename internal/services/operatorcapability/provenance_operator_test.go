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

func TestSelectProvenanceOperator(t *testing.T) {
	operators := []*operatorv1.OperatorDocument{
		{Id: "prov-1", OperatorSessionId: "sess-prov-1", Status: string(constants.OperatorStatusActive), OperatorType: string(constants.OperatorTypeRemote), RuntimeConfig: &operatorv1.OperatorRuntimeConfig{ProvenanceOperatorEnabled: true}},
	}
	selected, err := SelectProvenanceOperator(operators, "")
	assert.NoError(t, err)
	assert.Equal(t, "sess-prov-1", selected.OperatorSessionID)
}

func TestActiveProvenanceOperatorsFiltersAndProjectsOperators(t *testing.T) {
	t.Parallel()

	operators := []*operatorv1.OperatorDocument{
		{Id: "inactive", OperatorSessionId: "sess-inactive", Status: string(constants.OperatorStatusAvailable), OperatorType: string(constants.OperatorTypeRemote), RuntimeConfig: &operatorv1.OperatorRuntimeConfig{ProvenanceOperatorEnabled: true}},
		{Id: "local", OperatorSessionId: "sess-local", Status: string(constants.OperatorStatusActive), OperatorType: string(constants.OperatorTypeEmbedded), RuntimeConfig: &operatorv1.OperatorRuntimeConfig{ProvenanceOperatorEnabled: true}},
		{Id: "disabled", OperatorSessionId: "sess-disabled", Status: string(constants.OperatorStatusActive), OperatorType: string(constants.OperatorTypeRemote), RuntimeConfig: &operatorv1.OperatorRuntimeConfig{ProvenanceOperatorEnabled: false}},
		{Id: "missing-config", OperatorSessionId: "sess-missing-config", Status: string(constants.OperatorStatusActive), OperatorType: string(constants.OperatorTypeRemote)},
		{Id: "missing-session", Status: string(constants.OperatorStatusActive), OperatorType: string(constants.OperatorTypeRemote), RuntimeConfig: &operatorv1.OperatorRuntimeConfig{ProvenanceOperatorEnabled: true}},
		{Id: "provenance-1", OperatorSessionId: "sess-provenance-1", Status: string(constants.OperatorStatusActive), OperatorType: string(constants.OperatorTypeRemote), RuntimeConfig: &operatorv1.OperatorRuntimeConfig{ProvenanceOperatorEnabled: true, ProvenanceOperatorModelStorageRoot: "/models", Platform: "linux"}},
	}

	matches := ActiveProvenanceOperators(operators)
	require.Len(t, matches, 2)
	assert.Equal(t, ProvenanceOperatorStatus{OperatorID: "provenance-1", OperatorSessionID: "sess-provenance-1", Status: string(constants.OperatorStatusActive), ProvenanceEnabled: true, ModelStorageRoot: "/models", Platform: "linux"}, matches[1])
}

func TestSelectProvenanceOperatorBySessionRejectsUnknownSession(t *testing.T) {
	t.Parallel()

	operators := []*operatorv1.OperatorDocument{{
		Id: "prov-1", OperatorSessionId: "sess-prov-1", Status: string(constants.OperatorStatusActive), OperatorType: string(constants.OperatorTypeRemote),
		RuntimeConfig: &operatorv1.OperatorRuntimeConfig{ProvenanceOperatorEnabled: true},
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
	operators := []*operatorv1.OperatorDocument{
		{Id: "prov-1", OperatorSessionId: "sess-prov-1", Status: string(constants.OperatorStatusActive), OperatorType: string(constants.OperatorTypeRemote), RuntimeConfig: &operatorv1.OperatorRuntimeConfig{ProvenanceOperatorEnabled: true}},
		{Id: "prov-2", OperatorSessionId: "sess-prov-2", Status: string(constants.OperatorStatusActive), OperatorType: string(constants.OperatorTypeRemote), RuntimeConfig: &operatorv1.OperatorRuntimeConfig{ProvenanceOperatorEnabled: true}},
	}
	_, err := SelectProvenanceOperator(operators, "")
	assert.ErrorIs(t, err, constants.ErrProvenanceOperatorAmbiguous)
}
