// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package evidence

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/services/compliance/catalog"
	compliancev1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/compliance/v1"
)

const (
	freshnessFresh         = "fresh"
	freshnessStale         = "stale"
	freshnessIncomplete    = "incomplete"
	freshnessNotApplicable = "not_applicable"
	statusSatisfied        = "satisfied"
	statusNotSatisfied     = "not_satisfied"
	failureMissingEvidence = "required verified evidence is missing"
	failureStaleEvidence   = "required verified evidence is stale"
	failureGraderFailed    = "required deterministic grader did not pass"
)

type AssertionGradingRequest struct {
	ScopeID     string
	WindowStart time.Time
	WindowEnd   time.Time
	EvaluatedAt time.Time
	Assertions  *compliancev1.ControlAssertionCatalog
	Graph       *EvidenceGraph
}

type assertionEvidence struct {
	fresh       []*EvidenceNode
	stale       []*EvidenceNode
	evidenceRef map[string]struct{}
	metricRef   map[string]struct{}
}

type metricResult struct {
	MetricID           string                           `json:"metric_id"`
	MetricVersion      string                           `json:"metric_version"`
	Value              *float64                         `json:"value"`
	Eligible           *bool                            `json:"eligible"`
	VerificationStatus string                           `json:"verification_status"`
	Passed             *bool                            `json:"passed"`
	GraderRef          *compliancev1.VersionedReference `json:"grader_ref"`
}

func GradeControlAssertions(ctx context.Context, request AssertionGradingRequest) ([]*compliancev1.ControlAssertionAssessment, error) {
	if err := validateAssertionGradingRequest(request); err != nil {
		return nil, err
	}
	assertions := append([]*compliancev1.ControlAssertionDefinition(nil), request.Assertions.GetAssertions()...)
	sort.Slice(assertions, func(i, j int) bool {
		return versionedReferenceKey(assertions[i].GetAssertionId(), assertions[i].GetAssertionVersion()) < versionedReferenceKey(assertions[j].GetAssertionId(), assertions[j].GetAssertionVersion())
	})
	result := make([]*compliancev1.ControlAssertionAssessment, 0, len(assertions))
	for _, assertion := range assertions {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		assessment, err := gradeControlAssertion(request, assertion)
		if err != nil {
			return nil, err
		}
		result = append(result, assessment)
	}
	return result, nil
}

func validateAssertionGradingRequest(request AssertionGradingRequest) error {
	if request.ScopeID == "" || request.Graph == nil || request.Assertions == nil || request.WindowStart.IsZero() || request.WindowEnd.IsZero() || request.EvaluatedAt.IsZero() || request.WindowEnd.Before(request.WindowStart) || request.EvaluatedAt.Before(request.WindowStart) || request.EvaluatedAt.After(request.WindowEnd) {
		return fmt.Errorf("%w: assertion grading request is incomplete", constants.ErrInvalidEvidenceGraph)
	}
	if !request.Graph.Valid() {
		return fmt.Errorf("%w: assertion grading requires a valid evidence graph", constants.ErrInvalidEvidenceGraph)
	}
	if err := catalog.ValidateAssertionCatalog(request.Assertions); err != nil {
		return fmt.Errorf("assertion grading catalog: %w", err)
	}
	return nil
}

