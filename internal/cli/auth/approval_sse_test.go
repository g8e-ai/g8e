// Copyright (c) 2026 Lateralus Labs, LLC.
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package auth

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/g8e-ai/g8e/internal/models"
)

// sseApprovalServer returns an httptest.Server that writes the given SSE frames
// as text/event-stream. Each frame is an "approval.completed" event with a
// data payload shaped as internalSSEPushPayload wrapping an ApprovalCompletedEvent.
func sseApprovalServer(t *testing.T, userID, txHash string, delay time.Duration) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		if delay > 0 {
			time.Sleep(delay)
		}
		eventPayload, err := json.Marshal(models.ApprovalCompletedEvent{
			Type:   constants.SSEEventTypeApprovalCompleted,
			UserID: userID,
			TxHash: txHash,
		})
		require.NoError(t, err)
		envelope := models.SSEPushPayload{
			UserID:       userID,
			CliSessionID: "cli-session",
			Event:        eventPayload,
		}
		envelopeJSON, err := json.Marshal(envelope)
		require.NoError(t, err)
		fmt.Fprintf(w, "event: %s\ndata: %s\n\n", constants.SSEEventTypeApprovalCompleted, string(envelopeJSON))
	}))
}

func TestWaitForApprovalSSE_CorrectTxHashMatch(t *testing.T) {
	const cliSessionID = "cli-session"
	const txHash = "tx-match-123"

	srv := sseApprovalServer(t, "user-match", txHash, 0)
	t.Cleanup(srv.Close)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err := WaitForApprovalSSE(ctx, srv.Client(), srv.URL, cliSessionID, txHash)
	require.NoError(t, err)
}

func TestWaitForApprovalSSE_WrongTxHashFiltered(t *testing.T) {
	const cliSessionID = "cli-session"
	const sentTxHash = "tx-wrong-999"
	const expectedTxHash = "tx-correct-001"

	srv := sseApprovalServer(t, "user-filter", sentTxHash, 0)
	t.Cleanup(srv.Close)

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	err := WaitForApprovalSSE(ctx, srv.Client(), srv.URL, cliSessionID, expectedTxHash)
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrApprovalSSETimeout)
}

func TestWaitForApprovalSSE_EnvelopeUnmarshaling(t *testing.T) {
	const cliSessionID = "cli-session"
	const txHash = "tx-envelope-456"

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		inner := models.ApprovalCompletedEvent{
			Type:   constants.SSEEventTypeApprovalCompleted,
			UserID: "user-envelope",
			TxHash: txHash,
		}
		innerJSON, err := json.Marshal(inner)
		require.NoError(t, err)
		envelope := models.SSEPushPayload{
			UserID:       "user-envelope",
			CliSessionID: cliSessionID,
			Event:        innerJSON,
		}
		envelopeJSON, err := json.Marshal(envelope)
		require.NoError(t, err)
		fmt.Fprintf(w, "event: %s\ndata: %s\n\n", constants.SSEEventTypeApprovalCompleted, string(envelopeJSON))
	}))
	t.Cleanup(srv.Close)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err := WaitForApprovalSSE(ctx, srv.Client(), srv.URL, cliSessionID, txHash)
	require.NoError(t, err)
}

func TestWaitForApprovalSSE_TimeoutNoEvents(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		<-r.Context().Done()
	}))
	t.Cleanup(srv.Close)

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	err := WaitForApprovalSSE(ctx, srv.Client(), srv.URL, "cli-timeout", "tx-timeout")
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrApprovalSSETimeout)
}

func TestWaitForApprovalSSE_ContextCancellation(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		<-r.Context().Done()
	}))
	t.Cleanup(srv.Close)

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()

	err := WaitForApprovalSSE(ctx, srv.Client(), srv.URL, "cli-cancel", "tx-cancel")
	require.Error(t, err)
}

func TestWaitForApprovalSSE_EmptyTxHashAcceptsAny(t *testing.T) {
	const cliSessionID = "cli-session"
	const txHash = "tx-any-789"

	srv := sseApprovalServer(t, "user-any", txHash, 0)
	t.Cleanup(srv.Close)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err := WaitForApprovalSSE(ctx, srv.Client(), srv.URL, cliSessionID, "")
	require.NoError(t, err)
}

// TestWaitForApprovalSSE_NoEventHeaderExtractsTypeFromPayload verifies that
// when the server omits the SSE event: field (R14), the consumer extracts the
// event type from the inner ApprovalCompletedEvent.Type field and still
// matches the approval event.
func TestWaitForApprovalSSE_NoEventHeaderExtractsTypeFromPayload(t *testing.T) {
	const cliSessionID = "cli-session"
	const txHash = "tx-no-header-001"

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		inner := models.ApprovalCompletedEvent{
			Type:   constants.SSEEventTypeApprovalCompleted,
			UserID: "user-no-header",
			TxHash: txHash,
		}
		innerJSON, err := json.Marshal(inner)
		require.NoError(t, err)
		envelope := models.SSEPushPayload{
			UserID:       "user-no-header",
			CliSessionID: cliSessionID,
			Event:        innerJSON,
		}
		envelopeJSON, err := json.Marshal(envelope)
		require.NoError(t, err)
		// No event: field — only data: line. The consumer must extract the
		// type from the inner payload (R14 fallback).
		fmt.Fprintf(w, "data: %s\n\n", string(envelopeJSON))
	}))
	t.Cleanup(srv.Close)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err := WaitForApprovalSSE(ctx, srv.Client(), srv.URL, cliSessionID, txHash)
	require.NoError(t, err)
}
