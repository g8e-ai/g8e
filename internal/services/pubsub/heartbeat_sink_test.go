// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package pubsub

import (
	"context"
	"sync/atomic"
	"testing"

	"github.com/g8e-ai/g8e/v2/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHeartbeatService_RegisterSink_FiresOnSendAutomatic(t *testing.T) {
	t.Parallel()

	cfg := testutil.NewTestConfig(t)
	logger := testutil.NewTestLogger()
	svc := NewHeartbeatService(cfg, logger, nil)
	svc.SetContext(context.Background())

	var sinkCalled atomic.Bool
	svc.RegisterSink(func(ctx context.Context) {
		sinkCalled.Store(true)
	})

	err := svc.SendAutomatic()
	require.NoError(t, err)
	assert.True(t, sinkCalled.Load(), "sink should have been called during SendAutomatic")
}

func TestHeartbeatService_RegisterSink_MultipleSinksFireInRegistrationOrder(t *testing.T) {
	t.Parallel()

	cfg := testutil.NewTestConfig(t)
	logger := testutil.NewTestLogger()
	svc := NewHeartbeatService(cfg, logger, nil)
	svc.SetContext(context.Background())

	var order []int
	var mu atomic.Int32
	svc.RegisterSink(func(ctx context.Context) {
		order = append(order, 1)
		mu.Add(1)
	})
	svc.RegisterSink(func(ctx context.Context) {
		order = append(order, 2)
		mu.Add(1)
	})
	svc.RegisterSink(func(ctx context.Context) {
		order = append(order, 3)
		mu.Add(1)
	})

	err := svc.SendAutomatic()
	require.NoError(t, err)
	assert.Equal(t, []int{1, 2, 3}, order)
}

func TestHeartbeatService_SendAutomatic_NoPanicWhenSinkListIsEmpty(t *testing.T) {
	t.Parallel()

	cfg := testutil.NewTestConfig(t)
	logger := testutil.NewTestLogger()
	svc := NewHeartbeatService(cfg, logger, nil)
	svc.SetContext(context.Background())

	err := svc.SendAutomatic()
	require.NoError(t, err)
}

func TestHeartbeatService_RegisterSink_PanickingSinkDoesNotCrashHeartbeatCycle(t *testing.T) {
	t.Parallel()

	cfg := testutil.NewTestConfig(t)
	logger := testutil.NewTestLogger()
	svc := NewHeartbeatService(cfg, logger, nil)
	svc.SetContext(context.Background())

	var afterPanicCalled atomic.Bool
	svc.RegisterSink(func(ctx context.Context) {
		panic("sink explosion")
	})
	svc.RegisterSink(func(ctx context.Context) {
		afterPanicCalled.Store(true)
	})

	err := svc.SendAutomatic()
	require.NoError(t, err, "SendAutomatic should not fail even if a sink panics")
	assert.True(t, afterPanicCalled.Load(), "sink after the panicking sink should still be called")
}

func TestHeartbeatService_UnregisterSink_RemovesSink(t *testing.T) {
	t.Parallel()

	cfg := testutil.NewTestConfig(t)
	logger := testutil.NewTestLogger()
	svc := NewHeartbeatService(cfg, logger, nil)
	svc.SetContext(context.Background())

	var sinkCalled atomic.Bool
	id := svc.RegisterSink(func(ctx context.Context) {
		sinkCalled.Store(true)
	})

	svc.UnregisterSink(id)

	sinkCalled.Store(false)
	err := svc.SendAutomatic()
	require.NoError(t, err)
	assert.False(t, sinkCalled.Load(), "unregistered sink should not be called")
}

func TestHeartbeatService_UnregisterSink_NonexistentIDIsNoOp(t *testing.T) {
	t.Parallel()

	cfg := testutil.NewTestConfig(t)
	logger := testutil.NewTestLogger()
	svc := NewHeartbeatService(cfg, logger, nil)
	svc.SetContext(context.Background())

	var sinkCalled atomic.Bool
	svc.RegisterSink(func(ctx context.Context) {
		sinkCalled.Store(true)
	})

	svc.UnregisterSink(99999)

	sinkCalled.Store(false)
	err := svc.SendAutomatic()
	require.NoError(t, err)
	assert.True(t, sinkCalled.Load(), "registered sink should still be called after unregistering nonexistent ID")
}

func TestOperatorPubSubService_HeartbeatService_ReturnsNonNilAfterConstruction(t *testing.T) {
	t.Parallel()

	cfg := testutil.NewTestConfig(t)
	logger := testutil.NewTestLogger()

	// Construct a minimal OperatorPubSubService with just enough fields
	// to verify the HeartbeatService accessor. We build the struct directly
	// to avoid the full NewOperatorPubSubService dependency graph.
	rs := &OperatorPubSubService{
		config:    cfg,
		logger:    logger,
		heartbeat: NewHeartbeatService(cfg, logger, nil),
	}

	hb := rs.HeartbeatService()
	require.NotNil(t, hb)
}
