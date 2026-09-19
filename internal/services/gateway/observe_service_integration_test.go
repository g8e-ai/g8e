// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

//go:build integration

package gateway

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/marshaler"
	"github.com/g8e-ai/g8e/v2/internal/models"
	"github.com/g8e-ai/g8e/v2/internal/testutil"
)

// seedRunProjection writes a run projection document owned by userID into the
// observe_runs collection. The document id is the run_id so DocGet can look it
// up directly.
func seedRunProjection(t *testing.T, svc *ObserveService, userID, runID string, observedAt time.Time, status models.RunLifecycleStatus) {
	t.Helper()
	proj := runProjection{
		UserID:         userID,
		SchemaVersion:  constants.ObserveAPIReadModelSchemaVersion,
		RunID:          runID,
		RunKind:        models.RunKindInvestigation,
		DisplayName:    "run " + runID,
		Status:         status,
		TotalTasks:     3,
		CompletedTasks: 1,
		HasReceipts:    true,
		EvidenceCount:  2,
		ObservedAt:     observedAt,
	}
	b, err := json.Marshal(proj)
	require.NoError(t, err)
	require.NoError(t, svc.docStore.DocSet(marshaler.CollectionName(constants.CollectionObserveRuns), runID, b))
}

func seedEvalProjection(t *testing.T, svc *ObserveService, userID, runID string, observedAt time.Time) {
	t.Helper()
	proj := evalProjection{
		UserID: userID,
		EvalDetail: models.EvalDetail{
			SchemaVersion:      constants.ObserveAPIReadModelSchemaVersion,
			RunID:              runID,
			SuiteID:            "ifeval_subset",
			SuiteVersion:       "1.0.0",
			CampaignID:         "",
			ArmIDs:             []string{"direct"},
			ModelCohortIDs:     []string{},
			Status:             models.RunLifecycleStatusCompleted,
			VerificationStatus: models.EvalVerificationProjectionValidated,
			ReceiptCount:       5,
			AssignedTasks:      10,
			TerminalAttempts:   10,
			Metrics:            []models.EvalMetricSummary{},
			ObservedAt:         observedAt,
		},
	}
	b, err := json.Marshal(proj)
	require.NoError(t, err)
	require.NoError(t, svc.docStore.DocSet(marshaler.CollectionName(constants.CollectionObserveEvals), runID, b))
}

func seedDownloadProjection(t *testing.T, svc *ObserveService, userID, artifactID string, generatedAt time.Time) {
	t.Helper()
	proj := downloadProjection{
		UserID: userID,
		DownloadArtifact: models.DownloadArtifact{
			SchemaVersion:         constants.ObserveAPIReadModelSchemaVersion,
			ArtifactID:            artifactID,
			Filename:              artifactID + ".jsonl",
			MediaType:             "application/jsonl",
			ByteSize:              1024,
			SHA256:                "abc123",
			PrivacyClassification: models.DownloadPrivacyPublicSafe,
			SourceRunID:           "eval-run-1",
			DownloadURL:           "/api/v1/observe/downloads/" + artifactID,
			GeneratedAt:           generatedAt,
		},
	}
	b, err := json.Marshal(proj)
	require.NoError(t, err)
	require.NoError(t, svc.docStore.DocSet(marshaler.CollectionName(constants.CollectionObserveDownloads), artifactID, b))
}

func seedAgentStateProjection(t *testing.T, svc *ObserveService, userID, agentID string, observedAt time.Time, status models.AgentLifecycleStatus) {
	t.Helper()
	proj := agentStateProjection{
		UserID: userID,
		AgentStateProjection: models.AgentStateProjection{
			SchemaVersion: constants.ObserveEventPayloadSchemaVersion,
			AgentID:       agentID,
			DisplayName:   "agent " + agentID,
			Role:          "triage",
			Status:        status,
			Freshness:     models.SnapshotFreshnessObserved,
			ObservedAt:    observedAt,
		},
	}
	b, err := json.Marshal(proj)
	require.NoError(t, err)
	require.NoError(t, svc.docStore.DocSet(marshaler.CollectionName(constants.CollectionObserveAgentStates), agentID, b))
}

