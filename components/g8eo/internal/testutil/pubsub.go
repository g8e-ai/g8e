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
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/g8e-ai/g8e/components/g8eo/internal/constants"
	"github.com/g8e-ai/g8e/components/g8eo/internal/httpclient"
	"github.com/g8e-ai/g8e/components/g8eo/internal/shared/proto/pubsubv1"
	"github.com/gorilla/websocket"
	"google.golang.org/protobuf/proto"
)

// TestPubSubAvailable checks if the g8ed pub/sub gateway is reachable.
// Fatally fails the test when g8ed is unavailable — all callers are integration
// tests that require a live stack.
func TestPubSubAvailable(t *testing.T) {
	t.Helper()
	wsURL := GetTestOperatorDirectURL() + "/ws/pubsub"
	dialer, err := httpclient.WebSocketDialer()
	if err != nil {
		t.Fatalf("g8ed pub/sub TLS setup failed: %v", err)
	}
	ws, _, err := dialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("g8ed pub/sub not available at %s: %v", GetTestOperatorDirectURL(), err)
	}
	ws.Close()
}

// SubscribeToChannel subscribes to a operator pub/sub channel and returns a channel for receiving raw bytes from the Data field.
// baseURL is accepted for API compatibility but ignored — subscriptions always go to the
// operator pub/sub endpoint at GetTestOperatorDirectURL() using the TLS-aware dialer.
// The subscription runs until the test ends (via t.Cleanup).
func SubscribeToChannel(t *testing.T, _ string, channel string) <-chan []byte {
	t.Helper()

	wsURL := GetTestOperatorDirectURL() + "/ws/pubsub"

	dialer, err := httpclient.WebSocketDialer()
	if err != nil {
		t.Fatalf("Failed to build TLS dialer for operator pub/sub: %v", err)
	}
	ws, _, err := dialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("Failed to connect to operator pub/sub at %s: %v", wsURL, err)
	}

	subMsg := &pubsubv1.PubSubMessage{Action: constants.PubSubActionSubscribe, Channel: channel}
	subBytes, err := proto.Marshal(subMsg)
	if err != nil {
		ws.Close()
		t.Fatalf("Failed to marshal subscribe message: %v", err)
	}

	if err := ws.WriteMessage(websocket.BinaryMessage, subBytes); err != nil {
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
			var event pubsubv1.PubSubEvent
			if err := proto.Unmarshal(raw, &event); err != nil {
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
// g8ed is the single external entry point — operator is not directly accessible from outside
// the docker network. baseURL is accepted for API compatibility but ignored; all publishes
// go through GetTestOperatorDirectURL() (g8ed:443) which proxies to operator internally.
func PublishTestMessage(t *testing.T, _ string, channel string, message string) {
	t.Helper()

	wsURL := GetTestOperatorDirectURL() + "/ws/pubsub"

	dialer, err := httpclient.WebSocketDialer()
	if err != nil {
		t.Fatalf("Failed to build TLS dialer for pub/sub publish: %v", err)
	}
	ws, _, err := dialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("Failed to connect to g8ed pub/sub for publish on channel %s: %v", channel, err)
	}
	defer ws.Close()

	pubMsg := &pubsubv1.PubSubMessage{Action: constants.PubSubActionPublish, Channel: channel, Data: []byte(message)}
	pubBytes, err := proto.Marshal(pubMsg)
	if err != nil {
		t.Fatalf("Failed to marshal publish message: %v", err)
	}

	if err := ws.WriteMessage(websocket.BinaryMessage, pubBytes); err != nil {
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
