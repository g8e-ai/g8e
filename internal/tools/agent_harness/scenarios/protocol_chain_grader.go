// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package scenarios

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	compliancev1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/compliance/v1"
	operatorv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/operator/v1"
)

// canonicalStageOrder is the deterministic kind ordering for a fully verified
// chain. NormalizeDeterministicStages validates that stages appear in this
// order and GradeProtocolChain uses it for parent-link validation.
var canonicalStageOrder = []operatorv1.DeterministicStageKind{
	operatorv1.DeterministicStageKind_DETERMINISTIC_STAGE_KIND_L1_DOCTRINE,
	operatorv1.DeterministicStageKind_DETERMINISTIC_STAGE_KIND_PROTOCOL_L2,
	operatorv1.DeterministicStageKind_DETERMINISTIC_STAGE_KIND_L3_NOTARY,
	operatorv1.DeterministicStageKind_DETERMINISTIC_STAGE_KIND_L4_VERIFICATION,
	operatorv1.DeterministicStageKind_DETERMINISTIC_STAGE_KIND_RECEIPT_PERSISTENCE,
	operatorv1.DeterministicStageKind_DETERMINISTIC_STAGE_KIND_COMMITMENT_APPEND,
	operatorv1.DeterministicStageKind_DETERMINISTIC_STAGE_KIND_L5_EXECUTION,
}

// stageKindIndex returns the canonical position of a stage kind, or -1 if
// unknown.
func stageKindIndex(kind operatorv1.DeterministicStageKind) int {
	for i, k := range canonicalStageOrder {
		if k == kind {
			return i
		}
	}
	return -1
}

// stageLabelToKind maps the short labels used in canonical demo scenario
// definitions (L1, L2, L3, L5) to their protocol enum values. L4 is always
// present and never appears in a definition's required_deterministic_stages.
var stageLabelToKind = map[string]operatorv1.DeterministicStageKind{
	"L1": operatorv1.DeterministicStageKind_DETERMINISTIC_STAGE_KIND_L1_DOCTRINE,
	"L2": operatorv1.DeterministicStageKind_DETERMINISTIC_STAGE_KIND_PROTOCOL_L2,
	"L3": operatorv1.DeterministicStageKind_DETERMINISTIC_STAGE_KIND_L3_NOTARY,
	"L5": operatorv1.DeterministicStageKind_DETERMINISTIC_STAGE_KIND_L5_EXECUTION,
}

// l2StatusOutcome maps an L2Status to the expected deterministic stage
// outcome for the PROTOCOL_L2 stage.
var l2StatusOutcome = map[operatorv1.L2Status]operatorv1.DeterministicStageOutcome{
	operatorv1.L2Status_L2_STATUS_NOT_REQUIRED:    operatorv1.DeterministicStageOutcome_DETERMINISTIC_STAGE_OUTCOME_NOT_REQUIRED,
	operatorv1.L2Status_L2_STATUS_REQUIRED_VALID:  operatorv1.DeterministicStageOutcome_DETERMINISTIC_STAGE_OUTCOME_VERIFIED,
	operatorv1.L2Status_L2_STATUS_REQUIRED_FAILED: operatorv1.DeterministicStageOutcome_DETERMINISTIC_STAGE_OUTCOME_FAILED,
}

// l3StatusOutcome maps an L3Status to the expected deterministic stage
// outcome for the L3_NOTARY stage.
var l3StatusOutcome = map[operatorv1.L3Status]operatorv1.DeterministicStageOutcome{
	operatorv1.L3Status_L3_STATUS_NOT_REQUIRED:    operatorv1.DeterministicStageOutcome_DETERMINISTIC_STAGE_OUTCOME_NOT_REQUIRED,
	operatorv1.L3Status_L3_STATUS_REQUIRED_VALID:  operatorv1.DeterministicStageOutcome_DETERMINISTIC_STAGE_OUTCOME_VERIFIED,
	operatorv1.L3Status_L3_STATUS_REQUIRED_FAILED: operatorv1.DeterministicStageOutcome_DETERMINISTIC_STAGE_OUTCOME_FAILED,
}

