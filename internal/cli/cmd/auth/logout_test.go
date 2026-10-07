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
	"encoding/json"
	"errors"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/v2/internal/cli/auth"
	"github.com/g8e-ai/g8e/v2/internal/cli/cmd/cmdtest"
	"github.com/g8e-ai/g8e/v2/internal/cli/config"
	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/services/fs"
)

// fakeLogoutClient records the scopes it was asked to log out and returns a
// canned result, so the command layer is tested without network I/O.
type fakeLogoutClient struct {
	scopes []constants.LogoutScope
	result auth.CLISessionLogout
	err    error
}

func (f *fakeLogoutClient) Logout(_ context.Context, _ fs.RuntimeFileService, scope constants.LogoutScope) (auth.CLISessionLogout, error) {
	f.scopes = append(f.scopes, scope)
	if f.err != nil {
		return auth.CLISessionLogout{}, f.err
	}
	result := f.result
	result.Scope = scope
	return result, nil
}

func fakeLogoutFactory(c *fakeLogoutClient) LogoutClientFactory {
	return func(*config.Config) LogoutClient { return c }
}

func successfulLogoutResult() auth.CLISessionLogout {
	return auth.CLISessionLogout{
		UserID:                    "user-456",
		WebSessionsTerminated:     2,
		CLISessionsTerminated:     3,
		CLICertificatesRevoked:    1,
		UnboundOperatorSessionIDs: []string{"op-sess-1", "op-sess-2"},
	}
}

// logoutEnv seeds local CLI credential material and the shared trust bundle.
type logoutEnv struct {
	fileSvc fs.RuntimeFileService
	cfg     *config.Config
}

func newLogoutEnv(t *testing.T, loggedIn bool) logoutEnv {
	t.Helper()
	fileSvc, cfg := cmdtest.NewCmdTestEnv(t)
	if loggedIn {
		require.NoError(t, auth.SaveCredentials(fileSvc, cfg, &auth.Credentials{
			OperatorSessionID: "op-sess-123",
			UserID:            "user-456",
			OperatorID:        "op-789",
			CLISessionID:      "cli-sess-abc",
		}))
		ctx := context.Background()
		require.NoError(t, fileSvc.WriteFile(ctx, cmdtest.MustRel(t, fileSvc, cfg.CLICertFile()), []byte("cli-cert"), constants.PermFilePrivate))
		require.NoError(t, fileSvc.WriteFile(ctx, cmdtest.MustRel(t, fileSvc, cfg.CLIKeyFile()), []byte("cli-key"), constants.PermFilePrivate))
		require.NoError(t, fileSvc.WriteFile(ctx, cmdtest.MustRel(t, fileSvc, cfg.ResolvedTrustBundlePath()), []byte("root-ca-pem"), constants.PermFilePrivate))
	}
	return logoutEnv{fileSvc: fileSvc, cfg: cfg}
}

func (e logoutEnv) fileExists(t *testing.T, absPath string) bool {
	t.Helper()
	exists, err := e.fileSvc.FileExists(context.Background(), cmdtest.MustRel(t, e.fileSvc, absPath))
	require.NoError(t, err)
	return exists
}

func (e logoutEnv) requireLocalCredentials(t *testing.T, want bool) {
	t.Helper()
	for _, p := range []string{e.cfg.CredentialsFile(), e.cfg.CLICertFile(), e.cfg.CLIKeyFile()} {
		assert.Equal(t, want, e.fileExists(t, p), "local credential file %s present=%v expected", p, want)
	}
}

// runLogout executes 'g8e logout <args...>' through cobra dispatch.
func runLogoutCmd(t *testing.T, env logoutEnv, client *fakeLogoutClient, args ...string) (string, error) {
	t.Helper()
	logout := logoutCmdWithConfig(cmdtest.ConfigLoaderFor(env.cfg), cmdtest.FileSvcFactoryFor(env.fileSvc), fakeLogoutFactory(client))
	root := &cobra.Command{Use: "g8e", SilenceUsage: true, SilenceErrors: true}
	root.PersistentFlags().Bool("json", false, "")
	root.AddCommand(logout)
	var buf bytes.Buffer
	root.SetOut(&buf)
	root.SetErr(&buf)
	root.SetContext(context.Background())
	root.SetArgs(append([]string{"logout"}, args...))
	err := root.Execute()
	return buf.String(), err
}

func TestLogoutCmd_Structure(t *testing.T) {
	logout := LogoutCmd()
	assert.Equal(t, "logout", logout.Name())

	byName := map[string]*cobra.Command{}
	for _, sub := range logout.Commands() {
		byName[sub.Name()] = sub
	}
	require.Contains(t, byName, "web")
	require.Contains(t, byName, "cli")

	assert.NotNil(t, logout.Flags().Lookup("local-only"))
	assert.NotNil(t, byName["cli"].Flags().Lookup("local-only"))
	assert.Nil(t, byName["web"].Flags().Lookup("local-only"), "web logout clears no local credentials, so --local-only makes no sense")
}

func TestLogoutCmd_NoActiveSessionDoesNotContactGateway(t *testing.T) {
	env := newLogoutEnv(t, false)
	client := &fakeLogoutClient{result: successfulLogoutResult()}

	out, err := runLogoutCmd(t, env, client)

	require.NoError(t, err)
	assert.Contains(t, out, "No active session found")
	assert.Empty(t, client.scopes, "with no local identity there is nothing to authenticate a logout with")
}

