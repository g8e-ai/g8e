// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package mcp

import (
	"bytes"
	"os"
	"testing"

	"github.com/g8e-ai/g8e/v2/internal/cli/agent"
	authcmd "github.com/g8e-ai/g8e/v2/internal/cli/cmd/auth"

	"github.com/g8e-ai/g8e/v2/internal/cli/cmd/shared"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/testutil"
)

func TestRunMCPAgentRun_NoArgsReturnsError(t *testing.T) {
	err := runMCPAgentRun(nil, false, shared.NewFileSvc, authcmd.PanickingEnrollerFactory())
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrAgentNotFound)
	assert.Contains(t, err.Error(), "specify an agent name")
}

func TestRunMCPAgentRun_UnknownAgentReturnsError(t *testing.T) {
	err := runMCPAgentRun([]string{"unknown-agent"}, false, shared.NewFileSvc, authcmd.PanickingEnrollerFactory())
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrAgentNotFound)
}

func TestAgentVerify_PassesForEveryRegisteredAgentWithoutTheAgentInstalled(t *testing.T) {
	t.Setenv("PATH", "")
	home := testutil.TempDir(t)
	t.Setenv("HOME", home)

	for _, integration := range agent.All() {
		t.Run(string(integration.ID), func(t *testing.T) {
			cmd := agentVerifyCmd()
			var out bytes.Buffer
			cmd.SetOut(&out)
			cmd.SetErr(&out)

			require.NoError(t, runAgentVerify(cmd, string(integration.ID)))
			assert.Contains(t, out.String(), "PASS "+string(integration.ID))
		})
	}

	entries, err := os.ReadDir(home)
	require.NoError(t, err)
	assert.Empty(t, entries, "verify must not touch the user's real agent config")
}

func TestAgentVerify_UnknownAgentFailsClosed(t *testing.T) {
	err := runAgentVerify(agentVerifyCmd(), "cursor")
	require.ErrorIs(t, err, constants.ErrAgentNotFound)
}

func TestExtractURLFromText_FindsApproveURL(t *testing.T) {
	text := `Transaction pending approval: https://g8e.local/approve/abc123 please review`
	url := extractURLFromText(text)
	assert.Contains(t, url, "https://g8e.local/approve/abc123")
}

func TestExtractURLFromText_FindsGenericURL(t *testing.T) {
	text := `Check this out: https://example.com/page`
	url := extractURLFromText(text)
	assert.Equal(t, "https://example.com/page", url)
}

func TestExtractURLFromText_ReturnsEmptyForNoURL(t *testing.T) {
	text := `No URL here`
	url := extractURLFromText(text)
	assert.Empty(t, url)
}

func TestExtractURLFromText_ReturnsEmptyForEmptyString(t *testing.T) {
	url := extractURLFromText("")
	assert.Empty(t, url)
}
