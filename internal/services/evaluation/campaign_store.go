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
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"sort"
	"strings"

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

// SaveRun persists one canonical model-campaign run record.
func (s *Store) SaveRun(ctx context.Context, run *evalv1.EvaluationRun) error {
	if s == nil || s.files == nil || run == nil || !complianceevidence.ValidPathElement(run.GetRunId()) {
		return fmt.Errorf("%w: run and run ID are required", constants.ErrEvaluationReportPersistFailed)
	}
	if run.GetSchemaVersion() != CampaignSchemaVersion {
		return fmt.Errorf("%w: unsupported run schema version", constants.ErrEvaluationReportPersistFailed)
	}
	if run.GetCampaignBinding() == nil || run.GetCampaignBinding().GetCampaignId() == "" {
		return fmt.Errorf("%w: model campaign binding is required", constants.ErrEvaluationReportPersistFailed)
	}
	body, err := evalv1.MarshalCanonical(run)
	if err != nil {
		return fmt.Errorf("%w: canonicalize run: %w", constants.ErrEvaluationReportPersistFailed, err)
	}
	path := runStatePath(run.GetRunId())
	if err := s.files.MkdirAll(ctx, filepath.Dir(path), constants.PermDirStandard); err != nil {
		return fmt.Errorf("%w: create run directory: %w", constants.ErrEvaluationReportPersistFailed, err)
	}
	if err := s.files.WriteFile(ctx, path, body, constants.PermFileReadOnly); err != nil {
		return fmt.Errorf("%w: write run: %w", constants.ErrEvaluationReportPersistFailed, err)
	}
	return nil
}

// RunExists reports whether the canonical run record exists on the host store.
func (s *Store) RunExists(ctx context.Context, runID string) (bool, error) {
	if s == nil || s.files == nil || !complianceevidence.ValidPathElement(runID) {
		return false, fmt.Errorf("%w: file service and run ID are required", constants.ErrEvidenceArtifactMalformed)
	}
	exists, err := s.files.FileExists(ctx, runStatePath(runID))
	if err != nil {
		return false, fmt.Errorf("evaluation: run exists: %w", err)
	}
	return exists, nil
}

// LoadRun reads one persisted model-campaign run record.
func (s *Store) LoadRun(ctx context.Context, runID string) (*evalv1.EvaluationRun, error) {
	if s == nil || s.files == nil || !complianceevidence.ValidPathElement(runID) {
		return nil, fmt.Errorf("%w: file service and run ID are required", constants.ErrEvidenceArtifactMalformed)
	}
	body, err := s.files.ReadFile(ctx, runStatePath(runID))
	if err != nil {
		return nil, fmt.Errorf("evaluation: read run: %w", err)
	}
	run := &evalv1.EvaluationRun{}
	if err := evalv1.UnmarshalCanonical(body, run); err != nil {
		return nil, fmt.Errorf("%w: canonical run: %v", constants.ErrEvidenceArtifactMalformed, err)
	}
	if run.GetRunId() != runID {
		return nil, fmt.Errorf("%w: run ID does not match requested run", constants.ErrEvidenceScopeMismatch)
	}
	return run, nil
}

// ListAssignments returns all persisted assignments for one run in deterministic order.
func (s *Store) ListAssignments(ctx context.Context, runID string) ([]*evalv1.EvaluationAssignment, error) {
	if s == nil || s.files == nil || !complianceevidence.ValidPathElement(runID) {
		return nil, fmt.Errorf("%w: file service and run ID are required", constants.ErrEvidenceArtifactMalformed)
	}
	dir := filepath.Join(evaluationRunDir(runID), constants.EvaluationAssignmentsDirname)
	entries, err := s.files.ReadDir(ctx, dir)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) || errors.Is(err, constants.ErrNotFound) {
			return nil, nil
		}
		return nil, fmt.Errorf("evaluation: list assignments: %w", err)
	}
	assignments := make([]*evalv1.EvaluationAssignment, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), constants.FileExtJSON) || strings.HasSuffix(entry.Name(), "-result"+constants.FileExtJSON) || strings.HasSuffix(entry.Name(), "-trace"+constants.FileExtJSON) {
			continue
		}
		assignmentID := strings.TrimSuffix(entry.Name(), constants.FileExtJSON)
		assignment, err := s.LoadAssignment(ctx, runID, assignmentID)
		if err != nil {
			return nil, err
		}
		assignments = append(assignments, assignment)
	}
	sortAssignmentsDeterministic(assignments)
	return assignments, nil
}

