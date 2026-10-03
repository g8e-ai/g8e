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
	"encoding/csv"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/testutil"
	evalv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/eval/v1"
)

func TestCampaignExporter_PassRatesExcludeUnjudgedFailuresInCSVAndSQLite(t *testing.T) {
	for _, tc := range []struct {
		name     string
		judged   uint32
		passed   uint32
		csvRate  string
		wantRate sql.NullFloat64
	}{
		{name: "one pass one miss and a provider failure", judged: 2, passed: 1, csvRate: "0.500000", wantRate: sql.NullFloat64{Float64: 0.5, Valid: true}},
		{name: "judged model misses preserve scored zero", judged: 2, csvRate: "0.000000", wantRate: sql.NullFloat64{Valid: true}},
		{name: "no model verdicts", csvRate: "", wantRate: sql.NullFloat64{}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			assignments := make([]*evalv1.EvaluationAssignment, 0, 3)
			results := make(map[string]*evalv1.EvaluationAssignmentResult)
			for i := uint32(0); i < 3; i++ {
				id := strconv.FormatUint(uint64(i), 10)
				assignments = append(assignments, homogeneousAssignment(id, "variant", evalv1.ModelCampaignRole_MODEL_CAMPAIGN_ROLE_PRIMARY))
				result := &evalv1.EvaluationAssignmentResult{AssignmentId: id, LifecycleStatus: evalv1.EvaluationAssignmentLifecycleStatus_EVALUATION_ASSIGNMENT_LIFECYCLE_STATUS_PROVIDER_FAILED}
				if i < tc.judged {
					result.LifecycleStatus = evalv1.EvaluationAssignmentLifecycleStatus_EVALUATION_ASSIGNMENT_LIFECYCLE_STATUS_COMPLETED
					status := evalv1.EvaluationVerdictStatus_EVALUATION_VERDICT_STATUS_FAIL
					if i < tc.passed {
						status = evalv1.EvaluationVerdictStatus_EVALUATION_VERDICT_STATUS_PASS
					}
					result.DeterministicGrades = []*evalv1.DeterministicGrade{{Basis: basisObservation, Status: status}}
				}
				results[id] = result
			}
			state, err := CollectRunAggregateState(assignments, results)
			require.NoError(t, err)
			summary := buildCampaignRunSummaryExport(&evalv1.EvaluationRun{RunId: "run"}, &evalv1.EvaluationCampaignSpec{}, CampaignRelease{Basis: ReleaseBasisUnknown}, &evalv1.EvaluationScenarioCatalog{}, archiveTestNow, &CampaignPopulationReport{}, state)
			assert.Equal(t, tc.judged, summary.JudgedAssignments)
			assert.Equal(t, uint32(3), summary.TerminalAssignments)
			body, err := buildModelSummariesCSV(state)
			require.NoError(t, err)
			rows, err := csv.NewReader(strings.NewReader(string(body))).ReadAll()
			require.NoError(t, err)
			require.Len(t, rows, 2)
			require.Contains(t, rows[0], "pass_rate_estimate")
			require.Contains(t, rows[0], "judged")
			for i, name := range rows[0] {
				if name == "pass_rate_estimate" {
					assert.Equal(t, tc.csvRate, rows[1][i])
				}
				if name == "judged" {
					assert.Equal(t, strconv.FormatUint(uint64(tc.judged), 10), rows[1][i])
				}
				if name == "evaluation_coverage" {
					assert.Equal(t, "1.000000", rows[1][i], "coverage still counts all terminal assignments")
				}
			}
			sqlitePath := filepath.Join(testutil.TempDir(t), constants.EvaluationCampaignExportSQLiteFilename)
			require.NoError(t, NewCampaignExporter(nil).writeSQLiteExport(context.Background(), sqlitePath,
				&evalv1.EvaluationRun{RunId: "run"}, &evalv1.EvaluationCampaignSpec{}, CampaignRelease{Basis: ReleaseBasisUnknown}, &evalv1.EvaluationScenarioCatalog{}, archiveTestNow,
				&CampaignPopulationReport{}, nil, state, nil))
			db, err := sql.Open("sqlite", sqlitePath)
			require.NoError(t, err)
			t.Cleanup(func() { require.NoError(t, db.Close()) })
			var rate sql.NullFloat64
			var judged uint32
			var coverage float64
			require.NoError(t, db.QueryRow(`SELECT pass_rate_estimate, judged, evaluation_coverage FROM model_summaries`).Scan(&rate, &judged, &coverage))
			assert.Equal(t, tc.wantRate, rate)
			assert.Equal(t, tc.judged, judged)
			assert.Equal(t, 1.0, coverage)
		})
	}
}
