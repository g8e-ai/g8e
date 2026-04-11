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
type wsSubscriber struct {
	ws      *websocket.Conn
	send    chan []byte
	closed  bool
	closeMu sync.Mutex
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
	b.mu.RLock()
	defer b.mu.RUnlock()

	count := 0

	if subs, ok := b.subscribers[channel]; ok {
		event := PubSubEvent{Type: constants.PubSubEventMessage, Channel: channel, Data: data}
		msg, _ := json.Marshal(event)
		for sub := range subs {
			if b.trySend(sub, msg) {
				count++
			}
		}
	}

	for pattern, subs := range b.patterns {
		if globMatch(pattern, channel) {
			event := PubSubEvent{Type: constants.PubSubEventPMessage, Channel: channel, Pattern: pattern, Data: data}
			msg, _ := json.Marshal(event)
			for sub := range subs {
				if b.trySend(sub, msg) {
					count++
				}
			}
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
	// Start write loop
	go func() {
		for msg := range h.sub.send {
			if err := h.sub.ws.WriteMessage(websocket.TextMessage, msg); err != nil {
				h.broker.removeSub(h.sub)
				return
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
	h.sub.closeMu.Lock()
	if !h.sub.closed {
		h.sub.closed = true
		close(h.sub.send)
	}
	h.sub.closeMu.Unlock()
	h.sub.ws.Close()
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
	sub.closeMu.Lock()
	defer sub.closeMu.Unlock()

	if sub.closed {
		return false
	}

	select {
	case sub.send <- msg:
		return true
	default:
		sub.closed = true
		close(sub.send)
		if sub.ws != nil {
			go sub.ws.Close()
		}
		return false
	}
}

// Close disconnects all subscribers.
func (b *PubSubBroker) Close() {
	b.mu.Lock()
	defer b.mu.Unlock()

	for _, subs := range b.subscribers {
		for sub := range subs {
			sub.closeMu.Lock()
			if !sub.closed {
				sub.closed = true
				close(sub.send)
				if sub.ws != nil {
					sub.ws.Close()
				}
			}
			sub.closeMu.Unlock()
		}
	}
	for _, subs := range b.patterns {
		for sub := range subs {
			sub.closeMu.Lock()
			if !sub.closed {
				sub.closed = true
				close(sub.send)
				if sub.ws != nil {
					sub.ws.Close()
				}
			}
			sub.closeMu.Unlock()
		}
	}

	b.subscribers = make(map[string]map[*wsSubscriber]struct{})
	b.patterns = make(map[string]map[*wsSubscriber]struct{})
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
