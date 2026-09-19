// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package evaluation

import (
	"encoding/json"
	"fmt"
	"time"

	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	complianceevidence "github.com/g8e-ai/g8e/v2/internal/services/compliance/evidence"
	compliancev1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/compliance/v1"
)

const (
	StandardCatalogID      = "north-star-25"
	StandardCatalogVersion = "1.0.0"
	StandardCatalogScopeID = "north-star-25"

	scenarioInputSchemaVersion = "1.0.0"
	scenarioGoldSchemaVersion  = "1.0.0"
	scenarioInputSchemaRef     = "evaluation-scenario-input@1.0.0"
	scenarioGoldSchemaRef      = "evaluation-scenario-gold@1.0.0"
)

var northStarCatalogFreezeTime = time.Date(2026, 9, 16, 0, 0, 0, 0, time.UTC)

// ScenarioAttachment is one labeled synthetic fixture attachment.
type ScenarioAttachment struct {
	Kind    string `json:"kind"`
	Label   string `json:"label"`
	Content string `json:"content"`
}

// ScenarioInputFixture is the typed private prompt/input payload for one scenario.
type ScenarioInputFixture struct {
	SchemaVersion  string               `json:"schema_version"`
	ScenarioID     string               `json:"scenario_id"`
	SyntheticLabel string               `json:"synthetic_label"`
	UserPrompt     string               `json:"user_prompt"`
	SystemContext  string               `json:"system_context,omitempty"`
	Attachments    []ScenarioAttachment `json:"attachments,omitempty"`
}

// ScenarioCriterion describes one responsibility-specific scoring criterion.
type ScenarioCriterion struct {
	CriterionID   string `json:"criterion_id"`
	Description   string `json:"description"`
	Deterministic bool   `json:"deterministic"`
}

// ScenarioRoleCriteria binds scoring criteria to one designated model role.
type ScenarioRoleCriteria struct {
	Role     string              `json:"role"`
	Criteria []ScenarioCriterion `json:"criteria"`
}

// ScenarioPipelineCriteria binds scoring criteria to one evaluation lane shape.
type ScenarioPipelineCriteria struct {
	Lane     string              `json:"lane"`
	Criteria []ScenarioCriterion `json:"criteria"`
}

// ScenarioArgumentCheck describes tool argument schema and semantic expectations.
type ScenarioArgumentCheck struct {
	ToolName     string `json:"tool_name"`
	SchemaRef    string `json:"schema_ref"`
	SemanticRule string `json:"semantic_rule"`
}

// ScenarioPolicyExpectation records the expected governed policy outcome.
type ScenarioPolicyExpectation struct {
	ExpectedOutcome string `json:"expected_outcome"`
	Detail          string `json:"detail"`
}

// ScenarioEscalationExpectation records whether escalation is expected.
type ScenarioEscalationExpectation struct {
	Expected bool   `json:"expected"`
	FromRole string `json:"from_role,omitempty"`
	ToRole   string `json:"to_role,omitempty"`
	Detail   string `json:"detail,omitempty"`
}

// ScenarioRecoveryExpectation records whether recovery behavior is expected.
type ScenarioRecoveryExpectation struct {
	Expected bool   `json:"expected"`
	Kind     string `json:"kind,omitempty"`
	Detail   string `json:"detail,omitempty"`
}

// ScenarioGoldCriteria is the typed private gold standard for one scenario.
type ScenarioGoldCriteria struct {
	SchemaVersion         string                        `json:"schema_version"`
	ScenarioID            string                        `json:"scenario_id"`
	ExpectedBehavior      string                        `json:"expected_behavior"`
	RoleCriteria          []ScenarioRoleCriteria        `json:"role_criteria"`
	PipelineCriteria      []ScenarioPipelineCriteria    `json:"pipeline_criteria"`
	ArgumentChecks        []ScenarioArgumentCheck       `json:"argument_checks,omitempty"`
	PolicyExpectation     ScenarioPolicyExpectation     `json:"policy_expectation"`
	EscalationExpectation ScenarioEscalationExpectation `json:"escalation_expectation"`
	RecoveryExpectation   ScenarioRecoveryExpectation   `json:"recovery_expectation"`
	RequiredEvidenceTypes []string                      `json:"required_evidence_types"`
}

// ScenarioArtifactPair holds canonical fixture bytes and their evidence reference.
type ScenarioArtifactPair struct {
	Body      []byte
	Reference *compliancev1.ComplianceEvidenceReference
}

func marshalScenarioFixture(value any) ([]byte, error) {
	body, err := json.Marshal(value)
	if err != nil {
		return nil, fmt.Errorf("evaluation: marshal scenario fixture: %w", err)
	}
	if err := complianceevidence.ValidateCanonicalJSON(body); err != nil {
		return nil, fmt.Errorf("evaluation: marshal scenario fixture: %w", err)
	}
	return body, nil
}

func buildScenarioArtifactReference(artifactType complianceevidence.ArtifactType, schemaRef, scenarioID string, body []byte) (*compliancev1.ComplianceEvidenceReference, error) {
	if scenarioID == "" || len(body) == 0 {
		return nil, fmt.Errorf("evaluation: build scenario artifact reference: %w", constants.ErrMissingRequiredField)
	}
	artifactID := complianceevidence.ContentAddress(artifactType, body)
	_, digest, ok := complianceevidence.ParseContentAddress(artifactID)
	if !ok {
		return nil, fmt.Errorf("evaluation: build scenario artifact reference: invalid content address")
	}
	freeze := timestamppb.New(northStarCatalogFreezeTime)
	return &compliancev1.ComplianceEvidenceReference{
		ArtifactId:         artifactID,
		ArtifactType:       string(artifactType),
		Sha256:             digest,
		MediaType:          constants.MediaTypeJSON,
		SchemaRef:          schemaRef,
		ProducerIdentity:   "g8e-eval-catalog",
		ProducedAt:         freeze,
		ScopeId:            StandardCatalogScopeID,
		RunId:              "catalog",
		ScenarioId:         scenarioID,
		VerificationStatus: "verified",
		VerifierId:         GraderID,
		VerifierVersion:    GraderVersion,
		VerifiedAt:         freeze,
	}, nil
}

func buildScenarioInputArtifact(input ScenarioInputFixture) (ScenarioArtifactPair, error) {
	input.SchemaVersion = scenarioInputSchemaVersion
	body, err := marshalScenarioFixture(input)
	if err != nil {
		return ScenarioArtifactPair{}, err
	}
	ref, err := buildScenarioArtifactReference(complianceevidence.ArtifactTypeEvaluationScenarioInput, scenarioInputSchemaRef, input.ScenarioID, body)
	if err != nil {
		return ScenarioArtifactPair{}, err
	}
	return ScenarioArtifactPair{Body: body, Reference: ref}, nil
}

func buildScenarioGoldArtifact(gold ScenarioGoldCriteria) (ScenarioArtifactPair, error) {
	gold.SchemaVersion = scenarioGoldSchemaVersion
	body, err := marshalScenarioFixture(gold)
	if err != nil {
		return ScenarioArtifactPair{}, err
	}
	ref, err := buildScenarioArtifactReference(complianceevidence.ArtifactTypeEvaluationScenarioGold, scenarioGoldSchemaRef, gold.ScenarioID, body)
	if err != nil {
		return ScenarioArtifactPair{}, err
	}
	return ScenarioArtifactPair{Body: body, Reference: ref}, nil
}
