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

// SSE Event Bridge
//
// POST /api/v1/sse/push     → Producer (g8e-compatible agentic ensemble) appends an event.
//                            Body MUST set exactly one of
//                            web_session_id, cli_session_id, user_id.
// GET  /api/v1/sse/events   → Consumer (CLI / dashboard) polls events.
//                            Query string MUST set exactly one of
//                            web_session_id, cli_session_id, user_id,
//                            plus since_id=N and limit=K.
// GET  /api/v1/sse/stream   → Consumer streams events via SSE.
//                            Auth: mTLS (Operator session) or web session cookie.
//                            mTLS clients pass routing target via query params;
//                            cookie clients are scoped to their web_session_id.
//
// The Gateway refuses to talk about a bare session id - every routing
// target is tagged at the type level so a web_session_id can never be
// mis-delivered as a cli_session_id (or vice versa).

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/g8e-ai/g8e/internal/marshaler"
	"github.com/g8e-ai/g8e/internal/models"
	"github.com/g8e-ai/g8e/protocol"
)

// internalSSEPushPayload mirrors the wire shape produced by g8e-compatible agentic ensembles
// (SessionEventWire | BackgroundEventWire). Producers MUST set exactly one of
// web_session_id (web UI session), cli_session_id (CLI / BYO session), or
// user_id (background fan-out across every session a user owns).
type internalSSEPushPayload struct {
	WebSessionID string          `json:"web_session_id"`
	CliSessionID string          `json:"cli_session_id"`
	UserID       string          `json:"user_id"`
	Event        json.RawMessage `json:"event"`
}

