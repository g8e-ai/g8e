// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

//go:build integration

package gateway

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/models"
)

// Flush is the client's permission to start workers. Publish synchronously at
// that boundary: no sleeps, retry, replay, or scheduler luck can hide a gap.
func TestHandleInternalSSEStream_FirstFlushCanDeliverDeploymentAnnouncements(t *testing.T) {
	for _, query := range []string{"?since_id=0", ""} {
		t.Run("query="+query, func(t *testing.T) {
			h, _, _ := setupTestHTTPHandler(t)
			ctx, userID, cliSessionID := seedCLISessionCtx(t, h, "startup")
			ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
			defer cancel()
			route := SSERoute{UserID: userID, CLISessionID: cliSessionID}
			publisher := NewSSEEventPublisher(h.dataController.sseStore, h.GetGatewayWebSocketHandler())
			writer := &startupStreamWriter{ResponseRecorder: httptest.NewRecorder()}
			var publishErrors []error
			var connectedAtFlush []string
			var deadlineClearedAtFlush bool
			writer.onFirstFlush = func() {
				connectedAtFlush = publisher.ConnectedCLISessionIDs()
				deadlineClearedAtFlush = writer.deadlineCleared
				publishErrors = append(publishErrors, publisher.PublishEphemeral(route,
					string(constants.EventPlatformApprovalsChanged), models.ApprovalsChangedPayload{
						Subject: models.ApprovalsChangedEnrollments, DeploymentID: "launch-at-flush", RequestID: "request-at-flush",
					}))
				publishErrors = append(publishErrors, publisher.PublishEphemeral(route,
					string(constants.EventOperatorStatusUpdatedActive), models.OperatorStatusUpdatedPayload{
						OperatorID: "operator-at-flush", OperatorSessionID: "session-at-flush",
						DeploymentID: "launch-at-flush", Status: constants.OperatorStatusActive,
					}))
			}
			writer.afterFlush = func() {
				if strings.Contains(writer.Body.String(), "request-at-flush") && strings.Contains(writer.Body.String(), "session-at-flush") {
					cancel()
				}
			}
			request := httptest.NewRequest(http.MethodGet, "/api/v1/sse/stream"+query, nil).WithContext(ctx)
			h.sseController.handleInternalSSEStream(writer, request)

			for _, err := range publishErrors {
				require.NoError(t, err)
			}
			assert.Contains(t, connectedAtFlush, cliSessionID, "the listener must exist when the client sees HTTP 200")
			assert.True(t, deadlineClearedAtFlush, "clear the server write deadline before exposing the stream")
			assert.Equal(t, 1, strings.Count(writer.Body.String(), "request-at-flush"))
			assert.Equal(t, 1, strings.Count(writer.Body.String(), "session-at-flush"))
			assert.NotContains(t, writer.Body.String(), "id:", "ephemeral announcements must not alter the replay cursor")
			assert.Empty(t, publisher.ConnectedCLISessionIDs(), "cancellation must unregister the listener")
			rows, err := h.dataController.sseStore.SSEEventsListSince(route, 0, 100)
			require.NoError(t, err)
			assert.Empty(t, rows, "the test cannot recover a missed announcement through durable replay")
		})
	}
}

type startupStreamWriter struct {
	*httptest.ResponseRecorder
	onFirstFlush    func()
	afterFlush      func()
	flushed         bool
	deadlineCleared bool
}

func (w *startupStreamWriter) SetWriteDeadline(deadline time.Time) error {
	w.deadlineCleared = deadline.IsZero()
	return nil
}

func (w *startupStreamWriter) Flush() {
	w.ResponseRecorder.Flush()
	if !w.flushed {
		w.flushed = true
		w.onFirstFlush()
	}
	w.afterFlush()
}
