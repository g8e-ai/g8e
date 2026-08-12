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
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/internal/cli/auth"
	"github.com/g8e-ai/g8e/internal/cli/config"
	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/g8e-ai/g8e/internal/services/fs"
)

func TestEnrollCmdWithConfig_ConfigLoaderError(t *testing.T) {
	failLoader := func(string) (*config.Config, error) {
		return nil, errors.New("config load error")
	}

	cmd := enrollCmdWithConfig(failLoader, newFileSvc, auth.CheckOperatorRunning)
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	err := cmd.RunE(cmd, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "config load error")
}

func TestEnrollCmdWithConfig_OperatorNotRunningReturnsError(t *testing.T) {
	config.SetEndpointOverride("127.0.0.1:1")
	defer config.SetEndpointOverride("")

	_, cfg := newCmdTestEnv(t)

	loader := func(string) (*config.Config, error) {
		return cfg, nil
	}

	cmd := enrollCmdWithConfig(loader, newFileSvc, auth.CheckOperatorRunning)
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	err := cmd.RunE(cmd, nil)
	require.Error(t, err)
}

func TestEnrollCmdWithConfig_NoTPMFlagOnNonWindows(t *testing.T) {
	cmd := enrollCmdWithConfig(loadConfig, newFileSvc, auth.CheckOperatorRunning)
	tpmFlag := cmd.Flags().Lookup("tpm")
	if tpmFlag != nil {
		assert.Equal(t, "false", tpmFlag.DefValue)
	}
}

func TestEnrollCmdWithConfig_HasRunE(t *testing.T) {
	cmd := enrollCmdWithConfig(loadConfig, newFileSvc, auth.CheckOperatorRunning)
	require.NotNil(t, cmd.RunE)
}

// TestEnrollCmdWithConfig_FlagsRegistered verifies the command registers the
// --no-system-trust and --rotate-cli flags with the correct defaults. The
// coordinator itself is exercised by internal/cli/auth tests; this asserts the
// command adapter exposes the new options.
func TestEnrollCmdWithConfig_FlagsRegistered(t *testing.T) {
	fileSvc, cfg := newCmdTestEnv(t)
	cmd := enrollCmdWithConfig(func(string) (*config.Config, error) { return cfg, nil }, fileSvcFactoryFor(fileSvc), auth.CheckOperatorRunning)
	noSystemTrustFlag := cmd.Flags().Lookup("no-system-trust")
	require.NotNil(t, noSystemTrustFlag)
	assert.Equal(t, "false", noSystemTrustFlag.DefValue)
	rotateFlag := cmd.Flags().Lookup("rotate-cli")
	require.NotNil(t, rotateFlag)
	assert.Equal(t, "false", rotateFlag.DefValue)
}

func TestEnrollCmdWithConfig_UsesInjectedConfigLoader(t *testing.T) {
	called := false
	loader := func(string) (*config.Config, error) {
		called = true
		return nil, errors.New("injected error")
	}

	cmd := enrollCmdWithConfig(loader, newFileSvc, auth.CheckOperatorRunning)
	_ = cmd.RunE(cmd, nil)

	assert.True(t, called, "config loader should have been called")
}

func TestEnrollCmdWithConfig_PropagatesConfigError(t *testing.T) {
	expectedErr := constants.ErrConfigLoadFailed
	loader := func(string) (*config.Config, error) {
		return nil, expectedErr
	}

	cmd := enrollCmdWithConfig(loader, newFileSvc, auth.CheckOperatorRunning)
	err := cmd.RunE(cmd, nil)
	require.Error(t, err)
}

