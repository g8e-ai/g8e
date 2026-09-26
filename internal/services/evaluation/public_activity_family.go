// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package evaluation

import (
	"encoding/json"
	"fmt"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	evalv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/eval/v1"
	"google.golang.org/protobuf/proto"
)

var publicActivitySourceNotCaptured = evalv1.PublicUnavailableReason_PUBLIC_UNAVAILABLE_REASON_SOURCE_NOT_CAPTURED

type publicActivityFamilyWire struct {
	Availability      string          `json:"availability"`
	UnavailableReason string          `json:"unavailable_reason,omitempty"`
	Records           json.RawMessage `json:"records"`
}

type publicActivitySummaryWire struct {
	ModelActivity   json.RawMessage `json:"model_activity"`
	ToolDecisions   json.RawMessage `json:"tool_decisions"`
	ToolCalls       json.RawMessage `json:"tool_calls"`
	PolicyDecisions json.RawMessage `json:"policy_decisions"`
	GovernedActions json.RawMessage `json:"governed_actions"`
}

func publicModelActivityUnavailable(reason evalv1.PublicUnavailableReason) *evalv1.PublicModelActivity {
	return &evalv1.PublicModelActivity{
		Availability:      evalv1.PublicActivityAvailability_PUBLIC_ACTIVITY_AVAILABILITY_UNAVAILABLE,
		UnavailableReason: reason,
		Records:           []*evalv1.PublicModelActivityRecord{},
	}
}

func publicModelActivityObserved(records []*evalv1.PublicModelActivityRecord) *evalv1.PublicModelActivity {
	if records == nil {
		records = []*evalv1.PublicModelActivityRecord{}
	}
	return &evalv1.PublicModelActivity{
		Availability: evalv1.PublicActivityAvailability_PUBLIC_ACTIVITY_AVAILABILITY_OBSERVED,
		Records:      records,
	}
}

func publicToolDecisionActivityUnavailable(reason evalv1.PublicUnavailableReason) *evalv1.PublicToolDecisionActivity {
	return &evalv1.PublicToolDecisionActivity{
		Availability:      evalv1.PublicActivityAvailability_PUBLIC_ACTIVITY_AVAILABILITY_UNAVAILABLE,
		UnavailableReason: reason,
		Records:           []*evalv1.PublicToolDecisionActivityRecord{},
	}
}

func publicToolDecisionActivityObserved(records []*evalv1.PublicToolDecisionActivityRecord) *evalv1.PublicToolDecisionActivity {
	if records == nil {
		records = []*evalv1.PublicToolDecisionActivityRecord{}
	}
	return &evalv1.PublicToolDecisionActivity{
		Availability: evalv1.PublicActivityAvailability_PUBLIC_ACTIVITY_AVAILABILITY_OBSERVED,
		Records:      records,
	}
}

func publicToolCallActivityUnavailable(reason evalv1.PublicUnavailableReason) *evalv1.PublicToolCallActivity {
	return &evalv1.PublicToolCallActivity{
		Availability:      evalv1.PublicActivityAvailability_PUBLIC_ACTIVITY_AVAILABILITY_UNAVAILABLE,
		UnavailableReason: reason,
		Records:           []*evalv1.PublicToolCallActivityRecord{},
	}
}

func publicToolCallActivityObserved(records []*evalv1.PublicToolCallActivityRecord) *evalv1.PublicToolCallActivity {
	if records == nil {
		records = []*evalv1.PublicToolCallActivityRecord{}
	}
	return &evalv1.PublicToolCallActivity{
		Availability: evalv1.PublicActivityAvailability_PUBLIC_ACTIVITY_AVAILABILITY_OBSERVED,
		Records:      records,
	}
}

func publicPolicyActivityUnavailable(reason evalv1.PublicUnavailableReason) *evalv1.PublicPolicyDecisionActivity {
	return &evalv1.PublicPolicyDecisionActivity{
		Availability:      evalv1.PublicActivityAvailability_PUBLIC_ACTIVITY_AVAILABILITY_UNAVAILABLE,
		UnavailableReason: reason,
		Records:           []*evalv1.PublicPolicyDecisionActivityRecord{},
	}
}

