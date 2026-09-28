// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package eval

import (
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/models"
)

func remoteOperator(id, session string, cfg *models.RuntimeConfig) models.OperatorDocumentGo {
	return models.OperatorDocumentGo{
		ID:                id,
		OperatorSessionID: session,
		Status:            constants.OperatorStatusActive,
		OperatorType:      constants.OperatorTypeRemote,
		RuntimeConfig:     cfg,
	}
}

func inferenceOperatorFixture(id, session string) models.OperatorDocumentGo {
	return remoteOperator(id, session, &models.RuntimeConfig{InferenceEnabled: true, InferenceOllamaEndpoint: "http://provider.example:11434"})
}

func dataOperatorFixture(id, session string) models.OperatorDocumentGo {
	return remoteOperator(id, session, &models.RuntimeConfig{})
}

func TestResolveOperatorSessionsFrom(t *testing.T) {
	single := []models.OperatorDocumentGo{inferenceOperatorFixture("i1", "infer-1"), dataOperatorFixture("d1", "data-1")}
	many := []models.OperatorDocumentGo{
		inferenceOperatorFixture("i1", "infer-1"), inferenceOperatorFixture("i2", "infer-2"),
		dataOperatorFixture("d1", "data-1"), dataOperatorFixture("d2", "data-2"),
	}
	observer := remoteOperator("obs", "obs-1", &models.RuntimeConfig{ProviderBoundaryObserverEnabled: true})
	inactive := inferenceOperatorFixture("i1", "infer-1")
	inactive.Status = constants.OperatorStatusOffline

	tests := []struct {
		name      string
		operators []models.OperatorDocumentGo
		inference string
		data      string
		roles     []operatorRole
		want      operatorSessions
		wantErr   error
		wantMsg   []string
	}{
		{
			name: "one session per role", operators: single,
			roles: []operatorRole{operatorRoleInference, operatorRoleData},
			want:  operatorSessions{InferenceSessionID: "infer-1", DataSessionID: "data-1", DataOperatorID: "d1"},
		},
		{
			name: "only requested roles are resolved", operators: []models.OperatorDocumentGo{inferenceOperatorFixture("i1", "infer-1")},
			roles: []operatorRole{operatorRoleInference},
			want:  operatorSessions{InferenceSessionID: "infer-1"},
		},
		{
			name: "pin selects among many", operators: many, inference: "infer-2", data: "data-2",
			roles: []operatorRole{operatorRoleInference, operatorRoleData},
			want:  operatorSessions{InferenceSessionID: "infer-2", DataSessionID: "data-2", DataOperatorID: "d2"},
		},
		{
			name: "several inference sessions list candidates", operators: many,
			roles:   []operatorRole{operatorRoleInference},
			wantErr: constants.ErrOperatorSessionAmbiguous,
			wantMsg: []string{"infer-1", "infer-2", "--inference-session"},
		},
		{
			name: "several data sessions list candidates", operators: many,
			roles:   []operatorRole{operatorRoleData},
			wantErr: constants.ErrOperatorSessionAmbiguous,
			wantMsg: []string{"data-1", "data-2", "--data-session"},
		},
		{
			name: "no inference session", operators: []models.OperatorDocumentGo{dataOperatorFixture("d1", "data-1")},
			roles:   []operatorRole{operatorRoleInference},
			wantErr: constants.ErrOperatorSessionNotFound,
			wantMsg: []string{"no active inference operator session"},
		},
		{
			name: "no data session", operators: []models.OperatorDocumentGo{inferenceOperatorFixture("i1", "infer-1")},
			roles:   []operatorRole{operatorRoleData},
			wantErr: constants.ErrOperatorSessionNotFound,
			wantMsg: []string{"no active data operator session"},
		},
		{
			name: "pinned session not active", operators: single, data: "gone",
			roles:   []operatorRole{operatorRoleData},
			wantErr: constants.ErrOperatorSessionNotFound,
			wantMsg: []string{`"gone"`},
		},
		{
			name: "observers are not data sessions", operators: []models.OperatorDocumentGo{observer},
			roles:   []operatorRole{operatorRoleData},
			wantErr: constants.ErrOperatorSessionNotFound,
		},
		{
			name: "inactive operators are ignored", operators: []models.OperatorDocumentGo{inactive},
			roles:   []operatorRole{operatorRoleInference},
			wantErr: constants.ErrOperatorSessionNotFound,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := resolveOperatorSessionsFrom(test.operators, test.inference, test.data, test.roles...)
			if test.wantErr != nil {
				require.ErrorIs(t, err, test.wantErr)
				for _, fragment := range test.wantMsg {
					assert.ErrorContains(t, err, fragment)
				}
				return
			}
			require.NoError(t, err)
			assert.Equal(t, test.want, got)
		})
	}
}

func TestResolveOperatorSessionsFromRejectsUnknownRole(t *testing.T) {
	_, err := resolveOperatorSessionsFrom(nil, "", "", operatorRole("bogus"))
	require.ErrorContains(t, err, "unknown operator role")
}

func TestSessionFlagsArePersistentOnEvalGroup(t *testing.T) {
	group := evalCmdWithConfig(nativeEvalDeps{})
	assert.NotNil(t, group.PersistentFlags().Lookup(flagInferenceSession))
	assert.NotNil(t, group.PersistentFlags().Lookup(flagDataSession))

	child := &cobra.Command{Use: "child", RunE: func(cmd *cobra.Command, _ []string) error {
		inference, data, err := sessionPinsFromFlags(cmd)
		require.NoError(t, err)
		assert.Equal(t, "infer-9", inference)
		assert.Equal(t, "data-9", data)
		return nil
	}}
	parent := &cobra.Command{Use: "parent"}
	bindSessionFlags(parent)
	parent.AddCommand(child)
	parent.SetArgs([]string{"child", "--inference-session", " infer-9 ", "--data-session", "data-9"})
	require.NoError(t, parent.Execute())
}