// AssignmentResultExists reports whether a terminal assignment result is persisted.
func (s *Store) AssignmentResultExists(ctx context.Context, runID, assignmentID string) (bool, error) {
	if s == nil || s.files == nil || !complianceevidence.ValidPathElement(runID) || !complianceevidence.ValidPathElement(assignmentID) {
		return false, fmt.Errorf("%w: file service, run ID, and assignment ID are required", constants.ErrEvidenceArtifactMalformed)
	}
	exists, err := s.files.FileExists(ctx, assignmentResultPath(runID, assignmentID))
	if err != nil {
		return false, fmt.Errorf("evaluation: assignment result exists: %w", err)
	}
	return exists, nil
}

func sortAssignmentsDeterministic(assignments []*evalv1.EvaluationAssignment) {
	sort.Slice(assignments, func(i, j int) bool {
		return assignments[i].GetDeterministicIdentity() < assignments[j].GetDeterministicIdentity()
	})
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

// SaveAssignmentTrace persists one imported g8ee assignment trace body.
func (s *Store) SaveAssignmentTrace(ctx context.Context, runID, assignmentID string, body []byte) error {
	if s == nil || s.files == nil || !complianceevidence.ValidPathElement(runID) || !complianceevidence.ValidPathElement(assignmentID) || len(body) == 0 {
		return fmt.Errorf("%w: assignment trace, run ID, assignment ID, and body are required", constants.ErrEvaluationReportPersistFailed)
	}
	if err := complianceevidence.ValidateCanonicalJSON(body); err != nil {
		return fmt.Errorf("%w: assignment trace is not canonical JSON: %v", constants.ErrEvaluationReportPersistFailed, err)
	}
	path := assignmentTracePath(runID, assignmentID)
	if err := s.files.MkdirAll(ctx, filepath.Dir(path), constants.PermDirStandard); err != nil {
		return fmt.Errorf("%w: create assignment trace directory: %w", constants.ErrEvaluationReportPersistFailed, err)
	}
	if err := s.files.WriteFile(ctx, path, body, constants.PermFileReadOnly); err != nil {
		return fmt.Errorf("%w: write assignment trace: %w", constants.ErrEvaluationReportPersistFailed, err)
	}
	return nil
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

// LoadAssignmentTrace reads one persisted imported assignment trace.
func (s *Store) LoadAssignmentTrace(ctx context.Context, runID, assignmentID string) (map[string]any, error) {
	if s == nil || s.files == nil {
		return nil, fmt.Errorf("evaluation: load assignment trace: %w", constants.ErrMissingRequiredField)
	}
	return LoadAssignmentTraceEvidence(ctx, s.files, runID, assignmentID)
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

func campaignsRootDir() string {
	return filepath.Join(constants.DataDirname, constants.EvaluationDirname, constants.EvaluationCampaignsDirname)
}

func evaluationRunsRootDir() string {
	return filepath.Join(constants.DataDirname, constants.EvaluationDirname, constants.EvaluationRunsDirname)
}

type RunKind string

const (
	RunKindNative      RunKind = "native"
	RunKindCampaign    RunKind = "campaign"
	RunKindIncomplete  RunKind = "incomplete"
	RunKindUnsupported RunKind = "unsupported"
	RunKindMalformed   RunKind = "malformed"
)

type RunInventoryEntry struct {
	RunID  string
	Kind   RunKind
	Reason string
}

func (s *Store) InspectRun(ctx context.Context, runID string) (RunInventoryEntry, error) {
	if s == nil || s.files == nil || !complianceevidence.ValidPathElement(runID) {
		return RunInventoryEntry{}, fmt.Errorf("%w: file service and run ID are required", constants.ErrEvidenceArtifactMalformed)
	}
	entry := RunInventoryEntry{RunID: runID}
	nativeMarker, err := s.files.FileExists(ctx, filepath.Join(evaluationRunDir(runID), constants.EvaluationReportFilename))
	if err != nil {
		return RunInventoryEntry{}, fmt.Errorf("evaluation: inspect native run marker: %w", err)
	}
	campaignMarker, err := s.files.FileExists(ctx, runStatePath(runID))
	if err != nil {
		return RunInventoryEntry{}, fmt.Errorf("evaluation: inspect campaign run marker: %w", err)
	}
	if nativeMarker && campaignMarker {
		entry.Kind = RunKindMalformed
		entry.Reason = "native and campaign markers are both present"
		return entry, nil
	}
	if !nativeMarker && !campaignMarker {
		entry.Kind = RunKindIncomplete
		entry.Reason = "native and campaign markers are both missing"
		return entry, nil
	}
	if nativeMarker {
		report, loadErr := s.LoadReport(ctx, runID)
		if loadErr != nil {
			entry.Kind = RunKindMalformed
			entry.Reason = "native evaluation report is malformed"
			return entry, nil
		}
		run := report.GetRun()
		if report.GetSchemaVersion() != RegistryVersion || run == nil || run.GetSchemaVersion() != RegistryVersion || run.GetSuiteRef().GetId() != CoreExecutionBoundarySuiteID || run.GetSuiteRef().GetVersion() != CoreExecutionBoundarySuiteVersion {
			entry.Kind = RunKindUnsupported
			entry.Reason = "native evaluation suite or schema is unsupported"
			return entry, nil
		}
		entry.Kind = RunKindNative
		return entry, nil
	}
	run, loadErr := s.LoadRun(ctx, runID)
	if loadErr != nil || run.GetCampaignBinding() == nil || run.GetCampaignBinding().GetCampaignId() == "" {
		entry.Kind = RunKindMalformed
		entry.Reason = "campaign run record is malformed"
		return entry, nil
	}
	if run.GetSchemaVersion() != CampaignSchemaVersion {
		entry.Kind = RunKindUnsupported
		entry.Reason = "campaign run schema is unsupported"
		return entry, nil
	}
	entry.Kind = RunKindCampaign
	return entry, nil
}

func (s *Store) ListRunInventory(ctx context.Context) ([]RunInventoryEntry, error) {
	if s == nil || s.files == nil {
		return nil, fmt.Errorf("%w: file service is required", constants.ErrEvidenceArtifactMalformed)
	}
	entries, err := s.files.ReadDir(ctx, evaluationRunsRootDir())
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) || errors.Is(err, constants.ErrNotFound) {
			return nil, nil
		}
		return nil, fmt.Errorf("evaluation: list run inventory: %w", err)
	}
	inventory := make([]RunInventoryEntry, 0, len(entries))
	for _, candidate := range entries {
		if !candidate.IsDir() || !complianceevidence.ValidPathElement(candidate.Name()) {
			inventory = append(inventory, RunInventoryEntry{RunID: candidate.Name(), Kind: RunKindMalformed, Reason: "run inventory entry is not a valid directory"})
			continue
		}
		entry, inspectErr := s.InspectRun(ctx, candidate.Name())
		if inspectErr != nil {
			return nil, inspectErr
		}
		inventory = append(inventory, entry)
	}
	sort.Slice(inventory, func(left, right int) bool { return inventory[left].RunID < inventory[right].RunID })
	return inventory, nil
}

// CampaignListEntry summarizes one persisted campaign.
type CampaignListEntry struct {
	CampaignID             string
	ModelCount             int
	ScenarioCount          uint32
	RepetitionCount        uint32
	ModelRegistryDigest    string
	CatalogDigest          string
	HasHeterogeneousStacks bool
	RunIDs                 []string
}

// ListCampaigns returns all persisted campaign specs in deterministic order.
func (s *Store) ListCampaigns(ctx context.Context) ([]CampaignListEntry, error) {
	if s == nil || s.files == nil {
		return nil, fmt.Errorf("%w: file service is required", constants.ErrEvidenceArtifactMalformed)
	}
	runIDsByCampaign, err := s.listRunIDsByCampaign(ctx)
	if err != nil {
		return nil, err
	}
	entries, err := s.files.ReadDir(ctx, campaignsRootDir())
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) || errors.Is(err, constants.ErrNotFound) {
			return nil, nil
		}
		return nil, fmt.Errorf("evaluation: list campaigns: %w", err)
	}
	campaigns := make([]CampaignListEntry, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() || !complianceevidence.ValidPathElement(entry.Name()) {
			continue
		}
		spec, err := s.LoadCampaignSpec(ctx, entry.Name())
		if err != nil {
			continue
		}
		hasStacks, err := s.files.FileExists(ctx, heterogeneousStackSetPath(entry.Name()))
		if err != nil {
			return nil, fmt.Errorf("evaluation: list campaigns: %w", err)
		}
		campaigns = append(campaigns, CampaignListEntry{
			CampaignID:             spec.GetCampaignId(),
			ModelCount:             len(spec.GetModelRegistry()),
			ScenarioCount:          spec.GetScenarioCount(),
			RepetitionCount:        spec.GetRepetitionCount(),
			ModelRegistryDigest:    spec.GetModelRegistryDigest(),
			CatalogDigest:          spec.GetCatalogDigest(),
			HasHeterogeneousStacks: hasStacks,
			RunIDs:                 runIDsByCampaign[spec.GetCampaignId()],
		})
	}
	sort.Slice(campaigns, func(i, j int) bool {
		return campaigns[i].CampaignID < campaigns[j].CampaignID
	})
	return campaigns, nil
}

