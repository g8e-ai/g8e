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
	complianceevidence "github.com/g8e-ai/g8e/v2/internal/services/compliance/evidence"
	evalv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/eval/v1"
)

// CampaignAssignmentVerificationRequest carries persisted assignment state for
// read-only independent verification.
type CampaignAssignmentVerificationRequest struct {
	Assignment                *evalv1.EvaluationAssignment
	Result                    *evalv1.EvaluationAssignmentResult
	ScenarioInput             ScenarioInputFixture
	ScenarioGold              ScenarioGoldCriteria
	ScenarioTools             ScenarioToolExpectations
	GradingMethod             evalv1.EvaluationGradingMethod
	Trace                     map[string]any
	ProviderObservationReader *CampaignProviderObservationReader
	ProviderObservationPolicy ProviderObservationPolicy
}

// CampaignAssignmentVerifier independently verifies one persisted assignment
// result without invoking inference, providers, tools, or mutation.
type CampaignAssignmentVerifier struct {
	now func() time.Time
}

func NewCampaignAssignmentVerifier(now func() time.Time) *CampaignAssignmentVerifier {
	if now == nil {
		now = time.Now
	}
	return &CampaignAssignmentVerifier{now: now}
}

// Verify recomputes assignment digests and deterministic grades from declared
// evidence and returns a typed verification report.
func (v *CampaignAssignmentVerifier) Verify(ctx context.Context, req CampaignAssignmentVerificationRequest) (*evalv1.EvaluationVerificationReport, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	verifiedAt := v.now().UTC()
	report := &evalv1.EvaluationVerificationReport{
		SchemaVersion: CampaignSchemaVersion,
		ReportId:      req.Result.GetAssignmentId(),
		RunId:         req.Result.GetRunId(),
		AssignmentId:  req.Result.GetAssignmentId(),
		VerifiedAt:    timestamppb.New(verifiedAt),
	}
	failures := make([]string, 0)
	if req.Assignment == nil || req.Result == nil {
		failures = append(failures, "assignment and result are required")
		return finalizeCampaignVerificationReport(report, failures), nil
	}
	if req.Assignment.GetAssignmentId() != req.Result.GetAssignmentId() || req.Assignment.GetRunId() != req.Result.GetRunId() {
		failures = append(failures, "assignment and result binding mismatch")
	}
	if err := ValidateAssignmentResultDigest(req.Result); err != nil {
		failures = append(failures, "result digest validation failed: "+err.Error())
	}
	if len(req.Trace) == 0 {
		failures = append(failures, "imported assignment trace is required for verification")
	} else if err := validateImportedTraceDigest(req.Trace); err != nil {
		failures = append(failures, "trace digest validation failed: "+err.Error())
	}
	designatedRole, err := designatedRoleFromAssignment(req.Assignment)
	if err != nil {
		failures = append(failures, err.Error())
	} else {
		recomputed, err := GradeHomogeneousScenario(ScenarioGradingRequest{
			AssignmentID:   req.Assignment.GetAssignmentId(),
			ScenarioID:     req.Assignment.GetScenarioId(),
			DesignatedRole: designatedRole,
			GradingMethod:  req.GradingMethod,
			ScenarioInput:  req.ScenarioInput,
			ScenarioGold:   req.ScenarioGold,
			ScenarioTools:  req.ScenarioTools,
			Trace:          req.Trace,
			Lifecycle:      req.Result.GetLifecycleStatus(),
		})
		if err != nil {
			failures = append(failures, "deterministic grade recomputation failed: "+err.Error())
		} else if !gradesEquivalent(req.Result.GetDeterministicGrades(), recomputed.DeterministicGrades) {
			failures = append(failures, "stored deterministic grades do not match recomputation")
		}
	}
	if req.ProviderObservationReader != nil && len(scoredModelInferences(req.Result)) > 0 {
		observationFailures, _ := req.ProviderObservationReader.VerifyAssignmentProviderObservations(ctx, req.Result, req.ProviderObservationPolicy)
		failures = append(failures, observationFailures...)
	}
	return finalizeCampaignVerificationReport(report, failures), nil
}

func finalizeCampaignVerificationReport(report *evalv1.EvaluationVerificationReport, failures []string) *evalv1.EvaluationVerificationReport {
	report.FailureReasons = failures
	report.FailureCount = uint32(len(failures))
	if len(failures) == 0 {
		report.Status = evalv1.EvaluationVerdictStatus_EVALUATION_VERDICT_STATUS_PASS
		return report
	}
	report.Status = evalv1.EvaluationVerdictStatus_EVALUATION_VERDICT_STATUS_FAIL
	return report
}

func validateImportedTraceDigest(trace map[string]any) error {
	if err := validateTraceDigest(trace); err != nil {
		return err
	}
	return nil
}

func designatedRoleFromAssignment(assignment *evalv1.EvaluationAssignment) (string, error) {
	homogeneous, ok := assignment.GetTarget().(*evalv1.EvaluationAssignment_Homogeneous)
	if !ok || homogeneous.Homogeneous == nil {
		return "", fmt.Errorf("evaluation: designated role lookup: homogeneous target required")
	}
	return modelCampaignRoleLabel(homogeneous.Homogeneous.GetDesignatedRole())
}

// LoadAssignmentTraceEvidence reads one persisted imported assignment trace.
func LoadAssignmentTraceEvidence(ctx context.Context, reader complianceevidence.ArtifactReader, runID, assignmentID string) (map[string]any, error) {
	if reader == nil || !complianceevidence.ValidPathElement(runID) || !complianceevidence.ValidPathElement(assignmentID) {
		return nil, fmt.Errorf("evaluation: load assignment trace evidence: %w", constants.ErrMissingRequiredField)
	}
	path := assignmentTracePath(runID, assignmentID)
	body, err := reader.ReadFile(ctx, path)
	if err != nil {
		return nil, err
	}
	if err := complianceevidence.ValidateCanonicalJSON(body); err != nil {
		return nil, fmt.Errorf("evaluation: load assignment trace evidence: %w", err)
	}
	trace := map[string]any{}
	if err := json.Unmarshal(body, &trace); err != nil {
		return nil, fmt.Errorf("evaluation: load assignment trace evidence: %w", err)
	}
	return trace, nil
}
