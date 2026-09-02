// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package gateway

import (
	"fmt"
	"log/slog"
	"net/http"
	"path"
	"strings"
	"sync"

	"github.com/gorilla/websocket"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/models"
	"github.com/g8e-ai/g8e/v2/internal/services/governance"
	"github.com/g8e-ai/g8e/v2/internal/services/pubsub"
	"github.com/g8e-ai/g8e/v2/protocol"
	commonv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/common/v1"
	operatorv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/operator/v1"
	pubsubv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/pubsub/v1"
)

// GatewayWebSocketHandler manages WebSocket-based publish/subscribe channels.
type GatewayWebSocketHandler struct {
	logger *slog.Logger

	mu          sync.RWMutex
	subscribers map[string]map[*wsSubscriber]struct{}
	// patternSubscribers maps a glob pattern (e.g. "heartbeat:*") to the
	// subscribers registered via PSUBSCRIBE. Fan-out in Publish matches the
	// published channel against each pattern with path.Match and delivers a
	// pmessage event to every matching subscriber. Keyed by glob pattern,
	// parallel to subscribers.
	patternSubscribers map[string]map[*wsSubscriber]struct{}

	// In-process handlers for governance command processing and SSE streaming
	handlersMu    sync.RWMutex
	handlers      map[string]map[int64]func(string, []byte)
	nextHandlerID int64

	// onHeartbeat is called for every publish to a heartbeat: channel.
	onHeartbeatMu sync.RWMutex
	onHeartbeat   func(channel string, data []byte)

	// stateRootProvider supplies the gateway's current state Merkle root for
	// command intent relay. Nil disables the cmd: relay path (publishes to
	// cmd: are rejected fail-closed when the relay is not configured).
	stateRootProvider governance.StateRootProvider

	// sessionValidator resolves operator session IDs for command intent
	// relay. Nil disables the cmd: relay path.
	sessionValidator operatorSessionValidator

	// posture is the gateway's governance posture, injected into every
	// envelope built by the cmd: relay so the operator reads it
	// per-transaction at L4 verification time.
	posture string

	// receiptRelayDeps are wired by SetReceiptRelayDeps for the receipts:
	// relay path. When set, publishes to receipts: channels from
	// authorized operators are intercepted, the ActionReceipt signature is
	// verified against the operator's actuator public key, the receipt is
	// recorded in the gateway's SQLAuditStore, and the envelope is fanned
	// out to subscribers. When nil (the default), the receipts: relay path
	// is disabled and publishes to receipts: are rejected fail-closed.
	receiptRelayMu       sync.RWMutex
	receiptSignerStore   governance.SignerStore
	receiptAuditRecorder ReceiptRecorder
}

// ReceiptRecorder is the narrow interface wrapping
// RecordActionReceipt(*models.ActionReceiptRecord) error so the pubsub broker
// does not depend directly on *storage.SQLAuditStore. Implemented by
// *storage.SQLAuditStore.
type ReceiptRecorder interface {
	RecordActionReceipt(record *models.ActionReceiptRecord) error
}

// dropOldestBuf is a bounded buffer that enforces drop-oldest back-pressure
// under a mutex. When the buffer is full, send evicts the oldest queued
// message to make room for the newer one, keeping the subscriber connected.
// This trades lossy delivery for connection stability under bursts, which is
// the correct trade-off for an append-only event log where the consumer can
// recover dropped events via DB replay on reconnect (R5).
//
// dropped is the cumulative count of evicted messages; it is returned from
// send so callers can log it. The mutex makes the drain+enqueue sequence
// atomic with respect to concurrent send calls on the same buffer.
type dropOldestBuf struct {
	ch      chan []byte
	mu      sync.Mutex
	dropped int64
}

// newDropOldestBuf creates a dropOldestBuf with the given channel capacity.
func newDropOldestBuf(cap int) *dropOldestBuf {
	return &dropOldestBuf{ch: make(chan []byte, cap)}
}

// send enqueues msg. If the buffer is full, it evicts the oldest queued
// message and increments dropped. Returns (true, dropped) on success and
// (false, dropped) only if the enqueue fails after drain (defensive — should
// not happen under mu since the channel can only drain during this window).
func (b *dropOldestBuf) send(msg []byte) (bool, int64) {
	b.mu.Lock()
	defer b.mu.Unlock()

	select {
	case b.ch <- msg:
		return true, b.dropped
	default:
		// Back-pressure: drop-oldest policy. Evict one queued frame to make
		// room for the newer one. b.mu is held across the drain+enqueue, so
		// no other send can race us. The consumer only receives from b.ch,
		// so capacity can only increase during this window; the second send
		// is guaranteed to succeed.
		select {
		case <-b.ch:
		default:
		}
		b.dropped++
		total := b.dropped
		select {
		case b.ch <- msg:
			return true, total
		default:
			// Defensive: enqueue should always succeed after drain under
			// b.mu. If it does not, something is deeply wrong; surface it
			// rather than silently losing the message.
			return false, total
		}
	}
}

// recv returns the channel for receiving buffered messages. Callers read
// from this channel directly; the buffer does not wrap receive logic.
func (b *dropOldestBuf) recv() <-chan []byte {
	return b.ch
}

