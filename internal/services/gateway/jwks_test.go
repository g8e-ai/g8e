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
	"encoding/base64"
	"encoding/json"
	"math/big"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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

func TestJWKSProvider_GetKey(t *testing.T) {
	t.Run("Returns cached key within cache duration", func(t *testing.T) {
		testKey := &rsa.PublicKey{N: big.NewInt(123), E: 65537}
		provider := &JWKSProvider{
			keys:      map[string]*rsa.PublicKey{"key1": testKey},
			lastFetch: time.Now(),
		}

		key, err := provider.GetKey(context.Background(), "key1")

		assert.NoError(t, err)
		assert.Equal(t, testKey, key)
	})

	t.Run("Returns error when key not found after fetch", func(t *testing.T) {
		jwksResponse := JWKS{Keys: []JWK{}}

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(jwksResponse)
		}))
		defer server.Close()

		provider := NewJWKSProvider(server.URL)
		provider.lastFetch = time.Now().Add(-1 * time.Hour)

		key, err := provider.GetKey(context.Background(), "nonexistent")

		assert.Error(t, err)
		assert.Nil(t, key)
		assert.ErrorIs(t, err, constants.ErrJWKSKeyNotFound)
	})

	t.Run("Fetches keys when cache expired", func(t *testing.T) {
		testKey := &rsa.PublicKey{N: big.NewInt(456), E: 65537}
		nBytes := testKey.N.Bytes()
		eBytes := []byte{0x01, 0x00, 0x01}

		jwksResponse := JWKS{
			Keys: []JWK{
				{
					Kty: "RSA",
					Kid: "key1",
					Use: "sig",
					N:   base64.RawURLEncoding.EncodeToString(nBytes),
					E:   base64.RawURLEncoding.EncodeToString(eBytes),
				},
			},
		}

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(jwksResponse)
		}))
		defer server.Close()

		provider := NewJWKSProvider(server.URL)
		provider.lastFetch = time.Now().Add(-1 * time.Hour)

		key, err := provider.GetKey(context.Background(), "key1")

		assert.NoError(t, err)
		assert.Equal(t, testKey.N, key.N)
		assert.Equal(t, testKey.E, key.E)
	})

	t.Run("Returns error when fetch fails", func(t *testing.T) {
		provider := &JWKSProvider{
			keys:      make(map[string]*rsa.PublicKey),
			lastFetch: time.Now().Add(-1 * time.Hour),
			url:       "http://invalid-url-that-does-not-exist.local/jwks",
			httpClient: &http.Client{
				Timeout: 100 * time.Millisecond,
			},
		}

		key, err := provider.GetKey(context.Background(), "key1")

		assert.Error(t, err)
		assert.Nil(t, key)
	})
}