// TestEnrollCmdWithConfig_GatewayDownReturnsError verifies that when the
// coordinator factory is the production default and the gateway is
// unreachable, the command returns an error rather than silently succeeding.
// This replaces the old TestPerformEnroll_* tests that exercised the deleted
// performEnroll function.
func TestEnrollCmdWithConfig_GatewayDownReturnsError(t *testing.T) {
	config.SetEndpointOverride("127.0.0.1:1")
	defer config.SetEndpointOverride("")

	fileSvc, cfg := newCmdTestEnv(t)

	cmd := enrollCmdWithConfig(func(string) (*config.Config, error) {
		return cfg, nil
	}, fileSvcFactoryFor(fileSvc), auth.CheckOperatorRunning)
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	// The production coordinator factory will try to reach the gateway
	// (CheckBootstrapStatus) and fail because the endpoint is unreachable.
	// CheckOperatorRunning already fails before the coordinator is built,
	// so this asserts the preflight check fails closed.
	err := cmd.RunE(cmd, nil)
	require.Error(t, err)
}

// TestLogoutCmdWithConfig_RemovesLocalCredentials verifies logout routes
// through CredentialStore.Clear and removes the local CLI credential material
// (credentials JSON, CLI cert, CLI key) while leaving the OS root CA untouched
// (Clear never touches the OS trust store).
func TestLogoutCmdWithConfig_RemovesLocalCredentials(t *testing.T) {
	fileSvc, cfg := newCmdTestEnv(t)

	creds := &auth.Credentials{
		OperatorSessionID: "op-sess-123",
		UserID:            "user-456",
		OperatorID:        "op-789",
		CLISessionID:      "cli-sess-abc",
	}
	require.NoError(t, auth.SaveCredentials(fileSvc, cfg, creds))
	require.NoError(t, fileSvc.WriteFile(context.Background(), mustRel(t, fileSvc, cfg.CLICertFile()), []byte("cli-cert"), constants.PermFilePrivate))
	require.NoError(t, fileSvc.WriteFile(context.Background(), mustRel(t, fileSvc, cfg.CLIKeyFile()), []byte("cli-key"), constants.PermFilePrivate))

	cmd := logoutCmdWithConfig(func(_ string) (*config.Config, error) { return cfg, nil }, fileSvcFactoryFor(fileSvc))
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	ctx := context.Background()
	cmd.SetContext(ctx)

	require.NoError(t, cmd.RunE(cmd, nil))
	assert.Contains(t, buf.String(), "Logged out successfully")

	exists, err := fileSvc.FileExists(context.Background(), mustRel(t, fileSvc, cfg.CredentialsFile()))
	require.NoError(t, err)
	assert.False(t, exists)
	exists, err = fileSvc.FileExists(context.Background(), mustRel(t, fileSvc, cfg.CLICertFile()))
	require.NoError(t, err)
	assert.False(t, exists)
	exists, err = fileSvc.FileExists(context.Background(), mustRel(t, fileSvc, cfg.CLIKeyFile()))
	require.NoError(t, err)
	assert.False(t, exists)
}

// --- Mock Enroller for command-layer tests (Phase 3, §11.5) ---

// mockEnroller records the options it was called with and returns a canned
// EnrollmentResult/error. It satisfies the Enroller interface without any
// network I/O, sudo, or browser launches.
type mockEnroller struct {
	mu           sync.Mutex
	lastOpts     auth.EnrollmentOptions
	calls        int
	result       *auth.EnrollmentResult
	err          error
	panickOnCall bool
}

func (m *mockEnroller) Enroll(ctx context.Context, opts auth.EnrollmentOptions) (*auth.EnrollmentResult, error) {
	m.mu.Lock()
	m.calls++
	m.lastOpts = opts
	m.mu.Unlock()
	if m.panickOnCall {
		panic("mockEnroller: enrollCoordinatorFactory should not have been called")
	}
	return m.result, m.err
}

func (m *mockEnroller) callCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.calls
}

func (m *mockEnroller) lastOptions() auth.EnrollmentOptions {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.lastOpts
}

// withMockEnroller swaps enrollCoordinatorFactory for the duration of fn and
// restores it on return. It returns the mock so the caller can assert on it.
func withMockEnroller(mock *mockEnroller, fn func()) {
	orig := enrollCoordinatorFactory
	enrollCoordinatorFactory = func(_ auth.OutputFunc, _ fs.RuntimeFileService, _ *config.Config) Enroller {
		return mock
	}
	defer func() { enrollCoordinatorFactory = orig }()
	fn()
}