func TestLogoutCmd_BareLogoutEndsEverythingThenClearsLocalCredentials(t *testing.T) {
	env := newLogoutEnv(t, true)
	client := &fakeLogoutClient{result: successfulLogoutResult()}

	out, err := runLogoutCmd(t, env, client)

	require.NoError(t, err)
	assert.Equal(t, []constants.LogoutScope{constants.LogoutScopeAll}, client.scopes)
	assert.Contains(t, out, "Logged out of all web and CLI sessions")
	assert.Contains(t, out, "Web sessions ended:        2")
	assert.Contains(t, out, "CLI sessions ended:        3")
	assert.Contains(t, out, "CLI certificates revoked:  1")
	assert.Contains(t, out, "Operators unbound:         2")
	env.requireLocalCredentials(t, false)
	assert.True(t, env.fileExists(t, env.cfg.ResolvedTrustBundlePath()), "the shared OS root CA must survive logout")
}

func TestLogoutCmd_CLISubcommandEndsCLISessionsAndClearsLocalCredentials(t *testing.T) {
	env := newLogoutEnv(t, true)
	client := &fakeLogoutClient{result: successfulLogoutResult()}

	out, err := runLogoutCmd(t, env, client, "cli")

	require.NoError(t, err)
	assert.Equal(t, []constants.LogoutScope{constants.LogoutScopeCLI}, client.scopes)
	assert.Contains(t, out, "Logged out of all CLI sessions")
	assert.NotContains(t, out, "Web sessions ended")
	env.requireLocalCredentials(t, false)
}

func TestLogoutCmd_WebSubcommandKeepsLocalCredentials(t *testing.T) {
	env := newLogoutEnv(t, true)
	client := &fakeLogoutClient{result: successfulLogoutResult()}

	out, err := runLogoutCmd(t, env, client, "web")

	require.NoError(t, err)
	assert.Equal(t, []constants.LogoutScope{constants.LogoutScopeWeb}, client.scopes)
	assert.Contains(t, out, "Logged out of all web sessions")
	assert.NotContains(t, out, "CLI certificates revoked")
	env.requireLocalCredentials(t, true)
}

func TestLogoutCmd_GatewayFailureChangesNothingLocally(t *testing.T) {
	for _, args := range [][]string{nil, {"cli"}} {
		env := newLogoutEnv(t, true)
		boom := errors.New("gateway unreachable")
		client := &fakeLogoutClient{err: boom}

		_, err := runLogoutCmd(t, env, client, args...)

		require.ErrorIs(t, err, boom, "args %v", args)
		assert.Contains(t, err.Error(), "--local-only", "the error must say how to proceed offline (args %v)", args)
		env.requireLocalCredentials(t, true)
	}
}

func TestLogoutCmd_LocalOnlySkipsGatewayAndSaysSo(t *testing.T) {
	for _, args := range [][]string{{"--local-only"}, {"cli", "--local-only"}} {
		env := newLogoutEnv(t, true)
		client := &fakeLogoutClient{err: errors.New("must not be called")}

		out, err := runLogoutCmd(t, env, client, args...)

		require.NoError(t, err, "args %v", args)
		assert.Empty(t, client.scopes, "--local-only must not contact the gateway (args %v)", args)
		assert.Contains(t, out, "were not changed", "output must not imply server-side sessions ended (args %v)", args)
		assert.NotContains(t, out, "Logged out of all")
		env.requireLocalCredentials(t, false)
	}
}

func TestLogoutCmd_RejectsInvalidInvocations(t *testing.T) {
	tests := []struct {
		name string
		args []string
	}{
		{"unknown subcommand", []string{"bogus"}},
		{"stray argument", []string{"web", "extra"}},
		{"web has no local-only flag", []string{"web", "--local-only"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			env := newLogoutEnv(t, true)
			client := &fakeLogoutClient{result: successfulLogoutResult()}

			_, err := runLogoutCmd(t, env, client, tt.args...)

			require.Error(t, err)
			assert.Empty(t, client.scopes)
			env.requireLocalCredentials(t, true)
		})
	}
}

func TestLogoutCmd_JSONOutput(t *testing.T) {
	env := newLogoutEnv(t, true)
	client := &fakeLogoutClient{result: successfulLogoutResult()}

	out, err := runLogoutCmd(t, env, client, "--json")

	require.NoError(t, err)
	var got logoutOutput
	require.NoError(t, json.Unmarshal([]byte(out), &got), out)
	assert.True(t, got.Success)
	assert.Equal(t, constants.LogoutScopeAll, got.Scope)
	assert.Equal(t, "user-456", got.UserID)
	assert.True(t, got.LocalCredentialsCleared)
	assert.False(t, got.LocalOnly)
	assert.Equal(t, 2, got.WebSessionsTerminated)
	assert.Equal(t, 3, got.CLISessionsTerminated)
	assert.Equal(t, 1, got.CLICertificatesRevoked)
	assert.Equal(t, []string{"op-sess-1", "op-sess-2"}, got.UnboundOperatorSessionIDs)
}

func TestLogoutCmd_ConfigLoaderErrorPropagates(t *testing.T) {
	env := newLogoutEnv(t, true)
	boom := errors.New("config load error")
	logout := logoutCmdWithConfig(func(string) (*config.Config, error) { return nil, boom }, cmdtest.FileSvcFactoryFor(env.fileSvc), fakeLogoutFactory(&fakeLogoutClient{}))
	logout.SetOut(&bytes.Buffer{})
	logout.SetErr(&bytes.Buffer{})

	require.ErrorIs(t, logout.RunE(logout, nil), boom)
}
