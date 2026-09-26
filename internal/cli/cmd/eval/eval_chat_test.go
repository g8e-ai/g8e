// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package eval

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/v2/internal/cli/auth"
	authcmd "github.com/g8e-ai/g8e/v2/internal/cli/cmd/auth"
	"github.com/g8e-ai/g8e/v2/internal/cli/cmd/cmdtest"
	"github.com/g8e-ai/g8e/v2/internal/cli/config"
	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/models"
	"github.com/g8e-ai/g8e/v2/internal/services/evaluation"
)

func TestChatEvalEnsureOperatorBinding_RefreshesStaleBinding(t *testing.T) {
	fileSvc, cfg := cmdtest.NewCmdTestEnv(t)
	require.NoError(t, auth.SaveCredentials(fileSvc, cfg, &auth.Credentials{
		UserID:            "user-1",
		CLISessionID:      "cli-stale",
		OperatorSessionID: "data-stale",
		OperatorID:        "op-stale",
	}))

	stub := &authcmd.StubRefreshClient{Result: auth.CLISessionRefresh{
		CLISessionID:      "cli-fresh",
		OperatorSessionID: "data-active",
		OperatorID:        "op-active",
		UserID:            "user-1",
	},
	}
	deps := chatEvalDeps{refreshClientFactory: authcmd.StubRefreshClientFactory(stub)}
	authContext := &auth.ClientAuthContext{
		UserID:            "user-1",
		CLISessionID:      "cli-stale",
		OperatorSessionID: "data-stale",
		OperatorID:        "op-stale",
		ClientCert:        cfg.CLICertFile(),
		ClientKey:         cfg.CLIKeyFile(),
	}
	operators := []models.OperatorDocumentGo{{
		ID:                "op-active",
		Status:            constants.OperatorStatusActive,
		OperatorType:      constants.OperatorTypeRemote,
		OperatorSessionID: "data-active",
	}}

	cmd := &cobra.Command{}
	updated, err := chatEvalEnsureOperatorBinding(cmd, deps, cfg, fileSvc, authContext, operators, "")
	require.NoError(t, err)
	assert.True(t, stub.Called())
	assert.Equal(t, "cli-fresh", updated.CLISessionID)
	assert.Equal(t, "data-active", updated.OperatorSessionID)
	assert.Equal(t, "op-active", updated.OperatorID)

	loaded, err := auth.LoadCredentials(fileSvc, cfg)
	require.NoError(t, err)
	require.NotNil(t, loaded)
	assert.Equal(t, "cli-fresh", loaded.CLISessionID)
	assert.Equal(t, "data-active", loaded.OperatorSessionID)
}

func TestChatEvalEnsureOperatorBinding_SkipsWhenBindingCurrent(t *testing.T) {
	stub := &authcmd.StubRefreshClient{}
	deps := chatEvalDeps{refreshClientFactory: authcmd.StubRefreshClientFactory(stub)}
	authContext := &auth.ClientAuthContext{
		UserID:            "user-1",
		CLISessionID:      "cli-current",
		OperatorSessionID: "data-active",
	}
	operators := []models.OperatorDocumentGo{{
		ID:                "op-active",
		Status:            constants.OperatorStatusActive,
		OperatorType:      constants.OperatorTypeRemote,
		OperatorSessionID: "data-active",
	}}

	cmd := &cobra.Command{}
	updated, err := chatEvalEnsureOperatorBinding(cmd, deps, &config.Config{}, nil, authContext, operators, "")
	require.NoError(t, err)
	assert.False(t, stub.Called())
	assert.Equal(t, authContext, updated)
}

func TestChatEvalResolveDataOperator_UsesPinnedSession(t *testing.T) {
	operators := []models.OperatorDocumentGo{
		{
			ID:                "op-data",
			Status:            constants.OperatorStatusActive,
			OperatorType:      constants.OperatorTypeRemote,
			OperatorSessionID: "data-active",
		},
		{
			ID:                "op-inference",
			Status:            constants.OperatorStatusActive,
			OperatorType:      constants.OperatorTypeRemote,
			OperatorSessionID: "infer-active",
			RuntimeConfig:     &models.RuntimeConfig{InferenceEnabled: true},
		},
	}
	selected, err := chatEvalResolveDataOperator(operators, &auth.ClientAuthContext{OperatorSessionID: "data-stale"}, "data-active")
	require.NoError(t, err)
	assert.Equal(t, "data-active", selected.OperatorSessionID)
}

func TestChatTraceFetchIsFatal(t *testing.T) {
	assert.True(t, chatTraceFetchIsFatal(fmt.Errorf("ensemble evaluation trace: status 401: auth required")))
	assert.False(t, chatTraceFetchIsFatal(fmt.Errorf("ensemble evaluation trace: status 404: missing")))
}

func TestFormatChatAcceptElapsed(t *testing.T) {
	assert.Equal(t, "12s", formatChatAcceptElapsed(12*time.Second))
	assert.Equal(t, "1m5s", formatChatAcceptElapsed(65*time.Second))
}

func TestChatEvalWaitForTrace_ReportsProgress(t *testing.T) {
	var calls int
	fetch := func(context.Context) (evaluation.EvaluationTrace, error) {
		calls++
		switch calls {
		case 1:
			return nil, assert.AnError
		case 2:
			return evaluation.EvaluationTrace{
				"status":      "running",
				"model_calls": []any{map[string]any{"agent_role": "primary"}},
			}, nil
		default:
			return evaluation.EvaluationTrace{"status": "completed"}, nil
		}
	}

	var output bytes.Buffer
	reporter := newChatAcceptReporter(&output, false)
	trace, err := chatEvalWaitForTraceWithPoll(context.Background(), fetch, reporter, time.Millisecond)
	require.NoError(t, err)
	require.Equal(t, "completed", trace["status"])
	require.GreaterOrEqual(t, calls, 3)

	text := output.String()
	assert.Contains(t, text, "trace not yet available")
	assert.Contains(t, text, "trace status=running")
	assert.Contains(t, text, "model_calls=1")
	assert.Contains(t, text, "trace completed after")
}

func TestChatEvalWaitForTrace_QuietJSONMode(t *testing.T) {
	var calls int
	fetch := func(context.Context) (evaluation.EvaluationTrace, error) {
		calls++
		if calls == 1 {
			return evaluation.EvaluationTrace{"status": "completed"}, nil
		}
		return nil, assert.AnError
	}

	var output bytes.Buffer
	reporter := newChatAcceptReporter(&output, true)
	trace, err := chatEvalWaitForTrace(context.Background(), fetch, reporter)
	require.NoError(t, err)
	require.Equal(t, "completed", trace["status"])
	assert.Equal(t, "", strings.TrimSpace(output.String()))
}

func TestChatEvalEnsureOperatorBinding_RefreshFailure(t *testing.T) {
	deps := chatEvalDeps{
		refreshClientFactory: authcmd.StubRefreshClientFactory(&authcmd.StubRefreshClient{Err: context.Canceled}),
	}
	authContext := &auth.ClientAuthContext{OperatorSessionID: "data-stale"}
	operators := []models.OperatorDocumentGo{{
		ID:                "op-active",
		Status:            constants.OperatorStatusActive,
		OperatorType:      constants.OperatorTypeRemote,
		OperatorSessionID: "data-active",
	}}

	_, err := chatEvalEnsureOperatorBinding(&cobra.Command{}, deps, &config.Config{}, nil, authContext, operators, "data-active")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "auth refresh")
}