// ProtocolChainGrade is the typed result of grading a deterministic protocol
// chain extracted from a resolved ActionReceipt. Verified is true only when
// the full chain is valid and all required stages are present with correct
// outcomes. StageEvidenceRef is a SHA-256 content address over the canonical
// serialization of the normalized stage slice, suitable for use as a
// DemoScenarioResult evidence reference.
type ProtocolChainGrade struct {
	Verified         bool     `json:"verified"`
	Value            float64  `json:"value"`
	Failure          string   `json:"failure,omitempty"`
	StageEvidenceRef string   `json:"stage_evidence_ref,omitempty"`
	EvidenceRefs     []string `json:"evidence_refs,omitempty"`
}

// NormalizeDeterministicStages extracts the deterministic stage evidence from
// a receipt, validates transaction binding, unique stage IDs, unique kinds,
// and canonical kind ordering, and returns the stages in canonical order. It
// fails closed with ErrInvalidEvidenceGraph on any structural violation.
func NormalizeDeterministicStages(receipt *operatorv1.ActionReceipt) ([]*operatorv1.DeterministicStageEvidence, error) {
	if receipt == nil {
		return nil, fmt.Errorf("%w: receipt is nil", constants.ErrInvalidEvidenceGraph)
	}
	stages := receipt.GetDeterministicStageEvidence()
	if len(stages) == 0 {
		return nil, fmt.Errorf("%w: deterministic stage evidence is missing", constants.ErrInvalidEvidenceGraph)
	}
	txID := receipt.GetTransactionId()
	txHash := receipt.GetTransactionHash()
	seenIDs := make(map[string]struct{}, len(stages))
	seenKinds := make(map[operatorv1.DeterministicStageKind]struct{}, len(stages))
	for _, stage := range stages {
		if stage.GetStageId() == "" {
			return nil, fmt.Errorf("%w: deterministic stage has an empty stage_id", constants.ErrInvalidEvidenceGraph)
		}
		if _, dup := seenIDs[stage.GetStageId()]; dup {
			return nil, fmt.Errorf("%w: duplicate deterministic stage_id %s", constants.ErrInvalidEvidenceGraph, stage.GetStageId())
		}
		seenIDs[stage.GetStageId()] = struct{}{}
		if stage.GetTransactionId() != txID || stage.GetTransactionHash() != txHash {
			return nil, fmt.Errorf("%w: deterministic stage %s transaction does not match receipt", constants.ErrInvalidEvidenceGraph, stage.GetStageId())
		}
		kind := stage.GetKind()
		if stageKindIndex(kind) < 0 {
			return nil, fmt.Errorf("%w: deterministic stage %s has unknown kind %s", constants.ErrInvalidEvidenceGraph, stage.GetStageId(), kind.String())
		}
		if _, dup := seenKinds[kind]; dup {
			return nil, fmt.Errorf("%w: duplicate deterministic stage kind %s", constants.ErrInvalidEvidenceGraph, kind.String())
		}
		seenKinds[kind] = struct{}{}
	}
	// Validate the input order is monotonically increasing in canonical kind
	// index. The gateway emits stages in canonical order; an out-of-order chain
	// indicates tampering or a malformed receipt. This mirrors the Python eval
	// ProtocolChainGrader which checks kinds != sorted(kinds) and fails.
	for i := 1; i < len(stages); i++ {
		if stageKindIndex(stages[i].GetKind()) <= stageKindIndex(stages[i-1].GetKind()) {
			return nil, fmt.Errorf("%w: deterministic stage order is invalid", constants.ErrInvalidEvidenceGraph)
		}
	}
	return stages, nil
}

