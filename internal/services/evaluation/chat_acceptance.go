// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package evaluation

import (
	"fmt"
)

// ChatAcceptanceCaseID names one Phase 1A chat-path vertical gate.
type ChatAcceptanceCaseID string

const (
	ChatAcceptanceCaseSystemBasic        ChatAcceptanceCaseID = "system-basic"
	ChatAcceptanceCaseRolePrimary        ChatAcceptanceCaseID = "role-primary"
	ChatAcceptanceCaseRoleAssistant      ChatAcceptanceCaseID = "role-assistant"
	ChatAcceptanceCaseRoleLite           ChatAcceptanceCaseID = "role-lite"
	ChatAcceptanceCaseBackgroundBarrier  ChatAcceptanceCaseID = "background-barrier"
)

// ChatAcceptanceCase describes one Phase 1A chat-path acceptance probe.
type ChatAcceptanceCase struct {
	ID    ChatAcceptanceCaseID
	Apply func(base ChatProbeRequest) ChatProbeRequest
}

// DefaultChatAcceptanceCases returns the Phase 1A chat-path vertical matrix.
func DefaultChatAcceptanceCases() []ChatAcceptanceCase {
	return []ChatAcceptanceCase{
		{
			ID: ChatAcceptanceCaseSystemBasic,
			Apply: func(base ChatProbeRequest) ChatProbeRequest {
				base.EvaluationLane = "system"
				base.DesignatedModelRole = ""
				base.Message = "Reply with exactly: chat-system-ok"
				return base
			},
		},
		{
			ID: ChatAcceptanceCaseRolePrimary,
			Apply: func(base ChatProbeRequest) ChatProbeRequest {
				base.EvaluationLane = "model_role"
				base.DesignatedModelRole = "primary"
				base.Message = "Reply with exactly: chat-role-primary-ok"
				return base
			},
		},
		{
			ID: ChatAcceptanceCaseRoleAssistant,
			Apply: func(base ChatProbeRequest) ChatProbeRequest {
				base.EvaluationLane = "model_role"
				base.DesignatedModelRole = "assistant"
				base.Message = "Reply with exactly: chat-role-assistant-ok"
				return base
			},
		},
		{
			ID: ChatAcceptanceCaseRoleLite,
			Apply: func(base ChatProbeRequest) ChatProbeRequest {
				base.EvaluationLane = "model_role"
				base.DesignatedModelRole = "lite"
				base.Message = "Reply with exactly: chat-role-lite-ok"
				return base
			},
		},
		{
			ID: ChatAcceptanceCaseBackgroundBarrier,
			Apply: func(base ChatProbeRequest) ChatProbeRequest {
				base.EvaluationLane = "system"
				base.DesignatedModelRole = ""
				base.Message = "Reply with exactly: chat-barrier-ok"
				return base
			},
		},
	}
}

// LookupChatAcceptanceCase returns one case by ID.
func LookupChatAcceptanceCase(id ChatAcceptanceCaseID) (ChatAcceptanceCase, error) {
	for _, candidate := range DefaultChatAcceptanceCases() {
		if candidate.ID == id {
			return candidate, nil
		}
	}
	return ChatAcceptanceCase{}, fmt.Errorf("evaluation: unknown chat acceptance case %q", id)
}

// SelectChatAcceptanceCases resolves the requested case IDs. An empty slice
// selects the full default matrix.
func SelectChatAcceptanceCases(ids []ChatAcceptanceCaseID) ([]ChatAcceptanceCase, error) {
	if len(ids) == 0 {
		return DefaultChatAcceptanceCases(), nil
	}
	selected := make([]ChatAcceptanceCase, 0, len(ids))
	for _, id := range ids {
		candidate, err := LookupChatAcceptanceCase(id)
		if err != nil {
			return nil, err
		}
		selected = append(selected, candidate)
	}
	return selected, nil
}

// ValidateChatAcceptanceCase applies case-specific success checks after the
// shared chat trace validation passes.
func ValidateChatAcceptanceCase(caseID ChatAcceptanceCaseID, req ChatProbeRequest, trace map[string]any) error {
	modelCalls, _ := trace["model_calls"].([]any)
	switch caseID {
	case ChatAcceptanceCaseSystemBasic:
		return nil
	case ChatAcceptanceCaseRolePrimary:
		return validateHomogeneousRoleTrace(trace, req.DesignatedModelRole, modelCalls)
	case ChatAcceptanceCaseRoleAssistant:
		return validateHomogeneousRoleTrace(trace, req.DesignatedModelRole, modelCalls)
	case ChatAcceptanceCaseRoleLite:
		return validateHomogeneousRoleTrace(trace, req.DesignatedModelRole, modelCalls)
	case ChatAcceptanceCaseBackgroundBarrier:
		if !hasAgentRole(modelCalls, "codex") {
			return fmt.Errorf("evaluation: background barrier missing codex model call")
		}
		return nil
	default:
		return fmt.Errorf("evaluation: validate chat acceptance case: unknown case %q", caseID)
	}
}

func validateHomogeneousRoleTrace(trace map[string]any, designatedRole string, modelCalls []any) error {
	if !hasControlledRoleAssignment(trace, designatedRole) {
		return fmt.Errorf("evaluation: homogeneous role trace missing controlled_role_assignment for %q", designatedRole)
	}
	if !hasDesignatedRoleOutcome(trace, designatedRole) {
		return fmt.Errorf("evaluation: homogeneous role trace role_outcome not invoked for %q", designatedRole)
	}
	for _, rawCall := range modelCalls {
		call, ok := rawCall.(map[string]any)
		if !ok {
			continue
		}
		agentRole, _ := call["agent_role"].(string)
		if agentRole != "sage" && agentRole != "dash" {
			continue
		}
		modelRole, _ := call["model_role"].(string)
		if modelRole == designatedRole {
			return nil
		}
	}
	return fmt.Errorf("evaluation: homogeneous role trace missing scored model call for %q", designatedRole)
}
