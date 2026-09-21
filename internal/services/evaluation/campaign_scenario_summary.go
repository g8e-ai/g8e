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
	"fmt"
	"sort"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	compliancev1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/compliance/v1"
	evalv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/eval/v1"
)

func ResolvePublicScenarioContext(ctx context.Context, store *Store, run *evalv1.EvaluationRun, catalog *evalv1.EvaluationScenarioCatalog, assignment *evalv1.EvaluationAssignment, artifacts map[string]ScenarioArtifacts) (*PublicScenarioContext, error) {
	if ctx == nil || run == nil || catalog == nil || assignment == nil {
		return nil, fmt.Errorf("evaluation: resolve public scenario: %w", constants.ErrEvidenceScopeMismatch)
	}
	binding := run.GetCampaignBinding()
	if binding == nil || binding.GetCampaignId() == "" || assignment.GetAssignmentId() == "" || assignment.GetRunId() != run.GetRunId() || assignment.GetCampaignId() != binding.GetCampaignId() {
		return nil, fmt.Errorf("evaluation: resolve public scenario: %w", constants.ErrEvidenceScopeMismatch)
	}
	if binding.GetCatalogDigest() == "" || binding.GetCatalogDigest() != catalog.GetCatalogDigest() || binding.GetCatalogRef() == nil || !sameReference(binding.GetCatalogRef(), catalog.GetCatalogRef()) {
		return nil, fmt.Errorf("evaluation: resolve public scenario catalog binding: %w", constants.ErrEvidenceScopeMismatch)
	}
	if store != nil {
		spec, err := store.LoadCampaignSpec(ctx, binding.GetCampaignId())
		if err != nil {
			return nil, fmt.Errorf("evaluation: resolve public scenario campaign: %w", err)
		}
		if spec.GetCampaignDigest() == "" || spec.GetCampaignDigest() != binding.GetCampaignDigest() || spec.GetCatalogDigest() != binding.GetCatalogDigest() || spec.GetCatalogRef() == nil || !sameReference(spec.GetCatalogRef(), binding.GetCatalogRef()) {
			return nil, fmt.Errorf("evaluation: resolve public scenario campaign binding: %w", constants.ErrEvidenceScopeMismatch)
		}
	}
	scenario := findScenario(catalog, assignment.GetScenarioId(), assignment.GetScenarioRef())
	if scenario == nil || scenario.GetPublicDescription() == "" || len(scenario.GetPublicCriteria()) == 0 {
		return nil, fmt.Errorf("evaluation: resolve public scenario: %w", constants.ErrEvidenceArtifactMalformed)
	}
	artifactsForScenario, ok := artifacts[scenario.GetScenarioId()]
	if !ok || !sameEvidenceReference(scenario.GetInputFixtureRef(), artifactsForScenario.Input.Reference) || !sameEvidenceReference(scenario.GetGoldCriteriaRef(), artifactsForScenario.Gold.Reference) || !contentMatchesReference(artifactsForScenario.Input.Body, artifactsForScenario.Input.Reference) || !contentMatchesReference(artifactsForScenario.Gold.Body, artifactsForScenario.Gold.Reference) {
		return nil, fmt.Errorf("evaluation: resolve public scenario artifact binding: %w", constants.ErrEvidenceScopeMismatch)
	}
	return &PublicScenarioContext{
		CampaignID: binding.GetCampaignId(), RunID: run.GetRunId(), ScenarioID: scenario.GetScenarioId(), ScenarioVersion: scenario.GetScenarioVersion(), Category: scenario.GetCategory(), PublicDescription: scenario.GetPublicDescription(), GradingMethod: scenario.GetGradingMethod(), AllowedTools: append([]string(nil), scenario.GetAllowedTools()...), ExpectedTools: append([]string(nil), scenario.GetExpectedTools()...), ForbiddenTools: append([]string(nil), scenario.GetForbiddenTools()...), Criteria: cloneCriteria(scenario.GetPublicCriteria()), ToolScoreDimensions: cloneDimensions(scenario.GetPublicToolScoreDimensions()), CatalogRef: cloneReference(catalog.GetCatalogRef()), CatalogDigest: catalog.GetCatalogDigest(), ScenarioReference: cloneEvidenceReference(scenario.GetGoldCriteriaRef()),
	}, nil
}