func (s *Store) listRunIDsByCampaign(ctx context.Context) (map[string][]string, error) {
	entries, err := s.files.ReadDir(ctx, evaluationRunsRootDir())
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) || errors.Is(err, constants.ErrNotFound) {
			return map[string][]string{}, nil
		}
		return nil, fmt.Errorf("evaluation: list campaign runs: %w", err)
	}
	byCampaign := make(map[string][]string)
	for _, entry := range entries {
		if !entry.IsDir() || !complianceevidence.ValidPathElement(entry.Name()) {
			continue
		}
		run, err := s.LoadRun(ctx, entry.Name())
		if err != nil {
			continue
		}
		campaignID := run.GetCampaignBinding().GetCampaignId()
		if campaignID == "" {
			continue
		}
		byCampaign[campaignID] = append(byCampaign[campaignID], run.GetRunId())
	}
	for campaignID := range byCampaign {
		sort.Strings(byCampaign[campaignID])
	}
	return byCampaign, nil
}

// SaveHeterogeneousStackSet persists one frozen heterogeneous stack set for a campaign.
func (s *Store) SaveHeterogeneousStackSet(ctx context.Context, campaignID string, stackSet *HeterogeneousStackSet) error {
	if s == nil || s.files == nil || stackSet == nil || !complianceevidence.ValidPathElement(campaignID) {
		return fmt.Errorf("%w: heterogeneous stack set and campaign ID are required", constants.ErrEvaluationReportPersistFailed)
	}
	if err := ValidateHeterogeneousStackSet(stackSet); err != nil {
		return fmt.Errorf("%w: %v", constants.ErrEvaluationReportPersistFailed, err)
	}
	body, err := marshalHeterogeneousStackSet(stackSet)
	if err != nil {
		return fmt.Errorf("%w: canonicalize heterogeneous stack set: %w", constants.ErrEvaluationReportPersistFailed, err)
	}
	path := heterogeneousStackSetPath(campaignID)
	if err := s.files.MkdirAll(ctx, filepath.Dir(path), constants.PermDirStandard); err != nil {
		return fmt.Errorf("%w: create campaign directory: %w", constants.ErrEvaluationReportPersistFailed, err)
	}
	if err := s.files.WriteFile(ctx, path, body, constants.PermFileReadOnly); err != nil {
		return fmt.Errorf("%w: write heterogeneous stack set: %w", constants.ErrEvaluationReportPersistFailed, err)
	}
	return nil
}

