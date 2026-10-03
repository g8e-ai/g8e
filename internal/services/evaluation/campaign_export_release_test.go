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
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/v2/internal/constants"
)

func TestCampaignExporter_ReleaseProvenanceMatchesTheCampaign(t *testing.T) {
	for _, tc := range []struct {
		name    string
		legacy  bool
		tag     string
		release string
		basis   ReleaseBasis
	}{
		{name: "recorded", release: testPlatform.Release, basis: ReleaseBasisRecorded},
		{name: "asserted", legacy: true, tag: "v2.2.7", release: "v2.2.7", basis: ReleaseBasisAsserted},
		{name: "unknown", legacy: true, basis: ReleaseBasisUnknown},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			f := newArchiveFixture(t)
			runID := f.runIDs[0]
			if tc.legacy {
				saveLegacyCampaign(t, f, "legacy-export")
				run, err := f.controller.StartRun(ctx, RunStartRequest{CampaignID: "legacy-export", RunID: "legacy-run", PlatformRelease: testPlatform.Release})
				require.NoError(t, err)
				runID = run.GetRunId()
				_, err = f.controller.ScheduleHomogeneousRun(ctx, runID)
				require.NoError(t, err)
				if tc.tag != "" {
					require.NoError(t, f.store.TagCampaignRelease(ctx, "legacy-export", tc.tag, archiveTestNow))
				}
			}
			run, err := f.store.LoadRun(ctx, runID)
			require.NoError(t, err)
			specBefore, err := f.store.LoadCampaignSpec(ctx, run.GetCampaignBinding().GetCampaignId())
			require.NoError(t, err)
			report, err := NewCampaignExporter(nil).ExportRun(ctx, f.store, f.files, runID, "")
			require.NoError(t, err)
			assert.Equal(t, tc.release, report.Release)
			assert.Equal(t, tc.basis, report.ReleaseBasis)
			body, err := f.files.ReadFile(ctx, filepath.Join(report.OutputDir, constants.EvaluationRunSummaryFilename))
			require.NoError(t, err)
			var summary struct {
				Release        string       `json:"release"`
				ReleaseBasis   ReleaseBasis `json:"release_basis"`
				SourceRevision string       `json:"source_revision"`
				SchemaVersion  string       `json:"schema_version"`
			}
			require.NoError(t, json.Unmarshal(body, &summary))
			assert.Equal(t, tc.release, summary.Release)
			assert.Equal(t, tc.basis, summary.ReleaseBasis)
			assert.Equal(t, specBefore.GetSourceRevision(), summary.SourceRevision)
			assert.Equal(t, "1.2.0", summary.SchemaVersion)
			require.Contains(t, string(body), `"release":`, "unknown releases remain explicit")

			db, err := sql.Open("sqlite", f.files.Resolve(filepath.Join(report.OutputDir, constants.EvaluationCampaignExportSQLiteFilename)))
			require.NoError(t, err)
			t.Cleanup(func() { require.NoError(t, db.Close()) })
			var release, basis, revision string
			require.NoError(t, db.QueryRow(`SELECT release, release_basis, source_revision FROM export_metadata`).Scan(&release, &basis, &revision))
			assert.Equal(t, tc.release, release)
			assert.Equal(t, string(tc.basis), basis)
			assert.Equal(t, specBefore.GetSourceRevision(), revision)
			specAfter, err := f.store.LoadCampaignSpec(ctx, run.GetCampaignBinding().GetCampaignId())
			require.NoError(t, err)
			assert.Equal(t, specBefore.GetCampaignDigest(), specAfter.GetCampaignDigest())
		})
	}
}

func TestCampaignExporter_RejectsMalformedReleaseTags(t *testing.T) {
	f := newArchiveFixture(t)
	ctx := context.Background()
	saveLegacyCampaign(t, f, "legacy-export")
	run, err := f.controller.StartRun(ctx, RunStartRequest{CampaignID: "legacy-export", RunID: "legacy-run", PlatformRelease: testPlatform.Release})
	require.NoError(t, err)
	require.NoError(t, f.files.WriteFile(ctx, f.store.layout.campaignReleaseTagPath("legacy-export"), []byte(`{"release":"v2.2.7","campaign_id":"wrong-campaign"}`), constants.PermFilePrivate))
	_, err = NewCampaignExporter(nil).ExportRun(ctx, f.store, f.files, run.GetRunId(), "")
	require.ErrorIs(t, err, constants.ErrEvidenceArtifactMalformed)
}