// ValidateProtocolChain validates the deterministic protocol chain structure
// extracted from a resolved ActionReceipt without checking scenario-specific
// required stages. It normalizes the stages, validates the verified or
// rejected chain logic (parent links, outcomes, receipt status consistency,
// identity consistency), and returns a typed grade with a content-addressed
// stage evidence reference. ResolveReceiptEvidence calls this to grade the
// chain structure as part of receipt evidence resolution; the parent demo
// runner calls GradeProtocolChain with the scenario definition's required
// stages to additionally verify that all declared stages are present.
func ValidateProtocolChain(receipt *operatorv1.ActionReceipt) (*ProtocolChainGrade, error) {
	grade, stages, stagesByKind, l4, err := validateChainStructure(receipt)
	if err != nil {
		return grade, err
	}
	_ = stages
	_ = stagesByKind
	_ = l4
	ref, refErr := stageContentAddress(stages)
	if refErr != nil {
		return failedGrade(refErr.Error()), refErr
	}
	return &ProtocolChainGrade{
		Verified:         true,
		Value:            1.0,
		StageEvidenceRef: ref,
		EvidenceRefs:     []string{receipt.GetTransactionId()},
	}, nil
}

// GradeProtocolChain validates the full deterministic protocol chain extracted
// from a resolved ActionReceipt against the required stage labels declared by
// the canonical demo scenario definition. It validates the chain structure
// (via ValidateProtocolChain) and then checks that every required stage is
// present with the correct outcome.
//
// For a verified chain (L4 outcome VERIFIED): all canonical stages must be
// present, L1 must be VERIFIED, receipt-persistence and commitment-append
// must be COMPLETED, L5 outcome must match the receipt status, L5 state roots
// must match the receipt, and parent links must follow the L4→{L1,L2,L3} and
// L5→{L4,persistence,commitment} topology.
//
// For a rejected chain (L4 outcome FAILED): the chain is a prefix of
// (L1[,L2[,L3]],L4) where the last prerequisite may be FAILED, the receipt
// status must be FAILED, and all parent links point to L4.
func GradeProtocolChain(receipt *operatorv1.ActionReceipt, requiredStages []string) (*ProtocolChainGrade, error) {
	if receipt == nil {
		return failedGrade("receipt is nil"), fmt.Errorf("%w: receipt is nil", constants.ErrInvalidEvidenceGraph)
	}
	if len(requiredStages) == 0 {
		return failedGrade("required stages are empty"), fmt.Errorf("%w: required stages are empty", constants.ErrInvalidEvidenceGraph)
	}
	requiredKinds := make(map[operatorv1.DeterministicStageKind]struct{}, len(requiredStages))
	for _, label := range requiredStages {
		kind, ok := stageLabelToKind[label]
		if !ok {
			return failedGrade("unknown required stage label: " + label), fmt.Errorf("%w: unknown required stage label %s", constants.ErrInvalidEvidenceGraph, label)
		}
		requiredKinds[kind] = struct{}{}
	}
	grade, stages, stagesByKind, l4, err := validateChainStructure(receipt)
	if err != nil {
		return grade, err
	}
	for kind := range requiredKinds {
		stage, ok := stagesByKind[kind]
		if !ok {
			return failedGrade("required stage " + kindLabel(kind) + " is missing"), fmt.Errorf("%w: required stage %s is missing", constants.ErrInvalidEvidenceGraph, kindLabel(kind))
		}
		if !stageOutcomeValid(kind, stage.GetOutcome(), l4.GetOutcome()) {
			return failedGrade("required stage " + kindLabel(kind) + " has an invalid outcome"), fmt.Errorf("%w: required stage %s has an invalid outcome", constants.ErrInvalidEvidenceGraph, kindLabel(kind))
		}
	}
	ref, refErr := stageContentAddress(stages)
	if refErr != nil {
		return failedGrade(refErr.Error()), refErr
	}
	return &ProtocolChainGrade{
		Verified:         true,
		Value:            1.0,
		StageEvidenceRef: ref,
		EvidenceRefs:     []string{receipt.GetTransactionId()},
	}, nil
}