// Close closes the underlying channel. After Close, send returns false.
func (b *dropOldestBuf) Close() {
	close(b.ch)
}

// wsSubscriber represents a single WebSocket connection.
//
// Shutdown is expressed as a single atomic event: shutdown() closes done
// (the sole "closed?" signal) and the underlying websocket, guarded by
// sync.Once so repeated calls from any lifecycle path (writer error, read
// loop exit, broker Close) are safe and coalesced. The send buffer is
// deliberately never closed: the writer goroutine exits via <-done, and
// trySend is fully non-blocking, so there is no sender/close race and
// therefore no need to track a separate "send closed" flag.
//
// buf is a dropOldestBuf that enforces drop-oldest back-pressure under a
// per-subscriber mutex (R5). It unifies back-pressure semantics across the
// WebSocket and SSE transport paths.
type wsSubscriber struct {
	ws           *websocket.Conn
	buf          *dropOldestBuf
	done         chan struct{}
	shutdownOnce sync.Once

	// mTLS identity for topic ACL enforcement (Plan §5)
	identitySPIFFEID string // SPIFFE ID from the client certificate's URI SAN
	operatorID       string // Extracted operator_id for channel matching
}

// isDone reports whether shutdown has been initiated.
func (s *wsSubscriber) isDone() bool {
	select {
	case <-s.done:
		return true
	default:
		return false
	}
}

// shutdown atomically tears down the subscriber exactly once: it signals
// done (causing the writer goroutine to exit and future trySends to fail
// fast) and closes the underlying websocket. Safe to call from any
// goroutine and any number of times.
func (s *wsSubscriber) shutdown() {
	s.shutdownOnce.Do(func() {
		close(s.done)
		if s.ws != nil {
			_ = s.ws.Close()
		}
	})
}

// NewGatewayWebSocketHandler creates a new pub/sub broker.
func NewGatewayWebSocketHandler(logger *slog.Logger) *GatewayWebSocketHandler {
	return &GatewayWebSocketHandler{
		logger:             logger,
		subscribers:        make(map[string]map[*wsSubscriber]struct{}),
		patternSubscribers: make(map[string]map[*wsSubscriber]struct{}),
		handlers:           make(map[string]map[int64]func(string, []byte)),
	}
}

// SetCommandRelayDeps wires the StateRootProvider, session validator, and
// gateway posture required for the cmd: command intent relay. When set,
// publishes to cmd: channels from authorized app workloads are intercepted,
// transformed into governed GovernanceEnvelopes with the gateway's state
// root and posture, and fanned out to subscribers. When nil (the default),
// the cmd: relay path is disabled and publishes to cmd: are rejected
// fail-closed. Called once during gateway startup after the auth service
// and state root service are constructed. The posture is injected into
// every relayed envelope so the operator reads it per-transaction at L4
// verification time instead of from out-of-band config.
func (b *GatewayWebSocketHandler) SetCommandRelayDeps(stateRootProvider governance.StateRootProvider, sessionValidator operatorSessionValidator, posture string) {
	b.mu.Lock()
	b.stateRootProvider = stateRootProvider
	b.sessionValidator = sessionValidator
	b.posture = posture
	b.mu.Unlock()
}

// SetReceiptRelayDeps wires the SignerStore and ReceiptRecorder required for
// the receipts: ActionReceipt relay. When set, publishes to receipts:
// channels from authorized operators are intercepted, the ActionReceipt
// signature is verified against the operator's actuator public key (looked up
// via the SignerStore), the receipt is recorded in the gateway's
// SQLAuditStore via the ReceiptRecorder, and the envelope is fanned out to
// subscribers. When nil (the default), the receipts: relay path is disabled
// and publishes to receipts: are rejected fail-closed. Called once during
// gateway startup after the SignerStore and AuditStore are constructed.
func (b *GatewayWebSocketHandler) SetReceiptRelayDeps(signerStore governance.SignerStore, recorder ReceiptRecorder) {
	b.receiptRelayMu.Lock()
	b.receiptSignerStore = signerStore
	b.receiptAuditRecorder = recorder
	b.receiptRelayMu.Unlock()
}

// SetHeartbeatHandler registers a callback invoked for every publish to a
// heartbeat:<op_id>:<session_id> channel. Replaces any prior handler.
func (b *GatewayWebSocketHandler) SetHeartbeatHandler(fn func(channel string, data []byte)) {
	b.onHeartbeatMu.Lock()
	b.onHeartbeat = fn
	b.onHeartbeatMu.Unlock()
}

var wsUpgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

