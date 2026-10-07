// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

//go:build integration

package client

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	operatorv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/operator/v1"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/models"
	"github.com/g8e-ai/g8e/v2/internal/tools/agent_harness/config"
)

func TestDiscoverRemoteOperator_SelectsTheStackDataOperator(t *testing.T) {
	operators := []*operatorv1.OperatorDocument{
		{Id: "op-embedded", OperatorSessionId: "sess-embedded", CurrentHostname: constants.DataOperatorHostname, Status: string(constants.OperatorStatusActive), OperatorType: string(constants.OperatorTypeEmbedded)},
		{Id: "op-offline", OperatorSessionId: "sess-offline", CurrentHostname: constants.DataOperatorHostname, Status: string(constants.OperatorStatusOffline), OperatorType: string(constants.OperatorTypeRemote)},
		{Id: "op-nosession", CurrentHostname: constants.DataOperatorHostname, Status: string(constants.OperatorStatusActive), OperatorType: string(constants.OperatorTypeRemote)},
		{Id: "op-elsewhere-1", OperatorSessionId: "sess-elsewhere-1", CurrentHostname: "elsewhere-1", Status: string(constants.OperatorStatusActive), OperatorType: string(constants.OperatorTypeRemote)},
		{Id: "op-elsewhere-2", OperatorSessionId: "sess-elsewhere-2", CurrentHostname: "elsewhere-2", Status: string(constants.OperatorStatusActive), OperatorType: string(constants.OperatorTypeRemote)},
		stackDataOperator("op-remote", "sess-remote"),
	}
	body, err := json.Marshal(models.OperatorSlotResponse{Success: true, Operators: operators})
	require.NoError(t, err)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodGet, r.Method)
		assert.Equal(t, constants.APIPaths.Operators, r.URL.Path)
		assert.Equal(t, "user-1", r.URL.Query().Get("user_id"))
		assert.Equal(t, "cli-1", r.Header.Get(constants.HeaderCLISessionID))
		w.Header().Set("Content-Type", "application/json")
		_, writeErr := w.Write(body)
		require.NoError(t, writeErr)
	}))
	t.Cleanup(server.Close)
	client, err := New(config.Config{MTLSBaseURL: server.URL, CLISessionID: "cli-1", UserID: "user-1"})
	require.NoError(t, err)
	exchanges := []Exchange{}
	client.Record(&exchanges)

	doc, raw, err := client.DiscoverRemoteOperator(context.Background())

	require.NoError(t, err)
	assert.Equal(t, "op-remote", doc.Id)
	assert.Equal(t, "sess-remote", doc.OperatorSessionId)
	assert.Equal(t, body, raw)
	require.Len(t, exchanges, 1)
}

func TestDiscoverRemoteOperator_SelectsHostNativeDataOperator(t *testing.T) {
	operators := []*operatorv1.OperatorDocument{
		{Id: "op-local", OperatorSessionId: "sess-local", CurrentHostname: "beepboop", Status: string(constants.OperatorStatusActive), OperatorType: string(constants.OperatorTypeRemote)},
	}
	body, err := json.Marshal(models.OperatorSlotResponse{Success: true, Operators: operators})
	require.NoError(t, err)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, writeErr := w.Write(body)
		require.NoError(t, writeErr)
	}))
	t.Cleanup(server.Close)
	client, err := New(config.Config{MTLSBaseURL: server.URL, CLISessionID: "cli-1", UserID: "user-1"})
	require.NoError(t, err)

	doc, raw, err := client.DiscoverRemoteOperator(context.Background())
	require.NoError(t, err)
	assert.Equal(t, "op-local", doc.Id)
	assert.Equal(t, "sess-local", doc.OperatorSessionId)
	assert.Equal(t, body, raw)
}

