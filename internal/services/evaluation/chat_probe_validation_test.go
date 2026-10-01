// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package evaluation

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	harnessclient "github.com/g8e-ai/g8e/v2/internal/tools/agent_harness/client"
)

func probeRequest() ChatProbeRequest {
	return ChatProbeRequest{
		AssignmentID:            "assignment-1",
		EvaluationAttemptID:     "attempt-1",
		CampaignID:              "campaign-1",
		RunID:                   "run-1",
		ScenarioID:              "scenario-1",
		TargetOperatorSessionID: "session-1",
		ModelRegistryDigest:     "d" + repeatHex('d', 63),
		EvaluationLane:          "system",
	}
}

func probeTraceBase() EvaluationTrace {
	return EvaluationTrace{
		"schema_version":    "6",
		"chat_execution_id": "exec-1",
		"status":            "completed",
		"completed_at":      "2026-09-15T00:00:00+00:00",
		"evaluation_context": EvaluationTrace{
			"campaign_id":                "campaign-1",
			"run_id":                     "run-1",
			"assignment_id":              "assignment-1",
			"evaluation_attempt_id":      "attempt-1",
			"scenario_id":                "scenario-1",
			"model_registry_digest":      "d" + repeatHex('d', 63),
			"target_operator_session_id": "session-1",
			"evaluation_lane":            "system",
		},
		"model_calls": []any{governedModelCall()},
	}
}

func governedModelCall() EvaluationTrace {
	return EvaluationTrace{
		"agent_role":              "sage",
		"provider":                "G8EProvider",
		"governed_transaction_id": "tx-1",
		"governed_result_digest":  "a" + repeatHex('a', 63),
		"provider_attempt_id":     "attempt-1",
	}
}

// sealedProbeTrace builds the base trace, applies mutate, and stamps the
// digest last, so a test varies exactly one thing and the digest still holds.
func sealedProbeTrace(t *testing.T, mutate func(trace EvaluationTrace)) EvaluationTrace {
	t.Helper()
	trace := probeTraceBase()
	if mutate != nil {
		mutate(trace)
	}
	digest, err := ComputeChatProbeTraceDigest(trace)
	require.NoError(t, err)
	trace["trace_digest"] = digest
	return trace
}

func evalContextOf(trace EvaluationTrace) EvaluationTrace {
	return trace["evaluation_context"].(EvaluationTrace)
}

func TestValidateChatProbeTrace_AcceptsTheBaseTrace(t *testing.T) {
	t.Parallel()
	require.NoError(t, ValidateChatProbeTrace(probeRequest(), sealedProbeTrace(t, nil)))
}

func TestValidateChatProbeTrace_RejectsMalformedTraces(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		mutate  func(trace EvaluationTrace)
		wantErr string
	}{
		{name: "status is not terminal", mutate: func(trace EvaluationTrace) { trace["status"] = "running" }, wantErr: `trace status "running" is not terminal`},
		{name: "status is missing", mutate: func(trace EvaluationTrace) { delete(trace, "status") }, wantErr: `trace status "" is not terminal`},
		{name: "assignment failed without a recorded error", mutate: func(trace EvaluationTrace) { trace["status"] = "failed" }, wantErr: "assignment failed without a recorded error"},
		{name: "assignment failed with the recorded error", mutate: func(trace EvaluationTrace) {
			trace["status"] = "failed"
			trace["error"] = "dispatch: command delivered to no operator subscribers"
		}, wantErr: "assignment failed: dispatch: command delivered to no operator subscribers"},
		{name: "evaluation_context missing", mutate: func(trace EvaluationTrace) { delete(trace, "evaluation_context") }, wantErr: "missing evaluation_context"},
		{name: "evaluation_context of the wrong type", mutate: func(trace EvaluationTrace) { trace["evaluation_context"] = "x" }, wantErr: "missing evaluation_context"},
		{name: "chat_execution_id missing", mutate: func(trace EvaluationTrace) { delete(trace, "chat_execution_id") }, wantErr: "missing chat_execution_id"},
		{name: "completed_at missing", mutate: func(trace EvaluationTrace) { delete(trace, "completed_at") }, wantErr: "missing completed_at"},
		{name: "model_calls missing", mutate: func(trace EvaluationTrace) { delete(trace, "model_calls") }, wantErr: "missing model_calls"},
		{name: "model_calls empty", mutate: func(trace EvaluationTrace) { trace["model_calls"] = []any{} }, wantErr: "missing model_calls"},
		{name: "model_calls of the wrong type", mutate: func(trace EvaluationTrace) { trace["model_calls"] = "x" }, wantErr: "missing model_calls"},
		{
			name:    "only a non-governed provider",
			mutate:  func(trace EvaluationTrace) { trace["model_calls"] = []any{EvaluationTrace{"provider": "ollama"}} },
			wantErr: "no governed model calls recorded",
		},
		{
			name:    "only non-object calls",
			mutate:  func(trace EvaluationTrace) { trace["model_calls"] = []any{"junk"} },
			wantErr: "no governed model calls recorded",
		},
		{
			name: "only failed governed calls",
			mutate: func(trace EvaluationTrace) {
				call := governedModelCall()
				call["succeeded"] = false
				trace["model_calls"] = []any{call}
			},
			wantErr: "no governed model calls recorded",
		},
		{
			name: "governed call without a transaction id",
			mutate: func(trace EvaluationTrace) {
				call := governedModelCall()
				delete(call, "governed_transaction_id")
				trace["model_calls"] = []any{call}
			},
			wantErr: "governed model call missing governed_transaction_id",
		},
		{
			name: "governed call without a result digest",
			mutate: func(trace EvaluationTrace) {
				call := governedModelCall()
				call["governed_result_digest"] = ""
				trace["model_calls"] = []any{call}
			},
			wantErr: "governed model call missing governed_result_digest",
		},
		{
			name: "governed call without a provider attempt id",
			mutate: func(trace EvaluationTrace) {
				call := governedModelCall()
				delete(call, "provider_attempt_id")
				trace["model_calls"] = []any{call}
			},
			wantErr: "governed model call missing provider_attempt_id",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := ValidateChatProbeTrace(probeRequest(), sealedProbeTrace(t, tt.mutate))
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
		})
	}
}