// Publish sends a message to all subscribers of a channel.
func (b *GatewayWebSocketHandler) Publish(channel string, data []byte) int {
	// Snapshot targets and precomputed payloads under RLock, then release the
	// broker lock before invoking trySend. trySend may block briefly on a
	// per-subscriber mutex; doing that under the broker RLock would stall
	// subscribe/unsubscribe/Close for unrelated subscribers.
	type delivery struct {
		sub *wsSubscriber
		msg []byte
	}
	var deliveries []delivery

	b.mu.RLock()
	if subs, ok := b.subscribers[channel]; ok {
		event := &pubsubv1.PubSubEvent{
			Type:    constants.PubSubEventMessage,
			Channel: channel,
			Data:    data,
		}
		msg, err := proto.Marshal(event)
		if err != nil {
			b.logger.Error("pubsub: failed to marshal event", "channel", channel, "error", err)
			b.mu.RUnlock()
			return 0
		}
		for sub := range subs {
			deliveries = append(deliveries, delivery{sub: sub, msg: msg})
		}
	}
	// Pattern fan-out: for each registered glob pattern, match it against
	// the published channel and deliver a pmessage event to every matching
	// pattern subscriber. path.Match implements the same glob semantics as
	// Redis PSUBSCRIBE and the mock gateway's fnmatch: '*' spans the whole
	// tail because channels use ':' (never '/') as the delimiter.
	for pattern, subs := range b.patternSubscribers {
		matched, err := path.Match(pattern, channel)
		if err != nil {
			b.logger.Warn("pubsub: malformed pattern in patternSubscribers, skipping", "pattern", pattern, "error", err)
			continue
		}
		if !matched {
			continue
		}
		event := &pubsubv1.PubSubEvent{
			Type:    constants.PubSubEventPMessage,
			Channel: channel,
			Pattern: pattern,
			Data:    data,
		}
		msg, err := proto.Marshal(event)
		if err != nil {
			b.logger.Error("pubsub: failed to marshal pmessage event", "channel", channel, "pattern", pattern, "error", err)
			continue
		}
		for sub := range subs {
			deliveries = append(deliveries, delivery{sub: sub, msg: msg})
		}
	}
	b.mu.RUnlock()

	count := 0
	for _, d := range deliveries {
		if b.trySend(d.sub, d.msg) {
			count++
		}
	}

	// Dispatch to in-process handlers (governance command processing and SSE streaming)
	b.handlersMu.RLock()
	if hs, ok := b.handlers[channel]; ok {
		for _, handler := range hs {
			handler(channel, data)
			count++
		}
	}
	b.handlersMu.RUnlock()

	// Dispatch heartbeat publishes to the dedicated heartbeat handler.
	if strings.HasPrefix(channel, "heartbeat:") {
		b.onHeartbeatMu.RLock()
		fn := b.onHeartbeat
		b.onHeartbeatMu.RUnlock()
		if fn != nil {
			fn(channel, data)
		}
	}

	return count
}

// RegisterHandler registers an in-process handler for a channel.
// Used for governance command processing in gateway mode and SSE streaming.
// Returns a function that unregisters the handler.
func (b *GatewayWebSocketHandler) RegisterHandler(channel string, handler func(string, []byte)) func() {
	b.handlersMu.Lock()
	defer b.handlersMu.Unlock()

	b.nextHandlerID++
	id := b.nextHandlerID

	if b.handlers[channel] == nil {
		b.handlers[channel] = make(map[int64]func(string, []byte))
	}
	b.handlers[channel][id] = handler
	b.logger.Debug("Registered in-process handler for channel", "channel", channel, "id", id)

	return func() {
		b.handlersMu.Lock()
		defer b.handlersMu.Unlock()
		if hs, ok := b.handlers[channel]; ok {
			delete(hs, id)
			if len(hs) == 0 {
				delete(b.handlers, channel)
			}
			b.logger.Debug("Unregistered in-process handler for channel", "channel", channel, "id", id)
		}
	}
}

// HandleWebSocket upgrades the HTTP connection and passes it to a new session handler.
// Extracts mTLS identity for topic ACL enforcement (Plan §5).
// @Summary		WebSocket pub/sub
// @Description	Upgrades to a WebSocket connection for bidirectional publish/subscribe messaging.
// @Description	Requires mTLS authentication. Topic ACLs enforce that subscribers can only
// @Description	subscribe to channels matching their mTLS workload identity.
// @Tags			pubsub
// @Produce		text/event-stream
// @Success		101	{string}	string	"WebSocket Upgrade"
// @Router			/ws/v1/pubsub [get]
func (b *GatewayWebSocketHandler) HandleWebSocket(w http.ResponseWriter, r *http.Request) {
	ws, err := wsUpgrader.Upgrade(w, r, nil)
	if err != nil {
		b.logger.Error("gateway: websocket upgrade failed", "error", err)
		return
	}

	// Extract mTLS identity for topic ACL enforcement
	identitySPIFFEID, operatorID := extractMTLSIdentity(r)

	handler := &pubSubSessionHandler{
		broker: b,
		sub: &wsSubscriber{
			ws:               ws,
			buf:              newDropOldestBuf(4096),
			done:             make(chan struct{}),
			identitySPIFFEID: identitySPIFFEID,
			operatorID:       operatorID,
		},
	}
	handler.run()
}

// pubSubSessionHandler manages the lifecycle of a single WebSocket session.
type pubSubSessionHandler struct {
	broker *GatewayWebSocketHandler
	sub    *wsSubscriber
}