func TestDiscoverRemoteOperator_RejectsZeroOrAmbiguousMatches(t *testing.T) {
	tests := []struct {
		name      string
		operators []*operatorv1.OperatorDocument
		wantErr   error
	}{
		{
			name:      "no operators",
			operators: nil,
			wantErr:   constants.ErrEvaluationTargetUnavailable,
		},
		{
			name: "no active data-operator session",
			operators: []*operatorv1.OperatorDocument{
				{Id: "op-1", OperatorSessionId: "sess-1", CurrentHostname: constants.DataOperatorHostname, Status: string(constants.OperatorStatusStale), OperatorType: string(constants.OperatorTypeRemote)},
			},
			wantErr: constants.ErrEvaluationTargetUnavailable,
		},
		{
			name: "inference operators are not data operators",
			operators: []*operatorv1.OperatorDocument{
				{Id: "op-1", OperatorSessionId: "sess-1", CurrentHostname: "elsewhere", Status: string(constants.OperatorStatusActive), OperatorType: string(constants.OperatorTypeRemote), RuntimeConfig: &operatorv1.OperatorRuntimeConfig{InferenceEnabled: true}},
			},
			wantErr: constants.ErrEvaluationTargetUnavailable,
		},
		{
			name: "ambiguous active data-operator sessions",
			operators: []*operatorv1.OperatorDocument{
				stackDataOperator("op-1", "sess-1"),
				stackDataOperator("op-2", "sess-2"),
			},
			wantErr: constants.ErrEvaluationTargetAmbiguous,
		},
		{
			name: "ambiguous host-native data-operator sessions without stack",
			operators: []*operatorv1.OperatorDocument{
				{Id: "op-1", OperatorSessionId: "sess-1", CurrentHostname: "elsewhere-1", Status: string(constants.OperatorStatusActive), OperatorType: string(constants.OperatorTypeRemote)},
				{Id: "op-2", OperatorSessionId: "sess-2", CurrentHostname: "elsewhere-2", Status: string(constants.OperatorStatusActive), OperatorType: string(constants.OperatorTypeRemote)},
			},
			wantErr: constants.ErrEvaluationTargetAmbiguous,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body, err := json.Marshal(models.OperatorSlotResponse{Success: true, Operators: tt.operators})
			require.NoError(t, err)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write(body)
			}))
			t.Cleanup(server.Close)
			client, err := New(config.Config{MTLSBaseURL: server.URL, UserID: "user-1"})
			require.NoError(t, err)

			_, _, err = client.DiscoverRemoteOperator(context.Background())

			require.Error(t, err)
			assert.ErrorIs(t, err, tt.wantErr)
		})
	}
}

func TestDiscoverRemoteOperator_RejectsInvalidResponses(t *testing.T) {
	tests := []struct {
		name    string
		status  int
		body    string
		wantErr error
	}{
		{name: "gateway error status", status: http.StatusInternalServerError, body: `error`, wantErr: constants.ErrHTTPStatusError},
		{name: "malformed json", status: http.StatusOK, body: `not-json`, wantErr: constants.ErrInvalidJSONResponse},
		{name: "unsuccessful response", status: http.StatusOK, body: `{"success":false,"operators":[]}`, wantErr: constants.ErrInvalidJSONResponse},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tt.status)
				_, _ = w.Write([]byte(tt.body))
			}))
			t.Cleanup(server.Close)
			client, err := New(config.Config{MTLSBaseURL: server.URL, UserID: "user-1"})
			require.NoError(t, err)

			_, _, err = client.DiscoverRemoteOperator(context.Background())

			require.Error(t, err)
			assert.ErrorIs(t, err, tt.wantErr)
		})
	}
}

func TestDiscoverRemoteOperator_PropagatesTransportErrors(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	serverURL := server.URL
	server.Close()
	client, err := New(config.Config{MTLSBaseURL: serverURL, UserID: "user-1"})
	require.NoError(t, err)

	_, _, err = client.DiscoverRemoteOperator(context.Background())

	require.Error(t, err)
	assert.False(t, errors.Is(err, constants.ErrEvaluationTargetUnavailable))
}
