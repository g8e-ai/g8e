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

	"github.com/g8e-ai/g8e/v2/internal/constants"
	complianceevidence "github.com/g8e-ai/g8e/v2/internal/services/compliance/evidence"
	evalv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/eval/v1"
)

// SaveCampaignSpec persists one canonical campaign spec beneath the campaign
// directory after validating its digest binding.
func (s *Store) SaveCampaignSpec(ctx context.Context, spec *evalv1.EvaluationCampaignSpec) error {
	if s == nil || s.files == nil || spec == nil || !complianceevidence.ValidPathElement(spec.GetCampaignId()) {
		return fmt.Errorf("%w: campaign spec and campaign ID are required", constants.ErrEvaluationReportPersistFailed)
	}
	if spec.GetSchemaVersion() != CampaignSchemaVersion {
		return fmt.Errorf("%w: unsupported campaign spec schema version", constants.ErrEvaluationReportPersistFailed)
	}
	if err := ValidateCampaignSpecDigest(spec); err != nil {
		return fmt.Errorf("%w: %v", constants.ErrEvaluationReportPersistFailed, err)
	}
	body, err := evalv1.MarshalCanonical(spec)
	if err != nil {
		return fmt.Errorf("%w: canonicalize campaign spec: %w", constants.ErrEvaluationReportPersistFailed, err)
	}
	path := campaignSpecPath(spec.GetCampaignId())
	if err := s.files.MkdirAll(ctx, filepath.Dir(path), constants.PermDirStandard); err != nil {
		return fmt.Errorf("%w: create campaign directory: %w", constants.ErrEvaluationReportPersistFailed, err)
	}
	if err := s.files.WriteFile(ctx, path, body, constants.PermFileReadOnly); err != nil {
		return fmt.Errorf("%w: write campaign spec: %w", constants.ErrEvaluationReportPersistFailed, err)
	}
	return nil
}

// LoadCampaignSpec reads one persisted campaign spec.
func (s *Store) LoadCampaignSpec(ctx context.Context, campaignID string) (*evalv1.EvaluationCampaignSpec, error) {
	if s == nil || s.files == nil || !complianceevidence.ValidPathElement(campaignID) {
		return nil, fmt.Errorf("%w: file service and campaign ID are required", constants.ErrEvidenceArtifactMalformed)
	}
	body, err := s.files.ReadFile(ctx, campaignSpecPath(campaignID))
	if err != nil {
		return nil, fmt.Errorf("evaluation: read campaign spec: %w", err)
	}
	spec := &evalv1.EvaluationCampaignSpec{}
	if err := evalv1.UnmarshalCanonical(body, spec); err != nil {
		return nil, fmt.Errorf("%w: canonical campaign spec: %v", constants.ErrEvidenceArtifactMalformed, err)
	}
	if spec.GetCampaignId() != campaignID {
		return nil, fmt.Errorf("%w: campaign spec ID does not match requested campaign", constants.ErrEvidenceScopeMismatch)
	}
	if err := ValidateCampaignSpecDigest(spec); err != nil {
		return nil, fmt.Errorf("%w: %v", constants.ErrEvidenceArtifactMalformed, err)
	}
	return spec, nil
}

// SaveScenarioCatalog persists one canonical scenario catalog for a campaign.
func (s *Store) SaveScenarioCatalog(ctx context.Context, campaignID string, catalog *evalv1.EvaluationScenarioCatalog) error {
	if s == nil || s.files == nil || catalog == nil || !complianceevidence.ValidPathElement(campaignID) {
		return fmt.Errorf("%w: scenario catalog and campaign ID are required", constants.ErrEvaluationReportPersistFailed)
	}
	if catalog.GetSchemaVersion() != CampaignSchemaVersion {
		return fmt.Errorf("%w: unsupported scenario catalog schema version", constants.ErrEvaluationReportPersistFailed)
	}
	if err := ValidateScenarioCatalogDigest(catalog); err != nil {
		return fmt.Errorf("%w: %v", constants.ErrEvaluationReportPersistFailed, err)
	}
	body, err := evalv1.MarshalCanonical(catalog)
	if err != nil {
		return fmt.Errorf("%w: canonicalize scenario catalog: %w", constants.ErrEvaluationReportPersistFailed, err)
	}
	path := scenarioCatalogPath(campaignID)
	if err := s.files.MkdirAll(ctx, filepath.Dir(path), constants.PermDirStandard); err != nil {
		return fmt.Errorf("%w: create campaign directory: %w", constants.ErrEvaluationReportPersistFailed, err)
	}
	if err := s.files.WriteFile(ctx, path, body, constants.PermFileReadOnly); err != nil {
		return fmt.Errorf("%w: write scenario catalog: %w", constants.ErrEvaluationReportPersistFailed, err)
	}
	return nil
}

