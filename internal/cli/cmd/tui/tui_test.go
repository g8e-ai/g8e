// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package tuicmd

import (
	"bytes"
	"context"
	"fmt"
	"testing"

	"github.com/g8e-ai/g8e/v2/internal/cli/cmd/shared"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/v2/internal/cli/auth"
	"github.com/g8e-ai/g8e/v2/internal/cli/cmd/cmdtest"
	"github.com/g8e-ai/g8e/v2/internal/cli/config"
	"github.com/g8e-ai/g8e/v2/internal/cli/sse"
	"github.com/g8e-ai/g8e/v2/internal/cli/tui"
	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/services/fs"
)

// --- helpers ---

// stubTUIDeps returns a tuiDeps where every dependency succeeds by default.
// Individual tests override specific fields to simulate failure scenarios.
func stubTUIDeps(t *testing.T, cfg *config.Config) tuiDeps {
	t.Helper()
	return tuiDeps{
		configLoader: func(string) (*config.Config, error) {
			return cfg, nil
		},
		fileSvcFactory: shared.NewFileSvc,
		checkOperatorRunning: func(*config.Config) error {
			return nil
		},
		inspectDockerGateway: func(context.Context) (dockerContainerState, error) {
			return dockerContainerState{}, constants.ErrNotFound
		},
		loadAuthContext: func(fs.RuntimeFileService, *config.Config) (*auth.ClientAuthContext, error) {
			return &auth.ClientAuthContext{
				OperatorSessionID: "op-sess-test",
				UserID:            "user-test",
				OperatorID:        "operator-test",
				CLISessionID:      "cli-sess-test",
			}, nil
		},
		newSession: func(fs.RuntimeFileService, *config.Config) (tui.Session, error) {
			return &stubSession{}, nil
		},
		openBrowser: func(string) error { return nil },
		tuiRun: func(ctx context.Context, opts tui.Options) error {
			return nil
		},
	}
}

// stubSession is a tui.Session that never reaches a gateway.
type stubSession struct{}

func (*stubSession) NewSSEClient() *sse.Client { return sse.NewClient("", nil) }

func (*stubSession) DoRequestContext(context.Context, string, string, interface{}) ([]byte, error) {
	return []byte(`{"transactions":[]}`), nil
}

// setupTUITestConfig creates a minimal config in a temp directory for hermetic tests.
func setupTUITestConfig(t *testing.T) *config.Config {
	t.Helper()
	_, cfg := cmdtest.NewCmdTestEnv(t)
	return cfg
}

// newRootCmdWithVersion creates a root cobra command with the given version,
// so runTUI can read cmd.Root().Version.
func newRootCmdWithVersion(version string) *cobra.Command {
	root := &cobra.Command{Use: "g8e", Version: version}
	return root
}

// --- command structure tests ---

func TestTUICmdStructure(t *testing.T) {
	t.Run("command has correct use and short description", func(t *testing.T) {
		cmd := Cmd()
		assert.Equal(t, "tui", cmd.Use)
		assert.Contains(t, cmd.Short, "Tactical Governance Console")
	})

	t.Run("command long description contains controls and enrollment hint", func(t *testing.T) {
		cmd := Cmd()
		assert.Contains(t, cmd.Long, "SSE")
		assert.Contains(t, cmd.Long, "g8e auth enroll user")
		assert.Contains(t, cmd.Long, "g8e auth refresh")
		assert.Contains(t, cmd.Long, "Quit")
		assert.Contains(t, cmd.Long, "Focus the next / previous pane")
		assert.Contains(t, cmd.Long, "Move in the focused pane")
		assert.Contains(t, cmd.Long, "Jump to ledger bottom")
		assert.Contains(t, cmd.Long, "Approve the selected pending transaction")
		assert.Contains(t, cmd.Long, "Refresh approvals, operators, and posture")
	})

	t.Run("command has a RunE function", func(t *testing.T) {
		cmd := Cmd()
		assert.NotNil(t, cmd.RunE)
	})
}

// --- config load failure ---

func TestTUI_ConfigLoadFailure(t *testing.T) {
	t.Run("returns error when config loader fails", func(t *testing.T) {
		deps := stubTUIDeps(t, nil)
		deps.configLoader = func(string) (*config.Config, error) {
			return nil, fmt.Errorf("config disk read failure")
		}
		cmd := tuiCmdWithDeps(deps)
		cmd.SetArgs([]string{})
		err := cmd.Execute()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "config disk read failure")
	})
}

