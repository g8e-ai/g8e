// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package governance

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	compliancev1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/compliance/v1"
	operatorv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/operator/v1"
)

var deterministicStageOrder = []operatorv1.DeterministicStageKind{
	operatorv1.DeterministicStageKind_DETERMINISTIC_STAGE_KIND_L1_DOCTRINE,
	operatorv1.DeterministicStageKind_DETERMINISTIC_STAGE_KIND_PROTOCOL_L2,
	operatorv1.DeterministicStageKind_DETERMINISTIC_STAGE_KIND_L3_NOTARY,
	operatorv1.DeterministicStageKind_DETERMINISTIC_STAGE_KIND_L4_VERIFICATION,
	operatorv1.DeterministicStageKind_DETERMINISTIC_STAGE_KIND_RECEIPT_PERSISTENCE,
	operatorv1.DeterministicStageKind_DETERMINISTIC_STAGE_KIND_COMMITMENT_APPEND,
	operatorv1.DeterministicStageKind_DETERMINISTIC_STAGE_KIND_L5_EXECUTION,
}

var deterministicL2StatusOutcome = map[operatorv1.L2Status]operatorv1.DeterministicStageOutcome{
	operatorv1.L2Status_L2_STATUS_NOT_REQUIRED:    operatorv1.DeterministicStageOutcome_DETERMINISTIC_STAGE_OUTCOME_NOT_REQUIRED,
	operatorv1.L2Status_L2_STATUS_REQUIRED_VALID:  operatorv1.DeterministicStageOutcome_DETERMINISTIC_STAGE_OUTCOME_VERIFIED,
	operatorv1.L2Status_L2_STATUS_REQUIRED_FAILED: operatorv1.DeterministicStageOutcome_DETERMINISTIC_STAGE_OUTCOME_FAILED,
}

var deterministicL3StatusOutcome = map[operatorv1.L3Status]operatorv1.DeterministicStageOutcome{
	operatorv1.L3Status_L3_STATUS_NOT_REQUIRED:    operatorv1.DeterministicStageOutcome_DETERMINISTIC_STAGE_OUTCOME_NOT_REQUIRED,
	operatorv1.L3Status_L3_STATUS_REQUIRED_VALID:  operatorv1.DeterministicStageOutcome_DETERMINISTIC_STAGE_OUTCOME_VERIFIED,
	operatorv1.L3Status_L3_STATUS_REQUIRED_FAILED: operatorv1.DeterministicStageOutcome_DETERMINISTIC_STAGE_OUTCOME_FAILED,
}

type DeterministicProtocolChain struct {
	Stages           []*operatorv1.DeterministicStageEvidence
	StagesByKind     map[operatorv1.DeterministicStageKind]*operatorv1.DeterministicStageEvidence
	L4               *operatorv1.DeterministicStageEvidence
	ContentReference string
}

func ValidateDeterministicProtocolChain(receipt *operatorv1.ActionReceipt) (*DeterministicProtocolChain, error) {
	stages, err := NormalizeDeterministicStages(receipt)
	if err != nil {
		return nil, err
	}
	stagesByKind := make(map[operatorv1.DeterministicStageKind]*operatorv1.DeterministicStageEvidence, len(stages))
	for _, stage := range stages {
		stagesByKind[stage.GetKind()] = stage
	}
	l4 := stagesByKind[operatorv1.DeterministicStageKind_DETERMINISTIC_STAGE_KIND_L4_VERIFICATION]
	if l4 == nil {
		return nil, fmt.Errorf("%w: L4 verification stage is missing", constants.ErrInvalidEvidenceGraph)
	}
	if failure := validateDeterministicIdentityConsistency(stages); failure != "" {
		return nil, fmt.Errorf("%w: %s", constants.ErrInvalidEvidenceGraph, failure)
	}
	if failure := validateDeterministicPostureStageOutcomes(stagesByKind, receipt); failure != "" {
		return nil, fmt.Errorf("%w: %s", constants.ErrInvalidEvidenceGraph, failure)
	}
	var failure string
	switch l4.GetOutcome() {
	case operatorv1.DeterministicStageOutcome_DETERMINISTIC_STAGE_OUTCOME_VERIFIED:
		failure = validateVerifiedDeterministicChain(receipt, stagesByKind)
	case operatorv1.DeterministicStageOutcome_DETERMINISTIC_STAGE_OUTCOME_FAILED:
		failure = validateRejectedDeterministicChain(receipt, stages)
	default:
		failure = "L4 verification stage has an invalid outcome"
	}
	if failure != "" {
		return nil, fmt.Errorf("%w: %s", constants.ErrInvalidEvidenceGraph, failure)
	}
	contentReference, err := DeterministicStagesContentAddress(stages)
	if err != nil {
		return nil, err
	}
	return &DeterministicProtocolChain{Stages: stages, StagesByKind: stagesByKind, L4: l4, ContentReference: contentReference}, nil
}

