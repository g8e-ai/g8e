// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

//go:build integration

package gateway

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseAndVerifyJWT(t *testing.T) {
	privKey, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)

	kid := "test-key-id"
	jwks := &JWKSProvider{
		keys: map[string]*rsa.PublicKey{
			kid: &privKey.PublicKey,
		},
		lastFetch: time.Now(),
	}

	validClaims := map[string]interface{}{
		"sub":       "user123",
		"iss":       "test-issuer",
		"aud":       "test-audience",
		"exp":       time.Now().Add(time.Hour).Unix(),
		"iat":       time.Now().Unix(),
		"tenant_id": "tenant-abc",
		"roles":     []string{"admin"},
	}

	t.Run("Valid token", func(t *testing.T) {
		token := generateTestJWT(t, privKey, kid, nil, validClaims)
		jwt, err := ParseAndVerifyJWT(context.Background(), token, jwks, "roles", "test-issuer", "test-audience")

		require.NoError(t, err)
		assert.Equal(t, "user123", jwt.Claims.Sub)
		assert.Equal(t, "test-issuer", jwt.Claims.Iss)
		assert.Equal(t, "test-audience", jwt.Claims.Aud)
		assert.Equal(t, "tenant-abc", jwt.Claims.TenantID)
		assert.Equal(t, []string{"admin"}, jwt.Roles)
	})

	t.Run("Invalid format", func(t *testing.T) {
		_, err := ParseAndVerifyJWT(context.Background(), "invalid.token", jwks, "roles", "", "")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "invalid JWT format")
	})

	t.Run("Invalid base64 header", func(t *testing.T) {
		token := "!!!." + base64.RawURLEncoding.EncodeToString([]byte("{}")) + ".sig"
		_, err := ParseAndVerifyJWT(context.Background(), token, jwks, "roles", "", "")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "decode header")
	})

	t.Run("Invalid base64 payload", func(t *testing.T) {
		token := base64.RawURLEncoding.EncodeToString([]byte("{}")) + ".!!!.sig"
		_, err := ParseAndVerifyJWT(context.Background(), token, jwks, "roles", "", "")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "decode payload")
	})

	t.Run("Invalid base64 signature", func(t *testing.T) {
		token := base64.RawURLEncoding.EncodeToString([]byte("{}")) + "." + base64.RawURLEncoding.EncodeToString([]byte("{}")) + ".!!!"
		_, err := ParseAndVerifyJWT(context.Background(), token, jwks, "roles", "", "")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "decode signature")
	})

	t.Run("Invalid JSON header", func(t *testing.T) {
		token := base64.RawURLEncoding.EncodeToString([]byte("{invalid")) + "." + base64.RawURLEncoding.EncodeToString([]byte("{}")) + "." + base64.RawURLEncoding.EncodeToString([]byte("sig"))
		_, err := ParseAndVerifyJWT(context.Background(), token, jwks, "roles", "", "")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "unmarshal header")
	})

	t.Run("Unsupported algorithm", func(t *testing.T) {
		header := map[string]interface{}{"alg": "HS256", "kid": kid}
		token := generateTestJWT(t, privKey, kid, header, validClaims)
		_, err := ParseAndVerifyJWT(context.Background(), token, jwks, "roles", "", "")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "unsupported signing algorithm")
	})

	t.Run("Missing kid", func(t *testing.T) {
		header := map[string]interface{}{"alg": "RS256"}
		token := generateTestJWT(t, privKey, "", header, validClaims)
		_, err := ParseAndVerifyJWT(context.Background(), token, jwks, "roles", "", "")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "missing kid")
	})

	t.Run("Invalid JSON payload", func(t *testing.T) {
		header := map[string]interface{}{"alg": "RS256", "kid": kid}
		headerBytes, _ := json.Marshal(header)
		headerB64 := base64.RawURLEncoding.EncodeToString(headerBytes)
		payloadB64 := base64.RawURLEncoding.EncodeToString([]byte("{invalid"))
		token := headerB64 + "." + payloadB64 + ".sig"
		_, err := ParseAndVerifyJWT(context.Background(), token, jwks, "roles", "", "")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "unmarshal payload")
	})

	t.Run("Expired token", func(t *testing.T) {
		claims := map[string]interface{}{
			"exp": time.Now().Add(-time.Hour).Unix(),
		}
		token := generateTestJWT(t, privKey, kid, nil, claims)
		_, err := ParseAndVerifyJWT(context.Background(), token, jwks, "roles", "", "")
		assert.ErrorIs(t, err, constants.ErrExpired)
	})

	t.Run("Not yet valid (nbf)", func(t *testing.T) {
		claims := map[string]interface{}{
			"nbf": time.Now().Add(2 * time.Minute).Unix(),
		}
		token := generateTestJWT(t, privKey, kid, nil, claims)
		_, err := ParseAndVerifyJWT(context.Background(), token, jwks, "roles", "", "")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "token is not yet valid")
	})

	t.Run("Valid within clock skew (nbf)", func(t *testing.T) {
		claims := map[string]interface{}{
			"nbf": time.Now().Add(30 * time.Second).Unix(), // 30s in future, skew is 60s
		}
		token := generateTestJWT(t, privKey, kid, nil, claims)
		_, err := ParseAndVerifyJWT(context.Background(), token, jwks, "roles", "", "")
		assert.NoError(t, err)
	})

	t.Run("Issuer mismatch", func(t *testing.T) {
		token := generateTestJWT(t, privKey, kid, nil, validClaims)
		_, err := ParseAndVerifyJWT(context.Background(), token, jwks, "roles", "wrong-issuer", "")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "token issuer mismatch")
	})

	t.Run("Audience mismatch", func(t *testing.T) {
		token := generateTestJWT(t, privKey, kid, nil, validClaims)
		_, err := ParseAndVerifyJWT(context.Background(), token, jwks, "roles", "", "wrong-audience")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "token audience mismatch")
	})

	t.Run("Signature verification failure", func(t *testing.T) {
		token := generateTestJWT(t, privKey, kid, nil, validClaims)
		parts := strings.Split(token, ".")
		// Tamper with signature
		tamperedSig := base64.RawURLEncoding.EncodeToString([]byte("invalid signature"))
		tamperedToken := parts[0] + "." + parts[1] + "." + tamperedSig

		_, err := ParseAndVerifyJWT(context.Background(), tamperedToken, jwks, "roles", "", "")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "verify signature")
	})
}

func TestParseAndVerifyJWT_KeyNotFound(t *testing.T) {
	// Using httptest for the "Key not found" scenario which triggers a fetch
	privKey, _ := rsa.GenerateKey(rand.Reader, 2048)

	// Server that returns empty JWKS
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"keys": []}`)
	}))
	defer ts.Close()

	jwks := NewJWKSProvider(ts.URL)
	token := generateTestJWT(t, privKey, "unknown-kid", nil, map[string]interface{}{"sub": "test"})

	_, err := ParseAndVerifyJWT(context.Background(), token, jwks, "roles", "", "")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "key not found")
}
