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
	evalv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/eval/v1"
)

const (
	StandardCatalogID      = "north-star-25"
	StandardCatalogVersion = "1.1.0"
	StandardCatalogScopeID = "north-star-25"

	scenarioInputSchemaVersion = "1.2.0"
	scenarioGoldSchemaVersion  = "1.1.0"
	scenarioInputSchemaRef     = "evaluation-scenario-input@1.2.0"
	scenarioGoldSchemaRef      = "evaluation-scenario-gold@1.1.0"

	// ScenarioWorkspaceToken is the typed template variable a prompt, seed, or
	// argument validator uses for the attempt-scoped fixture workspace root.
	// It is replaced with the absolute root before a prompt is sent or a
	// validator runs; nothing else is templated.
	ScenarioWorkspaceToken = "{{workspace}}"
)

var northStarCatalogFreezeTime = time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)

// ScenarioInlineContent is synthetic content the scenario expects the model to
// reason over in the same turn. It is rendered as a labelled block beneath the
// user prompt in the outgoing chat message (see renderScenarioMessage); there
// is no separate attachment channel on this path.
type ScenarioInlineContent struct {
	Kind    string `json:"kind"`
	Label   string `json:"label"`
	Content string `json:"content"`
}

// ScenarioWorkspaceFile is one fixture file materialized under the
// attempt-scoped workspace on the bound Data Operator before the scored chat
// request is sent (one governed write per file, each with a receipt). RelPath
// is slash-separated and relative to the workspace root. Decoy files exist so
// a search can be wrong. Content is never sent to the model directly: the
// point of these scenarios is that the model must use a tool to read it.
type ScenarioWorkspaceFile struct {
	Label   string `json:"label"`
	RelPath string `json:"rel_path"`
	Content string `json:"content"`
	Decoy   bool   `json:"decoy,omitempty"`
}

// InvestigationSeedTurn is one prior conversation turn. Sender is "user",
// "primary", or "assistant". When GuidanceVectorID is set, catalog build
// appends the vector's real model-visible error to Content (so a prior AI turn
// quotes the text the model actually saw) and clears the ID.
type InvestigationSeedTurn struct {
	Sender           string `json:"sender"`
	Content          string `json:"content"`
	GuidanceVectorID string `json:"guidance_vector_id,omitempty"`
}

// InvestigationSeedHistoryEvent is one prior history-trail event. When
// GuidanceVectorID is set, the event is a prior tool call whose ErrorType,
// Error, ExecutionID, and ArgumentsJSON are copied at catalog build from the
// generated agent tool registry, i.e. the genuine text g8ee returned when the
// failing call ran, never hand-written prose.
type InvestigationSeedHistoryEvent struct {
	EventType        string `json:"event_type"`
	Actor            string `json:"actor"`
	Summary          string `json:"summary"`
	ToolName         string `json:"tool_name,omitempty"`
	GuidanceVectorID string `json:"guidance_vector_id,omitempty"`
	ExecutionID      string `json:"execution_id,omitempty"`
	ArgumentsJSON    string `json:"arguments_json,omitempty"`
	Command          string `json:"command,omitempty"`
	Error            string `json:"error,omitempty"`
	ErrorType        string `json:"error_type,omitempty"`
}

// InvestigationSeedMemory is the case memory of the seeded investigation.
type InvestigationSeedMemory struct {
	InvestigationSummary     string `json:"investigation_summary,omitempty"`
	CommunicationPreferences string `json:"communication_preferences,omitempty"`
	TechnicalBackground      string `json:"technical_background,omitempty"`
	ResponseStyle            string `json:"response_style,omitempty"`
	ProblemSolvingApproach   string `json:"problem_solving_approach,omitempty"`
	InteractionStyle         string `json:"interaction_style,omitempty"`
}

// InvestigationSeed is the investigation a scored turn runs in. g8ee applies
// it through its own investigation and memory services before triage; only
// the scored turn is a real chat call. CaseTitle is a realistic case title the
// model sees in its system prompt; it never names the evaluation.
type InvestigationSeed struct {
	CaseTitle       string                          `json:"case_title"`
	CaseDescription string                          `json:"case_description,omitempty"`
	Turns           []InvestigationSeedTurn         `json:"turns,omitempty"`
	HistoryEvents   []InvestigationSeedHistoryEvent `json:"history_events,omitempty"`
	CaseMemory      *InvestigationSeedMemory        `json:"case_memory,omitempty"`
}

