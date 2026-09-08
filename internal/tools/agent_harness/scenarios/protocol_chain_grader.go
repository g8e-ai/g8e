// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package scenarios

import (
	"fmt"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/services/governance"
	operatorv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/operator/v1"
)

var stageLabelToKind = map[string]operatorv1.DeterministicStageKind{
	"L1": operatorv1.DeterministicStageKind_DETERMINISTIC_STAGE_KIND_L1_DOCTRINE,
	"L2": operatorv1.DeterministicStageKind_DETERMINISTIC_STAGE_KIND_PROTOCOL_L2,
	"L3": operatorv1.DeterministicStageKind_DETERMINISTIC_STAGE_KIND_L3_NOTARY,
	"L5": operatorv1.DeterministicStageKind_DETERMINISTIC_STAGE_KIND_L5_EXECUTION,
}

type ProtocolChainGrade struct {
	Verified         bool     `json:"verified"`
	Value            float64  `json:"value"`
	Failure          string   `json:"failure,omitempty"`
	StageEvidenceRef string   `json:"stage_evidence_ref,omitempty"`
	EvidenceRefs     []string `json:"evidence_refs,omitempty"`
}

func GradeProtocolChain(receipt *operatorv1.ActionReceipt, requiredStages []string) (*ProtocolChainGrade, error) {
	if receipt == nil {
		return failedProtocolChainGrade("receipt is nil"), fmt.Errorf("%w: receipt is nil", constants.ErrInvalidEvidenceGraph)
	}
	if len(requiredStages) == 0 {
		return failedProtocolChainGrade("required stages are empty"), fmt.Errorf("%w: required stages are empty", constants.ErrInvalidEvidenceGraph)
	}
	requiredKinds := make(map[operatorv1.DeterministicStageKind]struct{}, len(requiredStages))
	for _, label := range requiredStages {
		kind, exists := stageLabelToKind[label]
		if !exists {
			return failedProtocolChainGrade("unknown required stage label: " + label), fmt.Errorf("%w: unknown required stage label %s", constants.ErrInvalidEvidenceGraph, label)
		}
		requiredKinds[kind] = struct{}{}
	}
	chain, err := governance.ValidateDeterministicProtocolChain(receipt)
	if err != nil {
		return failedProtocolChainGrade(err.Error()), err
	}
	for kind := range requiredKinds {
		stage := chain.StagesByKind[kind]
		if stage == nil {
			return failedProtocolChainGrade("required stage " + protocolChainKindLabel(kind) + " is missing"), fmt.Errorf("%w: required stage %s is missing", constants.ErrInvalidEvidenceGraph, protocolChainKindLabel(kind))
		}
		if !requiredProtocolChainStageOutcomeValid(kind, stage.GetOutcome(), chain.L4.GetOutcome()) {
			return failedProtocolChainGrade("required stage " + protocolChainKindLabel(kind) + " has an invalid outcome"), fmt.Errorf("%w: required stage %s has an invalid outcome", constants.ErrInvalidEvidenceGraph, protocolChainKindLabel(kind))
		}
	}
	return &ProtocolChainGrade{Verified: true, Value: 1, StageEvidenceRef: chain.ContentReference, EvidenceRefs: []string{receipt.GetTransactionId()}}, nil
}

func requiredProtocolChainStageOutcomeValid(kind operatorv1.DeterministicStageKind, outcome, l4Outcome operatorv1.DeterministicStageOutcome) bool {
	if l4Outcome == operatorv1.DeterministicStageOutcome_DETERMINISTIC_STAGE_OUTCOME_FAILED {
		return outcome == operatorv1.DeterministicStageOutcome_DETERMINISTIC_STAGE_OUTCOME_VERIFIED || outcome == operatorv1.DeterministicStageOutcome_DETERMINISTIC_STAGE_OUTCOME_FAILED
	}
	if kind == operatorv1.DeterministicStageKind_DETERMINISTIC_STAGE_KIND_L5_EXECUTION {
		return outcome == operatorv1.DeterministicStageOutcome_DETERMINISTIC_STAGE_OUTCOME_COMPLETED || outcome == operatorv1.DeterministicStageOutcome_DETERMINISTIC_STAGE_OUTCOME_FAILED
	}
	return outcome == operatorv1.DeterministicStageOutcome_DETERMINISTIC_STAGE_OUTCOME_VERIFIED
}

func protocolChainKindLabel(kind operatorv1.DeterministicStageKind) string {
	for label, candidate := range stageLabelToKind {
		if candidate == kind {
			return label
		}
	}
	return kind.String()
}

func failedProtocolChainGrade(failure string) *ProtocolChainGrade {
	return &ProtocolChainGrade{Failure: failure}
}
