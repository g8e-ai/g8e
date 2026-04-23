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

package listen

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"sync"

	"github.com/gorilla/websocket"

	"github.com/g8e-ai/g8e/components/g8eo/constants"
)

// PubSubBroker manages WebSocket-based publish/subscribe channels.
type PubSubBroker struct {
	logger *slog.Logger

	mu          sync.RWMutex
	subscribers map[string]map[*wsSubscriber]struct{}
	patterns    map[string]map[*wsSubscriber]struct{}
}

// wsSubscriber represents a single WebSocket connection.
//
// Shutdown is expressed as a single atomic event: shutdown() closes done
// (the sole "closed?" signal) and the underlying websocket, guarded by
// sync.Once so repeated calls from any lifecycle path (writer error, read
// loop exit, broker Close) are safe and coalesced. The send channel is
// deliberately never closed: the writer goroutine exits via <-done, and
// trySend is fully non-blocking, so there is no sender/close race and
// therefore no need to track a separate "send closed" flag.
//
// mu is a narrow lock that exists only to make the drop-oldest
// drain+enqueue sequence atomic with respect to concurrent trySend calls
// on the same subscriber. It does NOT participate in shutdown tracking.
type wsSubscriber struct {
	ws           *websocket.Conn
	send         chan []byte
	done         chan struct{}
	shutdownOnce sync.Once

	mu      sync.Mutex
	dropped uint64 // cumulative back-pressure drops; guarded by mu
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

// NewPubSubBroker creates a new pub/sub broker.
func NewPubSubBroker(logger *slog.Logger) *PubSubBroker {
	return &PubSubBroker{
		logger:      logger,
		subscribers: make(map[string]map[*wsSubscriber]struct{}),
		patterns:    make(map[string]map[*wsSubscriber]struct{}),
	}
}

var wsUpgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

// PubSubMessage is the wire format for pub/sub messages.
type PubSubMessage struct {
	Action  string          `json:"action"`
	Channel string          `json:"channel,omitempty"`
	Data    json.RawMessage `json:"data,omitempty"`
}

// PubSubEvent is sent to subscribers when a message is published.
type PubSubEvent struct {
	Type    string          `json:"type"`
	Channel string          `json:"channel"`
	Pattern string          `json:"pattern,omitempty"`
	Data    json.RawMessage `json:"data"`
}

// Publish sends a message to all subscribers of a channel (exact + pattern matches).
func (b *PubSubBroker) Publish(channel string, data json.RawMessage) int {
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
		event := PubSubEvent{Type: constants.PubSubEventMessage, Channel: channel, Data: data}
		msg, _ := json.Marshal(event)
		for sub := range subs {
			deliveries = append(deliveries, delivery{sub: sub, msg: msg})
		}
	}
	for pattern, subs := range b.patterns {
		if globMatch(pattern, channel) {
			event := PubSubEvent{Type: constants.PubSubEventPMessage, Channel: channel, Pattern: pattern, Data: data}
			msg, _ := json.Marshal(event)
			for sub := range subs {
				deliveries = append(deliveries, delivery{sub: sub, msg: msg})
			}
		}
	}
	b.mu.RUnlock()

	count := 0
	for _, d := range deliveries {
		if b.trySend(d.sub, d.msg) {
			count++
		}
	}
	return count
}

// HandleWebSocket upgrades the HTTP connection and passes it to a new session handler.
func (b *PubSubBroker) HandleWebSocket(w http.ResponseWriter, r *http.Request) {
	ws, err := wsUpgrader.Upgrade(w, r, nil)
	if err != nil {
		b.logger.Warn("WebSocket upgrade failed", "error", err)
		return
	}

	handler := &pubSubSessionHandler{
		broker: b,
		sub: &wsSubscriber{
			ws:   ws,
			send: make(chan []byte, 4096),
			done: make(chan struct{}),
		},
	}
	handler.run()
}

// pubSubSessionHandler manages the lifecycle of a single WebSocket session.
type pubSubSessionHandler struct {
	broker *PubSubBroker
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
			case msg := <-h.sub.send:
				if err := h.sub.ws.WriteMessage(websocket.TextMessage, msg); err != nil {
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

		var msg PubSubMessage
		if err := json.Unmarshal(raw, &msg); err != nil {
			continue
		}

		h.handleAction(msg)
	}

	h.cleanup()
}

func (h *pubSubSessionHandler) handleAction(msg PubSubMessage) {
	switch msg.Action {
	case constants.PubSubActionSubscribe:
		h.broker.subscribe(msg.Channel, h.sub)
		h.broker.sendAck(h.sub, msg.Channel)
	case constants.PubSubActionPSubscribe:
		h.broker.psubscribe(msg.Channel, h.sub)
		h.broker.sendAck(h.sub, msg.Channel)
	case constants.PubSubActionUnsubscribe:
		h.broker.unsubscribe(msg.Channel, h.sub)
	case constants.PubSubActionPublish:
		h.broker.Publish(msg.Channel, msg.Data)
	}
}

