// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package agent

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/v2/internal/constants"
)

func TestAll_ListsExactlyTheSupportedAgents(t *testing.T) {
	ids := make([]constants.AgentBinary, 0, len(registry))
	for _, integration := range All() {
		ids = append(ids, integration.ID)
	}
	assert.Equal(t, []constants.AgentBinary{
		constants.AgentBinaryClaude,
		constants.AgentBinaryCodex,
		constants.AgentBinaryDevin,
		constants.AgentBinaryGemini,
		constants.AgentBinaryGoose,
	}, ids)
}

func TestAll_ReturnsACopy(t *testing.T) {
	all := All()
	all[0].DisplayName = "mutated"
	assert.NotEqual(t, "mutated", All()[0].DisplayName)
}

func TestRegistry_EveryEntryIsFullyDefined(t *testing.T) {
	for _, integration := range All() {
		t.Run(string(integration.ID), func(t *testing.T) {
			assert.NotEmpty(t, integration.DisplayName)
			assert.Equal(t, string(integration.ID), integration.BinaryName)
			assert.NotEmpty(t, integration.ConfigStrategy)
			assert.NotEmpty(t, integration.LaunchStrategy)
			assert.NotEmpty(t, integration.ToolLockdown)
			assert.NotEmpty(t, integration.VerifyHooks, "an agent without verify hooks cannot prove its lockdown")
		})
	}
}

func TestRegistry_OnlyDevinHasPartialLockdown(t *testing.T) {
	for _, integration := range All() {
		want := LockdownStrict
		if integration.ID == constants.AgentBinaryDevin {
			want = LockdownPartial
		}
		assert.Equal(t, want, integration.ToolLockdown, string(integration.ID))
	}
}

func TestLookup(t *testing.T) {
	tests := []struct {
		name    string
		id      string
		want    constants.AgentBinary
		wantErr error
	}{
		{name: "exact", id: "claude", want: constants.AgentBinaryClaude},
		{name: "case insensitive", id: "CLAUDE", want: constants.AgentBinaryClaude},
		{name: "goose", id: "goose", want: constants.AgentBinaryGoose},
		{name: "unknown agent", id: "cursor", wantErr: constants.ErrAgentNotFound},
		{name: "empty", id: "", wantErr: constants.ErrAgentNotFound},
		{name: "shell-looking name is not executed or resolved", id: "npx", wantErr: constants.ErrAgentNotFound},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Lookup(tt.id)
			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got.ID)
		})
	}
}