func NormalizeDeterministicStages(receipt *operatorv1.ActionReceipt) ([]*operatorv1.DeterministicStageEvidence, error) {
	if receipt == nil {
		return nil, fmt.Errorf("%w: receipt is nil", constants.ErrInvalidEvidenceGraph)
	}
	stages := receipt.GetDeterministicStageEvidence()
	if len(stages) == 0 {
		return nil, fmt.Errorf("%w: deterministic stage evidence is missing", constants.ErrInvalidEvidenceGraph)
	}
	if _, err := DeterministicStageActionType(receipt); err != nil {
		return nil, fmt.Errorf("%w: %v", constants.ErrInvalidEvidenceGraph, err)
	}
	seenIDs := make(map[string]struct{}, len(stages))
	seenKinds := make(map[operatorv1.DeterministicStageKind]struct{}, len(stages))
	for _, stage := range stages {
		if stage.GetStageId() == "" {
			return nil, fmt.Errorf("%w: deterministic stage has an empty stage_id", constants.ErrInvalidEvidenceGraph)
		}
		if _, exists := seenIDs[stage.GetStageId()]; exists {
			return nil, fmt.Errorf("%w: duplicate deterministic stage_id %s", constants.ErrInvalidEvidenceGraph, stage.GetStageId())
		}
		seenIDs[stage.GetStageId()] = struct{}{}
		if stage.GetTransactionId() != receipt.GetTransactionId() || stage.GetTransactionHash() != receipt.GetTransactionHash() {
			return nil, fmt.Errorf("%w: deterministic stage %s transaction does not match receipt", constants.ErrInvalidEvidenceGraph, stage.GetStageId())
		}
		kind := stage.GetKind()
		if deterministicStageKindIndex(kind) < 0 {
			return nil, fmt.Errorf("%w: deterministic stage %s has unknown kind %s", constants.ErrInvalidEvidenceGraph, stage.GetStageId(), kind.String())
		}
		if _, exists := seenKinds[kind]; exists {
			return nil, fmt.Errorf("%w: duplicate deterministic stage kind %s", constants.ErrInvalidEvidenceGraph, kind.String())
		}
		seenKinds[kind] = struct{}{}
	}
	for index := 1; index < len(stages); index++ {
		if deterministicStageKindIndex(stages[index].GetKind()) <= deterministicStageKindIndex(stages[index-1].GetKind()) {
			return nil, fmt.Errorf("%w: deterministic stage order is invalid", constants.ErrInvalidEvidenceGraph)
		}
	}
	return stages, nil
}

func DeterministicStagesContentAddress(stages []*operatorv1.DeterministicStageEvidence) (string, error) {
	hasher := sha256.New()
	for _, stage := range stages {
		encoded, err := compliancev1.MarshalCanonical(stage)
		if err != nil {
			return "", fmt.Errorf("%w: canonicalize deterministic stage evidence: %w", constants.ErrInvalidEvidenceGraph, err)
		}
		if _, err := hasher.Write(encoded); err != nil {
			return "", fmt.Errorf("%w: hash deterministic stage evidence: %w", constants.ErrInvalidEvidenceGraph, err)
		}
	}
	return "deterministic-stages:sha256:" + hex.EncodeToString(hasher.Sum(nil)), nil
}

func deterministicStageKindIndex(kind operatorv1.DeterministicStageKind) int {
	for index, candidate := range deterministicStageOrder {
		if candidate == kind {
			return index
		}
	}
	return -1
}

