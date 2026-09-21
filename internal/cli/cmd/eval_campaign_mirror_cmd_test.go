// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package cmd

import (
	"bytes"
	"context"
	"log/slog"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/g8e-ai/g8e/v2/internal/services/evaluation"
	"github.com/g8e-ai/g8e/v2/internal/services/fs"
	evalv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/eval/v1"
)

func TestReconcileVerifiedCampaignMirrorQueue_LoadsQueueAndRestores(t *testing.T) {
	root := t.TempDir()
	files, err := fs.NewRuntimeFileService(root, slog.Default())
	require.NoError(t, err)
	store := evaluation.NewStore(files)
	exporter := &evaluationRecordingCampaignFeedExporter{}
	probe := &evaluationStubCampaignMirrorProbe{present: map[string]bool{}}
	coordinator := evaluation.NewCampaignPublicationCoordinator(
		store, files, evaluation.NewMemoryCampaignPublicationStateStore(), exporter, nil,
	).WithMirrorProbe(probe)
	controller := evaluation.NewCampaignController(
		store,
		&evaluationStubCampaignExecutor{},
		func() time.Time { return time.Unix(1_700_000_000, 0).UTC() },
		func(prefix string) string { return prefix + "-1" },
	).WithPublication(coordinator)

	req := evaluationTestCampaignInitRequest(t)
	catalog := req.Catalog
	truncated := &evalv1.EvaluationScenarioCatalog{
		SchemaVersion: catalog.GetSchemaVersion(),
		CatalogRef:    catalog.GetCatalogRef(),
		Scenarios:     catalog.GetScenarios()[:1],
	}
	truncatedDigest, err := evaluation.ComputeScenarioCatalogDigest(truncated)
	require.NoError(t, err)
	truncated.CatalogDigest = truncatedDigest
	req.Catalog = truncated

	run, err := controller.InitializeCampaign(context.Background(), req)
	require.NoError(t, err)
	_, err = controller.ScheduleHomogeneousRun(context.Background(), run.GetRunId())
	require.NoError(t, err)
	_, _, err = controller.ExecuteNextAssignment(context.Background(), run.GetRunId(), evaluation.CampaignExecutionBinding{
		InferenceOperatorSessionID: "inf-session",
		DataOperatorID:             "data-op",
		DataOperatorSessionID:      "data-session",
		ModelRegistryDigest:        req.Inventory.RegistryDigest,
		ModelRegistry:              evaluation.InferenceVariantsFromEvalRegistry(req.Inventory.Variants),
	}, req.ScenarioArtifacts)
	require.NoError(t, err)

	queue := &evaluation.CampaignQueue{
		Models: []evaluation.CampaignQueueModel{{
			VariantID:     "gemma4-e4b",
			Status:        "verified",
			VerifiedRunID: run.GetRunId(),
		}},
	}
	queuePath := filepath.Join(root, evaluation.DefaultInitCampaignQueueRelPath)
	require.NoError(t, evaluation.SaveInitCampaignQueue(queuePath, queue))

	reconciler := evaluation.NewCampaignMirrorReconciler(coordinator, store, probe)
	result, err := reconcileVerifiedCampaignMirrorQueue(context.Background(), root, reconciler)
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, []string{run.GetRunId()}, result.RestoredRunIDs)
}

type evaluationRecordingCampaignFeedExporter struct {
	records []evaluation.CampaignPublicFeedRecord
}

func (e *evaluationRecordingCampaignFeedExporter) HighWaterSequence(context.Context) (int64, error) {
	if e == nil || len(e.records) == 0 {
		return 0, nil
	}
	return e.records[len(e.records)-1].Sequence, nil
}

func (e *evaluationRecordingCampaignFeedExporter) ExportBatch(_ context.Context, records []evaluation.CampaignPublicFeedRecord) error {
	e.records = append(e.records, records...)
	return nil
}

type evaluationStubCampaignMirrorProbe struct {
	present map[string]bool
}

func (s *evaluationStubCampaignMirrorProbe) DatasetPresent(_ context.Context, datasetID string) (bool, error) {
	if s == nil || s.present == nil {
		return false, nil
	}
	return s.present[datasetID], nil
}

type evaluationStubCampaignExecutor struct{}

func (evaluationStubCampaignExecutor) ExecuteAssignment(_ context.Context, req evaluation.AssignmentExecutionRequest) (*evalv1.EvaluationAssignmentResult, error) {
	result := &evalv1.EvaluationAssignmentResult{
		SchemaVersion:   evaluation.CampaignSchemaVersion,
		AssignmentId:    req.Assignment.GetAssignmentId(),
		RunId:           req.Assignment.GetRunId(),
		CampaignId:      req.Assignment.GetCampaignId(),
		Lane:            req.Assignment.GetLane(),
		LifecycleStatus: evalv1.EvaluationAssignmentLifecycleStatus_EVALUATION_ASSIGNMENT_LIFECYCLE_STATUS_COMPLETED,
		CompletedAt:     timestamppb.Now(),
	}
	digest, err := evaluation.ComputeAssignmentResultDigest(result)
	if err != nil {
		return nil, err
	}
	result.ResultDigest = digest
	return result, nil
}

