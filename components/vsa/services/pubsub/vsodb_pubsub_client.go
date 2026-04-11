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

package pubsub

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/gorilla/websocket"

	"github.com/g8e-ai/g8e/components/vsa/certs"
	"github.com/g8e-ai/g8e/components/vsa/constants"
	"github.com/g8e-ai/g8e/components/vsa/httpclient"
	listen "github.com/g8e-ai/g8e/components/vsa/services/listen"
)

// PubSubClient is the interface implemented by both VSODBPubSubClient and MockVSODBPubSubClient.
// All service fields and function parameters use this interface to allow test doubles.
type PubSubClient interface {
	Subscribe(ctx context.Context, channel string) (<-chan []byte, error)
	Publish(ctx context.Context, channel string, data []byte) error
	Close()
}

// VSODBPubSubClient connects to a VSODB instance's WebSocket pub/sub endpoint.
// It provides Subscribe (receive) and Publish (send) over the channel
// naming convention:
//
//	cmd:{operator_id}:{operator_session_id}       (g8ee → Operator)
//	results:{operator_id}:{operator_session_id}    (Operator → g8ee)
//	heartbeat:{operator_id}:{operator_session_id}  (Operator → g8ee)
type VSODBPubSubClient struct {
	baseURL    string // e.g. "wss://g8e.local"
	logger     *slog.Logger
	tlsConfig  *tls.Config // embedded CA trust; nil falls back to system CAs (plain ws://)
	serverName string      // TLS SNI override when endpoint is a raw IP

	mu     sync.Mutex
	closed bool
	pubWs  *websocket.Conn // persistent WebSocket for publishing
}

// NewVSODBPubSubClient creates a client that connects to a VSODB pub/sub endpoint.
// baseURL must use ws:// or wss:// scheme.
// serverName overrides the TLS SNI hostname; pass an empty string when the
// endpoint is a hostname (no override needed).
func NewVSODBPubSubClient(baseURL, serverName string, logger *slog.Logger) (*VSODBPubSubClient, error) {
	if baseURL == "" {
		return nil, fmt.Errorf("VSODB pub/sub URL is required")
	}

	isSecure := len(baseURL) >= 6 && baseURL[:6] == "wss://"

	// For secure connections, load the embedded CA so the operator trusts
	// the server's self-signed certificate. Plain ws:// (local dev) skips this.
	var tlsCfg *tls.Config
	if isSecure {
		var err error
		tlsCfg, err = certs.GetTLSConfig()
		if err != nil {
			return nil, fmt.Errorf("failed to configure transport security: %w", err)
		}
		if serverName != "" {
			tlsCfg.ServerName = serverName
		}
	}

	return &VSODBPubSubClient{
		baseURL:    baseURL,
		logger:     logger,
		tlsConfig:  tlsCfg,
		serverName: serverName,
	}, nil
}

// Subscribe subscribes to a VSODB pub/sub channel and returns a channel that
// delivers raw JSON payloads. The returned channel is closed when the
// subscription ends (context cancelled or connection lost).
//
// Subscribe blocks until the broker sends the "subscribed" ACK for the
// requested channel before returning. This guarantees that any publish sent
// immediately after Subscribe returns will be delivered to this subscriber —
// there is no window where the broker could g8e a message because the
// subscription was not yet registered.
func (c *VSODBPubSubClient) Subscribe(ctx context.Context, channel string) (<-chan []byte, error) {
	wsURL := c.pubSubWSURL()

	c.logger.Info("Dialing VSODB pub/sub WebSocket",
		"url", wsURL,
		"channel", channel,
		"tls", c.tlsConfig != nil)

	var dialer websocket.Dialer
	if c.tlsConfig != nil {
		dialer = *httpclient.WebSocketDialerWithTLS(c.tlsConfig)
	}
	ws, resp, err := dialer.DialContext(ctx, wsURL, nil)
	if err != nil {
		statusCode := 0
		if resp != nil {
			statusCode = resp.StatusCode
		}
		c.logger.Error("VSODB pub/sub WebSocket dial failed",
			"url", wsURL,
			"error", err,
			"http_status", statusCode,
			"tls_enabled", c.tlsConfig != nil)
		return nil, fmt.Errorf("failed to connect to VSODB pub/sub (http_status=%d): %w", statusCode, err)
	}

	subMsg := listen.PubSubMessage{
		Action:  constants.PubSubActionSubscribe,
		Channel: channel,
	}
	subJSON, _ := json.Marshal(subMsg)
	if err := ws.WriteMessage(websocket.TextMessage, subJSON); err != nil {
		ws.Close()
		return nil, fmt.Errorf("failed to subscribe to channel %s: %w", channel, err)
	}

	// Block until the broker confirms the subscription is registered. Frames
	// that arrive before the ACK (e.g. stale messages from a previous session)
	// are buffered and replayed into the output channel once the goroutine starts.
	var pending []json.RawMessage
	if err := c.waitForSubscribedACK(ctx, ws, channel, &pending); err != nil {
		ws.Close()
		return nil, fmt.Errorf("subscription ACK not received for channel %s: %w", channel, err)
	}

	out := make(chan []byte, 64)

	// Drain any messages that arrived before the ACK into the output channel.
	for _, raw := range pending {
		out <- []byte(raw)
	}

	go func() {
		defer close(out)
		defer ws.Close()

		for {
			select {
			case <-ctx.Done():
				return
			default:
			}

			_, raw, err := ws.ReadMessage()
			if err != nil {
				return
			}

			var event listen.PubSubEvent
			if err := json.Unmarshal(raw, &event); err != nil {
				c.logger.Warn("Failed to parse pub/sub event", "error", err)
				continue
			}

			if event.Type != constants.PubSubEventMessage && event.Type != constants.PubSubEventPMessage {
				continue
			}

			select {
			case out <- []byte(event.Data):
			case <-ctx.Done():
				return
			}
		}
	}()

	return out, nil
}

