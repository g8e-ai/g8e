// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package evaluation

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/models"
	complianceevidence "github.com/g8e-ai/g8e/v2/internal/services/compliance/evidence"
	compliancev1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/compliance/v1"
	evalv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/eval/v1"
)

const formationRunEvidenceSchemaVersion = "formation-run-evidence.v1"

// FormationRunEvidence is the canonical persisted envelope for one governed
// heterogeneous assignment formation run.
type FormationRunEvidence struct {
	SchemaVersion        string                     `json:"schema_version"`
	EvidenceDigest       string                     `json:"evidence_digest"`
	RunID                string                     `json:"run_id"`
	AssignmentID         string                     `json:"assignment_id"`
	EvaluationAttemptID  string                     `json:"evaluation_attempt_id"`
	CampaignID           string                     `json:"campaign_id"`
	ScenarioID           string                     `json:"scenario_id"`
	FormationID          string                     `json:"formation_id"`
	ModelRegistryDigest  string                     `json:"model_registry_digest"`
	InferenceSessionID   string                     `json:"inference_session_id"`
	DataSessionID        string                     `json:"data_session_id"`
	Result               persistedFormationRunResult `json:"result"`
}

type persistedFormationRunResult struct {
	SchemaVersion        string                           `json:"schema_version"`
	FormationID          string                           `json:"formation_id"`
	Passed               bool                             `json:"passed"`
	PeakVRAMMiB          uint64                           `json:"peak_vram_mib,omitempty"`
	Roles                []persistedFormationRoleTelemetry `json:"roles"`
	MutationIntercepted  bool                             `json:"mutation_intercepted"`
	AllPolicyLayersValid bool                             `json:"all_policy_layers_valid"`
}

type persistedFormationRoleTelemetry struct {
	Role                         string  `json:"role"`
	VariantID                    string  `json:"variant_id"`
	ProviderClass                string  `json:"provider_class"`
	ServedModelTag               string  `json:"served_model_tag"`
	ModelDigest                  string  `json:"model_digest"`
	Family                       string  `json:"family"`
	AttemptID                    string  `json:"attempt_id"`
	AttestationStatus            string  `json:"attestation_status"`
	AttestationVerified          bool    `json:"attestation_verified"`
	AttestationDigest            string  `json:"attestation_digest,omitempty"`
	ProviderAttemptID            string  `json:"provider_attempt_id"`
	ObserverObservationDigest    string  `json:"observer_observation_digest,omitempty"`
	ProvenanceAttestationDigest  string  `json:"provenance_attestation_digest,omitempty"`
	PeakVRAMMiB                  uint64  `json:"peak_vram_mib,omitempty"`
	TTFTNanos                    uint64  `json:"ttft_nanos,omitempty"`
	GenerationTokens             uint32  `json:"generation_tokens,omitempty"`
	GenerationDurationNanos      uint64  `json:"generation_duration_nanos,omitempty"`
	GenerationTokensPerSec       float64 `json:"generation_tokens_per_sec,omitempty"`
}

// CampaignFormationRunStore persists canonical formation-run evidence for one
// heterogeneous assignment.
type CampaignFormationRunStore interface {
	SaveAssignmentFormationRun(ctx context.Context, runID, assignmentID string, body []byte) error
}

// BuildFormationRunEvidence materializes one content-addressed formation-run
// envelope from governed execution output.
func BuildFormationRunEvidence(req AssignmentExecutionRequest, runContext FormationRunContext, formationResult *FormationRunResult) ([]byte, *FormationRunEvidence, error) {
	if req.Assignment == nil || formationResult == nil || runContext.RunID == "" || runContext.AssignmentID == "" || runContext.EvaluationAttemptID == "" {
		return nil, nil, fmt.Errorf("evaluation: build formation run evidence: %w", constants.ErrMissingRequiredField)
	}
	evidence := &FormationRunEvidence{
		SchemaVersion:       formationRunEvidenceSchemaVersion,
		RunID:               runContext.RunID,
		AssignmentID:        runContext.AssignmentID,
		EvaluationAttemptID: runContext.EvaluationAttemptID,
		CampaignID:          runContext.CampaignID,
		ScenarioID:          runContext.ScenarioID,
		FormationID:           formationResult.FormationID,
		ModelRegistryDigest: runContext.ModelRegistryDigest,
		InferenceSessionID:  runContext.InferenceSessionID,
		DataSessionID:       runContext.DataSessionID,
		Result:              formationRunResultToPersisted(formationResult),
	}
	digest, err := ComputeFormationRunEvidenceDigest(evidence)
	if err != nil {
		return nil, nil, err
	}
	evidence.EvidenceDigest = digest
	body, err := MarshalFormationRunEvidence(evidence)
	if err != nil {
		return nil, nil, err
	}
	return body, evidence, nil
}

