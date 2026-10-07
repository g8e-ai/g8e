// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

//go:build integration

package inference

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/testutil"
)

func newResidencyBackend(t *testing.T, handler http.HandlerFunc) *OllamaBackend {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	backend, err := NewOllamaBackend(server.URL, testutil.NewTestLogger())
	require.NoError(t, err)
	return backend
}

func TestOllamaBackendReadResidency_ReturnsResidentModels(t *testing.T) {
	backend := newResidencyBackend(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/ps", r.URL.Path)
		_, _ = w.Write([]byte(`{"models":[{"name":"qwen3:0.6b"},{"name":"qwen3:1.7b"}]}`))
	})

	residency, err := backend.ReadResidency(context.Background())

	require.NoError(t, err)
	assert.Equal(t, ProviderResidency{Models: []ProviderResidencyModel{{Name: "qwen3:0.6b"}, {Name: "qwen3:1.7b"}}}, residency)
}

func TestOllamaBackendReadResidency_RejectsInvalidResponses(t *testing.T) {
	tests := []struct {
		name    string
		handler http.HandlerFunc
		wantErr error
	}{
		{
			name: "malformed JSON",
			handler: func(w http.ResponseWriter, _ *http.Request) {
				_, _ = w.Write([]byte(`{"models":[`))
			},
			wantErr: constants.ErrInferenceProviderResponseInvalid,
		},
		{
			name: "resident model without a name",
			handler: func(w http.ResponseWriter, _ *http.Request) {
				_, _ = w.Write([]byte(`{"models":[{"name":""}]}`))
			},
			wantErr: constants.ErrInferenceProviderResponseInvalid,
		},
		{
			name: "non-OK status",
			handler: func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusInternalServerError)
			},
			wantErr: constants.ErrInferenceBackendUnavailable,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			backend := newResidencyBackend(t, test.handler)

			_, err := backend.ReadResidency(context.Background())

			assert.ErrorIs(t, err, test.wantErr)
		})
	}
}