func gradeControlAssertion(request AssertionGradingRequest, assertion *compliancev1.ControlAssertionDefinition) (*compliancev1.ControlAssertionAssessment, error) {
	candidates := collectAssertionEvidence(request, assertion)
	missing, stale, graderFailed := evaluateAssertionRequirements(assertion, candidates)
	status, freshness, failure := assertionOutcome(assertion.GetMissingEvidencePolicy(), missing, stale, graderFailed)
	evidenceLevel := assertion.GetMinimumEvidenceLevel()
	if missing && !stale {
		evidenceLevel = "L0"
	}
	assessment := &compliancev1.ControlAssertionAssessment{
		AssessmentId:    assertionAssessmentID(request.ScopeID, assertion),
		ScopeId:         request.ScopeID,
		AssertionRef:    &compliancev1.VersionedReference{Id: assertion.GetAssertionId(), Version: assertion.GetAssertionVersion()},
		Status:          status,
		EvidenceLevel:   evidenceLevel,
		EvaluatedAt:     timestamppb.New(request.EvaluatedAt),
		VerifierRef:     &compliancev1.VersionedReference{Id: constants.AssertionGraderID, Version: constants.AssertionGraderVersion},
		EvidenceRefs:    sortedKeys(candidates.evidenceRef),
		MetricRefs:      sortedKeys(candidates.metricRef),
		FreshnessStatus: freshness,
		FailureReason:   failure,
		Limitations:     []string{},
	}
	if status == statusNotSatisfied && failure == "" {
		assessment.FailureReason = failureGraderFailed
	}
	if err := catalog.ValidateAssertionAssessment(assessment, request.ScopeID, request.Assertions); err != nil {
		return nil, fmt.Errorf("assertion grading %s: %w", assertion.GetAssertionId(), err)
	}
	return assessment, nil
}

func collectAssertionEvidence(request AssertionGradingRequest, assertion *compliancev1.ControlAssertionDefinition) assertionEvidence {
	result := assertionEvidence{evidenceRef: make(map[string]struct{}), metricRef: make(map[string]struct{})}
	freshnessStart := request.EvaluatedAt.Add(-assertionValidationCycle(assertion.GetValidationCycle()))
	if request.WindowStart.After(freshnessStart) {
		freshnessStart = request.WindowStart
	}
	for _, node := range request.Graph.NodesByScope(request.ScopeID) {
		if node.VerificationStatus != VerificationStatusVerified {
			continue
		}
		if node.ProducedAt.Before(freshnessStart) || node.ProducedAt.After(request.WindowEnd) {
			result.stale = append(result.stale, node)
			continue
		}
		result.fresh = append(result.fresh, node)
	}
	return result
}

func assertionValidationCycle(cycle string) time.Duration {
	switch cycle {
	case "7d":
		return 7 * 24 * time.Hour
	case "90d":
		return 90 * 24 * time.Hour
	default:
		return 0
	}
}

func evaluateAssertionRequirements(assertion *compliancev1.ControlAssertionDefinition, candidates assertionEvidence) (bool, bool, bool) {
	missing := false
	stale := false
	graderFailed := false
	for _, requiredType := range assertion.GetRequiredEvidenceTypes() {
		fresh := matchingNodes(candidates.fresh, func(node *EvidenceNode) bool { return nodeMatchesEvidenceType(node, requiredType) })
		if len(fresh) == 0 {
			missing = true
			staleNodes := matchingNodes(candidates.stale, func(node *EvidenceNode) bool { return nodeMatchesEvidenceType(node, requiredType) })
			stale = stale || len(staleNodes) > 0
			addEvidenceReferences(candidates.evidenceRef, staleNodes)
			continue
		}
		addEvidenceReferences(candidates.evidenceRef, fresh)
	}
	for _, required := range assertion.GetRequiredVerifierRefs() {
		fresh := matchingNodes(candidates.fresh, func(node *EvidenceNode) bool { return nodeMatchesVerifier(node, required) })
		if len(fresh) == 0 {
			missing = true
			staleNodes := matchingNodes(candidates.stale, func(node *EvidenceNode) bool { return nodeMatchesVerifier(node, required) })
			stale = stale || len(staleNodes) > 0
			addEvidenceReferences(candidates.evidenceRef, staleNodes)
			continue
		}
		addEvidenceReferences(candidates.evidenceRef, fresh)
	}
	for _, required := range assertion.GetRequiredGraderRefs() {
		fresh, passed := matchingGraderNodes(candidates.fresh, required)
		if len(fresh) == 0 {
			missing = true
			staleNodes, _ := matchingGraderNodes(candidates.stale, required)
			stale = stale || len(staleNodes) > 0
			addEvidenceReferences(candidates.evidenceRef, staleNodes)
			addMetricReferences(candidates.metricRef, staleNodes)
			continue
		}
		addEvidenceReferences(candidates.evidenceRef, fresh)
		addMetricReferences(candidates.metricRef, fresh)
		graderFailed = graderFailed || !passed
	}
	return missing, stale, graderFailed
}