func TestJWKSProvider_fetchKeys(t *testing.T) {
	t.Run("Successfully fetches and parses valid JWKS", func(t *testing.T) {
		testKey := &rsa.PublicKey{N: big.NewInt(789), E: 65537}
		nBytes := testKey.N.Bytes()
		eBytes := []byte{0x01, 0x00, 0x01}

		jwksResponse := JWKS{
			Keys: []JWK{
				{
					Kty: "RSA",
					Kid: "test-key",
					Use: "sig",
					N:   base64.RawURLEncoding.EncodeToString(nBytes),
					E:   base64.RawURLEncoding.EncodeToString(eBytes),
				},
			},
		}

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			assert.Equal(t, http.MethodGet, r.Method)
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(jwksResponse)
		}))
		defer server.Close()

		provider := NewJWKSProvider(server.URL)

		err := provider.fetchKeys(context.Background())

		assert.NoError(t, err)
		assert.Contains(t, provider.keys, "test-key")
	})

	t.Run("Skips non-RSA keys", func(t *testing.T) {
		jwksResponse := JWKS{
			Keys: []JWK{
				{
					Kty: "EC",
					Kid: "ec-key",
					Use: "sig",
				},
			},
		}

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(jwksResponse)
		}))
		defer server.Close()

		provider := NewJWKSProvider(server.URL)

		err := provider.fetchKeys(context.Background())

		assert.NoError(t, err)
		assert.Empty(t, provider.keys)
	})

	t.Run("Skips keys with non-sig use", func(t *testing.T) {
		testKey := &rsa.PublicKey{N: big.NewInt(789), E: 65537}
		nBytes := testKey.N.Bytes()
		eBytes := []byte{0x01, 0x00, 0x01}

		jwksResponse := JWKS{
			Keys: []JWK{
				{
					Kty: "RSA",
					Kid: "enc-key",
					Use: "enc",
					N:   base64.RawURLEncoding.EncodeToString(nBytes),
					E:   base64.RawURLEncoding.EncodeToString(eBytes),
				},
			},
		}

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(jwksResponse)
		}))
		defer server.Close()

		provider := NewJWKSProvider(server.URL)

		err := provider.fetchKeys(context.Background())

		assert.NoError(t, err)
		assert.Empty(t, provider.keys)
	})

	t.Run("Handles invalid base64 in modulus", func(t *testing.T) {
		jwksResponse := JWKS{
			Keys: []JWK{
				{
					Kty: "RSA",
					Kid: "invalid-n",
					Use: "sig",
					N:   "!!!invalid-base64!!!",
					E:   "AQAB",
				},
			},
		}

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(jwksResponse)
		}))
		defer server.Close()

		provider := NewJWKSProvider(server.URL)

		err := provider.fetchKeys(context.Background())

		assert.NoError(t, err)
		assert.Empty(t, provider.keys)
	})

	t.Run("Handles invalid base64 in exponent", func(t *testing.T) {
		testKey := &rsa.PublicKey{N: big.NewInt(789), E: 65537}
		nBytes := testKey.N.Bytes()

		jwksResponse := JWKS{
			Keys: []JWK{
				{
					Kty: "RSA",
					Kid: "invalid-e",
					Use: "sig",
					N:   base64.RawURLEncoding.EncodeToString(nBytes),
					E:   "!!!invalid-base64!!!",
				},
			},
		}

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(jwksResponse)
		}))
		defer server.Close()

		provider := NewJWKSProvider(server.URL)

		err := provider.fetchKeys(context.Background())

		assert.NoError(t, err)
		assert.Empty(t, provider.keys)
	})

	t.Run("Returns error on non-200 status", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
		}))
		defer server.Close()

		provider := NewJWKSProvider(server.URL)

		err := provider.fetchKeys(context.Background())

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "unexpected status code")
	})

	t.Run("Returns error on invalid JSON", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte("invalid json"))
		}))
		defer server.Close()

		provider := NewJWKSProvider(server.URL)

		err := provider.fetchKeys(context.Background())

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to decode response")
	})

	t.Run("Returns error on request creation failure", func(t *testing.T) {
		provider := NewJWKSProvider("://invalid-url")

		err := provider.fetchKeys(context.Background())

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to create request")
	})

	t.Run("Returns error on HTTP request failure", func(t *testing.T) {
		provider := NewJWKSProvider("http://localhost:99999/jwks")

		err := provider.fetchKeys(context.Background())

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to fetch keys")
	})

	t.Run("Updates lastFetch timestamp on success", func(t *testing.T) {
		jwksResponse := JWKS{Keys: []JWK{}}

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(jwksResponse)
		}))
		defer server.Close()

		provider := NewJWKSProvider(server.URL)
		provider.lastFetch = time.Time{}

		err := provider.fetchKeys(context.Background())

		require.NoError(t, err)
		assert.False(t, provider.lastFetch.IsZero())
	})

	t.Run("Replaces keys map on successful fetch", func(t *testing.T) {
		testKey := &rsa.PublicKey{N: big.NewInt(789), E: 65537}
		nBytes := testKey.N.Bytes()
		eBytes := []byte{0x01, 0x00, 0x01}

		jwksResponse := JWKS{
			Keys: []JWK{
				{
					Kty: "RSA",
					Kid: "new-key",
					Use: "sig",
					N:   base64.RawURLEncoding.EncodeToString(nBytes),
					E:   base64.RawURLEncoding.EncodeToString(eBytes),
				},
			},
		}

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(jwksResponse)
		}))
		defer server.Close()

		provider := NewJWKSProvider(server.URL)
		provider.keys = map[string]*rsa.PublicKey{"old-key": testKey}

		err := provider.fetchKeys(context.Background())

		require.NoError(t, err)
		assert.NotContains(t, provider.keys, "old-key")
		assert.Contains(t, provider.keys, "new-key")
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

func TestJWKSProvider_ErrorWrapping(t *testing.T) {
	t.Run("Errors are wrapped with constants", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNotFound)
		}))
		defer server.Close()

		provider := NewJWKSProvider(server.URL)

		err := provider.fetchKeys(context.Background())

		assert.Error(t, err)
		assert.ErrorIs(t, err, constants.ErrJWKSUnexpectedStatus)
	})
}
