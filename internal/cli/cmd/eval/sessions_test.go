// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package eval

import (
	"testing"

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

// dataOperatorFixture is the stack's data-operator: a data Operator whose
// heartbeat hostname is constants.DataOperatorHostname.
func dataOperatorFixture(id, session string) models.OperatorDocumentGo {
	op := remoteOperator(id, session, &models.RuntimeConfig{})
	op.CurrentHostname = constants.DataOperatorHostname
	return op
}

// otherDataOperatorFixture is a data Operator enrolled from elsewhere.
func otherDataOperatorFixture(id, session string) models.OperatorDocumentGo {
	op := remoteOperator(id, session, &models.RuntimeConfig{})
	op.CurrentHostname = "other-host"
	return op
}

func TestResolveOperatorSessionsFrom(t *testing.T) {
	single := []models.OperatorDocumentGo{inferenceOperatorFixture("i1", "infer-1"), dataOperatorFixture("d1", "data-1")}
	withOthers := []models.OperatorDocumentGo{
		inferenceOperatorFixture("i1", "infer-1"),
		otherDataOperatorFixture("x1", "other-1"), dataOperatorFixture("d1", "data-1"), otherDataOperatorFixture("x2", "other-2"),
	}
	manyInference := []models.OperatorDocumentGo{inferenceOperatorFixture("i1", "infer-1"), inferenceOperatorFixture("i2", "infer-2")}
	observer := remoteOperator("obs", "obs-1", &models.RuntimeConfig{ProviderBoundaryObserverEnabled: true})
	observer.CurrentHostname = constants.DataOperatorHostname
	inactive := inferenceOperatorFixture("i1", "infer-1")
	inactive.Status = constants.OperatorStatusOffline

	tests := []struct {
		name      string
		operators []models.OperatorDocumentGo
		roles     []operatorRole
		want      operatorSessions
		wantErr   error
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
			name: "other data operators are irrelevant", operators: withOthers,
			roles: []operatorRole{operatorRoleInference, operatorRoleData},
			want:  operatorSessions{InferenceSessionID: "infer-1", DataSessionID: "data-1", DataOperatorID: "d1"},
		},
		{
			name: "several inference sessions are ambiguous", operators: manyInference,
			roles:   []operatorRole{operatorRoleInference},
			wantErr: constants.ErrInferenceOperatorAmbiguous,
		},
		{
			name: "no inference session", operators: []models.OperatorDocumentGo{dataOperatorFixture("d1", "data-1")},
			roles:   []operatorRole{operatorRoleInference},
			wantErr: constants.ErrInferenceOperatorNotFound,
		},
		{
			name: "no data-operator among other data operators", operators: []models.OperatorDocumentGo{otherDataOperatorFixture("x1", "other-1")},
			roles:   []operatorRole{operatorRoleData},
			wantErr: constants.ErrDataOperatorNotFound,
		},
		{
			name: "observers are not data-operators", operators: []models.OperatorDocumentGo{observer},
			roles:   []operatorRole{operatorRoleData},
			wantErr: constants.ErrDataOperatorNotFound,
		},
		{
			name: "inactive operators are ignored", operators: []models.OperatorDocumentGo{inactive},
			roles:   []operatorRole{operatorRoleInference},
			wantErr: constants.ErrInferenceOperatorNotFound,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := resolveOperatorSessionsFrom(test.operators, test.roles...)
			if test.wantErr != nil {
				require.ErrorIs(t, err, test.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, test.want, got)
		})
	}
}

func TestResolveOperatorSessionsFromRejectsUnknownRole(t *testing.T) {
	_, err := resolveOperatorSessionsFrom(nil, operatorRole("bogus"))
	require.ErrorContains(t, err, "unknown operator role")
}

func TestEvalGroupHasNoSessionFlags(t *testing.T) {
	group := evalCmdWithConfig(nativeEvalDeps{})
	assert.Nil(t, group.PersistentFlags().Lookup("data-session"))
	assert.Nil(t, group.PersistentFlags().Lookup("inference-session"))
}
