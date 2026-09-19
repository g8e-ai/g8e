// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package evaluation

import (
	"context"
	"fmt"
	"path/filepath"

	"google.golang.org/protobuf/proto"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	complianceevidence "github.com/g8e-ai/g8e/v2/internal/services/compliance/evidence"
	"github.com/g8e-ai/g8e/v2/internal/services/fs"
	compliancev1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/compliance/v1"
	evalv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/eval/v1"
)

// EvidenceScope binds a persisted evidence artifact to its run, scenario,
// attempt, and transaction so a returned reference can never describe a body
// outside its declared scope.
type EvidenceScope struct {
	RunID         string
	ScenarioID    string
	AttemptID     string
	TransactionID string
}

type Store struct {
	files fs.RuntimeFileService
}

func NewStore(files fs.RuntimeFileService) *Store {
	return &Store{files: files}
}

// SaveTargetState persists one canonical EvaluationTargetState observation
// body under the run's content-addressed evidence directory and returns the
// scoped reference.
func (s *Store) SaveTargetState(ctx context.Context, evidence *evalv1.EvaluationTargetState) (*compliancev1.ComplianceEvidenceReference, error) {
	if s == nil || s.files == nil || evidence == nil || evidence.GetSchemaVersion() != RegistryVersion || !complianceevidence.ValidPathElement(evidence.GetRunId()) || !complianceevidence.ValidPathElement(evidence.GetScenarioId()) || !complianceevidence.ValidPathElement(evidence.GetAttemptId()) || evidence.GetTargetResource() == "" || evidence.GetObservedAt() == nil || evidence.GetObservedAt().CheckValid() != nil || (!evidence.GetPresent() && len(evidence.GetContent()) != 0) {
		return nil, fmt.Errorf("%w: target state evidence is incomplete", constants.ErrEvaluationReportPersistFailed)
	}
	return s.SaveProtoArtifact(ctx, EvidenceScope{
		RunID: evidence.GetRunId(), ScenarioID: evidence.GetScenarioId(), AttemptID: evidence.GetAttemptId(),
	}, complianceevidence.ArtifactTypeEvalObservation, evidence)
}

// SaveProtoArtifact persists one canonical protobuf evidence body beneath the
// run's evidence directory and returns a scope-bound reference. The body is
// durably written before the reference is returned.
func (s *Store) SaveProtoArtifact(ctx context.Context, scope EvidenceScope, artifactType complianceevidence.ArtifactType, message proto.Message) (*compliancev1.ComplianceEvidenceReference, error) {
	if s == nil || s.files == nil || message == nil || !validEvidenceScope(scope) || !complianceevidence.ContainsArtifactType(complianceevidence.SupportedArtifactTypes(), artifactType) {
		return nil, fmt.Errorf("%w: evidence scope, supported artifact type, and protocol message are required", constants.ErrEvaluationReportPersistFailed)
	}
	artifact, err := complianceevidence.PersistCanonicalProtoArtifact(ctx, s.files, evaluationEvidenceDir(scope.RunID), artifactType, message)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", constants.ErrEvaluationReportPersistFailed, err)
	}
	return scopedReference(artifact.Reference, scope), nil
}

// SaveJSONArtifact persists one already-canonical JSON evidence body, such as
// a recorded exchange or admission response, beneath the run's evidence
// directory and returns a scope-bound reference.
func (s *Store) SaveJSONArtifact(ctx context.Context, scope EvidenceScope, artifactType complianceevidence.ArtifactType, body []byte) (*compliancev1.ComplianceEvidenceReference, error) {
	if s == nil || s.files == nil || len(body) == 0 || !validEvidenceScope(scope) || !complianceevidence.ContainsArtifactType(complianceevidence.SupportedArtifactTypes(), artifactType) {
		return nil, fmt.Errorf("%w: evidence scope, supported artifact type, and canonical body are required", constants.ErrEvaluationReportPersistFailed)
	}
	if err := complianceevidence.ValidateCanonicalJSON(body); err != nil {
		return nil, fmt.Errorf("%w: canonical evidence body: %w", constants.ErrEvaluationReportPersistFailed, err)
	}
	artifactID := complianceevidence.ContentAddress(artifactType, body)
	_, digest, ok := complianceevidence.ParseContentAddress(artifactID)
	if !ok {
		return nil, fmt.Errorf("%w: evidence content address is invalid", constants.ErrEvaluationReportPersistFailed)
	}
	directory := evaluationEvidenceDir(scope.RunID)
	if err := s.files.MkdirAll(ctx, directory, constants.PermDirStandard); err != nil {
		return nil, fmt.Errorf("%w: create evidence directory: %w", constants.ErrEvaluationReportPersistFailed, err)
	}
	if err := s.files.WriteFile(ctx, filepath.Join(directory, digest+constants.FileExtJSON), body, constants.PermFileReadOnly); err != nil {
		return nil, fmt.Errorf("%w: write evidence artifact: %w", constants.ErrEvaluationReportPersistFailed, err)
	}
	return scopedReference(&compliancev1.ComplianceEvidenceReference{
		ArtifactId: artifactID, ArtifactType: string(artifactType), Sha256: digest, MediaType: constants.MediaTypeJSON,
	}, scope), nil
}

func validEvidenceScope(scope EvidenceScope) bool {
	if !complianceevidence.ValidPathElement(scope.RunID) {
		return false
	}
	if scope.ScenarioID != "" && !complianceevidence.ValidPathElement(scope.ScenarioID) {
		return false
	}
	if scope.AttemptID != "" && !complianceevidence.ValidPathElement(scope.AttemptID) {
		return false
	}
	return true
}