// newObserveServiceWithRealStore creates an ObserveService backed by a real
// SQLite document store for integration testing.
func newObserveServiceWithRealStore(t *testing.T) *ObserveService {
	t.Helper()
	return NewObserveService(newDocumentStoreService(t), testutil.NewTestLogger())
}

func TestObserveService_GetBootstrapSnapshot_EmptyStateReturnsUnavailableFreshness(t *testing.T) {
	svc := newObserveServiceWithRealStore(t)

	snapshot, err := svc.GetBootstrapSnapshot(context.Background(), "user-empty")
	require.NoError(t, err)
	require.NotNil(t, snapshot)

	assert.Equal(t, constants.ObserveAPIReadModelSchemaVersion, snapshot.SchemaVersion)
	assert.Empty(t, snapshot.Agents)
	assert.Nil(t, snapshot.ActiveRun)
	assert.Empty(t, snapshot.RecentRuns)
	assert.Empty(t, snapshot.LatestEvals)
	assert.Empty(t, snapshot.Downloads)
	assert.Equal(t, 0, snapshot.Overview.AgentsRunning)
	assert.Equal(t, models.SnapshotFreshnessUnavailable, snapshot.Overview.AgentsRunningFreshness)
	assert.Equal(t, 0, snapshot.Overview.TasksInQueue)
	assert.Equal(t, models.SnapshotFreshnessUnavailable, snapshot.Overview.TasksInQueueFreshness)
	// Measurement cards remain unavailable until a real host telemetry collector exists.
	assert.Nil(t, snapshot.Measurements.TotalThroughput)
	assert.Nil(t, snapshot.Measurements.CPU)
}

func TestObserveService_GetRun_UnknownIDReturnsErrObserveRunNotFound(t *testing.T) {
	svc := newObserveServiceWithRealStore(t)

	_, err := svc.GetRun(context.Background(), "user-a", "nonexistent-run")
	require.Error(t, err)
	assert.True(t, errors.Is(err, constants.ErrObserveRunNotFound))
}

func TestObserveService_GetEval_UnknownIDReturnsErrObserveEvalNotFound(t *testing.T) {
	svc := newObserveServiceWithRealStore(t)

	_, err := svc.GetEval(context.Background(), "user-a", "nonexistent-eval")
	require.Error(t, err)
	assert.True(t, errors.Is(err, constants.ErrObserveEvalNotFound))
}

func TestObserveService_GetDownload_UnknownIDReturnsErrObserveDownloadNotFound(t *testing.T) {
	svc := newObserveServiceWithRealStore(t)

	_, err := svc.GetDownload(context.Background(), "user-a", "nonexistent-artifact")
	require.Error(t, err)
	assert.True(t, errors.Is(err, constants.ErrObserveDownloadNotFound))
}

func TestObserveService_CrossUserIsolation_RunDetail(t *testing.T) {
	svc := newObserveServiceWithRealStore(t)
	now := time.Now().UTC()

	seedRunProjection(t, svc, "user-a", "run-a-1", now, models.RunLifecycleStatusRunning)

	// user-a can read their own run.
	detail, err := svc.GetRun(context.Background(), "user-a", "run-a-1")
	require.NoError(t, err)
	require.NotNil(t, detail)
	assert.Equal(t, "run-a-1", detail.RunID)

	// user-b cannot read user-a's run (returns not found, not the record).
	_, err = svc.GetRun(context.Background(), "user-b", "run-a-1")
	require.Error(t, err)
	assert.True(t, errors.Is(err, constants.ErrObserveRunNotFound))
}

func TestObserveService_CrossUserIsolation_EvalDetail(t *testing.T) {
	svc := newObserveServiceWithRealStore(t)
	now := time.Now().UTC()

	seedEvalProjection(t, svc, "user-a", "eval-a-1", now)

	detail, err := svc.GetEval(context.Background(), "user-a", "eval-a-1")
	require.NoError(t, err)
	require.NotNil(t, detail)
	assert.Equal(t, "eval-a-1", detail.RunID)

	_, err = svc.GetEval(context.Background(), "user-b", "eval-a-1")
	require.Error(t, err)
	assert.True(t, errors.Is(err, constants.ErrObserveEvalNotFound))
}