// --- gateway not reachable ---

func TestTUI_GatewayNotReachable(t *testing.T) {
	t.Run("returns wrapped error when gateway is not reachable", func(t *testing.T) {
		cfg := setupTUITestConfig(t)
		deps := stubTUIDeps(t, cfg)
		gwErr := fmt.Errorf("connection refused")
		deps.checkOperatorRunning = func(*config.Config) error {
			return gwErr
		}
		cmd := tuiCmdWithDeps(deps)
		cmd.SetArgs([]string{})
		err := cmd.Execute()
		require.Error(t, err)
		assert.ErrorIs(t, err, constants.ErrGatewayNotReachable)
		assert.ErrorIs(t, err, gwErr)
	})
}

func TestTUI_GatewayNotReachableReportsDockerState(t *testing.T) {
	testCases := []struct {
		name           string
		state          dockerContainerState
		inspectErr     error
		expectedDetail string
	}{
		{name: "exited unhealthy gateway", state: dockerContainerState{Status: dockerContainerStatusExited, Health: dockerContainerHealthUnhealthy}, expectedDetail: "Docker gateway is exited (health: unhealthy)"},
		{name: "gateway healthcheck starting", state: dockerContainerState{Status: dockerContainerStatusRunning, Health: dockerContainerHealthStarting}, expectedDetail: "Docker gateway is running and its healthcheck is starting"},
		{name: "running unhealthy gateway", state: dockerContainerState{Status: dockerContainerStatusRunning, Health: dockerContainerHealthUnhealthy}, expectedDetail: "Docker gateway is running but unhealthy"},
		{name: "healthy gateway with unreachable configured endpoint", state: dockerContainerState{Status: dockerContainerStatusRunning, Health: dockerContainerHealthHealthy}, expectedDetail: "Docker gateway is healthy, but the configured endpoint"},
		{name: "gateway container not found", inspectErr: constants.ErrNotFound, expectedDetail: "no running gateway was detected"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := setupTUITestConfig(t)
			deps := stubTUIDeps(t, cfg)
			gwErr := fmt.Errorf("connection refused")
			deps.checkOperatorRunning = func(*config.Config) error { return gwErr }
			deps.inspectDockerGateway = func(context.Context) (dockerContainerState, error) {
				return tc.state, tc.inspectErr
			}
			cmd := tuiCmdWithDeps(deps)
			cmd.SetArgs([]string{})

			err := cmd.Execute()

			require.Error(t, err)
			assert.ErrorIs(t, err, constants.ErrGatewayNotReachable)
			assert.ErrorIs(t, err, gwErr)
			assert.Contains(t, err.Error(), tc.expectedDetail, cmdtest.RegressionMarkerAfterFix)
		})
	}
}

func TestTUICmdSuppressesUsageForRuntimeFailures(t *testing.T) {
	cmd := Cmd()
	assert.True(t, cmd.SilenceErrors, cmdtest.RegressionMarkerAfterFix)
	assert.True(t, cmd.SilenceUsage, cmdtest.RegressionMarkerAfterFix)
}

// --- CLI identity loading ---

func TestTUI_AuthContextLoadFailure(t *testing.T) {
	t.Run("returns the CLI's identity error unchanged in the chain", func(t *testing.T) {
		cfg := setupTUITestConfig(t)
		deps := stubTUIDeps(t, cfg)
		credErr := fmt.Errorf("%w: corrupt credentials file", constants.ErrFailedToLoadCredentials)
		deps.loadAuthContext = func(fs.RuntimeFileService, *config.Config) (*auth.ClientAuthContext, error) {
			return nil, credErr
		}
		deps.newSession = func(fs.RuntimeFileService, *config.Config) (tui.Session, error) {
			panic("newSession should not be called when the identity fails to load")
		}
		cmd := tuiCmdWithDeps(deps)
		cmd.SetArgs([]string{})
		err := cmd.Execute()
		require.Error(t, err)
		assert.ErrorIs(t, err, constants.ErrFailedToLoadCredentials)
		assert.ErrorIs(t, err, credErr)
	})
}

// --- CLI session open failure ---

