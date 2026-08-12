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

// Package sse provides a reusable SSE (Server-Sent Events) client that connects
// to an SSE endpoint, parses frames, and dispatches events to a handler.
// It supports reconnection with backoff and custom headers (e.g., mTLS session IDs).
package sse

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"math"
	"math/rand"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/g8e-ai/g8e/internal/constants"
)

// EventHandler receives parsed SSE events. The eventType is the value from the
// `event:` line (or empty if not set). The data is the accumulated `data:` payload.
type EventHandler func(eventType, data string)

// Client is a reusable SSE client that connects to an SSE endpoint,
// parses frames, and dispatches events to a handler.
type Client struct {
	url         string
	headers     map[string]string
	client      *http.Client
	lastEventID int64
	onConnect   func() // called once per successful HTTP connection (status 200)
}

// NewClient creates an SSE client. The http.Client should be configured with
// mTLS certs, timeouts, etc. by the caller. If httpClient is nil, a default
// client with no timeout is used (suitable for context-controlled cancellation).
func NewClient(url string, httpClient *http.Client) *Client {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 0}
	}
	return &Client{
		url:     url,
		headers: make(map[string]string),
		client:  httpClient,
	}
}

// SetHeader sets a custom header on the SSE request (e.g., X-G8E-CLI-Session-ID).
// Must be called before Run or ConnectOnce. Modifying headers after Run starts
// is not safe for concurrent access.
func (c *Client) SetHeader(key, value string) {
	c.headers[key] = value
}

// SetOnConnect sets a callback invoked once per successful HTTP connection
// (after the response status is verified as 200). Callers can use this to
// signal readiness before performing dependent actions (e.g., opening a
// browser only after the SSE listener is established). Must be called before
// Run or ConnectOnce. The callback must not block.
func (c *Client) SetOnConnect(fn func()) {
	c.onConnect = fn
}

// Run connects to the SSE stream and calls handler for each event.
// Reconnects with exponential backoff and jitter on error. Returns when ctx
// is cancelled.
func (c *Client) Run(ctx context.Context, handler EventHandler) {
	if c.url == "" {
		return
	}

	attempt := 0
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		// R13: reset the backoff attempt counter once a connection is
		// established and delivering events, so a transient drop after a
		// healthy session does not inherit the accumulated backoff. The
		// wrapper runs synchronously inside ConnectOnce (parseSSEStream),
		// so received is read only after ConnectOnce returns — no race.
		received := false
		wrapped := func(eventType, data string) {
			received = true
			handler(eventType, data)
		}

		err := c.ConnectOnce(ctx, wrapped)
		if err == nil {
			return
		}
		if received {
			attempt = 0
		}

		// R13: exponential backoff with jitter, capped at 30s.
		base := time.Duration(math.Pow(2, float64(attempt))) * time.Second
		if base > 30*time.Second {
			base = 30 * time.Second
		}
		jitter := time.Duration(float64(base) * 0.2 * (2*randFloat() - 1))
		backoff := base + jitter
		if backoff < 0 {
			backoff = base
		}
		attempt++

		select {
		case <-ctx.Done():
			return
		case <-time.After(backoff):
		}
	}
}

// ConnectOnce connects and streams events until the connection breaks.
// Does NOT reconnect — caller controls retry. Returns nil on context cancel.
func (c *Client) ConnectOnce(ctx context.Context, handler EventHandler) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.url, nil)
	if err != nil {
		return fmt.Errorf("sse client: build request: %w", err)
	}
	req.Header.Set("Accept", "text/event-stream")
	req.Header.Set("Cache-Control", "no-cache")
	// R12: send Last-Event-ID header on reconnect so the server can replay
	// events received after the last DB-backed replay cursor.
	if c.lastEventID > 0 {
		req.Header.Set(constants.HeaderLastEventID, strconv.FormatInt(c.lastEventID, 10))
	}
	for key, value := range c.headers {
		req.Header.Set(key, value)
	}

	resp, err := c.client.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return nil
		}
		return fmt.Errorf("sse client: connect: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("sse client: SSE returned %d", resp.StatusCode)
	}

	// Signal readiness so callers that depend on the listener being
	// established (e.g., the passkey registrar opening a browser) can
	// proceed. Called once per successful connection, including reconnects.
	if c.onConnect != nil {
		c.onConnect()
	}

	// Close the response body when the context is cancelled to unblock
	// any pending reads in parseSSEStream. Without this, scanner.Scan()
	// can block indefinitely on platforms where the HTTP transport does
	// not immediately close the connection on context cancellation.
	stop := context.AfterFunc(ctx, func() {
		resp.Body.Close()
	})
	defer stop()

	return parseSSEStream(ctx, resp.Body, handler, c)
}

// parseSSEStream reads SSE frames from the reader and dispatches events to
// the handler. It parses id:, event:, and data: lines per the SSE spec. The
// lastEventID field on Client is updated when an id: line is encountered so
// that subsequent reconnects can send Last-Event-ID.
func parseSSEStream(ctx context.Context, r io.Reader, handler EventHandler, c *Client) error {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	var eventType, data string

	for scanner.Scan() {
		line := scanner.Text()

		if line == "" {
			if data != "" {
				handler(eventType, data)
			}
			eventType = ""
			data = ""
			continue
		}

		if strings.HasPrefix(line, "id: ") {
			// R11: parse id: line to track last received event ID.
			idStr := strings.TrimPrefix(line, "id: ")
			if id, err := strconv.ParseInt(idStr, 10, 64); err == nil {
				if c != nil {
					c.lastEventID = id
				}
			}
		} else if strings.HasPrefix(line, "event: ") {
			eventType = strings.TrimPrefix(line, "event: ")
		} else if strings.HasPrefix(line, "data: ") {
			if data != "" {
				data += "\n"
			}
			data += strings.TrimPrefix(line, "data: ")
		}
	}

	if err := scanner.Err(); err != nil {
		if ctx.Err() != nil {
			return nil
		}
		return fmt.Errorf("sse client: scan: %w", err)
	}
	return nil
}

// randFloat returns a pseudo-random float64 in [0, 1). It uses a package-level
// rand source to avoid the global lock contention of math/rand's top-level
// functions.
var (
	randMu  sync.Mutex
	randSrc = rand.NewSource(time.Now().UnixNano())
)

func randFloat() float64 {
	randMu.Lock()
	defer randMu.Unlock()
	return float64(randSrc.Int63()) / float64(1<<63)
}
