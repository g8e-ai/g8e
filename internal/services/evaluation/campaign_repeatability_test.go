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
)

func TestValidateRepeatabilitySpec(t *testing.T) {
	spec := testCampaignSpec()
	spec.RepetitionCount = RepeatabilityRepetitionCount
	require.NoError(t, ValidateRepeatabilitySpec(spec))
	spec.RepetitionCount = 1
	assert.Error(t, ValidateRepeatabilitySpec(spec))
}

func TestComputeRepeatabilityMatrixSize(t *testing.T) {
	assert.Equal(t, uint64(375), ComputeRepeatabilityMatrixSize(1))
	assert.Equal(t, uint64(13125), ComputeRepeatabilityMatrixSize(35))
}