// ScenarioInputFixture is the typed private prompt/input payload for one scenario.
type ScenarioInputFixture struct {
	SchemaVersion  string                  `json:"schema_version"`
	ScenarioID     string                  `json:"scenario_id"`
	SyntheticLabel string                  `json:"synthetic_label"`
	UserPrompt     string                  `json:"user_prompt"`
	InlineContext  []ScenarioInlineContent `json:"inline_context,omitempty"`
	WorkspaceFiles []ScenarioWorkspaceFile `json:"workspace_files,omitempty"`
	Seed           InvestigationSeed       `json:"seed"`
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

// ScenarioContentCheck is a typed, conjunctive check of the designated role's
// final output. Every set constraint must hold. Term matching is
// case-insensitive; each RequiredTerms group must match at least one term.
type ScenarioContentCheck struct {
	ExactToken        string            `json:"exact_token,omitempty"`
	ExactLabels       []string          `json:"exact_labels,omitempty"`
	LeadingLabel      string            `json:"leading_label,omitempty"`
	WordCount         int               `json:"word_count,omitempty"`
	MaxSentences      int               `json:"max_sentences,omitempty"`
	RequiredTerms     [][]string        `json:"required_terms,omitempty"`
	ForbiddenTerms    []string          `json:"forbidden_terms,omitempty"`
	JSONStringFields  map[string]string `json:"json_string_fields,omitempty"`
	JSONIntegerFields map[string]int64  `json:"json_integer_fields,omitempty"`
}

// ToolArgumentConstraint constrains one named argument of a tool call. Values
// may contain ScenarioWorkspaceToken. Every set constraint must hold.
type ToolArgumentConstraint struct {
	Name string `json:"name"`
	// Equals requires the exact string value.
	Equals string `json:"equals,omitempty"`
	// OneOf requires the string value to be one of the listed values.
	OneOf []string `json:"one_of,omitempty"`
	// PathEquals requires the path, resolved against the operator working
	// directory and cleaned, to equal this path.
	PathEquals string `json:"path_equals,omitempty"`
	// PathUnder requires the resolved path to equal or sit under this path.
	PathUnder string `json:"path_under,omitempty"`
	// RegexMatches and RegexRejects treat the value as a regular expression
	// that must match every RegexMatches sample and no RegexRejects sample, so
	// an escaped-equivalent pattern passes and an over-broad one fails.
	RegexMatches []string `json:"regex_matches,omitempty"`
	RegexRejects []string `json:"regex_rejects,omitempty"`
	// RequiredTerms and ForbiddenTerms apply to free-text values.
	RequiredTerms  [][]string `json:"required_terms,omitempty"`
	ForbiddenTerms []string   `json:"forbidden_terms,omitempty"`
}

// ToolArgumentValidator is the typed argument contract for one tool, keyed by
// tool name. CommandForbiddenTerms applies to the shell command the Tribunal
// generated for a run_commands_with_operator call, when the trace has one.
type ToolArgumentValidator struct {
	ToolName              string                   `json:"tool_name"`
	Arguments             []ToolArgumentConstraint `json:"arguments"`
	CommandForbiddenTerms []string                 `json:"command_forbidden_terms,omitempty"`
}

// ScenarioHintArgument says where one required argument of a hinted tool comes
// from. Value is the literal the prompt or seed must contain for PROMPT and
// SEED sources, and is private (it never reaches the public projection).
type ScenarioHintArgument struct {
	ToolName string                              `json:"tool_name"`
	Name     string                              `json:"name"`
	Source   evalv1.EvaluationHintArgumentSource `json:"source"`
	Value    string                              `json:"value,omitempty"`
}

// ScenarioPromptHint records which tools the prompt hints at and where each
// required argument of each hinted tool is derivable from.
type ScenarioPromptHint struct {
	HintedTools []string               `json:"hinted_tools"`
	Arguments   []ScenarioHintArgument `json:"arguments"`
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
	ContentCheck          *ScenarioContentCheck         `json:"content_check,omitempty"`
	ArgumentValidators    []ToolArgumentValidator       `json:"argument_validators,omitempty"`
	PromptHint            *ScenarioPromptHint           `json:"prompt_hint,omitempty"`
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