// LoadHeterogeneousStackSet reads one persisted heterogeneous stack set.
func (s *Store) LoadHeterogeneousStackSet(ctx context.Context, campaignID string) (*HeterogeneousStackSet, error) {
	if s == nil || s.files == nil || !complianceevidence.ValidPathElement(campaignID) {
		return nil, fmt.Errorf("%w: file service and campaign ID are required", constants.ErrEvidenceArtifactMalformed)
	}
	body, err := s.files.ReadFile(ctx, heterogeneousStackSetPath(campaignID))
	if err != nil {
		return nil, fmt.Errorf("evaluation: read heterogeneous stack set: %w", err)
	}
	stackSet, err := unmarshalHeterogeneousStackSet(body)
	if err != nil {
		return nil, fmt.Errorf("%w: canonical heterogeneous stack set: %v", constants.ErrEvidenceArtifactMalformed, err)
	}
	if stackSet.CampaignID != campaignID {
		return nil, fmt.Errorf("%w: heterogeneous stack set campaign ID does not match requested campaign", constants.ErrEvidenceScopeMismatch)
	}
	if err := ValidateHeterogeneousStackSet(stackSet); err != nil {
		return nil, fmt.Errorf("%w: %v", constants.ErrEvidenceArtifactMalformed, err)
	}
	return stackSet, nil
}

