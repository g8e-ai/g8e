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

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	complianceevidence "github.com/g8e-ai/g8e/v2/internal/services/compliance/evidence"
	compliancev1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/compliance/v1"
	evalv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/eval/v1"
)

const (
	campaignVerificationSchemaVersion = "2.0.0"
	campaignVerificationArtifactType  = "campaign-verification-report"
)

// CampaignRunVerifier independently verifies all persisted terminal assignment
// results for one campaign run.
type CampaignRunVerifier struct {
	assignmentVerifier        *CampaignAssignmentVerifier
	providerObservationReader *CampaignProviderObservationReader
	providerObservationPolicy ProviderObservationPolicy
	modelProvenanceReader     *CampaignModelProvenanceReader
	modelProvenancePolicy     ModelProvenancePolicy
	now                       func() time.Time
}

func NewCampaignRunVerifier(now func() time.Time) *CampaignRunVerifier {
	if now == nil {
		now = time.Now
	}
	return &CampaignRunVerifier{
		assignmentVerifier: NewCampaignAssignmentVerifier(now),
		now:                now,
	}
}

// WithProviderObservationReader enables provider-boundary observation coverage
// checks during run verification.
func (v *CampaignRunVerifier) WithProviderObservationReader(reader *CampaignProviderObservationReader, policy ProviderObservationPolicy) *CampaignRunVerifier {
	if v == nil {
		return v
	}
	v.providerObservationReader = reader
	v.providerObservationPolicy = policy
	return v
}

// WithModelProvenanceReader enables model provenance attestation coverage
// checks during run verification.
func (v *CampaignRunVerifier) WithModelProvenanceReader(reader *CampaignModelProvenanceReader, policy ModelProvenancePolicy) *CampaignRunVerifier {
	if v == nil {
		return v
	}
	v.modelProvenanceReader = reader
	v.modelProvenancePolicy = policy
	return v
}

