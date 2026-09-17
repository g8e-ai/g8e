// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package evaluation

import (
	"context"
	"database/sql"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/timestamppb"

	evalv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/eval/v1"
	_ "modernc.org/sqlite"
)

func TestCampaignExporter_ExportRunWritesDisclosureSafeBundle(t *testing.T) {
	files := newCampaignMemoryFileService()
	store := NewStore(files)
	controller := NewCampaignController(store, &stubCampaignExecutor{}, func() time.Time { return time.Unix(1_700_000_000, 0).UTC() }, func(prefix string) string { return prefix + "-1" })
	req := testCampaignInitRequest(t)
	catalog := req.Catalog
	truncated := &evalv1.EvaluationScenarioCatalog{
		SchemaVersion: catalog.GetSchemaVersion(),
		CatalogRef:    catalog.GetCatalogRef(),
		Scenarios:     catalog.GetScenarios()[:1],
	}
	truncatedDigest, err := ComputeScenarioCatalogDigest(truncated)
	require.NoError(t, err)
	truncated.CatalogDigest = truncatedDigest
	req.Catalog = truncated

	_, err = controller.InitializeCampaign(context.Background(), req)
	require.NoError(t, err)
	_, err = controller.ScheduleHomogeneousRun(context.Background(), req.RunID)
	require.NoError(t, err)
	first, ok, err := controller.ResumeNextAssignment(context.Background(), req.RunID)
	require.NoError(t, err)
	require.True(t, ok)
	_, executed, err := controller.ExecuteNextAssignment(context.Background(), req.RunID, CampaignExecutionBinding{
		InferenceOperatorSessionID: "inf-session",
		DataOperatorID:             "data-op",
		DataOperatorSessionID:      "data-session",
		ModelRegistryDigest:        req.Inventory.RegistryDigest,
		ModelRegistry:              req.Inventory.ToModelRegistryFreeze().Variants,
	}, req.ScenarioArtifacts)
	require.NoError(t, err)
	require.True(t, executed)

	outputDir := filepath.Join(t.TempDir(), "export")
	exporter := NewCampaignExporter(func() time.Time { return time.Unix(1_700_000_100, 0).UTC() })
	report, err := exporter.ExportRun(context.Background(), store, files, req.RunID, outputDir)
	require.NoError(t, err)
	require.NotNil(t, report)
	assert.Equal(t, req.RunID, report.RunID)
	assert.Equal(t, uint32(3), report.AssignmentCount)
	assert.Equal(t, uint32(1), report.TerminalResultCount)
	assert.Len(t, report.Files, 7)

	assignmentsJSONL, err := files.ReadFile(context.Background(), filepath.Join(outputDir, "assignments.jsonl"))
	require.NoError(t, err)
	lines := strings.Split(strings.TrimSpace(string(assignmentsJSONL)), "\n")
	require.Len(t, lines, 1)
	var assignmentRecord CampaignExportAssignmentRecord
	require.NoError(t, json.Unmarshal([]byte(lines[0]), &assignmentRecord))
	assert.Equal(t, campaignExportSchemaVersion, assignmentRecord.SchemaVersion)
	assert.Equal(t, publicMessageTypeAssignmentResult, assignmentRecord.RecordType)
	assert.Equal(t, first.GetAssignmentId(), assignmentRecord.Projection.GetAssignmentId())
	assert.NotContains(t, lines[0], "prompt")
	assert.NotContains(t, lines[0], "raw_output")

	runSummaryBody, err := files.ReadFile(context.Background(), filepath.Join(outputDir, "run_summary.json"))
	require.NoError(t, err)
	var runSummary map[string]any
	require.NoError(t, json.Unmarshal(runSummaryBody, &runSummary))
	assert.Equal(t, req.RunID, runSummary["run_id"])
	assert.Equal(t, truncated.GetCatalogDigest(), runSummary["catalog_digest"])

	db, err := sql.Open("sqlite", filepath.Join(outputDir, "campaign_export.sqlite"))
	require.NoError(t, err)
	defer db.Close()
	var assignmentCount int
	require.NoError(t, db.QueryRow(`SELECT COUNT(*) FROM assignment_results`).Scan(&assignmentCount))
	assert.Equal(t, 1, assignmentCount)
	var modelSummaryCount int
	require.NoError(t, db.QueryRow(`SELECT COUNT(*) FROM model_summaries`).Scan(&modelSummaryCount))
	assert.Equal(t, 3, modelSummaryCount)
}

func TestCampaignExporter_ExportRunRequiresRunID(t *testing.T) {
	files := newCampaignMemoryFileService()
	store := NewStore(files)
	exporter := NewCampaignExporter(nil)
	_, err := exporter.ExportRun(context.Background(), store, files, "", "")
	require.Error(t, err)
}

func TestBuildAssignmentResultsCSV_EmitsHeaderAndRow(t *testing.T) {
	completedAt := timestamppb.New(time.Unix(1_700_000_000, 0).UTC())
	csvBody, err := buildAssignmentResultsCSV([]CampaignExportAssignmentRecord{
		{
			SchemaVersion: campaignExportSchemaVersion,
			RecordType:    publicMessageTypeAssignmentResult,
			Projection: &evalv1.PublicAssignmentResultProjection{
				AssignmentId:       "assignment-1",
				RunId:              "run-1",
				ScenarioId:         "scenario-1",
				ScenarioCategory:   evalv1.EvaluationScenarioCategory_EVALUATION_SCENARIO_CATEGORY_INSTRUCTION_ADHERENCE,
				Lane:               evalv1.EvaluationLane_EVALUATION_LANE_MODEL_ROLE,
				DesignatedRole:     evalv1.ModelCampaignRole_MODEL_CAMPAIGN_ROLE_PRIMARY,
				VariantId:          "variant-1",
				LifecycleStatus:    evalv1.EvaluationAssignmentLifecycleStatus_EVALUATION_ASSIGNMENT_LIFECYCLE_STATUS_COMPLETED,
				SummaryStatus:      evalv1.EvaluationVerdictStatus_EVALUATION_VERDICT_STATUS_PASS,
				ResultDigest:       "digest-1",
				VerificationStatus: "unverified",
				CompletedAt:        completedAt,
			},
			BenchmarkObservations: &PublicBenchmarkObservations{
				Timing: &PublicBenchmarkTiming{
					GenerationMS: &PublicMetricValue{Value: 42.5},
				},
			},
		},
	})
	require.NoError(t, err)
	text := string(csvBody)
	assert.Contains(t, text, "assignment_id,run_id,scenario_id")
	assert.Contains(t, text, "assignment-1,run-1,scenario-1")
	assert.Contains(t, text, "42.500")
}
