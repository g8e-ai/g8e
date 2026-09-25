// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package evaluation

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/g8e-ai/g8e/v2/internal/models"
	evalv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/eval/v1"
)

type recordingCampaignProofPublisher struct {
	inputs []AssignmentAuditProofInput
}

func (r *recordingCampaignProofPublisher) IngestAssignmentAuditSlices(_ context.Context, inputs []AssignmentAuditProofInput, _ bool) error {
	r.inputs = append(r.inputs, inputs...)
	return nil
}

func (r *recordingCampaignProofPublisher) PruneRunProofCatalog(context.Context, string) error {
	return nil
}

func (r *recordingCampaignProofPublisher) FlushProofCatalog(context.Context) error {
	return nil
}

func TestCollectAssignmentAuditEventsFromBodies(t *testing.T) {
	body, err := MarshalPublicLiveEvent(map[string]any{
		"schema_version": "1.5.0",
		"kind":           "stage_updated",
		"dataset_id":     "dataset-1",
		"quality_state":  "live_in_progress",
		"observed_at":    "2026-09-17T00:00:00Z",
		"event_id":       "event-1",
		"run_id":         "run-1",
		"assignment_id":  "assignment-1",
		"variant_id":     "variant-a",
		"role":           "primary",
		"lifecycle_status": "running",
		"completed":      1,
		"total":          2,
		"stage_label":    "model role invoked · primary · variant-a",
	})
	require.NoError(t, err)

	events, err := CollectAssignmentAuditEventsFromBodies([][]byte{body})
	require.NoError(t, err)
	require.Len(t, events, 1)
	assert.Equal(t, "stage_updated", events[0].Type)
	assert.Equal(t, "2026-09-17T00:00:00Z", events[0].Timestamp)
}

func TestCampaignPublicationCoordinator_BuildAssignmentAuditBindings(t *testing.T) {
	files := newCampaignMemoryFileService()
	exporter := &recordingCampaignFeedExporter{}
	proofPublisher := &recordingCampaignProofPublisher{}
	coordinator := NewCampaignPublicationCoordinator(NewStore(files), files, NewMemoryCampaignPublicationStateStore(), exporter, nil).
		WithProofPublisher(proofPublisher)

	runID := "run-audit-1"
	assignmentID := "assign-audit-1"
	assignment := &evalv1.EvaluationAssignment{
		SchemaVersion:   CampaignSchemaVersion,
		AssignmentId:    assignmentID,
		RunId:           runID,
		CampaignId:      "campaign-1",
		ScenarioId:      "scenario-1",
		Repetition:      1,
		Lane:            evalv1.EvaluationLane_EVALUATION_LANE_MODEL_ROLE,
		LifecycleStatus: evalv1.EvaluationAssignmentLifecycleStatus_EVALUATION_ASSIGNMENT_LIFECYCLE_STATUS_COMPLETED,
		Target: &evalv1.EvaluationAssignment_Homogeneous{
			Homogeneous: &evalv1.HomogeneousAssignmentTarget{
				DesignatedRole:   evalv1.ModelCampaignRole_MODEL_CAMPAIGN_ROLE_PRIMARY,
				CandidateVariant: &evalv1.ModelVariant{VariantId: "qwen3-4b"},
			},
		},
	}
	result := &evalv1.EvaluationAssignmentResult{
		SchemaVersion:   CampaignSchemaVersion,
		AssignmentId:    assignmentID,
		RunId:           runID,
		CampaignId:      "campaign-1",
		Lane:            evalv1.EvaluationLane_EVALUATION_LANE_MODEL_ROLE,
		LifecycleStatus: evalv1.EvaluationAssignmentLifecycleStatus_EVALUATION_ASSIGNMENT_LIFECYCLE_STATUS_COMPLETED,
		CompletedAt:     timestamppb.Now(),
		ModelInferences: []*evalv1.ModelInferenceRecord{{
			ModelRole:         evalv1.ModelCampaignRole_MODEL_CAMPAIGN_ROLE_PRIMARY,
			ModelVariant:      &evalv1.ModelVariant{VariantId: "qwen3-4b"},
			UsageAvailability: evalv1.EvaluationUsageAvailability_EVALUATION_USAGE_AVAILABILITY_REPORTED,
		}},
	}
	digest, err := ComputeAssignmentResultDigest(result)
	require.NoError(t, err)
	result.ResultDigest = digest

	store := NewStore(files)
	spec := testCampaignSpec()
	spec.CampaignId = "campaign-1"
	specDigest, err := ComputeCampaignSpecDigest(spec)
	require.NoError(t, err)
	spec.CampaignDigest = specDigest
	require.NoError(t, store.SaveCampaignSpec(context.Background(), spec))
	require.NoError(t, store.SaveRun(context.Background(), &evalv1.EvaluationRun{
		SchemaVersion:   CampaignSchemaVersion,
		RunId:           runID,
		CampaignBinding: &evalv1.ModelCampaignBinding{CampaignId: "campaign-1"},
	}))
	require.NoError(t, store.SaveAssignment(context.Background(), assignment))
	require.NoError(t, store.SaveAssignmentResult(context.Background(), result))

	liveRequests, err := coordinator.buildAssignmentLiveEventPublishRequests(context.Background(), assignment, result)
	require.NoError(t, err)
	require.NotEmpty(t, liveRequests)

	bindings, err := coordinator.BuildAssignmentAuditBindings(context.Background(), assignment, result, liveRequests, true)
	require.NoError(t, err)
	require.Len(t, bindings, 2)
	assert.Equal(t, AssignmentAuditSliceKind, bindings[0].GetKind())
	assert.Equal(t, AssignmentAuditVaultKeyKind, bindings[1].GetKind())
	require.Len(t, proofPublisher.inputs, 1)
	assert.Equal(t, assignmentID, proofPublisher.inputs[0].AssignmentID)
	assert.NotEmpty(t, proofPublisher.inputs[0].Artifacts.Database)
	assert.NotEmpty(t, proofPublisher.inputs[0].Artifacts.VaultKey)
}

func TestCampaignPublicationCoordinator_FlushesDeferredProofBatch(t *testing.T) {
	files := newCampaignMemoryFileService()
	exporter := &recordingCampaignFeedExporter{}
	proofPublisher := &recordingCampaignProofPublisher{}
	coordinator := NewCampaignPublicationCoordinator(NewStore(files), files, NewMemoryCampaignPublicationStateStore(), exporter, nil).
		WithProofPublisher(proofPublisher)
	coordinator.deferProofMirrorPush = true
	coordinator.pendingProofInputs = []AssignmentAuditProofInput{
		{AssignmentID: "assignment-1"},
		{AssignmentID: "assignment-2"},
	}
	coordinator.pendingProofMirrorPush = true

	require.NoError(t, coordinator.flushProofCatalog(context.Background()))
	require.Len(t, proofPublisher.inputs, 2)
	assert.Equal(t, "assignment-1", proofPublisher.inputs[0].AssignmentID)
	assert.Equal(t, "assignment-2", proofPublisher.inputs[1].AssignmentID)
}

func TestCollectAssignmentAuditEventBodies_SkipsProjections(t *testing.T) {
	bodies := collectAssignmentAuditEventBodies([]campaignFeedPublishRequest{
		{RecordType: models.PublicFeedRecordTypeProjection, Body: []byte(`{"kind":"assignment_result"}`)},
		{RecordType: models.PublicFeedRecordTypeEvent, Body: []byte(`{"kind":"metric_updated","observed_at":"2026-09-17T00:00:00Z"}`)},
	})
	require.Len(t, bodies, 1)
}
