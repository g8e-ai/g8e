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
	failureEvidenceLevel   = "reproduced checks do not meet the minimum evidence level"
)

type AssertionApplicability struct {
	Components    []string
	ActionClasses []string
	Arms          []string
}

type AssertionSubject struct {
	RunID         string
	AttemptID     string
	ScenarioID    string
	TransactionID string
}

type AssertionPopulation struct {
	Subjects []AssertionSubject
}

type AssertionGradingRequest struct {
	ScopeID       string
	WindowStart   time.Time
	WindowEnd     time.Time
	EvaluatedAt   time.Time
	Applicability *AssertionApplicability
	Population    *AssertionPopulation
	Assertions    *compliancev1.ControlAssertionCatalog
	Graph         *EvidenceGraph
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

type assertionRequirementMatch struct {
	node     *EvidenceNode
	passed   bool
	isMetric bool
	strength string
}

type assertionRequirementResult struct {
	complete     bool
	graderFailed bool
	selected     []assertionRequirementMatch
	available    []assertionRequirementMatch
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
	if request.Applicability != nil {
		if err := validateAssertionApplicability(request.Applicability); err != nil {
			return err
		}
	}
	if request.Population != nil {
		if err := validateAssertionPopulation(request.Population); err != nil {
			return err
		}
	}
	if !request.Graph.Valid() {
		return fmt.Errorf("%w: assertion grading requires a valid evidence graph", constants.ErrInvalidEvidenceGraph)
	}
	if err := catalog.ValidateAssertionCatalog(request.Assertions); err != nil {
		return fmt.Errorf("assertion grading catalog: %w", err)
	}
	return nil
}

func validateAssertionApplicability(applicability *AssertionApplicability) error {
	selections := [][]string{applicability.Components, applicability.ActionClasses, applicability.Arms}
	for _, selection := range selections {
		if len(selection) == 0 {
			return fmt.Errorf("%w: assertion applicability selection is incomplete", constants.ErrInvalidEvidenceGraph)
		}
		seen := make(map[string]struct{}, len(selection))
		for _, value := range selection {
			if value == "" {
				return fmt.Errorf("%w: assertion applicability selection contains an empty value", constants.ErrInvalidEvidenceGraph)
			}
			if _, exists := seen[value]; exists {
				return fmt.Errorf("%w: assertion applicability selection contains duplicate value %s", constants.ErrInvalidEvidenceGraph, value)
			}
			seen[value] = struct{}{}
		}
	}
	return nil
}

func validateAssertionPopulation(population *AssertionPopulation) error {
	seen := make(map[string]struct{}, len(population.Subjects))
	for _, subject := range population.Subjects {
		if subject.RunID == "" || subject.AttemptID == "" && subject.ScenarioID == "" && subject.TransactionID == "" {
			return fmt.Errorf("%w: selected assertion subject is incomplete", constants.ErrInvalidEvidenceGraph)
		}
		key := strings.Join([]string{subject.RunID, subject.AttemptID, subject.ScenarioID, subject.TransactionID}, "\x00")
		if _, exists := seen[key]; exists {
			return fmt.Errorf("%w: duplicate selected assertion subject", constants.ErrInvalidEvidenceGraph)
		}
		seen[key] = struct{}{}
	}
	return nil
}

func gradeControlAssertion(request AssertionGradingRequest, assertion *compliancev1.ControlAssertionDefinition) (*compliancev1.ControlAssertionAssessment, error) {
	candidates := assertionEvidence{evidenceRef: make(map[string]struct{}), metricRef: make(map[string]struct{})}
	status := "not_applicable"
	freshness := freshnessNotApplicable
	failure := ""
	evidenceLevel := "L0"
	limitations := []string{}
	if request.Applicability == nil || assertionApplies(assertion, request.Applicability) {
		candidates = collectAssertionEvidence(request, assertion)
		var missing, stale, graderFailed bool
		var achievedLevel string
		if request.Population == nil {
			missing, stale, graderFailed, achievedLevel = evaluateAssertionRequirements(assertion, candidates)
		} else {
			var assessed, unavailable int
			missing, stale, graderFailed, achievedLevel, assessed, unavailable = evaluateAssertionPopulation(assertion, candidates, request.Population)
			limitations = append(limitations, fmt.Sprintf("selected population: %d; assessed: %d; unavailable: %d", len(request.Population.Subjects), assessed, unavailable))
		}
		status, freshness, failure = assertionOutcome(assertion.GetMissingEvidencePolicy(), missing, stale, graderFailed)
		evidenceLevel = achievedLevel
		if status == statusSatisfied && !evidenceLevelMeets(assertion.GetMinimumEvidenceLevel(), evidenceLevel) {
			status = "unverifiable"
			freshness = freshnessIncomplete
			failure = failureEvidenceLevel
		}
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
		Limitations:     limitations,
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

func evaluateAssertionRequirements(assertion *compliancev1.ControlAssertionDefinition, candidates assertionEvidence) (bool, bool, bool, string) {
	fresh := matchAssertionRequirements(assertion, candidates.fresh)
	selected := fresh.available
	stale := false
	if fresh.complete {
		selected = fresh.selected
	} else {
		staleResult := matchAssertionRequirements(assertion, candidates.stale)
		stale = len(staleResult.available) > 0
		selected = append(selected, staleResult.available...)
	}
	addRequirementReferences(candidates, selected)
	return !fresh.complete, stale, fresh.graderFailed, achievedEvidenceLevel(fresh.available)
}

func evaluateAssertionPopulation(assertion *compliancev1.ControlAssertionDefinition, candidates assertionEvidence, population *AssertionPopulation) (bool, bool, bool, string, int, int) {
	if len(population.Subjects) == 0 {
		return true, false, false, "L0", 0, 0
	}
	assessed := 0
	unavailable := 0
	stale := false
	failedLevel := "L0"
	availableLevel := "L0"
	passingLevel := ""
	graderFailed := false
	for _, subject := range population.Subjects {
		subjectCandidates := assertionEvidence{
			fresh:       filterAssertionNodes(candidates.fresh, subject),
			stale:       filterAssertionNodes(candidates.stale, subject),
			evidenceRef: candidates.evidenceRef,
			metricRef:   candidates.metricRef,
		}
		missing, subjectStale, subjectFailed, level := evaluateAssertionRequirements(assertion, subjectCandidates)
		availableLevel = higherEvidenceLevel(availableLevel, level)
		if missing {
			unavailable++
			stale = stale || subjectStale
			continue
		}
		assessed++
		if subjectFailed {
			graderFailed = true
			failedLevel = higherEvidenceLevel(failedLevel, level)
			continue
		}
		if passingLevel == "" || evidenceLevelIndexOrdered(level) < evidenceLevelIndexOrdered(passingLevel) {
			passingLevel = level
		}
	}
	if graderFailed {
		return false, false, true, failedLevel, assessed, unavailable
	}
	if unavailable > 0 {
		return true, stale, false, availableLevel, assessed, unavailable
	}
	return false, false, false, passingLevel, assessed, unavailable
}

func filterAssertionNodes(nodes []*EvidenceNode, subject AssertionSubject) []*EvidenceNode {
	result := make([]*EvidenceNode, 0)
	for _, node := range nodes {
		if node.RunID != subject.RunID || subject.AttemptID != "" && node.AttemptID != subject.AttemptID || subject.ScenarioID != "" && node.ScenarioID != subject.ScenarioID || subject.TransactionID != "" && node.TransactionID != subject.TransactionID {
			continue
		}
		result = append(result, node)
	}
	return result
}

func higherEvidenceLevel(left, right string) string {
	if evidenceLevelIndexOrdered(right) > evidenceLevelIndexOrdered(left) {
		return right
	}
	return left
}

func matchAssertionRequirements(assertion *compliancev1.ControlAssertionDefinition, nodes []*EvidenceNode) assertionRequirementResult {
	requirements := make([][]assertionRequirementMatch, 0, len(assertion.GetRequiredEvidenceTypes())+len(assertion.GetRequiredVerifierRefs())+len(assertion.GetRequiredGraderRefs()))
	available := make([]assertionRequirementMatch, 0)
	for _, requiredType := range assertion.GetRequiredEvidenceTypes() {
		matches := requirementMatches(nodes, func(node *EvidenceNode) (bool, bool) { return nodeMatchesEvidenceType(node, requiredType), true }, "L1")
		requirements = append(requirements, matches)
		available = append(available, matches...)
	}
	for _, required := range assertion.GetRequiredVerifierRefs() {
		matches := requirementMatches(nodes, func(node *EvidenceNode) (bool, bool) { return nodeMatchesVerifier(node, required), true }, "L2")
		requirements = append(requirements, matches)
		available = append(available, matches...)
	}
	for _, required := range assertion.GetRequiredGraderRefs() {
		matches := requirementMatches(nodes, func(node *EvidenceNode) (bool, bool) { return nodeMatchesGrader(node, required) }, "L2")
		for idx := range matches {
			matches[idx].isMetric = matches[idx].node.ArtifactType == ArtifactTypeEvalMetric || matches[idx].node.ArtifactType == ArtifactTypeDemoMetric
		}
		requirements = append(requirements, matches)
		available = append(available, matches...)
	}
	for _, matches := range requirements {
		if len(matches) == 0 {
			return assertionRequirementResult{available: available}
		}
	}
	selected, complete := coherentRequirementSelection(requirements)
	if !complete {
		return assertionRequirementResult{available: available}
	}
	graderFailed := false
	for _, match := range selected {
		if !match.passed {
			graderFailed = true
		}
	}
	return assertionRequirementResult{complete: true, graderFailed: graderFailed, selected: selected, available: available}
}

func requirementMatches(nodes []*EvidenceNode, matcher func(*EvidenceNode) (bool, bool), strength string) []assertionRequirementMatch {
	result := make([]assertionRequirementMatch, 0)
	for _, node := range nodes {
		matches, passed := matcher(node)
		if matches {
			result = append(result, assertionRequirementMatch{node: node, passed: passed, strength: strength})
		}
	}
	return result
}

func coherentRequirementSelection(requirements [][]assertionRequirementMatch) ([]assertionRequirementMatch, bool) {
	for requirementIndex, matches := range requirements {
		for _, match := range matches {
			if match.passed {
				continue
			}
			if selected, complete := selectCoherentRequirements(requirements, requirementIndex, &match); complete {
				return selected, true
			}
		}
	}
	return selectCoherentRequirements(requirements, -1, nil)
}

func selectCoherentRequirements(requirements [][]assertionRequirementMatch, forcedIndex int, forced *assertionRequirementMatch) ([]assertionRequirementMatch, bool) {
	var search func(int, []assertionRequirementMatch) ([]assertionRequirementMatch, bool)
	search = func(index int, selected []assertionRequirementMatch) ([]assertionRequirementMatch, bool) {
		if index == len(requirements) {
			return append([]assertionRequirementMatch(nil), selected...), true
		}
		matches := requirements[index]
		if index == forcedIndex {
			matches = []assertionRequirementMatch{*forced}
		}
		for _, candidate := range matches {
			compatible := true
			for _, existing := range selected {
				if !sameAssertionSubject(existing.node, candidate.node) {
					compatible = false
					break
				}
			}
			if !compatible {
				continue
			}
			if result, found := search(index+1, append(selected, candidate)); found {
				return result, true
			}
		}
		return nil, false
	}
	return search(0, nil)
}

func sameAssertionSubject(left, right *EvidenceNode) bool {
	if left == nil || right == nil || left.ScopeID != right.ScopeID {
		return false
	}
	bindings := [][2]string{{left.RunID, right.RunID}, {left.AttemptID, right.AttemptID}, {left.ScenarioID, right.ScenarioID}, {left.TransactionID, right.TransactionID}}
	for _, binding := range bindings {
		if binding[0] != "" && binding[1] != "" && binding[0] != binding[1] {
			return false
		}
	}
	if isMetricNode(left) && isObservationNode(right) {
		return Contains(left.References, right.ArtifactID)
	}
	if isObservationNode(left) && isMetricNode(right) {
		return Contains(right.References, left.ArtifactID)
	}
	return true
}

func isMetricNode(node *EvidenceNode) bool {
	return node.ArtifactType == ArtifactTypeEvalMetric || node.ArtifactType == ArtifactTypeDemoMetric
}

func isObservationNode(node *EvidenceNode) bool {
	return node.ArtifactType == ArtifactTypeStateObservation || node.ArtifactType == ArtifactTypeEvalObservation
}

func addRequirementReferences(candidates assertionEvidence, matches []assertionRequirementMatch) {
	for _, match := range matches {
		candidates.evidenceRef[match.node.ArtifactID] = struct{}{}
		if match.isMetric {
			candidates.metricRef[match.node.ArtifactID] = struct{}{}
		}
	}
}

func achievedEvidenceLevel(matches []assertionRequirementMatch) string {
	level := "L0"
	for _, match := range matches {
		if evidenceLevelIndexOrdered(match.strength) > evidenceLevelIndexOrdered(level) {
			level = match.strength
		}
		if !isObservationNode(match.node) {
			continue
		}
		for _, deterministic := range matches {
			if deterministic.strength == "L2" && sameAssertionSubject(match.node, deterministic.node) {
				return "L3"
			}
		}
	}
	return level
}

func assertionOutcome(missingPolicy string, missing, stale, graderFailed bool) (string, string, string) {
	if missing {
		freshness := freshnessIncomplete
		failure := failureMissingEvidence
		if stale {
			freshness = freshnessStale
			failure = failureStaleEvidence
		}
		return missingPolicy, freshness, failure
	}
	if graderFailed {
		return statusNotSatisfied, freshnessFresh, failureGraderFailed
	}
	return statusSatisfied, freshnessFresh, ""
}

func assertionApplies(assertion *compliancev1.ControlAssertionDefinition, applicability *AssertionApplicability) bool {
	return stringSetsIntersect(assertion.GetComponentScope(), applicability.Components) &&
		stringSetsIntersect(assertion.GetApplicableActionClasses(), applicability.ActionClasses) &&
		stringSetsIntersect(assertion.GetApplicableArms(), applicability.Arms)
}

func stringSetsIntersect(left, right []string) bool {
	values := make(map[string]struct{}, len(left))
	for _, value := range left {
		values[value] = struct{}{}
	}
	for _, value := range right {
		if _, exists := values[value]; exists {
			return true
		}
	}
	return false
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
	case "final_persistence_attestation":
		return node.ArtifactType == ArtifactTypeReceiptPersistence
	case "deterministic_stage":
		return node.ArtifactType == ArtifactTypeEvalStage || node.ArtifactType == ArtifactTypeProtocolChain
	case "state_observation":
		return node.ArtifactType == ArtifactTypeEvalObservation
	case "metric", "eval_metric":
		return node.ArtifactType == ArtifactTypeEvalMetric || node.ArtifactType == ArtifactTypeDemoMetric
	default:
		return false
	}
}

func nodeMatchesVerifier(node *EvidenceNode, required *compliancev1.VersionedReference) bool {
	if required == nil || required.GetVersion() != "1.0.0" {
		return false
	}
	switch required.GetId() {
	case "receipt_integrity":
		return node.ArtifactType == ArtifactTypeActionReceipt || node.ArtifactType == ArtifactTypeEvalReceipt
	case "receipt_persistence":
		return node.ArtifactType == ArtifactTypeReceiptPersistence
	case "deterministic_stage_chain":
		return node.ArtifactType == ArtifactTypeProtocolChain || node.ArtifactType == ArtifactTypeEvalStage
	case "commitment_attestation", "commitment_chain":
		return node.ArtifactType == ArtifactTypeCommitment
	case "state_observation":
		return node.ArtifactType == ArtifactTypeStateObservation || node.ArtifactType == ArtifactTypeEvalObservation
	case "eval_metric":
		return node.ArtifactType == ArtifactTypeEvalMetric || node.ArtifactType == ArtifactTypeDemoMetric
	case "identity_attestation":
		return node.ArtifactType == ArtifactTypeCustomerAttestation || node.ArtifactType == ArtifactTypeAssessorAttestation
	case "build_provenance":
		return node.ArtifactType == ArtifactTypeBuildAttestation
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
	if node.ArtifactType == ArtifactTypeEvalMetric || node.ArtifactType == ArtifactTypeDemoMetric {
		return metricNodePassed(node, required)
	}
	switch required.GetId() {
	case "receipt_integrity":
		return nodeMatchesVerifier(node, &compliancev1.VersionedReference{Id: "receipt_integrity", Version: required.GetVersion()}), true
	case "receipt_persistence":
		return nodeMatchesVerifier(node, &compliancev1.VersionedReference{Id: "receipt_persistence", Version: required.GetVersion()}), true
	case "commitment_attestation", "commitment_chain":
		return nodeMatchesVerifier(node, &compliancev1.VersionedReference{Id: required.GetId(), Version: required.GetVersion()}), true
	case "protocol_chain":
		return node.ArtifactType == ArtifactTypeProtocolChain, true
	default:
		return false, false
	}
}

func metricNodePassed(node *EvidenceNode, required *compliancev1.VersionedReference) (bool, bool) {
	metric := metricResult{}
	if err := json.Unmarshal(node.CanonicalBytes, &metric); err != nil {
		return false, false
	}
	identityMatched := false
	if metric.GraderRef != nil {
		if metric.GraderRef.GetId() != required.GetId() || metric.GraderRef.GetVersion() != required.GetVersion() {
			return false, false
		}
		identityMatched = true
	}
	if metric.MetricID != "" {
		if metric.MetricID != required.GetId() || metric.MetricVersion != required.GetVersion() {
			return false, false
		}
		identityMatched = true
	}
	if !identityMatched {
		return false, false
	}
	if metric.VerificationStatus != "" && metric.VerificationStatus != string(VerificationStatusVerified) {
		return false, false
	}
	if metric.Eligible != nil && !*metric.Eligible {
		return false, false
	}
	if metric.Passed != nil {
		return true, *metric.Passed
	}
	return true, metric.Value != nil && *metric.Value == 1
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