func (h *pubSubSessionHandler) run() {
	// Writer goroutine: drains send until either shutdown is signalled or a
	// websocket write fails. On any exit path it triggers shutdown (idempotent)
	// and evicts from broker maps so the subscriber cannot linger as a zombie.
	go func() {
		defer h.broker.removeSub(h.sub)
		defer h.sub.shutdown()
		for {
			select {
			case <-h.sub.done:
				return
			case msg := <-h.sub.buf.recv():
				if err := h.sub.ws.WriteMessage(websocket.BinaryMessage, msg); err != nil {
					return
				}
			}
		}
	}()

	// Read loop
	for {
		_, raw, err := h.sub.ws.ReadMessage()
		if err != nil {
			break
		}

		var msg pubsubv1.PubSubMessage
		if err := proto.Unmarshal(raw, &msg); err != nil {
			h.broker.logger.Warn("pubsub: failed to unmarshal message", "error", err)
			continue
		}

		h.handleAction(&msg)
	}

	h.cleanup()
}

func (h *pubSubSessionHandler) handleAction(msg *pubsubv1.PubSubMessage) {
	switch msg.Action {
	case constants.PubSubActionSubscribe:
		// Enforce topic ACL: subscriber can only subscribe to channels matching their identity
		if err := verifyChannelACL(msg.Channel, h.sub.operatorID, h.sub.identitySPIFFEID); err != nil {
			h.broker.logger.Warn("PubSub subscription rejected: ACL violation", "channel", msg.Channel, "error", err.Error())
			return
		}
		h.broker.subscribe(msg.Channel, h.sub)
		if err := h.broker.sendAck(h.sub, msg.Channel); err != nil {
			h.broker.logger.Warn("pubsub: failed to send subscription ack", "channel", msg.Channel, "error", err)
		}
	case constants.PubSubActionPSubscribe:
		// Enforce topic ACL on the pattern before registering it. The
		// wildcard '*' is permitted only at the operator_id segment
		// position; cross-operator patterns are rejected so a subscriber
		// cannot PSUBSCRIBE heartbeat:* and receive every operator's
		// heartbeats.
		if err := verifyPatternACL(msg.Channel, h.sub.operatorID, h.sub.identitySPIFFEID); err != nil {
			h.broker.logger.Warn("PubSub pattern subscription rejected: ACL violation", "pattern", msg.Channel, "error", err.Error())
			return
		}
		h.broker.psubscribe(msg.Channel, h.sub)
		if err := h.broker.sendAck(h.sub, msg.Channel); err != nil {
			h.broker.logger.Warn("pubsub: failed to send psubscribe ack", "pattern", msg.Channel, "error", err)
		}
	case constants.PubSubActionUnsubscribe:
		// The Python client sends UNSUBSCRIBE for both exact and pattern
		// subscriptions; there is no separate PUNSUBSCRIBE action on the
		// wire. Evict from both maps so either kind of subscription is
		// torn down.
		h.broker.unsubscribe(msg.Channel, h.sub)
		h.broker.punsubscribe(msg.Channel, h.sub)
	case constants.PubSubActionPublish:
		h.handlePublish(msg)
	}
}

// handlePublish enforces publish ACLs and, for cmd: channels from
// authorized app workloads, intercepts the command intent payload,
// constructs a governed GovernanceEnvelope with the gateway's state root,
// and fans out the transformed envelope to subscribers. All other
// authorized publishes (heartbeat:, results:) are fanned out verbatim.
// Any ACL violation, decoding failure, or session validation failure is
// fail-closed: the frame is dropped and the error is logged.
func (h *pubSubSessionHandler) handlePublish(msg *pubsubv1.PubSubMessage) {
	if err := verifyPublishACL(msg.Channel, h.sub.operatorID, h.sub.identitySPIFFEID); err != nil {
		h.broker.logger.Warn("PubSub publish rejected: ACL violation",
			"channel", msg.Channel,
			"spiffe_id", h.sub.identitySPIFFEID,
			"operator_id", h.sub.operatorID,
			"error", err.Error())
		return
	}

	// cmd: channels from app workloads are intercepted and transformed
	// into governed GovernanceEnvelopes. The gateway owns envelope
	// construction, state root, and governance proofs; the publisher
	// supplies only the command intent.
	if strings.HasPrefix(msg.Channel, constants.ChannelPrefixCmd+":") {
		h.relayCommandIntent(msg.Channel, msg.Data)
		return
	}

	// receipts: channels from operators are intercepted: the gateway
	// verifies the ActionReceipt signature, records the receipt in its
	// SQLAuditStore, and fans out the envelope to subscribers.
	if strings.HasPrefix(msg.Channel, constants.ChannelPrefixReceipts+":") {
		h.relayActionReceipt(msg.Channel, msg.Data)
		return
	}

	// All other authorized publishes (heartbeat:, results:) are fanned
	// out verbatim. The broker does not transform operator-originated
	// frames.
	h.broker.Publish(msg.Channel, msg.Data)
}

