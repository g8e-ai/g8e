// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package pubsubtest

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewMockOperatorPubSubClient(t *testing.T) {
	t.Run("creates mock client successfully", func(t *testing.T) {
		t.Parallel()
		client := NewMockOperatorPubSubClient()
		require.NotNil(t, client)
		assert.NotNil(t, client.subscribers)
		assert.Equal(t, 0, client.PublishedCount())
	})
}

func TestMockOperatorPubSubClient_Subscribe(t *testing.T) {
	t.Run("subscribes to channel successfully", func(t *testing.T) {
		t.Parallel()
		client := NewMockOperatorPubSubClient()

		ctx := context.Background()
		ch, err := client.Subscribe(ctx, "test-channel")
		require.NoError(t, err)
		require.NotNil(t, ch)
	})

	t.Run("receives injected messages", func(t *testing.T) {
		t.Parallel()
		client := NewMockOperatorPubSubClient()

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		ch, err := client.Subscribe(ctx, "test-channel")
		require.NoError(t, err)

		go func() {
			time.Sleep(10 * time.Millisecond)
			client.InjectMessage("test-channel", []byte("test message"))
		}()

		select {
		case msg := <-ch:
			assert.Equal(t, []byte("test message"), msg)
		case <-time.After(100 * time.Millisecond):
			t.Fatal("did not receive injected message")
		}
	})
}

func TestMockOperatorPubSubClient_Publish(t *testing.T) {
	t.Run("records published message", func(t *testing.T) {
		t.Parallel()
		client := NewMockOperatorPubSubClient()

		ctx := context.Background()
		err := client.Publish(ctx, "test-channel", []byte("test message"))
		require.NoError(t, err)

		assert.Equal(t, 1, client.PublishedCount())
		published := client.Published()
		require.Len(t, published, 1)
		assert.Equal(t, "test-channel", published[0].Channel)
		assert.Equal(t, []byte("test message"), published[0].Data)
	})

	t.Run("fans out to subscribers", func(t *testing.T) {
		t.Parallel()
		client := NewMockOperatorPubSubClient()

		ctx1, cancel1 := context.WithCancel(context.Background())
		defer cancel1()
		ctx2, cancel2 := context.WithCancel(context.Background())
		defer cancel2()

		ch1, err := client.Subscribe(ctx1, "test-channel")
		require.NoError(t, err)
		ch2, err := client.Subscribe(ctx2, "test-channel")
		require.NoError(t, err)

		ctx := context.Background()
		err = client.Publish(ctx, "test-channel", []byte("test message"))
		require.NoError(t, err)

		msg1 := <-ch1
		msg2 := <-ch2
		assert.Equal(t, []byte("test message"), msg1)
		assert.Equal(t, []byte("test message"), msg2)
	})
}

func TestMockOperatorPubSubClient_InjectMessage(t *testing.T) {
	t.Run("injects message to subscribers", func(t *testing.T) {
		t.Parallel()
		client := NewMockOperatorPubSubClient()

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		ch, err := client.Subscribe(ctx, "test-channel")
		require.NoError(t, err)

		client.InjectMessage("test-channel", []byte("injected message"))

		select {
		case msg := <-ch:
			assert.Equal(t, []byte("injected message"), msg)
		case <-time.After(100 * time.Millisecond):
			t.Fatal("did not receive injected message")
		}
	})

	t.Run("does not record injected messages", func(t *testing.T) {
		t.Parallel()
		client := NewMockOperatorPubSubClient()

		client.InjectMessage("test-channel", []byte("injected message"))

		assert.Equal(t, 0, client.PublishedCount())
	})
}

func TestMockOperatorPubSubClient_Published(t *testing.T) {
	t.Run("returns all published messages", func(t *testing.T) {
		t.Parallel()
		client := NewMockOperatorPubSubClient()

		ctx := context.Background()
		client.Publish(ctx, "channel-1", []byte("msg-1"))
		client.Publish(ctx, "channel-2", []byte("msg-2"))
		client.Publish(ctx, "channel-1", []byte("msg-3"))

		published := client.Published()
		assert.Len(t, published, 3)
		assert.Equal(t, "channel-1", published[0].Channel)
		assert.Equal(t, "channel-2", published[1].Channel)
		assert.Equal(t, "channel-1", published[2].Channel)
	})

	t.Run("returns copy of published messages", func(t *testing.T) {
		t.Parallel()
		client := NewMockOperatorPubSubClient()

		ctx := context.Background()
		client.Publish(ctx, "test-channel", []byte("test message"))

		published1 := client.Published()
		published2 := client.Published()

		assert.Equal(t, published1, published2)
		assert.NotSame(t, &published1[0], &published2[0])
	})
}

func TestMockOperatorPubSubClient_PublishedCount(t *testing.T) {
	t.Run("returns zero when no messages published", func(t *testing.T) {
		t.Parallel()
		client := NewMockOperatorPubSubClient()
		assert.Equal(t, 0, client.PublishedCount())
	})

	t.Run("returns count of published messages", func(t *testing.T) {
		t.Parallel()
		client := NewMockOperatorPubSubClient()

		ctx := context.Background()
		client.Publish(ctx, "channel-1", []byte("msg-1"))
		client.Publish(ctx, "channel-2", []byte("msg-2"))

		assert.Equal(t, 2, client.PublishedCount())
	})
}

func TestMockOperatorPubSubClient_LastPublished(t *testing.T) {
	t.Run("returns nil when no messages published", func(t *testing.T) {
		t.Parallel()
		client := NewMockOperatorPubSubClient()
		assert.Nil(t, client.LastPublished())
	})

	t.Run("returns last published message", func(t *testing.T) {
		t.Parallel()
		client := NewMockOperatorPubSubClient()

		ctx := context.Background()
		client.Publish(ctx, "channel-1", []byte("msg-1"))
		client.Publish(ctx, "channel-2", []byte("msg-2"))

		last := client.LastPublished()
		require.NotNil(t, last)
		assert.Equal(t, "channel-2", last.Channel)
		assert.Equal(t, []byte("msg-2"), last.Data)
	})
}

func TestMockOperatorPubSubClient_Reset(t *testing.T) {
	t.Run("clears published messages", func(t *testing.T) {
		t.Parallel()
		client := NewMockOperatorPubSubClient()

		ctx := context.Background()
		client.Publish(ctx, "channel-1", []byte("msg-1"))
		client.Publish(ctx, "channel-2", []byte("msg-2"))

		assert.Equal(t, 2, client.PublishedCount())

		client.Reset()

		assert.Equal(t, 0, client.PublishedCount())
		assert.Nil(t, client.LastPublished())
	})
}

func TestMockOperatorPubSubClient_Close(t *testing.T) {
	t.Run("closes subscriber channels", func(t *testing.T) {
		t.Parallel()
		client := NewMockOperatorPubSubClient()

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		ch, err := client.Subscribe(ctx, "test-channel")
		require.NoError(t, err)

		client.Close()

		_, ok := <-ch
		assert.False(t, ok)
	})

	t.Run("clears subscribers", func(t *testing.T) {
		t.Parallel()
		client := NewMockOperatorPubSubClient()

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		_, err := client.Subscribe(ctx, "test-channel")
		require.NoError(t, err)

		client.Close()

		ctx2, cancel2 := context.WithCancel(context.Background())
		defer cancel2()

		ch2, err := client.Subscribe(ctx2, "test-channel-2")
		require.NoError(t, err)
		require.NotNil(t, ch2)
	})
}
