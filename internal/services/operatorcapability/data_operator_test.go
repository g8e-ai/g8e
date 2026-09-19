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

func TestIsGovernedDataOperator_ExcludesSpecializedRoles(t *testing.T) {
	t.Parallel()

	data := models.OperatorDocumentGo{
		ID:                "data-1",
		OperatorSessionID: "sess-data-1",
		Status:            constants.OperatorStatusActive,
		OperatorType:      constants.OperatorTypeRemote,
	}
	assert.True(t, IsGovernedDataOperator(data))

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
	}
	for _, op := range cases {
		assert.False(t, IsGovernedDataOperator(op))
	}
}