// noopCheckOperatorRunning is a checkOperatorRunning stub that always succeeds,
// so command-layer tests can reach the coordinator without a running gateway.
func noopCheckOperatorRunning(_ *config.Config) error { return nil }

// --- §11.5 command-layer tests ---

// TestEnrollCmd_OptionPropagation verifies that --no-system-trust and
// --rotate-cli flag values reach EnrollmentOptions on the coordinator.
func TestEnrollCmd_OptionPropagation(t *testing.T) {
	tests := []struct {
		name        string
		args        []string
		wantNoTrust bool
		wantRotate  bool
	}{
		{"defaults", nil, false, false},
		{"no-system-trust only", []string{"--no-system-trust"}, true, false},
		{"rotate-cli only", []string{"--rotate-cli"}, false, true},
		{"both flags", []string{"--no-system-trust", "--rotate-cli"}, true, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			fileSvc, cfg := newCmdTestEnv(t)
			mock := &mockEnroller{
				result: &auth.EnrollmentResult{
					UserID:       "user-1",
					CLISessionID: "sess-1",
				},
			}

			withMockEnroller(mock, func() {
				cmd := enrollCmdWithConfig(
					func(string) (*config.Config, error) { return cfg, nil },
					fileSvcFactoryFor(fileSvc),
					noopCheckOperatorRunning,
				)
				var buf bytes.Buffer
				cmd.SetOut(&buf)
				cmd.SetErr(&buf)
				cmd.SetContext(context.Background())
				cmd.SetArgs(tc.args)
				require.NoError(t, cmd.ParseFlags(tc.args))
				require.NoError(t, cmd.RunE(cmd, nil))
			})

			assert.Equal(t, 1, mock.callCount(), "coordinator should be called once")
			opts := mock.lastOptions()
			assert.Equal(t, tc.wantNoTrust, opts.NoSystemTrust, "NoSystemTrust mismatch")
			assert.Equal(t, tc.wantRotate, opts.RotateCLI, "RotateCLI mismatch")
		})
	}
}

// TestEnrollCmd_CoordinatorErrorPropagates verifies that an error from the
// coordinator is returned by the command (covers the trust-failure-stops-
// before-browser case at the command layer: the coordinator returns
// ErrSystemTrustInstallFailed and the command surfaces it).
func TestEnrollCmd_CoordinatorErrorPropagates(t *testing.T) {
	fileSvc, cfg := newCmdTestEnv(t)
	expectedErr := constants.ErrSystemTrustInstallFailed
	mock := &mockEnroller{err: expectedErr}

	var cmdErr error
	withMockEnroller(mock, func() {
		cmd := enrollCmdWithConfig(
			func(string) (*config.Config, error) { return cfg, nil },
			fileSvcFactoryFor(fileSvc),
			noopCheckOperatorRunning,
		)
		var buf bytes.Buffer
		cmd.SetOut(&buf)
		cmd.SetErr(&buf)
		cmd.SetContext(context.Background())
		cmdErr = cmd.RunE(cmd, nil)
	})

	require.Error(t, cmdErr)
	assert.ErrorIs(t, cmdErr, expectedErr)
}

// TestEnrollCmd_HealthyReusedIdentityNoRotate verifies that when the
// coordinator reports a reused identity (Reused=true), the command prints
// "Reusing existing CLI identity" and does NOT issue a new certificate. This
// is the command-layer assertion of the "healthy auth enroll does not rotate"
// invariant — the mock coordinator records that it was called with
// RotateCLI=false and returns Reused=true.
func TestEnrollCmd_HealthyReusedIdentityNoRotate(t *testing.T) {
	fileSvc, cfg := newCmdTestEnv(t)
	mock := &mockEnroller{
		result: &auth.EnrollmentResult{
			Reused:       true,
			UserID:       "user-reused",
			CLISessionID: "sess-reused",
		},
	}

	var buf bytes.Buffer
	withMockEnroller(mock, func() {
		cmd := enrollCmdWithConfig(
			func(string) (*config.Config, error) { return cfg, nil },
			fileSvcFactoryFor(fileSvc),
			noopCheckOperatorRunning,
		)
		buf = bytes.Buffer{}
		cmd.SetOut(&buf)
		cmd.SetErr(&buf)
		cmd.SetContext(context.Background())
		require.NoError(t, cmd.RunE(cmd, nil))
	})

	out := buf.String()
	assert.Contains(t, out, "Reusing existing CLI identity")
	assert.Contains(t, out, "user-reused")
	assert.Contains(t, out, "sess-reused")
	// The coordinator was called with RotateCLI=false (no --rotate-cli flag).
	assert.False(t, mock.lastOptions().RotateCLI, "should not force rotation on healthy identity")
}

