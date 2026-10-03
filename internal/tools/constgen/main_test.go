// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidateCommittedEventRegistry(t *testing.T) {
	root, err := findRepoRoot()
	require.NoError(t, err)

	eventsPath := filepath.Join(root, "protocol/constants/events.json")
	statusPath := filepath.Join(root, "protocol/constants/status.json")

	events, err := loadRegistry(eventsPath)
	require.NoError(t, err)
	actionTypeMeta, err := loadActionTypeMeta(statusPath)
	require.NoError(t, err)

	require.NoError(t, validateRegistry(events, actionTypeValues(actionTypeMeta)))
}

func TestValidateRegistryRejectsDuplicateWireValues(t *testing.T) {
	reg := registryFile{
		Events: map[string]eventEntry{
			"A": {GoConst: "EventA", Value: "g8e.v1.app.case.created"},
			"B": {GoConst: "EventB", Value: "g8e.v1.app.case.created"},
		},
	}
	err := validateRegistry(reg, map[string]struct{}{"DOCUMENT_UPDATE": {}})
	require.Error(t, err)
}

func TestValidateRegistryRejectsGovernanceOnNonRequest(t *testing.T) {
	reg := registryFile{
		Events: map[string]eventEntry{
			"A": {
				GoConst:   "EventA",
				Value:     "g8e.v1.app.case.created",
				Kind:      "outcome",
				Transport: []string{"governed"},
				Governance: &struct {
					ActionType string `json:"action_type"`
					Payload    string `json:"payload"`
				}{ActionType: "DOCUMENT_UPDATE", Payload: "g8e.operator.v1.DocumentUpdateRequested"},
			},
		},
	}
	err := validateRegistry(reg, map[string]struct{}{"DOCUMENT_UPDATE": {}})
	require.Error(t, err)
}

func TestValidateRegistryRejectsMissingProducers(t *testing.T) {
	reg := registryFile{
		Events: map[string]eventEntry{
			"A": {
				GoConst:     "EventA",
				Value:       "g8e.v1.app.case.created",
				Kind:        "outcome",
				Persistence: "ephemeral",
			},
		},
	}
	err := validateRegistry(reg, map[string]struct{}{})
	require.Error(t, err)
	require.Contains(t, err.Error(), "missing producers")
}

func TestValidateRegistryRejectsMissingOutcomeReference(t *testing.T) {
	reg := registryFile{
		Events: map[string]eventEntry{
			"Request": {
				GoConst:     "EventRequest",
				Value:       "g8e.v1.operator.audit.user.record.requested",
				Kind:        "request",
				Transport:   []string{"pubsub"},
				Producers:   []string{"ensemble"},
				Persistence: "operator.audit_log",
				Outcomes:    []string{"MissingOutcome"},
			},
		},
	}
	err := validateRegistry(reg, map[string]struct{}{})
	require.Error(t, err)
	require.Contains(t, err.Error(), "missing event")
}

func TestValidateRegistryRejectsActionTypeWithoutRequestEvent(t *testing.T) {
	reg := registryFile{
		Events: map[string]eventEntry{
			"Command": {
				GoConst:     "EventOperatorCommandRequested",
				Value:       "g8e.v1.operator.command.requested",
				Kind:        "request",
				Transport:   []string{"governed"},
				Producers:   []string{"ensemble"},
				Persistence: "ephemeral",
				Governance: &struct {
					ActionType string `json:"action_type"`
					Payload    string `json:"payload"`
				}{ActionType: "EXECUTE_BASH", Payload: "g8e.operator.v1.CommandRequested"},
			},
		},
	}
	err := validateRegistry(reg, map[string]struct{}{
		"EXECUTE_BASH": {},
		"FS_READ":      {},
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), `action type "FS_READ" has no governed request event`)
}

func TestLoadRegistryFromCommittedFile(t *testing.T) {
	root, err := findRepoRoot()
	require.NoError(t, err)
	data, err := os.ReadFile(filepath.Join(root, "protocol/constants/events.json"))
	require.NoError(t, err)
	require.True(t, json.Valid(data))
}

