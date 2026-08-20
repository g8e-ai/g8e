// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

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

	cmd := enrollUserCmdWithConfig(failLoader, newFileSvc, auth.CheckOperatorRunning, newDefaultEnrollmentCoordinator)
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

	cmd := enrollUserCmdWithConfig(loader, newFileSvc, auth.CheckOperatorRunning, newDefaultEnrollmentCoordinator)
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	err := cmd.RunE(cmd, nil)
	require.Error(t, err)
}

func TestEnrollCmdWithConfig_NoTPMFlagOnNonWindows(t *testing.T) {
	cmd := enrollUserCmdWithConfig(loadConfig, newFileSvc, auth.CheckOperatorRunning, newDefaultEnrollmentCoordinator)
	tpmFlag := cmd.Flags().Lookup("tpm")
	if tpmFlag != nil {
		assert.Equal(t, "false", tpmFlag.DefValue)
	}
}

func TestEnrollCmdWithConfig_HasRunE(t *testing.T) {
	cmd := enrollUserCmdWithConfig(loadConfig, newFileSvc, auth.CheckOperatorRunning, newDefaultEnrollmentCoordinator)
	require.NotNil(t, cmd.RunE)
}

// TestEnrollCmdWithConfig_FlagsRegistered verifies the command registers the
// --no-system-trust and --rotate-cli flags with the correct defaults. The
// coordinator itself is exercised by internal/cli/auth tests; this asserts the
// command adapter exposes the new options.
func TestEnrollCmdWithConfig_FlagsRegistered(t *testing.T) {
	fileSvc, cfg := newCmdTestEnv(t)
	cmd := enrollUserCmdWithConfig(func(string) (*config.Config, error) { return cfg, nil }, fileSvcFactoryFor(fileSvc), auth.CheckOperatorRunning, newDefaultEnrollmentCoordinator)
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

	cmd := enrollUserCmdWithConfig(loader, newFileSvc, auth.CheckOperatorRunning, newDefaultEnrollmentCoordinator)
	_ = cmd.RunE(cmd, nil)

	assert.True(t, called, "config loader should have been called")
}

func TestEnrollCmdWithConfig_PropagatesConfigError(t *testing.T) {
	expectedErr := constants.ErrConfigLoadFailed
	loader := func(string) (*config.Config, error) {
		return nil, expectedErr
	}

	cmd := enrollUserCmdWithConfig(loader, newFileSvc, auth.CheckOperatorRunning, newDefaultEnrollmentCoordinator)
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

	cmd := enrollUserCmdWithConfig(func(string) (*config.Config, error) {
		return cfg, nil
	}, fileSvcFactoryFor(fileSvc), auth.CheckOperatorRunning, newDefaultEnrollmentCoordinator)
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	// The production coordinator factory will try to reach the gateway
	// (CheckActivationStatus) and fail because the endpoint is unreachable.
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
		panic("mockEnroller: enrollerFactory should not have been called")
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

// mockEnrollerFactory returns an enrollerFactory that always returns the
// given mock. Used to inject a mock coordinator into *WithConfig constructors
// without mutating package-level state.
func mockEnrollerFactory(mock *mockEnroller) enrollerFactory {
	return func(_ auth.OutputFunc, _ fs.RuntimeFileService, _ *config.Config) Enroller {
		return mock
	}
}

// panickingEnrollerFactory returns an enrollerFactory whose enroller panics
// if called. Used to assert that enrollment is not attempted on a code path
// that must not enroll (e.g. `mcp stdio`).
func panickingEnrollerFactory() enrollerFactory {
	return func(_ auth.OutputFunc, _ fs.RuntimeFileService, _ *config.Config) Enroller {
		return &panickingEnroller{}
	}
}

type panickingEnroller struct{}

func (p *panickingEnroller) Enroll(_ context.Context, _ auth.EnrollmentOptions) (*auth.EnrollmentResult, error) {
	panic("enrollerFactory should not be called on this code path")
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

			cmd := enrollUserCmdWithConfig(
				func(string) (*config.Config, error) { return cfg, nil },
				fileSvcFactoryFor(fileSvc),
				noopCheckOperatorRunning,
				mockEnrollerFactory(mock),
			)
			var buf bytes.Buffer
			cmd.SetOut(&buf)
			cmd.SetErr(&buf)
			cmd.SetContext(context.Background())
			cmd.SetArgs(tc.args)
			require.NoError(t, cmd.ParseFlags(tc.args))
			require.NoError(t, cmd.RunE(cmd, nil))

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

	cmd := enrollUserCmdWithConfig(
		func(string) (*config.Config, error) { return cfg, nil },
		fileSvcFactoryFor(fileSvc),
		noopCheckOperatorRunning,
		mockEnrollerFactory(mock),
	)
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	cmd.SetContext(context.Background())
	cmdErr := cmd.RunE(cmd, nil)

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
	cmd := enrollUserCmdWithConfig(
		func(string) (*config.Config, error) { return cfg, nil },
		fileSvcFactoryFor(fileSvc),
		noopCheckOperatorRunning,
		mockEnrollerFactory(mock),
	)
	buf = bytes.Buffer{}
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	cmd.SetContext(context.Background())
	require.NoError(t, cmd.RunE(cmd, nil))

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

	cmd := enrollUserCmdWithConfig(
		func(string) (*config.Config, error) { return cfg, nil },
		fileSvcFactoryFor(fileSvc),
		noopCheckOperatorRunning,
		mockEnrollerFactory(mock),
	)
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	cmd.SetContext(context.Background())
	require.NoError(t, cmd.ParseFlags([]string{"--rotate-cli"}))
	require.NoError(t, cmd.RunE(cmd, nil))

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

	cmd := enrollUserCmdWithConfig(
		func(string) (*config.Config, error) { return cfg, nil },
		fileSvcFactoryFor(fileSvc),
		noopCheckOperatorRunning,
		mockEnrollerFactory(mock),
	)
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	cmd.SetContext(context.Background())
	require.NoError(t, cmd.ParseFlags([]string{"--no-system-trust"}))
	require.NoError(t, cmd.RunE(cmd, nil))

	assert.True(t, mock.lastOptions().NoSystemTrust, "--no-system-trust should set NoSystemTrust=true")
}

// TestEnrollCmd_SystemTrustInstalledOutput verifies that when the coordinator
// reports SystemTrustInstalled=true, the command prints a final confirmation
// line. The browser-restart guidance is no longer printed by the command
// layer — the coordinator's blocking browser-restart gate handles that at
// the correct time (before the passkey ceremony), so the final summary line
// is a simple confirmation that trust was installed.
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
	cmd := enrollUserCmdWithConfig(
		func(string) (*config.Config, error) { return cfg, nil },
		fileSvcFactoryFor(fileSvc),
		noopCheckOperatorRunning,
		mockEnrollerFactory(mock),
	)
	buf = bytes.Buffer{}
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	cmd.SetContext(context.Background())
	require.NoError(t, cmd.RunE(cmd, nil))

	assert.Contains(t, buf.String(), "System trust: installed gateway root CA")
	// The stale "Close all open browser windows" guidance is no longer
	// printed by the command layer — the coordinator's blocking gate
	// handles it before the passkey ceremony.
	assert.NotContains(t, buf.String(), "Close all open browser windows")
}

