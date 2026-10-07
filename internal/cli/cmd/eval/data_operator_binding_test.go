// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package eval

import (
	"fmt"
	"log/slog"
	"testing"

	operatorv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/operator/v1"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/v2/internal/cli/auth"
	"github.com/g8e-ai/g8e/v2/internal/cli/cmd/cmdtest"
	"github.com/g8e-ai/g8e/v2/internal/cli/config"
	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/services/fs"
	"github.com/g8e-ai/g8e/v2/internal/testutil"
)

func activeOperatorSessionDoc(id, sessionID string) *operatorv1.OperatorDocument {
	return &operatorv1.OperatorDocument{
		Id:                id,
		OperatorSessionId: sessionID,
		Status:            string(constants.OperatorStatusActive),
		OperatorType:      string(constants.OperatorTypeRemote),
	}
}

func TestChatEvalBindDataOperator(t *testing.T) {
	const dataSession = "data-session"
	errGateway := fmt.Errorf("gateway unavailable")
	operators := []*operatorv1.OperatorDocument{
		activeOperatorSessionDoc("primary", "primary-session"),
		activeOperatorSessionDoc("other", "other-session"),
		activeOperatorSessionDoc("data", dataSession),
	}

	tests := []struct {
		name        string
		bound       []string
		autoBind    bool
		infoErr     error
		bindErr     error
		wantBinds   [][]string
		wantSession string
		wantErr     error
	}{
		{
			name:        "data-operator already bound as a non-primary session leaves the CLI session untouched",
			bound:       []string{"primary-session", dataSession},
			autoBind:    true,
			wantSession: "cli-1",
		},
		{
			name:        "unbound data-operator is added to the active bound sessions in one call",
			bound:       []string{"primary-session", "other-session"},
			autoBind:    true,
			wantBinds:   [][]string{{"primary-session", "other-session", dataSession}},
			wantSession: "cli-bound",
		},
		{
			name:        "bound sessions that are no longer active are dropped",
			bound:       []string{"gone-session", "primary-session"},
			autoBind:    true,
			wantBinds:   [][]string{{"primary-session", dataSession}},
			wantSession: "cli-bound",
		},
		{
			name:        "a CLI session with no bound operators binds only the data-operator",
			autoBind:    true,
			wantBinds:   [][]string{{dataSession}},
			wantSession: "cli-bound",
		},
		{
			name:     "an unbound data-operator is reported when auto-bind is off",
			bound:    []string{"primary-session"},
			autoBind: false,
			wantErr:  constants.ErrDataOperatorNotBound,
		},
		{
			name:     "a failed read of the CLI session bindings is returned",
			autoBind: true,
			infoErr:  errGateway,
			wantErr:  errGateway,
		},
		{
			name:     "a failed bind is returned",
			bound:    []string{"primary-session"},
			autoBind: true,
			bindErr:  errGateway,
			wantErr:  errGateway,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := testutil.TempDir(t)
			fileSvc, err := fs.NewRuntimeFileService(root, slog.Default())
			require.NoError(t, err)
			paths := config.DefaultPathsConfig()
			cfg := &config.Config{ProjectRoot: root, RuntimeDir: fileSvc.Resolve(""), Paths: &paths}
			require.NoError(t, auth.SaveCredentials(fileSvc, cfg, &auth.Credentials{CLISessionID: "cli-1", UserID: "user-1"}))
			client := &fakeBindClient{bound: tt.bound, infoErr: tt.infoErr, bindErr: tt.bindErr}
			deps := chatEvalDeps{bindClientFactory: func(*config.Config) chatEvalBindClient { return client }}
			cmd := cmdtest.SilentCobraCommand()
			cmd.SetContext(testVersionContext())
			authContext := &auth.ClientAuthContext{CLISessionID: "cli-1", UserID: "user-1", ClientCert: "cert", ClientKey: "key"}

			got, err := chatEvalBindDataOperator(cmd, deps, cfg, fileSvc, authContext, operators, dataSession, tt.autoBind)

			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)
				assert.Empty(t, client.binds)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.wantBinds, client.binds)
			assert.Equal(t, tt.wantSession, got.CLISessionID)
			assert.Equal(t, "cert", got.ClientCert)
			stored, err := auth.LoadCredentials(fileSvc, cfg)
			require.NoError(t, err)
			assert.Equal(t, tt.wantSession, stored.CLISessionID)
		})
	}
}
