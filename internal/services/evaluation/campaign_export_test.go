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

	"github.com/g8e-ai/g8e/v2/internal/constants"
	compliancev1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/compliance/v1"
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
	assert.Len(t, report.Files, 8)
	var assignmentFile CampaignExportFile
	for _, file := range report.Files {
		if file.Name == "assignments.jsonl" {
			assignmentFile = file
			break
		}
	}
	assert.Equal(t, uint32(1), assignmentFile.RecordCount)

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
	var runSummary campaignRunSummaryExport
	require.NoError(t, json.Unmarshal(runSummaryBody, &runSummary))
	assert.Equal(t, req.RunID, runSummary.RunID)
	assert.Equal(t, truncated.GetCatalogDigest(), runSummary.CatalogDigest)

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

func TestBuildCampaignExportSchemaDocumentUsesTypedFileAndTableContracts(t *testing.T) {
	document := buildCampaignExportSchemaDocument()
	body, err := json.Marshal(document)
	require.NoError(t, err)

	var decoded struct {
		SchemaVersion string `json:"schema_version"`
		Files         map[string]struct {
			Format string `json:"format"`
			Tables []struct {
				Name       string          `json:"name"`
				PrimaryKey json.RawMessage `json:"primary_key"`
			} `json:"tables"`
		} `json:"files"`
	}
	require.NoError(t, json.Unmarshal(body, &decoded))
	assert.Equal(t, campaignExportSchemaVersion, decoded.SchemaVersion)
	assert.Equal(t, "json", decoded.Files[constants.EvaluationRunSummaryFilename].Format)

	sqliteSchema := decoded.Files[constants.EvaluationCampaignExportSQLiteFilename]
	require.Len(t, sqliteSchema.Tables, 3)
	assert.JSONEq(t, `"run_id"`, string(sqliteSchema.Tables[0].PrimaryKey))
	assert.JSONEq(t, `["variant_id","role"]`, string(sqliteSchema.Tables[2].PrimaryKey))
}

func TestCampaignExporter_JSONLUsesCanonicalProtoJSONAndRichExtensions(t *testing.T) {
	body, err := marshalJSONL([]CampaignExportAssignmentRecord{{
		SchemaVersion: campaignExportSchemaVersion,
		RecordType:    publicMessageTypeAssignmentResult,
		Projection: &evalv1.PublicAssignmentResultProjection{
			AssignmentId:    "assignment-rich",
			RunId:           "run-rich",
			ActivitySummary: &evalv1.PublicAssignmentActivitySummary{ModelActivity: &evalv1.PublicModelActivity{Records: []*evalv1.PublicModelActivityRecord{{InputTokens: 7}}}},
		},
		ResourceSummary: &PublicResourceSummary{Retries: PublicResourceMetric{UnavailableReason: evalv1.PublicUnavailableReason_PUBLIC_UNAVAILABLE_REASON_SOURCE_NOT_CAPTURED}},
	}})
	require.NoError(t, err)
	assert.Contains(t, string(body), `"assignment_id":"assignment-rich"`)
	assert.Contains(t, string(body), `"source_not_captured"`)
	assert.NotContains(t, string(body), `"InputTokens"`)
}

func TestCampaignExporter_BuildAssignmentExportRecordIncludesGradeOnlyObservations(t *testing.T) {
	files := newCampaignMemoryFileService()
	reader, err := NewCampaignProviderObservationReader(files)
	require.NoError(t, err)

	result := &evalv1.EvaluationAssignmentResult{
		AssignmentId:    "assignment-grade-only",
		RunId:           "run-grade-only",
		LifecycleStatus: evalv1.EvaluationAssignmentLifecycleStatus_EVALUATION_ASSIGNMENT_LIFECYCLE_STATUS_COMPLETED,
		DeterministicGrades: []*evalv1.DeterministicGrade{{
			CriterionId: "tool-selection",
			Status:      evalv1.EvaluationVerdictStatus_EVALUATION_VERDICT_STATUS_FAIL,
			Detail:      "private grading detail",
		}},
	}
	record, err := (&CampaignExporter{}).buildAssignmentExportRecord(
		context.Background(),
		NewStore(files),
		&evalv1.EvaluationRun{RunId: "run-grade-only"},
		&evalv1.EvaluationScenarioCatalog{CatalogDigest: "historical-catalog"},
		&evalv1.EvaluationAssignment{AssignmentId: "assignment-grade-only", RunId: "run-grade-only"},
		result,
		evalv1.EvaluationScenarioCategory_EVALUATION_SCENARIO_CATEGORY_TOOL_SELECTION,
		reader,
		false,
		nil,
	)
	require.NoError(t, err)
	require.NotNil(t, record.BenchmarkObservations)
	require.Len(t, record.BenchmarkObservations.GradeSummaries, 1)
	assert.Equal(t, "tool-selection", record.BenchmarkObservations.GradeSummaries[0].CriterionID)
	assert.Empty(t, record.BenchmarkObservations.GradeSummaries[0].Detail)
	require.NotNil(t, record.BenchmarkObservations.ToolScorecard["tool_selection"])
	assert.Equal(t, 0.0, *record.BenchmarkObservations.ToolScorecard["tool_selection"].Value)
}

