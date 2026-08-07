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
	"net/http"
	"testing"
	"time"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/internal/cli/auth"
	"github.com/g8e-ai/g8e/internal/cli/config"
	"github.com/g8e-ai/g8e/internal/cli/tui"
	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/g8e-ai/g8e/internal/services/fs"
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
		fileSvcFactory: newFileSvc,
		checkOperatorRunning: func(*config.Config) error {
			return nil
		},
		loadCredentials: func(fs.RuntimeFileService, *config.Config) (*auth.Credentials, error) {
			return &auth.Credentials{
				OperatorSessionID: "op-sess-test",
				UserID:            "user-test",
				OperatorID:        "operator-test",
				CLISessionID:      "cli-sess-test",
			}, nil
		},
		buildMTLSClient: func(fs.RuntimeFileService, *config.Config, time.Duration) (*http.Client, error) {
			return &http.Client{}, nil
		},
		tuiRun: func(ctx context.Context, opts tui.Options) error {
			return nil
		},
	}
}

// setupTUITestConfig creates a minimal config in a temp directory for hermetic tests.
func setupTUITestConfig(t *testing.T) *config.Config {
	t.Helper()
	_, cfg := newCmdTestEnv(t)
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
		cmd := tuiCmd()
		assert.Equal(t, "tui", cmd.Use)
		assert.Contains(t, cmd.Short, "Tactical Governance Console")
	})

	t.Run("command long description contains controls and enrollment hint", func(t *testing.T) {
		cmd := tuiCmd()
		assert.Contains(t, cmd.Long, "SSE")
		assert.Contains(t, cmd.Long, "g8e auth enroll")
		assert.Contains(t, cmd.Long, "Quit")
		assert.Contains(t, cmd.Long, "Scroll ledger down")
		assert.Contains(t, cmd.Long, "Scroll ledger up")
		assert.Contains(t, cmd.Long, "Jump to ledger bottom")
		assert.Contains(t, cmd.Long, "Jump to ledger top")
	})

	t.Run("command has a RunE function", func(t *testing.T) {
		cmd := tuiCmd()
		assert.NotNil(t, cmd.RunE)
	})
}

// --- config load failure ---