// VerifyRun recomputes assignment verification for every terminal assignment
// with a persisted result and trace in one run.
func (v *CampaignRunVerifier) VerifyRun(ctx context.Context, store *Store, runID string, catalog *evalv1.EvaluationScenarioCatalog, artifacts map[string]ScenarioArtifacts) (*evalv1.EvaluationVerificationReport, error) {
	if v == nil || store == nil || runID == "" || catalog == nil {
		return nil, fmt.Errorf("evaluation: verify campaign run: %w", constants.ErrMissingRequiredField)
	}
	assignments, err := store.ListAssignments(ctx, runID)
	if err != nil {
		return nil, err
	}
	report := &evalv1.EvaluationVerificationReport{
		SchemaVersion: CampaignSchemaVersion,
		ReportId:      runID,
		RunId:         runID,
		VerifiedAt:    timestamppb.New(v.now().UTC()),
	}
	failures := make([]string, 0)
	verifiedCount := 0
	for _, assignment := range assignments {
		if assignment == nil {
			continue
		}
		if !isTerminalAssignmentLifecycle(assignment.GetLifecycleStatus()) {
			continue
		}
		if exists, err := store.AssignmentResultExists(ctx, runID, assignment.GetAssignmentId()); err != nil {
			return nil, err
		} else if !exists {
			failures = append(failures, fmt.Sprintf("assignment %s is terminal without a persisted result", assignment.GetAssignmentId()))
			continue
		}
		result, err := store.LoadAssignmentResult(ctx, runID, assignment.GetAssignmentId())
		if err != nil {
			failures = append(failures, fmt.Sprintf("assignment %s result load failed: %v", assignment.GetAssignmentId(), err))
			continue
		}
		trace, err := LoadAssignmentTraceEvidence(ctx, store.files, runID, assignment.GetAssignmentId())
		if err != nil {
			failures = append(failures, fmt.Sprintf("assignment %s trace load failed: %v", assignment.GetAssignmentId(), err))
			continue
		}
		artifact, ok := artifacts[assignment.GetScenarioId()]
		if !ok {
			failures = append(failures, fmt.Sprintf("assignment %s missing scenario artifacts", assignment.GetAssignmentId()))
			continue
		}
		var scenarioInput ScenarioInputFixture
		if err := json.Unmarshal(artifact.Input.Body, &scenarioInput); err != nil {
			failures = append(failures, fmt.Sprintf("assignment %s scenario input decode failed: %v", assignment.GetAssignmentId(), err))
			continue
		}
		var scenarioGold ScenarioGoldCriteria
		if err := json.Unmarshal(artifact.Gold.Body, &scenarioGold); err != nil {
			failures = append(failures, fmt.Sprintf("assignment %s scenario gold decode failed: %v", assignment.GetAssignmentId(), err))
			continue
		}
		gradingMethod, err := scenarioGradingMethodForAssignment(catalog, assignment)
		if err != nil {
			failures = append(failures, err.Error())
			continue
		}
		scenarioTools, err := scenarioToolsForAssignment(catalog, assignment)
		if err != nil {
			failures = append(failures, err.Error())
			continue
		}
		assignmentReport, err := v.assignmentVerifier.Verify(ctx, CampaignAssignmentVerificationRequest{
			Assignment:                assignment,
			Result:                    result,
			ScenarioInput:             scenarioInput,
			ScenarioGold:              scenarioGold,
			ScenarioTools:             scenarioTools,
			GradingMethod:             gradingMethod,
			Trace:                     trace,
			ProviderObservationReader: v.providerObservationReader,
			ProviderObservationPolicy: v.providerObservationPolicy,
			ModelProvenanceReader:     v.modelProvenanceReader,
			ModelProvenancePolicy:     v.modelProvenancePolicy,
		})
		if err != nil {
			return nil, err
		}
		verifiedCount++
		if assignmentReport.GetStatus() != evalv1.EvaluationVerdictStatus_EVALUATION_VERDICT_STATUS_PASS {
			for _, reason := range assignmentReport.GetFailureReasons() {
				failures = append(failures, fmt.Sprintf("assignment %s: %s", assignment.GetAssignmentId(), reason))
			}
		}
	}
	if verifiedCount == 0 {
		failures = append(failures, "no terminal assignments with persisted results were available for verification")
	}
	return finalizeCampaignVerificationReport(report, failures), nil
}

