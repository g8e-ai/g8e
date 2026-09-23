// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package evaluation

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	compliancev1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/compliance/v1"
	evalv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/eval/v1"
)

func TestBuildPublicScenarioSummaryCopiesOnlyApprovedFields(t *testing.T) {
	t.Parallel()
	context := &PublicScenarioContext{
		ScenarioID: "scenario-1", ScenarioVersion: "1.0.0", Category: evalv1.EvaluationScenarioCategory_EVALUATION_SCENARIO_CATEGORY_TOOL_SELECTION,
		PublicDescription: "Choose the approved tool", GradingMethod: evalv1.EvaluationGradingMethod_EVALUATION_GRADING_METHOD_DETERMINISTIC,
		AllowedTools: []string{"safe_tool"}, Criteria: []*evalv1.PublicScenarioCriterion{{CriterionId: "criterion-1", PublicLabel: "Tool choice", PublicDescription: "Chooses the approved tool", GradingMethod: evalv1.EvaluationGradingMethod_EVALUATION_GRADING_METHOD_DETERMINISTIC, Required: true}},
	}
	summary, err := BuildPublicScenarioSummary(context)
	require.NoError(t, err)
	assert.Equal(t, context.ScenarioID, summary.GetScenarioId())
	assert.Equal(t, "Chooses the approved tool", summary.GetCriteria()[0].GetPublicDescription())
}

func TestResolvePublicScenarioContextRejectsMismatchedCatalogBinding(t *testing.T) {
	t.Parallel()
	catalog := &evalv1.EvaluationScenarioCatalog{CatalogDigest: "catalog-digest", CatalogRef: &compliancev1.VersionedReference{Id: "catalog", Version: "1.0.0"}}
	run := &evalv1.EvaluationRun{RunId: "run-1", CampaignBinding: &evalv1.ModelCampaignBinding{CampaignId: "campaign-1", CatalogDigest: "different", CatalogRef: catalog.GetCatalogRef()}}
	assignment := &evalv1.EvaluationAssignment{AssignmentId: "assignment-1", RunId: "run-1", CampaignId: "campaign-1", ScenarioId: "scenario-1"}
	_, err := ResolvePublicScenarioContext(context.Background(), nil, run, catalog, assignment, nil)
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrEvidenceScopeMismatch)
}

func TestResolvePublicScenarioContextRejectsFixtureDigestMismatch(t *testing.T) {
	t.Parallel()
	input := []byte(`{"fixture":"input"}`)
	gold := []byte(`{"fixture":"gold"}`)
	inputRef := evidenceReference(input, "input")
	goldRef := evidenceReference(gold, "gold")
	catalog := &evalv1.EvaluationScenarioCatalog{CatalogDigest: "catalog-digest", CatalogRef: &compliancev1.VersionedReference{Id: "catalog", Version: "1.0.0"}, Scenarios: []*evalv1.EvaluationScenarioDefinition{{ScenarioId: "scenario-1", ScenarioVersion: "1.0.0", PublicDescription: "Public", PublicCriteria: []*evalv1.PublicScenarioCriterion{{CriterionId: "criterion-1"}}, InputFixtureRef: inputRef, GoldCriteriaRef: goldRef}}}
	run := &evalv1.EvaluationRun{RunId: "run-1", CampaignBinding: &evalv1.ModelCampaignBinding{CampaignId: "campaign-1", CatalogDigest: catalog.GetCatalogDigest(), CatalogRef: catalog.GetCatalogRef()}}
	assignment := &evalv1.EvaluationAssignment{AssignmentId: "assignment-1", RunId: "run-1", CampaignId: "campaign-1", ScenarioId: "scenario-1", ScenarioRef: &compliancev1.VersionedReference{Id: "scenario-1", Version: "1.0.0"}}
	artifacts := map[string]ScenarioArtifacts{"scenario-1": {Input: ScenarioArtifactPair{Body: []byte("wrong"), Reference: inputRef}, Gold: ScenarioArtifactPair{Body: gold, Reference: goldRef}}}
	_, err := ResolvePublicScenarioContext(context.Background(), nil, run, catalog, assignment, artifacts)
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrEvidenceScopeMismatch)
}

func evidenceReference(body []byte, kind string) *compliancev1.ComplianceEvidenceReference {
	digest := sha256.Sum256(body)
	return &compliancev1.ComplianceEvidenceReference{ArtifactId: "artifact-" + kind, ArtifactType: kind, Sha256: hex.EncodeToString(digest[:]), SchemaRef: "schema@1.0.0"}
}

func TestResolvePublicScenarioContextCopiesBoundScenarioAndArtifacts(t *testing.T) {
	t.Parallel()
	input := []byte(`{"fixture":"input"}`)
	gold := []byte(`{"fixture":"gold"}`)
	inputRef := evidenceReference(input, "input")
	goldRef := evidenceReference(gold, "gold")
	catalogRef := &compliancev1.VersionedReference{Id: "catalog", Version: "1.0.0"}
	catalog := &evalv1.EvaluationScenarioCatalog{
		CatalogDigest: "catalog-digest", CatalogRef: catalogRef,
		Scenarios: []*evalv1.EvaluationScenarioDefinition{{
			ScenarioId: "scenario-1", ScenarioVersion: "1.0.0", PublicDescription: "Public", GradingMethod: evalv1.EvaluationGradingMethod_EVALUATION_GRADING_METHOD_DETERMINISTIC,
			PublicCriteria:            []*evalv1.PublicScenarioCriterion{{CriterionId: "b"}, {CriterionId: "a"}},
			PublicToolScoreDimensions: []*evalv1.PublicToolScoreDimensionRequirement{{Dimension: evalv1.PublicToolScoreDimension_PUBLIC_TOOL_SCORE_DIMENSION_TOOL_SELECTION}}, InputFixtureRef: inputRef, GoldCriteriaRef: goldRef,
		}},
	}
	run := &evalv1.EvaluationRun{RunId: "run-1", CampaignBinding: &evalv1.ModelCampaignBinding{CampaignId: "campaign-1", CampaignDigest: "campaign-digest", CatalogDigest: catalog.GetCatalogDigest(), CatalogRef: catalogRef}}
	assignment := &evalv1.EvaluationAssignment{AssignmentId: "assignment-1", RunId: "run-1", CampaignId: "campaign-1", ScenarioId: "scenario-1", ScenarioRef: &compliancev1.VersionedReference{Id: "scenario-1", Version: "1.0.0"}}
	artifacts := map[string]ScenarioArtifacts{"scenario-1": {Input: ScenarioArtifactPair{Body: input, Reference: inputRef}, Gold: ScenarioArtifactPair{Body: gold, Reference: goldRef}}}
	context, err := ResolvePublicScenarioContext(context.Background(), nil, run, catalog, assignment, artifacts)
	require.NoError(t, err)
	assert.Equal(t, "scenario-1", context.ScenarioID)
	assert.Len(t, context.Criteria, 2)
	assert.Equal(t, "a", context.Criteria[0].GetCriterionId())
	assert.Equal(t, "artifact-gold", context.ScenarioReference.GetArtifactId())
	assert.Equal(t, "catalog", context.CatalogRef.GetId())
}
