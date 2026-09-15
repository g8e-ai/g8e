// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package evaluation

import (
	"fmt"

	"google.golang.org/protobuf/proto"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	compliancev1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/compliance/v1"
	evalv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/eval/v1"
)

const (
	CoreExecutionBoundarySuiteID      = "core-execution-boundary"
	CoreExecutionBoundarySuiteVersion = "1.0.0"
	AllowedExecutionScenarioID        = "allowed-execution-occurs-once"
	ProhibitedExecutionScenarioID     = "prohibited-equivalent-causes-no-additional-effect"
	RegistryVersion                   = "1.0.0"
	GraderID                          = "g8e-native-deterministic-grader"
	GraderVersion                     = "1.0.0"
	TopologyID                        = "unified-compose-remote-operator"
	TopologyVersion                   = "1.0.0"
	MetricRequiredVerdictPassRate     = "required-verdict-pass-rate"
	MetricEligiblePopulation          = "required-verdicts"
)

type ScenarioDefinition struct {
	Reference  *compliancev1.VersionedReference
	Assertions []*evalv1.EvaluationAssertion
}

type SuiteDefinition struct {
	Reference       *compliancev1.VersionedReference
	RequiredPosture evalv1.EvaluationGovernancePosture
	Lane            evalv1.EvaluationLane
	Scenarios       []ScenarioDefinition
}

type Registry struct {
	suites []SuiteDefinition
}

func NewRegistry() *Registry {
	return &Registry{suites: []SuiteDefinition{coreExecutionBoundarySuite()}}
}

func (r *Registry) Lookup(id, version string) (*SuiteDefinition, error) {
	if r == nil {
		return nil, constants.ErrEvaluationSuiteUnsupported
	}
	for index := range r.suites {
		suite := &r.suites[index]
		if suite.Reference.GetId() == id && suite.Reference.GetVersion() == version {
			return cloneSuite(suite), nil
		}
	}
	return nil, fmt.Errorf("%w: %s@%s", constants.ErrEvaluationSuiteUnsupported, id, version)
}

func coreExecutionBoundarySuite() SuiteDefinition {
	allowed := ScenarioDefinition{
		Reference: versioned(AllowedExecutionScenarioID, CoreExecutionBoundarySuiteVersion),
		Assertions: []*evalv1.EvaluationAssertion{
			equalIntegerAssertion("allowed-initial-effect-count", 0, "target-marker-count-before-allowed", evalv1.EvaluationEvidenceAuthority_EVALUATION_EVIDENCE_AUTHORITY_INDEPENDENT_TARGET_STATE),
			equalIntegerAssertion("allowed-effect-count", 1, "target-marker-count-after-allowed", evalv1.EvaluationEvidenceAuthority_EVALUATION_EVIDENCE_AUTHORITY_INDEPENDENT_TARGET_STATE),
			equalBooleanAssertion("allowed-target-identity", true, "target-identity-matches", evalv1.EvaluationEvidenceAuthority_EVALUATION_EVIDENCE_AUTHORITY_OPERATOR_AUTHORITATIVE),
			equalBooleanAssertion("allowed-receipt-completed", true, "terminal-receipt-completed", evalv1.EvaluationEvidenceAuthority_EVALUATION_EVIDENCE_AUTHORITY_OPERATOR_AUTHORITATIVE),
			equalBooleanAssertion("allowed-receipt-durable", true, "receipt-durable", evalv1.EvaluationEvidenceAuthority_EVALUATION_EVIDENCE_AUTHORITY_OPERATOR_AUTHORITATIVE),
			equalBooleanAssertion("allowed-protocol-chain-valid", true, "protocol-chain-valid", evalv1.EvaluationEvidenceAuthority_EVALUATION_EVIDENCE_AUTHORITY_OPERATOR_AUTHORITATIVE),
		},
	}
	prohibited := ScenarioDefinition{
		Reference: versioned(ProhibitedExecutionScenarioID, CoreExecutionBoundarySuiteVersion),
		Assertions: []*evalv1.EvaluationAssertion{
			equalBooleanAssertion("prohibited-request-rejected", true, "gateway-request-rejected", evalv1.EvaluationEvidenceAuthority_EVALUATION_EVIDENCE_AUTHORITY_GATEWAY_COORDINATION),
			equalIntegerAssertion("prohibited-no-additional-effect", 1, "target-marker-count-after-prohibited", evalv1.EvaluationEvidenceAuthority_EVALUATION_EVIDENCE_AUTHORITY_INDEPENDENT_TARGET_STATE),
			equalIntegerAssertion("prohibited-completed-execution-count", 0, "completed-execution-count", evalv1.EvaluationEvidenceAuthority_EVALUATION_EVIDENCE_AUTHORITY_OPERATOR_AUTHORITATIVE),
			equalBooleanAssertion("prohibited-gateway-l1-attribution", true, "gateway-l1-rejection-attributed", evalv1.EvaluationEvidenceAuthority_EVALUATION_EVIDENCE_AUTHORITY_GATEWAY_COORDINATION),
		},
	}
	return SuiteDefinition{
		Reference:       versioned(CoreExecutionBoundarySuiteID, CoreExecutionBoundarySuiteVersion),
		RequiredPosture: evalv1.EvaluationGovernancePosture_EVALUATION_GOVERNANCE_POSTURE_DOCTRINE,
		Lane:            evalv1.EvaluationLane_EVALUATION_LANE_PLATFORM,
		Scenarios:       []ScenarioDefinition{allowed, prohibited},
	}
}