func (h *HTTPHandler) handleInternalSSEPush(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		h.responder.Error(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	// Strictly verify that the caller is an app workload via mTLS peer certificate URI SAN
	if r.TLS == nil || len(r.TLS.PeerCertificates) == 0 {
		h.logger.Warn("Unauthorized SSE push attempt: missing mTLS client certificate", "path", r.URL.Path)
		h.responder.Error(w, http.StatusUnauthorized, "mTLS client certificate required")
		return
	}

	cert := r.TLS.PeerCertificates[0]
	appID := ""
	isAppWorkload := false
	for _, uri := range cert.URIs {
		// Only g8e-compatible agentic ensembles are authorized to push SSE events, as they act as the centralized event broker
		// between LLM generations and the end user. Accept any app workload identity (SPIFFE ID with /app/ prefix)
		// except Operator identities (g8eo, g8eg).
		if strings.HasPrefix(uri.Path, "/app/") && uri.Path != "/app/g8eo" && uri.Path != "/app/g8eg" {
			isAppWorkload = true
			appID = uri.String()
			break
		}
	}
	if !isAppWorkload {
		h.logger.Warn("Unauthorized SSE push attempt: not app workload identity", "path", r.URL.Path, "uris", cert.URIs)
		h.responder.Error(w, http.StatusForbidden, "unauthorized client identity")
		return
	}

	body, err := h.readBody(r)
	if err != nil {
		h.responder.Error(w, http.StatusBadRequest, "failed to read body")
		return
	}

	var p internalSSEPushPayload
	if err := json.Unmarshal(body, &p); err != nil {
		h.responder.Error(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	if len(p.Event) == 0 {
		h.responder.Error(w, http.StatusBadRequest, "event field is required")
		return
	}

	route := SSERoute{
		WebSessionID: strings.TrimSpace(p.WebSessionID),
		CLISessionID: strings.TrimSpace(p.CliSessionID),
		UserID:       strings.TrimSpace(p.UserID),
	}

	// Extract event.type for indexing/filtering. Store the full envelope as the payload.
	var inner struct {
		Type string `json:"type"`
	}
	_ = json.Unmarshal(p.Event, &inner)
	if inner.Type == "" {
		inner.Type = string(constants.SystemHealthUnknown)
	}

	if err := h.db.SSEStore.SSEEventsAppend(route, inner.Type, string(body), appID); err != nil {
		h.logger.Error("SSE push: failed to append event", string(constants.ConnectionStateError), err, "type", inner.Type)
		h.responder.Error(w, http.StatusBadRequest, err.Error())
		return
	}

	// Authorization: Enforce producer-to-target ownership.
	// The app identity extracted from the peer certificate must be associated with the target.
	if route.WebSessionID != "" {
		webBindKey := sessionWebBindKey(route.WebSessionID)
		raw, ok := h.db.KVStore.KVGet(webBindKey)
		if !ok {
			h.logger.Warn("SSE push: target web session has no bound operators", "web_session_id", route.WebSessionID, "app_id", appID)
			h.responder.Error(w, http.StatusForbidden, "target session not found or not bound")
			return
		}
		var operatorSessionIDs []string
		if err := json.Unmarshal([]byte(raw), &operatorSessionIDs); err != nil {
			h.logger.Error("SSE push: failed to parse web session bindings", "web_session_id", route.WebSessionID, "error", err)
			h.responder.Error(w, http.StatusInternalServerError, "failed to verify session ownership")
			return
		}

		// Check if any bound Operator session is associated with this appID
		authorized := false
		for _, opSessID := range operatorSessionIDs {
			opDoc, err := h.db.DocStore.DocGet(marshaler.CollectionName(constants.CollectionOperators), opSessID)
			if err != nil || opDoc == nil {
				continue
			}
			// AppID format in this context is the SPIFFE ID string
			if opDoc.ID == appID || strings.HasSuffix(appID, "/app/"+opSessID) {
				authorized = true
				break
			}
			// Alternative: check if the app is explicitly allowed by the operator's policy or if it's the engine
			// For now, we'll keep it simple: if the app is spiffe://g8e.local/app/<operator_id>, it's authorized.
			// MatchesApp(spiffeID, operatorID) from workload_identity.go
			wid := protocol.NewWorkloadIdentity()
			if wid.MatchesApp(appID, opDoc.ID) {
				authorized = true
				break
			}
		}

		if !authorized {
			h.logger.Warn("SSE push: app not authorized for target web session", "app_id", appID, "web_session_id", route.WebSessionID)
			h.responder.Error(w, http.StatusForbidden, "unauthorized for target session")
			return
		}
	} else if route.CLISessionID != "" {
		doc, err := h.db.DocStore.DocGet(marshaler.CollectionName(constants.CollectionCLISessions), route.CLISessionID)
		if err != nil || doc == nil {
			h.logger.Warn("SSE push: target CLI session not found", "cli_session_id", route.CLISessionID, "app_id", appID)
			h.responder.Error(w, http.StatusForbidden, "target session not found")
			return
		}
		var cliSess models.CLISession
		b, err := json.Marshal(doc.Data)
		if err != nil {
			h.logger.Error("SSE push: failed to marshal CLI session", "cli_session_id", route.CLISessionID, "error", err)
			h.responder.Error(w, http.StatusInternalServerError, "failed to verify session ownership")
			return
		}
		if err := json.Unmarshal(b, &cliSess); err != nil {
			h.logger.Error("SSE push: failed to parse CLI session", "cli_session_id", route.CLISessionID, "error", err)
			h.responder.Error(w, http.StatusInternalServerError, "failed to verify session ownership")
			return
		}

		// Verify app owns the Operator session bound to this CLI session
		opDoc, err := h.db.DocStore.DocGet(marshaler.CollectionName(constants.CollectionOperators), cliSess.OperatorSessionID)
		if err != nil || opDoc == nil {
			h.logger.Warn("SSE push: Operator session for CLI session not found", "operator_session_id", cliSess.OperatorSessionID, "cli_session_id", route.CLISessionID)
			h.responder.Error(w, http.StatusForbidden, "operator session not found")
			return
		}

		wid := protocol.NewWorkloadIdentity()
		if !wid.MatchesApp(appID, opDoc.ID) {
			h.logger.Warn("SSE push: app not authorized for target CLI session", "app_id", appID, "cli_session_id", route.CLISessionID)
			h.responder.Error(w, http.StatusForbidden, "unauthorized for target session")
			return
		}
	} else if route.UserID != "" {
		// User-scoped pushes: app must be authorized for AT LEAST ONE session belonging to the user.
		// We check if the app identity corresponds to an Operator owned by this user.
		filters := []models.DocFilter{
			{Field: "user_id", Op: "==", Value: json.RawMessage(fmt.Sprintf("%q", route.UserID))},
		}
		docs, err := h.db.DocStore.DocQuery(marshaler.CollectionName(constants.CollectionOperators), filters, "", 100)
		if err != nil || len(docs) == 0 {
			h.logger.Warn("SSE push: user has no operators", "user_id", route.UserID, "app_id", appID)
			h.responder.Error(w, http.StatusForbidden, "unauthorized for target user")
			return
		}

		// Check if the app is authorized for any of the user's operators
		authorized := false
		wid := protocol.NewWorkloadIdentity()
		for _, doc := range docs {
			if wid.MatchesApp(appID, doc.ID) {
				authorized = true
				break
			}
		}

		if !authorized {
			h.logger.Warn("SSE push: app not authorized for target user", "app_id", appID, "user_id", route.UserID)
			h.responder.Error(w, http.StatusForbidden, "unauthorized for target user")
			return
		}
	}

	// Publish to pub/sub for real-time streaming
	// We use the same routing logic: exactly one of web_session_id, cli_session_id, or user_id.
	var channel string
	switch {
	case route.CLISessionID != "":
		channel = "sse:cli:" + route.CLISessionID
	case route.WebSessionID != "":
		channel = "sse:web:" + route.WebSessionID
	case route.UserID != "":
		channel = "sse:user:" + route.UserID
	}

	if channel != "" {
		// We publish the full body which is the internalSSEPushPayload JSON.
		// The streamer will wrap this in SSE format.
		h.pubsub.Publish(channel, body)
	}

	h.responder.JSON(w, http.StatusOK, models.SSEPushResponse{
		Success:   true,
		Delivered: 1,
	})
}

func (h *HTTPHandler) handleInternalSSEEvents(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		h.responder.Error(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	q := r.URL.Query()
	route := SSERoute{
		WebSessionID: strings.TrimSpace(q.Get("web_session_id")),
		CLISessionID: strings.TrimSpace(q.Get("cli_session_id")),
		UserID:       strings.TrimSpace(q.Get("user_id")),
	}
	sinceID, _ := strconv.ParseInt(q.Get("since_id"), 10, 64)
	limit, _ := strconv.Atoi(q.Get("limit"))

	// Authorization: ensure the authenticated Operator session has the right
	// to access the requested routing buffer. Without this check, any operator
	// could drain any other client's event buffer, creating a multi-tenant
	// data leak.
	operatorSessionID := h.auth.extractOperatorSessionIDFromMTLS(r)
	if operatorSessionID == "" {
		h.responder.Error(w, http.StatusUnauthorized, "missing Operator session id")
		return
	}

	// Consumers MUST declare exactly one routing target. The Gateway refuses
	// to fall back to a single shared namespace because that is precisely the
	// conflation we are eliminating.
	switch {
	case route.CLISessionID != "" && route.WebSessionID == "" && route.UserID == "":
		// Verify operator_session_id is bound to this cli_session_id.
		doc, err := h.db.DocStore.DocGet(marshaler.CollectionName(constants.CollectionCLISessions), route.CLISessionID)
		if err != nil {
			h.logger.Error("Failed to fetch CLI session", string(constants.ConnectionStateError), err, "cli_session_id", route.CLISessionID)
			h.responder.Error(w, http.StatusInternalServerError, "failed to verify cli session")
			return
		}
		if doc == nil {
			h.responder.Error(w, http.StatusForbidden, "cli session not found")
			return
		}
		var cliSess models.CLISession
		b, err := json.Marshal(doc.ForWire())
		if err != nil {
			h.logger.Error("Failed to marshal CLI session", string(constants.ConnectionStateError), err)
			h.responder.Error(w, http.StatusInternalServerError, "failed to verify cli session")
			return
		}
		if err := json.Unmarshal(b, &cliSess); err != nil {
			h.logger.Error("Failed to unmarshal CLI session", string(constants.ConnectionStateError), err)
			h.responder.Error(w, http.StatusInternalServerError, "failed to verify cli session")
			return
		}
		if cliSess.OperatorSessionID != operatorSessionID {
			h.responder.Error(w, http.StatusForbidden, "operator session does not own this cli session")
			return
		}
	case route.WebSessionID != "" && route.CLISessionID == "" && route.UserID == "":
		// Verify operator_session_id is bound to this web_session_id.
		operatorBindKey := sessionOperatorBindKey(operatorSessionID)
		boundWebSessionID, ok := h.db.KVStore.KVGet(operatorBindKey)
		if !ok || boundWebSessionID != route.WebSessionID {
			h.responder.Error(w, http.StatusForbidden, "operator session does not own this web session")
			return
		}
	case route.UserID != "" && route.WebSessionID == "" && route.CLISessionID == "":
		// User-scoped events are accessible to any Operator owned by that user.
		op, err := h.auth.ValidateOperatorSession(operatorSessionID)
		if err != nil {
			h.responder.Error(w, http.StatusUnauthorized, "invalid Operator session")
			return
		}
		if op.UserID != route.UserID {
			h.responder.Error(w, http.StatusForbidden, "operator does not belong to this user")
			return
		}
	default:
		h.responder.Error(w, http.StatusBadRequest, "exactly one of web_session_id, cli_session_id, user_id is required")
		return
	}

	rows, err := h.db.SSEStore.SSEEventsListSince(route, sinceID, limit)
	if err != nil {
		h.responder.Error(w, http.StatusBadRequest, err.Error())
		return
	}
	h.responder.JSON(w, http.StatusOK, models.SSEEventsResponse{
		Events: rows,
		Count:  len(rows),
	})
}

func (h *HTTPHandler) handleInternalSSEStream(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		h.responder.Error(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	q := r.URL.Query()
	route := SSERoute{
		WebSessionID: strings.TrimSpace(q.Get("web_session_id")),
		CLISessionID: strings.TrimSpace(q.Get("cli_session_id")),
		UserID:       strings.TrimSpace(q.Get("user_id")),
	}
	sinceIDStr := q.Get("since_id")
	if lastEventID := r.Header.Get("Last-Event-ID"); lastEventID != "" {
		sinceIDStr = lastEventID
	}
	sinceID, _ := strconv.ParseInt(sinceIDStr, 10, 64)

	operatorSessionID := h.auth.extractOperatorSessionIDFromMTLS(r)
	if operatorSessionID == "" {
		h.responder.Error(w, http.StatusUnauthorized, "missing Operator session id")
		return
	}

	var channel string
	switch {
	case route.CLISessionID != "" && route.WebSessionID == "" && route.UserID == "":
		doc, err := h.db.DocStore.DocGet(marshaler.CollectionName(constants.CollectionCLISessions), route.CLISessionID)
		if err != nil || doc == nil {
			h.responder.Error(w, http.StatusForbidden, "not authorized for this cli session")
			return
		}
		var cliSess models.CLISession
		b, err := json.Marshal(doc.ForWire())
		if err != nil {
			h.responder.Error(w, http.StatusInternalServerError, "failed to verify cli session")
			return
		}
		if err := json.Unmarshal(b, &cliSess); err != nil {
			h.responder.Error(w, http.StatusInternalServerError, "failed to verify cli session")
			return
		}
		if cliSess.OperatorSessionID != operatorSessionID {
			h.responder.Error(w, http.StatusForbidden, "not authorized for this cli session")
			return
		}
		channel = "sse:cli:" + route.CLISessionID
	case route.WebSessionID != "" && route.CLISessionID == "" && route.UserID == "":
		operatorBindKey := sessionOperatorBindKey(operatorSessionID)
		boundWebSessionID, ok := h.db.KVStore.KVGet(operatorBindKey)
		if !ok || boundWebSessionID != route.WebSessionID {
			h.responder.Error(w, http.StatusForbidden, "not authorized for this web session")
			return
		}
		channel = "sse:web:" + route.WebSessionID
	case route.UserID != "" && route.WebSessionID == "" && route.CLISessionID == "":
		op, err := h.auth.ValidateOperatorSession(operatorSessionID)
		if err != nil || op.UserID != route.UserID {
			h.responder.Error(w, http.StatusForbidden, "not authorized for this user")
			return
		}
		channel = "sse:user:" + route.UserID
	default:
		h.responder.Error(w, http.StatusBadRequest, "exactly one routing target required")
		return
	}
	clientLabel := "operator_session_id=" + operatorSessionID

	// Set SSE Headers
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	origin := r.Header.Get("Origin")
	if origin != "" {
		w.Header().Set("Access-Control-Allow-Origin", origin)
		w.Header().Set("Access-Control-Allow-Credentials", "true")
	} else {
		w.Header().Set("Access-Control-Allow-Origin", "*")
	}
	w.Header().Set("X-Accel-Buffering", "no") // For Nginx

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "Streaming unsupported", http.StatusInternalServerError)
		return
	}

	// Subscribe to real-time events FIRST to avoid missing any during replay
	eventCh := make(chan []byte, 100)
	unregister := h.pubsub.RegisterHandler(channel, func(ch string, data []byte) {
		select {
		case eventCh <- data:
		default:
			h.logger.Warn("SSE Stream: back-pressure dropping event", "channel", channel)
		}
	})
	defer unregister()

	// Replay from DB if sinceID is provided
	if sinceID > 0 {
		rows, err := h.db.SSEStore.SSEEventsListSince(route, sinceID, 1000)
		if err == nil {
			for _, row := range rows {
				fmt.Fprintf(w, "id: %d\nevent: %s\ndata: %s\n\n", row.ID, row.EventType, row.Payload)
			}
			flusher.Flush()
		}
	}

	// Stream from pubsub
	ctx := r.Context()
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	h.logger.Info("SSE Stream: client connected", "channel", channel, "client", clientLabel)

	for {
		select {
		case <-ctx.Done():
			h.logger.Info("SSE Stream: client disconnected", "channel", channel)
			return
		case <-ticker.C:
			fmt.Fprintf(w, ": heartbeat\n\n")
			flusher.Flush()
		case raw := <-eventCh:
			var p internalSSEPushPayload
			if err := json.Unmarshal(raw, &p); err == nil {
				var inner struct {
					Type string `json:"type"`
				}
				_ = json.Unmarshal(p.Event, &inner)
				if inner.Type == "" {
					inner.Type = string(constants.SystemHealthUnknown)
				}

				fmt.Fprintf(w, "event: %s\ndata: %s\n\n", inner.Type, string(raw))
				flusher.Flush()
			}
		}
	}
}