func assertionOutcome(missingPolicy string, missing, stale, graderFailed bool) (string, string, string) {
	if missing {
		freshness := freshnessIncomplete
		failure := failureMissingEvidence
		if stale {
			freshness = freshnessStale
			failure = failureStaleEvidence
		}
		if missingPolicy == "not_applicable" {
			return "not_applicable", freshnessNotApplicable, failure
		}
		return missingPolicy, freshness, failure
	}
	if graderFailed {
		return statusNotSatisfied, freshnessFresh, failureGraderFailed
	}
	return statusSatisfied, freshnessFresh, ""
}

func matchingNodes(nodes []*EvidenceNode, matches func(*EvidenceNode) bool) []*EvidenceNode {
	result := make([]*EvidenceNode, 0)
	for _, node := range nodes {
		if matches(node) {
			result = append(result, node)
		}
	}
	return result
}

func matchingGraderNodes(nodes []*EvidenceNode, required *compliancev1.VersionedReference) ([]*EvidenceNode, bool) {
	result := make([]*EvidenceNode, 0)
	passed := true
	for _, node := range nodes {
		matches, nodePassed := nodeMatchesGrader(node, required)
		if !matches {
			continue
		}
		result = append(result, node)
		passed = passed && nodePassed
	}
	return result, passed
}

func nodeMatchesEvidenceType(node *EvidenceNode, required string) bool {
	normalized := normalizeEvidenceIdentity(required)
	actual := normalizeEvidenceIdentity(string(node.ArtifactType))
	if actual == normalized {
		return true
	}
	switch normalized {
	case "action_receipt":
		return node.ArtifactType == ArtifactTypeEvalReceipt
	case "deterministic_stage":
		return node.ArtifactType == ArtifactTypeEvalStage || node.ArtifactType == ArtifactTypeProtocolChain
	case "state_observation":
		return node.ArtifactType == ArtifactTypeEvalObservation
	case "metric", "eval_metric":
		return node.ArtifactType == ArtifactTypeEvalMetric || node.ArtifactType == ArtifactTypeDemoMetric
	}
	return strings.Contains(normalizeEvidenceIdentity(node.SchemaRef), normalized)
}

func nodeMatchesVerifier(node *EvidenceNode, required *compliancev1.VersionedReference) bool {
	if required == nil || required.GetVersion() != "1.0.0" {
		return false
	}
	switch required.GetId() {
	case "receipt_integrity", "notary_proof":
		return node.ArtifactType == ArtifactTypeActionReceipt || node.ArtifactType == ArtifactTypeEvalReceipt
	case "receipt_persistence":
		return node.ArtifactType == ArtifactTypeReceiptPersistence
	case "deterministic_stage_chain":
		return node.ArtifactType == ArtifactTypeProtocolChain || node.ArtifactType == ArtifactTypeEvalStage
	case "commitment_chain":
		return node.ArtifactType == ArtifactTypeCommitment || node.ArtifactType == ArtifactTypeLedgerCommit
	case "state_observation":
		return node.ArtifactType == ArtifactTypeStateObservation || node.ArtifactType == ArtifactTypeEvalObservation
	case "eval_metric":
		return node.ArtifactType == ArtifactTypeEvalMetric || node.ArtifactType == ArtifactTypeDemoMetric
	case "identity_attestation":
		return node.ArtifactType == ArtifactTypeCustomerAttestation || node.ArtifactType == ArtifactTypeAssessorAttestation
	case "build_provenance":
		return node.ArtifactType == ArtifactTypeBuildAttestation
	case "runtime_fips":
		return node.ArtifactType == ArtifactTypeConfigAttestation
	case "compliance_bundle":
		return node.ArtifactType == ArtifactTypeDemoManifest || node.ArtifactType == ArtifactTypeEvalManifest
	default:
		return false
	}
}