func (h *pubSubSessionHandler) cleanup() {
	h.broker.removeSub(h.sub)
	h.sub.shutdown()
}

func (b *PubSubBroker) sendAck(sub *wsSubscriber, channel string) {
	type ack struct {
		Type    string `json:"type"`
		Channel string `json:"channel"`
	}
	msg, _ := json.Marshal(ack{Type: constants.PubSubEventSubscribed, Channel: channel})
	b.trySend(sub, msg)
}

func (b *PubSubBroker) subscribe(channel string, sub *wsSubscriber) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.subscribers[channel] == nil {
		b.subscribers[channel] = make(map[*wsSubscriber]struct{})
	}
	b.subscribers[channel][sub] = struct{}{}
}

func (b *PubSubBroker) psubscribe(pattern string, sub *wsSubscriber) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.patterns[pattern] == nil {
		b.patterns[pattern] = make(map[*wsSubscriber]struct{})
	}
	b.patterns[pattern][sub] = struct{}{}
}

func (b *PubSubBroker) unsubscribe(channel string, sub *wsSubscriber) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if subs, ok := b.subscribers[channel]; ok {
		delete(subs, sub)
		if len(subs) == 0 {
			delete(b.subscribers, channel)
		}
	}
	if subs, ok := b.patterns[channel]; ok {
		delete(subs, sub)
		if len(subs) == 0 {
			delete(b.patterns, channel)
		}
	}
}

func (b *PubSubBroker) removeSub(sub *wsSubscriber) {
	b.mu.Lock()
	defer b.mu.Unlock()

	for ch, subs := range b.subscribers {
		delete(subs, sub)
		if len(subs) == 0 {
			delete(b.subscribers, ch)
		}
	}
	for pat, subs := range b.patterns {
		delete(subs, sub)
		if len(subs) == 0 {
			delete(b.patterns, pat)
		}
	}
}

func (b *PubSubBroker) trySend(sub *wsSubscriber, msg []byte) bool {
	sub.mu.Lock()
	defer sub.mu.Unlock()

	if sub.isDone() {
		return false
	}

	select {
	case sub.send <- msg:
		return true
	default:
		// Back-pressure: drop-oldest policy. Evict one queued frame to make
		// room for the newer one, keeping the subscriber connected. This
		// trades lossy delivery for connection stability under bursts
		// (e.g., large stdout frame followed by rapid heartbeats), which is
		// the correct trade-off for status/heartbeat streams where newer
		// frames supersede older ones.
		//
		// sub.mu is held across the drain+enqueue, so no other trySend can
		// race us on this subscriber. The writer goroutine only receives
		// from sub.send, so capacity can only increase during this window;
		// the second send is guaranteed to succeed.
		select {
		case <-sub.send:
		default:
		}
		sub.dropped++
		dropped := sub.dropped
		remote := ""
		if sub.ws != nil {
			remote = sub.ws.RemoteAddr().String()
		}
		select {
		case sub.send <- msg:
			b.logger.Warn("pubsub back-pressure: dropped oldest queued message",
				"remote", remote,
				"buffer_capacity", cap(sub.send),
				"message_bytes", len(msg),
				"dropped_total", dropped,
			)
			return true
		default:
			// Defensive: enqueue should always succeed after drain under
			// sub.mu. If it does not, something is deeply wrong; surface it
			// rather than silently losing the message.
			b.logger.Error("pubsub back-pressure: enqueue failed after drop-oldest",
				"remote", remote,
				"buffer_capacity", cap(sub.send),
				"dropped_total", dropped,
			)
			return false
		}
	}
}

// Close disconnects all subscribers.
func (b *PubSubBroker) Close() {
	b.mu.Lock()
	// Collect unique subscribers under the lock, then shutdown outside the
	// lock. shutdown() is idempotent via sync.Once, so a subscriber that
	// appears in both maps is torn down exactly once.
	seen := make(map[*wsSubscriber]struct{})
	for _, subs := range b.subscribers {
		for sub := range subs {
			seen[sub] = struct{}{}
		}
	}
	for _, subs := range b.patterns {
		for sub := range subs {
			seen[sub] = struct{}{}
		}
	}
	b.subscribers = make(map[string]map[*wsSubscriber]struct{})
	b.patterns = make(map[string]map[*wsSubscriber]struct{})
	b.mu.Unlock()

	for sub := range seen {
		sub.shutdown()
	}
}

// globMatch matches a Redis-style glob pattern against a string.
// Supports * (any sequence) and ? (any single char).
func globMatch(pattern, str string) bool {
	px, sx := 0, 0
	starPx, starSx := -1, -1

	for sx < len(str) {
		if px < len(pattern) && (pattern[px] == '?' || pattern[px] == str[sx]) {
			px++
			sx++
		} else if px < len(pattern) && pattern[px] == '*' {
			starPx = px
			starSx = sx
			px++
		} else if starPx >= 0 {
			px = starPx + 1
			starSx++
			sx = starSx
		} else {
			return false
		}
	}

	for px < len(pattern) && pattern[px] == '*' {
		px++
	}
	return px == len(pattern)
}