func TestTUI_NewSessionFailure(t *testing.T) {
	t.Run("returns error when the CLI API session cannot be opened", func(t *testing.T) {
		cfg := setupTUITestConfig(t)
		deps := stubTUIDeps(t, cfg)
		tlsErr := fmt.Errorf("cert file missing")
		deps.newSession = func(fs.RuntimeFileService, *config.Config) (tui.Session, error) {
			return nil, tlsErr
		}
		cmd := tuiCmdWithDeps(deps)
		cmd.SetArgs([]string{})
		err := cmd.Execute()
		require.Error(t, err)
		assert.ErrorIs(t, err, tlsErr)
	})
}

// --- tui.Run invocation ---

func TestTUI_TUIRunCalledWithCorrectOptions(t *testing.T) {
	t.Run("passes version, CLI identity, CLI session, and the approve flow to tui.Run", func(t *testing.T) {
		cfg := setupTUITestConfig(t)
		deps := stubTUIDeps(t, cfg)

		var capturedOpts tui.Options
		deps.tuiRun = func(ctx context.Context, opts tui.Options) error {
			capturedOpts = opts
			return nil
		}

		root := newRootCmdWithVersion("v9.9.9")
		cmd := tuiCmdWithDeps(deps)
		root.AddCommand(cmd)
		root.SetArgs([]string{"tui"})
		err := root.Execute()
		require.NoError(t, err)

		assert.Equal(t, "v9.9.9", capturedOpts.Version)
		assert.Equal(t, tui.Identity{UserID: "user-test", CLISessionID: "cli-sess-test", OperatorID: "operator-test"}, capturedOpts.Identity)
		assert.IsType(t, &stubSession{}, capturedOpts.Session, "the CLI API session must be threaded into tui.Options")
		require.NotNil(t, capturedOpts.ApprovalURL)
		assert.Equal(t, auth.ApprovalPageURL(cfg, "tx-1"), capturedOpts.ApprovalURL("tx-1"), "the TUI must open the same page as 'g8e auth approve'")
		assert.NotNil(t, capturedOpts.OpenBrowser)
	})

	t.Run("defaults version to dev when root version is empty", func(t *testing.T) {
		cfg := setupTUITestConfig(t)
		deps := stubTUIDeps(t, cfg)

		var capturedOpts tui.Options
		deps.tuiRun = func(ctx context.Context, opts tui.Options) error {
			capturedOpts = opts
			return nil
		}

		root := newRootCmdWithVersion("")
		cmd := tuiCmdWithDeps(deps)
		root.AddCommand(cmd)
		root.SetArgs([]string{"tui"})
		err := root.Execute()
		require.NoError(t, err)

		assert.Equal(t, "dev", capturedOpts.Version)
	})

	t.Run("derives the run context from the command context", func(t *testing.T) {
		cfg := setupTUITestConfig(t)
		deps := stubTUIDeps(t, cfg)

		parent, cancel := context.WithCancel(context.Background())
		var runCtx context.Context
		deps.tuiRun = func(ctx context.Context, _ tui.Options) error {
			runCtx = ctx
			return nil
		}
		cmd := tuiCmdWithDeps(deps)
		cmd.SetArgs([]string{})
		require.NoError(t, cmd.ExecuteContext(parent))

		require.NotNil(t, runCtx)
		require.NoError(t, runCtx.Err())
		cancel()
		assert.ErrorIs(t, runCtx.Err(), context.Canceled, "cancelling the command context must cancel the TUI")
	})

	t.Run("returns error from tui.Run", func(t *testing.T) {
		cfg := setupTUITestConfig(t)
		deps := stubTUIDeps(t, cfg)
		runErr := fmt.Errorf("tui crashed")
		deps.tuiRun = func(context.Context, tui.Options) error {
			return runErr
		}
		cmd := tuiCmdWithDeps(deps)
		cmd.SetArgs([]string{})
		err := cmd.Execute()
		require.Error(t, err)
		assert.ErrorIs(t, err, runErr)
	})
}

// --- CLI session wiring ---

func TestTUI_DefaultSessionIsCLIAPIClient(t *testing.T) {
	t.Run("default session factory is the shared CLI API client", func(t *testing.T) {
		fileSvc, cfg := cmdtest.NewCmdTestEnv(t)
		_, err := defaultTUIDeps().newSession(fileSvc, cfg)
		require.Error(t, err, "api.NewClient must reject an unenrolled environment")
		assert.ErrorIs(t, err, constants.ErrNotAuthenticated)
	})
}

