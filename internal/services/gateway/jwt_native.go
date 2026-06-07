package gateway

import (
	"crypto"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
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

func ParseAndVerifyJWT(tokenString string, jwks *JWKSProvider, roleClaim string, expectedIssuer, expectedAudience string) (*NativeJWT, error) {
	parts := strings.Split(tokenString, ".")
	if len(parts) != 3 {
		return nil, errors.New("invalid JWT format")
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
		return nil, fmt.Errorf("unsupported signing algorithm: %s", header.Alg)
	}
	if header.Kid == "" {
		return nil, errors.New("missing kid in header")
	}

	var claims JWTClaims
	if err := json.Unmarshal(payloadBytes, &claims); err != nil {
		return nil, fmt.Errorf("jwt: unmarshal payload: %w", err)
	}

	now := time.Now().Unix()
	if claims.Exp != 0 && now > claims.Exp {
		return nil, errors.New("token is expired")
	}

	// Validate not-before with clock skew allowance
	if claims.Nbf != 0 {
		nbfWithSkew := claims.Nbf - clockSkewAllowance
		if now < nbfWithSkew {
			return nil, errors.New("token is not yet valid (nbf)")
		}
	}

	// Validate issuer if expected
	if expectedIssuer != "" && claims.Iss != expectedIssuer {
		return nil, fmt.Errorf("token issuer mismatch: expected %s, got %s", expectedIssuer, claims.Iss)
	}

	// Validate audience if expected
	if expectedAudience != "" && claims.Aud != expectedAudience {
		return nil, fmt.Errorf("token audience mismatch: expected %s, got %s", expectedAudience, claims.Aud)
	}

	pubKey, err := jwks.GetKey(header.Kid)
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