// TestEnrollCmd_RotateCLIFlagForcesRotation verifies that --rotate-cli sets
// RotateCLI=true on the coordinator options, which is the command-layer
// assertion that the flag is wired through to the rotation path.
func TestEnrollCmd_RotateCLIFlagForcesRotation(t *testing.T) {
	fileSvc, cfg := newCmdTestEnv(t)
	mock := &mockEnroller{
		result: &auth.EnrollmentResult{
			Source:       auth.EnrollmentSourceRotation,
			UserID:       "user-rot",
			CLISessionID: "sess-rot",
		},
	}

	withMockEnroller(mock, func() {
		cmd := enrollCmdWithConfig(
			func(string) (*config.Config, error) { return cfg, nil },
			fileSvcFactoryFor(fileSvc),
			noopCheckOperatorRunning,
		)
		var buf bytes.Buffer
		cmd.SetOut(&buf)
		cmd.SetErr(&buf)
		cmd.SetContext(context.Background())
		require.NoError(t, cmd.ParseFlags([]string{"--rotate-cli"}))
		require.NoError(t, cmd.RunE(cmd, nil))
	})

	assert.True(t, mock.lastOptions().RotateCLI, "--rotate-cli should set RotateCLI=true")
}

// TestEnrollCmd_NoSystemTrustFlagWired verifies that --no-system-trust sets
// NoSystemTrust=true on the coordinator options. The coordinator's
// ensureSystemTrust method skips the installer when this is true (already
// tested in internal/cli/auth); this test asserts the flag reaches the
// coordinator from the command layer.
func TestEnrollCmd_NoSystemTrustFlagWired(t *testing.T) {
	fileSvc, cfg := newCmdTestEnv(t)
	mock := &mockEnroller{
		result: &auth.EnrollmentResult{
			UserID:       "user-nstrust",
			CLISessionID: "sess-nstrust",
		},
	}

	withMockEnroller(mock, func() {
		cmd := enrollCmdWithConfig(
			func(string) (*config.Config, error) { return cfg, nil },
			fileSvcFactoryFor(fileSvc),
			noopCheckOperatorRunning,
		)
		var buf bytes.Buffer
		cmd.SetOut(&buf)
		cmd.SetErr(&buf)
		cmd.SetContext(context.Background())
		require.NoError(t, cmd.ParseFlags([]string{"--no-system-trust"}))
		require.NoError(t, cmd.RunE(cmd, nil))
	})

	assert.True(t, mock.lastOptions().NoSystemTrust, "--no-system-trust should set NoSystemTrust=true")
}

// TestEnrollCmd_SystemTrustInstalledOutput verifies that when the coordinator
// reports SystemTrustInstalled=true, the command prints the browser-restart
// guidance line.
func TestEnrollCmd_SystemTrustInstalledOutput(t *testing.T) {
	fileSvc, cfg := newCmdTestEnv(t)
	mock := &mockEnroller{
		result: &auth.EnrollmentResult{
			Source:               auth.EnrollmentSourceBootstrap,
			UserID:               "user-trust",
			CLISessionID:         "sess-trust",
			SystemTrustInstalled: true,
		},
	}

	var buf bytes.Buffer
	withMockEnroller(mock, func() {
		cmd := enrollCmdWithConfig(
			func(string) (*config.Config, error) { return cfg, nil },
			fileSvcFactoryFor(fileSvc),
			noopCheckOperatorRunning,
		)
		buf = bytes.Buffer{}
		cmd.SetOut(&buf)
		cmd.SetErr(&buf)
		cmd.SetContext(context.Background())
		require.NoError(t, cmd.RunE(cmd, nil))
	})

	assert.Contains(t, buf.String(), "System trust: installed gateway root CA")
	assert.Contains(t, buf.String(), "Restart any open browsers")
}