func TestTUI_ConfigLoadFailure(t *testing.T) {
	t.Run("returns error when config loader fails", func(t *testing.T) {
		deps := stubTUIDeps(t, nil)
		deps.configLoader = func(string) (*config.Config, error) {
			return nil, errors.New("config disk read failure")
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
		gwErr := errors.New("connection refused")
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

// --- credential loading ---

func TestTUI_CredentialLoadFailure(t *testing.T) {
	t.Run("returns wrapped ErrFailedToLoadCredentials when loadCredentials errors", func(t *testing.T) {
		cfg := setupTUITestConfig(t)
		deps := stubTUIDeps(t, cfg)
		credErr := errors.New("corrupt credentials file")
		deps.loadCredentials = func(fs.RuntimeFileService, *config.Config) (*auth.Credentials, error) {
			return nil, credErr
		}
		cmd := tuiCmdWithDeps(deps)
		cmd.SetArgs([]string{})
		err := cmd.Execute()
		require.Error(t, err)
		assert.ErrorIs(t, err, constants.ErrFailedToLoadCredentials)
		assert.ErrorIs(t, err, credErr)
	})
}

func TestTUI_NotEnrolled(t *testing.T) {
	t.Run("returns enrollment error when credentials are nil", func(t *testing.T) {
		cfg := setupTUITestConfig(t)
		deps := stubTUIDeps(t, cfg)
		deps.loadCredentials = func(fs.RuntimeFileService, *config.Config) (*auth.Credentials, error) {
			return nil, nil
		}
		cmd := tuiCmdWithDeps(deps)
		cmd.SetArgs([]string{})
		err := cmd.Execute()
		require.Error(t, err)
		assert.ErrorIs(t, err, constants.ErrNotEnrolled)
		assert.Contains(t, err.Error(), "g8e auth enroll")
	})
}

// --- mTLS client build failure ---

func TestTUI_BuildMTLSClientFailure(t *testing.T) {
	t.Run("returns error when BuildMTLSClient fails", func(t *testing.T) {
		cfg := setupTUITestConfig(t)
		deps := stubTUIDeps(t, cfg)
		tlsErr := errors.New("cert file missing")
		deps.buildMTLSClient = func(fs.RuntimeFileService, *config.Config, time.Duration) (*http.Client, error) {
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
	t.Run("passes correct version, node name, net label, and SSE URL to tui.Run", func(t *testing.T) {
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
		assert.Equal(t, "operator-test", capturedOpts.NodeName)
		assert.Equal(t, "mTLS", capturedOpts.NetLabel)
		assert.Contains(t, capturedOpts.SSEURL, constants.APIPaths.SSEStream)
		assert.NotContains(t, capturedOpts.SSEURL, "cli_session_id=", "routing IDs must not appear in SSE URL query string")
		assert.Equal(t, "cli-sess-test", capturedOpts.CLISessionID, "CLISessionID must be threaded into tui.Options for X-G8E-CLI-Session-ID header")
		assert.NotNil(t, capturedOpts.HTTPClient)
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

	t.Run("returns error from tui.Run", func(t *testing.T) {
		cfg := setupTUITestConfig(t)
		deps := stubTUIDeps(t, cfg)
		runErr := errors.New("tui crashed")
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

// --- SSE URL construction ---

func TestTUI_SSEURLConstruction(t *testing.T) {
	t.Run("SSE URL is built from OperatorHTTPURL and SSEStream path", func(t *testing.T) {
		cfg := setupTUITestConfig(t)
		deps := stubTUIDeps(t, cfg)

		var capturedSSEURL string
		deps.tuiRun = func(_ context.Context, opts tui.Options) error {
			capturedSSEURL = opts.SSEURL
			return nil
		}

		root := newRootCmdWithVersion("test")
		cmd := tuiCmdWithDeps(deps)
		root.AddCommand(cmd)
		root.SetArgs([]string{"tui"})
		require.NoError(t, root.Execute())

		expectedBase := cfg.OperatorHTTPURL()
		assert.Contains(t, capturedSSEURL, expectedBase)
		assert.Contains(t, capturedSSEURL, constants.APIPaths.SSEStream)
	})
}

// --- integration-style: real config, no gateway ---

func TestTUI_RealConfigNoGateway(t *testing.T) {
	t.Run("fails with gateway not reachable when using real config and no gateway", func(t *testing.T) {
		fileSvc, cfg := newCmdTestEnv(t)

		deps := tuiDeps{
			configLoader:         func(string) (*config.Config, error) { return cfg, nil },
			fileSvcFactory:       fileSvcFactoryFor(fileSvc),
			checkOperatorRunning: func(*config.Config) error { return constants.ErrGatewayNotReachable },
			loadCredentials:      auth.LoadCredentials,
			buildMTLSClient:      auth.BuildMTLSClient,
			tuiRun:               func(context.Context, tui.Options) error { return nil },
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
	t.Run("fails with not enrolled when credentials file does not exist", func(t *testing.T) {
		fileSvc, cfg := newCmdTestEnv(t)

		// Ensure no credentials file exists
		exists, err := fileSvc.FileExists(context.Background(), mustRel(t, fileSvc, cfg.CredentialsFile()))
		require.NoError(t, err)
		require.False(t, exists)

		deps := tuiDeps{
			configLoader:         func(string) (*config.Config, error) { return cfg, nil },
			fileSvcFactory:       fileSvcFactoryFor(fileSvc),
			checkOperatorRunning: func(*config.Config) error { return nil },
			loadCredentials:      auth.LoadCredentials,
			buildMTLSClient:      auth.BuildMTLSClient,
			tuiRun:               func(context.Context, tui.Options) error { return nil },
		}

		cmd := tuiCmdWithDeps(deps)
		cmd.SetArgs([]string{})
		err = cmd.Execute()
		require.Error(t, err)
		assert.ErrorIs(t, err, constants.ErrNotEnrolled)
	})
}

// --- real credentials file: corrupt JSON ---

func TestTUI_RealCredentialsCorruptJSON(t *testing.T) {
	t.Run("fails with ErrFailedToLoadCredentials when credentials file is corrupt", func(t *testing.T) {
		fileSvc, cfg := newCmdTestEnv(t)

		require.NoError(t, fileSvc.WriteFile(context.Background(), mustRel(t, fileSvc, cfg.CredentialsFile()), []byte("{invalid json"), constants.PermFilePrivate))

		deps := tuiDeps{
			configLoader:         func(string) (*config.Config, error) { return cfg, nil },
			fileSvcFactory:       fileSvcFactoryFor(fileSvc),
			checkOperatorRunning: func(*config.Config) error { return nil },
			loadCredentials:      auth.LoadCredentials,
			buildMTLSClient:      auth.BuildMTLSClient,
			tuiRun:               func(context.Context, tui.Options) error { return nil },
		}

		cmd := tuiCmdWithDeps(deps)
		cmd.SetArgs([]string{})
		err := cmd.Execute()
		require.Error(t, err)
		assert.ErrorIs(t, err, constants.ErrFailedToLoadCredentials)
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