// --- integration-style: real config, no gateway ---

func TestTUI_RealConfigNoGateway(t *testing.T) {
	t.Run("fails with gateway not reachable when using real config and no gateway", func(t *testing.T) {
		fileSvc, cfg := cmdtest.NewCmdTestEnv(t)

		deps := tuiDeps{
			configLoader:         func(string) (*config.Config, error) { return cfg, nil },
			fileSvcFactory:       cmdtest.FileSvcFactoryFor(fileSvc),
			checkOperatorRunning: func(*config.Config) error { return constants.ErrGatewayNotReachable },
			inspectDockerGateway: func(context.Context) (dockerContainerState, error) {
				return dockerContainerState{}, constants.ErrNotFound
			},
			loadAuthContext: auth.LoadClientAuthContext,
			newSession:      newAPISession,
			tuiRun:          func(context.Context, tui.Options) error { return nil },
		}

		cmd := tuiCmdWithDeps(deps)
		cmd.SetArgs([]string{})
		err := cmd.Execute()
		require.Error(t, err)
		assert.ErrorIs(t, err, constants.ErrGatewayNotReachable)
	})
}

// --- real credentials file: not enrolled ---

func TestTUI_RealCredentialsNotEnrolled(t *testing.T) {
	t.Run("fails with not authenticated when credentials file does not exist", func(t *testing.T) {
		fileSvc, cfg := cmdtest.NewCmdTestEnv(t)

		// Ensure no credentials file exists
		exists, err := fileSvc.FileExists(context.Background(), cmdtest.MustRel(t, fileSvc, cfg.CredentialsFile()))
		require.NoError(t, err)
		require.False(t, exists)

		deps := tuiDeps{
			configLoader:         func(string) (*config.Config, error) { return cfg, nil },
			fileSvcFactory:       cmdtest.FileSvcFactoryFor(fileSvc),
			checkOperatorRunning: func(*config.Config) error { return nil },
			loadAuthContext:      auth.LoadClientAuthContext,
			newSession:           newAPISession,
			tuiRun:               func(context.Context, tui.Options) error { return nil },
		}

		cmd := tuiCmdWithDeps(deps)
		cmd.SetArgs([]string{})
		err = cmd.Execute()
		require.Error(t, err)
		assert.ErrorIs(t, err, constants.ErrNotAuthenticated)
		assert.Contains(t, err.Error(), "g8e auth enroll user")
	})
}

// --- real credentials file: corrupt JSON ---

func TestTUI_RealCredentialsCorruptJSON(t *testing.T) {
	t.Run("fails with ErrInvalidJSONBody when credentials file is corrupt", func(t *testing.T) {
		fileSvc, cfg := cmdtest.NewCmdTestEnv(t)

		require.NoError(t, fileSvc.WriteFile(context.Background(), cmdtest.MustRel(t, fileSvc, cfg.CredentialsFile()), []byte("{invalid json"), constants.PermFilePrivate))

		deps := tuiDeps{
			configLoader:         func(string) (*config.Config, error) { return cfg, nil },
			fileSvcFactory:       cmdtest.FileSvcFactoryFor(fileSvc),
			checkOperatorRunning: func(*config.Config) error { return nil },
			loadAuthContext:      auth.LoadClientAuthContext,
			newSession:           newAPISession,
			tuiRun:               func(context.Context, tui.Options) error { return nil },
		}

		cmd := tuiCmdWithDeps(deps)
		cmd.SetArgs([]string{})
		err := cmd.Execute()
		require.Error(t, err)
		assert.ErrorIs(t, err, constants.ErrInvalidJSONBody)
	})
}

// --- output suppression ---

func TestTUI_NoStdoutOnSuccess(t *testing.T) {
	t.Run("produces no stdout output on successful tui.Run", func(t *testing.T) {
		cfg := setupTUITestConfig(t)
		deps := stubTUIDeps(t, cfg)
		deps.tuiRun = func(context.Context, tui.Options) error {
			return nil
		}

		cmd := tuiCmdWithDeps(deps)
		var buf bytes.Buffer
		cmd.SetOut(&buf)
		cmd.SetErr(&buf)
		cmd.SetArgs([]string{})
		err := cmd.Execute()
		require.NoError(t, err)
		assert.Empty(t, buf.String())
	})
}