// waitForSubscribedACK reads frames from ws until it sees a {"type":"subscribed","channel":channel}
// frame. Any data-bearing message frames received before the ACK are appended to pending so
// the caller can replay them. Returns an error if the context is cancelled or the connection
// closes before the ACK arrives.
func (c *VSODBPubSubClient) waitForSubscribedACK(ctx context.Context, ws *websocket.Conn, channel string, pending *[]json.RawMessage) error {
	const ackTimeout = 5 * time.Second
	ws.SetReadDeadline(time.Now().Add(ackTimeout))
	defer ws.SetReadDeadline(time.Time{})

	// Close the WebSocket when the context is cancelled so that ws.ReadMessage
	// unblocks immediately rather than waiting for the ackTimeout to expire.
	stop := make(chan struct{})
	defer close(stop)
	go func() {
		select {
		case <-ctx.Done():
			ws.Close()
		case <-stop:
		}
	}()

	for {
		_, raw, err := ws.ReadMessage()
		if err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			return fmt.Errorf("connection error while waiting for subscribed ACK: %w", err)
		}

		var event listen.PubSubEvent
		if err := json.Unmarshal(raw, &event); err != nil {
			continue
		}

		if event.Type == constants.PubSubEventSubscribed && event.Channel == channel {
			c.logger.Info("VSODB pub/sub subscription confirmed", "channel", channel)
			return nil
		}

		if event.Type == constants.PubSubEventMessage || event.Type == constants.PubSubEventPMessage {
			*pending = append(*pending, event.Data)
		}
	}
}

// pubSubWSURL returns the full WebSocket URL for /ws/pubsub.
func (c *VSODBPubSubClient) pubSubWSURL() string {
	return c.baseURL + "/ws/pubsub"
}

// connectPubWs opens a new persistent publish WebSocket connection.
// Caller must hold c.mu and must only call this when c.pubWs is nil.
func (c *VSODBPubSubClient) connectPubWs() error {
	wsURL := c.pubSubWSURL()

	var dialer websocket.Dialer
	if c.tlsConfig != nil {
		dialer = *httpclient.WebSocketDialerWithTLS(c.tlsConfig)
	}

	ws, _, err := dialer.Dial(wsURL, nil)
	if err != nil {
		return fmt.Errorf("failed to connect publish WebSocket: %w", err)
	}
	c.pubWs = ws
	return nil
}

// Publish sends a message to a VSODB pub/sub channel via the persistent WebSocket.
// This is fire-and-forget: the server fans out to all subscribers.
func (c *VSODBPubSubClient) Publish(ctx context.Context, channel string, data []byte) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.closed {
		return fmt.Errorf("VSODB pub/sub client is closed")
	}

	if c.pubWs == nil {
		if err := c.connectPubWs(); err != nil {
			return err
		}
	}

	msg := listen.PubSubMessage{
		Action:  constants.PubSubActionPublish,
		Channel: channel,
		Data:    json.RawMessage(data),
	}
	msgJSON, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("failed to marshal publish payload: %w", err)
	}

	if err := c.pubWs.WriteMessage(websocket.TextMessage, msgJSON); err != nil {
		c.pubWs.Close()
		c.pubWs = nil
		if err := c.connectPubWs(); err != nil {
			return fmt.Errorf("failed to reconnect publish WebSocket: %w", err)
		}
		if err := c.pubWs.WriteMessage(websocket.TextMessage, msgJSON); err != nil {
			c.pubWs.Close()
			c.pubWs = nil
			return fmt.Errorf("failed to publish to VSODB after reconnect: %w", err)
		}
	}

	return nil
}

// checkTLSConnectivity performs a raw WebSocket dial to verify that TCP and TLS
// are operational. A rejection with an HTTP response means the server replied
// (TCP+TLS healthy). Only a TLS-level error (no HTTP response) is fatal.
func (c *VSODBPubSubClient) checkTLSConnectivity(ctx context.Context) error {
	wsURL := c.pubSubWSURL()

	var dialer websocket.Dialer
	if c.tlsConfig != nil {
		dialer = *httpclient.WebSocketDialerWithTLS(c.tlsConfig)
	}

	ws, resp, err := dialer.DialContext(ctx, wsURL, nil)
	if err == nil {
		// Unexpected: proxy accepted a sessionless connection — close it cleanly
		ws.Close()
		return nil
	}

	// A non-nil HTTP response means the server replied — TCP and TLS are healthy.
	// The specific status code is irrelevant; what matters is that it responded.
	if resp != nil {
		return nil
	}

	// No HTTP response: the error occurred at the transport layer (TLS handshake
	// failure, connection refused, etc.). Propagate so the caller can inspect it.
	return err
}

// Close marks the client as closed and tears down the publish WebSocket.
// Existing subscriptions will drain naturally when their contexts are cancelled.
func (c *VSODBPubSubClient) Close() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.closed = true
	if c.pubWs != nil {
		c.pubWs.Close()
		c.pubWs = nil
	}
}