// validOutcome is a registry entry that satisfies every validateRegistry rule.
func validOutcome() eventEntry {
	return eventEntry{
		GoConst:     "EventAppCaseCreated",
		Value:       "g8e.v1.app.case.created",
		Kind:        "outcome",
		Transport:   []string{"pubsub"},
		Producers:   []string{"gateway"},
		Persistence: "ephemeral",
	}
}

// validGovernedRequest is a governed request for DOCUMENT_UPDATE that
// satisfies every validateRegistry rule.
func validGovernedRequest() eventEntry {
	entry := validOutcome()
	entry.GoConst = "EventAppDocumentRequested"
	entry.Value = "g8e.v1.app.document.requested"
	entry.Kind = "request"
	entry.Transport = []string{"governed"}
	entry.Governance = &governanceBlock{ActionType: "DOCUMENT_UPDATE", Payload: "g8e.operator.v1.DocumentUpdateRequested"}
	return entry
}

var documentActionTypes = map[string]struct{}{"DOCUMENT_UPDATE": {}}

func TestValidateRegistry_AcceptsGovernedRequestWithItsOutcome(t *testing.T) {
	request := validGovernedRequest()
	request.Outcomes = []string{"Created"}
	reg := registryFile{Events: map[string]eventEntry{"Document": request, "Created": validOutcome()}}

	require.NoError(t, validateRegistry(reg, documentActionTypes))
}

func TestValidateRegistry_AcceptsStreamEventOnSSEWithEphemeralPersistence(t *testing.T) {
	stream := validOutcome()
	stream.Kind = "stream"
	stream.Transport = []string{"sse"}
	stream.Value = "g8e.v1.llm.chat.delta.received"
	reg := registryFile{Events: map[string]eventEntry{"Stream": stream}}

	require.NoError(t, validateRegistry(reg, map[string]struct{}{}))
}

func TestValidateRegistry_RejectsEmptyRegistry(t *testing.T) {
	err := validateRegistry(registryFile{}, map[string]struct{}{})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "events registry is empty")
}

func TestValidateRegistry_RejectsEntriesThatBreakASingleRule(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*eventEntry)
		want   string
	}{
		{name: "missing go const", mutate: func(e *eventEntry) { e.GoConst = "" }, want: "A: missing _go_const"},
		{name: "go const not Event-prefixed", mutate: func(e *eventEntry) { e.GoConst = "AppCaseCreated" }, want: `A: invalid _go_const "AppCaseCreated"`},
		{name: "go const with lowercase after prefix", mutate: func(e *eventEntry) { e.GoConst = "Eventapp" }, want: `A: invalid _go_const "Eventapp"`},
		{name: "missing wire value", mutate: func(e *eventEntry) { e.Value = "" }, want: "A: missing value"},
		{name: "wire value with wrong version", mutate: func(e *eventEntry) { e.Value = "g8e.v2.app.case.created" }, want: `A: invalid wire value "g8e.v2.app.case.created"`},
		{name: "wire value with uppercase segment", mutate: func(e *eventEntry) { e.Value = "g8e.v1.app.Case.created" }, want: `A: invalid wire value "g8e.v1.app.Case.created"`},
		{name: "wire value without entity segment", mutate: func(e *eventEntry) { e.Value = "g8e.v1.app" }, want: `A: invalid wire value "g8e.v1.app"`},
		{name: "unknown kind", mutate: func(e *eventEntry) { e.Kind = "notification" }, want: `A: unknown kind "notification"`},
		{name: "unknown transport", mutate: func(e *eventEntry) { e.Transport = []string{"pubsub", "carrier-pigeon"} }, want: `A: unknown transport "carrier-pigeon"`},
		{name: "unknown producer", mutate: func(e *eventEntry) { e.Producers = []string{"gateway", "intern"} }, want: `A: unknown producer "intern"`},
		{name: "removed dashboard producer", mutate: func(e *eventEntry) { e.Producers = []string{"dashboard"} }, want: `A: unknown producer "dashboard"`},
		{name: "no producers", mutate: func(e *eventEntry) { e.Producers = nil }, want: "A: missing producers"},
		{name: "missing persistence", mutate: func(e *eventEntry) { e.Persistence = "" }, want: "A: missing persistence"},
		{name: "unknown persistence", mutate: func(e *eventEntry) { e.Persistence = "gateway.scratchpad" }, want: `A: unknown persistence "gateway.scratchpad"`},
		{name: "grammar allowlist owner is closed", mutate: func(e *eventEntry) { e.GrammarAllowlistOwner = "gateway" }, want: "A: grammar allowlist is closed"},
		{name: "terminal outside closed list", mutate: func(e *eventEntry) { e.Value = "g8e.v1.app.case.exploded" }, want: `A: grammar: terminal "exploded" not in closed list`},
		{name: "stream event with non-sse transport", mutate: func(e *eventEntry) {
			e.Kind = "stream"
			e.Transport = []string{"pubsub"}
		}, want: "A: stream events must use sse transport only"},
		{name: "stream event with durable persistence", mutate: func(e *eventEntry) {
			e.Kind = "stream"
			e.Transport = []string{"sse"}
			e.Persistence = "gateway.sse_store"
		}, want: "A: stream events must use ephemeral persistence"},
		{name: "outcome references missing event", mutate: func(e *eventEntry) { e.Outcomes = []string{"Ghost"} }, want: `A: outcomes references missing event "Ghost"`},
		{name: "outcome references a request event", mutate: func(e *eventEntry) { e.Outcomes = []string{"B"} }, want: `A: outcomes entry "B" must be kind outcome or fact, got "request"`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			entry := validOutcome()
			tt.mutate(&entry)
			reg := registryFile{Events: map[string]eventEntry{"A": entry, "B": validGovernedRequest()}}

			err := validateRegistry(reg, documentActionTypes)

			require.Error(t, err)
			assert.Contains(t, err.Error(), "event registry validation failed")
			assert.Contains(t, err.Error(), tt.want)
		})
	}
}

