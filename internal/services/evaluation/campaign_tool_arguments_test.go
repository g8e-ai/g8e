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
)

func TestValidateToolCall_Constraints(t *testing.T) {
	ws := ScenarioWorkspace{Root: "/work/workspaces/ws-0123456789abcdef", OperatorWorkingDirectory: "/work"}

	grep := []ToolArgumentValidator{{
		ToolName: "recursive_grep_search",
		Arguments: []ToolArgumentConstraint{
			{Name: "pattern", RegexMatches: []string{"AUTH_FAILURE"}, RegexRejects: []string{"", "AUTH_SUCCESS"}},
			{Name: "path", PathUnder: "{{workspace}}"},
		},
	}}
	read := []ToolArgumentValidator{{
		ToolName:  "file_read_on_operator",
		Arguments: []ToolArgumentConstraint{{Name: "file_path", PathEquals: "{{workspace}}/config/retry-config.env"}},
	}}
	run := []ToolArgumentValidator{{
		ToolName: "run_commands_with_operator",
		Arguments: []ToolArgumentConstraint{{
			Name:           "request",
			RequiredTerms:  [][]string{{"health.txt"}},
			ForbiddenTerms: []string{"rm ", "delete"},
		}},
		CommandForbiddenTerms: []string{"rm ", ">", "sed -i"},
	}}
	enum := []ToolArgumentValidator{{
		ToolName:  "query_investigation_context",
		Arguments: []ToolArgumentConstraint{{Name: "data_type", OneOf: []string{"history_trail", "operator_actions"}}},
	}}
	equals := []ToolArgumentValidator{{
		ToolName:  "query_investigation_context",
		Arguments: []ToolArgumentConstraint{{Name: "data_type", Equals: "history_trail"}},
	}}

	tests := []struct {
		name       string
		call       traceToolCall
		validators []ToolArgumentValidator
		wantOK     bool
		wantDetail string
	}{
		{name: "a tool without a validator always passes", call: traceToolCall{ToolName: "get_command_constraints", ArgumentsJSON: `{}`}, validators: grep, wantOK: true},

		{name: "grep with the exact pattern and the workspace passes", call: traceToolCall{ToolName: "recursive_grep_search", ArgumentsJSON: `{"pattern":"AUTH_FAILURE","path":"` + ws.Root + `"}`}, validators: grep, wantOK: true},
		{name: "grep with an escaped-equivalent pattern passes", call: traceToolCall{ToolName: "recursive_grep_search", ArgumentsJSON: `{"pattern":"AUTH_FAIL(URE)","path":"` + ws.Root + `/logs"}`}, validators: grep, wantOK: true},
		{name: "grep with the wrong pattern fails", call: traceToolCall{ToolName: "recursive_grep_search", ArgumentsJSON: `{"pattern":"FOO","path":"` + ws.Root + `"}`}, validators: grep, wantDetail: `argument "pattern"`},
		{name: "grep with an empty pattern fails", call: traceToolCall{ToolName: "recursive_grep_search", ArgumentsJSON: `{"pattern":"","path":"` + ws.Root + `"}`}, validators: grep, wantDetail: `argument "pattern"`},
		{name: "grep with an over-broad pattern fails", call: traceToolCall{ToolName: "recursive_grep_search", ArgumentsJSON: `{"pattern":"AUTH_.*","path":"` + ws.Root + `"}`}, validators: grep, wantDetail: `unexpectedly matched "AUTH_SUCCESS"`},
		{name: "grep with an invalid regex fails", call: traceToolCall{ToolName: "recursive_grep_search", ArgumentsJSON: `{"pattern":"AUTH(","path":"` + ws.Root + `"}`}, validators: grep, wantDetail: "regex compilation failed"},
		{name: "grep outside the workspace fails", call: traceToolCall{ToolName: "recursive_grep_search", ArgumentsJSON: `{"pattern":"AUTH_FAILURE","path":"/etc"}`}, validators: grep, wantDetail: `argument "path"`},
		{name: "grep path escaping the workspace with dot dots fails", call: traceToolCall{ToolName: "recursive_grep_search", ArgumentsJSON: `{"pattern":"AUTH_FAILURE","path":"` + ws.Root + `/../ws-other"}`}, validators: grep, wantDetail: `argument "path"`},
		{name: "grep with a relative workspace path resolves against the working directory", call: traceToolCall{ToolName: "recursive_grep_search", ArgumentsJSON: `{"pattern":"AUTH_FAILURE","path":"workspaces/ws-0123456789abcdef"}`}, validators: grep, wantOK: true},
		{name: "a missing argument fails", call: traceToolCall{ToolName: "recursive_grep_search", ArgumentsJSON: `{"pattern":"AUTH_FAILURE"}`}, validators: grep, wantDetail: `argument "path" is missing`},
		{name: "a non string argument fails", call: traceToolCall{ToolName: "recursive_grep_search", ArgumentsJSON: `{"pattern":7,"path":"` + ws.Root + `"}`}, validators: grep, wantDetail: `argument "pattern" is not a string`},
		{name: "malformed arguments json fails", call: traceToolCall{ToolName: "recursive_grep_search", ArgumentsJSON: `{`}, validators: grep, wantDetail: "invalid arguments json"},

		{name: "read of the exact file passes", call: traceToolCall{ToolName: "file_read_on_operator", ArgumentsJSON: `{"file_path":"` + ws.Root + `/config/retry-config.env"}`}, validators: read, wantOK: true},
		{name: "read of an unclean path to the exact file passes", call: traceToolCall{ToolName: "file_read_on_operator", ArgumentsJSON: `{"file_path":"` + ws.Root + `/config/./retry-config.env"}`}, validators: read, wantOK: true},
		{name: "read of the decoy file fails", call: traceToolCall{ToolName: "file_read_on_operator", ArgumentsJSON: `{"file_path":"` + ws.Root + `/config/retry-config.env.bak"}`}, validators: read, wantDetail: `argument "file_path"`},

		{name: "a run request naming the file passes", call: traceToolCall{ToolName: "run_commands_with_operator", ArgumentsJSON: `{"request":"print status/health.txt"}`, Command: "cat status/health.txt"}, validators: run, wantOK: true},
		{name: "a run request missing the required term fails", call: traceToolCall{ToolName: "run_commands_with_operator", ArgumentsJSON: `{"request":"list the status directory"}`}, validators: run, wantDetail: "missing required term"},
		{name: "a run request with a forbidden term fails", call: traceToolCall{ToolName: "run_commands_with_operator", ArgumentsJSON: `{"request":"delete health.txt"}`}, validators: run, wantDetail: `argument "request"`},
		{name: "a generated command with a forbidden term fails", call: traceToolCall{ToolName: "run_commands_with_operator", ArgumentsJSON: `{"request":"print health.txt"}`, Command: "cat health.txt > out"}, validators: run, wantDetail: "command contains forbidden term"},

		{name: "one of accepts a listed value", call: traceToolCall{ToolName: "query_investigation_context", ArgumentsJSON: `{"data_type":"operator_actions"}`}, validators: enum, wantOK: true},
		{name: "one of rejects an unlisted value", call: traceToolCall{ToolName: "query_investigation_context", ArgumentsJSON: `{"data_type":"memories"}`}, validators: enum, wantDetail: `argument "data_type"`},
		{name: "equals accepts the value", call: traceToolCall{ToolName: "query_investigation_context", ArgumentsJSON: `{"data_type":"history_trail"}`}, validators: equals, wantOK: true},
		{name: "equals rejects another value", call: traceToolCall{ToolName: "query_investigation_context", ArgumentsJSON: `{"data_type":"operator_actions"}`}, validators: equals, wantDetail: `argument "data_type"`},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ok, detail := validateToolCall(tc.call, tc.validators, ws)
			assert.Equal(t, tc.wantOK, ok, "detail: %s", detail)
			if tc.wantOK {
				assert.Empty(t, detail)
				return
			}
			assert.Contains(t, detail, tc.wantDetail)
		})
	}
}
