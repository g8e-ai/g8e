// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

//go:build e2e

package e2e

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/v2/internal/cli/sse"
	"github.com/g8e-ai/g8e/v2/internal/constants"
)

// TestSSE_LiveChatObserving verifies the end-to-end SSE bridge against the live
// stack (D.4):
//  1. The client establishes an authenticated SSE stream connection to the gateway.
//  2. The client sends a chat request to the ensemble with its CLI session ID.
//  3. The ensemble emits events (chat, iteration, approval) and pushes them to the gateway SSE bridge.
//  4. The gateway delivers the events to the client's live SSE stream.
func TestSSE_LiveChatObserving(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	operators, err := e2eClient.ListOperators(ctx)
	require.NoError(t, err, "operator list must succeed")
	require.True(t, operators.Success, "operator list response must report success")

	var targetOperatorID string
	var targetSessionID string
	for _, op := range operators.Operators {
		if op.Status == constants.OperatorStatusActive {
			targetOperatorID = op.ID
			targetSessionID = op.OperatorSessionID
			break
		}
	}
	require.NotEmpty(t, targetOperatorID, "active operator must exist")

	// 1. Establish SSE stream connection
	sseHTTPClient := &http.Client{
		Timeout:   0,
		Transport: e2eClient.mtlsClient.Transport,
	}
	sseURL := fmt.Sprintf("%s%s?since_id=0", e2eClient.gatewayHTTPS, constants.APIPaths.SSEStream)
	sseClient := sse.NewClient(sseURL, sseHTTPClient)
	sseClient.SetHeader(constants.HeaderCLISessionID, e2eClient.cliSessionID)

	connectedCh := make(chan struct{})
	var connectOnce sync.Once
	sseClient.SetOnConnect(func() {
		connectOnce.Do(func() {
			close(connectedCh)
		})
	})

	var mu sync.Mutex
	receivedEvents := make([]string, 0)
	subCtx, subCancel := context.WithCancel(ctx)
	defer subCancel()

	go sseClient.Run(subCtx, func(eventType, data string) {
		mu.Lock()
		defer mu.Unlock()
		receivedEvents = append(receivedEvents, eventType)
	})

	select {
	case <-connectedCh:
		t.Logf("SSE stream connected for cli_session_id=%s", e2eClient.cliSessionID)
	case <-time.After(15 * time.Second):
		t.Fatal("SSE stream failed to connect within 15s")
	}

	// 2. Also start the auto-approver for any approval request that may be emitted
	approver := e2eClient.StartApprovalAutoApprover(ctx, e2eCfg.ensembleURL)
	defer approver.Stop()

	// 3. Send a chat request
	runID := fmt.Sprintf("%d-%d", time.Now().UnixNano(), os.Getpid())
	filePath := fmt.Sprintf("/tmp/g8e-e2e-sse-smoke-%s.txt", runID)
	fileContent := fmt.Sprintf("g8e sse smoke test run=%s", runID)
	message := fmt.Sprintf("Create a new file at %s with the content: %s", filePath, fileContent)

	chatReq := EnsembleChatRequest{
		Context: EnsembleRequestContext{
			CLISessionID:    e2eClient.cliSessionID,
			UserID:          e2eClient.userID,
			SourceComponent: "CLIENT",
			BoundOperators: []EnsembleBoundOperator{
				{
					OperatorID:        targetOperatorID,
					OperatorSessionID: targetSessionID,
					Status:            string(constants.OperatorStatusBound),
				},
			},
		},
		Message:              message,
		SentinelMode:         true,
		ResourceCreation:     &EnsembleResourceCreation{CreateCase: true, CaseTitle: fmt.Sprintf("SSE observing %s", runID)},
		LLMPrimaryProvider:   "fake",
		LLMPrimaryModel:      "fake",
		LLMAssistantProvider: "fake",
		LLMAssistantModel:    "fake",
		LLMLiteProvider:      "fake",
		LLMLiteModel:         "fake",
	}

	chatResp, err := e2eClient.SendChatRequest(ctx, e2eCfg.ensembleURL, chatReq)
	require.NoError(t, err, "ensemble chat request must succeed")
	require.True(t, chatResp.Success, "ensemble chat must return success=true")

	// 4. Assert that events are received on the SSE stream
	require.Eventually(t, func() bool {
		mu.Lock()
		defer mu.Unlock()
		return len(receivedEvents) > 0
	}, 30*time.Second, 500*time.Millisecond, "at least one SSE event must be delivered on the stream")

	mu.Lock()
	count := len(receivedEvents)
	mu.Unlock()
	assert.Greater(t, count, 0, "SSE stream must deliver events to the connected client")
	t.Logf("SSE stream successfully observed %d live events", count)
}