func TestObserveService_CrossUserIsolation_DownloadDetail(t *testing.T) {
	svc := newObserveServiceWithRealStore(t)
	now := time.Now().UTC()

	seedDownloadProjection(t, svc, "user-a", "art-a-1", now)

	artifact, err := svc.GetDownload(context.Background(), "user-a", "art-a-1")
	require.NoError(t, err)
	require.NotNil(t, artifact)
	assert.Equal(t, "art-a-1", artifact.ArtifactID)

	_, err = svc.GetDownload(context.Background(), "user-b", "art-a-1")
	require.Error(t, err)
	assert.True(t, errors.Is(err, constants.ErrObserveDownloadNotFound))
}

func TestObserveService_CrossUserIsolation_ListRunsOnlyReturnsOwned(t *testing.T) {
	svc := newObserveServiceWithRealStore(t)
	base := time.Now().UTC()

	seedRunProjection(t, svc, "user-a", "run-a-1", base, models.RunLifecycleStatusRunning)
	seedRunProjection(t, svc, "user-a", "run-a-2", base.Add(-1*time.Hour), models.RunLifecycleStatusCompleted)
	seedRunProjection(t, svc, "user-b", "run-b-1", base, models.RunLifecycleStatusRunning)
	seedRunProjection(t, svc, "user-b", "run-b-2", base.Add(-2*time.Hour), models.RunLifecycleStatusFailed)

	pageA, err := svc.ListRuns(context.Background(), "user-a", "", 100)
	require.NoError(t, err)
	var runsA []models.RunSummary
	require.NoError(t, json.Unmarshal(pageA.Items, &runsA))
	require.Len(t, runsA, 2)
	for _, r := range runsA {
		assert.Contains(t, r.RunID, "run-a-", "user-a should only see their own runs")
	}

	pageB, err := svc.ListRuns(context.Background(), "user-b", "", 100)
	require.NoError(t, err)
	var runsB []models.RunSummary
	require.NoError(t, json.Unmarshal(pageB.Items, &runsB))
	require.Len(t, runsB, 2)
	for _, r := range runsB {
		assert.Contains(t, r.RunID, "run-b-", "user-b should only see their own runs")
	}
}

func TestObserveService_PaginationCursorAdvancesAcrossMultiPageDataset(t *testing.T) {
	svc := newObserveServiceWithRealStore(t)
	base := time.Now().UTC()

	// Seed 5 runs with distinct observed_at timestamps (DESC order: run-5 newest).
	for i := 1; i <= 5; i++ {
		seedRunProjection(t, svc, "user-p", "run-p-"+string(rune('0'+i)), base.Add(-time.Duration(i)*time.Hour), models.RunLifecycleStatusCompleted)
	}

	// Page 1: limit=2, no cursor.
	page1, err := svc.ListRuns(context.Background(), "user-p", "", 2)
	require.NoError(t, err)
	assert.True(t, page1.HasMore)
	assert.NotEmpty(t, page1.Cursor)
	var items1 []models.RunSummary
	require.NoError(t, json.Unmarshal(page1.Items, &items1))
	require.Len(t, items1, 2)
	// Newest two first (DESC by observed_at).
	assert.Equal(t, "run-p-1", items1[0].RunID)
	assert.Equal(t, "run-p-2", items1[1].RunID)

	// Page 2: use cursor from page 1.
	page2, err := svc.ListRuns(context.Background(), "user-p", page1.Cursor, 2)
	require.NoError(t, err)
	assert.True(t, page2.HasMore)
	assert.NotEmpty(t, page2.Cursor)
	var items2 []models.RunSummary
	require.NoError(t, json.Unmarshal(page2.Items, &items2))
	require.Len(t, items2, 2)
	assert.Equal(t, "run-p-3", items2[0].RunID)
	assert.Equal(t, "run-p-4", items2[1].RunID)

	// Page 3: final page.
	page3, err := svc.ListRuns(context.Background(), "user-p", page2.Cursor, 2)
	require.NoError(t, err)
	assert.False(t, page3.HasMore)
	assert.Empty(t, page3.Cursor)
	var items3 []models.RunSummary
	require.NoError(t, json.Unmarshal(page3.Items, &items3))
	require.Len(t, items3, 1)
	assert.Equal(t, "run-p-5", items3[0].RunID)
}