func publicPolicyActivityObserved(records []*evalv1.PublicPolicyDecisionActivityRecord) *evalv1.PublicPolicyDecisionActivity {
	if records == nil {
		records = []*evalv1.PublicPolicyDecisionActivityRecord{}
	}
	return &evalv1.PublicPolicyDecisionActivity{
		Availability: evalv1.PublicActivityAvailability_PUBLIC_ACTIVITY_AVAILABILITY_OBSERVED,
		Records:      records,
	}
}

func publicGovernedActionActivityUnavailable(reason evalv1.PublicUnavailableReason) *evalv1.PublicGovernedActionActivity {
	return &evalv1.PublicGovernedActionActivity{
		Availability:      evalv1.PublicActivityAvailability_PUBLIC_ACTIVITY_AVAILABILITY_UNAVAILABLE,
		UnavailableReason: reason,
		Records:           []*evalv1.PublicGovernedActionActivityRecord{},
	}
}

func publicGovernedActionActivityObserved(records []*evalv1.PublicGovernedActionActivityRecord) *evalv1.PublicGovernedActionActivity {
	if records == nil {
		records = []*evalv1.PublicGovernedActionActivityRecord{}
	}
	return &evalv1.PublicGovernedActionActivity{
		Availability: evalv1.PublicActivityAvailability_PUBLIC_ACTIVITY_AVAILABILITY_OBSERVED,
		Records:      records,
	}
}

func marshalPublicActivityFamilyWire(family proto.Message) (json.RawMessage, error) {
	if family == nil {
		return nil, fmt.Errorf("evaluation: marshal public activity family wire: %w", constants.ErrMissingRequiredField)
	}
	canonical, err := evalv1.MarshalCanonical(family)
	if err != nil {
		return nil, fmt.Errorf("evaluation: marshal public activity family wire: %w", err)
	}
	var fields publicActivityFamilyWire
	if err := json.Unmarshal(canonical, &fields); err != nil {
		return nil, fmt.Errorf("evaluation: marshal public activity family wire: %w", err)
	}
	if len(fields.Records) == 0 {
		fields.Records = json.RawMessage("[]")
	}
	body, err := json.Marshal(fields)
	if err != nil {
		return nil, fmt.Errorf("evaluation: marshal public activity family wire: %w", err)
	}
	return body, nil
}

func marshalPublicActivitySummaryWire(summary *evalv1.PublicAssignmentActivitySummary) (json.RawMessage, error) {
	if summary == nil {
		return nil, fmt.Errorf("evaluation: marshal public activity summary wire: %w", constants.ErrMissingRequiredField)
	}
	modelActivity, err := marshalPublicActivityFamilyWire(summary.GetModelActivity())
	if err != nil {
		return nil, err
	}
	toolDecisions, err := marshalPublicActivityFamilyWire(summary.GetToolDecisions())
	if err != nil {
		return nil, err
	}
	toolCalls, err := marshalPublicActivityFamilyWire(summary.GetToolCalls())
	if err != nil {
		return nil, err
	}
	policyDecisions, err := marshalPublicActivityFamilyWire(summary.GetPolicyDecisions())
	if err != nil {
		return nil, err
	}
	governedActions, err := marshalPublicActivityFamilyWire(summary.GetGovernedActions())
	if err != nil {
		return nil, err
	}
	body, err := json.Marshal(publicActivitySummaryWire{
		ModelActivity:   modelActivity,
		ToolDecisions:   toolDecisions,
		ToolCalls:       toolCalls,
		PolicyDecisions: policyDecisions,
		GovernedActions: governedActions,
	})
	if err != nil {
		return nil, fmt.Errorf("evaluation: marshal public activity summary wire: %w", err)
	}
	return body, nil
}
