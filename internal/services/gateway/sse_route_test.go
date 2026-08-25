// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package gateway

import (
	"context"
	"testing"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/stretchr/testify/assert"
)

// TestBuildSSERouteFromContext verifies the pure function that constructs an
// SSERoute from auth context values. This is a Tier 1 unit test — no integration
// tag required because buildSSERouteFromContext only reads context.Context.
func TestBuildSSERouteFromContext(t *testing.T) {
	t.Run("UserID + CLISessionID returns route with both set", func(t *testing.T) {
		ctx := context.WithValue(context.Background(), constants.ContextKeyUserID, "user-1")
		ctx = context.WithValue(ctx, constants.ContextKeyCLISessionID, "cli-1")

		route := buildSSERouteFromContext(ctx)

		assert.Equal(t, "user-1", route.UserID)
		assert.Equal(t, "cli-1", route.CLISessionID)
		assert.Empty(t, route.WebSessionID)
	})

	t.Run("UserID + WebSessionID (no CLI) returns route with both set", func(t *testing.T) {
		ctx := context.WithValue(context.Background(), constants.ContextKeyUserID, "user-1")
		ctx = context.WithValue(ctx, constants.ContextKeyWebSessionID, "web-1")

		route := buildSSERouteFromContext(ctx)

		assert.Equal(t, "user-1", route.UserID)
		assert.Equal(t, "web-1", route.WebSessionID)
		assert.Empty(t, route.CLISessionID)
	})

	t.Run("UserID only returns route with UserID and no session", func(t *testing.T) {
		// The caller's validate() will reject this; buildSSERouteFromContext
		// itself does not validate, it only constructs.
		ctx := context.WithValue(context.Background(), constants.ContextKeyUserID, "user-1")

		route := buildSSERouteFromContext(ctx)

		assert.Equal(t, "user-1", route.UserID)
		assert.Empty(t, route.CLISessionID)
		assert.Empty(t, route.WebSessionID)
	})

	t.Run("Empty context returns empty route", func(t *testing.T) {
		route := buildSSERouteFromContext(context.Background())

		assert.Empty(t, route.UserID)
		assert.Empty(t, route.CLISessionID)
		assert.Empty(t, route.WebSessionID)
	})

	t.Run("CLISessionID takes precedence over WebSessionID when both present", func(t *testing.T) {
		ctx := context.WithValue(context.Background(), constants.ContextKeyUserID, "user-1")
		ctx = context.WithValue(ctx, constants.ContextKeyCLISessionID, "cli-1")
		ctx = context.WithValue(ctx, constants.ContextKeyWebSessionID, "web-1")

		route := buildSSERouteFromContext(ctx)

		assert.Equal(t, "user-1", route.UserID)
		assert.Equal(t, "cli-1", route.CLISessionID)
		assert.Empty(t, route.WebSessionID, "CLI session must take precedence over web session")
	})

	t.Run("Non-string context values are ignored", func(t *testing.T) {
		// Defense-in-depth: a malformed context value should not panic and
		// should be treated as absent.
		ctx := context.WithValue(context.Background(), constants.ContextKeyUserID, 12345)

		route := buildSSERouteFromContext(ctx)

		assert.Empty(t, route.UserID)
		assert.Empty(t, route.CLISessionID)
		assert.Empty(t, route.WebSessionID)
	})
}
