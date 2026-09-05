// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package gateway

import (
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// stubJWKSProvider is a stub JWKSProvider for Tier 1 unit tests (no HTTP calls).
// It directly uses the JWKSProvider struct but pre-populates keys to avoid network calls.
func stubJWKSProvider(keys map[string]*rsa.PublicKey) *JWKSProvider {
	return &JWKSProvider{
		keys:      keys,
		lastFetch: time.Now(),
		// Set a dummy HTTP client to prevent nil pointer panic if fetch is triggered
		httpClient: &http.Client{},
	}
}

// generateTestJWT creates a test JWT with the given private key, key ID, header, and claims.
// This helper function is used for Tier 1 unit tests to generate valid JWT tokens
// without requiring external dependencies.
func generateTestJWT(t *testing.T, privKey *rsa.PrivateKey, kid string, header map[string]interface{}, claims map[string]interface{}) string {
	t.Helper()
	if header == nil {
		header = map[string]interface{}{
			"alg": "RS256",
			"kid": kid,
			"typ": "JWT",
		}
	}

	headerBytes, err := json.Marshal(header)
	require.NoError(t, err)
	claimsBytes, err := json.Marshal(claims)
	require.NoError(t, err)

	headerB64 := base64.RawURLEncoding.EncodeToString(headerBytes)
	claimsB64 := base64.RawURLEncoding.EncodeToString(claimsBytes)

	signingString := headerB64 + "." + claimsB64
	hasher := sha256.New()
	hasher.Write([]byte(signingString))
	hashed := hasher.Sum(nil)

	sigBytes, err := rsa.SignPKCS1v15(rand.Reader, privKey, crypto.SHA256, hashed)
	require.NoError(t, err)

	sigB64 := base64.RawURLEncoding.EncodeToString(sigBytes)
	return signingString + "." + sigB64
}

func TestExtractRoles(t *testing.T) {
	tests := []struct {
		name      string
		payload   map[string]interface{}
		roleClaim string
		expected  []string
	}{
		{
			name: "Array of strings",
			payload: map[string]interface{}{
				"roles": []string{"admin", "user"},
			},
			roleClaim: "roles",
			expected:  []string{"admin", "user"},
		},
		{
			name: "Single string",
			payload: map[string]interface{}{
				"role": "admin",
			},
			roleClaim: "role",
			expected:  []string{"admin"},
		},
		{
			name: "Claim missing",
			payload: map[string]interface{}{
				"other": "value",
			},
			roleClaim: "roles",
			expected:  nil,
		},
		{
			name: "Wrong type (int)",
			payload: map[string]interface{}{
				"roles": 123,
			},
			roleClaim: "roles",
			expected:  nil,
		},
		{
			name: "Empty array",
			payload: map[string]interface{}{
				"roles": []string{},
			},
			roleClaim: "roles",
			expected:  []string{},
		},
		{
			name: "Multiple roles",
			payload: map[string]interface{}{
				"roles": []string{"read", "write", "admin", "superuser"},
			},
			roleClaim: "roles",
			expected:  []string{"read", "write", "admin", "superuser"},
		},
		{
			name: "Wrong type (bool)",
			payload: map[string]interface{}{
				"roles": true,
			},
			roleClaim: "roles",
			expected:  nil,
		},
		{
			name: "Wrong type (object)",
			payload: map[string]interface{}{
				"roles": map[string]string{"key": "value"},
			},
			roleClaim: "roles",
			expected:  nil,
		},
		{
			name: "Null value",
			payload: map[string]interface{}{
				"roles": nil,
			},
			roleClaim: "roles",
			expected:  nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			payloadBytes, err := json.Marshal(tt.payload)
			require.NoError(t, err)
			actual := extractRoles(payloadBytes, tt.roleClaim)
			assert.Equal(t, tt.expected, actual)
		})
	}

	t.Run("Invalid JSON", func(t *testing.T) {
		actual := extractRoles([]byte("{invalid json}"), "roles")
		assert.Nil(t, actual)
	})

	t.Run("Empty payload", func(t *testing.T) {
		actual := extractRoles([]byte("{}"), "roles")
		assert.Nil(t, actual)
	})
}

