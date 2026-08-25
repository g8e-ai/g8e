// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

//go:build integration

package gateway

import (
	"testing"
	"time"

	"github.com/g8e-ai/g8e/v2/internal/models"
)

// BenchmarkCacheGet benchmarks cache retrieval (hit scenario).
func BenchmarkCacheGet(b *testing.B) {
	auth := &AuthService{}
	user := &models.User{ID: "test-user-id"}
	auth.cacheUser(user.ID, user)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = auth.getCachedUser(user.ID)
	}
}

// BenchmarkCacheSet benchmarks cache insertion.
func BenchmarkCacheSet(b *testing.B) {
	auth := &AuthService{}
	user := &models.User{ID: "test-user-id"}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		auth.cacheUser(user.ID, user)
	}
}

// BenchmarkCacheInvalidate benchmarks cache invalidation.
func BenchmarkCacheInvalidate(b *testing.B) {
	auth := &AuthService{}
	user := &models.User{ID: "test-user-id"}
	auth.cacheUser(user.ID, user)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		auth.InvalidateUserCache(user.ID)
		// Re-populate for next iteration
		auth.cacheUser(user.ID, user)
	}
}

// BenchmarkCacheExpiry benchmarks cache expiry check (expired entry).
func BenchmarkCacheExpiry(b *testing.B) {
	auth := &AuthService{}
	user := &models.User{ID: "test-user-id"}
	// Manually set an expired entry
	auth.userCache.Store(user.ID, &cacheEntry[*models.User]{
		value:     user,
		expiresAt: time.Now().Add(-1 * time.Hour),
	})

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = auth.getCachedUser(user.ID)
	}
}