// BuildAssignmentFormationRunEvidenceReference returns a content-addressed
// evidence reference for one persisted formation-run envelope.
func BuildAssignmentFormationRunEvidenceReference(runID, assignmentID, attemptID string, evidence *FormationRunEvidence, producedAt time.Time) (*compliancev1.ComplianceEvidenceReference, error) {
	if runID == "" || assignmentID == "" || attemptID == "" || evidence == nil {
		return nil, fmt.Errorf("evaluation: build formation run evidence reference: %w", constants.ErrMissingRequiredField)
	}
	body, err := MarshalFormationRunEvidence(evidence)
	if err != nil {
		return nil, err
	}
	if err := complianceevidence.ValidateCanonicalJSON(body); err != nil {
		return nil, fmt.Errorf("evaluation: build formation run evidence reference: %w", err)
	}
	artifactID := complianceevidence.ContentAddress(complianceevidence.ArtifactTypeEvaluationAssignmentFormationRun, body)
	_, digest, ok := complianceevidence.ParseContentAddress(artifactID)
	if !ok {
		return nil, fmt.Errorf("evaluation: build formation run evidence reference: invalid content address")
	}
	if producedAt.IsZero() {
		producedAt = time.Now().UTC()
	}
	return &compliancev1.ComplianceEvidenceReference{
		ArtifactId:         artifactID,
		ArtifactType:       string(complianceevidence.ArtifactTypeEvaluationAssignmentFormationRun),
		Sha256:             digest,
		MediaType:          constants.MediaTypeJSON,
		SchemaRef:          "g8e.eval.v1.EvaluationAssignmentFormationRun",
		ProducerIdentity:   "g8e-eval-campaign",
		ProducedAt:         timestamppb.New(producedAt),
		ScopeId:            runID,
		RunId:              runID,
		AttemptId:          attemptID,
		VerificationStatus: "imported",
	}, nil
}

// MarshalFormationRunEvidence returns canonical JSON bytes for one envelope.
func MarshalFormationRunEvidence(evidence *FormationRunEvidence) ([]byte, error) {
	if evidence == nil {
		return nil, fmt.Errorf("evaluation: marshal formation run evidence: %w", constants.ErrMissingRequiredField)
	}
	return MarshalCanonicalJSONObject(evidence)
}

// ComputeFormationRunEvidenceDigest returns the SHA-256 digest for one envelope
// with evidence_digest cleared before canonicalization.
func ComputeFormationRunEvidenceDigest(evidence *FormationRunEvidence) (string, error) {
	if evidence == nil {
		return "", fmt.Errorf("evaluation: compute formation run evidence digest: %w", constants.ErrMissingRequiredField)
	}
	payload := *evidence
	payload.EvidenceDigest = ""
	body, err := MarshalCanonicalJSONObject(payload)
	if err != nil {
		return "", fmt.Errorf("evaluation: compute formation run evidence digest: %w", err)
	}
	return models.SHA256Hex(body), nil
}

// ValidateFormationRunEvidenceDigest verifies one envelope digest binding.
func ValidateFormationRunEvidenceDigest(evidence *FormationRunEvidence) error {
	if evidence == nil || evidence.EvidenceDigest == "" {
		return fmt.Errorf("evaluation: validate formation run evidence: missing evidence_digest")
	}
	expected, err := ComputeFormationRunEvidenceDigest(evidence)
	if err != nil {
		return err
	}
	if evidence.EvidenceDigest != expected {
		return fmt.Errorf("evaluation: validate formation run evidence: evidence digest mismatch")
	}
	return nil
}