// relayCommandIntent decodes a command intent payload published by an
// authorized app workload to a cmd: channel, constructs a governed
// GovernanceEnvelope with the gateway's current state root, and fans
// out the protojson envelope to subscribers. Fail-closed on any error.
func (h *pubSubSessionHandler) relayCommandIntent(channel string, data []byte) {
	b := h.broker
	b.mu.RLock()
	stateRootProvider := b.stateRootProvider
	sessionValidator := b.sessionValidator
	b.mu.RUnlock()

	if stateRootProvider == nil || sessionValidator == nil {
		b.logger.Warn("PubSub cmd: relay disabled: state root provider or session validator not configured",
			"channel", channel)
		return
	}

	// 1. Decode the command intent payload as protojson into the typed
	// commonv1.CommandIntent. The ensemble publishes a CommandIntent
	// protojson object; raw G8eMessage JSON or any other shape is
	// rejected fail-closed by the unmarshaler.
	intent := &commonv1.CommandIntent{}
	if err := protojson.Unmarshal(data, intent); err != nil {
		b.logger.Warn("PubSub cmd: relay: failed to decode command intent",
			"channel", channel, "error", err.Error())
		return
	}
	if intent.OperatorId == "" || intent.OperatorSessionId == "" {
		b.logger.Warn("PubSub cmd: relay: command intent missing operator identifiers",
			"channel", channel)
		return
	}
	if intent.ActionType == "" {
		b.logger.Warn("PubSub cmd: relay: command intent missing action_type",
			"channel", channel)
		return
	}
	if len(intent.Payload) == 0 {
		b.logger.Warn("PubSub cmd: relay: command intent missing payload",
			"channel", channel)
		return
	}

	// 2. Validate the channel matches the intent's operator identifiers.
	// The channel is cmd:<operator_id>:<operator_session_id>; the intent
	// must target the same operator and session. This prevents an
	// authorized app from publishing to one operator's channel while
	// claiming to target another.
	expectedChannel := pubsub.CmdChannel(intent.OperatorId, intent.OperatorSessionId)
	if channel != expectedChannel {
		b.logger.Warn("PubSub cmd: relay: channel does not match command intent operator identifiers",
			"channel", channel,
			"expected", expectedChannel,
			"intent_operator_id", intent.OperatorId,
			"intent_session_id", intent.OperatorSessionId)
		return
	}

	// 3. Validate the target operator session.
	op, err := sessionValidator.ValidateOperatorSession(intent.OperatorSessionId)
	if err != nil {
		b.logger.Warn("PubSub cmd: relay: operator session validation failed",
			"channel", channel,
			"operator_session_id", intent.OperatorSessionId,
			"error", err.Error())
		return
	}

	// 4. Fetch the gateway's current state root.
	stateRoot, err := stateRootProvider.GetCurrentStateRoot()
	if err != nil {
		b.logger.Warn("PubSub cmd: relay: failed to fetch state root",
			"channel", channel, "error", err.Error())
		return
	}

	// 5. Build the governed GovernanceEnvelope. The acting app identity
	// is extracted from the mTLS certificate's SPIFFE ID; the requestor
	// user identity and application context are supplied by the command
	// intent.
	actingAppID := h.sub.identitySPIFFEID
	env, err := BuildGovernanceEnvelope(BuildEnvelopeParams{
		OperatorID:        op.ID,
		OperatorSessionID: op.OperatorSessionID,
		ActionType:        intent.ActionType,
		Payload:           intent.Payload,
		TargetResource:    intent.TargetResource,
		RequestorUserID:   intent.RequestorUserId,
		ActingAppID:       actingAppID,
		StateMerkleRoot:   stateRoot,
		CaseID:            intent.CaseId,
		InvestigationID:   intent.InvestigationId,
		TaskID:            intent.TaskId,
		WebSessionID:      intent.WebSessionId,
		CliSessionID:      intent.CliSessionId,
		Posture:           b.posture,
	})
	if err != nil {
		b.logger.Warn("PubSub cmd: relay: failed to build governance envelope",
			"channel", channel, "error", err.Error())
		return
	}

	// 6. Marshal as protojson (the canonical wire format).
	wire, err := protojson.Marshal(env)
	if err != nil {
		b.logger.Warn("PubSub cmd: relay: failed to marshal governance envelope",
			"channel", channel, "error", err.Error())
		return
	}

	// 7. Fan out the governed envelope to subscribers on the cmd: channel.
	delivered := b.Publish(channel, wire)
	b.logger.Info("PubSub cmd: relay: transformed and fanned out command intent",
		"channel", channel,
		"transaction_id", env.Id,
		"action_type", env.ActionType,
		"delivered", delivered)
}