func TestValidateChatProbeTrace_RejectsAnEmptyTraceAndDigestProblems(t *testing.T) {
	t.Parallel()
	err := ValidateChatProbeTrace(probeRequest(), EvaluationTrace{})
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrMissingRequiredField)

	t.Run("missing digest", func(t *testing.T) {
		t.Parallel()
		trace := sealedProbeTrace(t, nil)
		delete(trace, "trace_digest")
		require.ErrorContains(t, ValidateChatProbeTrace(probeRequest(), trace), "missing trace_digest")
	})
	t.Run("digest that does not match the body", func(t *testing.T) {
		t.Parallel()
		trace := sealedProbeTrace(t, nil)
		trace["chat_execution_id"] = "tampered"
		require.ErrorContains(t, ValidateChatProbeTrace(probeRequest(), trace), "trace digest mismatch")
	})
	t.Run("a tampered tool call is caught by the digest", func(t *testing.T) {
		t.Parallel()
		trace := sealedProbeTrace(t, func(trace EvaluationTrace) {
			trace["tool_calls"] = []any{toolCall("c1", toolGrep, false, nil)}
		})
		trace["tool_calls"] = []any{toolCall("c1", toolGrep, true, nil)}
		require.ErrorContains(t, ValidateChatProbeTrace(probeRequest(), trace), "trace digest mismatch")
	})
}

func TestValidateChatProbeTrace_EveryEvaluationContextBindingMustMatchTheRequest(t *testing.T) {
	t.Parallel()
	fields := []string{"campaign_id", "run_id", "assignment_id", "evaluation_attempt_id", "scenario_id", "model_registry_digest", "target_operator_session_id"}
	for _, field := range fields {
		t.Run(field+" differs", func(t *testing.T) {
			t.Parallel()
			trace := sealedProbeTrace(t, func(trace EvaluationTrace) { evalContextOf(trace)[field] = "another-value" })
			err := ValidateChatProbeTrace(probeRequest(), trace)
			require.Error(t, err)
			assert.Contains(t, err.Error(), "evaluation_context."+field)
		})
		t.Run(field+" absent", func(t *testing.T) {
			t.Parallel()
			trace := sealedProbeTrace(t, func(trace EvaluationTrace) { delete(evalContextOf(trace), field) })
			err := ValidateChatProbeTrace(probeRequest(), trace)
			require.Error(t, err)
			assert.Contains(t, err.Error(), "evaluation_context."+field)
		})
	}
}