func scopedReference(reference *compliancev1.ComplianceEvidenceReference, scope EvidenceScope) *compliancev1.ComplianceEvidenceReference {
	reference.RunId = scope.RunID
	reference.ScenarioId = scope.ScenarioID
	reference.AttemptId = scope.AttemptID
	reference.TransactionId = scope.TransactionID
	return reference
}

func (s *Store) SaveReport(ctx context.Context, report *evalv1.EvaluationReport) error {
	if s == nil || s.files == nil || report == nil || report.GetRun() == nil || !complianceevidence.ValidPathElement(report.GetRun().GetRunId()) {
		return fmt.Errorf("%w: file service and canonical report run ID are required", constants.ErrEvaluationReportPersistFailed)
	}
	body, err := evalv1.MarshalCanonical(report)
	if err != nil {
		return fmt.Errorf("%w: canonicalize report: %w", constants.ErrEvaluationReportPersistFailed, err)
	}
	runDir := evaluationRunDir(report.GetRun().GetRunId())
	if err := s.files.MkdirAll(ctx, filepath.Join(runDir, constants.EvaluationEvidenceDirname), constants.PermDirStandard); err != nil {
		return fmt.Errorf("%w: create run directory: %w", constants.ErrEvaluationReportPersistFailed, err)
	}
	if err := s.files.WriteFile(ctx, filepath.Join(runDir, constants.EvaluationReportFilename), body, constants.PermFileReadOnly); err != nil {
		return fmt.Errorf("%w: write report: %w", constants.ErrEvaluationReportPersistFailed, err)
	}
	return nil
}

func (s *Store) LoadReport(ctx context.Context, runID string) (*evalv1.EvaluationReport, error) {
	if s == nil || s.files == nil || !complianceevidence.ValidPathElement(runID) {
		return nil, fmt.Errorf("%w: file service and canonical run ID are required", constants.ErrEvidenceArtifactMalformed)
	}
	body, err := s.files.ReadFile(ctx, filepath.Join(evaluationRunDir(runID), constants.EvaluationReportFilename))
	if err != nil {
		return nil, fmt.Errorf("evaluation: read report: %w", err)
	}
	report := &evalv1.EvaluationReport{}
	if err := evalv1.UnmarshalCanonical(body, report); err != nil {
		return nil, fmt.Errorf("%w: canonical evaluation report: %v", constants.ErrEvidenceArtifactMalformed, err)
	}
	if report.GetRun().GetRunId() != runID {
		return nil, fmt.Errorf("%w: report run ID does not match requested run", constants.ErrEvidenceScopeMismatch)
	}
	return report, nil
}

func (s *Store) SaveVerification(ctx context.Context, runID string, report *compliancev1.ComplianceVerificationReport) (*compliancev1.ComplianceEvidenceReference, error) {
	if s == nil || s.files == nil || !complianceevidence.ValidPathElement(runID) || report == nil || report.GetReportId() != runID {
		return nil, fmt.Errorf("%w: verification report binding is invalid", constants.ErrEvaluationReportPersistFailed)
	}
	body, err := compliancev1.MarshalCanonical(report)
	if err != nil {
		return nil, fmt.Errorf("%w: canonicalize verification report: %w", constants.ErrEvaluationReportPersistFailed, err)
	}
	if err := s.files.WriteFile(ctx, filepath.Join(evaluationRunDir(runID), constants.EvaluationVerificationFilename), body, constants.PermFileReadOnly); err != nil {
		return nil, fmt.Errorf("%w: write verification report: %w", constants.ErrEvaluationReportPersistFailed, err)
	}
	artifactID := complianceevidence.ContentReferenceForBody("evaluation-verification", body)
	_, digest, ok := complianceevidence.ParseExpectedContentReference(artifactID, "evaluation-verification")
	if !ok {
		return nil, fmt.Errorf("%w: verification content address is invalid", constants.ErrEvaluationReportPersistFailed)
	}
	return &compliancev1.ComplianceEvidenceReference{ArtifactId: artifactID, ArtifactType: "evaluation-verification", Sha256: digest, MediaType: constants.MediaTypeJSON, RunId: runID}, nil
}

func (s *Store) LoadVerification(ctx context.Context, runID string) (*compliancev1.ComplianceVerificationReport, error) {
	if s == nil || s.files == nil || !complianceevidence.ValidPathElement(runID) {
		return nil, fmt.Errorf("%w: file service and canonical run ID are required", constants.ErrEvidenceArtifactMalformed)
	}
	body, err := s.files.ReadFile(ctx, filepath.Join(evaluationRunDir(runID), constants.EvaluationVerificationFilename))
	if err != nil {
		return nil, fmt.Errorf("evaluation: read verification report: %w", err)
	}
	report := &compliancev1.ComplianceVerificationReport{}
	if err := compliancev1.UnmarshalCanonical(body, report); err != nil {
		return nil, fmt.Errorf("%w: canonical verification report: %v", constants.ErrEvidenceArtifactMalformed, err)
	}
	if report.GetReportId() != runID {
		return nil, fmt.Errorf("%w: verification report run ID does not match requested run", constants.ErrEvidenceScopeMismatch)
	}
	return report, nil
}

func evaluationRunDir(runID string) string {
	return filepath.Join(constants.DataDirname, constants.EvaluationDirname, constants.EvaluationRunsDirname, runID)
}

func evaluationEvidenceDir(runID string) string {
	return filepath.Join(evaluationRunDir(runID), constants.EvaluationEvidenceDirname)
}