func TestValidateRegistry_RejectsGovernanceBlocksThatBreakASingleRule(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*eventEntry)
		want   string
	}{
		{name: "wire value is not a .requested event", mutate: func(e *eventEntry) { e.Value = "g8e.v1.app.document.created" }, want: "A: governance requires .requested wire value"},
		{name: "missing action type", mutate: func(e *eventEntry) { e.Governance.ActionType = "" }, want: "A: governance requires action_type and payload"},
		{name: "missing payload", mutate: func(e *eventEntry) { e.Governance.Payload = "" }, want: "A: governance requires action_type and payload"},
		{name: "action type not in status registry", mutate: func(e *eventEntry) { e.Governance.ActionType = "DELETE_EVERYTHING" }, want: `A: unknown governance.action_type "DELETE_EVERYTHING"`},
		{name: "governed request lacks governed transport", mutate: func(e *eventEntry) { e.Transport = []string{"pubsub"} }, want: "A: governed request missing transport governed"},
		{name: "kind is not request", mutate: func(e *eventEntry) { e.Kind = "fact" }, want: "A: governance requires kind=request"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			entry := validGovernedRequest()
			tt.mutate(&entry)
			reg := registryFile{Events: map[string]eventEntry{"A": entry}}

			err := validateRegistry(reg, documentActionTypes)

			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.want)
		})
	}
}

func TestValidateRegistry_RejectsDuplicateGoConstsAcrossDifferentWireValues(t *testing.T) {
	first := validOutcome()
	second := validOutcome()
	second.Value = "g8e.v1.app.case.updated"
	reg := registryFile{Events: map[string]eventEntry{"A": first, "B": second}}

	err := validateRegistry(reg, map[string]struct{}{})

	require.Error(t, err)
	assert.Contains(t, err.Error(), `B: duplicate _go_const "EventAppCaseCreated" (also A)`)
}

func TestValidateRegistry_RejectsDuplicateWireValueNamingBothKeys(t *testing.T) {
	first := validOutcome()
	second := validOutcome()
	second.GoConst = "EventAppCaseCreatedAgain"
	reg := registryFile{Events: map[string]eventEntry{"A": first, "B": second}}

	err := validateRegistry(reg, map[string]struct{}{})

	require.Error(t, err)
	assert.Contains(t, err.Error(), `B: duplicate wire value "g8e.v1.app.case.created" (also A)`)
}