// validateChainStructure performs the chain-structure validation shared by
// ValidateProtocolChain and GradeProtocolChain. It returns a failed grade and
// error on any structural violation, or the normalized stages, stages-by-kind
// map, L4 stage, and a zero-grade on success.
func validateChainStructure(receipt *operatorv1.ActionReceipt) (*ProtocolChainGrade, []*operatorv1.DeterministicStageEvidence, map[operatorv1.DeterministicStageKind]*operatorv1.DeterministicStageEvidence, *operatorv1.DeterministicStageEvidence, error) {
	if receipt == nil {
		return failedGrade("receipt is nil"), nil, nil, nil, fmt.Errorf("%w: receipt is nil", constants.ErrInvalidEvidenceGraph)
	}
	stages, err := NormalizeDeterministicStages(receipt)
	if err != nil {
		return failedGrade(err.Error()), nil, nil, nil, err
	}
	stagesByKind := make(map[operatorv1.DeterministicStageKind]*operatorv1.DeterministicStageEvidence, len(stages))
	for _, s := range stages {
		stagesByKind[s.GetKind()] = s
	}
	l4 := stagesByKind[operatorv1.DeterministicStageKind_DETERMINISTIC_STAGE_KIND_L4_VERIFICATION]
	if l4 == nil {
		return failedGrade("exactly one L4 verification stage is required"), nil, nil, nil, fmt.Errorf("%w: L4 verification stage is missing", constants.ErrInvalidEvidenceGraph)
	}
	if failure := validateIdentityConsistency(stages); failure != "" {
		return failedGrade(failure), nil, nil, nil, fmt.Errorf("%w: %s", constants.ErrInvalidEvidenceGraph, failure)
	}
	if failure := validatePostureStageOutcomes(stagesByKind, receipt); failure != "" {
		return failedGrade(failure), nil, nil, nil, fmt.Errorf("%w: %s", constants.ErrInvalidEvidenceGraph, failure)
	}
	var failure string
	switch l4.GetOutcome() {
	case operatorv1.DeterministicStageOutcome_DETERMINISTIC_STAGE_OUTCOME_VERIFIED:
		failure = validateVerifiedChain(receipt, stagesByKind)
	case operatorv1.DeterministicStageOutcome_DETERMINISTIC_STAGE_OUTCOME_FAILED:
		failure = validateRejectedChain(receipt, stages)
	default:
		failure = "L4 verification stage has an invalid outcome"
	}
	if failure != "" {
		return failedGrade(failure), nil, nil, nil, fmt.Errorf("%w: %s", constants.ErrInvalidEvidenceGraph, failure)
	}
	return nil, stages, stagesByKind, l4, nil
}

