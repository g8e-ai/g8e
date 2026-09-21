// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package inference

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestWaitForProviderModelsAbsent_WaitsForNamedModelToDisappear(t *testing.T) {
	t.Parallel()
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/api/ps", r.URL.Path)
		if calls.Add(1) == 1 {
			_, _ = w.Write([]byte(`{"models":[{"name":"qwen3:0.6b"}]}`))
			return
		}
		_, _ = w.Write([]byte(`{"models":[]}`))
	}))
	t.Cleanup(server.Close)

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	err := WaitForProviderModelsAbsent(ctx, ProviderResidencyOptions{
		Endpoint:     server.URL,
		PollInterval: time.Millisecond,
	}, []string{"qwen3:0.6b"})
	require.NoError(t, err)
	require.Equal(t, int32(2), calls.Load())
}

func TestReadProviderResidency_RejectsMalformedResponse(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"models":[`))
	}))
	t.Cleanup(server.Close)

	_, err := ReadProviderResidency(context.Background(), ProviderResidencyOptions{Endpoint: server.URL})
	require.Error(t, err)
}

func TestWaitForProviderModelsAbsent_DoesNotTreatStableResidentModelAsAbsent(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"models":[{"name":"qwen3:0.6b"}]}`))
	}))
	t.Cleanup(server.Close)

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	err := WaitForProviderModelsAbsent(ctx, ProviderResidencyOptions{
		Endpoint:     server.URL,
		PollInterval: time.Millisecond,
	}, []string{"qwen3:0.6b"})
	require.Error(t, err)
	require.ErrorIs(t, err, context.DeadlineExceeded)
}

func TestReadProviderResidency_RejectsInvalidEndpoint(t *testing.T) {
	t.Parallel()
	_, err := ReadProviderResidency(context.Background(), ProviderResidencyOptions{Endpoint: "ftp://bad"})
	require.Error(t, err)
}