func bindCampaignVerificationReport(report *evalv1.EvaluationVerificationReport, run *evalv1.EvaluationRun, spec *evalv1.EvaluationCampaignSpec, catalog *evalv1.EvaluationScenarioCatalog, assignments []*evalv1.EvaluationAssignment, results map[string]*evalv1.EvaluationAssignmentResult, policy CampaignVerificationPolicy) (*evalv1.EvaluationVerificationReport, *RunVerificationApplicability, error) {
	if report == nil || run == nil || spec == nil || catalog == nil || policy.VerifierReleaseVersion == "" || report.GetRunId() != run.GetRunId() {
		return nil, nil, fmt.Errorf("evaluation: bind campaign verification report: %w", constants.ErrMissingRequiredField)
	}
	binding := run.GetCampaignBinding()
	if binding == nil || binding.GetCampaignId() != spec.GetCampaignId() || binding.GetCampaignDigest() != spec.GetCampaignDigest() || binding.GetCatalogDigest() != spec.GetCatalogDigest() || binding.GetModelRegistryDigest() != spec.GetModelRegistryDigest() || !sameReference(binding.GetCatalogRef(), spec.GetCatalogRef()) || spec.GetCatalogDigest() != catalog.GetCatalogDigest() || !sameReference(spec.GetCatalogRef(), catalog.GetCatalogRef()) {
		return nil, nil, fmt.Errorf("evaluation: bind campaign verification report: %w", constants.ErrEvidenceScopeMismatch)
	}
	bound := proto.Clone(report).(*evalv1.EvaluationVerificationReport)
	population, err := BuildRunVerificationApplicability(run, spec, catalog, assignments, results, bound)
	if err != nil {
		return nil, nil, err
	}
	populationDigest, err := ComputeVerifiedPopulationDigest(population.Population)
	if err != nil {
		return nil, nil, err
	}
	bound.SchemaVersion = campaignVerificationSchemaVersion
	bound.VerifierReleaseVersion = policy.VerifierReleaseVersion
	bound.VerifierContractVersion = constants.CampaignVerifierVersion
	bound.ProviderObservationPolicy = providerObservationPolicyProto(policy.ProviderObservation)
	bound.ModelProvenancePolicy = modelProvenancePolicyProto(policy.ModelProvenance)
	bound.VerifiedPopulationDigest = populationDigest
	bound.ExpectedAssignmentCount = population.ExpectedAssignmentCount
	bound.VerifiedAssignmentCount = population.VerifiedAssignmentCount
	bound.CampaignDigest = spec.GetCampaignDigest()
	bound.CatalogDigest = spec.GetCatalogDigest()
	bound.ModelRegistryDigest = spec.GetModelRegistryDigest()
	bound.ReportDigestRef = nil
	reportDigest, err := digestProto(bound)
	if err != nil {
		return nil, nil, fmt.Errorf("evaluation: bind campaign verification report digest: %w", err)
	}
	bound.ReportDigestRef = &compliancev1.ComplianceEvidenceReference{
		ArtifactId:         fmt.Sprintf("%s:sha256:%s", campaignVerificationArtifactType, reportDigest),
		ArtifactType:       campaignVerificationArtifactType,
		Sha256:             reportDigest,
		MediaType:          constants.MediaTypeJSON,
		SchemaRef:          "g8e.eval.v1.EvaluationVerificationReport",
		ProducerIdentity:   constants.CampaignVerifierID,
		RunId:              run.GetRunId(),
		VerificationStatus: string(complianceevidence.VerificationStatusVerified),
		VerifierId:         constants.CampaignVerifierID,
		VerifierVersion:    constants.CampaignVerifierVersion,
		ProducedAt:         bound.GetVerifiedAt(),
		VerifiedAt:         bound.GetVerifiedAt(),
	}
	applicability, err := BuildRunVerificationApplicability(run, spec, catalog, assignments, results, bound)
	if err != nil {
		return nil, nil, err
	}
	if !applicability.Applicable {
		return nil, nil, fmt.Errorf("evaluation: bind campaign verification report: %w", constants.ErrEvidenceScopeMismatch)
	}
	return bound, applicability, nil
}

type CampaignVerificationPolicy struct {
	VerifierReleaseVersion string
	ProviderObservation    ProviderObservationPolicy
	ModelProvenance        ModelProvenancePolicy
	AssessmentTime         func() time.Time
}

func BindStoredCampaignVerificationReport(ctx context.Context, store *Store, report *evalv1.EvaluationVerificationReport, policy CampaignVerificationPolicy) (*evalv1.EvaluationVerificationReport, *RunVerificationApplicability, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if store == nil || store.files == nil || report == nil || !complianceevidence.ValidPathElement(report.GetRunId()) || policy.VerifierReleaseVersion == "" {
		return nil, nil, fmt.Errorf("evaluation: bind stored campaign verification report: %w", constants.ErrMissingRequiredField)
	}
	run, err := store.LoadRun(ctx, report.GetRunId())
	if err != nil {
		return nil, nil, err
	}
	binding := run.GetCampaignBinding()
	if binding == nil || binding.GetCampaignId() == "" {
		return nil, nil, fmt.Errorf("evaluation: bind stored campaign verification report: %w", constants.ErrMissingRequiredField)
	}
	spec, err := store.LoadCampaignSpec(ctx, binding.GetCampaignId())
	if err != nil {
		return nil, nil, err
	}
	catalog, err := store.LoadScenarioCatalog(ctx, binding.GetCampaignId())
	if err != nil {
		return nil, nil, err
	}
	assignments, err := store.ListAssignments(ctx, report.GetRunId())
	if err != nil {
		return nil, nil, err
	}
	results, err := loadCampaignAssignmentResults(ctx, store, report.GetRunId(), assignments)
	if err != nil {
		return nil, nil, err
	}
	return bindCampaignVerificationReport(report, run, spec, catalog, assignments, results, policy)
}

