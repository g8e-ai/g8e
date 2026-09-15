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
	"path/filepath"
	"time"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	complianceevidence "github.com/g8e-ai/g8e/v2/internal/services/compliance/evidence"
	"github.com/g8e-ai/g8e/v2/internal/services/fs"
	compliancev1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/compliance/v1"
	evalv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/eval/v1"
)

type TargetStateEvidence struct {
	SchemaVersion  string    `json:"schema_version"`
	RunID          string    `json:"run_id"`
	ScenarioID     string    `json:"scenario_id"`
	AttemptID      string    `json:"attempt_id"`
	TargetResource string    `json:"target_resource"`
	ObservedAt     time.Time `json:"observed_at"`
	Present        bool      `json:"present"`
	Content        []byte    `json:"content"`
}

type Store struct {
	files fs.RuntimeFileService
}

func NewStore(files fs.RuntimeFileService) *Store {
	return &Store{files: files}
}

func (s *Store) SaveTargetState(ctx context.Context, evidence *TargetStateEvidence) (*compliancev1.ComplianceEvidenceReference, error) {
	if s == nil || s.files == nil || evidence == nil || evidence.SchemaVersion != RegistryVersion || !complianceevidence.ValidPathElement(evidence.RunID) || !complianceevidence.ValidPathElement(evidence.ScenarioID) || !complianceevidence.ValidPathElement(evidence.AttemptID) || evidence.TargetResource == "" || evidence.ObservedAt.IsZero() || (!evidence.Present && len(evidence.Content) != 0) {
		return nil, fmt.Errorf("%w: target state evidence is incomplete", constants.ErrEvaluationReportPersistFailed)
	}
	body, err := json.Marshal(evidence)
	if err != nil {
		return nil, fmt.Errorf("%w: canonicalize target state evidence: %w", constants.ErrEvaluationReportPersistFailed, err)
	}
	if err := complianceevidence.ValidateCanonicalJSON(body); err != nil {
		return nil, fmt.Errorf("%w: canonical target state evidence: %w", constants.ErrEvaluationReportPersistFailed, err)
	}
	artifactType := string(complianceevidence.ArtifactTypeEvalObservation)
	artifactID := complianceevidence.ContentReferenceForBody(artifactType, body)
	_, digest, ok := complianceevidence.ParseExpectedContentReference(artifactID, artifactType)
	if !ok {
		return nil, fmt.Errorf("%w: target state content address is invalid", constants.ErrEvaluationReportPersistFailed)
	}
	directory := evaluationEvidenceDir(evidence.RunID)
	if err := s.files.MkdirAll(ctx, directory, constants.PermDirStandard); err != nil {
		return nil, fmt.Errorf("%w: create evidence directory: %w", constants.ErrEvaluationReportPersistFailed, err)
	}
	if err := s.files.WriteFile(ctx, filepath.Join(directory, digest+constants.FileExtJSON), body, constants.PermFileReadOnly); err != nil {
		return nil, fmt.Errorf("%w: write target state evidence: %w", constants.ErrEvaluationReportPersistFailed, err)
	}
	return &compliancev1.ComplianceEvidenceReference{
		ArtifactId: artifactID, ArtifactType: artifactType, Sha256: digest, MediaType: constants.MediaTypeJSON,
		RunId: evidence.RunID, ScenarioId: evidence.ScenarioID, AttemptId: evidence.AttemptID,
	}, nil
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
