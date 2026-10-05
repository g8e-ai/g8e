// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

//go:build integration

package evaluation

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/services/fs"
	"github.com/g8e-ai/g8e/v2/internal/testutil"
	evalv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/eval/v1"
)

// TestCampaignWitnessEvidence_DecoupledObservationAndProvenance exercises the
// independent capture and verification paths of provider-boundary observation
// and storage-side model provenance over a real filesystem-backed evaluation store.
func TestCampaignWitnessEvidence_DecoupledObservationAndProvenance(t *testing.T) {
	fileSvc, err := fs.NewRuntimeFileService(testutil.TempDir(t), testutil.NewTestLogger())
	require.NoError(t, err)
	require.NoError(t, fileSvc.CreateRuntimeTree(context.Background()))
	store := NewStore(fileSvc)

	clock := func() time.Time { return time.Unix(1_700_000_000, 0).UTC() }
	idGen := func(prefix string) string { return prefix + "-integ" }
	controller := NewCampaignController(store, &stubCampaignExecutor{}, clock, idGen)

	req := testCampaignInitRequest(t)
	spec, err := controller.CreateCampaign(context.Background(), CampaignCreateRequest{
		CampaignID:        req.CampaignID,
		Catalog:           req.Catalog,
		Inventory:         req.Inventory,
		ScenarioArtifacts: req.ScenarioArtifacts,
		RepetitionCount:   req.RepetitionCount,
		Platform:          req.Platform,
	})
	require.NoError(t, err)
	require.NotNil(t, spec)

	// Idempotent re-creation across source revisions must succeed against real store
	recreated, err := controller.CreateCampaign(context.Background(), CampaignCreateRequest{
		CampaignID:        req.CampaignID,
		Catalog:           req.Catalog,
		Inventory:         req.Inventory,
		ScenarioArtifacts: req.ScenarioArtifacts,
		RepetitionCount:   req.RepetitionCount,
		Platform:          req.Platform,
	})
	require.NoError(t, err)
	assert.Equal(t, spec.GetCampaignDigest(), recreated.GetCampaignDigest())

	// Parameter drift must trigger campaign conflict
	_, err = controller.CreateCampaign(context.Background(), CampaignCreateRequest{
		CampaignID:        req.CampaignID,
		Catalog:           req.Catalog,
		Inventory:         req.Inventory,
		ScenarioArtifacts: req.ScenarioArtifacts,
		RepetitionCount:   req.RepetitionCount + 1,
		Platform:          req.Platform,
	})
	require.ErrorIs(t, err, constants.ErrEvaluationCampaignConflict)

	// Initialize and schedule run
	run, err := controller.InitializeCampaign(context.Background(), req)
	require.NoError(t, err)
	count, err := controller.ScheduleHomogeneousRun(context.Background(), req.RunID)
	require.NoError(t, err)
	require.Greater(t, count, 0)

	assignments, err := store.ListAssignments(context.Background(), req.RunID)
	require.NoError(t, err)
	require.NotEmpty(t, assignments)

	first := assignments[0]
	result := &evalv1.EvaluationAssignmentResult{
		SchemaVersion:   CampaignSchemaVersion,
		AssignmentId:    first.GetAssignmentId(),
		RunId:           req.RunID,
		CampaignId:      req.CampaignID,
		Lane:            first.GetLane(),
		LifecycleStatus: evalv1.EvaluationAssignmentLifecycleStatus_EVALUATION_ASSIGNMENT_LIFECYCLE_STATUS_COMPLETED,
		CompletedAt:     timestamppb.Now(),
		ModelInferences: StubHomogeneousAssignmentModelInferences(first),
	}
	resultDigest, err := ComputeAssignmentResultDigest(result)
	require.NoError(t, err)
	result.ResultDigest = resultDigest
	require.NoError(t, store.SaveAssignmentResult(context.Background(), result))

	// Create real readers backed by runtime files
	obsReader, err := NewCampaignProviderObservationReader(fileSvc)
	require.NoError(t, err)
	provReader, err := NewCampaignModelProvenanceReader(fileSvc)
	require.NoError(t, err)

	ctx := context.Background()

	// Provider observation capture must succeed independently
	err = CaptureCampaignRunProviderObservationEvidence(ctx, store, req.RunID, obsReader)
	require.NoError(t, err)

	// Model provenance capture must succeed independently
	err = CaptureCampaignRunModelProvenanceEvidence(ctx, store, req.RunID, provReader)
	require.NoError(t, err)

	// Both can also be invoked together through the unified helper
	err = CaptureCampaignRunWitnessEvidence(ctx, store, req.RunID, obsReader, provReader)
	require.NoError(t, err)

	// Nil reader guards: running one with nil reader must not affect the other
	err = CaptureCampaignRunProviderObservationEvidence(ctx, store, req.RunID, nil)
	require.NoError(t, err)
	err = CaptureCampaignRunModelProvenanceEvidence(ctx, store, req.RunID, nil)
	require.NoError(t, err)

	// Verification using decoupled verifier with real file store
	verifier := NewCampaignRunVerifier(clock).
		WithProviderObservationReader(obsReader, ProviderObservationPolicyInterim).
		WithModelProvenanceReader(provReader, ModelProvenancePolicyInterim)

	report, err := verifier.VerifyRun(ctx, store, req.RunID, req.Catalog, req.ScenarioArtifacts)
	require.NoError(t, err)
	require.NotNil(t, report)
	assert.Equal(t, run.GetRunId(), report.GetRunId())
}