func providerObservationPolicyProto(policy ProviderObservationPolicy) evalv1.EvaluationWitnessPolicy {
	if policy == ProviderObservationPolicyStrict {
		return evalv1.EvaluationWitnessPolicy_EVALUATION_WITNESS_POLICY_STRICT
	}
	return evalv1.EvaluationWitnessPolicy_EVALUATION_WITNESS_POLICY_INTERIM
}

func modelProvenancePolicyProto(policy ModelProvenancePolicy) evalv1.EvaluationWitnessPolicy {
	if policy == ModelProvenancePolicyStrict {
		return evalv1.EvaluationWitnessPolicy_EVALUATION_WITNESS_POLICY_STRICT
	}
	return evalv1.EvaluationWitnessPolicy_EVALUATION_WITNESS_POLICY_INTERIM
}

type campaignRunVerificationResult struct {
	Run           *evalv1.EvaluationRun
	Report        *evalv1.EvaluationVerificationReport
	Population    *CampaignPopulationReport
	Applicability *RunVerificationApplicability
}

func verifyCampaignRunReadOnly(ctx context.Context, store *Store, runID string, policy CampaignVerificationPolicy) (*campaignRunVerificationResult, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if store == nil || store.files == nil || !complianceevidence.ValidPathElement(runID) || policy.VerifierReleaseVersion == "" || policy.AssessmentTime == nil {
		return nil, fmt.Errorf("evaluation: verify campaign run read-only: %w", constants.ErrMissingRequiredField)
	}
	run, err := store.LoadRun(ctx, runID)
	if err != nil {
		return nil, err
	}
	campaignID := run.GetCampaignBinding().GetCampaignId()
	spec, err := store.LoadCampaignSpec(ctx, campaignID)
	if err != nil {
		return nil, err
	}
	catalog, err := store.LoadScenarioCatalog(ctx, campaignID)
	if err != nil {
		return nil, err
	}
	artifacts, err := store.LoadScenarioArtifacts(ctx, campaignID, catalog)
	if err != nil {
		return nil, fmt.Errorf("evaluation: verify campaign frozen scenario inputs: %w", err)
	}
	observationReader, err := NewCampaignProviderObservationReader(store.files)
	if err != nil {
		return nil, err
	}
	provenanceReader, err := NewCampaignModelProvenanceReader(store.files)
	if err != nil {
		return nil, err
	}
	verifier := NewCampaignRunVerifier(policy.AssessmentTime).
		WithProviderObservationReader(observationReader, policy.ProviderObservation).
		WithModelProvenanceReader(provenanceReader, policy.ModelProvenance)
	report, err := verifier.VerifyRun(ctx, store, runID, catalog, artifacts)
	if err != nil {
		return nil, err
	}
	population, err := NewCampaignPopulationAccountant(policy.AssessmentTime).AccountRun(ctx, store, runID, catalog)
	if err != nil {
		return nil, err
	}
	assignments, err := store.ListAssignments(ctx, runID)
	if err != nil {
		return nil, err
	}
	results, err := loadCampaignAssignmentResults(ctx, store, runID, assignments)
	if err != nil {
		return nil, err
	}
	bound, applicability, err := bindCampaignVerificationReport(report, run, spec, catalog, assignments, results, policy)
	if err != nil {
		return nil, err
	}
	return &campaignRunVerificationResult{Run: run, Report: bound, Population: population, Applicability: applicability}, nil
}

