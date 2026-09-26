// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

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