// LoadScenarioCatalog reads one persisted scenario catalog.
func (s *Store) LoadScenarioCatalog(ctx context.Context, campaignID string) (*evalv1.EvaluationScenarioCatalog, error) {
	if s == nil || s.files == nil || !complianceevidence.ValidPathElement(campaignID) {
		return nil, fmt.Errorf("%w: file service and campaign ID are required", constants.ErrEvidenceArtifactMalformed)
	}
	body, err := s.files.ReadFile(ctx, scenarioCatalogPath(campaignID))
	if err != nil {
		return nil, fmt.Errorf("evaluation: read scenario catalog: %w", err)
	}
	catalog := &evalv1.EvaluationScenarioCatalog{}
	if err := evalv1.UnmarshalCanonical(body, catalog); err != nil {
		return nil, fmt.Errorf("%w: canonical scenario catalog: %v", constants.ErrEvidenceArtifactMalformed, err)
	}
	if err := ValidateScenarioCatalogDigest(catalog); err != nil {
		return nil, fmt.Errorf("%w: %v", constants.ErrEvidenceArtifactMalformed, err)
	}
	return catalog, nil
}

// SaveAssignment persists one queued or running assignment record.
func (s *Store) SaveAssignment(ctx context.Context, assignment *evalv1.EvaluationAssignment) error {
	if s == nil || s.files == nil || assignment == nil || !complianceevidence.ValidPathElement(assignment.GetRunId()) || !complianceevidence.ValidPathElement(assignment.GetAssignmentId()) {
		return fmt.Errorf("%w: assignment, run ID, and assignment ID are required", constants.ErrEvaluationReportPersistFailed)
	}
	if assignment.GetSchemaVersion() != CampaignSchemaVersion {
		return fmt.Errorf("%w: unsupported assignment schema version", constants.ErrEvaluationReportPersistFailed)
	}
	identity, err := ComputeAssignmentDeterministicIdentity(assignment)
	if err != nil {
		return fmt.Errorf("%w: %v", constants.ErrEvaluationReportPersistFailed, err)
	}
	if assignment.GetDeterministicIdentity() != "" && assignment.GetDeterministicIdentity() != identity {
		return fmt.Errorf("%w: assignment deterministic identity mismatch", constants.ErrEvaluationReportPersistFailed)
	}
	assignment.DeterministicIdentity = identity
	body, err := evalv1.MarshalCanonical(assignment)
	if err != nil {
		return fmt.Errorf("%w: canonicalize assignment: %w", constants.ErrEvaluationReportPersistFailed, err)
	}
	path := assignmentPath(assignment.GetRunId(), assignment.GetAssignmentId())
	if err := s.files.MkdirAll(ctx, filepath.Dir(path), constants.PermDirStandard); err != nil {
		return fmt.Errorf("%w: create assignment directory: %w", constants.ErrEvaluationReportPersistFailed, err)
	}
	if err := s.files.WriteFile(ctx, path, body, constants.PermFileReadOnly); err != nil {
		return fmt.Errorf("%w: write assignment: %w", constants.ErrEvaluationReportPersistFailed, err)
	}
	return nil
}

// LoadAssignment reads one persisted assignment record.
func (s *Store) LoadAssignment(ctx context.Context, runID, assignmentID string) (*evalv1.EvaluationAssignment, error) {
	if s == nil || s.files == nil || !complianceevidence.ValidPathElement(runID) || !complianceevidence.ValidPathElement(assignmentID) {
		return nil, fmt.Errorf("%w: file service, run ID, and assignment ID are required", constants.ErrEvidenceArtifactMalformed)
	}
	body, err := s.files.ReadFile(ctx, assignmentPath(runID, assignmentID))
	if err != nil {
		return nil, fmt.Errorf("evaluation: read assignment: %w", err)
	}
	assignment := &evalv1.EvaluationAssignment{}
	if err := evalv1.UnmarshalCanonical(body, assignment); err != nil {
		return nil, fmt.Errorf("%w: canonical assignment: %v", constants.ErrEvidenceArtifactMalformed, err)
	}
	if assignment.GetRunId() != runID || assignment.GetAssignmentId() != assignmentID {
		return nil, fmt.Errorf("%w: assignment binding does not match requested run/assignment", constants.ErrEvidenceScopeMismatch)
	}
	expected, err := ComputeAssignmentDeterministicIdentity(assignment)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", constants.ErrEvidenceArtifactMalformed, err)
	}
	if assignment.GetDeterministicIdentity() != expected {
		return nil, fmt.Errorf("%w: assignment deterministic identity mismatch", constants.ErrEvidenceArtifactMalformed)
	}
	return assignment, nil
}

