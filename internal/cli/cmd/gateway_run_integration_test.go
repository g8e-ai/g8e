//go:build integration

// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package cmd

import (
	"bytes"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/internal/constants"
)

// These tests call cmd.RunE on gateway start/reset/restart commands, which
// invoke pm.StartOperator() — exec'ing the real gateway binary and polling a
// health endpoint (20 × 500ms = 10s timeout). They require the g8e binary to
// be built and are therefore Tier 2 integration tests.

func TestGatewayRestartCmd_NoConfigReturnsError(t *testing.T) {
	chdirTemp(t)

	cmd := gatewayRestartCmd()
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	err := cmd.RunE(cmd, nil)
	require.Error(t, err)
}

func TestGatewayStartCmd_ValidPostureDoesNotReturnPostureError(t *testing.T) {
	setupGatewayTestEnv(t)

	cmd := gatewayStartCmd()
	cmd.Flags().Set("posture", "consensus")
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	err := cmd.RunE(cmd, nil)
	if err != nil {
		assert.NotErrorIs(t, err, constants.ErrInvalidPosture)
	}
}

func TestGatewayResetCmd_ForceFlagSkipsPrompt(t *testing.T) {
	setupGatewayTestEnv(t)

	cmd := gatewayResetCmd()
	cmd.Flags().Set("force", "true")
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	err := cmd.RunE(cmd, nil)
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrProcessStartFailed)
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