func TestCampaignExporter_ExportRunRequiresRunID(t *testing.T) {
	files := newCampaignMemoryFileService()
	store := NewStore(files)
	exporter := NewCampaignExporter(nil)
	_, err := exporter.ExportRun(context.Background(), store, files, "", "")
	require.Error(t, err)
}

func TestCampaignExporter_ExportRunWithVerification(t *testing.T) {
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

	run, err := controller.InitializeCampaign(context.Background(), req)
	require.NoError(t, err)
	_, err = controller.ScheduleHomogeneousRun(context.Background(), req.RunID)
	require.NoError(t, err)
	for i := 0; i < 3; i++ {
		_, executed, err := controller.ExecuteNextAssignment(context.Background(), req.RunID, CampaignExecutionBinding{
			InferenceOperatorSessionID: "inf-session",
			DataOperatorID:             "data-op",
			DataOperatorSessionID:      "data-session",
			ModelRegistryDigest:        req.Inventory.RegistryDigest,
			ModelRegistry:              req.Inventory.ToModelRegistryFreeze().Variants,
		}, req.ScenarioArtifacts)
		require.NoError(t, err)
		require.True(t, executed)
	}

	report := &evalv1.EvaluationVerificationReport{
		SchemaVersion:           "2.0.0",
		ReportId:                req.RunID,
		RunId:                   req.RunID,
		Status:                  evalv1.EvaluationVerdictStatus_EVALUATION_VERDICT_STATUS_PASS,
		VerifiedAt:              timestamppb.New(time.Unix(1_700_000_200, 0).UTC()),
		VerifierReleaseVersion:  "test-release",
		VerifierContractVersion: "2.0.0",
		ReportDigestRef:         &compliancev1.ComplianceEvidenceReference{Sha256: strings.Repeat("a", 64)},
	}
	assignments, err := store.ListAssignments(context.Background(), req.RunID)
	require.NoError(t, err)
	results := make(map[string]*evalv1.EvaluationAssignmentResult, len(assignments))
	for _, assignment := range assignments {
		result, loadErr := store.LoadAssignmentResult(context.Background(), req.RunID, assignment.GetAssignmentId())
		require.NoError(t, loadErr)
		results[assignment.GetAssignmentId()] = result
	}
	spec, err := store.LoadCampaignSpec(context.Background(), req.CampaignID)
	require.NoError(t, err)
	cat, err := store.LoadScenarioCatalog(context.Background(), req.CampaignID)
	require.NoError(t, err)
	applicability, err := BuildRunVerificationApplicability(run, spec, cat, assignments, results, report)
	require.NoError(t, err)
	populationDigest, err := digestProto(applicability.Population)
	require.NoError(t, err)
	report.VerifiedPopulationDigest = populationDigest
	report.ExpectedAssignmentCount = applicability.ExpectedAssignmentCount
	report.VerifiedAssignmentCount = applicability.VerifiedAssignmentCount
	report.CampaignDigest = spec.GetCampaignDigest()
	report.CatalogDigest = spec.GetCatalogDigest()
	report.ModelRegistryDigest = spec.GetModelRegistryDigest()
	require.NoError(t, store.SaveCampaignVerification(context.Background(), req.RunID, report))

	aggregateState, err := CollectRunAggregateState(assignments, results)
	require.NoError(t, err)
	expectedAggregateRecords, err := BuildRunAggregateViewRecords(run, aggregateState, report, time.Unix(1_700_000_300, 0).UTC())
	require.NoError(t, err)
	var expectedEvaluationSummaryBody []byte
	for _, record := range expectedAggregateRecords {
		var header struct {
			Kind string `json:"kind"`
		}
		require.NoError(t, json.Unmarshal(record.Body, &header))
		if header.Kind == "evaluation_summary" {
			expectedEvaluationSummaryBody = record.Body
			break
		}
	}
	require.NotEmpty(t, expectedEvaluationSummaryBody)

	outputDir := filepath.Join(t.TempDir(), "export-verified")
	exporter := NewCampaignExporter(func() time.Time { return time.Unix(1_700_000_300, 0).UTC() })
	exportReport, err := exporter.ExportRun(context.Background(), store, files, req.RunID, outputDir)
	require.NoError(t, err)
	require.NotNil(t, exportReport)
	assert.Equal(t, uint32(3), exportReport.TerminalResultCount)

	evaluationSummaryJSON, err := files.ReadFile(context.Background(), filepath.Join(outputDir, constants.CampaignExportEvaluationSummaryFilename))
	require.NoError(t, err)
	var evaluationSummary struct {
		SchemaVersion   string `json:"schema_version"`
		Kind            string `json:"kind"`
		VerifierState   string `json:"verifier_state"`
		HeadlineMetrics struct {
			PassRate struct {
				Unit             string `json:"unit"`
				ObservedCount    uint32 `json:"observed_count"`
				EligibleCount    uint32 `json:"eligible_count"`
				UnavailableCount uint32 `json:"unavailable_count"`
			} `json:"pass_rate"`
			LatencyP50MS struct {
				Unit              string `json:"unit"`
				UnavailableReason string `json:"unavailable_reason"`
			} `json:"latency_p50_ms"`
			OutputThroughputP50 struct {
				Unit              string `json:"unit"`
				UnavailableReason string `json:"unavailable_reason"`
			} `json:"output_throughput_p50_tokens_per_second"`
		} `json:"headline_metrics"`
		VerificationMetadata struct {
			ReportDigest     string `json:"report_digest"`
			PopulationDigest string `json:"population_digest"`
		} `json:"verification_metadata"`
	}
	require.NoError(t, json.Unmarshal(evaluationSummaryJSON, &evaluationSummary))
	assert.JSONEq(t, string(expectedEvaluationSummaryBody), string(evaluationSummaryJSON))
	assert.Equal(t, "1.5.0", evaluationSummary.SchemaVersion)
	assert.Equal(t, "evaluation_summary", evaluationSummary.Kind)
	assert.Equal(t, "passed", evaluationSummary.VerifierState)
	assert.Equal(t, "ratio", evaluationSummary.HeadlineMetrics.PassRate.Unit)
	assert.Equal(t, uint32(3), evaluationSummary.HeadlineMetrics.PassRate.ObservedCount)
	assert.Equal(t, uint32(3), evaluationSummary.HeadlineMetrics.PassRate.EligibleCount)
	assert.Equal(t, uint32(0), evaluationSummary.HeadlineMetrics.PassRate.UnavailableCount)
	assert.Equal(t, "milliseconds", evaluationSummary.HeadlineMetrics.LatencyP50MS.Unit)
	assert.Equal(t, "no_scored_calls", evaluationSummary.HeadlineMetrics.LatencyP50MS.UnavailableReason)
	assert.Equal(t, "tokens_per_second", evaluationSummary.HeadlineMetrics.OutputThroughputP50.Unit)
	assert.Equal(t, "no_scored_calls", evaluationSummary.HeadlineMetrics.OutputThroughputP50.UnavailableReason)
	assert.Equal(t, report.GetReportDigestRef().GetSha256(), evaluationSummary.VerificationMetadata.ReportDigest)
	assert.Equal(t, report.GetVerifiedPopulationDigest(), evaluationSummary.VerificationMetadata.PopulationDigest)

	modelSummariesJSONL, err := files.ReadFile(context.Background(), filepath.Join(outputDir, "model_summaries.jsonl"))
	require.NoError(t, err)
	modelLines := strings.Split(strings.TrimSpace(string(modelSummariesJSONL)), "\n")
	require.Len(t, modelLines, 3)
	for _, line := range modelLines {
		var ms modelSummaryRecord
		require.NoError(t, json.Unmarshal([]byte(line), &ms))
		assert.Equal(t, "exploratory_verified", ms.QualityState)
		assert.Equal(t, CampaignDatasetID(req.RunID), ms.DatasetID)
	}

	assignmentsJSONL, err := files.ReadFile(context.Background(), filepath.Join(outputDir, "assignments.jsonl"))
	require.NoError(t, err)
	assignmentLines := strings.Split(strings.TrimSpace(string(assignmentsJSONL)), "\n")
	require.Len(t, assignmentLines, 3)
	for _, line := range assignmentLines {
		var ar CampaignExportAssignmentRecord
		require.NoError(t, json.Unmarshal([]byte(line), &ar))
		assert.Equal(t, "verified", ar.Projection.GetVerificationStatus())
		require.NotNil(t, ar.Projection.GetVerificationMetadata())
		assert.Equal(t, evalv1.PublicVerificationProvenance_PUBLIC_VERIFICATION_PROVENANCE_BOUND, ar.Projection.GetVerificationMetadata().GetProvenance())
		assert.Equal(t, evalv1.EvaluationVerdictStatus_EVALUATION_VERDICT_STATUS_PASS, ar.Projection.GetVerificationMetadata().GetVerifierState())
	}
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
					GenerationMS: publicMetricValue(42.5),
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