// LoadAssignmentFormationRunEvidence reads one persisted formation-run envelope.
func LoadAssignmentFormationRunEvidence(ctx context.Context, reader complianceevidence.ArtifactReader, runID, assignmentID string) (*FormationRunEvidence, error) {
	if reader == nil || !complianceevidence.ValidPathElement(runID) || !complianceevidence.ValidPathElement(assignmentID) {
		return nil, fmt.Errorf("evaluation: load assignment formation run evidence: %w", constants.ErrMissingRequiredField)
	}
	path := assignmentFormationRunPath(runID, assignmentID)
	body, err := reader.ReadFile(ctx, path)
	if err != nil {
		return nil, err
	}
	if err := complianceevidence.ValidateCanonicalJSON(body); err != nil {
		return nil, fmt.Errorf("evaluation: load assignment formation run evidence: %w", err)
	}
	evidence := &FormationRunEvidence{}
	if err := json.Unmarshal(body, evidence); err != nil {
		return nil, fmt.Errorf("evaluation: load assignment formation run evidence: %w", err)
	}
	if evidence.RunID != runID || evidence.AssignmentID != assignmentID {
		return nil, fmt.Errorf("evaluation: load assignment formation run evidence: %w", constants.ErrEvidenceScopeMismatch)
	}
	if err := ValidateFormationRunEvidenceDigest(evidence); err != nil {
		return nil, err
	}
	return evidence, nil
}

// FormationRunResultFromEvidence converts one persisted envelope back into the
// governed FormationRunner result shape used by assignment import.
func FormationRunResultFromEvidence(evidence *FormationRunEvidence) (*FormationRunResult, error) {
	if evidence == nil {
		return nil, fmt.Errorf("evaluation: formation run result from evidence: %w", constants.ErrMissingRequiredField)
	}
	return persistedFormationRunResultToDomain(evidence.Result), nil
}

// VerifyFormationRunEvidenceMatchesResult independently checks that one stored
// assignment result matches persisted formation-run evidence.
func VerifyFormationRunEvidenceMatchesResult(assignment *evalv1.EvaluationAssignment, evidence *FormationRunEvidence, result *evalv1.EvaluationAssignmentResult) error {
	if assignment == nil || evidence == nil || result == nil {
		return fmt.Errorf("evaluation: verify formation run evidence matches result: %w", constants.ErrMissingRequiredField)
	}
	if evidence.AssignmentID != assignment.GetAssignmentId() || evidence.RunID != assignment.GetRunId() {
		return fmt.Errorf("formation run evidence binding mismatch")
	}
	formationResult, err := FormationRunResultFromEvidence(evidence)
	if err != nil {
		return err
	}
	imported, err := ImportAssignmentResultFromFormationRun(AssignmentExecutionRequest{
		Assignment: assignment,
		AttemptID:  evidence.EvaluationAttemptID,
	}, formationResult, importTime(result), func(prefix string) string { return prefix })
	if err != nil {
		return err
	}
	if len(imported.GetModelInferences()) != len(result.GetModelInferences()) {
		return fmt.Errorf("formation run evidence model inference count mismatch")
	}
	for index, expected := range imported.GetModelInferences() {
		actual := result.GetModelInferences()[index]
		expected.InferenceRecordId = actual.GetInferenceRecordId()
		if expected.GetProviderAttemptId() != actual.GetProviderAttemptId() ||
			expected.GetModelRole() != actual.GetModelRole() ||
			expected.GetCallSite() != actual.GetCallSite() ||
			expected.GetGenerationDurationNanos() != actual.GetGenerationDurationNanos() {
			return fmt.Errorf("formation run evidence model inference %d mismatch", index)
		}
	}
	if imported.GetLifecycleStatus() != result.GetLifecycleStatus() {
		return fmt.Errorf("formation run evidence lifecycle mismatch")
	}
	return nil
}

