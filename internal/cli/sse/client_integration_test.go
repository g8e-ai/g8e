// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

//go:build integration

package sse

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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