// SaveAssignmentResult persists one terminal assignment result.
func (s *Store) SaveAssignmentResult(ctx context.Context, result *evalv1.EvaluationAssignmentResult) error {
	if s == nil || s.files == nil || result == nil || !complianceevidence.ValidPathElement(result.GetRunId()) || !complianceevidence.ValidPathElement(result.GetAssignmentId()) {
		return fmt.Errorf("%w: assignment result, run ID, and assignment ID are required", constants.ErrEvaluationReportPersistFailed)
	}
	if result.GetSchemaVersion() != CampaignSchemaVersion {
		return fmt.Errorf("%w: unsupported assignment result schema version", constants.ErrEvaluationReportPersistFailed)
	}
	if err := ValidateAssignmentResultDigest(result); err != nil {
		return fmt.Errorf("%w: %v", constants.ErrEvaluationReportPersistFailed, err)
	}
	body, err := evalv1.MarshalCanonical(result)
	if err != nil {
		return fmt.Errorf("%w: canonicalize assignment result: %w", constants.ErrEvaluationReportPersistFailed, err)
	}
	path := assignmentResultPath(result.GetRunId(), result.GetAssignmentId())
	if err := s.files.MkdirAll(ctx, filepath.Dir(path), constants.PermDirStandard); err != nil {
		return fmt.Errorf("%w: create assignment result directory: %w", constants.ErrEvaluationReportPersistFailed, err)
	}
	if err := s.files.WriteFile(ctx, path, body, constants.PermFileReadOnly); err != nil {
		return fmt.Errorf("%w: write assignment result: %w", constants.ErrEvaluationReportPersistFailed, err)
	}
	return nil
}

// LoadAssignmentResult reads one persisted terminal assignment result.
func (s *Store) LoadAssignmentResult(ctx context.Context, runID, assignmentID string) (*evalv1.EvaluationAssignmentResult, error) {
	if s == nil || s.files == nil || !complianceevidence.ValidPathElement(runID) || !complianceevidence.ValidPathElement(assignmentID) {
		return nil, fmt.Errorf("%w: file service, run ID, and assignment ID are required", constants.ErrEvidenceArtifactMalformed)
	}
	body, err := s.files.ReadFile(ctx, assignmentResultPath(runID, assignmentID))
	if err != nil {
		return nil, fmt.Errorf("evaluation: read assignment result: %w", err)
	}
	result := &evalv1.EvaluationAssignmentResult{}
	if err := evalv1.UnmarshalCanonical(body, result); err != nil {
		return nil, fmt.Errorf("%w: canonical assignment result: %v", constants.ErrEvidenceArtifactMalformed, err)
	}
	if result.GetRunId() != runID || result.GetAssignmentId() != assignmentID {
		return nil, fmt.Errorf("%w: assignment result binding does not match requested run/assignment", constants.ErrEvidenceScopeMismatch)
	}
	if err := ValidateAssignmentResultDigest(result); err != nil {
		return nil, fmt.Errorf("%w: %v", constants.ErrEvidenceArtifactMalformed, err)
	}
	return result, nil
}

func campaignDir(campaignID string) string {
	return filepath.Join(constants.DataDirname, constants.EvaluationDirname, constants.EvaluationCampaignsDirname, campaignID)
}

func campaignSpecPath(campaignID string) string {
	return filepath.Join(campaignDir(campaignID), constants.EvaluationCampaignSpecFilename)
}

func scenarioCatalogPath(campaignID string) string {
	return filepath.Join(campaignDir(campaignID), constants.EvaluationScenarioCatalogFilename)
}

func assignmentPath(runID, assignmentID string) string {
	return filepath.Join(evaluationRunDir(runID), constants.EvaluationAssignmentsDirname, assignmentID+constants.FileExtJSON)
}

func assignmentResultPath(runID, assignmentID string) string {
	return filepath.Join(evaluationRunDir(runID), constants.EvaluationAssignmentsDirname, assignmentID+"-result"+constants.FileExtJSON)
}