// relayActionReceipt decodes a GovernanceEnvelope published by an authorized
// operator to a receipts: channel, verifies the embedded ActionReceipt
// signature against the operator's actuator public key, records the receipt
// in the gateway's SQLAuditStore, and fans out the original envelope to
// subscribers. Fail-closed on any error: a malformed envelope, identity
// mismatch, unknown signer, or invalid signature is dropped and logged.
func (h *pubSubSessionHandler) relayActionReceipt(channel string, data []byte) {
	b := h.broker
	b.receiptRelayMu.RLock()
	signerStore := b.receiptSignerStore
	recorder := b.receiptAuditRecorder
	b.receiptRelayMu.RUnlock()

	if signerStore == nil || recorder == nil {
		b.logger.Warn("PubSub receipts: relay disabled: signer store or recorder not configured",
			"channel", channel)
		return
	}

	// 1. Decode the protojson GovernanceEnvelope from the published data.
	env := &commonv1.GovernanceEnvelope{}
	if err := protojson.Unmarshal(data, env); err != nil {
		b.logger.Warn("PubSub receipts: relay: failed to decode governance envelope",
			"channel", channel, "error", err.Error())
		return
	}

	// 2. Validate the channel matches the envelope's operator identifiers.
	// The channel is receipts:<operator_id>:<operator_session_id>; the
	// envelope must carry the same operator and session. This prevents an
	// operator from publishing a receipt claiming to be from a different
	// operator session.
	expectedChannel := pubsub.ReceiptsChannel(env.OperatorId, env.OperatorSessionId)
	if channel != expectedChannel {
		b.logger.Warn("PubSub receipts: relay: channel does not match envelope operator identifiers",
			"channel", channel,
			"expected", expectedChannel,
			"envelope_operator_id", env.OperatorId,
			"envelope_session_id", env.OperatorSessionId)
		return
	}

	// 3. Unmarshal the envelope payload as an ActionReceipt.
	if len(env.Payload) == 0 {
		b.logger.Warn("PubSub receipts: relay: envelope missing ActionReceipt payload",
			"channel", channel, "transaction_id", env.Id)
		return
	}
	receipt := &operatorv1.ActionReceipt{}
	if err := proto.Unmarshal(env.Payload, receipt); err != nil {
		b.logger.Warn("PubSub receipts: relay: failed to decode ActionReceipt payload",
			"channel", channel, "error", err.Error())
		return
	}

	// 4. Verify the receipt signature. Look up the signer's public key via
	// the SignerStore, canonicalize the receipt, decode the hex signature,
	// and verify with ed25519. Fail-closed if the key is not found or the
	// signature is invalid.
	pubKey, err := signerStore.GetTrustedSigner(receipt.SignerKeyId)
	if err != nil {
		b.logger.Warn("PubSub receipts: relay: failed to look up trusted signer",
			"channel", channel,
			"signer_key_id", receipt.SignerKeyId,
			"error", err.Error())
		return
	}
	if pubKey == nil {
		b.logger.Warn("PubSub receipts: relay: unknown signer key id",
			"channel", channel,
			"signer_key_id", receipt.SignerKeyId)
		return
	}
	if err := governance.VerifyActionReceiptSignature(receipt, pubKey); err != nil {
		b.logger.Warn("PubSub receipts: relay: invalid receipt signature",
			"channel", channel,
			"transaction_id", receipt.TransactionId,
			"signer_key_id", receipt.SignerKeyId,
			"error", err.Error())
		return
	}
	if err := governance.VerifyReceiptPersistenceAttestation(receipt, pubKey); err != nil {
		b.logger.Warn("PubSub receipts: relay: invalid receipt persistence attestation",
			"channel", channel,
			"transaction_id", receipt.TransactionId,
			"signer_key_id", receipt.SignerKeyId,
			"error", err.Error())
		return
	}

	// 5. Build the ActionReceiptRecord via the shared single source of
	// truth, reusing the same construction the operator's L5Actuator uses.
	record := governance.BuildReceiptRecord(env, receipt)

	// 6. Record the receipt in the gateway's SQLAuditStore. Log on error
	// but do not block fan-out: the receipt is still useful to subscribers
	// even if the gateway's local recording fails.
	if err := recorder.RecordActionReceipt(record); err != nil {
		b.logger.Error("PubSub receipts: relay: failed to record receipt in audit store",
			"channel", channel,
			"transaction_id", receipt.TransactionId,
			"error", err.Error())
	} else {
		b.logger.Info("PubSub receipts: relay: recorded operator receipt in gateway audit store",
			"channel", channel,
			"transaction_id", receipt.TransactionId,
			"status", receipt.Status.String())
	}

	// 7. Fan out the original envelope to subscribers on the receipts:
	// channel. The gateway records as-is (preserving the operator's
	// signature) so downstream consumers can independently verify offline.
	delivered := b.Publish(channel, data)
	b.logger.Info("PubSub receipts: relay: verified and fanned out action receipt",
		"channel", channel,
		"transaction_id", receipt.TransactionId,
		"delivered", delivered)
}

func (h *pubSubSessionHandler) cleanup() {
	h.broker.removeSub(h.sub)
	h.sub.shutdown()
}

func (b *GatewayWebSocketHandler) sendAck(sub *wsSubscriber, channel string) error {
	event := &pubsubv1.PubSubEvent{
		Type:    constants.PubSubEventSubscribed,
		Channel: channel,
	}
	msg, err := proto.Marshal(event)
	if err != nil {
		return fmt.Errorf("gateway: failed to marshal ack event: %w", err)
	}
	b.trySend(sub, msg)
	return nil
}

func (b *GatewayWebSocketHandler) subscribe(channel string, sub *wsSubscriber) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.subscribers[channel] == nil {
		b.subscribers[channel] = make(map[*wsSubscriber]struct{})
	}
	b.subscribers[channel][sub] = struct{}{}
}