func marshalHeterogeneousStackSet(stackSet *HeterogeneousStackSet) ([]byte, error) {
	stackPayloads := make([]json.RawMessage, 0, len(stackSet.Stacks))
	for _, stack := range stackSet.Stacks {
		raw, err := evalv1.MarshalCanonical(stack)
		if err != nil {
			return nil, err
		}
		stackPayloads = append(stackPayloads, json.RawMessage(raw))
	}
	payload := struct {
		CampaignID     string                       `json:"campaign_id"`
		GenerationRule string                       `json:"generation_rule"`
		Seed           uint64                       `json:"seed"`
		SetDigest      string                       `json:"set_digest"`
		VariantIDs     []string                     `json:"variant_ids"`
		Stacks         []json.RawMessage            `json:"stacks"`
		Coverage       *HeterogeneousCoverageMatrix `json:"coverage"`
	}{
		CampaignID:     stackSet.CampaignID,
		GenerationRule: stackSet.GenerationRule,
		Seed:           stackSet.Seed,
		SetDigest:      stackSet.SetDigest,
		VariantIDs:     stackSet.VariantIDs,
		Stacks:         stackPayloads,
		Coverage:       stackSet.Coverage,
	}
	return json.Marshal(payload)
}

func unmarshalHeterogeneousStackSet(body []byte) (*HeterogeneousStackSet, error) {
	var payload struct {
		CampaignID     string                       `json:"campaign_id"`
		GenerationRule string                       `json:"generation_rule"`
		Seed           uint64                       `json:"seed"`
		SetDigest      string                       `json:"set_digest"`
		VariantIDs     []string                     `json:"variant_ids"`
		Stacks         []json.RawMessage            `json:"stacks"`
		Coverage       *HeterogeneousCoverageMatrix `json:"coverage"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, err
	}
	stacks := make([]*evalv1.HeterogeneousStackDefinition, 0, len(payload.Stacks))
	for _, raw := range payload.Stacks {
		stack := &evalv1.HeterogeneousStackDefinition{}
		if err := evalv1.UnmarshalCanonical(raw, stack); err != nil {
			return nil, err
		}
		stacks = append(stacks, stack)
	}
	return &HeterogeneousStackSet{
		CampaignID:     payload.CampaignID,
		GenerationRule: payload.GenerationRule,
		Seed:           payload.Seed,
		SetDigest:      payload.SetDigest,
		VariantIDs:     payload.VariantIDs,
		Stacks:         stacks,
		Coverage:       payload.Coverage,
	}, nil
}

func campaignSpecPath(campaignID string) string {
	return filepath.Join(campaignDir(campaignID), constants.EvaluationCampaignSpecFilename)
}

func heterogeneousStackSetPath(campaignID string) string {
	return filepath.Join(campaignDir(campaignID), constants.EvaluationHeterogeneousStackSetFilename)
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

func assignmentTracePath(runID, assignmentID string) string {
	return filepath.Join(evaluationRunDir(runID), constants.EvaluationAssignmentsDirname, assignmentID+"-trace"+constants.FileExtJSON)
}

func runStatePath(runID string) string {
	return filepath.Join(evaluationRunDir(runID), constants.EvaluationRunStateFilename)
}
