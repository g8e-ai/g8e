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

func TestContentCheck_Rules(t *testing.T) {
	ws := ScenarioWorkspace{Root: "/work/workspaces/ws-0123456789abcdef", OperatorWorkingDirectory: "/work"}

	tests := []struct {
		name    string
		output  string
		check   ScenarioContentCheck
		wantOK  bool
		wantMsg string
	}{
		{name: "exact token matches after trimming", output: " READY\n", check: ScenarioContentCheck{ExactToken: "READY"}, wantOK: true},
		{name: "exact token is case sensitive", output: "Ready", check: ScenarioContentCheck{ExactToken: "READY"}, wantMsg: "exact token mismatch"},
		{name: "exact token rejects added punctuation", output: "READY.", check: ScenarioContentCheck{ExactToken: "READY"}, wantMsg: "exact token mismatch"},
		{name: "exact token renders the workspace", output: ws.Root, check: ScenarioContentCheck{ExactToken: "{{workspace}}"}, wantOK: true},

		{name: "exact label ignores case, emphasis and trailing punctuation", output: "**error**.", check: ScenarioContentCheck{ExactLabels: []string{"ERROR"}}, wantOK: true},
		{name: "exact label ignores backticks", output: "`ERROR`", check: ScenarioContentCheck{ExactLabels: []string{"ERROR"}}, wantOK: true},
		{name: "exact label accepts any listed label", output: "noise", check: ScenarioContentCheck{ExactLabels: []string{"noise", "action"}}, wantOK: true},
		{name: "exact label rejects a different label", output: "WARN", check: ScenarioContentCheck{ExactLabels: []string{"ERROR"}}, wantMsg: "exact label mismatch"},
		{name: "exact label rejects a sentence containing the label", output: "The label is ERROR", check: ScenarioContentCheck{ExactLabels: []string{"ERROR"}}, wantMsg: "exact label mismatch"},

		{name: "leading label accepts a bare label", output: "yes", check: ScenarioContentCheck{LeadingLabel: "yes"}, wantOK: true},
		{name: "leading label accepts a label followed by a comma", output: "Yes, the records contradict each other.", check: ScenarioContentCheck{LeadingLabel: "yes"}, wantOK: true},
		{name: "leading label accepts a label followed by a colon", output: "Yes: the evidence satisfies the criterion.", check: ScenarioContentCheck{LeadingLabel: "yes"}, wantOK: true},
		{name: "leading label accepts emphasis and a full stop", output: "**Yes**. The upstream host matches.", check: ScenarioContentCheck{LeadingLabel: "yes"}, wantOK: true},
		{name: "leading label rejects a longer word with the same prefix", output: "Yesterday it was fine", check: ScenarioContentCheck{LeadingLabel: "yes"}, wantMsg: "leading label mismatch"},
		{name: "leading label rejects the opposite label", output: "No, they agree.", check: ScenarioContentCheck{LeadingLabel: "yes"}, wantMsg: "leading label mismatch"},
		{name: "leading label rejects an empty answer", output: "", check: ScenarioContentCheck{LeadingLabel: "yes"}, wantMsg: "leading label mismatch"},

		{name: "word count passes exactly", output: "clear bright blue", check: ScenarioContentCheck{WordCount: 3}, wantOK: true},
		{name: "word count fails when short", output: "blue", check: ScenarioContentCheck{WordCount: 3}, wantMsg: "word count 1, expected 3"},
		{name: "word count fails when long", output: "a very clear blue sky", check: ScenarioContentCheck{WordCount: 3}, wantMsg: "word count 5, expected 3"},

		{name: "one sentence passes", output: "checkout-api timeouts reached 18 percent during the deploy.", check: ScenarioContentCheck{MaxSentences: 1}, wantOK: true},
		{name: "an unterminated sentence counts as one", output: "checkout-api timeouts reached 18 percent", check: ScenarioContentCheck{MaxSentences: 1}, wantOK: true},
		{name: "two sentences fail a one sentence limit", output: "Timeouts rose. The deploy caused it.", check: ScenarioContentCheck{MaxSentences: 1}, wantMsg: "sentence count 2 exceeds maximum 1"},
		{name: "a decimal point is not a sentence end", output: "The rate reached 18.5 percent", check: ScenarioContentCheck{MaxSentences: 1}, wantOK: true},

		{name: "required term group needs one hit", output: "The 503 came from the health endpoint.", check: ScenarioContentCheck{RequiredTerms: [][]string{{"503"}, {"health", "status"}}}, wantOK: true},
		{name: "required terms are case insensitive", output: "SERVICE-A sets it", check: ScenarioContentCheck{RequiredTerms: [][]string{{"service-a"}}}, wantOK: true},
		{name: "every required group must hit", output: "The 503 came from somewhere.", check: ScenarioContentCheck{RequiredTerms: [][]string{{"503"}, {"health"}}}, wantMsg: "missing required term from group [health]"},
		{name: "required terms render the workspace", output: "found in " + ws.Root + "/logs", check: ScenarioContentCheck{RequiredTerms: [][]string{{"{{workspace}}/logs"}}}, wantOK: true},

		{name: "interrogation accepts exactly three numbered yes/no questions", output: "<interrogation>\n1. Did the failure start after the last deploy?\n2. Is the service running on the primary host?\n3) Have you changed the configuration since?\n</interrogation>", check: ScenarioContentCheck{Interrogation: true}, wantOK: true},
		{name: "interrogation rejects prose around the block", output: "A few questions first.\n<interrogation>\n1. Is it down?\n2. Is it slow?\n3. Is it new?\n</interrogation>", check: ScenarioContentCheck{Interrogation: true}, wantMsg: "not a single <interrogation> block"},
		{name: "interrogation rejects an answer with no block", output: "Which service do you mean?", check: ScenarioContentCheck{Interrogation: true}, wantMsg: "not a single <interrogation> block"},
		{name: "interrogation rejects two questions", output: "<interrogation>\n1. Is it down?\n2. Is it slow?\n</interrogation>", check: ScenarioContentCheck{Interrogation: true}, wantMsg: "has 2 questions, want exactly 3"},
		{name: "interrogation rejects four questions", output: "<interrogation>\n1. Is it down?\n2. Is it slow?\n3. Is it new?\n4. Is it local?\n</interrogation>", check: ScenarioContentCheck{Interrogation: true}, wantMsg: "has 4 questions, want exactly 3"},
		{name: "interrogation rejects an open question", output: "<interrogation>\n1. Is it down?\n2. Which service is failing?\n3. Is it new?\n</interrogation>", check: ScenarioContentCheck{Interrogation: true}, wantMsg: "is not strictly yes/no"},
		{name: "interrogation rejects a multiple choice question", output: "<interrogation>\n1. Is it down?\n2. Is it the API or the database?\n3. Is it new?\n</interrogation>", check: ScenarioContentCheck{Interrogation: true}, wantMsg: "is not strictly yes/no"},
		{name: "interrogation rejects an unnumbered line", output: "<interrogation>\n1. Is it down?\n2. Is it slow?\nIs it new?\n</interrogation>", check: ScenarioContentCheck{Interrogation: true}, wantMsg: "is not a numbered question"},

		{name: "forbidden term fails", output: "The value is retry_limit=9", check: ScenarioContentCheck{ForbiddenTerms: []string{"retry_limit=9"}}, wantMsg: `contains forbidden term "retry_limit=9"`},
		{name: "forbidden terms are case insensitive", output: "See AUTH.LOG", check: ScenarioContentCheck{ForbiddenTerms: []string{"auth.log"}}, wantMsg: `contains forbidden term "auth.log"`},
		{name: "forbidden terms absent passes", output: "checkout.log and billing.log", check: ScenarioContentCheck{ForbiddenTerms: []string{"auth.log"}}, wantOK: true},

		{name: "json fields pass", output: `{"status":"OK","code":200}`, check: ScenarioContentCheck{JSONStringFields: map[string]string{"status": "ok"}, JSONIntegerFields: map[string]int64{"code": 200}}, wantOK: true},
		{name: "json inside a fence passes", output: "```json\n{\"status\":\"ok\",\"code\":200}\n```", check: ScenarioContentCheck{JSONStringFields: map[string]string{"status": "ok"}, JSONIntegerFields: map[string]int64{"code": 200}}, wantOK: true},
		{name: "json string field mismatch fails", output: `{"status":"degraded","code":200}`, check: ScenarioContentCheck{JSONStringFields: map[string]string{"status": "ok"}}, wantMsg: `json field "status" mismatch`},
		{name: "json integer field mismatch fails", output: `{"status":"ok","code":500}`, check: ScenarioContentCheck{JSONIntegerFields: map[string]int64{"code": 200}}, wantMsg: `json integer field "code" mismatch`},
		{name: "json integer field as a string fails", output: `{"code":"200"}`, check: ScenarioContentCheck{JSONIntegerFields: map[string]int64{"code": 200}}, wantMsg: `json integer field "code" mismatch`},
		{name: "non json output fails a json check", output: "status ok, code 200", check: ScenarioContentCheck{JSONStringFields: map[string]string{"status": "ok"}}, wantMsg: "output is not valid JSON"},

		{name: "every rule must hold", output: "ready", check: ScenarioContentCheck{ExactLabels: []string{"ready"}, ForbiddenTerms: []string{"ready"}}, wantMsg: "contains forbidden term"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ok, detail := evaluateContentCheck(tc.output, tc.check, ws)
			assert.Equal(t, tc.wantOK, ok, "detail: %s", detail)
			if tc.wantOK {
				assert.Empty(t, detail)
				return
			}
			assert.Contains(t, detail, tc.wantMsg)
		})
	}
}