func nodeMatchesGrader(node *EvidenceNode, required *compliancev1.VersionedReference) (bool, bool) {
	if required == nil {
		return false, false
	}
	key := versionedReferenceKey(required.GetId(), required.GetVersion())
	if node.ProducerIdentity == key && (node.ArtifactType == ArtifactTypeEvalMetric || node.ArtifactType == ArtifactTypeDemoMetric) {
		return metricNodePassed(node, required)
	}
	switch required.GetId() {
	case "receipt_integrity":
		return nodeMatchesVerifier(node, &compliancev1.VersionedReference{Id: "receipt_integrity", Version: required.GetVersion()}), true
	case "receipt_persistence":
		return nodeMatchesVerifier(node, &compliancev1.VersionedReference{Id: "receipt_persistence", Version: required.GetVersion()}), true
	case "commitment_chain":
		return nodeMatchesVerifier(node, &compliancev1.VersionedReference{Id: "commitment_chain", Version: required.GetVersion()}), true
	case "independent_state":
		return nodeMatchesVerifier(node, &compliancev1.VersionedReference{Id: "state_observation", Version: required.GetVersion()}), true
	case "authenticated_operation":
		return nodeMatchesVerifier(node, &compliancev1.VersionedReference{Id: "identity_attestation", Version: required.GetVersion()}), true
	case "fips_mode":
		return nodeMatchesVerifier(node, &compliancev1.VersionedReference{Id: "runtime_fips", Version: required.GetVersion()}), true
	case "protocol_chain":
		return node.ArtifactType == ArtifactTypeProtocolChain, true
	default:
		return false, false
	}
}

func metricNodePassed(node *EvidenceNode, required *compliancev1.VersionedReference) (bool, bool) {
	metric := metricResult{}
	if err := json.Unmarshal(node.CanonicalBytes, &metric); err != nil {
		return true, false
	}
	if metric.GraderRef != nil && (metric.GraderRef.GetId() != required.GetId() || metric.GraderRef.GetVersion() != required.GetVersion()) {
		return false, false
	}
	if metric.MetricID != "" && (metric.MetricID != required.GetId() || metric.MetricVersion != required.GetVersion()) {
		return false, false
	}
	if metric.VerificationStatus != "" && metric.VerificationStatus != string(VerificationStatusVerified) {
		return true, false
	}
	if metric.Eligible != nil && !*metric.Eligible {
		return true, false
	}
	if metric.Passed != nil {
		return true, *metric.Passed
	}
	return true, metric.Value != nil && *metric.Value == 1
}

func addEvidenceReferences(target map[string]struct{}, nodes []*EvidenceNode) {
	for _, node := range nodes {
		target[node.ArtifactID] = struct{}{}
	}
}

func addMetricReferences(target map[string]struct{}, nodes []*EvidenceNode) {
	for _, node := range nodes {
		if node.ArtifactType == ArtifactTypeEvalMetric || node.ArtifactType == ArtifactTypeDemoMetric {
			target[node.ArtifactID] = struct{}{}
		}
	}
}

func sortedKeys(values map[string]struct{}) []string {
	result := make([]string, 0, len(values))
	for value := range values {
		result = append(result, value)
	}
	sort.Strings(result)
	return result
}

func assertionAssessmentID(scopeID string, assertion *compliancev1.ControlAssertionDefinition) string {
	identity := strings.Join([]string{scopeID, assertion.GetAssertionId(), assertion.GetAssertionVersion(), constants.AssertionGraderID, constants.AssertionGraderVersion}, "\x00")
	digest := sha256.Sum256([]byte(identity))
	return "assertion-assessment:sha256:" + hex.EncodeToString(digest[:])
}

func versionedReferenceKey(id, version string) string {
	return id + "@" + version
}

func normalizeEvidenceIdentity(value string) string {
	return strings.ReplaceAll(strings.ToLower(value), "-", "_")
}