func TestValidateRegistry_ReportsEveryIssueCountedAndSorted(t *testing.T) {
	broken := validOutcome()
	broken.Producers = nil
	broken.Persistence = ""
	reg := registryFile{Events: map[string]eventEntry{"A": broken}}

	err := validateRegistry(reg, map[string]struct{}{})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "(2 issues):")
	missingPersistence := strings.Index(err.Error(), "A: missing persistence")
	missingProducers := strings.Index(err.Error(), "A: missing producers")
	assert.Less(t, missingPersistence, missingProducers, "issues are sorted alphabetically")
}

func TestValidateRegistry_GovernedRequestWithExtraTransportsStillCoversItsActionType(t *testing.T) {
	request := validGovernedRequest()
	request.Transport = []string{"governed", "pubsub"}
	nonRequest := validOutcome()
	reg := registryFile{Events: map[string]eventEntry{"Request": request, "Outcome": nonRequest}}

	require.NoError(t, validateRegistry(reg, documentActionTypes))

	err := validateRegistry(reg, map[string]struct{}{"DOCUMENT_UPDATE": {}, "FS_LIST": {}})
	require.Error(t, err)
	assert.Contains(t, err.Error(), `action type "FS_LIST" has no governed request event in registry`)
	assert.NotContains(t, err.Error(), `action type "DOCUMENT_UPDATE"`)
}

func TestCheckGrammar_ValidatesTerminalSegmentAgainstClosedList(t *testing.T) {
	tests := []struct {
		name    string
		value   string
		wantErr string
	}{
		{name: "terminal in closed list", value: "g8e.v1.operator.command.requested"},
		{name: "terminal with digits in closed list", value: "g8e.v1.llm.model.tier1"},
		{name: "terminal outside closed list", value: "g8e.v1.app.case.exploded", wantErr: `terminal "exploded" not in closed list`},
		{name: "status updated infix is exempt", value: "g8e.v1.app.status.updated.foo"},
		{name: "stream infix is exempt", value: "g8e.v1.llm.chat.stream.foo"},
		{name: "chunk infix is exempt", value: "g8e.v1.llm.chat.chunk.foo"},
		{name: "delta infix is exempt", value: "g8e.v1.llm.chat.delta.foo"},
		{name: "keepalive infix is exempt", value: "g8e.v1.sse.conn.keepalive.foo"},
		{name: "thinking infix is exempt", value: "g8e.v1.llm.chat.thinking.foo"},
		{name: "infix exemption needs a trailing segment", value: "g8e.v1.app.foo.stream", wantErr: `terminal "stream" not in closed list`},
		{name: "too few segments", value: "g8e.v1.app", wantErr: "grammar: expected g8e.v1.<domain>.<entity>.<terminal>"},
		{name: "prefix only", value: "g8e.v1.", wantErr: "grammar: expected"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := checkGrammar(tt.value)

			if tt.wantErr == "" {
				assert.NoError(t, err)
				return
			}
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
		})
	}
}

func TestFindRepoRoot_WalksUpFromNestedDirectory(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, eventsRel), `{"events":{}}`)
	nested := filepath.Join(root, "internal", "tools", "constgen")
	require.NoError(t, os.MkdirAll(nested, 0o755))
	t.Chdir(nested)

	got, err := findRepoRoot()

	require.NoError(t, err)
	wantRoot, err := filepath.EvalSymlinks(root)
	require.NoError(t, err)
	gotRoot, err := filepath.EvalSymlinks(got)
	require.NoError(t, err)
	assert.Equal(t, wantRoot, gotRoot)
}

func TestFindRepoRoot_FailsWhenNoAncestorHoldsTheEventsRegistry(t *testing.T) {
	t.Chdir(t.TempDir())

	_, err := findRepoRoot()

	require.Error(t, err)
	assert.Contains(t, err.Error(), "could not find protocol/constants/events.json")
}

