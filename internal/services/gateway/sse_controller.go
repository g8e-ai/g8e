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
//                            Auth: dual — mTLS for CLI/operator, web session
//                            cookie for browser. The unified middleware stamps
//                            context with the appropriate identity.
//
// The Gateway refuses to talk about a bare session id - every routing
// target is tagged at the type level so a web_session_id can never be
// mis-delivered as a cli_session_id (or vice versa).
//
// All SSE handlers live on SSEController (sse_controller.go).

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/g8e-ai/g8e/internal/config"
	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/g8e-ai/g8e/internal/marshaler"
	"github.com/g8e-ai/g8e/internal/models"
	"github.com/g8e-ai/g8e/internal/response"
	"github.com/g8e-ai/g8e/protocol"
)

// SSEController handles SSE event push, poll, and stream endpoints.
type SSEController struct {
	cfg       *config.Config
	logger    *slog.Logger
	docStore  *DocumentStoreService
	kvStore   *KVStoreService
	sseStore  *SSEEventService
	pubsub    *GatewayWebSocketHandler
	auth      *AuthService
	responder *response.Writer
	heartbeat time.Duration
}

// newSSEController creates an SSEController with the given dependencies.
// If heartbeat is 0, a 30s default is applied.
func newSSEController(cfg *config.Config, logger *slog.Logger, docStore *DocumentStoreService, kvStore *KVStoreService, sseStore *SSEEventService, pubsub *GatewayWebSocketHandler, auth *AuthService, responder *response.Writer, heartbeat time.Duration) *SSEController {
	if heartbeat == 0 {
		heartbeat = 30 * time.Second
	}
	return &SSEController{
		cfg:       cfg,
		logger:    logger,
		docStore:  docStore,
		kvStore:   kvStore,
		sseStore:  sseStore,
		pubsub:    pubsub,
		auth:      auth,
		responder: responder,
		heartbeat: heartbeat,
	}
}

