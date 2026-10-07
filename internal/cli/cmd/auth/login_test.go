// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package authcmd

import (
	"bytes"
	"context"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/v2/internal/cli/auth"
	"github.com/g8e-ai/g8e/v2/internal/cli/cmd/cmdtest"
	"github.com/g8e-ai/g8e/v2/internal/cli/config"
	"github.com/g8e-ai/g8e/v2/internal/constants"
)

// runLogin executes 'g8e login <args...>' through cobra dispatch, so
// subcommand and alias resolution are exercised, against a mock coordinator.
func runLogin(t *testing.T, mock *mockEnroller, args ...string) (string, error) {
	t.Helper()
	fileSvc, cfg := cmdtest.NewCmdTestEnv(t)
	login := loginCmdWithConfig(
		func(string) (*config.Config, error) { return cfg, nil },
		cmdtest.FileSvcFactoryFor(fileSvc),
		noopCheckOperatorRunning,
		mockEnrollerFactory(mock),
	)
	root := &cobra.Command{Use: "g8e", SilenceUsage: true, SilenceErrors: true}
	root.AddCommand(login)
	var buf bytes.Buffer
	root.SetOut(&buf)
	root.SetErr(&buf)
	root.SetContext(context.Background())
	root.SetArgs(append([]string{"login"}, args...))
	err := root.Execute()
	return buf.String(), err
}

func loginResult() *auth.EnrollmentResult {
	return &auth.EnrollmentResult{UserID: "user-1", CLISessionID: "sess-1"}
}

func TestLoginCmd_WebUsesBrowserLogin(t *testing.T) {
	mock := &mockEnroller{result: loginResult()}
	out, err := runLogin(t, mock, "web")

	require.NoError(t, err)
	require.Equal(t, 1, mock.callCount())
	opts := mock.lastOptions()
	assert.False(t, opts.Headless, "web login must not be headless")
	assert.False(t, opts.NoSystemTrust, "browser login installs OS trust by default")
	assert.False(t, opts.RotateCLI)
	assert.Contains(t, out, "User ID: user-1")
	assert.Contains(t, out, "CLI Session ID: sess-1")
}

func TestLoginCmd_DefaultCLIAndHeadlessAliasAreHeadless(t *testing.T) {
	for _, name := range []string{"", "cli", "headless"} {
		t.Run(name, func(t *testing.T) {
			mock := &mockEnroller{result: loginResult()}
			var args []string
			if name != "" {
				args = []string{name}
			}
			out, err := runLogin(t, mock, args...)

			require.NoError(t, err)
			require.Equal(t, 1, mock.callCount())
			opts := mock.lastOptions()
			assert.True(t, opts.Headless)
			assert.False(t, opts.RotateCLI)
			assert.Contains(t, out, "User ID: user-1")
			assert.Contains(t, out, "CLI Session ID: sess-1")
			assert.True(t, opts.NoSystemTrust, "headless login never installs OS trust")
		})
	}
}

func TestLoginCmd_FlagPropagation(t *testing.T) {
	tests := []struct {
		name         string
		args         []string
		wantNoTrust  bool
		wantRotate   bool
		wantHeadless bool
	}{
		{"bare rotate-cli", []string{"--rotate-cli"}, true, true, true},
		{"web no-system-trust", []string{"web", "--no-system-trust"}, true, false, false},
		{"web both", []string{"web", "--no-system-trust", "--rotate-cli"}, true, true, false},
		{"cli rotate-cli", []string{"cli", "--rotate-cli"}, true, true, true},
		{"headless rotate-cli", []string{"headless", "--rotate-cli"}, true, true, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := &mockEnroller{result: loginResult()}
			_, err := runLogin(t, mock, tt.args...)

			require.NoError(t, err)
			opts := mock.lastOptions()
			assert.Equal(t, tt.wantNoTrust, opts.NoSystemTrust)
			assert.Equal(t, tt.wantRotate, opts.RotateCLI)
			assert.Equal(t, tt.wantHeadless, opts.Headless)
		})
	}
}

func TestLoginCmd_RejectsInvalidInvocations(t *testing.T) {
	tests := []struct {
		name string
		args []string
	}{
		{"unknown subcommand", []string{"bogus"}},
		{"stray argument on subcommand", []string{"cli", "extra"}},
		{"headless has no no-system-trust flag", []string{"cli", "--no-system-trust"}},
		{"bare login has no no-system-trust flag", []string{"--no-system-trust"}},
		{"login has no headless flag", []string{"--headless"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := &mockEnroller{result: loginResult()}
			_, err := runLogin(t, mock, tt.args...)

			require.Error(t, err)
			assert.Zero(t, mock.callCount(), "an invalid invocation must never reach the coordinator")
		})
	}
}

func TestLoginCmd_CoordinatorErrorPropagates(t *testing.T) {
	mock := &mockEnroller{err: constants.ErrSystemTrustInstallFailed}

	_, err := runLogin(t, mock)

	require.ErrorIs(t, err, constants.ErrSystemTrustInstallFailed)
}

func TestLoginCmd_Structure(t *testing.T) {
	login := LoginCmd()
	assert.Equal(t, "login", login.Name())
	assert.Contains(t, login.Short, "headless")

	byName := map[string]*cobra.Command{}
	for _, sub := range login.Commands() {
		byName[sub.Name()] = sub
	}
	require.Contains(t, byName, "web")
	require.Contains(t, byName, "cli")
	assert.Equal(t, []string{"headless"}, byName["cli"].Aliases)
	assert.Empty(t, byName["web"].Aliases)
}