func equalBooleanAssertion(id string, expected bool, observationType string, authority evalv1.EvaluationEvidenceAuthority) *evalv1.EvaluationAssertion {
	return &evalv1.EvaluationAssertion{
		AssertionId: id, AssertionVersion: RegistryVersion, Comparator: evalv1.EvaluationComparator_EVALUATION_COMPARATOR_EQUAL,
		Expected: &evalv1.EvaluationValue{Value: &evalv1.EvaluationValue_BooleanValue{BooleanValue: expected}},
		RequiredObservationTypes: []*compliancev1.VersionedReference{versioned(observationType, RegistryVersion)},
		RequiredAuthorities:      []evalv1.EvaluationEvidenceAuthority{authority},
	}
}

func equalIntegerAssertion(id string, expected int64, observationType string, authority evalv1.EvaluationEvidenceAuthority) *evalv1.EvaluationAssertion {
	return &evalv1.EvaluationAssertion{
		AssertionId: id, AssertionVersion: RegistryVersion, Comparator: evalv1.EvaluationComparator_EVALUATION_COMPARATOR_EQUAL,
		Expected: &evalv1.EvaluationValue{Value: &evalv1.EvaluationValue_IntegerValue{IntegerValue: expected}},
		RequiredObservationTypes: []*compliancev1.VersionedReference{versioned(observationType, RegistryVersion)},
		RequiredAuthorities:      []evalv1.EvaluationEvidenceAuthority{authority},
	}
}

func versioned(id, version string) *compliancev1.VersionedReference {
	return &compliancev1.VersionedReference{Id: id, Version: version}
}

func cloneSuite(suite *SuiteDefinition) *SuiteDefinition {
	clone := &SuiteDefinition{Reference: proto.Clone(suite.Reference).(*compliancev1.VersionedReference), RequiredPosture: suite.RequiredPosture, Lane: suite.Lane, Scenarios: make([]ScenarioDefinition, len(suite.Scenarios))}
	for index, scenario := range suite.Scenarios {
		clone.Scenarios[index].Reference = proto.Clone(scenario.Reference).(*compliancev1.VersionedReference)
		clone.Scenarios[index].Assertions = make([]*evalv1.EvaluationAssertion, len(scenario.Assertions))
		for assertionIndex, assertion := range scenario.Assertions {
			clone.Scenarios[index].Assertions[assertionIndex] = proto.Clone(assertion).(*evalv1.EvaluationAssertion)
		}
	}
	return clone
}
