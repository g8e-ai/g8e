// Copyright (c) 2026 Lateralus Labs, LLC.
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language and governing permissions and
// limitations under the License.

package cmd

import (
	"bytes"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGatewayCleanCmd_ConfirmYesProceedsPastPrompt(t *testing.T) {
	setupGatewayTestEnv(t)

	cmd := gatewayCleanCmd()
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	cmd.SetIn(strings.NewReader("y\n"))

	err := cmd.RunE(cmd, nil)
	require.NoError(t, err)
	assert.Contains(t, buf.String(), "Clean complete")
}

func TestGatewayCleanCmd_ConfirmYesUppercaseProceeds(t *testing.T) {
	setupGatewayTestEnv(t)

	cmd := gatewayCleanCmd()
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	cmd.SetIn(strings.NewReader("Y\n"))

	err := cmd.RunE(cmd, nil)
	require.NoError(t, err)
	assert.Contains(t, buf.String(), "Clean complete")
}

func TestGatewayCleanCmd_AbortsOnNViaCmdSetIn(t *testing.T) {
	setupGatewayTestEnv(t)

	cmd := gatewayCleanCmd()
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	cmd.SetIn(strings.NewReader("n\n"))

	err := cmd.RunE(cmd, nil)
	require.NoError(t, err)
	assert.Contains(t, buf.String(), "Aborted")
	assert.NotContains(t, buf.String(), "Clean complete")
}

func TestGatewayCleanCmd_AbortsOnEmptyResponseViaCmdSetIn(t *testing.T) {
	setupGatewayTestEnv(t)

	cmd := gatewayCleanCmd()
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	cmd.SetIn(strings.NewReader("\n"))

	err := cmd.RunE(cmd, nil)
	require.NoError(t, err)
	assert.Contains(t, buf.String(), "Aborted")
}

func TestGatewayResetCmd_AbortsOnNViaCmdSetIn(t *testing.T) {
	setupGatewayTestEnv(t)

	cmd := gatewayResetCmd()
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	cmd.SetIn(strings.NewReader("n\n"))

	err := cmd.RunE(cmd, nil)
	require.NoError(t, err)
	assert.Contains(t, buf.String(), "Aborted")
}

func TestGatewayResetCmd_ConfirmYesTriggersStopWhichFails(t *testing.T) {
	setupGatewayTestEnv(t)

	cmd := gatewayResetCmd()
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	cmd.SetIn(strings.NewReader("y\n"))

	err := cmd.RunE(cmd, nil)
	require.Error(t, err)
}

func TestGatewayResetCmd_ConfirmYesUppercaseTriggersStop(t *testing.T) {
	setupGatewayTestEnv(t)

	cmd := gatewayResetCmd()
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	cmd.SetIn(strings.NewReader("Y\n"))

	err := cmd.RunE(cmd, nil)
	require.Error(t, err)
}

func TestGatewayCleanCmd_ForceFlagSkipsPromptViaCmdSetIn(t *testing.T) {
	setupGatewayTestEnv(t)

	cmd := gatewayCleanCmd()
	cmd.Flags().Set("force", "true")
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	err := cmd.RunE(cmd, nil)
	require.NoError(t, err)
	assert.Contains(t, buf.String(), "Clean complete")
}

func TestGatewayCleanCmd_PromptTextContainsWarning(t *testing.T) {
	setupGatewayTestEnv(t)

	cmd := gatewayCleanCmd()
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	cmd.SetIn(strings.NewReader("n\n"))

	err := cmd.RunE(cmd, nil)
	require.NoError(t, err)

	output := buf.String()
	assert.Contains(t, output, "WARNING")
	assert.Contains(t, output, "permanently destroyed")
	assert.Contains(t, output, "Continue? [y/N]")
}

func TestGatewayResetCmd_PromptTextContainsResetSteps(t *testing.T) {
	setupGatewayTestEnv(t)

	cmd := gatewayResetCmd()
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	cmd.SetIn(strings.NewReader("n\n"))

	err := cmd.RunE(cmd, nil)
	require.NoError(t, err)

	output := buf.String()
	assert.Contains(t, output, "This command will:")
	assert.Contains(t, output, "Stop all running g8e services")
	assert.Contains(t, output, "Wipe the SQLite databases")
	assert.Contains(t, output, "Preserve your existing TLS/PKI")
	assert.Contains(t, output, "Continue? [y/N]")
}