func TestValidateChatProbeTrace_Lane(t *testing.T) {
	t.Parallel()
	modelRole := probeRequest()
	modelRole.EvaluationLane = "model_role"
	modelRole.DesignatedModelRole = "primary"
	tests := []struct {
		name    string
		req     ChatProbeRequest
		mutate  func(trace EvaluationTrace)
		wantErr string
	}{
		{name: "system lane defaults when the request names none", req: func() ChatProbeRequest { r := probeRequest(); r.EvaluationLane = ""; return r }(), mutate: nil},
		{name: "system lane defaults when the trace names none", req: probeRequest(), mutate: func(trace EvaluationTrace) { delete(evalContextOf(trace), "evaluation_lane") }},
		{
			name: "a model_role request answered by a system trace", req: modelRole,
			mutate:  func(trace EvaluationTrace) { evalContextOf(trace)["designated_model_role"] = "primary" },
			wantErr: `evaluation_context.evaluation_lane="system", want "model_role"`,
		},
		{
			name: "a system request answered by a model_role trace", req: probeRequest(),
			mutate:  func(trace EvaluationTrace) { evalContextOf(trace)["evaluation_lane"] = "model_role" },
			wantErr: `evaluation_context.evaluation_lane="model_role", want "system"`,
		},
		{
			name: "model_role with the designated role echoed", req: modelRole,
			mutate: func(trace EvaluationTrace) {
				evalContextOf(trace)["evaluation_lane"] = "model_role"
				evalContextOf(trace)["designated_model_role"] = "primary"
			},
		},
		{
			name: "model_role with a different designated role", req: modelRole,
			mutate: func(trace EvaluationTrace) {
				evalContextOf(trace)["evaluation_lane"] = "model_role"
				evalContextOf(trace)["designated_model_role"] = "lite"
			},
			wantErr: `evaluation_context.designated_model_role="lite", want "primary"`,
		},
		{
			name: "model_role with no designated role echoed", req: modelRole,
			mutate:  func(trace EvaluationTrace) { evalContextOf(trace)["evaluation_lane"] = "model_role" },
			wantErr: `evaluation_context.designated_model_role="", want "primary"`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := ValidateChatProbeTrace(tt.req, sealedProbeTrace(t, tt.mutate))
			if tt.wantErr == "" {
				require.NoError(t, err)
				return
			}
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
		})
	}
}

func workspaceEcho(ws ScenarioWorkspace) map[string]any {
	return map[string]any{"root": ws.Root, "operator_working_directory": ws.OperatorWorkingDirectory}
}

func TestValidateChatProbeTrace_WorkspaceIsEchoedAndDerivedFromRunAndAttempt(t *testing.T) {
	t.Parallel()
	ws := mustWorkspace(t, "/var/run/g8e", "run-1", "attempt-1")
	otherAttempt := mustWorkspace(t, "/var/run/g8e", "run-1", "attempt-2")
	withWorkspace := func() ChatProbeRequest {
		req := probeRequest()
		req.Workspace = &ws
		return req
	}
	tests := []struct {
		name    string
		req     ChatProbeRequest
		mutate  func(trace EvaluationTrace)
		wantErr string
	}{
		{name: "the echoed workspace", req: withWorkspace(), mutate: func(trace EvaluationTrace) { evalContextOf(trace)["workspace"] = workspaceEcho(ws) }},
		{name: "workspace not echoed", req: withWorkspace(), wantErr: "evaluation_context.workspace is required"},
		{name: "workspace echoed as null", req: withWorkspace(), mutate: func(trace EvaluationTrace) { evalContextOf(trace)["workspace"] = nil }, wantErr: "evaluation_context.workspace is required"},
		{name: "workspace echoed as a string", req: withWorkspace(), mutate: func(trace EvaluationTrace) { evalContextOf(trace)["workspace"] = "/x" }, wantErr: "evaluation_context.workspace must be an object"},
		{
			name: "a different root than the request carried", req: withWorkspace(),
			mutate:  func(trace EvaluationTrace) { evalContextOf(trace)["workspace"] = workspaceEcho(otherAttempt) },
			wantErr: "evaluation_context.workspace mismatch",
		},
		{
			name: "a different operator working directory", req: withWorkspace(),
			mutate: func(trace EvaluationTrace) {
				evalContextOf(trace)["workspace"] = map[string]any{"root": ws.Root, "operator_working_directory": "/elsewhere"}
			},
			wantErr: "evaluation_context.workspace mismatch",
		},
		{
			name: "a request and trace that agree on a root this attempt could not have derived",
			req: func() ChatProbeRequest {
				req := probeRequest()
				req.Workspace = &otherAttempt
				return req
			}(),
			mutate:  func(trace EvaluationTrace) { evalContextOf(trace)["workspace"] = workspaceEcho(otherAttempt) },
			wantErr: "attempt-scoped root",
		},
		{
			name:   "an unrequested workspace that this attempt derives is accepted",
			req:    probeRequest(),
			mutate: func(trace EvaluationTrace) { evalContextOf(trace)["workspace"] = workspaceEcho(ws) },
		},
		{
			name:    "an unrequested workspace belonging to another attempt is rejected",
			req:     probeRequest(),
			mutate:  func(trace EvaluationTrace) { evalContextOf(trace)["workspace"] = workspaceEcho(otherAttempt) },
			wantErr: "attempt-scoped root",
		},
		{
			name:   "an unrequested workspace echoed as null is ignored",
			req:    probeRequest(),
			mutate: func(trace EvaluationTrace) { evalContextOf(trace)["workspace"] = nil },
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := ValidateChatProbeTrace(tt.req, sealedProbeTrace(t, tt.mutate))
			if tt.wantErr == "" {
				require.NoError(t, err)
				return
			}
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
		})
	}
}

