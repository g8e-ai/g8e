// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package pubsub

import (
	"context"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/v2/internal/constants"
)

// TestGatewayEnvProcAdapter_SetTarget_RaceWithProcessEnvelope guards against a
// data race between SetTarget (called by NewGatewayOperatorPubSubService during
// boot to wire the real target) and ProcessEnvelope (called on the request
// path). The adapter's target field is backed by atomic.Pointer: SetTarget
// calls Store and ProcessEnvelope calls Load, which provides the required
// happens-before edge. Under `go test -race` this test passes with the
// atomic.Pointer backing and fails if the field regresses to a raw
// *OperatorPubSubService (which carries no memory ordering semantics).
//
// The reader observes one of two fail-closed states when the target is nil
// (constants.ErrGatewayNotReady) or a no-op target that returns early on empty
// payload (constants.ErrPubSubEmptyPayload); neither path touches shared
// mutable state beyond the atomic cell under test.
func TestGatewayEnvProcAdapter_SetTarget_RaceWithProcessEnvelope(t *testing.T) {
	adapter := &GatewayEnvProcAdapter{}
	target := &OperatorPubSubService{}
	ctx := context.Background()

	var wg sync.WaitGroup
	wg.Add(2)

	// Writer: mimics boot wiring the real target, alternating between the
	// real service and the not-yet-wired (nil) state.
	go func() {
		defer wg.Done()
		for i := 0; i < 2000; i++ {
			adapter.SetTarget(target)
			adapter.SetTarget(nil)
		}
	}()

	// Reader: mimics the request path loading the target and delegating.
	// Empty payload makes the real ProcessEnvelope return early with
	// ErrPubSubEmptyPayload, so the reader never touches target-internal
	// state — the only shared cell under test is the adapter's target.
	go func() {
		defer wg.Done()
		for i := 0; i < 2000; i++ {
			_, err := adapter.ProcessEnvelope(ctx, nil)
			require.Error(t, err, "ProcessEnvelope must always return an error in the race fixture")
		}
	}()

	wg.Wait()
}

// TestGatewaySessionValidatorAdapter_SetTarget_RaceWithValidateSession is the
// SessionValidator analogue of the EnvProc race test above. ValidateSession on
// a zero-value OperatorPubSubService returns (true, nil) without touching
// shared state, so the only cell under test is the adapter's atomic target.
func TestGatewaySessionValidatorAdapter_SetTarget_RaceWithValidateSession(t *testing.T) {
	adapter := &GatewaySessionValidatorAdapter{}
	target := &OperatorPubSubService{}

	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		for i := 0; i < 2000; i++ {
			adapter.SetTarget(target)
			adapter.SetTarget(nil)
		}
	}()

	go func() {
		defer wg.Done()
		for i := 0; i < 2000; i++ {
			_, err := adapter.ValidateSession("session-race")
			// Either ErrGatewayNotReady (nil target) or nil (real target).
			if err != nil {
				require.ErrorIs(t, err, constants.ErrGatewayNotReady, "nil-target error must be ErrGatewayNotReady")
			}
		}
	}()

	wg.Wait()
}
