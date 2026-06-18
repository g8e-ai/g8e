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

//go:build integration

package gateway

import (
	"testing"
	"time"

	"github.com/g8e-ai/g8e/internal/models"
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
	auth.userCache.Store(user.ID, &cacheEntry{
		value:     user,
		expiresAt: time.Now().Add(-1 * time.Hour),
	})

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = auth.getCachedUser(user.ID)
	}
}
