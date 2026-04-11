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

package testutil

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/g8e-ai/g8e/components/g8eo/constants"
	"github.com/g8e-ai/g8e/components/g8eo/httpclient"
	"github.com/gorilla/websocket"
)

// pubsubEvent is the wire format for g8es pub/sub WebSocket messages
type pubsubEvent struct {
	Type    string          `json:"type"`
	Channel string          `json:"channel"`
	Data    json.RawMessage `json:"data"`
}

// pubsubAction is sent to subscribe/publish via WebSocket
type pubsubAction struct {
	Action  string          `json:"action"`
	Channel string          `json:"channel"`
	Data    json.RawMessage `json:"data,omitempty"`
}

// TestPubSubAvailable checks if the g8ed pub/sub gateway is reachable.
// Fatally fails the test when g8ed is unavailable — all callers are integration
// tests that require a live stack.
func TestPubSubAvailable(t *testing.T) {
	t.Helper()
	wsURL := GetTestG8esDirectURL() + "/ws/pubsub"
	dialer, err := httpclient.WebSocketDialer()
	if err != nil {
		t.Fatalf("g8ed pub/sub TLS setup failed: %v", err)
	}
	ws, _, err := dialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("g8ed pub/sub not available at %s: %v", GetTestG8esDirectURL(), err)
	}
	ws.Close()
}

// SubscribeToChannel subscribes to a g8es pub/sub channel and returns a channel for receiving raw JSON payloads.
// baseURL is accepted for API compatibility but ignored — subscriptions always go to the
// g8es pub/sub endpoint at GetTestG8esDirectURL() using the TLS-aware dialer.
// The subscription runs until the test ends (via t.Cleanup).
func SubscribeToChannel(t *testing.T, _ string, channel string) <-chan []byte {
	t.Helper()

	wsURL := GetTestG8esDirectURL() + "/ws/pubsub"

	dialer, err := httpclient.WebSocketDialer()
	if err != nil {
		t.Fatalf("Failed to build TLS dialer for g8es pub/sub: %v", err)
	}
	ws, _, err := dialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("Failed to connect to g8es pub/sub at %s: %v", wsURL, err)
	}

	subMsg := pubsubAction{Action: constants.PubSubActionSubscribe, Channel: channel}
	subJSON, _ := json.Marshal(subMsg)
	if err := ws.WriteMessage(websocket.TextMessage, subJSON); err != nil {
		ws.Close()
		t.Fatalf("Failed to subscribe to channel %s: %v", channel, err)
	}

	out := make(chan []byte, 64)

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(func() {
		cancel()
		ws.Close()
	})

	go func() {
		defer close(out)
		for {
			_, raw, err := ws.ReadMessage()
			if err != nil {
				return
			}
			var event pubsubEvent
			if err := json.Unmarshal(raw, &event); err != nil {
				continue
			}
			if event.Type != constants.PubSubEventMessage && event.Type != constants.PubSubEventPMessage {
				continue
			}
			select {
			case out <- event.Data:
			case <-ctx.Done():
				return
			}
		}
	}()

	return out
}

// PublishTestMessage publishes a message to a pub/sub channel via the g8ed WebSocket gateway.
// g8ed is the single external entry point — g8es is not directly accessible from outside
// the docker network. baseURL is accepted for API compatibility but ignored; all publishes
// go through GetTestG8esDirectURL() (g8ed:443) which proxies to g8es internally.
func PublishTestMessage(t *testing.T, _ string, channel string, message string) {
	t.Helper()

	wsURL := GetTestG8esDirectURL() + "/ws/pubsub"

	dialer, err := httpclient.WebSocketDialer()
	if err != nil {
		t.Fatalf("Failed to build TLS dialer for pub/sub publish: %v", err)
	}
	ws, _, err := dialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("Failed to connect to g8ed pub/sub for publish on channel %s: %v", channel, err)
	}
	defer ws.Close()

	var dataField json.RawMessage
	if json.Valid([]byte(message)) {
		dataField = json.RawMessage(message)
	} else {
		quoted, _ := json.Marshal(message)
		dataField = json.RawMessage(quoted)
	}

	pubMsg := pubsubAction{Action: constants.PubSubActionPublish, Channel: channel, Data: dataField}
	pubJSON, _ := json.Marshal(pubMsg)
	if err := ws.WriteMessage(websocket.TextMessage, pubJSON); err != nil {
		t.Fatalf("Failed to publish test message to channel %s: %v", channel, err)
	}
}

// WaitForMessage waits for a message on a channel with timeout
func WaitForMessage(t *testing.T, msgChan <-chan []byte, timeout time.Duration) []byte {
	t.Helper()

	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case msg := <-msgChan:
		return msg
	case <-timer.C:
		t.Fatal("Timeout waiting for pub/sub message")
		return nil
	}
}

// AssertMessageReceived asserts that a message is received on a channel within timeout
func AssertMessageReceived(t *testing.T, msgChan <-chan []byte, timeout time.Duration, expectedPattern string) []byte {
	t.Helper()

	payload := WaitForMessage(t, msgChan, timeout)
	if payload == nil {
		t.Fatal("Expected message but got nil")
	}

	if expectedPattern != "" && !strings.Contains(string(payload), expectedPattern) {
		t.Fatalf("Expected message to contain '%s' but got: %s", expectedPattern, string(payload))
	}

	return payload
}

// CreateTestChannel returns a unique channel name for testing
func CreateTestChannel(t *testing.T, prefix string) string {
	t.Helper()
	return fmt.Sprintf("%s:test:%s:%d", prefix, t.Name(), time.Now().UnixNano())
}
