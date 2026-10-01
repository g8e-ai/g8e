// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package scenarios

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/v2/internal/cli/agent"
)

func TestVerifyLauncherConfigs_CoversEveryRegisteredAgentWithoutItsBinary(t *testing.T) {
	t.Setenv("PATH", "")

	lines, err := verifyLauncherConfigs("/fake/g8e")
	require.NoError(t, err)

	all := agent.All()
	require.Len(t, lines, len(all))
	for i, integration := range all {
		assert.Contains(t, lines[i], string(integration.ID))
		assert.Contains(t, lines[i], string(integration.ToolLockdown))
	}
}

func TestAssertStdioBridgeWiring(t *testing.T) {
	const good = `{"mcpServers":{"g8e":{"command":"/fake/g8e","args":["mcp","stdio","--app","claude"]}}}`

	tests := []struct {
		name    string
		wiring  string
		binary  string
		app     string
		wantErr bool
	}{
		{name: "bridge wired under the agent's app name", wiring: good, binary: "/fake/g8e", app: "claude"},
		{name: "points at another binary", wiring: good, binary: "/usr/bin/npx", app: "claude", wantErr: true},
		{name: "selects another app identity", wiring: good, binary: "/fake/g8e", app: "codex", wantErr: true},
		{name: "not the stdio bridge", wiring: `{"command":"/fake/g8e","args":["serve","--app","claude"]}`, binary: "/fake/g8e", app: "claude", wantErr: true},
		{name: "empty binary path cannot prove anything", wiring: good, binary: "", app: "claude", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := assertStdioBridgeWiring(tt.wiring, tt.binary, tt.app)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
		})
	}
}