func (b *GatewayWebSocketHandler) unsubscribe(channel string, sub *wsSubscriber) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if subs, ok := b.subscribers[channel]; ok {
		delete(subs, sub)
		if len(subs) == 0 {
			delete(b.subscribers, channel)
		}
	}
}

// psubscribe registers sub for pattern deliveries. The caller must have
// already enforced verifyPatternACL; this method performs no ACL check.
func (b *GatewayWebSocketHandler) psubscribe(pattern string, sub *wsSubscriber) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.patternSubscribers[pattern] == nil {
		b.patternSubscribers[pattern] = make(map[*wsSubscriber]struct{})
	}
	b.patternSubscribers[pattern][sub] = struct{}{}
}

// punsubscribe removes sub from a single pattern's subscriber set. The
// Python client sends UNSUBSCRIBE for both exact and pattern subscriptions
// (there is no separate PUNSUBSCRIBE action on the wire), so the
// PubSubActionUnsubscribe case calls both unsubscribe and punsubscribe.
func (b *GatewayWebSocketHandler) punsubscribe(pattern string, sub *wsSubscriber) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if subs, ok := b.patternSubscribers[pattern]; ok {
		delete(subs, sub)
		if len(subs) == 0 {
			delete(b.patternSubscribers, pattern)
		}
	}
}

func (b *GatewayWebSocketHandler) removeSub(sub *wsSubscriber) {
	b.mu.Lock()
	defer b.mu.Unlock()

	for ch, subs := range b.subscribers {
		delete(subs, sub)
		if len(subs) == 0 {
			delete(b.subscribers, ch)
		}
	}
	for pattern, subs := range b.patternSubscribers {
		delete(subs, sub)
		if len(subs) == 0 {
			delete(b.patternSubscribers, pattern)
		}
	}
}

func (b *GatewayWebSocketHandler) trySend(sub *wsSubscriber, msg []byte) bool {
	if sub.isDone() {
		return false
	}

	ok, dropped := sub.buf.send(msg)
	if !ok {
		remote := ""
		if sub.ws != nil {
			remote = sub.ws.RemoteAddr().String()
		}
		b.logger.Error("pubsub back-pressure: enqueue failed after drop-oldest",
			"remote", remote,
			"buffer_capacity", cap(sub.buf.ch),
			"dropped_total", dropped,
		)
		return false
	}
	if dropped > 0 {
		remote := ""
		if sub.ws != nil {
			remote = sub.ws.RemoteAddr().String()
		}
		b.logger.Warn("pubsub back-pressure: dropped oldest queued message",
			"remote", remote,
			"buffer_capacity", cap(sub.buf.ch),
			"message_bytes", len(msg),
			"dropped_total", dropped,
		)
	}
	return true
}

// Close disconnects all subscribers.
func (b *GatewayWebSocketHandler) Close() {
	b.mu.Lock()
	// Collect unique subscribers under the lock, then shutdown outside the
	// lock. shutdown() is idempotent via sync.Once.
	seen := make(map[*wsSubscriber]struct{})
	for _, subs := range b.subscribers {
		for sub := range subs {
			seen[sub] = struct{}{}
		}
	}
	for _, subs := range b.patternSubscribers {
		for sub := range subs {
			seen[sub] = struct{}{}
		}
	}
	b.subscribers = make(map[string]map[*wsSubscriber]struct{})
	b.patternSubscribers = make(map[string]map[*wsSubscriber]struct{})
	b.mu.Unlock()

	for sub := range seen {
		sub.shutdown()
	}
}

// extractMTLSIdentity extracts the SPIFFE ID and operator_id from the mTLS certificate.
// Returns empty strings if mTLS is not present or the certificate has no URI SANs.
func extractMTLSIdentity(r *http.Request) (string, string) {
	if r.TLS == nil || len(r.TLS.PeerCertificates) == 0 {
		return "", ""
	}

	cert := r.TLS.PeerCertificates[0]
	if len(cert.URIs) == 0 {
		return "", ""
	}

	spiffeID := cert.URIs[0].String()

	// Extract operator_id from SPIFFE ID
	// Format: spiffe://g8e.local/operator/<org_id>/<operator_id>/<cli_session_id>
	// or spiffe://g8e.local/app/<operator_id>
	var operatorID string
	if strings.HasPrefix(spiffeID, "spiffe://"+protocol.TrustDomain+"/operator/") {
		parts := strings.Split(spiffeID, "/")
		if len(parts) >= 6 {
			operatorID = parts[5] // operator_id is at position 5
		}
	} else if strings.HasPrefix(spiffeID, "spiffe://"+protocol.TrustDomain+"/app/") {
		parts := strings.Split(spiffeID, "/")
		if len(parts) >= 5 {
			operatorID = parts[4] // operator_id is at position 4
		}
	}

	return spiffeID, operatorID
}

