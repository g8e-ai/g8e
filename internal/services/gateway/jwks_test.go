// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package gateway

import (
	"context"
	"crypto/rsa"
	"math/big"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestNewJWKSProvider(t *testing.T) {
	t.Run("Creates provider with valid URL", func(t *testing.T) {
		provider := NewJWKSProvider("https://example.com/.well-known/jwks.json")

		assert.NotNil(t, provider)
		assert.Equal(t, "https://example.com/.well-known/jwks.json", provider.url)
		assert.NotNil(t, provider.httpClient)
		assert.NotNil(t, provider.keys)
	})

	t.Run("HTTP client has timeout configured", func(t *testing.T) {
		provider := NewJWKSProvider("https://example.com/.well-known/jwks.json")

		assert.Equal(t, 10*time.Second, provider.httpClient.Timeout)
	})

	t.Run("Keys map is initialized empty", func(t *testing.T) {
		provider := NewJWKSProvider("https://example.com/.well-known/jwks.json")

		assert.Empty(t, provider.keys)
	})

	t.Run("LastFetch is zero time initially", func(t *testing.T) {
		provider := NewJWKSProvider("https://example.com/.well-known/jwks.json")

		assert.True(t, provider.lastFetch.IsZero())
	})
}

func TestJWKSProvider_Concurrency(t *testing.T) {
	t.Run("Concurrent GetKey calls are safe", func(t *testing.T) {
		testKey := &rsa.PublicKey{N: big.NewInt(123), E: 65537}
		provider := &JWKSProvider{
			keys:      map[string]*rsa.PublicKey{"key1": testKey},
			lastFetch: time.Now(),
		}

		ctx := context.Background()
		done := make(chan bool, 10)

		for i := 0; i < 10; i++ {
			go func() {
				_, _ = provider.GetKey(ctx, "key1")
				done <- true
			}()
		}

		for i := 0; i < 10; i++ {
			<-done
		}
	})
}