func BuildPublicScenarioSummary(scenario *PublicScenarioContext) (*evalv1.PublicScenarioSummary, error) {
	if scenario == nil || scenario.ScenarioID == "" || scenario.ScenarioVersion == "" || scenario.PublicDescription == "" || len(scenario.Criteria) == 0 {
		return nil, fmt.Errorf("evaluation: build public scenario summary: %w", constants.ErrEvidenceArtifactMalformed)
	}
	return &evalv1.PublicScenarioSummary{ScenarioId: scenario.ScenarioID, ScenarioVersion: scenario.ScenarioVersion, Category: scenario.Category, PublicDescription: scenario.PublicDescription, GradingMethod: scenario.GradingMethod, AllowedTools: append([]string(nil), scenario.AllowedTools...), ExpectedTools: append([]string(nil), scenario.ExpectedTools...), ForbiddenTools: append([]string(nil), scenario.ForbiddenTools...), Criteria: cloneCriteria(scenario.Criteria), ToolScoreDimensions: cloneDimensions(scenario.ToolScoreDimensions)}, nil
}

func findScenario(catalog *evalv1.EvaluationScenarioCatalog, id string, ref *compliancev1.VersionedReference) *evalv1.EvaluationScenarioDefinition {
	for _, scenario := range catalog.GetScenarios() {
		if scenario == nil || scenario.GetScenarioId() != id {
			continue
		}
		if ref != nil && ref.GetId() == scenario.GetScenarioId() && ref.GetVersion() == scenario.GetScenarioVersion() {
			return scenario
		}
	}
	return nil
}

func sameReference(left, right *compliancev1.VersionedReference) bool {
	return left != nil && right != nil && left.GetId() == right.GetId() && left.GetVersion() == right.GetVersion()
}

func sameEvidenceReference(left, right *compliancev1.ComplianceEvidenceReference) bool {
	return left != nil && right != nil && left.GetArtifactId() == right.GetArtifactId() && left.GetSha256() == right.GetSha256() && left.GetSchemaRef() == right.GetSchemaRef()
}

func contentMatchesReference(body []byte, ref *compliancev1.ComplianceEvidenceReference) bool {
	if len(body) == 0 || ref == nil || len(ref.GetSha256()) != sha256.Size*2 {
		return false
	}
	digest := sha256.Sum256(body)
	return hex.EncodeToString(digest[:]) == ref.GetSha256()
}

func cloneReference(ref *compliancev1.VersionedReference) *compliancev1.VersionedReference {
	if ref == nil {
		return nil
	}
	return &compliancev1.VersionedReference{Id: ref.GetId(), Version: ref.GetVersion()}
}

func cloneEvidenceReference(ref *compliancev1.ComplianceEvidenceReference) *compliancev1.ComplianceEvidenceReference {
	if ref == nil {
		return nil
	}
	return &compliancev1.ComplianceEvidenceReference{ArtifactId: ref.GetArtifactId(), ArtifactType: ref.GetArtifactType(), Sha256: ref.GetSha256(), SchemaRef: ref.GetSchemaRef()}
}

func cloneCriteria(criteria []*evalv1.PublicScenarioCriterion) []*evalv1.PublicScenarioCriterion {
	out := make([]*evalv1.PublicScenarioCriterion, 0, len(criteria))
	for _, criterion := range criteria {
		if criterion == nil {
			continue
		}
		out = append(out, &evalv1.PublicScenarioCriterion{CriterionId: criterion.GetCriterionId(), PublicLabel: criterion.GetPublicLabel(), PublicDescription: criterion.GetPublicDescription(), GradingMethod: criterion.GetGradingMethod(), Required: criterion.GetRequired()})
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].GetCriterionId() < out[j].GetCriterionId() })
	return out
}

func cloneDimensions(dimensions []*evalv1.PublicToolScoreDimensionRequirement) []*evalv1.PublicToolScoreDimensionRequirement {
	out := make([]*evalv1.PublicToolScoreDimensionRequirement, 0, len(dimensions))
	for _, dimension := range dimensions {
		if dimension == nil {
			continue
		}
		out = append(out, &evalv1.PublicToolScoreDimensionRequirement{Dimension: dimension.GetDimension(), Required: dimension.GetRequired()})
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].GetDimension() < out[j].GetDimension() })
	return out
}