// verifyChannelACL enforces topic ACLs (Plan §5).
// Subscribers can only subscribe to channels matching their mTLS workload identity.
// Channel format: results:<operator_id>:<cli_session_id> or heartbeat:<operator_id>
// The ensemble (g8ee) is the centralized event broker that subscribes to result
// channels for any operator to relay command results back to callers, so the
// per-operator ownership check is skipped for it (mirroring the SSE push
// authorization in sse_controller.go).
func verifyChannelACL(channel, operatorID, identitySPIFFEID string) error {
	wid := protocol.NewWorkloadIdentity()
	if wid.IsEnsembleApp(identitySPIFFEID) {
		return nil
	}

	if operatorID == "" {
		// If no operator_id in cert, reject subscription
		return constants.ErrPubSubCertificateMissingOperatorID
	}

	// Check if channel starts with the operator_id
	// Format: results:<operator_id>:... or heartbeat:<operator_id>
	parts := strings.Split(channel, ":")
	if len(parts) < 2 {
		return constants.ErrPubSubInvalidChannelFormat
	}

	// The second part should be the operator_id
	if parts[1] != operatorID {
		return fmt.Errorf("channel operator_id mismatch: channel=%s, cert=%s", parts[1], operatorID)
	}

	return nil
}

// verifyPatternACL enforces topic ACLs for pattern subscriptions
// (PSUBSCRIBE). It mirrors verifyChannelACL but the operator_id segment of
// the pattern may be the wildcard '*' (matching the subscriber's own
// operator_id implicitly) or the subscriber's own operator_id; any other
// concrete operator_id is rejected. This is the security-critical piece:
// without it a subscriber could PSUBSCRIBE heartbeat:* and receive every
// operator's heartbeats. The wildcard is permitted only at the operator_id
// segment position (parts[1]); cross-operator patterns such as
// heartbeat:op-other:* are rejected. The ensemble (g8ee) is the centralized
// event broker and is authorized to subscribe to patterns for any operator,
// so the per-operator check is skipped for it.
func verifyPatternACL(pattern, operatorID, identitySPIFFEID string) error {
	wid := protocol.NewWorkloadIdentity()
	if wid.IsEnsembleApp(identitySPIFFEID) {
		return nil
	}

	if operatorID == "" {
		return constants.ErrPubSubCertificateMissingOperatorID
	}
	parts := strings.Split(pattern, ":")
	if len(parts) < 2 {
		return constants.ErrPubSubInvalidChannelFormat
	}
	if parts[1] != "*" && parts[1] != operatorID {
		return fmt.Errorf("pattern operator_id mismatch: pattern=%s, cert=%s", parts[1], operatorID)
	}
	return nil
}

// verifyPublishACL enforces publish-time authorization for all PUBLISH
// actions. The gateway is the relay and enforcement point for operator
// command dispatch: operators may publish only to their own heartbeat:,
// results:, and receipts: channels, and app workloads
// (spiffe://g8e.local/app/...) may publish command intent to
// cmd:<operator_id>:<session_id> channels. The ensemble (g8ee) is the
// centralized event broker and is authorized to publish to any operator's
// cmd: channel. All unauthorized publish attempts fail closed with
// ErrPubSubPublishUnauthorized.
//
// Channel formats:
//
//	cmd:<operator_id>:<operator_session_id>       App -> Operator (intercepted)
//	results:<operator_id>:<operator_session_id>   Operator -> App (verbatim)
//	heartbeat:<operator_id>:<operator_session_id> Operator -> App (verbatim)
//	receipts:<operator_id>:<operator_session_id>  Operator -> Gateway (intercepted)
func verifyPublishACL(channel, publisherOperatorID, publisherSPIFFEID string) error {
	parts := strings.Split(channel, ":")
	if len(parts) < 2 {
		return constants.ErrPubSubInvalidChannelFormat
	}

	prefix := parts[0]
	wid := protocol.NewWorkloadIdentity()

	switch prefix {
	case constants.ChannelPrefixCmd:
		// Only app workloads may publish command intent to cmd: channels.
		// Operators are explicitly prohibited from publishing to cmd:
		// (they receive commands, they do not issue them to themselves).
		// The ensemble (g8ee) is authorized as the centralized event
		// broker for any operator.
		if wid.IsAppSAN(publisherSPIFFEID) {
			return nil
		}
		return fmt.Errorf("%w: operators cannot publish to cmd: channels", constants.ErrPubSubPublishUnauthorized)

	case constants.ChannelPrefixResults, constants.ChannelPrefixHeartbeat, constants.ChannelPrefixReceipts:
		// Operators may publish only to their own heartbeat:, results:, and
		// receipts: channels. The ensemble (g8ee) is authorized to publish
		// to any operator's results:, heartbeat:, and receipts: channels as
		// the centralized event broker.
		if wid.IsEnsembleApp(publisherSPIFFEID) {
			return nil
		}
		if publisherOperatorID == "" {
			return constants.ErrPubSubCertificateMissingOperatorID
		}
		if len(parts) < 3 {
			return constants.ErrPubSubInvalidChannelFormat
		}
		if parts[1] != publisherOperatorID {
			return fmt.Errorf("%w: channel operator_id mismatch: channel=%s, cert=%s",
				constants.ErrPubSubPublishUnauthorized, channel, publisherOperatorID)
		}
		return nil

	default:
		// Unknown channel prefixes are rejected fail-closed. The broker
		// does not relay publishes to channels it does not recognize.
		return fmt.Errorf("%w: unknown channel prefix %q", constants.ErrPubSubPublishUnauthorized, prefix)
	}
}