// TestLogoutCmd_OSRootCARetained verifies that logout removes local CLI
// credential material but does NOT delete the OS root CA file. This is the
// §11.5 3.9 assertion: the OS root CA is shared and must survive logout.
func TestLogoutCmd_OSRootCARetained(t *testing.T) {
	fileSvc, cfg := newCmdTestEnv(t)

	// Seed credentials + CLI cert/key + trust bundle (the OS root CA file).
	creds := &auth.Credentials{
		OperatorSessionID: "op-sess-123",
		UserID:            "user-456",
		OperatorID:        "op-789",
		CLISessionID:      "cli-sess-abc",
	}
	require.NoError(t, auth.SaveCredentials(fileSvc, cfg, creds))
	require.NoError(t, fileSvc.WriteFile(context.Background(), mustRel(t, fileSvc, cfg.CLICertFile()), []byte("cli-cert"), constants.PermFilePrivate))
	require.NoError(t, fileSvc.WriteFile(context.Background(), mustRel(t, fileSvc, cfg.CLIKeyFile()), []byte("cli-key"), constants.PermFilePrivate))
	require.NoError(t, fileSvc.WriteFile(context.Background(), mustRel(t, fileSvc, cfg.ResolvedTrustBundlePath()), []byte("root-ca-pem"), constants.PermFilePrivate))

	cmd := logoutCmdWithConfig(func(_ string) (*config.Config, error) { return cfg, nil }, fileSvcFactoryFor(fileSvc))
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	cmd.SetContext(context.Background())

	require.NoError(t, cmd.RunE(cmd, nil))
	assert.Contains(t, buf.String(), "Logged out successfully")

	// Local CLI credential material is gone.
	for _, p := range []string{cfg.CredentialsFile(), cfg.CLICertFile(), cfg.CLIKeyFile()} {
		exists, err := fileSvc.FileExists(context.Background(), mustRel(t, fileSvc, p))
		require.NoError(t, err)
		assert.False(t, exists, "local credential file should be removed: %s", p)
	}

	// The OS root CA (trust bundle) is retained — it is shared and may be
	// used by another runtime or gateway. Logout must NOT delete it.
	exists, err := fileSvc.FileExists(context.Background(), mustRel(t, fileSvc, cfg.ResolvedTrustBundlePath()))
	require.NoError(t, err)
	assert.True(t, exists, "OS root CA (trust bundle) must NOT be removed by logout")
}

// TestMCPStdio_DoesNotInvokeEnrollment verifies that the `mcp stdio` command
// path does NOT call enrollCoordinatorFactory. Direct `mcp stdio` is a
// credential consumer only — it loads credentials and builds an mTLS
// connection, never enrolling, opening a browser, or installing system trust.
// This is the §11.5 3.8 negative assertion.
func TestMCPStdio_DoesNotInvokeEnrollment(t *testing.T) {
	fileSvc, _ := newCmdTestEnv(t)

	// Swap the factory with a panicking mock. If the stdio path calls it,
	// the test panics.
	mock := &mockEnroller{panickOnCall: true}

	withMockEnroller(mock, func() {
		cmd := mcpStdioCmdWithConfig(fileSvcFactoryFor(fileSvc))
		var buf bytes.Buffer
		cmd.SetOut(&buf)
		cmd.SetErr(&buf)
		cmd.SetContext(context.Background())

		// runMCPStdioProxy will fail because there is no gateway and no
		// credentials, but it must fail WITHOUT calling the coordinator
		// factory. The error is expected; the assertion is that no panic
		// occurred (the mock was never called).
		_ = cmd.RunE(cmd, nil)
	})

	assert.Equal(t, 0, mock.callCount(), "mcp stdio must NOT invoke the enrollment coordinator factory")
}