// validateVerifiedChain validates the full verified chain topology. This
// mirrors the Python eval ProtocolChainGrader._validate_verified_chain logic.
func validateVerifiedChain(receipt *operatorv1.ActionReceipt, stagesByKind map[operatorv1.DeterministicStageKind]*operatorv1.DeterministicStageEvidence) string {
	for _, kind := range canonicalStageOrder {
		if _, ok := stagesByKind[kind]; !ok {
			return "verified protocol chain is missing required stages"
		}
	}
	if stagesByKind[operatorv1.DeterministicStageKind_DETERMINISTIC_STAGE_KIND_L1_DOCTRINE].GetOutcome() != operatorv1.DeterministicStageOutcome_DETERMINISTIC_STAGE_OUTCOME_VERIFIED {
		return "L1 doctrine stage is not verified"
	}
	if stagesByKind[operatorv1.DeterministicStageKind_DETERMINISTIC_STAGE_KIND_RECEIPT_PERSISTENCE].GetOutcome() != operatorv1.DeterministicStageOutcome_DETERMINISTIC_STAGE_OUTCOME_COMPLETED {
		return "receipt-persistence stage is not completed"
	}
	if stagesByKind[operatorv1.DeterministicStageKind_DETERMINISTIC_STAGE_KIND_COMMITMENT_APPEND].GetOutcome() != operatorv1.DeterministicStageOutcome_DETERMINISTIC_STAGE_OUTCOME_COMPLETED {
		return "commitment-append stage is not completed"
	}
	l5 := stagesByKind[operatorv1.DeterministicStageKind_DETERMINISTIC_STAGE_KIND_L5_EXECUTION]
	expectedL5Outcome := operatorv1.DeterministicStageOutcome_DETERMINISTIC_STAGE_OUTCOME_UNSPECIFIED
	switch receipt.GetStatus() {
	case operatorv1.ExecutionStatus_EXECUTION_STATUS_COMPLETED:
		expectedL5Outcome = operatorv1.DeterministicStageOutcome_DETERMINISTIC_STAGE_OUTCOME_COMPLETED
	case operatorv1.ExecutionStatus_EXECUTION_STATUS_FAILED:
		expectedL5Outcome = operatorv1.DeterministicStageOutcome_DETERMINISTIC_STAGE_OUTCOME_FAILED
	default:
		return "L5 execution outcome does not match the signed receipt status"
	}
	if l5.GetOutcome() != expectedL5Outcome {
		return "L5 execution outcome does not match the signed receipt status"
	}
	if l5.GetStateRootBefore() != receipt.GetStateRootBefore() || l5.GetStateRootAfter() != receipt.GetStateRootAfter() {
		return "L5 execution state roots do not match the signed receipt"
	}
	l4ID := stagesByKind[operatorv1.DeterministicStageKind_DETERMINISTIC_STAGE_KIND_L4_VERIFICATION].GetStageId()
	l5ID := l5.GetStageId()
	expectedParents := map[operatorv1.DeterministicStageKind]string{
		operatorv1.DeterministicStageKind_DETERMINISTIC_STAGE_KIND_L1_DOCTRINE:          l4ID,
		operatorv1.DeterministicStageKind_DETERMINISTIC_STAGE_KIND_PROTOCOL_L2:         l4ID,
		operatorv1.DeterministicStageKind_DETERMINISTIC_STAGE_KIND_L3_NOTARY:           l4ID,
		operatorv1.DeterministicStageKind_DETERMINISTIC_STAGE_KIND_L4_VERIFICATION:     l5ID,
		operatorv1.DeterministicStageKind_DETERMINISTIC_STAGE_KIND_RECEIPT_PERSISTENCE: l5ID,
		operatorv1.DeterministicStageKind_DETERMINISTIC_STAGE_KIND_COMMITMENT_APPEND:   l5ID,
		operatorv1.DeterministicStageKind_DETERMINISTIC_STAGE_KIND_L5_EXECUTION:        "",
	}
	for kind, parentID := range expectedParents {
		if stagesByKind[kind].GetParentStageId() != parentID {
			return "deterministic stage parent relationship is invalid"
		}
	}
	return ""
}

