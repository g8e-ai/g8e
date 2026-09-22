// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

//go:build integration

package gateway_test

import (
	"bufio"
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/models"
	"github.com/g8e-ai/g8e/v2/internal/services/evaluation"
	"github.com/g8e-ai/g8e/v2/internal/services/fs"
	"github.com/g8e-ai/g8e/v2/internal/services/gateway"
	"github.com/g8e-ai/g8e/v2/internal/testutil"
	compliancev1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/compliance/v1"
	evalv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/eval/v1"
)

func newPublicCampaignFileService(t *testing.T) fs.RuntimeFileService {
	t.Helper()
	fileSvc, err := fs.NewRuntimeFileService(testutil.TempDir(t), testutil.NewTestLogger())
	require.NoError(t, err)
	require.NoError(t, fileSvc.CreateRuntimeTree(context.Background()))
	return fileSvc
}

func TestCampaignEvaluationSummaryPublishesThroughSignedMirrorReconstruction(t *testing.T) {
	ctx := context.Background()
	publicKey, privateKey, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)
	exportConfig := models.PublicExportConfig{
		Enabled:                 true,
		MirrorOrigin:            "http://test-mirror.invalid",
		SourceID:                "test-source-1",
		SigningKeyID:            "test-key-1",
		BatchMaxRecords:         constants.PublicFeedBatchMaxRecords,
		BatchMaxBytes:           constants.PublicFeedBatchMaxBytes,
		RetryMaxAttempts:        constants.PublicFeedRetryMaxAttempts,
		RetryInitialBackoffSecs: 0,
		RetryMaxBackoffSecs:     1,
		AckWindowSecs:           constants.PublicFeedAckWindowSeconds,
	}
	publisher := gateway.NewPublicPublisherService(nil, newPublicCampaignFileService(t), testutil.NewTestLogger(), exportConfig, privateKey, exportConfig.SigningKeyID)
	mirror, err := gateway.NewPublicMirrorServer(testutil.NewTestLogger(), gateway.NewRuntimePublicMirrorStore(newPublicCampaignFileService(t)), gateway.PublicMirrorConfig{})
	require.NoError(t, err)
	require.NoError(t, mirror.RegisterSourceKey(ctx, exportConfig.SourceID, exportConfig.SigningKeyID, publicKey))
	server := httptest.NewServer(mirror.Handler())
	t.Cleanup(server.Close)
	publisher.SetMirrorOrigin(server.URL)

	spanNanos := uint64(200_000_000)
	assignment := &evalv1.EvaluationAssignment{
		AssignmentId:    "assignment-1",
		RunId:           "run-1",
		ScenarioId:      "instruction-exact-format",
		Lane:            evalv1.EvaluationLane_EVALUATION_LANE_MODEL_ROLE,
		LifecycleStatus: evalv1.EvaluationAssignmentLifecycleStatus_EVALUATION_ASSIGNMENT_LIFECYCLE_STATUS_COMPLETED,
		Target: &evalv1.EvaluationAssignment_Homogeneous{Homogeneous: &evalv1.HomogeneousAssignmentTarget{
			CandidateVariant: &evalv1.ModelVariant{VariantId: "qwen3-4b"},
			DesignatedRole:   evalv1.ModelCampaignRole_MODEL_CAMPAIGN_ROLE_PRIMARY,
		}},
		QueuedAt: timestamppb.New(time.Unix(1_700_000_000, 0).UTC()),
	}
	result := &evalv1.EvaluationAssignmentResult{
		AssignmentId:             assignment.GetAssignmentId(),
		RunId:                    assignment.GetRunId(),
		LifecycleStatus:          evalv1.EvaluationAssignmentLifecycleStatus_EVALUATION_ASSIGNMENT_LIFECYCLE_STATUS_COMPLETED,
		ScoredInferenceSpanNanos: &spanNanos,
		DeterministicGrades:      []*evalv1.DeterministicGrade{{Status: evalv1.EvaluationVerdictStatus_EVALUATION_VERDICT_STATUS_PASS}},
		ModelInferences: []*evalv1.ModelInferenceRecord{{
			InferenceRecordId:       "inference-1",
			UsageAvailability:       evalv1.EvaluationUsageAvailability_EVALUATION_USAGE_AVAILABILITY_REPORTED,
			CompletionTokens:        10,
			GenerationDurationNanos: 1_000_000_000,
		}},
	}
	assignmentWithoutMetrics := &evalv1.EvaluationAssignment{
		AssignmentId:    "assignment-2",
		RunId:           assignment.GetRunId(),
		ScenarioId:      assignment.GetScenarioId(),
		Lane:            assignment.GetLane(),
		LifecycleStatus: evalv1.EvaluationAssignmentLifecycleStatus_EVALUATION_ASSIGNMENT_LIFECYCLE_STATUS_COMPLETED,
		Target: &evalv1.EvaluationAssignment_Homogeneous{Homogeneous: &evalv1.HomogeneousAssignmentTarget{
			CandidateVariant: &evalv1.ModelVariant{VariantId: "qwen3-4b"},
			DesignatedRole:   evalv1.ModelCampaignRole_MODEL_CAMPAIGN_ROLE_PRIMARY,
		}},
		QueuedAt: timestamppb.New(time.Unix(1_700_000_001, 0).UTC()),
	}
	resultWithoutMetrics := &evalv1.EvaluationAssignmentResult{
		AssignmentId:        assignmentWithoutMetrics.GetAssignmentId(),
		RunId:               assignmentWithoutMetrics.GetRunId(),
		LifecycleStatus:     evalv1.EvaluationAssignmentLifecycleStatus_EVALUATION_ASSIGNMENT_LIFECYCLE_STATUS_COMPLETED,
		DeterministicGrades: []*evalv1.DeterministicGrade{{Status: evalv1.EvaluationVerdictStatus_EVALUATION_VERDICT_STATUS_PASS}},
		ModelInferences:     []*evalv1.ModelInferenceRecord{{InferenceRecordId: "inference-2"}},
	}
	aggregate, err := evaluation.CollectRunAggregateState(
		[]*evalv1.EvaluationAssignment{assignment, assignmentWithoutMetrics},
		map[string]*evalv1.EvaluationAssignmentResult{
			assignment.GetAssignmentId():               result,
			assignmentWithoutMetrics.GetAssignmentId(): resultWithoutMetrics,
		},
	)
	require.NoError(t, err)

	campaignDigest := strings.Repeat("c", 64)
	catalogDigest := strings.Repeat("d", 64)
	registryDigest := strings.Repeat("e", 64)
	run := &evalv1.EvaluationRun{
		RunId:     assignment.GetRunId(),
		StartedAt: timestamppb.New(time.Unix(1_700_000_000, 0).UTC()),
		CampaignBinding: &evalv1.ModelCampaignBinding{
			CampaignId:          "campaign-1",
			CampaignDigest:      campaignDigest,
			CatalogDigest:       catalogDigest,
			ModelRegistryDigest: registryDigest,
		},
	}
	reportDigest := strings.Repeat("a", 64)
	populationDigest := strings.Repeat("b", 64)
	report := &evalv1.EvaluationVerificationReport{
		SchemaVersion:            "2.0.0",
		ReportId:                 run.GetRunId(),
		RunId:                    run.GetRunId(),
		Status:                   evalv1.EvaluationVerdictStatus_EVALUATION_VERDICT_STATUS_PASS,
		ReportDigestRef:          &compliancev1.ComplianceEvidenceReference{Sha256: reportDigest},
		VerifiedAt:               timestamppb.New(time.Unix(1_700_000_100, 0).UTC()),
		VerifierReleaseVersion:   "v2.1.12",
		VerifierContractVersion:  "2.0.0",
		VerifiedPopulationDigest: populationDigest,
		ExpectedAssignmentCount:  2,
		VerifiedAssignmentCount:  2,
		CampaignDigest:           campaignDigest,
		CatalogDigest:            catalogDigest,
		ModelRegistryDigest:      registryDigest,
	}
	viewRecords, err := evaluation.BuildRunAggregateViewRecords(run, aggregate, report, time.Unix(1_700_000_100, 0).UTC())
	require.NoError(t, err)
	feedRecords := make([]models.PublicFeedRecord, len(viewRecords))
	for index, record := range viewRecords {
		digest := sha256.Sum256(record.Body)
		feedRecords[index] = models.PublicFeedRecord{
			Sequence:    int64(index + 1),
			RecordType:  models.PublicFeedRecordTypeProjection,
			RecordHash:  hex.EncodeToString(digest[:]),
			RecordBytes: string(record.Body),
		}
	}
	require.NoError(t, publisher.ExportBatch(ctx, feedRecords))

	var bootstrap models.PublicFeedBootstrap
	response, err := server.Client().Get(server.URL + "/bootstrap?source=" + exportConfig.SourceID)
	require.NoError(t, err)
	require.NoError(t, json.NewDecoder(response.Body).Decode(&bootstrap))
	require.NoError(t, response.Body.Close())
	assert.Equal(t, int64(len(viewRecords)), bootstrap.Snapshot.HighWaterSequence)

	var history struct {
		Items []struct {
			SchemaVersion   string `json:"schema_version"`
			Kind            string `json:"kind"`
			VerifierState   string `json:"verifier_state"`
			HeadlineMetrics struct {
				PassRate struct {
					Value             float64 `json:"value"`
					Unit              string  `json:"unit"`
					ObservedCount     uint32  `json:"observed_count"`
					EligibleCount     uint32  `json:"eligible_count"`
					UnavailableCount  uint32  `json:"unavailable_count"`
					UnavailableReason string  `json:"unavailable_reason"`
				} `json:"pass_rate"`
				LatencyP50MS struct {
					Value             float64 `json:"value"`
					Unit              string  `json:"unit"`
					ObservedCount     uint32  `json:"observed_count"`
					EligibleCount     uint32  `json:"eligible_count"`
					UnavailableCount  uint32  `json:"unavailable_count"`
					UnavailableReason string  `json:"unavailable_reason"`
				} `json:"latency_p50_ms"`
				OutputThroughputP50 struct {
					Value             float64 `json:"value"`
					Unit              string  `json:"unit"`
					ObservedCount     uint32  `json:"observed_count"`
					EligibleCount     uint32  `json:"eligible_count"`
					UnavailableCount  uint32  `json:"unavailable_count"`
					UnavailableReason string  `json:"unavailable_reason"`
				} `json:"output_throughput_p50_tokens_per_second"`
			} `json:"headline_metrics"`
			VerificationMetadata struct {
				ReportDigest     string `json:"report_digest"`
				PopulationDigest string `json:"population_digest"`
			} `json:"verification_metadata"`
		} `json:"items"`
	}
	response, err = server.Client().Get(server.URL + "/history?source=" + exportConfig.SourceID + "&kind=evaluation_summary&limit=10")
	require.NoError(t, err)
	require.NoError(t, json.NewDecoder(response.Body).Decode(&history))
	require.NoError(t, response.Body.Close())
	require.Len(t, history.Items, 1)
	summary := history.Items[0]
	assert.Equal(t, "1.5.0", summary.SchemaVersion)
	assert.Equal(t, "evaluation_summary", summary.Kind)
	assert.Equal(t, "passed", summary.VerifierState)
	assert.Equal(t, 1.0, summary.HeadlineMetrics.PassRate.Value)
	assert.Equal(t, "ratio", summary.HeadlineMetrics.PassRate.Unit)
	assert.Equal(t, uint32(2), summary.HeadlineMetrics.PassRate.ObservedCount)
	assert.Equal(t, uint32(2), summary.HeadlineMetrics.PassRate.EligibleCount)
	assert.Zero(t, summary.HeadlineMetrics.PassRate.UnavailableCount)
	assert.Empty(t, summary.HeadlineMetrics.PassRate.UnavailableReason)
	assert.Equal(t, 200.0, summary.HeadlineMetrics.LatencyP50MS.Value)
	assert.Equal(t, "milliseconds", summary.HeadlineMetrics.LatencyP50MS.Unit)
	assert.Equal(t, uint32(1), summary.HeadlineMetrics.LatencyP50MS.ObservedCount)
	assert.Equal(t, uint32(2), summary.HeadlineMetrics.LatencyP50MS.EligibleCount)
	assert.Equal(t, uint32(1), summary.HeadlineMetrics.LatencyP50MS.UnavailableCount)
	assert.Empty(t, summary.HeadlineMetrics.LatencyP50MS.UnavailableReason)
	assert.Equal(t, 10.0, summary.HeadlineMetrics.OutputThroughputP50.Value)
	assert.Equal(t, "tokens_per_second", summary.HeadlineMetrics.OutputThroughputP50.Unit)
	assert.Equal(t, uint32(1), summary.HeadlineMetrics.OutputThroughputP50.ObservedCount)
	assert.Equal(t, uint32(2), summary.HeadlineMetrics.OutputThroughputP50.EligibleCount)
	assert.Equal(t, uint32(1), summary.HeadlineMetrics.OutputThroughputP50.UnavailableCount)
	assert.Empty(t, summary.HeadlineMetrics.OutputThroughputP50.UnavailableReason)
	assert.Equal(t, reportDigest, summary.VerificationMetadata.ReportDigest)
	assert.Equal(t, populationDigest, summary.VerificationMetadata.PopulationDigest)

	var snapshot models.PublicFeedSnapshot
	response, err = server.Client().Get(server.URL + "/snapshot?source=" + exportConfig.SourceID)
	require.NoError(t, err)
	require.NoError(t, json.NewDecoder(response.Body).Decode(&snapshot))
	require.NoError(t, response.Body.Close())
	assert.Equal(t, int64(len(viewRecords)), snapshot.HighWaterSequence)
	assert.NotEqual(t, constants.PublicFeedZeroHashHex, snapshot.FeedChainHash)

	streamContext, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	streamRequest, err := http.NewRequestWithContext(streamContext, http.MethodGet, server.URL+"/stream?source="+exportConfig.SourceID+"&since_id=0", nil)
	require.NoError(t, err)
	streamResponse, err := server.Client().Do(streamRequest)
	require.NoError(t, err)
	defer streamResponse.Body.Close()
	scanner := bufio.NewScanner(streamResponse.Body)
	eventCount := 0
	for scanner.Scan() {
		if strings.HasPrefix(scanner.Text(), "event: ") {
			eventCount++
		}
		if eventCount >= len(viewRecords)+1 {
			break
		}
	}
	assert.GreaterOrEqual(t, eventCount, len(viewRecords)+1)
}
