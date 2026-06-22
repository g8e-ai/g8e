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

package gateway

import (
	"context"
	"crypto"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/g8e-ai/g8e/internal/constants"
)

type JWTClaims struct {
	Sub      string `json:"sub"`
	Iss      string `json:"iss"`
	Aud      string `json:"aud"`
	Exp      int64  `json:"exp"`
	Nbf      int64  `json:"nbf"`
	Iat      int64  `json:"iat"`
	TenantID string `json:"tenant_id"`
}

type JWTHeader struct {
	Alg string `json:"alg"`
	Kid string `json:"kid"`
	Typ string `json:"typ"`
}

const (
	clockSkewAllowance = 60
)

type NativeJWT struct {
	Header JWTHeader
	Claims JWTClaims
	Roles  []string
}

func extractRoles(payloadBytes []byte, roleClaim string) []string {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(payloadBytes, &raw); err != nil {
		return nil
	}

	rolesRaw, ok := raw[roleClaim]
	if !ok {
		return nil
	}

	var rolesArray []string
	if err := json.Unmarshal(rolesRaw, &rolesArray); err == nil {
		return rolesArray
	}

	var roleString string
	if err := json.Unmarshal(rolesRaw, &roleString); err == nil {
		return []string{roleString}
	}

	return nil
}

func ParseAndVerifyJWT(ctx context.Context, tokenString string, jwks *JWKSProvider, roleClaim string, expectedIssuer, expectedAudience string) (*NativeJWT, error) {
	parts := strings.Split(tokenString, ".")
	if len(parts) != 3 {
		return nil, constants.ErrJWTInvalidFormat
	}

	headerB64, payloadB64, sigB64 := parts[0], parts[1], parts[2]

	headerBytes, err := base64.RawURLEncoding.DecodeString(headerB64)
	if err != nil {
		return nil, fmt.Errorf("jwt: decode header: %w", err)
	}

	payloadBytes, err := base64.RawURLEncoding.DecodeString(payloadB64)
	if err != nil {
		return nil, fmt.Errorf("jwt: decode payload: %w", err)
	}

	sigBytes, err := base64.RawURLEncoding.DecodeString(sigB64)
	if err != nil {
		return nil, fmt.Errorf("jwt: decode signature: %w", err)
	}

	var header JWTHeader
	if err := json.Unmarshal(headerBytes, &header); err != nil {
		return nil, fmt.Errorf("jwt: unmarshal header: %w", err)
	}

	if header.Alg != "RS256" {
		return nil, fmt.Errorf("jwt: unsupported algorithm %s: %w", header.Alg, constants.ErrJWTUnsupportedAlg)
	}
	if header.Kid == "" {
		return nil, constants.ErrJWTMissingKid
	}

	var claims JWTClaims
	if err := json.Unmarshal(payloadBytes, &claims); err != nil {
		return nil, fmt.Errorf("jwt: unmarshal payload: %w", err)
	}

	now := time.Now().Unix()
	if claims.Exp != 0 && now > claims.Exp {
		return nil, constants.ErrExpired
	}

	// Validate not-before with clock skew allowance
	if claims.Nbf != 0 {
		nbfWithSkew := claims.Nbf - clockSkewAllowance
		if now < nbfWithSkew {
			return nil, constants.ErrJWTNotYetValid
		}
	}

	// Validate issuer if expected
	if expectedIssuer != "" && claims.Iss != expectedIssuer {
		return nil, fmt.Errorf("jwt: issuer mismatch (expected %s, got %s): %w", expectedIssuer, claims.Iss, constants.ErrJWTIssuerMismatch)
	}

	// Validate audience if expected
	if expectedAudience != "" && claims.Aud != expectedAudience {
		return nil, fmt.Errorf("jwt: audience mismatch (expected %s, got %s): %w", expectedAudience, claims.Aud, constants.ErrJWTAudienceMismatch)
	}

	pubKey, err := jwks.GetKey(ctx, header.Kid)
	if err != nil {
		return nil, fmt.Errorf("jwt: get public key: %w", err)
	}

	signingString := headerB64 + "." + payloadB64
	hasher := sha256.New()
	hasher.Write([]byte(signingString))
	hashed := hasher.Sum(nil)

	if err := rsa.VerifyPKCS1v15(pubKey, crypto.SHA256, hashed, sigBytes); err != nil {
		return nil, fmt.Errorf("jwt: verify signature: %w", err)
	}

	roles := extractRoles(payloadBytes, roleClaim)

	return &NativeJWT{
		Header: header,
		Claims: claims,
		Roles:  roles,
	}, nil
}
