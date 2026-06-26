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

package gateway

// Browser-Facing SSE Audit Stream
//
// GET /api/v1/audit/stream → Consumer (browser console) streams SSE events
//                            scoped to the authenticated web session.
//
// Authentication is via WebSessionAuth middleware (cookie-based), not mTLS.
// The web_session_id from the cookie is used as the SSE routing target,
// ensuring events are scoped to the browser session that owns them.
//
// Query parameters:
//   since_id — replay events with id > since_id before entering live stream

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/g8e-ai/g8e/internal/constants"
)

// handleWebAuditStream streams SSE events to the browser console, scoped to
// the authenticated web session.  This handler is mounted under WebSessionAuth
// so the cookie has already been validated and user_id is stamped in context.
//
// @Summary		Live Audit Stream (SSE)
// @Description	Server-sent events stream of audit events scoped to the authenticated web session.
// @Tags			audit
// @Produce		text/event-stream
// @Param			since_id	query		int		false	"Replay events with id greater than this value"
// @Success		200			{string}	string	"SSE event stream"
// @Router			/api/v1/audit/stream [get]
func (h *HTTPHandler) handleWebAuditStream(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		h.responder.Error(w, http.StatusMethodNotAllowed, constants.ErrMethodNotAllowed.Error())
		return
	}

	// The WebSessionAuth middleware has already validated the cookie and
	// stamped user_id in context.  We extract the raw web_session_id from
	// the cookie to use as the SSE routing target.
	cookie, err := r.Cookie(constants.WebSessionCookieName)
	if err != nil || cookie == nil || cookie.Value == "" {
		h.responder.Error(w, http.StatusUnauthorized, constants.ErrWebSessionCookieRequired.Error())
		return
	}
	webSessionID := cookie.Value

	// Parse since_id for optional replay (also honors Last-Event-ID header)
	sinceIDStr := r.URL.Query().Get("since_id")
	if lastEventID := r.Header.Get("Last-Event-ID"); lastEventID != "" {
		sinceIDStr = lastEventID
	}
	sinceID, _ := strconv.ParseInt(sinceIDStr, 10, 64)

	route := SSERoute{WebSessionID: webSessionID}
	channel := "sse:web:" + webSessionID

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}

	// Set SSE headers
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no") // Nginx: disable proxy buffering
	origin := r.Header.Get("Origin")
	if origin != "" {
		w.Header().Set("Access-Control-Allow-Origin", origin)
		w.Header().Set("Access-Control-Allow-Credentials", "true")
	} else {
		w.Header().Set("Access-Control-Allow-Origin", "*")
	}

	// Subscribe to real-time events FIRST to avoid gaps during replay
	eventCh := make(chan []byte, 100)
	unregister := h.pubsub.RegisterHandler(channel, func(ch string, data []byte) {
		select {
		case eventCh <- data:
		default:
			h.logger.Warn("Web Audit Stream: back-pressure dropping event", "channel", channel)
		}
	})
	defer unregister()

	// Replay from DB if since_id is provided
	if sinceID > 0 {
		rows, err := h.db.SSEStore.SSEEventsListSince(route, sinceID, 1000)
		if err != nil {
			h.logger.Error("Web Audit Stream: replay failed", "channel", channel, "error", err)
		} else {
			for _, row := range rows {
				if _, err := fmt.Fprintf(w, "id: %d\ndata: %s\n\n", row.ID, row.Payload); err != nil {
					h.logger.Info("Web Audit Stream: client disconnected during replay", "channel", channel)
					return
				}
			}
			flusher.Flush()
		}
	}

	// Stream from pubsub
	ctx := r.Context()
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	h.logger.Info("Web Audit Stream: browser client connected", "channel", channel)

	for {
		select {
		case <-ctx.Done():
			h.logger.Info("Web Audit Stream: browser client disconnected", "channel", channel)
			return
		case <-ticker.C:
			if _, err := fmt.Fprintf(w, ": heartbeat\n\n"); err != nil {
				h.logger.Info("Web Audit Stream: client disconnected during heartbeat", "channel", channel)
				return
			}
			flusher.Flush()
		case raw := <-eventCh:
			var p internalSSEPushPayload
			if err := json.Unmarshal(raw, &p); err == nil {
				// No event: field — the type is already embedded in the JSON payload.
				// Using named events would require addEventListener per type on the
				// client side; onmessage only fires for unnamed (or "message") events.
				if _, err := fmt.Fprintf(w, "data: %s\n\n", string(raw)); err != nil {
					h.logger.Info("Web Audit Stream: client disconnected during live stream", "channel", channel)
					return
				}
				flusher.Flush()
			}
		}
	}
}