func TestObserveService_LimitBoundsEnforced(t *testing.T) {
	svc := newObserveServiceWithRealStore(t)
	base := time.Now().UTC()

	for i := 1; i <= 10; i++ {
		seedRunProjection(t, svc, "user-l", "run-l-"+string(rune('0'+i)), base.Add(-time.Duration(i)*time.Minute), models.RunLifecycleStatusCompleted)
	}

	// limit=0 defaults to ObserveDefaultLimit (20), returns all 10.
	page, err := svc.ListRuns(context.Background(), "user-l", "", 0)
	require.NoError(t, err)
	var items []models.RunSummary
	require.NoError(t, json.Unmarshal(page.Items, &items))
	assert.Len(t, items, 10)
	assert.False(t, page.HasMore)

	// limit=3 returns exactly 3 with has_more.
	page3, err := svc.ListRuns(context.Background(), "user-l", "", 3)
	require.NoError(t, err)
	var items3 []models.RunSummary
	require.NoError(t, json.Unmarshal(page3.Items, &items3))
	assert.Len(t, items3, 3)
	assert.True(t, page3.HasMore)

	// limit=101 clamped to ObserveMaxLimit (100), returns all 10.
	pageBig, err := svc.ListRuns(context.Background(), "user-l", "", 101)
	require.NoError(t, err)
	var itemsBig []models.RunSummary
	require.NoError(t, json.Unmarshal(pageBig.Items, &itemsBig))
	assert.Len(t, itemsBig, 10)
	assert.False(t, pageBig.HasMore)
}

func TestObserveService_GetBootstrapSnapshot_PopulatedStateReturnsObservedFreshness(t *testing.T) {
	svc := newObserveServiceWithRealStore(t)
	now := time.Now().UTC()

	seedAgentStateProjection(t, svc, "user-bs", "agent-1", now, models.AgentLifecycleStatusRunning)
	seedAgentStateProjection(t, svc, "user-bs", "agent-2", now, models.AgentLifecycleStatusIdle)
	seedRunProjection(t, svc, "user-bs", "run-bs-1", now, models.RunLifecycleStatusRunning)
	seedRunProjection(t, svc, "user-bs", "run-bs-2", now.Add(-1*time.Hour), models.RunLifecycleStatusCompleted)
	seedEvalProjection(t, svc, "user-bs", "eval-bs-1", now.Add(-30*time.Minute))
	seedDownloadProjection(t, svc, "user-bs", "art-bs-1", now.Add(-15*time.Minute))

	snapshot, err := svc.GetBootstrapSnapshot(context.Background(), "user-bs")
	require.NoError(t, err)
	require.NotNil(t, snapshot)

	assert.Len(t, snapshot.Agents, 2)
	require.NotNil(t, snapshot.ActiveRun)
	assert.Equal(t, "run-bs-1", snapshot.ActiveRun.RunID)
	assert.Equal(t, 1, snapshot.Overview.AgentsRunning, "one agent running, one idle")
	assert.Equal(t, models.SnapshotFreshnessObserved, snapshot.Overview.AgentsRunningFreshness)
	assert.Equal(t, models.SnapshotFreshnessObserved, snapshot.Overview.TasksInQueueFreshness)
	// run-bs-1 is running with 3 total, 1 completed => 2 in queue.
	assert.Equal(t, 2, snapshot.Overview.TasksInQueue)
	require.Len(t, snapshot.RecentRuns, 2)
	require.Len(t, snapshot.LatestEvals, 1)
	require.Len(t, snapshot.Downloads, 1)
}

func TestObserveService_GetRun_OwnedRunStripsUserID(t *testing.T) {
	svc := newObserveServiceWithRealStore(t)
	now := time.Now().UTC()

	seedRunProjection(t, svc, "user-s", "run-s-1", now, models.RunLifecycleStatusCompleted)

	detail, err := svc.GetRun(context.Background(), "user-s", "run-s-1")
	require.NoError(t, err)
	require.NotNil(t, detail)

	// The wire model must not expose the user_id ownership field.
	b, err := json.Marshal(detail)
	require.NoError(t, err)
	assert.NotContains(t, string(b), "user_id", "RunDetail wire model must strip user_id")
}
