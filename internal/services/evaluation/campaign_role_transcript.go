// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package evaluation

import (
	"strings"
	"unicode/utf8"

	"github.com/g8e-ai/g8e/v2/internal/services/publicdisclosure"
)

// publicToolResultMaxBytes caps one tool result's public text so a large file
// read or command output cannot push the assignment record past the public
// feed's record-size bound. The full result stays bound by trace_digest.
const publicToolResultMaxBytes = 16 * 1024

// PublicRoleTranscript is what one model role actually did during an
// assignment, lifted verbatim from that role's digest-bound g8ee trace: its
// designated output and every tool call with the model's exact arguments, the
// resolved command, and the tool's result.
type PublicRoleTranscript struct {
	Role         string                     `json:"role"`
	Response     string                     `json:"response,omitempty"`
	FinishReason string                     `json:"finish_reason,omitempty"`
	TraceDigest  string                     `json:"trace_digest,omitempty"`
	ToolCalls    []PublicToolCallTranscript `json:"tool_calls,omitempty"`
}

// PublicToolCallTranscript is one executed tool call. ArgumentsHash is
// sha256(ArgumentsJSON) as recorded by g8ee, so readers can check the shown
// arguments against the hash bound into the trace.
type PublicToolCallTranscript struct {
	ToolName        string `json:"tool_name"`
	ArgumentsJSON   string `json:"arguments_json,omitempty"`
	ArgumentsHash   string `json:"arguments_hash,omitempty"`
	Command         string `json:"command,omitempty"`
	Success         bool   `json:"success"`
	ErrorType       string `json:"error_type,omitempty"`
	ResultJSON      string `json:"result_json,omitempty"`
	ResultRedaction string `json:"result_redaction,omitempty"`
}

// BuildPublicRoleTranscript projects one role's g8ee trace into its public
// transcript. Text that must never be published is withheld here rather than
// left for the disclosure validator to reject the whole batch.
func BuildPublicRoleTranscript(role string, trace EvaluationTrace) PublicRoleTranscript {
	transcript := PublicRoleTranscript{
		Role:         role,
		Response:     publicTranscriptText(stringValue(trace["designated_role_output"])),
		FinishReason: stringValue(trace["finish_reason"]),
		TraceDigest:  stringValue(trace["trace_digest"]),
	}
	for _, rawCall := range traceToolRecords(trace, "tool_calls") {
		call, ok := evaluationTrace(rawCall)
		if !ok {
			continue
		}
		success, _ := call["success"].(bool)
		result, redaction := publicToolResultText(stringValue(call["result_json"]))
		transcript.ToolCalls = append(transcript.ToolCalls, PublicToolCallTranscript{
			ToolName:        stringValue(call["tool_name"]),
			ArgumentsJSON:   publicTranscriptText(stringValue(call["arguments_json"])),
			ArgumentsHash:   stringValue(call["arguments_hash"]),
			Command:         publicTranscriptText(stringValue(call["command"])),
			Success:         success,
			ErrorType:       stringValue(call["error_type"]),
			ResultJSON:      result,
			ResultRedaction: redaction,
		})
	}
	return transcript
}

// buildHomogeneousRoleTranscripts returns the single-role transcript for a
// homogeneous assignment's persisted trace.
func buildHomogeneousRoleTranscripts(trace EvaluationTrace) []PublicRoleTranscript {
	if len(trace) == 0 {
		return nil
	}
	role := ""
	if contextValues, ok := evaluationTrace(trace["evaluation_context"]); ok {
		role = stringValue(contextValues["designated_model_role"])
	}
	if role == "" {
		return nil
	}
	return []PublicRoleTranscript{BuildPublicRoleTranscript(role, trace)}
}

// buildFormationRoleTranscripts returns one transcript per formation role, in
// execution order, for roles that ran through g8ee and carry a trace.
func buildFormationRoleTranscripts(evidence *FormationRunEvidence) []PublicRoleTranscript {
	if evidence == nil {
		return nil
	}
	var transcripts []PublicRoleTranscript
	for _, role := range evidence.Result.Roles {
		if len(role.Trace) == 0 {
			continue
		}
		transcripts = append(transcripts, BuildPublicRoleTranscript(role.Role, role.Trace))
	}
	return transcripts
}

func publicTranscriptText(text string) string {
	if publicdisclosure.RestrictedModelText(text) {
		return ""
	}
	return text
}

func publicToolResultText(text string) (string, string) {
	if publicdisclosure.RestrictedModelText(text) {
		return "", "restricted"
	}
	if len(text) <= publicToolResultMaxBytes {
		return text, ""
	}
	cut := publicToolResultMaxBytes
	for cut > 0 && !utf8.RuneStart(text[cut]) {
		cut--
	}
	return strings.Clone(text[:cut]), "truncated"
}
