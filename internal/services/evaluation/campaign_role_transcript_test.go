// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package evaluation

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/v2/internal/models"
	evalv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/eval/v1"
)

func transcriptTestTrace(role, output string, calls ...map[string]any) EvaluationTrace {
	toolCalls := make([]any, 0, len(calls))
	for _, call := range calls {
		toolCalls = append(toolCalls, call)
	}
	return EvaluationTrace{
		"evaluation_context":     map[string]any{"designated_model_role": role},
		"designated_role_output": output,
		"finish_reason":          "stop",
		"trace_digest":           strings.Repeat("a", 64),
		"tool_calls":             toolCalls,
	}
}

func TestBuildPublicRoleTranscriptLiftsToolCallsVerbatim(t *testing.T) {
	t.Parallel()
	args := `{"path":"logs/auth.log","pattern":"AUTH_FAILURE"}`
	trace := transcriptTestTrace("primary", "Delegating correlation to Assistant.", map[string]any{
		"call_id":        "exec-1",
		"tool_name":      "recursive_grep_search",
		"arguments_json": args,
		"arguments_hash": models.SHA256Hex([]byte(args)),
		"command":        "grep -rn AUTH_FAILURE logs",
		"result_json":    `{"output":"auth.log:3: AUTH_FAILURE","success":true}`,
		"success":        true,
		"error_type":     nil,
	})

	transcripts := buildHomogeneousRoleTranscripts(trace)

	require.Len(t, transcripts, 1)
	transcript := transcripts[0]
	assert.Equal(t, "primary", transcript.Role)
	assert.Equal(t, "Delegating correlation to Assistant.", transcript.Response)
	assert.Equal(t, strings.Repeat("a", 64), transcript.TraceDigest)
	require.Len(t, transcript.ToolCalls, 1)
	call := transcript.ToolCalls[0]
	assert.Equal(t, "recursive_grep_search", call.ToolName)
	assert.Equal(t, args, call.ArgumentsJSON)
	assert.Equal(t, models.SHA256Hex([]byte(call.ArgumentsJSON)), call.ArgumentsHash)
	assert.Equal(t, "grep -rn AUTH_FAILURE logs", call.Command)
	assert.True(t, call.Success)
	assert.Empty(t, call.ErrorType)
	assert.Empty(t, call.ResultRedaction)
}

func TestBuildPublicRoleTranscriptWithholdsRestrictedAndTruncatesLargeResults(t *testing.T) {
	t.Parallel()
	trace := transcriptTestTrace("assistant", "see spiffe://g8e/operator",
		map[string]any{"tool_name": "read_file", "result_json": "-----BEGIN PRIVATE KEY-----", "success": true},
		map[string]any{"tool_name": "read_file", "result_json": strings.Repeat("é", publicToolResultMaxBytes), "success": true},
	)

	transcript := BuildPublicRoleTranscript("assistant", trace)

	assert.Empty(t, transcript.Response)
	require.Len(t, transcript.ToolCalls, 2)
	assert.Empty(t, transcript.ToolCalls[0].ResultJSON)
	assert.Equal(t, "restricted", transcript.ToolCalls[0].ResultRedaction)
	assert.Equal(t, "truncated", transcript.ToolCalls[1].ResultRedaction)
	assert.LessOrEqual(t, len(transcript.ToolCalls[1].ResultJSON), publicToolResultMaxBytes)
	assert.True(t, strings.HasSuffix(transcript.ToolCalls[1].ResultJSON, "é"), "truncation must land on a rune boundary")
}

func TestBuildFormationRoleTranscriptsKeepsExecutionOrderAndSkipsTracelessRoles(t *testing.T) {
	t.Parallel()
	evidence := &FormationRunEvidence{Result: persistedFormationRunResult{Roles: []persistedFormationRoleTelemetry{
		{Role: "lite", Trace: transcriptTestTrace("lite", "triaged")},
		{Role: "assistant"},
		{Role: "primary", Trace: transcriptTestTrace("primary", "final answer")},
	}}}

	transcripts := buildFormationRoleTranscripts(evidence)

	require.Len(t, transcripts, 2)
	assert.Equal(t, "lite", transcripts[0].Role)
	assert.Equal(t, "primary", transcripts[1].Role)
	assert.Equal(t, "final answer", transcripts[1].Response)
}

func TestMarshalPublicAssignmentRecordAcceptsRoleTranscripts(t *testing.T) {
	t.Parallel()
	transcript := BuildPublicRoleTranscript("primary", transcriptTestTrace("primary", "multi\nline answer ../ with paths", map[string]any{
		"tool_name":      "read_file",
		"arguments_json": `{"path":"../fixtures/retry.yaml"}`,
		"result_json":    strings.Repeat("x", 2048),
		"success":        false,
		"error_type":     "PERMISSION_DENIED",
	}))

	_, err := MarshalPublicAssignmentRecord(&PublicAssignmentRecord{
		Projection: &evalv1.PublicAssignmentResultProjection{AssignmentId: "assignment-1", RunId: "run-1", ScenarioId: "scenario-1"},
		Extensions: PublicAssignmentRecordExtensions{RoleTranscripts: []PublicRoleTranscript{transcript}},
	})
	require.NoError(t, err)
}

func TestMarshalPublicAssignmentRecordRejectsMalformedRoleTranscripts(t *testing.T) {
	t.Parallel()
	for name, transcript := range map[string]PublicRoleTranscript{
		"unknown role":        {Role: "operator"},
		"bad trace digest":    {Role: "primary", TraceDigest: "not-a-digest"},
		"restricted response": {Role: "primary", Response: "BEGIN PRIVATE KEY"},
		"bad arguments hash":  {Role: "primary", ToolCalls: []PublicToolCallTranscript{{ToolName: "read_file", ArgumentsHash: "abc"}}},
		"unnamed tool":        {Role: "primary", ToolCalls: []PublicToolCallTranscript{{}}},
		"unknown redaction":   {Role: "primary", ToolCalls: []PublicToolCallTranscript{{ToolName: "read_file", ResultRedaction: "hidden"}}},
	} {
		_, err := MarshalPublicAssignmentRecord(&PublicAssignmentRecord{
			Projection: &evalv1.PublicAssignmentResultProjection{AssignmentId: "assignment-1", RunId: "run-1", ScenarioId: "scenario-1"},
			Extensions: PublicAssignmentRecordExtensions{RoleTranscripts: []PublicRoleTranscript{transcript}},
		})
		assert.Error(t, err, name)
	}
}
