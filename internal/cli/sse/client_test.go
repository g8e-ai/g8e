// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package sse

import (
	"context"
	"net/http"
	"strings"
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
