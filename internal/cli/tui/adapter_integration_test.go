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
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/models"
)

// runAdapter starts the adapter against session and returns a stop func that
// cancels it and waits for Run to return.
func runAdapter(t *testing.T, session Session, sender messageSender) func() {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		NewAdapter(session, "test@example.com", sender).Run(ctx)
		close(done)
	}()
	return func() {
		cancel()
		select {
		case <-done:
		case <-time.After(3 * time.Second):
			t.Fatal("adapter.Run did not return after context cancellation")
		}
	}
}

// TestAdapterRun_ConnectedOnOpenStream verifies the adapter reports CONNECTED
// once the stream opens, before any event arrives. The Gateway's heartbeats
// are SSE comments, so a quiet live-only stream never dispatches an event.
func TestAdapterRun_ConnectedOnOpenStream(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		w.(http.Flusher).Flush()
		<-r.Context().Done()
	}))
	defer srv.Close()

	sender := &mockSender{}
	stop := runAdapter(t, &testSession{url: srv.URL}, sender)
	defer stop()

	require.Eventually(t, func() bool {
		for _, m := range sender.snapshot() {
			if cs, ok := m.(ConnStatusMsg); ok && cs.Status == ConnConnected {
				return true
			}
		}
		return false
	}, 3*time.Second, 50*time.Millisecond, "adapter never emitted ConnConnected")
}

// TestAdapterRun_ReconcilesPendingApprovals verifies the pending-approval list
// is fetched on connect and re-fetched on approvals.changed.
func TestAdapterRun_ReconcilesPendingApprovals(t *testing.T) {
	changed := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		w.(http.Flusher).Flush()
		select {
		case <-changed:
			frame := gatewayFrame(t, string(constants.EventPlatformApprovalsChanged), models.ApprovalsChangedPayload{Subject: models.ApprovalsChangedTransactions})
			fmt.Fprintf(w, "data: %s\n\n", frame)
			w.(http.Flusher).Flush()
		case <-r.Context().Done():
			return
		}
		<-r.Context().Done()
	}))
	defer srv.Close()

	session := &testSession{url: srv.URL, pendingJSON: `{"transactions":[{"transaction_hash":"tx-pending-1","tool_name":"run_command"}]}`}
	sender := &mockSender{}
	stop := runAdapter(t, session, sender)
	defer stop()

	require.Eventually(t, func() bool { return session.pendingListCalls() == 1 }, 3*time.Second, 20*time.Millisecond, "pending approvals not fetched on connect")
	close(changed)
	require.Eventually(t, func() bool { return session.pendingListCalls() == 2 }, 3*time.Second, 20*time.Millisecond, "pending approvals not re-fetched on approvals.changed")

	var got PendingApprovalsMsg
	for _, m := range sender.snapshot() {
		if pm, ok := m.(PendingApprovalsMsg); ok {
			got = pm
		}
	}
	require.NoError(t, got.Err)
	require.Len(t, got.Transactions, 1)
	assert.Equal(t, "tx-pending-1", got.Transactions[0].TransactionHash)
}
