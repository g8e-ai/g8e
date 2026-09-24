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
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestClient_Copy_Succeeds(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPost, r.Method)
		assert.Equal(t, "/api/copy", r.URL.Path)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	base, err := url.Parse(server.URL)
	require.NoError(t, err)

	client := NewClient(base, server.Client())
	err = client.Copy(context.Background(), "source-model", "alias-model")
	require.NoError(t, err)
}

func TestClient_Pull_ReportsProgress(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPost, r.Method)
		assert.Equal(t, "/api/pull", r.URL.Path)
		w.Header().Set("Content-Type", "application/x-ndjson")
		_, _ = w.Write([]byte(`{"status":"pulling manifest"}` + "\n"))
	}))
	defer server.Close()

	base, err := url.Parse(server.URL)
	require.NoError(t, err)

	var statuses []string
	client := NewClient(base, server.Client())
	err = client.Pull(context.Background(), "glm-5.3-flash", func(progress ProgressResponse) error {
		statuses = append(statuses, progress.Status)
		return nil
	})
	require.NoError(t, err)
	assert.Equal(t, []string{"pulling manifest"}, statuses)
}

func TestClient_Pull_ReturnsAPIError(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/x-ndjson")
		_, _ = w.Write([]byte(`{"error":"model not found"}` + "\n"))
	}))
	defer server.Close()

	base, err := url.Parse(server.URL)
	require.NoError(t, err)

	client := NewClient(base, server.Client())
	err = client.Pull(context.Background(), "missing-model", nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "model not found")
}

func TestClient_Copy_ReturnsHTTPError(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":"invalid copy request"}`))
	}))
	defer server.Close()

	base, err := url.Parse(server.URL)
	require.NoError(t, err)

	client := NewClient(base, server.Client())
	err = client.Copy(context.Background(), "source-model", "alias-model")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid copy request")
}
