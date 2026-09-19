// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package client

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/models"
	"github.com/g8e-ai/g8e/v2/internal/tools/agent_harness/config"
)

func TestOperatorBySession_ReturnsTypedDocumentAndRecordsExchange(t *testing.T) {
	operator := &models.OperatorDocumentGo{
		ID: "op-1", UserID: "user-1", OperatorSessionID: "session-1",
		Status: constants.OperatorStatusActive, OperatorType: constants.OperatorTypeRemote,
	}
	body, err := json.Marshal(models.OperatorResponse{Success: true, Operator: operator})
	require.NoError(t, err)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodGet, r.Method)
		assert.Equal(t, constants.APIPaths.OperatorsSession+"session-1", r.URL.Path)
		assert.Equal(t, "cli-1", r.Header.Get(constants.HeaderCLISessionID))
		w.Header().Set("Content-Type", "application/json")
		_, writeErr := w.Write(body)
		require.NoError(t, writeErr)
	}))
	t.Cleanup(server.Close)
	client, err := New(config.Config{MTLSBaseURL: server.URL, CLISessionID: "cli-1"})
	require.NoError(t, err)
	exchanges := []Exchange{}
	client.Record(&exchanges)

	doc, raw, err := client.OperatorBySession(context.Background(), "session-1")

	require.NoError(t, err)
	assert.Equal(t, "op-1", doc.ID)
	assert.Equal(t, "session-1", doc.OperatorSessionID)
	assert.Equal(t, body, raw)
	require.Len(t, exchanges, 1)
	assert.Equal(t, constants.APIPaths.OperatorsSession+"session-1", exchanges[0].URL[len(server.URL):])
}

func TestOperatorBySession_RequiresSessionID(t *testing.T) {
	client, err := New(config.Config{MTLSBaseURL: "https://gateway.invalid"})
	require.NoError(t, err)

	_, _, err = client.OperatorBySession(context.Background(), "")

	assert.ErrorIs(t, err, constants.ErrOperatorSessionIDRequired)
}

func TestOperatorBySession_RejectsInvalidResponses(t *testing.T) {
	tests := []struct {
		name       string
		status     int
		body       string
		wantErr    error
		wantErrMsg string
	}{
		{name: "gateway rejects session", status: http.StatusUnauthorized, body: `unauthorized`, wantErr: constants.ErrHTTPStatusError},
		{name: "malformed json", status: http.StatusOK, body: `not-json`, wantErr: constants.ErrInvalidJSONResponse},
		{name: "unsuccessful response", status: http.StatusOK, body: `{"success":false}`, wantErr: constants.ErrInvalidJSONResponse},
		{name: "missing operator document", status: http.StatusOK, body: `{"success":true}`, wantErr: constants.ErrInvalidJSONResponse},
		{name: "session binding mismatch", status: http.StatusOK, body: `{"success":true,"operator":{"id":"op-1","operator_session_id":"other-session"}}`, wantErr: constants.ErrInvalidJSONResponse, wantErrMsg: "different session"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tt.status)
				_, _ = w.Write([]byte(tt.body))
			}))
			t.Cleanup(server.Close)
			client, err := New(config.Config{MTLSBaseURL: server.URL})
			require.NoError(t, err)

			_, _, err = client.OperatorBySession(context.Background(), "session-1")

			require.Error(t, err)
			assert.ErrorIs(t, err, tt.wantErr)
			if tt.wantErrMsg != "" {
				assert.Contains(t, err.Error(), tt.wantErrMsg)
			}
		})
	}
}

func TestDiscoverRemoteOperator_SelectsExactlyOneActiveRemoteSession(t *testing.T) {
	operators := []models.OperatorDocumentGo{
		{ID: "op-embedded", OperatorSessionID: "sess-embedded", Status: constants.OperatorStatusActive, OperatorType: constants.OperatorTypeEmbedded},
		{ID: "op-offline", OperatorSessionID: "sess-offline", Status: constants.OperatorStatusOffline, OperatorType: constants.OperatorTypeRemote},
		{ID: "op-nosession", Status: constants.OperatorStatusActive, OperatorType: constants.OperatorTypeRemote},
		{ID: "op-remote", OperatorSessionID: "sess-remote", Status: constants.OperatorStatusActive, OperatorType: constants.OperatorTypeRemote},
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
	assert.Equal(t, "op-remote", doc.ID)
	assert.Equal(t, "sess-remote", doc.OperatorSessionID)
	assert.Equal(t, body, raw)
	require.Len(t, exchanges, 1)
}

func TestDiscoverRemoteOperator_RequiresAuthenticatedUser(t *testing.T) {
	client, err := New(config.Config{MTLSBaseURL: "https://gateway.invalid"})
	require.NoError(t, err)

	_, _, err = client.DiscoverRemoteOperator(context.Background())

	assert.ErrorIs(t, err, constants.ErrMissingRequiredField)
}

func TestDiscoverRemoteOperator_RejectsZeroOrAmbiguousMatches(t *testing.T) {
	tests := []struct {
		name      string
		operators []models.OperatorDocumentGo
		wantErr   error
	}{
		{
			name:      "no operators",
			operators: nil,
			wantErr:   constants.ErrEvaluationTargetUnavailable,
		},
		{
			name: "no active remote session",
			operators: []models.OperatorDocumentGo{
				{ID: "op-1", OperatorSessionID: "sess-1", Status: constants.OperatorStatusStale, OperatorType: constants.OperatorTypeRemote},
			},
			wantErr: constants.ErrEvaluationTargetUnavailable,
		},
		{
			name: "ambiguous active remote sessions",
			operators: []models.OperatorDocumentGo{
				{ID: "op-1", OperatorSessionID: "sess-1", Status: constants.OperatorStatusActive, OperatorType: constants.OperatorTypeRemote},
				{ID: "op-2", OperatorSessionID: "sess-2", Status: constants.OperatorStatusActive, OperatorType: constants.OperatorTypeRemote},
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
