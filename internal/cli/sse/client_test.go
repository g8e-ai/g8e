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

package sse

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewClient(t *testing.T) {
	t.Run("nil http client gets default", func(t *testing.T) {
		c := NewClient("http://localhost:8080/sse", nil)
		assert.NotNil(t, c)
		assert.NotNil(t, c.client)
	})

	t.Run("provided http client is used", func(t *testing.T) {
		hc := &http.Client{Timeout: 5 * time.Second}
		c := NewClient("http://localhost:8080/sse", hc)
		assert.Same(t, hc, c.client)
	})
}

func TestSetHeader(t *testing.T) {
	c := NewClient("http://localhost:8080/sse", nil)
	c.SetHeader("X-Custom", "value")
	assert.Equal(t, "value", c.headers["X-Custom"])
}

func TestParseSSEStream(t *testing.T) {
	t.Run("dispatches event on blank line", func(t *testing.T) {
		input := "event: passkey.registered\ndata: {\"type\":\"passkey.registered\"}\n\n"
		var events []struct{ event, data string }
		handler := func(eventType, data string) {
			events = append(events, struct{ event, data string }{eventType, data})
		}
		err := parseSSEStream(context.Background(), strings.NewReader(input), handler, nil)
		require.NoError(t, err)
		require.Len(t, events, 1)
		assert.Equal(t, "passkey.registered", events[0].event)
		assert.Equal(t, `{"type":"passkey.registered"}`, events[0].data)
	})

	t.Run("multiple events", func(t *testing.T) {
		input := "event: first\ndata: one\n\nevent: second\ndata: two\n\n"
		var count int
		handler := func(eventType, data string) {
			count++
		}
		err := parseSSEStream(context.Background(), strings.NewReader(input), handler, nil)
		require.NoError(t, err)
		assert.Equal(t, 2, count)
	})

	t.Run("no dispatch for empty data", func(t *testing.T) {
		input := "event: noop\n\n"
		var count int
		handler := func(eventType, data string) {
			count++
		}
		err := parseSSEStream(context.Background(), strings.NewReader(input), handler, nil)
		require.NoError(t, err)
		assert.Equal(t, 0, count)
	})

	t.Run("event without explicit event type", func(t *testing.T) {
		input := "data: just data\n\n"
		var events []struct{ event, data string }
		handler := func(eventType, data string) {
			events = append(events, struct{ event, data string }{eventType, data})
		}
		err := parseSSEStream(context.Background(), strings.NewReader(input), handler, nil)
		require.NoError(t, err)
		require.Len(t, events, 1)
		assert.Equal(t, "", events[0].event)
		assert.Equal(t, "just data", events[0].data)
	})

	t.Run("ignores non-event non-data lines", func(t *testing.T) {
		input := ": this is a comment\nevent: test\ndata: payload\n\n"
		var events []struct{ event, data string }
		handler := func(eventType, data string) {
			events = append(events, struct{ event, data string }{eventType, data})
		}
		err := parseSSEStream(context.Background(), strings.NewReader(input), handler, nil)
		require.NoError(t, err)
		require.Len(t, events, 1)
		assert.Equal(t, "test", events[0].event)
	})
}

func TestConnectOnce(t *testing.T) {
	t.Run("receives events from server", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/event-stream")
			fmt.Fprintf(w, "event: test\ndata: hello\n\n")
			fmt.Fprintf(w, "event: test2\ndata: world\n\n")
		}))
		defer srv.Close()

		c := NewClient(srv.URL, nil)
		var events []struct{ event, data string }
		err := c.ConnectOnce(context.Background(), func(eventType, data string) {
			events = append(events, struct{ event, data string }{eventType, data})
		})
		require.NoError(t, err)
		require.Len(t, events, 2)
		assert.Equal(t, "test", events[0].event)
		assert.Equal(t, "hello", events[0].data)
		assert.Equal(t, "test2", events[1].event)
		assert.Equal(t, "world", events[1].data)
	})

	t.Run("returns error on non-200 status", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusUnauthorized)
		}))
		defer srv.Close()

		c := NewClient(srv.URL, nil)
		err := c.ConnectOnce(context.Background(), func(string, string) {})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "401")
	})

	t.Run("sends custom headers", func(t *testing.T) {
		var gotHeader string
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			gotHeader = r.Header.Get("X-Custom-Header")
			w.Header().Set("Content-Type", "text/event-stream")
			fmt.Fprintf(w, "data: ok\n\n")
		}))
		defer srv.Close()

		c := NewClient(srv.URL, nil)
		c.SetHeader("X-Custom-Header", "custom-value")
		err := c.ConnectOnce(context.Background(), func(string, string) {})
		require.NoError(t, err)
		assert.Equal(t, "custom-value", gotHeader)
	})

	t.Run("returns nil on context cancellation", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/event-stream")
			<-r.Context().Done()
		}))
		defer srv.Close()

		c := NewClient(srv.URL, nil)
		ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
		defer cancel()
		err := c.ConnectOnce(ctx, func(string, string) {})
		require.NoError(t, err)
	})
}