func TestLoadRegistry_ReportsMissingAndMalformedFiles(t *testing.T) {
	t.Run("missing file", func(t *testing.T) {
		_, err := loadRegistry(filepath.Join(t.TempDir(), "absent.json"))

		require.Error(t, err)
		assert.ErrorIs(t, err, os.ErrNotExist)
	})
	t.Run("malformed json names the file", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "events.json")
		writeFile(t, path, `{"events": {`)

		_, err := loadRegistry(path)

		require.Error(t, err)
		assert.Contains(t, err.Error(), "decode "+path)
	})
	t.Run("governance block decodes into typed fields", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "events.json")
		writeFile(t, path, `{"events":{"A":{"_go_const":"EventA","value":"g8e.v1.a.b.requested","governance":{"action_type":"X","payload":"P"},"reserved":true}}}`)

		got, err := loadRegistry(path)

		require.NoError(t, err)
		require.Contains(t, got.Events, "A")
		assert.Equal(t, "EventA", got.Events["A"].GoConst)
		assert.True(t, got.Events["A"].Reserved)
		require.NotNil(t, got.Events["A"].Governance)
		assert.Equal(t, "X", got.Events["A"].Governance.ActionType)
		assert.Equal(t, "P", got.Events["A"].Governance.Payload)
	})
}

func TestExecute_WriteGeneratesFilesThatCheckModeThenAccepts(t *testing.T) {
	root := newConstgenRoot(t)
	var stdout bytes.Buffer

	require.NoError(t, execute(root, true, &stdout))
	assert.Equal(t, "generated constants from protocol/constants/events.json\n", stdout.String())
	for _, rel := range []string{eventsGoRel, actionTypesGoRel, consoleTSRel} {
		_, err := os.Stat(filepath.Join(root, rel))
		assert.NoError(t, err, rel)
	}

	stdout.Reset()
	require.NoError(t, execute(root, false, &stdout))
	assert.Empty(t, stdout.String(), "check mode is silent on success")
}

func TestExecute_CheckFailsBeforeAnyGeneratedFileExists(t *testing.T) {
	root := newConstgenRoot(t)

	err := execute(root, false, &bytes.Buffer{})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "generated constants are stale:")
}

func TestExecute_CheckDetectsHandEditedGeneratedFile(t *testing.T) {
	root := newConstgenRoot(t)
	require.NoError(t, execute(root, true, &bytes.Buffer{}))
	edited := filepath.Join(root, eventsGoRel)
	original, err := os.ReadFile(edited)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(edited, append(original, []byte("// tampered\n")...), 0o644))

	err = execute(root, false, &bytes.Buffer{})

	require.Error(t, err)
	assert.Contains(t, err.Error(), edited)
	assert.Contains(t, err.Error(), "run make constants")
}

func TestExecute_RejectsInvalidRegistryWithoutWritingAnyGeneratedFile(t *testing.T) {
	root := newConstgenRoot(t)
	writeFile(t, filepath.Join(root, eventsRel), `{"events":{}}`)

	err := execute(root, true, &bytes.Buffer{})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "events registry is empty")
	for _, rel := range []string{eventsGoRel, actionTypesGoRel, consoleTSRel, pythonEventsRel} {
		_, statErr := os.Stat(filepath.Join(root, rel))
		assert.ErrorIs(t, statErr, os.ErrNotExist, rel)
	}
}

func TestExecute_ReportsMissingInputRegistries(t *testing.T) {
	tests := []struct {
		name    string
		remove  string
		wantErr string
	}{
		{name: "events registry", remove: eventsRel, wantErr: eventsRel},
		{name: "status registry", remove: statusRel, wantErr: statusRel},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := newConstgenRoot(t)
			require.NoError(t, os.Remove(filepath.Join(root, tt.remove)))

			err := execute(root, true, &bytes.Buffer{})

			require.Error(t, err)
			assert.ErrorIs(t, err, os.ErrNotExist)
			assert.Contains(t, err.Error(), tt.wantErr)
		})
	}
}

func TestExecute_CommittedGeneratedConstantsAreCurrent(t *testing.T) {
	root, err := findRepoRoot()
	require.NoError(t, err)

	require.NoError(t, execute(root, false, &bytes.Buffer{}))
}