func validateVerifiedDeterministicChain(receipt *operatorv1.ActionReceipt, stagesByKind map[operatorv1.DeterministicStageKind]*operatorv1.DeterministicStageEvidence) string {
	for _, kind := range deterministicStageOrder {
		if stagesByKind[kind] == nil {
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
	var expectedOutcome operatorv1.DeterministicStageOutcome
	switch receipt.GetStatus() {
	case operatorv1.ExecutionStatus_EXECUTION_STATUS_COMPLETED:
		expectedOutcome = operatorv1.DeterministicStageOutcome_DETERMINISTIC_STAGE_OUTCOME_COMPLETED
	case operatorv1.ExecutionStatus_EXECUTION_STATUS_FAILED:
		expectedOutcome = operatorv1.DeterministicStageOutcome_DETERMINISTIC_STAGE_OUTCOME_FAILED
	default:
		return "L5 execution outcome does not match the signed receipt status"
	}
	if l5.GetOutcome() != expectedOutcome {
		return "L5 execution outcome does not match the signed receipt status"
	}
	if l5.GetStateRootBefore() != receipt.GetStateRootBefore() || l5.GetStateRootAfter() != receipt.GetStateRootAfter() {
		return "L5 execution state roots do not match the signed receipt"
	}
	l4ID := stagesByKind[operatorv1.DeterministicStageKind_DETERMINISTIC_STAGE_KIND_L4_VERIFICATION].GetStageId()
	l5ID := l5.GetStageId()
	expectedParents := map[operatorv1.DeterministicStageKind]string{
		operatorv1.DeterministicStageKind_DETERMINISTIC_STAGE_KIND_L1_DOCTRINE:         l4ID,
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

func validateRejectedDeterministicChain(receipt *operatorv1.ActionReceipt, stages []*operatorv1.DeterministicStageEvidence) string {
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
	for _, stage := range stages[:len(stages)-1] {
		prefix = append(prefix, stage.GetKind())
	}
	if stages[len(stages)-1].GetKind() != operatorv1.DeterministicStageKind_DETERMINISTIC_STAGE_KIND_L4_VERIFICATION || !deterministicPrefixAllowed(prefix, allowedPrefixes) {
		return "rejected protocol chain contains invalid stages"
	}
	failedPrerequisites := make([]*operatorv1.DeterministicStageEvidence, 0)
	for _, stage := range stages[:len(stages)-1] {
		if stage.GetOutcome() == operatorv1.DeterministicStageOutcome_DETERMINISTIC_STAGE_OUTCOME_FAILED {
			failedPrerequisites = append(failedPrerequisites, stage)
		}
	}
	if len(failedPrerequisites) > 1 || len(failedPrerequisites) == 1 && failedPrerequisites[0] != stages[len(stages)-2] {
		return "rejected protocol chain has ambiguous prerequisite outcomes"
	}
	expectedOutcomes := map[operatorv1.DeterministicStageKind]operatorv1.DeterministicStageOutcome{
		operatorv1.DeterministicStageKind_DETERMINISTIC_STAGE_KIND_L1_DOCTRINE: operatorv1.DeterministicStageOutcome_DETERMINISTIC_STAGE_OUTCOME_VERIFIED,
		operatorv1.DeterministicStageKind_DETERMINISTIC_STAGE_KIND_PROTOCOL_L2: deterministicL2StatusOutcome[receipt.GetL2Status()],
		operatorv1.DeterministicStageKind_DETERMINISTIC_STAGE_KIND_L3_NOTARY:   deterministicL3StatusOutcome[receipt.GetL3Status()],
	}
	completedPrerequisites := stages[:len(stages)-1]
	if len(failedPrerequisites) > 0 {
		completedPrerequisites = stages[:len(stages)-2]
	}
	for _, stage := range completedPrerequisites {
		if stage.GetOutcome() != expectedOutcomes[stage.GetKind()] {
			return "rejected protocol chain has invalid prerequisite outcomes"
		}
	}
	l4 := stages[len(stages)-1]
	for _, stage := range stages[:len(stages)-1] {
		if stage.GetParentStageId() != l4.GetStageId() {
			return "deterministic stage parent relationship is invalid"
		}
	}
	if l4.GetParentStageId() != "" {
		return "deterministic stage parent relationship is invalid"
	}
	return ""
}

func validateDeterministicIdentityConsistency(stages []*operatorv1.DeterministicStageEvidence) string {
	identities := [][]string{
		{}, {}, {}, {}, {}, {}, {},
	}
	for _, stage := range stages {
		values := []string{stage.GetOperatorId(), stage.GetOperatorSessionId(), stage.GetRequestorUserId(), stage.GetActingAppId(), stage.GetCaseId(), stage.GetInvestigationId(), stage.GetTaskId()}
		for index, value := range values {
			if value != "" {
				identities[index] = append(identities[index], value)
			}
		}
	}
	for _, values := range identities {
		first := ""
		for _, value := range values {
			if first == "" {
				first = value
			} else if value != first {
				return "deterministic stage identity fields are inconsistent"
			}
		}
	}
	return ""
}

func validateDeterministicPostureStageOutcomes(stagesByKind map[operatorv1.DeterministicStageKind]*operatorv1.DeterministicStageEvidence, receipt *operatorv1.ActionReceipt) string {
	if stage := stagesByKind[operatorv1.DeterministicStageKind_DETERMINISTIC_STAGE_KIND_PROTOCOL_L2]; stage != nil && stage.GetOutcome() != deterministicL2StatusOutcome[receipt.GetL2Status()] {
		return "L2 stage outcome does not match the signed receipt status"
	}
	if stage := stagesByKind[operatorv1.DeterministicStageKind_DETERMINISTIC_STAGE_KIND_L3_NOTARY]; stage != nil && stage.GetOutcome() != deterministicL3StatusOutcome[receipt.GetL3Status()] {
		return "L3 stage outcome does not match the signed receipt status"
	}
	return ""
}

func deterministicPrefixAllowed(prefix []operatorv1.DeterministicStageKind, allowed [][]operatorv1.DeterministicStageKind) bool {
	for _, candidate := range allowed {
		if len(prefix) != len(candidate) {
			continue
		}
		matches := true
		for index := range prefix {
			if prefix[index] != candidate[index] {
				matches = false
				break
			}
		}
		if matches {
			return true
		}
	}
	return false
}