func TestValidateChatProbeTrace_WorkspaceMismatchWrapsTheSentinelForAnAttemptThatDoesNotOwnTheRoot(t *testing.T) {
	t.Parallel()
	other := mustWorkspace(t, "/var/run/g8e", "run-1", "attempt-2")
	req := probeRequest()
	req.Workspace = &other
	trace := sealedProbeTrace(t, func(trace EvaluationTrace) { evalContextOf(trace)["workspace"] = workspaceEcho(other) })

	err := ValidateChatProbeTrace(req, trace)

	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrEvaluationWorkspaceUnavailable)
}

func TestValidateChatProbeTrace_SeedThatCannotBeCanonicalized(t *testing.T) {
	t.Parallel()
	req := probeRequest()
	req.Seed = &harnessclient.EnsembleInvestigationSeed{CaseTitle: "Checkout payment timeouts"}

	t.Run("the echoed seed has the wrong shape", func(t *testing.T) {
		t.Parallel()
		trace := sealedProbeTrace(t, func(trace EvaluationTrace) { evalContextOf(trace)["seed"] = "not a seed" })
		require.ErrorContains(t, ValidateChatProbeTrace(req, trace), "canonicalize trace seed")
	})
	t.Run("a seed that echoes exactly", func(t *testing.T) {
		t.Parallel()
		trace := sealedProbeTrace(t, func(trace EvaluationTrace) {
			evalContextOf(trace)["seed"] = map[string]any{"case_title": "Checkout payment timeouts", "case_description": ""}
		})
		require.NoError(t, ValidateChatProbeTrace(req, trace))
	})
}

func TestHasAgentRoleAndRoleOutcomeHelpers(t *testing.T) {
	t.Parallel()
	calls := []any{EvaluationTrace{"agent_role": "sage"}, map[string]any{"agent_role": "triage"}, "junk"}
	assert.True(t, hasAgentRole(calls, "sage"))
	assert.True(t, hasAgentRole(calls, "triage"))
	assert.False(t, hasAgentRole(calls, "dash"))
	assert.False(t, hasAgentRole(nil, "sage"))

	assert.True(t, hasDesignatedRoleOutcome(EvaluationTrace{"role_outcome": "invoked"}, "primary"))
	assert.False(t, hasDesignatedRoleOutcome(EvaluationTrace{"role_outcome": "invoked"}, ""))
	assert.False(t, hasDesignatedRoleOutcome(EvaluationTrace{"role_outcome": "role_not_invoked"}, "primary"))

	assert.True(t, hasControlledRoleAssignment(EvaluationTrace{"controlled_role_assignment": EvaluationTrace{"designated_model_role": "lite"}}, "lite"))
	assert.False(t, hasControlledRoleAssignment(EvaluationTrace{"controlled_role_assignment": EvaluationTrace{"designated_model_role": "lite"}}, "primary"))
	assert.False(t, hasControlledRoleAssignment(EvaluationTrace{}, "lite"))
}

func TestDecodeEvaluationTrace(t *testing.T) {
	t.Parallel()
	trace, err := DecodeEvaluationTrace([]byte(`{"status":"completed","model_calls":[{"agent_role":"sage"}],"tool_turn_limit_reached":true}`))
	require.NoError(t, err)
	assert.Equal(t, "completed", trace["status"])
	assert.True(t, boolValue(trace["tool_turn_limit_reached"]))
	calls, ok := trace["model_calls"].([]any)
	require.True(t, ok)
	assert.True(t, hasAgentRole(calls, "sage"), "nested objects decode to values the helpers understand")

	_, err = DecodeEvaluationTrace([]byte(`{not json`))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "decode evaluation trace")

	null, err := DecodeEvaluationTrace([]byte(`null`))
	require.NoError(t, err)
	assert.Empty(t, null)
}
