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
	"net/url"
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
// user and blocks until an approval.completed event with a matching txHash
// arrives or the context expires. The httpClient must be configured with mTLS
// certificates. The baseURL should be the gateway's public HTTPS URL (e.g.,
// cfg.OperatorPublicURL()) to ensure TLS ServerName matches the cert SAN.
//
// If txHash is empty, the first approval.completed event for the user is
// accepted (useful for backward compatibility but not recommended).
func WaitForApprovalSSE(ctx context.Context, httpClient *http.Client, baseURL, userID, txHash string) error {
	sseURL := fmt.Sprintf("%s%s?user_id=%s&since_id=0",
		baseURL,
		constants.APIPaths.SSEStream,
		url.QueryEscape(userID))

	sseClient := sse.NewClient(sseURL, httpClient)

	waitCtx, cancel := context.WithTimeout(ctx, ApprovalSSETimeout)
	defer cancel()

	approved := make(chan struct{}, 1)
	var once sync.Once
	done := make(chan struct{})

	go func() {
		defer close(done)
		sseClient.Run(waitCtx, func(eventType, data string) {
			if eventType != constants.SSEEventTypeApprovalCompleted {
				return
			}

			var envelope models.SSEPushPayload
			if err := json.Unmarshal([]byte(data), &envelope); err != nil {
				return
			}

			var event models.ApprovalCompletedEvent
			if err := json.Unmarshal(envelope.Event, &event); err != nil {
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
