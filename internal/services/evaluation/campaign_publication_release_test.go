// Copyright (c) 2026 Lateralus Labs, LLC.
// Licensed under the Business Source License 1.1 — see LICENSE for details.

package evaluation

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/models"
	evalv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/eval/v1"
)

func savePublicRunIdentity(t *testing.T, store *Store, runID, campaignID string) {
	t.Helper()
	req := testCampaignInitRequest(t)
	spec, err := MaterializeCampaignSpec(campaignID, req.Catalog, req.Inventory, req.RepetitionCount, testPlatform)
	require.NoError(t, err)
	require.NoError(t, store.SaveCampaignSpec(context.Background(), spec))
	require.NoError(t, store.SaveRun(context.Background(), &evalv1.EvaluationRun{SchemaVersion: CampaignSchemaVersion, RunId: runID, CampaignBinding: &evalv1.ModelCampaignBinding{CampaignId: campaignID}}))
}

func TestCampaignPublication_ReleaseProvenanceReachesEveryPublicRecord(t *testing.T) {
	for _, tc := range []struct {
		name, release, basis, revision string
		legacy                         bool
	}{
		{name: "recorded", release: testPlatform.Release, basis: "recorded", revision: testPlatform.SourceRevision},
		{name: "asserted", release: "v2.2.7", basis: "asserted", legacy: true},
		{name: "unknown", basis: "unknown", legacy: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			f := newArchiveFixture(t)
			runID := f.runIDs[0]
			if tc.legacy {
				saveLegacyCampaign(t, f, "legacy-public")
				run, err := f.controller.StartRun(ctx, RunStartRequest{CampaignID: "legacy-public", RunID: "legacy-run", PlatformRelease: testPlatform.Release})
				require.NoError(t, err)
				runID = run.GetRunId()
				_, err = f.controller.ScheduleHomogeneousRun(ctx, runID)
				require.NoError(t, err)
				if tc.release != "" {
					require.NoError(t, f.store.TagCampaignRelease(ctx, "legacy-public", tc.release, archiveTestNow))
				}
			}
			exporter := &recordingCampaignFeedExporter{}
			coordinator := NewCampaignPublicationCoordinator(f.store, f.files, NewMemoryCampaignPublicationStateStore(), exporter, nil)
			f.controller.WithPublication(coordinator)
			_, err := coordinator.PublishRunCatchUp(ctx, runID)
			require.NoError(t, err)
			for range 3 {
				_, _, err = f.controller.ExecuteNextAssignment(ctx, runID, CampaignExecutionBinding{
					InferenceOperatorSessionID: "inf-session", DataOperatorID: "data-op", DataOperatorSessionID: "data-session",
					ModelRegistryDigest: f.req.Inventory.RegistryDigest, ModelRegistry: InferenceVariantsFromEvalRegistry(f.req.Inventory.Variants),
				}, f.req.ScenarioArtifacts)
				require.NoError(t, err)
			}
			_, err = coordinator.PublishRunCompletion(ctx, runID, archiveTestNow)
			require.NoError(t, err)
			run, err := f.store.LoadRun(ctx, runID)
			require.NoError(t, err)
			report := buildBoundTestReport(t, f.store, run, evalv1.EvaluationVerdictStatus_EVALUATION_VERDICT_STATUS_PASS)
			_, err = coordinator.PublishRunVerification(ctx, runID, report)
			require.NoError(t, err)
			kinds := map[string]bool{}
			for _, published := range exporter.records {
				var body struct {
					Kind        string          `json:"kind"`
					MessageType string          `json:"message_type"`
					Record      json.RawMessage `json:"record"`
				}
				require.NoError(t, json.Unmarshal([]byte(published.RecordBytes), &body))
				payload := []byte(published.RecordBytes)
				basis := tc.basis
				// Protobuf-owned records (assignment messages and live events)
				// carry the enum name; only the view snapshots keep the lowercase form.
				if body.MessageType != "" {
					payload = body.Record
					body.Kind = body.MessageType
				}
				if body.MessageType != "" || published.RecordType == models.PublicFeedRecordTypeEvent {
					basis = "PUBLIC_RELEASE_BASIS_" + map[string]string{"recorded": "RECORDED", "asserted": "ASSERTED", "unknown": "UNKNOWN"}[tc.basis]
				}
				var identity struct {
					Release        string `json:"release"`
					Basis          string `json:"release_basis"`
					SourceRevision string `json:"source_revision"`
				}
				require.NoError(t, json.Unmarshal(payload, &identity))
				assert.Equal(t, tc.release, identity.Release, body.Kind)
				assert.Equal(t, basis, identity.Basis, body.Kind)
				assert.Equal(t, tc.revision, identity.SourceRevision, body.Kind)
				kinds[body.Kind] = true
			}
			for _, kind := range []string{publicMessageTypeAssignmentLifecycle, publicMessageTypeAssignmentResult, "catalog_snapshot", "evaluation_summary", "model_summary", "methodology_snapshot", "stage_updated", "metric_updated"} {
				assert.True(t, kinds[kind], "missing public record %s", kind)
			}
		})
	}
}

func TestCampaignPublication_RejectsMalformedReleaseTags(t *testing.T) {
	ctx := context.Background()
	f := newArchiveFixture(t)
	saveLegacyCampaign(t, f, "legacy-public")
	run, err := f.controller.StartRun(ctx, RunStartRequest{CampaignID: "legacy-public", RunID: "legacy-run", PlatformRelease: testPlatform.Release})
	require.NoError(t, err)
	_, err = f.controller.ScheduleHomogeneousRun(ctx, run.GetRunId())
	require.NoError(t, err)
	require.NoError(t, f.files.WriteFile(ctx, f.store.layout.campaignReleaseTagPath("legacy-public"), []byte(`{"campaign_id":"another","release":"v2.2.7"}`), constants.PermFilePrivate))
	exporter := &recordingCampaignFeedExporter{}
	coordinator := NewCampaignPublicationCoordinator(f.store, f.files, NewMemoryCampaignPublicationStateStore(), exporter, nil)
	_, err = coordinator.PublishRunCatchUp(ctx, run.GetRunId())
	require.ErrorIs(t, err, constants.ErrEvidenceArtifactMalformed)
	assert.Empty(t, exporter.records)
}
