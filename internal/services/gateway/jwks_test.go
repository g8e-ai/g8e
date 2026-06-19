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

	"github.com/stretchr/testify/assert"
)

func TestNewJWKSProvider(t *testing.T) {

	t.Run("Creates provider with valid URL", func(t *testing.T) {
		provider := NewJWKSProvider("https://example.com/.well-known/jwks.json")

		assert.NotNil(t, provider)
		assert.Equal(t, "https://example.com/.well-known/jwks.json", provider.url)
		assert.NotNil(t, provider.httpClient)
		assert.NotNil(t, provider.keys)
		assert.NotNil(t, provider.lastFetch)
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