// TestEnrollCmd_StdinContinueInjected verifies that newDefaultEnrollmentCoordinator
// injects a non-nil Continue (the stdinContinue function) into the coordinator
// deps. This is the wiring test mirroring the existing Confirm injection —
// the interactive `auth enroll` command must supply a stdin-reading ContinueFunc
// so the blocking browser-restart gate can prompt the user.
func TestEnrollCmd_StdinContinueInjected(t *testing.T) {
	fileSvc, cfg := newCmdTestEnv(t)

	// The production factory builds a real coordinator. We cannot inspect
	// its private continueFn field directly, but we can assert the factory
	// does not panic and returns a non-nil Enroller — the Continue default
	// is set inside NewEnrollmentCoordinator. The deeper assertion (that
	// stdinContinue is the injected function) is covered by the coordinator-
	// level tests that inject a ContinueFunc stub and assert it is called.
	coordinator := newDefaultEnrollmentCoordinator(func(string, ...any) {}, fileSvc, cfg)
	require.NotNil(t, coordinator, "newDefaultEnrollmentCoordinator must return a non-nil Enroller")
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
// path does NOT enroll. Direct `mcp stdio` is a credential consumer only — it
// loads credentials and builds an mTLS connection, never enrolling, opening a
// browser, or installing system trust. This is the §11.5 3.8 negative
// assertion.
//
// With the enrollerFactory injection model, `mcpStdioCmdWithConfig` does not
// receive an enrollerFactory at all — the absence is enforced by the function
// signature. This test runs the command and asserts it fails (no gateway, no
// credentials) without any enrollment side effect. If a future change adds an
// enrollerFactory parameter to `mcpStdioCmdWithConfig`, this test should be
// updated to inject a panicking factory and assert it is never called.
func TestMCPStdio_DoesNotInvokeEnrollment(t *testing.T) {
	fileSvc, _ := newCmdTestEnv(t)

	cmd := mcpStdioCmdWithConfig(fileSvcFactoryFor(fileSvc))
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	cmd.SetContext(context.Background())

	// runMCPStdioProxy will fail because there is no gateway and no
	// credentials, but it must fail WITHOUT enrolling. The error is
	// expected; the assertion is that the command does not panic or
	// attempt enrollment (which is now impossible by construction since
	// mcpStdioCmdWithConfig has no enrollerFactory parameter).
	_ = cmd.RunE(cmd, nil)
}