// validateRejectedChain validates the rejected chain topology. This mirrors
// the Python eval ProtocolChainGrader._validate_rejected_chain logic.
func validateRejectedChain(receipt *operatorv1.ActionReceipt, stages []*operatorv1.DeterministicStageEvidence) string {
	if receipt.GetStatus() != operatorv1.ExecutionStatus_EXECUTION_STATUS_FAILED {
		return "rejected protocol chain does not have a failed receipt status"
	}
	allowedPrefixes := [][]operatorv1.DeterministicStageKind{
		{},
		{operatorv1.DeterministicStageKind_DETERMINISTIC_STAGE_KIND_L1_DOCTRINE},
		{operatorv1.DeterministicStageKind_DETERMINISTIC_STAGE_KIND_L1_DOCTRINE, operatorv1.DeterministicStageKind_DETERMINISTIC_STAGE_KIND_PROTOCOL_L2},
		{operatorv1.DeterministicStageKind_DETERMINISTIC_STAGE_KIND_L1_DOCTRINE, operatorv1.DeterministicStageKind_DETERMINISTIC_STAGE_KIND_PROTOCOL_L2, operatorv1.DeterministicStageKind_DETERMINISTIC_STAGE_KIND_L3_NOTARY},
	}
	prefix := make([]operatorv1.DeterministicStageKind, 0, len(stages)-1)
	for _, s := range stages[:len(stages)-1] {
		prefix = append(prefix, s.GetKind())
	}
	if stages[len(stages)-1].GetKind() != operatorv1.DeterministicStageKind_DETERMINISTIC_STAGE_KIND_L4_VERIFICATION {
		return "rejected protocol chain contains invalid stages"
	}
	if !prefixAllowed(prefix, allowedPrefixes) {
		return "rejected protocol chain contains invalid stages"
	}
	failedPrerequisites := make([]*operatorv1.DeterministicStageEvidence, 0)
	for _, s := range stages[:len(stages)-1] {
		if s.GetOutcome() == operatorv1.DeterministicStageOutcome_DETERMINISTIC_STAGE_OUTCOME_FAILED {
			failedPrerequisites = append(failedPrerequisites, s)
		}
	}
	if len(failedPrerequisites) > 1 {
		return "rejected protocol chain has ambiguous prerequisite outcomes"
	}
	if len(failedPrerequisites) == 1 && failedPrerequisites[0] != stages[len(stages)-2] {
		return "rejected protocol chain has ambiguous prerequisite outcomes"
	}
	expectedOutcomes := map[operatorv1.DeterministicStageKind]operatorv1.DeterministicStageOutcome{
		operatorv1.DeterministicStageKind_DETERMINISTIC_STAGE_KIND_L1_DOCTRINE:  operatorv1.DeterministicStageOutcome_DETERMINISTIC_STAGE_OUTCOME_VERIFIED,
		operatorv1.DeterministicStageKind_DETERMINISTIC_STAGE_KIND_PROTOCOL_L2: l2StatusOutcome[receipt.GetL2Status()],
		operatorv1.DeterministicStageKind_DETERMINISTIC_STAGE_KIND_L3_NOTARY:   l3StatusOutcome[receipt.GetL3Status()],
	}
	var completedPrerequisites []*operatorv1.DeterministicStageEvidence
	if len(failedPrerequisites) > 0 {
		completedPrerequisites = stages[:len(stages)-2]
	} else {
		completedPrerequisites = stages[:len(stages)-1]
	}
	for _, s := range completedPrerequisites {
		if s.GetOutcome() != expectedOutcomes[s.GetKind()] {
			return "rejected protocol chain has invalid prerequisite outcomes"
		}
	}
	l4 := stages[len(stages)-1]
	for _, s := range stages[:len(stages)-1] {
		if s.GetParentStageId() != l4.GetStageId() {
			return "deterministic stage parent relationship is invalid"
		}
	}
	if l4.GetParentStageId() != "" {
		return "deterministic stage parent relationship is invalid"
	}
	return ""
}

// validateIdentityConsistency checks that identity fields (investigation_id,
// operator_id, etc.) are consistent across all stages.
func validateIdentityConsistency(stages []*operatorv1.DeterministicStageEvidence) string {
	identityFields := []string{"OperatorId", "OperatorSessionId", "RequestorUserId", "ActingAppId", "CaseId", "InvestigationId", "TaskId"}
	for _, field := range identityFields {
		values := make(map[string]struct{})
		for _, s := range stages {
			val := getStageIdentityField(s, field)
			if val != "" {
				values[val] = struct{}{}
			}
		}
		if len(values) > 1 {
			return "deterministic stage identity fields are inconsistent"
		}
	}
	return ""
}

// getStageIdentityField reads a string identity field from a stage by name.
func getStageIdentityField(s *operatorv1.DeterministicStageEvidence, field string) string {
	switch field {
	case "OperatorId":
		return s.GetOperatorId()
	case "OperatorSessionId":
		return s.GetOperatorSessionId()
	case "RequestorUserId":
		return s.GetRequestorUserId()
	case "ActingAppId":
		return s.GetActingAppId()
	case "CaseId":
		return s.GetCaseId()
	case "InvestigationId":
		return s.GetInvestigationId()
	case "TaskId":
		return s.GetTaskId()
	}
	return ""
}

