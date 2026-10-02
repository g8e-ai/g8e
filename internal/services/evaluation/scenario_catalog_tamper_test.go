// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package evaluation

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"

	evalv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/eval/v1"
)

// cloneBuiltinCatalog returns a mutable copy of the built-in catalog and its
// artifacts.
func cloneBuiltinCatalog(t *testing.T) (*evalv1.EvaluationScenarioCatalog, map[string]ScenarioArtifacts) {
	t.Helper()
	catalog, artifacts, err := BuildScenarioCatalog()
	require.NoError(t, err)
	clone, ok := proto.Clone(catalog).(*evalv1.EvaluationScenarioCatalog)
	require.True(t, ok)
	copied := make(map[string]ScenarioArtifacts, len(artifacts))
	for id, pair := range artifacts {
		pair.Input.Body = append([]byte(nil), pair.Input.Body...)
		pair.Gold.Body = append([]byte(nil), pair.Gold.Body...)
		copied[id] = pair
	}
	return clone, copied
}

// resealCatalog recomputes the catalog digest so a tamper test reaches the
// check it targets instead of failing on the digest first.
func resealCatalog(t *testing.T, catalog *evalv1.EvaluationScenarioCatalog) {
	t.Helper()
	digest, err := ComputeScenarioCatalogDigest(catalog)
	require.NoError(t, err)
	catalog.CatalogDigest = digest
}

func TestValidateScenarioCatalog_AcceptsTheBuiltinCatalog(t *testing.T) {
	t.Parallel()
	catalog, artifacts := cloneBuiltinCatalog(t)
	require.NoError(t, ValidateScenarioCatalog(catalog, artifacts))
}

func TestValidateScenarioCatalog_RejectsTamperedCatalogs(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		mutate  func(catalog *evalv1.EvaluationScenarioCatalog, artifacts map[string]ScenarioArtifacts)
		reseal  bool
		wantErr string
	}{
		{name: "nil catalog", mutate: nil, wantErr: ""},
		{name: "a changed scenario without a new digest", mutate: func(c *evalv1.EvaluationScenarioCatalog, _ map[string]ScenarioArtifacts) {
			c.Scenarios[0].PublicDescription += " edited"
		}, wantErr: "digest mismatch"},
		{name: "an unsupported schema version", mutate: func(c *evalv1.EvaluationScenarioCatalog, _ map[string]ScenarioArtifacts) {
			c.SchemaVersion = "0.0.1"
		}, wantErr: "unsupported schema version"},
		{name: "a missing catalog identity", mutate: func(c *evalv1.EvaluationScenarioCatalog, _ map[string]ScenarioArtifacts) {
			c.CatalogRef.Version = ""
		}, wantErr: "catalog identity"},
		{name: "a duplicate scenario", reseal: true, mutate: func(c *evalv1.EvaluationScenarioCatalog, _ map[string]ScenarioArtifacts) {
			c.Scenarios = append(c.Scenarios, proto.Clone(c.Scenarios[0]).(*evalv1.EvaluationScenarioDefinition))
		}, wantErr: "duplicate scenario"},
		{name: "an unspecified category", reseal: true, mutate: func(c *evalv1.EvaluationScenarioCatalog, _ map[string]ScenarioArtifacts) {
			c.Scenarios[0].Category = evalv1.EvaluationScenarioCategory_EVALUATION_SCENARIO_CATEGORY_UNSPECIFIED
		}, wantErr: "unspecified category"},
		{name: "a missing public description", reseal: true, mutate: func(c *evalv1.EvaluationScenarioCatalog, _ map[string]ScenarioArtifacts) {
			c.Scenarios[0].PublicDescription = ""
		}, wantErr: "missing public description"},
		{name: "an unspecified grading method", reseal: true, mutate: func(c *evalv1.EvaluationScenarioCatalog, _ map[string]ScenarioArtifacts) {
			c.Scenarios[0].GradingMethod = evalv1.EvaluationGradingMethod_EVALUATION_GRADING_METHOD_UNSPECIFIED
		}, wantErr: "unspecified grading method"},
		{name: "missing fixture references", reseal: true, mutate: func(c *evalv1.EvaluationScenarioCatalog, _ map[string]ScenarioArtifacts) {
			c.Scenarios[0].InputFixtureRef = nil
		}, wantErr: "missing fixture references"},
		{name: "a scenario with no artifacts", mutate: func(c *evalv1.EvaluationScenarioCatalog, a map[string]ScenarioArtifacts) {
			delete(a, c.Scenarios[0].GetScenarioId())
		}, wantErr: "missing artifacts"},
		{name: "an input fixture swapped for another scenario's", mutate: func(c *evalv1.EvaluationScenarioCatalog, a map[string]ScenarioArtifacts) {
			first, second := c.Scenarios[0].GetScenarioId(), c.Scenarios[1].GetScenarioId()
			pair := a[first]
			pair.Input = a[second].Input
			a[first] = pair
		}, wantErr: "input fixture"},
		{name: "a tampered gold body", mutate: func(c *evalv1.EvaluationScenarioCatalog, a map[string]ScenarioArtifacts) {
			pair := a[c.Scenarios[0].GetScenarioId()]
			pair.Gold.Body = append(pair.Gold.Body, ' ')
			a[c.Scenarios[0].GetScenarioId()] = pair
		}, wantErr: "gold criteria"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			catalog, artifacts := cloneBuiltinCatalog(t)
			if tt.mutate == nil {
				assert.Error(t, ValidateScenarioCatalog(nil, artifacts))
				return
			}
			tt.mutate(catalog, artifacts)
			if tt.reseal {
				resealCatalog(t, catalog)
			}

			err := ValidateScenarioCatalog(catalog, artifacts)

			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
		})
	}
}

func TestValidateScenarioCatalog_RejectsAnEmptySuite(t *testing.T) {
	t.Parallel()
	catalog, artifacts := cloneBuiltinCatalog(t)
	catalog.Scenarios = nil
	catalog.CatalogDigest = ""

	err := ValidateScenarioCatalog(catalog, artifacts)

	require.Error(t, err)
}
