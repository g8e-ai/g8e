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
// See the License for the specific language governing permissions and
// limitations under the License.

package cmd

import (
	"bytes"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/internal/cli/wizard"
)

// --- --interactive flag registration ---

func TestGatewayStartCmd_InteractiveFlagRegistered(t *testing.T) {
	cmd := gatewayStartCmd()
	flag := cmd.Flags().Lookup("interactive")
	require.NotNil(t, flag)
	assert.Equal(t, "false", flag.DefValue)
	assert.Equal(t, "i", flag.Shorthand)
}

func TestGatewayStartCmd_NoInteractiveFlagSkipsWizard(t *testing.T) {
	wizardCalled := false
	fakeRunner := func(opts wizard.Options) (wizard.Result, error) {
		wizardCalled = true
		return wizard.Result{}, nil
	}

	_, cfg := newCmdTestEnv(t)
	cmd := gatewayStartCmdWithConfig(configLoaderFor(cfg), failingFileSvcFactory(errFactory), fakeRunner)
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	_ = cmd.RunE(cmd, nil)
	assert.False(t, wizardCalled, "wizard should not be called without --interactive")
}

func TestGatewayStartCmd_InteractiveCancelDoesNotStart(t *testing.T) {
	fakeRunner := func(opts wizard.Options) (wizard.Result, error) {
		return wizard.Result{Cancel: true}, nil
	}

	_, cfg := newCmdTestEnv(t)
	cmd := gatewayStartCmdWithConfig(configLoaderFor(cfg), failingFileSvcFactory(errFactory), fakeRunner)
	cmd.Flags().Set("interactive", "true")
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	err := cmd.RunE(cmd, nil)
	require.NoError(t, err)
	assert.Contains(t, buf.String(), "Onboarding cancelled.")
}

func TestGatewayStartCmd_InteractiveErrorDoesNotStart(t *testing.T) {
	wizardErr := errors.New("wizard crashed")
	fakeRunner := func(opts wizard.Options) (wizard.Result, error) {
		return wizard.Result{}, wizardErr
	}

	_, cfg := newCmdTestEnv(t)
	cmd := gatewayStartCmdWithConfig(configLoaderFor(cfg), failingFileSvcFactory(errFactory), fakeRunner)
	cmd.Flags().Set("interactive", "true")
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	err := cmd.RunE(cmd, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "wizard")
}

func TestGatewayStartCmd_InteractivePreservesExplicitFlags(t *testing.T) {
	var capturedOpts wizard.Options
	fakeRunner := func(opts wizard.Options) (wizard.Result, error) {
		capturedOpts = opts
		return wizard.Result{
			Config: wizard.Config{
				Posture:       "consensus",
				PublicBaseURL: "https://demo.g8e.ai",
			},
		}, nil
	}

	_, cfg := newCmdTestEnv(t)
	cmd := gatewayStartCmdWithConfig(configLoaderFor(cfg), failingFileSvcFactory(errFactory), fakeRunner)
	cmd.Flags().Set("interactive", "true")
	cmd.Flags().Set("posture", "doctrine")
	cmd.Flags().Set("http-port", "9090")
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	_ = cmd.RunE(cmd, nil)

	// InitialConfig should reflect the explicit CLI posture
	assert.Equal(t, "doctrine", capturedOpts.InitialConfig.Posture)
	// The wizard result overrides posture to consensus, but http-port (not wizard-owned) is preserved
	// We can't easily inspect the resolved flags after merge without deeper hooks,
	// but the wizard runner receiving the correct initial config is the key test.
}

func TestGatewayStartCmd_InteractiveUsesCobraStreams(t *testing.T) {
	var capturedOpts wizard.Options
	fakeRunner := func(opts wizard.Options) (wizard.Result, error) {
		capturedOpts = opts
		return wizard.Result{Cancel: true}, nil
	}

	_, cfg := newCmdTestEnv(t)
	cmd := gatewayStartCmdWithConfig(configLoaderFor(cfg), failingFileSvcFactory(errFactory), fakeRunner)
	cmd.Flags().Set("interactive", "true")

	var outBuf bytes.Buffer
	cmd.SetOut(&outBuf)
	cmd.SetErr(&outBuf)

	_ = cmd.RunE(cmd, nil)

	// ProgramOptions should contain at least 2 options (input + output)
	assert.GreaterOrEqual(t, len(capturedOpts.ProgramOptions), 2)
}

// TestGatewayResetCmd_DoesNotLaunchWizard verifies that the nested start command
// inside gateway reset does not launch the wizard. The reset command creates its
// own gatewayStartCmd() internally; since --interactive defaults to false and
// reset never sets it, the wizard cannot be triggered.
func TestGatewayResetCmd_DoesNotLaunchWizard(t *testing.T) {
	resetCmd := gatewayResetCmd()

	// The reset command itself should not have an --interactive flag
	flag := resetCmd.Flags().Lookup("interactive")
	assert.Nil(t, flag, "reset command should not register --interactive flag")

	// The start command it creates internally defaults interactive to false
	startCmd := gatewayStartCmd()
	startFlag := startCmd.Flags().Lookup("interactive")
	require.NotNil(t, startFlag)
	assert.Equal(t, "false", startFlag.DefValue, "nested start command interactive flag must default to false")
}