// validatePostureStageOutcomes validates that L2 and L3 stage outcomes (if
// present) match the receipt's L2/L3 status fields.
func validatePostureStageOutcomes(stagesByKind map[operatorv1.DeterministicStageKind]*operatorv1.DeterministicStageEvidence, receipt *operatorv1.ActionReceipt) string {
	if stage, ok := stagesByKind[operatorv1.DeterministicStageKind_DETERMINISTIC_STAGE_KIND_PROTOCOL_L2]; ok {
		if stage.GetOutcome() != l2StatusOutcome[receipt.GetL2Status()] {
			return "L2 stage outcome does not match the signed receipt status"
		}
	}
	if stage, ok := stagesByKind[operatorv1.DeterministicStageKind_DETERMINISTIC_STAGE_KIND_L3_NOTARY]; ok {
		if stage.GetOutcome() != l3StatusOutcome[receipt.GetL3Status()] {
			return "L3 stage outcome does not match the signed receipt status"
		}
	}
	return ""
}

// stageOutcomeValid returns true if the stage outcome is valid for the given
// kind in the context of the L4 outcome. Required stages in a verified chain
// must be VERIFIED (L1, L2, L3) or COMPLETED (L5). In a rejected chain, the
// failed prerequisite is allowed to be FAILED.
func stageOutcomeValid(kind operatorv1.DeterministicStageKind, outcome, l4Outcome operatorv1.DeterministicStageOutcome) bool {
	if l4Outcome == operatorv1.DeterministicStageOutcome_DETERMINISTIC_STAGE_OUTCOME_FAILED {
		return outcome == operatorv1.DeterministicStageOutcome_DETERMINISTIC_STAGE_OUTCOME_VERIFIED ||
			outcome == operatorv1.DeterministicStageOutcome_DETERMINISTIC_STAGE_OUTCOME_FAILED
	}
	switch kind {
	case operatorv1.DeterministicStageKind_DETERMINISTIC_STAGE_KIND_L5_EXECUTION:
		return outcome == operatorv1.DeterministicStageOutcome_DETERMINISTIC_STAGE_OUTCOME_COMPLETED ||
			outcome == operatorv1.DeterministicStageOutcome_DETERMINISTIC_STAGE_OUTCOME_FAILED
	default:
		return outcome == operatorv1.DeterministicStageOutcome_DETERMINISTIC_STAGE_OUTCOME_VERIFIED
	}
}

// kindLabel returns the short label (L1, L2, L3, L5) for a stage kind, or the
// enum name if no short label exists.
func kindLabel(kind operatorv1.DeterministicStageKind) string {
	for label, k := range stageLabelToKind {
		if k == kind {
			return label
		}
	}
	return kind.String()
}

// prefixAllowed returns true if the prefix matches one of the allowed
// prefixes.
func prefixAllowed(prefix []operatorv1.DeterministicStageKind, allowed [][]operatorv1.DeterministicStageKind) bool {
	for _, a := range allowed {
		if len(prefix) != len(a) {
			continue
		}
		match := true
		for i := range prefix {
			if prefix[i] != a[i] {
				match = false
				break
			}
		}
		if match {
			return true
		}
	}
	return false
}

// stageContentAddress computes a SHA-256 content address over the canonical
// protojson serialization of each normalized stage, concatenated in canonical
// order. This produces a stable, tamper-evident digest over the full stage
// slice without requiring a wrapper proto message.
func stageContentAddress(stages []*operatorv1.DeterministicStageEvidence) (string, error) {
	hasher := sha256.New()
	for _, stage := range stages {
		encoded, err := compliancev1.MarshalCanonical(stage)
		if err != nil {
			return "", fmt.Errorf("%w: canonicalize deterministic stage evidence: %w", constants.ErrInvalidEvidenceGraph, err)
		}
		hasher.Write(encoded)
	}
	digest := hasher.Sum(nil)
	return "deterministic-stages:sha256:" + hex.EncodeToString(digest), nil
}

// failedGrade returns a non-verified ProtocolChainGrade with the given
// failure reason.
func failedGrade(failure string) *ProtocolChainGrade {
	return &ProtocolChainGrade{
		Verified: false,
		Value:    0.0,
		Failure:  failure,
	}
}
