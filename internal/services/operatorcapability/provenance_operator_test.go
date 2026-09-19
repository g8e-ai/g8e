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