func formationRunResultToPersisted(result *FormationRunResult) persistedFormationRunResult {
	if result == nil {
		return persistedFormationRunResult{}
	}
	out := persistedFormationRunResult{
		SchemaVersion:        result.SchemaVersion,
		FormationID:          result.FormationID,
		Passed:               result.Passed,
		PeakVRAMMiB:          result.PeakVRAMMiB,
		MutationIntercepted:  result.MutationIntercepted,
		AllPolicyLayersValid: result.AllPolicyLayersValid,
		Roles:                make([]persistedFormationRoleTelemetry, 0, len(result.Roles)),
	}
	for _, role := range result.Roles {
		observerDigest, provenanceDigest := formationRoleWitnessDigests(role)
		out.Roles = append(out.Roles, persistedFormationRoleTelemetry{
			Role:                        string(role.Role),
			VariantID:                   role.Model.VariantID,
			ProviderClass:               role.Model.ProviderClass,
			ServedModelTag:              role.Model.ServedModelTag,
			ModelDigest:                 role.Model.ModelDigest,
			Family:                      role.Model.Family,
			AttemptID:                   role.AttemptID,
			AttestationStatus:           string(role.AttestationStatus),
			AttestationVerified:         role.AttestationVerified,
			AttestationDigest:           role.AttestationDigest,
			ProviderAttemptID:           role.ProviderAttemptID,
			ObserverObservationDigest:   observerDigest,
			ProvenanceAttestationDigest: provenanceDigest,
			PeakVRAMMiB:                 role.PeakVRAMMiB,
			TTFTNanos:                   role.TTFTNanos,
			GenerationTokens:            role.GenerationTokens,
			GenerationDurationNanos:     role.GenerationDurationNanos,
			GenerationTokensPerSec:      role.GenerationTokensPerSec,
		})
	}
	return out
}

func persistedFormationRunResultToDomain(result persistedFormationRunResult) *FormationRunResult {
	out := &FormationRunResult{
		SchemaVersion:        result.SchemaVersion,
		FormationID:          result.FormationID,
		Passed:               result.Passed,
		PeakVRAMMiB:          result.PeakVRAMMiB,
		MutationIntercepted:  result.MutationIntercepted,
		AllPolicyLayersValid: result.AllPolicyLayersValid,
		Roles:                make([]FormationRoleTelemetry, 0, len(result.Roles)),
	}
	for _, role := range result.Roles {
		telemetry := FormationRoleTelemetry{
			Role:                    FormationRole(role.Role),
			Model:                   formationModelFromPersisted(role),
			AttemptID:               role.AttemptID,
			AttestationStatus:       FormationAttestationStatus(role.AttestationStatus),
			AttestationVerified:     role.AttestationVerified,
			AttestationDigest:       role.AttestationDigest,
			ProviderAttemptID:       role.ProviderAttemptID,
			PeakVRAMMiB:             role.PeakVRAMMiB,
			TTFTNanos:               role.TTFTNanos,
			GenerationTokens:        role.GenerationTokens,
			GenerationDurationNanos: role.GenerationDurationNanos,
			GenerationTokensPerSec:  role.GenerationTokensPerSec,
		}
		if role.ObserverObservationDigest != "" {
			telemetry.ObserverEvidence = &FormationObserverEvidence{
				Window: &evalv1.ProviderBoundaryObservationWindow{
					ProviderAttemptId: role.ProviderAttemptID,
					ObservationDigest: role.ObserverObservationDigest,
				},
			}
		}
		if role.ProvenanceAttestationDigest != "" {
			telemetry.ProvenanceEvidence = &FormationAttestation{
				Verified: role.AttestationVerified,
				Digest:   role.AttestationDigest,
				Window: &evalv1.ModelProvenanceAttestationWindow{
					ProviderAttemptId: role.ProviderAttemptID,
					AttestationDigest: role.ProvenanceAttestationDigest,
				},
			}
		}
		out.Roles = append(out.Roles, telemetry)
	}
	return out
}

func formationModelFromPersisted(role persistedFormationRoleTelemetry) FormationModel {
	return FormationModel{
		VariantID:      role.VariantID,
		ProviderClass:  role.ProviderClass,
		ServedModelTag: role.ServedModelTag,
		ModelDigest:    role.ModelDigest,
		Family:         role.Family,
	}
}

func importTime(result *evalv1.EvaluationAssignmentResult) time.Time {
	if result != nil && result.GetCompletedAt() != nil {
		return result.GetCompletedAt().AsTime()
	}
	return time.Now().UTC()
}
