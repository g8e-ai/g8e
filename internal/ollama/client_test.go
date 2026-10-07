// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package ollama

import (
	"context"
	"net/http"
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewClient_FallsBackToDefaultHTTPClientWhenNoneIsProvided(t *testing.T) {
	t.Parallel()

	base, err := url.Parse("http://localhost:11434")
	require.NoError(t, err)

	client := NewClient(base, nil)

	assert.Same(t, http.DefaultClient, client.http)
	assert.Same(t, base, client.base)
}

func TestNewClient_UsesTheProvidedHTTPClient(t *testing.T) {
	t.Parallel()

	base, err := url.Parse("http://localhost:11434")
	require.NoError(t, err)
	custom := &http.Client{}

	assert.Same(t, custom, NewClient(base, custom).http)
}

func TestClient_RejectsUnparseableBaseURLBeforeSendingAnything(t *testing.T) {
	t.Parallel()

	client := NewClient(&url.URL{Scheme: "http", Host: "bad host"}, nil)

	copyErr := client.Copy(context.Background(), "a", "b")
	pullErr := client.Pull(context.Background(), "a", nil)

	var urlErr *url.Error
	require.ErrorAs(t, copyErr, &urlErr)
	assert.Equal(t, "parse", urlErr.Op)
	urlErr = nil
	require.ErrorAs(t, pullErr, &urlErr)
	assert.Equal(t, "parse", urlErr.Op)
}

func TestCheckResponse_OnlyStatusesAtOrAboveBadRequestAreErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		status  int
		body    string
		wantErr string
	}{
		{name: "ok", status: http.StatusOK, body: `{"error":"ignored"}`},
		{name: "redirect", status: http.StatusFound},
		{name: "last non-error status", status: 399},
		{name: "first error status without body", status: http.StatusBadRequest, wantErr: "ollama api: status 400"},
		{name: "api error message wins over status", status: http.StatusBadRequest, body: `{"error":"bad model name"}`, wantErr: "ollama api: bad model name"},
		{name: "extra fields are ignored", status: http.StatusTeapot, body: `{"error":"short and stout","code":418}`, wantErr: "ollama api: short and stout"},
		{name: "array body is not an api error", status: http.StatusInternalServerError, body: `["error"]`, wantErr: "ollama api: status 500"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := checkResponse(tt.status, []byte(tt.body))

			if tt.wantErr == "" {
				assert.NoError(t, err)
				return
			}
			require.Error(t, err)
			assert.Equal(t, tt.wantErr, err.Error())
		})
	}
}
