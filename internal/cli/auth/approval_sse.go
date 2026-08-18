// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package auth

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/g8e-ai/g8e/internal/cli/sse"
	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/g8e-ai/g8e/internal/models"
)

// ApprovalSSETimeout is the maximum time to wait for an approval.completed SSE
// event. The gateway's approval request TTL is 2 minutes; this allows a small
// grace period.
const ApprovalSSETimeout = 3 * time.Minute

// WaitForApprovalSSE subscribes to the gateway's SSE stream scoped to the given
// CLI session and blocks until an approval.completed event with a matching txHash
// arrives or the context expires. The httpClient must be configured with mTLS
// certificates. The baseURL should be the gateway's public HTTPS URL (e.g.,
// cfg.OperatorPublicURL()) to ensure TLS ServerName matches the cert SAN.
//
// The cliSessionID is sent as the X-G8E-CLI-Session-ID header so the gateway
// can route the SSE subscription to the correct session. The user_id is derived
// from the mTLS cert at the gateway — it is not passed in the URL.
//
// If txHash is empty, the first approval.completed event for the session is
// accepted (useful for backward compatibility but not recommended).
func WaitForApprovalSSE(ctx context.Context, httpClient *http.Client, baseURL, cliSessionID, txHash string) error {
	sseURL := fmt.Sprintf("%s%s?since_id=0",
		baseURL,
		constants.APIPaths.SSEStream)

	sseClient := sse.NewClient(sseURL, httpClient)
	sseClient.SetHeader(constants.HeaderCLISessionID, cliSessionID)

	waitCtx, cancel := context.WithTimeout(ctx, ApprovalSSETimeout)
	defer cancel()

	approved := make(chan struct{}, 1)
	var once sync.Once
	done := make(chan struct{})

	go func() {
		defer close(done)
		sseClient.Run(waitCtx, func(eventType, data string) {
			var envelope models.SSEPushPayload
			if err := json.Unmarshal([]byte(data), &envelope); err != nil {
				return
			}

			var event models.ApprovalCompletedEvent
			if err := json.Unmarshal(envelope.Event, &event); err != nil {
				return
			}

			// When the server omits the event: field (R14), eventType is empty.
			// Extract the type from the inner payload instead.
			innerType := eventType
			if innerType == "" {
				innerType = event.Type
			}
			if innerType != constants.SSEEventTypeApprovalCompleted {
				return
			}

			if txHash != "" && event.TxHash != txHash {
				return
			}

			once.Do(func() { close(approved) })
		})
	}()

	select {
	case <-approved:
		cancel()
		<-done
		return nil
	case <-waitCtx.Done():
		<-done
		return constants.ErrApprovalSSETimeout
	}
}
