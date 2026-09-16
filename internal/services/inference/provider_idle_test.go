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

func TestWaitForProviderIdle_WaitsForStablePSResponse(t *testing.T) {
	t.Parallel()
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/api/ps", r.URL.Path)
		if calls.Add(1) <= 2 {
			_, _ = w.Write([]byte(`{"models":[{"name":"qwen3:4b"}]}`))
			return
		}
		_, _ = w.Write([]byte(`{"models":[]}`))
	}))
	defer server.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	err := WaitForProviderIdle(ctx, ProviderIdleOptions{
		Endpoint:       server.URL,
		PollInterval:   25 * time.Millisecond,
		SettleDuration: 75 * time.Millisecond,
	})
	require.NoError(t, err)
	require.GreaterOrEqual(t, calls.Load(), int32(3))
}

func TestWaitForProviderIdle_RejectsInvalidEndpoint(t *testing.T) {
	t.Parallel()
	err := WaitForProviderIdle(context.Background(), ProviderIdleOptions{Endpoint: "ftp://bad"})
	require.Error(t, err)
}