func TestCampaignEvalMirrorRestoreQueue_ViaCLI(t *testing.T) {
	withGatewayHealthCheck(t, true)
	root, deps, _, cleanup := setupCampaignPublishGatewayEnv(t)
	defer cleanup()

	fileSvc, err := deps.fileSvcFactory(root, nil)
	require.NoError(t, err)
	store := evaluation.NewStore(fileSvc)
	exporter := &evaluationRecordingCampaignFeedExporter{}
	probe := &evaluationStubCampaignMirrorProbe{present: map[string]bool{}}
	coordinator := evaluation.NewCampaignPublicationCoordinator(
		store, fileSvc, evaluation.NewMemoryCampaignPublicationStateStore(), exporter, nil,
	).WithMirrorProbe(probe)
	controller := evaluation.NewCampaignController(
		store,
		&evaluationStubCampaignExecutor{},
		deps.now,
		func(prefix string) string { return prefix + "-1" },
	).WithPublication(coordinator)

	req := evaluationTestCampaignInitRequest(t)
	run, err := controller.InitializeCampaign(context.Background(), req)
	require.NoError(t, err)
	_, err = controller.ScheduleHomogeneousRun(context.Background(), run.GetRunId())
	require.NoError(t, err)
	_, _, err = controller.ExecuteNextAssignment(context.Background(), run.GetRunId(), evaluation.CampaignExecutionBinding{
		InferenceOperatorSessionID: "inf-session",
		DataOperatorID:             "data-op",
		DataOperatorSessionID:      "data-session",
		ModelRegistryDigest:        req.Inventory.RegistryDigest,
		ModelRegistry:              evaluation.InferenceVariantsFromEvalRegistry(req.Inventory.Variants),
	}, req.ScenarioArtifacts)
	require.NoError(t, err)

	queue := &evaluation.CampaignQueue{
		Models: []evaluation.CampaignQueueModel{{
			VariantID:     "qwen3-4b",
			Status:        "verified",
			VerifiedRunID: run.GetRunId(),
		}},
	}
	queuePath := filepath.Join(root, evaluation.DefaultInitCampaignQueueRelPath)
	require.NoError(t, evaluation.SaveInitCampaignQueue(queuePath, queue))

	command := evalCmdWithConfig(deps)
	var output bytes.Buffer
	command.SetOut(&output)
	command.SetArgs([]string{"campaign", "mirror", "restore", "--project-root", root, "--queue"})
	require.NoError(t, command.Execute())
	assert.Contains(t, output.String(), "Restored")
}

func TestCampaignEvalMirrorRestoreRun_ViaCLI(t *testing.T) {
	withGatewayHealthCheck(t, true)
	root, deps, _, cleanup := setupCampaignPublishGatewayEnv(t)
	defer cleanup()

	fileSvc, err := deps.fileSvcFactory(root, nil)
	require.NoError(t, err)
	store := evaluation.NewStore(fileSvc)
	exporter := &evaluationRecordingCampaignFeedExporter{}
	probe := &evaluationStubCampaignMirrorProbe{present: map[string]bool{}}
	coordinator := evaluation.NewCampaignPublicationCoordinator(
		store, fileSvc, evaluation.NewMemoryCampaignPublicationStateStore(), exporter, nil,
	).WithMirrorProbe(probe)
	controller := evaluation.NewCampaignController(
		store,
		&evaluationStubCampaignExecutor{},
		deps.now,
		func(prefix string) string { return prefix + "-1" },
	).WithPublication(coordinator)

	req := evaluationTestCampaignInitRequest(t)
	run, err := controller.InitializeCampaign(context.Background(), req)
	require.NoError(t, err)
	_, err = controller.ScheduleHomogeneousRun(context.Background(), run.GetRunId())
	require.NoError(t, err)
	_, _, err = controller.ExecuteNextAssignment(context.Background(), run.GetRunId(), evaluation.CampaignExecutionBinding{
		InferenceOperatorSessionID: "inf-session",
		DataOperatorID:             "data-op",
		DataOperatorSessionID:      "data-session",
		ModelRegistryDigest:        req.Inventory.RegistryDigest,
		ModelRegistry:              evaluation.InferenceVariantsFromEvalRegistry(req.Inventory.Variants),
	}, req.ScenarioArtifacts)
	require.NoError(t, err)

	command := evalCmdWithConfig(deps)
	var output bytes.Buffer
	command.SetOut(&output)
	command.SetArgs([]string{"campaign", "mirror", "restore", "--project-root", root, "--run-id", run.GetRunId()})
	require.NoError(t, command.Execute())
	assert.Contains(t, output.String(), run.GetRunId())
}

func evaluationTestCampaignInitRequest(t *testing.T) evaluation.CampaignInitRequest {
	t.Helper()
	catalog, artifacts, err := evaluation.LoadScenarioCatalog()
	require.NoError(t, err)
	inventory, err := evaluation.MaterializeModelRegistry("north-star-smoke", []*evalv1.ModelVariant{
		{
			VariantId:      "qwen3-4b",
			ServedModelTag: "qwen3:4b",
			ModelDigest:    "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
			ProviderClass:  "ollama",
		},
	})
	require.NoError(t, err)
	return evaluation.CampaignInitRequest{
		CampaignID:                 "north-star-smoke",
		RunID:                      "run-smoke-1",
		Catalog:                    catalog,
		Inventory:                  inventory,
		ScenarioArtifacts:          artifacts,
		RepetitionCount:            1,
		InferenceOperatorSessionID: "inf-session",
		DataOperatorSessionID:      "data-session",
	}
}