func loadCampaignAssignmentResults(ctx context.Context, store *Store, runID string, assignments []*evalv1.EvaluationAssignment) (map[string]*evalv1.EvaluationAssignmentResult, error) {
	results := make(map[string]*evalv1.EvaluationAssignmentResult)
	for _, assignment := range assignments {
		if assignment == nil {
			continue
		}
		exists, err := store.AssignmentResultExists(ctx, runID, assignment.GetAssignmentId())
		if err != nil {
			return nil, err
		}
		if !exists {
			continue
		}
		result, err := store.LoadAssignmentResult(ctx, runID, assignment.GetAssignmentId())
		if err != nil {
			return nil, err
		}
		results[assignment.GetAssignmentId()] = result
	}
	return results, nil
}

func isTerminalAssignmentLifecycle(status evalv1.EvaluationAssignmentLifecycleStatus) bool {
	switch status {
	case evalv1.EvaluationAssignmentLifecycleStatus_EVALUATION_ASSIGNMENT_LIFECYCLE_STATUS_QUEUED,
		evalv1.EvaluationAssignmentLifecycleStatus_EVALUATION_ASSIGNMENT_LIFECYCLE_STATUS_RUNNING,
		evalv1.EvaluationAssignmentLifecycleStatus_EVALUATION_ASSIGNMENT_LIFECYCLE_STATUS_UNSPECIFIED:
		return false
	default:
		return true
	}
}

// LoadCampaignVerification reads one persisted run-level campaign verification report.
func (s *Store) LoadCampaignVerification(ctx context.Context, runID string) (*evalv1.EvaluationVerificationReport, error) {
	if s == nil || s.files == nil || !complianceevidence.ValidPathElement(runID) {
		return nil, fmt.Errorf("%w: file service and run ID are required", constants.ErrEvidenceArtifactMalformed)
	}
	path := filepath.Join(evaluationRunDir(runID), constants.CampaignVerificationFilename)
	body, err := s.files.ReadFile(ctx, path)
	if err != nil {
		return nil, fmt.Errorf("evaluation: read campaign verification report: %w", err)
	}
	report := &evalv1.EvaluationVerificationReport{}
	if err := evalv1.UnmarshalCanonical(body, report); err != nil {
		return nil, fmt.Errorf("%w: canonical campaign verification report: %v", constants.ErrEvidenceArtifactMalformed, err)
	}
	if report.GetRunId() != runID {
		return nil, fmt.Errorf("%w: campaign verification report binding does not match requested run", constants.ErrEvidenceScopeMismatch)
	}
	return report, nil
}

// SaveCampaignVerification persists one run-level campaign verification report.
func (s *Store) SaveCampaignVerification(ctx context.Context, runID string, report *evalv1.EvaluationVerificationReport) error {
	if s == nil || s.files == nil || !complianceevidence.ValidPathElement(runID) || report == nil || report.GetRunId() != runID {
		return fmt.Errorf("%w: campaign verification report binding is invalid", constants.ErrEvaluationReportPersistFailed)
	}
	body, err := evalv1.MarshalCanonical(report)
	if err != nil {
		return fmt.Errorf("%w: canonicalize campaign verification report: %w", constants.ErrEvaluationReportPersistFailed, err)
	}
	path := filepath.Join(evaluationRunDir(runID), constants.CampaignVerificationFilename)
	if err := s.files.MkdirAll(ctx, filepath.Dir(path), constants.PermDirStandard); err != nil {
		return fmt.Errorf("%w: create campaign verification directory: %w", constants.ErrEvaluationReportPersistFailed, err)
	}
	if err := s.files.WriteFile(ctx, path, body, constants.PermFileReadOnly); err != nil {
		return fmt.Errorf("%w: write campaign verification report: %w", constants.ErrEvaluationReportPersistFailed, err)
	}
	return nil
}