// @Summary		Push SSE event
// @Description	Appends an event to the SSE event store and publishes it to the pub/sub channel.
// @Description	Requires mTLS app workload identity. Exactly one routing target must be set:
// @Description	web_session_id, cli_session_id, or user_id.
// @Tags			telemetry
// @Accept			json
// @Produce		json
// @Param			payload	body		models.SSEPushPayload	true	"SSE push payload"
// @Success		200		{object}	models.SSEPushResponse
// @Failure		400		{string}	string			"Bad Request"
// @Failure		401		{string}	string			"Unauthorized — mTLS required"
// @Failure		403		{string}	string			"Forbidden — not app workload or unauthorized target"
// @Router			/api/v1/sse/push [post]
func (c *SSEController) handleInternalSSEPush(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		c.responder.Error(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	// Strictly verify that the caller is an app workload via mTLS peer certificate URI SAN
	if r.TLS == nil || len(r.TLS.PeerCertificates) == 0 {
		c.logger.Warn("Unauthorized SSE push attempt: missing mTLS client certificate", "path", r.URL.Path)
		c.responder.Error(w, http.StatusUnauthorized, "mTLS client certificate required")
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
		c.logger.Warn("Unauthorized SSE push attempt: not app workload identity", "path", r.URL.Path, "uris", cert.URIs)
		c.responder.Error(w, http.StatusForbidden, "unauthorized client identity")
		return
	}

	body, err := readRequestBody(r, c.cfg.Gateway.MaxPayloadBytes)
	if err != nil {
		c.responder.Error(w, http.StatusBadRequest, "failed to read body")
		return
	}

	var p models.SSEPushPayload
	if err := json.Unmarshal(body, &p); err != nil {
		c.responder.Error(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	if len(p.Event) == 0 {
		c.responder.Error(w, http.StatusBadRequest, "event field is required")
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

	// Authorization: Enforce producer-to-target ownership.
	// The app identity extracted from the peer certificate must be associated with the target.
	// NOTE: SSEEventsAppend is called AFTER authorization (R15) so that an
	// unauthorized push never durably persists an event that would later be
	// replayed to consumers.
	if route.WebSessionID != "" {
		webBindKey := sessionWebBindKey(route.WebSessionID)
		raw, ok := c.kvStore.KVGet(webBindKey)
		if !ok {
			c.logger.Warn("SSE push: target web session has no bound operators", "web_session_id", route.WebSessionID, "app_id", appID)
			c.responder.Error(w, http.StatusForbidden, "target session not found or not bound")
			return
		}
		var operatorSessionIDs []string
		if err := json.Unmarshal([]byte(raw), &operatorSessionIDs); err != nil {
			c.logger.Error("SSE push: failed to parse web session bindings", "web_session_id", route.WebSessionID, "error", err)
			c.responder.Error(w, http.StatusInternalServerError, "failed to verify session ownership")
			return
		}

		// Check if any bound Operator session is associated with this appID
		authorized := false
		for _, opSessID := range operatorSessionIDs {
			opDoc, err := c.docStore.DocGet(marshaler.CollectionName(constants.CollectionOperators), opSessID)
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
			c.logger.Warn("SSE push: app not authorized for target web session", "app_id", appID, "web_session_id", route.WebSessionID)
			c.responder.Error(w, http.StatusForbidden, "unauthorized for target session")
			return
		}
	} else if route.CLISessionID != "" {
		doc, err := c.docStore.DocGet(marshaler.CollectionName(constants.CollectionCLISessions), route.CLISessionID)
		if err != nil || doc == nil {
			c.logger.Warn("SSE push: target CLI session not found", "cli_session_id", route.CLISessionID, "app_id", appID)
			c.responder.Error(w, http.StatusForbidden, "target session not found")
			return
		}
		var cliSess models.CLISession
		b, err := json.Marshal(doc.Data)
		if err != nil {
			c.logger.Error("SSE push: failed to marshal CLI session", "cli_session_id", route.CLISessionID, "error", err)
			c.responder.Error(w, http.StatusInternalServerError, "failed to verify session ownership")
			return
		}
		if err := json.Unmarshal(b, &cliSess); err != nil {
			c.logger.Error("SSE push: failed to parse CLI session", "cli_session_id", route.CLISessionID, "error", err)
			c.responder.Error(w, http.StatusInternalServerError, "failed to verify session ownership")
			return
		}

		// Verify app owns the Operator session bound to this CLI session
		opDoc, err := c.docStore.DocGet(marshaler.CollectionName(constants.CollectionOperators), cliSess.OperatorSessionID)
		if err != nil || opDoc == nil {
			c.logger.Warn("SSE push: Operator session for CLI session not found", "operator_session_id", cliSess.OperatorSessionID, "cli_session_id", route.CLISessionID)
			c.responder.Error(w, http.StatusForbidden, "operator session not found")
			return
		}

		wid := protocol.NewWorkloadIdentity()
		if !wid.MatchesApp(appID, opDoc.ID) {
			c.logger.Warn("SSE push: app not authorized for target CLI session", "app_id", appID, "cli_session_id", route.CLISessionID)
			c.responder.Error(w, http.StatusForbidden, "unauthorized for target session")
			return
		}
	} else if route.UserID != "" {
		// User-scoped pushes: app must be authorized for AT LEAST ONE session belonging to the user.
		// We check if the app identity corresponds to an Operator owned by this user.
		filters := []models.DocFilter{
			{Field: "user_id", Op: "==", Value: json.RawMessage(fmt.Sprintf("%q", route.UserID))},
		}
		docs, err := c.docStore.DocQuery(marshaler.CollectionName(constants.CollectionOperators), filters, "", 100)
		if err != nil || len(docs) == 0 {
			c.logger.Warn("SSE push: user has no operators", "user_id", route.UserID, "app_id", appID)
			c.responder.Error(w, http.StatusForbidden, "unauthorized for target user")
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
			c.logger.Warn("SSE push: app not authorized for target user", "app_id", appID, "user_id", route.UserID)
			c.responder.Error(w, http.StatusForbidden, "unauthorized for target user")
			return
		}
	}

	// Persist the event AFTER authorization (R15) and wrap the payload in a
	// models.SSEPublishedEvent envelope stamped with the DB row ID (R1). The
	// stream handler unmarshals this envelope to deduplicate live events
	// against replayed rows and to emit an `id:` field on the live path (R2).
	rowID, err := c.sseStore.SSEEventsAppend(route, inner.Type, string(body), appID)
	if err != nil {
		c.logger.Error("SSE push: failed to append event", string(constants.ConnectionStateError), err, "type", inner.Type)
		c.responder.Error(w, http.StatusBadRequest, err.Error())
		return
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
		pubEvent := models.SSEPublishedEvent{ID: rowID, Payload: json.RawMessage(body)}
		envelopeJSON, err := json.Marshal(pubEvent)
		if err != nil {
			c.logger.Error("SSE push: failed to marshal published event", string(constants.ConnectionStateError), err)
			c.responder.Error(w, http.StatusInternalServerError, "failed to publish event")
			return
		}
		c.pubsub.Publish(channel, envelopeJSON)
	}

	c.responder.JSON(w, http.StatusOK, models.SSEPushResponse{
		Success:   true,
		Delivered: 1,
	})
}

// sseAuthError carries an HTTP status and message for SSE authorization failures.
type sseAuthError struct {
	status  int
	message string
}

func (e *sseAuthError) Error() string { return e.message }

// authorizeSSERoute verifies that the authenticated identity (from context) is
// authorized to access the requested SSE routing target. Returns the pub/sub
// channel string on success. The middleware stamps context with either
// ContextKeyOperatorSessionID (mTLS path) or ContextKeyWebSessionID +
// ContextKeyUserID (cookie path); this helper enforces ownership for both.
func (c *SSEController) authorizeSSERoute(route SSERoute, r *http.Request) (string, error) {
	operatorSessionID, _ := r.Context().Value(constants.ContextKeyOperatorSessionID).(string)
	webSessionID, _ := r.Context().Value(constants.ContextKeyWebSessionID).(string)
	userID, _ := r.Context().Value(constants.ContextKeyUserID).(string)
	appID, _ := r.Context().Value(constants.ContextKeyAppID).(string)

	// CLI mTLS auth stamps ContextKeyUserID but not ContextKeyOperatorSessionID.
	// Exclude app certs (which also stamp ContextKeyUserID for delegated user SANs)
	// by requiring ContextKeyAppID to be empty.
	isMTLSAuth := operatorSessionID != "" || (userID != "" && webSessionID == "" && appID == "")
	isCookieAuth := webSessionID != ""

	if !isMTLSAuth && !isCookieAuth {
		return "", &sseAuthError{status: http.StatusUnauthorized, message: "missing auth identity"}
	}

	switch {
	case route.CLISessionID != "" && route.WebSessionID == "" && route.UserID == "":
		doc, err := c.docStore.DocGet(marshaler.CollectionName(constants.CollectionCLISessions), route.CLISessionID)
		if err != nil {
			c.logger.Error("SSE: failed to fetch CLI session", string(constants.ConnectionStateError), err, "cli_session_id", route.CLISessionID)
			return "", &sseAuthError{status: http.StatusInternalServerError, message: "failed to verify cli session"}
		}
		if doc == nil {
			return "", &sseAuthError{status: http.StatusForbidden, message: "cli session not found"}
		}
		var cliSess models.CLISession
		b, err := json.Marshal(doc.ForWire())
		if err != nil {
			return "", &sseAuthError{status: http.StatusInternalServerError, message: "failed to verify cli session"}
		}
		if err := json.Unmarshal(b, &cliSess); err != nil {
			return "", &sseAuthError{status: http.StatusInternalServerError, message: "failed to verify cli session"}
		}
		if isMTLSAuth {
			if operatorSessionID != "" {
				// Operator mTLS auth — check operator session ownership
				if cliSess.OperatorSessionID != operatorSessionID {
					return "", &sseAuthError{status: http.StatusForbidden, message: "operator session does not own this cli session"}
				}
			} else {
				// CLI mTLS auth — check user ownership
				if cliSess.UserID != userID {
					return "", &sseAuthError{status: http.StatusForbidden, message: "user does not own this cli session"}
				}
			}
		} else {
			if cliSess.UserID != userID {
				return "", &sseAuthError{status: http.StatusForbidden, message: "user does not own this cli session"}
			}
		}
		return "sse:cli:" + route.CLISessionID, nil

	case route.WebSessionID != "" && route.CLISessionID == "" && route.UserID == "":
		if isMTLSAuth {
			operatorBindKey := sessionOperatorBindKey(operatorSessionID)
			boundWebSessionID, ok := c.kvStore.KVGet(operatorBindKey)
			if !ok || boundWebSessionID != route.WebSessionID {
				return "", &sseAuthError{status: http.StatusForbidden, message: "operator session does not own this web session"}
			}
		} else {
			if route.WebSessionID != webSessionID {
				return "", &sseAuthError{status: http.StatusForbidden, message: "web session does not match authenticated session"}
			}
		}
		return "sse:web:" + route.WebSessionID, nil

	case route.UserID != "" && route.WebSessionID == "" && route.CLISessionID == "":
		if isMTLSAuth {
			op, err := c.auth.ValidateOperatorSession(operatorSessionID)
			if err != nil {
				return "", &sseAuthError{status: http.StatusUnauthorized, message: "invalid Operator session"}
			}
			if op.UserID != route.UserID {
				return "", &sseAuthError{status: http.StatusForbidden, message: "operator does not belong to this user"}
			}
		} else {
			if route.UserID != userID {
				return "", &sseAuthError{status: http.StatusForbidden, message: "user does not match authenticated user"}
			}
		}
		return "sse:user:" + route.UserID, nil

	default:
		return "", &sseAuthError{status: http.StatusBadRequest, message: "exactly one routing target required"}
	}
}

// @Summary		Poll SSE events
// @Description	Polls stored SSE events since a given ID. Dual auth: mTLS for CLI/operator, web session
// @Description	cookie for browser. Exactly one routing target must be set via query string.
// @Tags			telemetry
// @Produce		json
// @Param			web_session_id	query		string	false	"Web session ID"
// @Param			cli_session_id	query		string	false	"CLI session ID"
// @Param			user_id			query		string	false	"User ID"
// @Param			since_id		query		int		false	"Return events after this ID"
// @Param			limit			query		int		false	"Maximum events to return"
// @Success		200			{object}	models.SSEEventsResponse
// @Failure		400			{string}	string				"Bad Request"
// @Failure		403			{string}	string				"Forbidden — unauthorized"
// @Router			/api/v1/sse/events [get]
func (c *SSEController) handleInternalSSEEvents(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		c.responder.Error(w, http.StatusMethodNotAllowed, "method not allowed")
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

	// Authorization: verify the authenticated identity (from context) has the
	// right to access the requested routing buffer. Without this check, any
	// authenticated client could drain any other client's event buffer.
	_, err := c.authorizeSSERoute(route, r)
	if err != nil {
		if sseErr, ok := err.(*sseAuthError); ok {
			c.responder.Error(w, sseErr.status, sseErr.message)
		} else {
			c.responder.Error(w, http.StatusInternalServerError, err.Error())
		}
		return
	}

	rows, err := c.sseStore.SSEEventsListSince(route, sinceID, limit)
	if err != nil {
		c.responder.Error(w, http.StatusBadRequest, err.Error())
		return
	}
	c.responder.JSON(w, http.StatusOK, models.SSEEventsResponse{
		Events: rows,
		Count:  len(rows),
	})
}

// @Summary		Stream SSE events
// @Description	Streams events via Server-Sent Events (text/event-stream). Dual auth: mTLS for
// @Description	CLI/operator, web session cookie for browser. Supports Last-Event-ID header for
// @Description	reconnection. Exactly one routing target must be set via query string.
// @Tags			telemetry
// @Produce		text/event-stream
// @Param			web_session_id	query		string	false	"Web session ID"
// @Param			cli_session_id	query		string	false	"CLI session ID"
// @Param			user_id			query		string	false	"User ID"
// @Param			since_id		query		int		false	"Replay events after this ID"
// @Success		200			{string}	string			"SSE stream"
// @Failure		400			{string}	string			"Bad Request"
// @Failure		403			{string}	string			"Forbidden — unauthorized"
// @Router			/api/v1/sse/stream [get]
func (c *SSEController) handleInternalSSEStream(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		c.responder.Error(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	q := r.URL.Query()
	route := SSERoute{
		WebSessionID: strings.TrimSpace(q.Get("web_session_id")),
		CLISessionID: strings.TrimSpace(q.Get("cli_session_id")),
		UserID:       strings.TrimSpace(q.Get("user_id")),
	}
	sinceIDStr := q.Get("since_id")
	sinceIDPresent := sinceIDStr != ""
	lastEventIDHeader := r.Header.Get("Last-Event-ID")
	if lastEventIDHeader != "" {
		sinceIDStr = lastEventIDHeader
	}
	sinceID, _ := strconv.ParseInt(sinceIDStr, 10, 64)

	// Authorization: verify the authenticated identity (from context) has the
	// right to access the requested routing buffer. Returns the pub/sub channel.
	channel, err := c.authorizeSSERoute(route, r)
	if err != nil {
		if sseErr, ok := err.(*sseAuthError); ok {
			c.responder.Error(w, sseErr.status, sseErr.message)
		} else {
			c.responder.Error(w, http.StatusInternalServerError, err.Error())
		}
		return
	}

	operatorSessionID, _ := r.Context().Value(constants.ContextKeyOperatorSessionID).(string)
	webSessionID, _ := r.Context().Value(constants.ContextKeyWebSessionID).(string)
	userID, _ := r.Context().Value(constants.ContextKeyUserID).(string)
	var clientLabel string
	if operatorSessionID != "" {
		clientLabel = "operator_session_id=" + operatorSessionID
	} else if webSessionID != "" {
		clientLabel = "web_session_id=" + webSessionID
	} else if userID != "" {
		clientLabel = "cli_user_id=" + userID
	} else {
		clientLabel = "unknown"
	}

	// Set SSE Headers
	w.Header().Set(constants.HeaderContentType, "text/event-stream")
	w.Header().Set(constants.HeaderCacheControl, "no-cache")
	w.Header().Set(constants.HeaderConnection, "keep-alive")
	w.Header().Set(constants.HeaderXAccelBuffering, "no")

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "Streaming unsupported", http.StatusInternalServerError)
		return
	}

	// http.Server.WriteTimeout is an absolute deadline measured from the first
	// response write, not an idle-between-writes timeout. Left in place, it
	// closes the connection 30s into the stream regardless of heartbeats,
	// producing the connect→30s→disconnect→3s→reconnect cycle seen in the
	// operator log. Clear it for this long-lived stream; the heartbeat loop
	// below handles liveness and r.Context() handles cancellation.
	rc := http.NewResponseController(w)
	if err := rc.SetWriteDeadline(time.Time{}); err != nil {
		c.logger.Warn("SSE Stream: failed to clear WriteTimeout, stream may be killed by server deadline", "channel", channel, "error", err)
	}

	// Subscribe to real-time events FIRST to avoid missing any during replay.
	// The pub/sub payload is a models.SSEPublishedEvent JSON envelope carrying
	// the DB row ID, which the live loop uses to deduplicate against replayed
	// rows (R1) and to emit an `id:` field (R2).
	//
	// R5: back-pressure is enforced via dropOldestBuf, unifying semantics with
	// the WebSocket path. Drop-oldest is the correct policy for an append-only
	// event log: the consumer can recover evicted events via DB replay on
	// reconnect, while a dropped newest event would be lost until the next
	// reconnect. The buffer is closed on handler exit so the writer goroutine
	// (this loop) cannot block on a closed channel after unregister.
	buf := newDropOldestBuf(100)
	unregister := c.pubsub.RegisterHandler(channel, func(ch string, data []byte) {
		if ok, dropped := buf.send(data); !ok {
			c.logger.Error("SSE Stream: back-pressure enqueue failed after drop-oldest",
				"channel", channel, "dropped_total", dropped)
		} else if dropped > 0 {
			c.logger.Warn("SSE Stream: back-pressure dropped oldest queued event",
				"channel", channel, "dropped_total", dropped)
		}
	})
	defer func() {
		unregister()
		buf.Close()
	}()

	// lastEmittedID tracks the highest event ID emitted on this connection.
	// It is maintained inside the single HTTP-handler goroutine, so no mutex
	// is required. It seeds the dedup cursor from the replay cursor so that
	// any event appended between RegisterHandler and SSEEventsListSince (the
	// duplicate-delivery race window, F1) is suppressed on the live path.
	var lastEmittedID int64

	// Replay from DB. A replay is requested when:
	//   - Last-Event-ID header is present (replay from that cursor; 0 means
	//     "from the beginning"),
	//   - since_id query param is present and > 0,
	//   - Neither is present (fresh connection; replay full backlog from ID 0).
	// When since_id=0 is explicitly set without Last-Event-ID, skip replay —
	// the client is signaling it wants live events only.
	replayRequested := lastEventIDHeader != "" || !sinceIDPresent || sinceID > 0
	if replayRequested {
		rows, err := c.sseStore.SSEEventsListSince(route, sinceID, 1000)
		if err != nil {
			// R4: log replay failures and emit an error sentinel so the client
			// has a signal that a gap occurred, rather than silently swallowing
			// the error and proceeding to the live loop.
			c.logger.Error("SSE Stream: replay failed", "channel", channel, "error", err)
			sentinel, mErr := json.Marshal(models.SSEErrorEvent{Type: "error", Reason: "replay_failed"})
			if mErr != nil {
				c.logger.Error("SSE Stream: failed to marshal replay-error sentinel", "channel", channel, "error", mErr)
				return
			}
			if _, wErr := fmt.Fprintf(w, "data: %s\n\n", sentinel); wErr != nil {
				c.logger.Info("SSE Stream: write error during replay sentinel, disconnecting", "channel", channel, "error", wErr)
				return
			}
			flusher.Flush()
		} else {
			for _, row := range rows {
				// R3: capture write errors and exit immediately on a broken
				// pipe to avoid goroutine leaks on half-open connections.
				if _, wErr := fmt.Fprintf(w, "id: %d\ndata: %s\n\n", row.ID, row.Payload); wErr != nil {
					c.logger.Info("SSE Stream: write error during replay, disconnecting", "channel", channel, "error", wErr)
					return
				}
				lastEmittedID = row.ID
			}
			flusher.Flush()
			// R6: signal replay truncation. If the replay hit the limit, the
			// client received only the first `limit` rows and there may be
			// more backlog. Emit a sentinel so the client can decide to
			// reconnect with a higher since_id.
			if len(rows) == 1000 {
				truncSentinel, mErr := json.Marshal(models.SSETruncationEvent{Type: "truncated", SinceID: lastEmittedID, Limit: 1000})
				if mErr != nil {
					c.logger.Error("SSE Stream: failed to marshal truncation sentinel", "channel", channel, "error", mErr)
					return
				}
				if _, wErr := fmt.Fprintf(w, "data: %s\n\n", truncSentinel); wErr != nil {
					c.logger.Info("SSE Stream: write error during truncation sentinel, disconnecting", "channel", channel, "error", wErr)
					return
				}
				flusher.Flush()
			}
		}
	}

	// Stream from pubsub
	ctx := r.Context()
	ticker := time.NewTicker(c.heartbeat)
	defer ticker.Stop()

	c.logger.Info("SSE Stream: client connected", "channel", channel, "client", clientLabel)

	for {
		select {
		case <-ctx.Done():
			c.logger.Info("SSE Stream: client disconnected", "channel", channel)
			return
		case <-ticker.C:
			// R3: exit on heartbeat write error (broken pipe / half-open).
			if _, wErr := fmt.Fprintf(w, ": heartbeat\n\n"); wErr != nil {
				c.logger.Info("SSE Stream: write error on heartbeat, disconnecting", "channel", channel, "error", wErr)
				return
			}
			flusher.Flush()
		case raw, ok := <-buf.recv():
			// buf.Close() (on handler exit) closes the channel; exit cleanly.
			if !ok {
				c.logger.Info("SSE Stream: event buffer closed, disconnecting", "channel", channel)
				return
			}
			// R1/R2: unmarshal the SSEPublishedEvent envelope, dedup against
			// lastEmittedID, and emit `id:` + `data:` (no `event:` field — R14).
			var pubEvent models.SSEPublishedEvent
			if err := json.Unmarshal(raw, &pubEvent); err != nil {
				c.logger.Warn("SSE Stream: failed to unmarshal published event", "channel", channel, "error", err)
				continue
			}
			if pubEvent.ID <= lastEmittedID {
				// Already emitted via replay; suppress the duplicate.
				continue
			}
			if _, wErr := fmt.Fprintf(w, "id: %d\ndata: %s\n\n", pubEvent.ID, string(pubEvent.Payload)); wErr != nil {
				c.logger.Info("SSE Stream: write error on live event, disconnecting", "channel", channel, "error", wErr)
				return
			}
			lastEmittedID = pubEvent.ID
			flusher.Flush()
		}
	}
}