func TestParseAndVerifyJWT_Unit(t *testing.T) {
	privKey, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)

	kid := "test-key-id"
	mockJWKS := stubJWKSProvider(map[string]*rsa.PublicKey{
		kid: &privKey.PublicKey,
	})

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
		jwt, err := ParseAndVerifyJWT(context.Background(), token, mockJWKS, "roles", "test-issuer", "test-audience")

		require.NoError(t, err)
		assert.Equal(t, "user123", jwt.Claims.Sub)
		assert.Equal(t, "test-issuer", jwt.Claims.Iss)
		assert.Equal(t, "test-audience", jwt.Claims.Aud)
		assert.Equal(t, "tenant-abc", jwt.Claims.TenantID)
		assert.Equal(t, []string{"admin"}, jwt.Roles)
	})

	t.Run("Invalid format - missing parts", func(t *testing.T) {
		_, err := ParseAndVerifyJWT(context.Background(), "invalid.token", mockJWKS, "roles", "", "")
		assert.Error(t, err)
		assert.ErrorIs(t, err, constants.ErrJWTInvalidFormat)
	})

	t.Run("Invalid format - too many parts", func(t *testing.T) {
		_, err := ParseAndVerifyJWT(context.Background(), "a.b.c.d", mockJWKS, "roles", "", "")
		assert.Error(t, err)
		assert.ErrorIs(t, err, constants.ErrJWTInvalidFormat)
	})

	t.Run("Invalid base64 header", func(t *testing.T) {
		token := "!!!." + base64.RawURLEncoding.EncodeToString([]byte("{}")) + ".sig"
		_, err := ParseAndVerifyJWT(context.Background(), token, mockJWKS, "roles", "", "")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "decode header")
	})

	t.Run("Invalid base64 payload", func(t *testing.T) {
		token := base64.RawURLEncoding.EncodeToString([]byte("{}")) + ".!!!.sig"
		_, err := ParseAndVerifyJWT(context.Background(), token, mockJWKS, "roles", "", "")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "decode payload")
	})

	t.Run("Invalid base64 signature", func(t *testing.T) {
		token := base64.RawURLEncoding.EncodeToString([]byte("{}")) + "." + base64.RawURLEncoding.EncodeToString([]byte("{}")) + ".!!!"
		_, err := ParseAndVerifyJWT(context.Background(), token, mockJWKS, "roles", "", "")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "decode signature")
	})

	t.Run("Invalid JSON header", func(t *testing.T) {
		token := base64.RawURLEncoding.EncodeToString([]byte("{invalid")) + "." + base64.RawURLEncoding.EncodeToString([]byte("{}")) + "." + base64.RawURLEncoding.EncodeToString([]byte("sig"))
		_, err := ParseAndVerifyJWT(context.Background(), token, mockJWKS, "roles", "", "")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "unmarshal header")
	})

	t.Run("Unsupported algorithm", func(t *testing.T) {
		header := map[string]interface{}{"alg": "HS256", "kid": kid}
		token := generateTestJWT(t, privKey, kid, header, validClaims)
		_, err := ParseAndVerifyJWT(context.Background(), token, mockJWKS, "roles", "", "")
		assert.Error(t, err)
		assert.ErrorIs(t, err, constants.ErrJWTUnsupportedAlg)
	})

	t.Run("Missing kid", func(t *testing.T) {
		header := map[string]interface{}{"alg": "RS256"}
		token := generateTestJWT(t, privKey, "", header, validClaims)
		_, err := ParseAndVerifyJWT(context.Background(), token, mockJWKS, "roles", "", "")
		assert.Error(t, err)
		assert.ErrorIs(t, err, constants.ErrJWTMissingKid)
	})

	t.Run("Invalid JSON payload", func(t *testing.T) {
		header := map[string]interface{}{"alg": "RS256", "kid": kid}
		headerBytes, err := json.Marshal(header)
		require.NoError(t, err)
		headerB64 := base64.RawURLEncoding.EncodeToString(headerBytes)
		payloadB64 := base64.RawURLEncoding.EncodeToString([]byte("{invalid"))
		token := headerB64 + "." + payloadB64 + ".sig"
		_, err = ParseAndVerifyJWT(context.Background(), token, mockJWKS, "roles", "", "")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "unmarshal payload")
	})

	t.Run("Expired token", func(t *testing.T) {
		claims := map[string]interface{}{
			"exp": time.Now().Add(-time.Hour).Unix(),
		}
		token := generateTestJWT(t, privKey, kid, nil, claims)
		_, err := ParseAndVerifyJWT(context.Background(), token, mockJWKS, "roles", "", "")
		assert.Error(t, err)
		assert.ErrorIs(t, err, constants.ErrExpired)
	})

	t.Run("Not yet valid (nbf)", func(t *testing.T) {
		claims := map[string]interface{}{
			"nbf": time.Now().Add(2 * time.Minute).Unix(),
		}
		token := generateTestJWT(t, privKey, kid, nil, claims)
		_, err := ParseAndVerifyJWT(context.Background(), token, mockJWKS, "roles", "", "")
		assert.Error(t, err)
		assert.ErrorIs(t, err, constants.ErrJWTNotYetValid)
	})

	t.Run("Valid within clock skew (nbf)", func(t *testing.T) {
		claims := map[string]interface{}{
			"nbf": time.Now().Add(30 * time.Second).Unix(), // 30s in future, skew is 60s
		}
		token := generateTestJWT(t, privKey, kid, nil, claims)
		_, err := ParseAndVerifyJWT(context.Background(), token, mockJWKS, "roles", "", "")
		assert.NoError(t, err)
	})

	t.Run("Issued in future outside clock skew", func(t *testing.T) {
		claims := map[string]interface{}{
			"iat": time.Now().Add(2 * time.Minute).Unix(),
		}
		token := generateTestJWT(t, privKey, kid, nil, claims)
		_, err := ParseAndVerifyJWT(context.Background(), token, mockJWKS, "roles", "", "")
		assert.ErrorIs(t, err, constants.ErrJWTIssuedInFuture)
	})

	t.Run("Issuer mismatch", func(t *testing.T) {
		token := generateTestJWT(t, privKey, kid, nil, validClaims)
		_, err := ParseAndVerifyJWT(context.Background(), token, mockJWKS, "roles", "wrong-issuer", "")
		assert.Error(t, err)
		assert.ErrorIs(t, err, constants.ErrJWTIssuerMismatch)
	})

	t.Run("Audience mismatch", func(t *testing.T) {
		token := generateTestJWT(t, privKey, kid, nil, validClaims)
		_, err := ParseAndVerifyJWT(context.Background(), token, mockJWKS, "roles", "", "wrong-audience")
		assert.Error(t, err)
		assert.ErrorIs(t, err, constants.ErrJWTAudienceMismatch)
	})

	t.Run("Signature verification failure", func(t *testing.T) {
		token := generateTestJWT(t, privKey, kid, nil, validClaims)
		parts := strings.Split(token, ".")
		// Tamper with signature
		tamperedSig := base64.RawURLEncoding.EncodeToString([]byte("invalid signature"))
		tamperedToken := parts[0] + "." + parts[1] + "." + tamperedSig

		_, err := ParseAndVerifyJWT(context.Background(), tamperedToken, mockJWKS, "roles", "", "")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "verify signature")
	})

	t.Run("Valid token with no issuer/audience validation", func(t *testing.T) {
		claims := map[string]interface{}{
			"sub": "user456",
			"exp": time.Now().Add(time.Hour).Unix(),
		}
		token := generateTestJWT(t, privKey, kid, nil, claims)
		jwt, err := ParseAndVerifyJWT(context.Background(), token, mockJWKS, "roles", "", "")
		require.NoError(t, err)
		assert.Equal(t, "user456", jwt.Claims.Sub)
	})

	t.Run("Valid token with single role string", func(t *testing.T) {
		claims := map[string]interface{}{
			"sub":  "user789",
			"exp":  time.Now().Add(time.Hour).Unix(),
			"role": "superuser",
		}
		token := generateTestJWT(t, privKey, kid, nil, claims)
		jwt, err := ParseAndVerifyJWT(context.Background(), token, mockJWKS, "role", "", "")
		require.NoError(t, err)
		assert.Equal(t, []string{"superuser"}, jwt.Roles)
	})

	t.Run("Valid token with missing role claim", func(t *testing.T) {
		claims := map[string]interface{}{
			"sub": "user999",
			"exp": time.Now().Add(time.Hour).Unix(),
		}
		token := generateTestJWT(t, privKey, kid, nil, claims)
		jwt, err := ParseAndVerifyJWT(context.Background(), token, mockJWKS, "roles", "", "")
		require.NoError(t, err)
		assert.Nil(t, jwt.Roles)
	})

	t.Run("Empty token string", func(t *testing.T) {
		_, err := ParseAndVerifyJWT(context.Background(), "", mockJWKS, "roles", "", "")
		assert.Error(t, err)
		assert.ErrorIs(t, err, constants.ErrJWTInvalidFormat)
	})

	t.Run("Token with empty segments", func(t *testing.T) {
		_, err := ParseAndVerifyJWT(context.Background(), "..", mockJWKS, "roles", "", "")
		assert.Error(t, err)
		// Empty segments decode to empty bytes, which fail JSON unmarshaling
		assert.Contains(t, err.Error(), "unmarshal")
	})

	t.Run("Token with exp=0 (no expiration)", func(t *testing.T) {
		claims := map[string]interface{}{
			"sub": "user000",
			"exp": 0,
		}
		token := generateTestJWT(t, privKey, kid, nil, claims)
		_, err := ParseAndVerifyJWT(context.Background(), token, mockJWKS, "roles", "", "")
		assert.NoError(t, err) // Should not fail with exp=0
	})

	t.Run("Token with nbf=0 (no not-before)", func(t *testing.T) {
		claims := map[string]interface{}{
			"sub": "user111",
			"exp": time.Now().Add(time.Hour).Unix(),
			"nbf": 0,
		}
		token := generateTestJWT(t, privKey, kid, nil, claims)
		_, err := ParseAndVerifyJWT(context.Background(), token, mockJWKS, "roles", "", "")
		assert.NoError(t, err) // Should not fail with nbf=0
	})

	t.Run("JWKS key not found", func(t *testing.T) {
		token := generateTestJWT(t, privKey, kid, nil, validClaims)
		emptyJWKS := stubJWKSProvider(map[string]*rsa.PublicKey{})
		_, err := ParseAndVerifyJWT(context.Background(), token, emptyJWKS, "roles", "", "")
		assert.Error(t, err)
		// The error comes from JWKSProvider trying to fetch keys when key is not in cache
		assert.Contains(t, err.Error(), "get public key")
	})

	t.Run("Valid token with all standard claims", func(t *testing.T) {
		now := time.Now()
		claims := map[string]interface{}{
			"sub":       "user123",
			"iss":       "test-issuer",
			"aud":       "test-audience",
			"exp":       now.Add(time.Hour).Unix(),
			"nbf":       now.Unix(),
			"iat":       now.Unix(),
			"tenant_id": "tenant-abc",
			"roles":     []string{"admin", "user"},
		}
		token := generateTestJWT(t, privKey, kid, nil, claims)
		jwt, err := ParseAndVerifyJWT(context.Background(), token, mockJWKS, "roles", "test-issuer", "test-audience")
		require.NoError(t, err)
		assert.Equal(t, "user123", jwt.Claims.Sub)
		assert.Equal(t, "test-issuer", jwt.Claims.Iss)
		assert.Equal(t, "test-audience", jwt.Claims.Aud)
		assert.Equal(t, "tenant-abc", jwt.Claims.TenantID)
		assert.Equal(t, []string{"admin", "user"}, jwt.Roles)
		assert.NotZero(t, jwt.Claims.Exp)
		assert.NotZero(t, jwt.Claims.Nbf)
		assert.NotZero(t, jwt.Claims.Iat)
	})

	t.Run("Header typ field validation", func(t *testing.T) {
		header := map[string]interface{}{
			"alg": "RS256",
			"kid": kid,
			"typ": "JWT",
		}
		token := generateTestJWT(t, privKey, kid, header, validClaims)
		jwt, err := ParseAndVerifyJWT(context.Background(), token, mockJWKS, "roles", "", "")
		require.NoError(t, err)
		assert.Equal(t, "JWT", jwt.Header.Typ)
	})

	t.Run("Tampered payload", func(t *testing.T) {
		token := generateTestJWT(t, privKey, kid, nil, validClaims)
		parts := strings.Split(token, ".")
		// Tamper with payload
		tamperedPayload := base64.RawURLEncoding.EncodeToString([]byte(`{"sub":"hacker"}`))
		tamperedToken := parts[0] + "." + tamperedPayload + "." + parts[2]

		_, err := ParseAndVerifyJWT(context.Background(), tamperedToken, mockJWKS, "roles", "", "")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "verify signature")
	})
}
