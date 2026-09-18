// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package evaluation

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	evalv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/eval/v1"
)

func TestBuildHeterogeneousAssignmentMatrix_MaterializesStacksByScenarios(t *testing.T) {
	catalog, _, err := BuildScenarioCatalog()
	require.NoError(t, err)
	stackSet, err := GenerateHeterogeneousStackSet(HeterogeneousStackGenerationRequest{
		CampaignID: "north-star-heterogeneous",
		Seed:       11,
		Variants:   testHeterogeneousVariants(),
	})
	require.NoError(t, err)
	assignments, err := BuildHeterogeneousAssignmentMatrix(HeterogeneousScheduleRequest{
		CampaignID: "north-star-heterogeneous",
		RunID:      "run-heterogeneous-1",
		Catalog:    catalog,
		StackSet:   stackSet,
		QueuedAt:   time.Unix(1_700_000_000, 0).UTC(),
	})
	require.NoError(t, err)
	assert.Len(t, assignments, int(ComputeHeterogeneousMatrixSize(uint64(len(stackSet.Stacks)))))
	require.NoError(t, ValidateHeterogeneousAssignmentMatrix(catalog, stackSet, assignments))
	for _, assignment := range assignments {
		assert.Equal(t, evalv1.EvaluationLane_EVALUATION_LANE_SYSTEM, assignment.GetLane())
	}
}