func TestRun(t *testing.T) {
	t.Run("returns immediately on empty URL", func(t *testing.T) {
		c := NewClient("", nil)
		done := make(chan struct{})
		go func() {
			c.Run(context.Background(), func(string, string) {})
			close(done)
		}()
		select {
		case <-done:
		case <-time.After(1 * time.Second):
			t.Fatal("Run with empty URL should return immediately")
		}
	})

	t.Run("returns on context cancellation", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/event-stream")
			<-r.Context().Done()
		}))
		defer srv.Close()

		c := NewClient(srv.URL, nil)
		ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
		defer cancel()
		done := make(chan struct{})
		go func() {
			c.Run(ctx, func(string, string) {})
			close(done)
		}()
		select {
		case <-done:
		case <-time.After(2 * time.Second):
			t.Fatal("Run should return after context cancellation")
		}
	})

	t.Run("reconnects after connection error", func(t *testing.T) {
		var attempt atomic.Int32
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			n := attempt.Add(1)
			if n < 2 {
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
			w.Header().Set("Content-Type", "text/event-stream")
			fmt.Fprintf(w, "event: ready\ndata: ok\n\n")
		}))
		defer srv.Close()

		c := NewClient(srv.URL, &http.Client{Timeout: 2 * time.Second})
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		var gotEvent atomic.Bool
		c.Run(ctx, func(eventType, data string) {
			if eventType == "ready" {
				gotEvent.Store(true)
				cancel()
			}
		})
		assert.True(t, gotEvent.Load(), "should receive event after reconnect")
	})
}

func TestParseSSEStream_IDLine(t *testing.T) {
	t.Run("parses id line and updates client lastEventID", func(t *testing.T) {
		input := "id: 42\ndata: hello\n\n"
		c := NewClient("http://localhost", nil)

		err := parseSSEStream(context.Background(), strings.NewReader(input), func(eventType, data string) {}, c)
		require.NoError(t, err)
		assert.Equal(t, int64(42), c.lastEventID, "lastEventID should be set from id: line")
	})

	t.Run("ignores invalid id value", func(t *testing.T) {
		input := "id: notanumber\ndata: hello\n\n"
		c := NewClient("http://localhost", nil)

		err := parseSSEStream(context.Background(), strings.NewReader(input), func(eventType, data string) {}, c)
		require.NoError(t, err)
		assert.Equal(t, int64(0), c.lastEventID, "lastEventID should remain 0 on invalid id")
	})
}

func TestConnectOnce_SendsLastEventIDHeader(t *testing.T) {
	t.Run("sends Last-Event-ID header when lastEventID is set", func(t *testing.T) {
		var gotLastEventID string
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			gotLastEventID = r.Header.Get("Last-Event-ID")
			w.Header().Set("Content-Type", "text/event-stream")
			fmt.Fprintf(w, "data: ok\n\n")
		}))
		defer srv.Close()

		c := NewClient(srv.URL, nil)
		c.lastEventID = 99
		err := c.ConnectOnce(context.Background(), func(string, string) {})
		require.NoError(t, err)
		assert.Equal(t, "99", gotLastEventID, "Last-Event-ID header should be sent on reconnect")
	})

	t.Run("does not send Last-Event-ID header when lastEventID is zero", func(t *testing.T) {
		var gotLastEventID string
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			gotLastEventID = r.Header.Get("Last-Event-ID")
			w.Header().Set("Content-Type", "text/event-stream")
			fmt.Fprintf(w, "data: ok\n\n")
		}))
		defer srv.Close()

		c := NewClient(srv.URL, nil)
		err := c.ConnectOnce(context.Background(), func(string, string) {})
		require.NoError(t, err)
		assert.Empty(t, gotLastEventID, "Last-Event-ID header should not be sent on first connect")
	})
}
