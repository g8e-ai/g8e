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
)

func TestSelectProvenanceOperator(t *testing.T) {
	operators := []*operatorv1.OperatorDocument{
		{
			ID:                "prov-1",
			OperatorSessionID: "sess-prov-1",
			Status:            constants.OperatorStatusActive,
			OperatorType:      constants.OperatorTypeRemote,
			RuntimeConfig:     &operatorv1.OperatorRuntimeConfig{ProvenanceOperatorEnabled: true},
		},
	}
	selected, err := SelectProvenanceOperator(operators, "")
	require.NoError(t, err)
	assert.Equal(t, "sess-prov-1", selected.OperatorSessionID)
}

func TestSelectProvenanceOperator_NotFound(t *testing.T) {
	_, err := SelectProvenanceOperator(nil, "")
	assert.ErrorIs(t, err, constants.ErrProvenanceOperatorNotFound)
}

func TestSelectProvenanceOperator_Ambiguous(t *testing.T) {
	operators := []*operatorv1.OperatorDocument{
		{
			ID:                "prov-1",
			OperatorSessionID: "sess-prov-1",
			Status:            constants.OperatorStatusActive,
			OperatorType:      constants.OperatorTypeRemote,
			RuntimeConfig:     &operatorv1.OperatorRuntimeConfig{ProvenanceOperatorEnabled: true},
		},
		{
			ID:                "prov-2",
			OperatorSessionID: "sess-prov-2",
			Status:            constants.OperatorStatusActive,
			OperatorType:      constants.OperatorTypeRemote,
			RuntimeConfig:     &operatorv1.OperatorRuntimeConfig{ProvenanceOperatorEnabled: true},
		},
	}
	_, err := SelectProvenanceOperator(operators, "")
	assert.ErrorIs(t, err, constants.ErrProvenanceOperatorAmbiguous)
}
