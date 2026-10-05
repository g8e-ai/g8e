// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

//go:build integration

package tui

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/v2/internal/constants"
)

// TestAdapterNewAdapter_SetsCLISessionHeader verifies that the CLI session ID
// passed to NewAdapter is sent as the X-G8E-CLI-Session-ID header on the SSE
// request. Without this header, the gateway mTLS auth middleware cannot locate
// the CLI session and returns 401.
func TestAdapterNewAdapter_SetsCLISessionHeader(t *testing.T) {
	var mu sync.Mutex
	var gotHeader string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		gotHeader = r.Header.Get(constants.HeaderCLISessionID)
		mu.Unlock()
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprintf(w, "data: {\"type\":\"ledger.entry\",\"payload\":{\"level\":\"info\",\"message\":\"hi\"}}\n\n")
	}))
	defer srv.Close()

	sender := &mockSender{}
	a := NewAdapter(srv.URL, "", "cli-sess-123", sender, nil)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan struct{})
	go func() {
		a.Run(ctx)
		close(done)
	}()

	require.Eventually(t, func() bool {
		mu.Lock()
		defer mu.Unlock()
		return gotHeader != ""
	}, 3*time.Second, 50*time.Millisecond, "SSE request never received the CLI session header")

	mu.Lock()
	assert.Equal(t, "cli-sess-123", gotHeader, "X-G8E-CLI-Session-ID header must match cliSessionID arg")
	mu.Unlock()

	cancel()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("adapter.Run did not return after context cancellation")
	}
}

func TestAdapterRun_EmitsConnConnectedOnFirstEvent(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprintf(w, "data: {\"type\":\"ledger.entry\",\"payload\":{\"level\":\"info\",\"message\":\"hello\"}}\n\n")
	}))
	defer srv.Close()

	sender := &mockSender{}
	a := newAdapterWithSender(srv.URL, sender)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		a.Run(ctx)
		close(done)
	}()

	require.Eventually(t, func() bool {
		msgs := sender.snapshot()
		for _, m := range msgs {
			if cs, ok := m.(ConnStatusMsg); ok && cs.Status == ConnConnected {
				return true
			}
		}
		return false
	}, 3*time.Second, 50*time.Millisecond, "adapter never emitted ConnConnected")

	cancel()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("adapter.Run did not return after context cancellation")
	}

	msgs := sender.snapshot()
	var connecting, connected bool
	for _, m := range msgs {
		if cs, ok := m.(ConnStatusMsg); ok {
			if cs.Status == ConnConnecting {
				connecting = true
			}
			if cs.Status == ConnConnected {
				connected = true
			}
		}
	}
	assert.True(t, connecting, "expected ConnConnecting before ConnConnected")
	assert.True(t, connected, "expected ConnConnected after first event")
}
