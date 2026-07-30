// Copyright (c) 2026 Lateralus Labs, LLC.
// Licensed under the Apache License, Version 2.0.

package client

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/go-webauthn/webauthn/protocol/webauthncbor"

	"github.com/g8e-ai/g8e/internal/constants"
	commonv1 "github.com/g8e-ai/g8e/protocol/proto/g8e/common/v1"
)

// SoftAuthenticator is a software WebAuthn authenticator that generates genuine
// WebAuthn assertions. It registers a real passkey with the gateway via the
// console registration endpoints, then produces valid L3 proofs by signing
// assertion data with the credential's private key.
//
// The authenticator uses ECDSA P-256 (ES256, COSE alg -7), which is the most
// widely supported WebAuthn algorithm. The attestation format is "none" (self
// attestation with no attestation statement), which the go-webauthn library
// accepts without requiring a trusted attestation CA.
type SoftAuthenticator struct {
	rpID     string
	rpOrigin string
	credID   []byte
	priv     *ecdsa.PrivateKey
	counter  uint32
}

// NewSoftAuthenticator creates a new software authenticator with a fresh
// ECDSA P-256 key pair and random credential ID.
func NewSoftAuthenticator(rpID, rpOrigin string) (*SoftAuthenticator, error) {
	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("soft authenticator: generate key: %w", err)
	}
	credID := make([]byte, 32)
	if _, err := rand.Read(credID); err != nil {
		return nil, fmt.Errorf("soft authenticator: generate cred id: %w", err)
	}
	return &SoftAuthenticator{
		rpID:     rpID,
		rpOrigin: rpOrigin,
		credID:   credID,
		priv:     priv,
	}, nil
}

// Register registers the software authenticator's passkey with the gateway via
// the console registration endpoints. It calls the challenge endpoint to get
// the WebAuthn registration options, generates a valid attestation response,
// and submits it to the verify endpoint.
func (a *SoftAuthenticator) Register(ctx context.Context, c *Client, userID, userName, cliSessionID string) error {
	// 1. Request registration challenge from the gateway.
	challengeBody, _ := json.Marshal(map[string]string{
		"user_id":        userID,
		"user_name":      userName,
		"cli_session_id": cliSessionID,
	})
	status, respBody, err := c.do(ctx, Persona{ID: "agent-harness"}, http.MethodPost,
		c.cfg.PublicBaseURL+constants.APIPaths.AuthPasskeysConsoleRegisterChallenge, challengeBody)
	if err != nil {
		return fmt.Errorf("soft authenticator: register challenge: %w", err)
	}
	if status >= 300 {
		return fmt.Errorf("soft authenticator: register challenge: status %d: %s", status, respBody)
	}

	var challengeResp struct {
		Success bool             `json:"success"`
		UserID  string           `json:"user_id"`
		Options json.RawMessage  `json:"options"`
	}
	if err := json.Unmarshal(respBody, &challengeResp); err != nil {
		return fmt.Errorf("soft authenticator: parse challenge response: %w", err)
	}
	if !challengeResp.Success {
		return fmt.Errorf("soft authenticator: challenge request failed")
	}

	var opts struct {
		Response struct {
			Challenge string `json:"challenge"`
			RP        struct {
				ID   string `json:"id"`
				Name string `json:"name"`
			} `json:"rp"`
			User struct {
				ID          string `json:"id"`
				DisplayName string `json:"displayName"`
				Name        string `json:"name"`
			} `json:"user"`
		} `json:"response"`
	}
	if err := json.Unmarshal(challengeResp.Options, &opts); err != nil {
		return fmt.Errorf("soft authenticator: parse challenge options: %w", err)
	}

	challenge := opts.Response.Challenge
	rpID := opts.Response.RP.ID
	if rpID != "" {
		a.rpID = rpID
	}

	// 2. Build the attestation response.
	credIDB64 := base64.RawURLEncoding.EncodeToString(a.credID)

	// Client data JSON for registration.
	clientData := map[string]string{
		"type":      "webauthn.create",
		"challenge": challenge,
		"origin":    a.rpOrigin,
	}
	clientDataJSON, err := json.Marshal(clientData)
	if err != nil {
		return fmt.Errorf("soft authenticator: marshal client data: %w", err)
	}

	// Build authenticator data with attested credential data.
	authData, err := a.buildRegistrationAuthData()
	if err != nil {
		return fmt.Errorf("soft authenticator: build auth data: %w", err)
	}

	// Build attestation object (fmt="none", empty attStmt).
	attObj := map[string]any{
		"fmt":      "none",
		"attStmt":  map[string]any{},
		"authData": authData,
	}
	attestationObject, err := webauthncbor.Marshal(attObj)
	if err != nil {
		return fmt.Errorf("soft authenticator: marshal attestation object: %w", err)
	}

	// 3. Submit the attestation response to the verify endpoint.
	attestationResp := map[string]any{
		"id":                credIDB64,
		"rawId":             credIDB64,
		"clientDataJSON":    base64.RawURLEncoding.EncodeToString(clientDataJSON),
		"attestationObject": base64.RawURLEncoding.EncodeToString(attestationObject),
		"transports":        []string{"internal"},
	}
	verifyBody, _ := json.Marshal(map[string]any{
		"user_id":              userID,
		"cli_session_id":       cliSessionID,
		"attestation_response": attestationResp,
	})

	status, respBody, err = c.do(ctx, Persona{ID: "agent-harness"}, http.MethodPost,
		c.cfg.PublicBaseURL+constants.APIPaths.AuthPasskeysConsoleRegisterVerify, verifyBody)
	if err != nil {
		return fmt.Errorf("soft authenticator: register verify: %w", err)
	}
	if status >= 300 {
		return fmt.Errorf("soft authenticator: register verify: status %d: %s", status, respBody)
	}

	var verifyResp struct {
		Success bool   `json:"success"`
		Error   string `json:"error"`
	}
	if err := json.Unmarshal(respBody, &verifyResp); err != nil {
		return fmt.Errorf("soft authenticator: parse verify response: %w", err)
	}
	if !verifyResp.Success {
		return fmt.Errorf("soft authenticator: registration rejected: %s", verifyResp.Error)
	}

	return nil
}

// SignAssertion generates a genuine WebAuthn assertion for the given transaction
// hash. The assertion is signed with the authenticator's ECDSA P-256 private key
// and can be verified by the gateway's PasskeyService.VerifyL3Proof method.
func (a *SoftAuthenticator) SignAssertion(txHash string) (*commonv1.L3Proof, error) {
	// 1. Build authenticator data (assertion format: no attested credential data).
	authData := a.buildAssertionAuthData()

	// 2. Build client data JSON.
	// The challenge is the base64url-encoded transaction hash, matching
	// encodeChallenge in passkey_service.go.
	challenge := base64.RawURLEncoding.EncodeToString([]byte(txHash))
	clientData := map[string]string{
		"type":      "webauthn.get",
		"challenge": challenge,
		"origin":    a.rpOrigin,
	}
	clientDataJSON, err := json.Marshal(clientData)
	if err != nil {
		return nil, fmt.Errorf("soft authenticator: marshal client data: %w", err)
	}

	// 3. Sign authData || SHA256(clientDataJSON) with ECDSA P-256.
	clientDataHash := sha256.Sum256(clientDataJSON)
	sigData := append(authData, clientDataHash[:]...)

	sigASN1, err := ecdsa.SignASN1(rand.Reader, a.priv, sigData)
	if err != nil {
		return nil, fmt.Errorf("soft authenticator: sign assertion: %w", err)
	}

	// The go-webauthn library uses ASN.1-encoded ECDSA signatures (via
	// ECDSASignature.Unmarshal), so we return the ASN.1 form directly.

	credIDB64 := base64.RawURLEncoding.EncodeToString(a.credID)

	return &commonv1.L3Proof{
		CredentialId:      credIDB64,
		ClientDataJson:    base64.RawURLEncoding.EncodeToString(clientDataJSON),
		AuthenticatorData: base64.RawURLEncoding.EncodeToString(authData),
		Signature:         base64.RawURLEncoding.EncodeToString(sigASN1),
	}, nil
}

// SignL3 returns an L3Metadata containing the WebAuthn assertion proof.
func (a *SoftAuthenticator) SignL3(txHash string) *commonv1.L3Metadata {
	proof, err := a.SignAssertion(txHash)
	if err != nil {
		return &commonv1.L3Metadata{}
	}
	return &commonv1.L3Metadata{Proof: proof}
}

// buildRegistrationAuthData builds the authenticator data for a registration
// ceremony, including attested credential data with the CBOR-encoded public key.
func (a *SoftAuthenticator) buildRegistrationAuthData() ([]byte, error) {
	rpIDHash := sha256.Sum256([]byte(a.rpID))

	// Flags: UserPresent (0x01) | UserVerified (0x04) | AttestedCredentialData (0x40)
	flags := byte(0x01 | 0x04 | 0x40)

	// Counter (4 bytes, big-endian) — start at 0.
	counter := make([]byte, 4)
	binary.BigEndian.PutUint32(counter, 0)

	// AAGUID (16 bytes) — all zeros for a software authenticator.
	aaguid := make([]byte, 16)

	// Credential ID length (2 bytes, big-endian) + credential ID.
	credIDLen := make([]byte, 2)
	binary.BigEndian.PutUint16(credIDLen, uint16(len(a.credID)))

	// CBOR-encoded COSE public key (EC2 P-256 / ES256).
	pubKeyCBOR, err := a.cosePublicKey()
	if err != nil {
		return nil, err
	}

	// Assemble: rpIDHash(32) + flags(1) + counter(4) + aaguid(16) + credIDLen(2) + credID + pubKeyCBOR
	authData := make([]byte, 0, 37+16+2+len(a.credID)+len(pubKeyCBOR))
	authData = append(authData, rpIDHash[:]...)
	authData = append(authData, flags)
	authData = append(authData, counter...)
	authData = append(authData, aaguid...)
	authData = append(authData, credIDLen...)
	authData = append(authData, a.credID...)
	authData = append(authData, pubKeyCBOR...)

	return authData, nil
}

// buildAssertionAuthData builds the authenticator data for an assertion
// ceremony (login/get). This is 37 bytes: rpIDHash(32) + flags(1) + counter(4).
func (a *SoftAuthenticator) buildAssertionAuthData() []byte {
	rpIDHash := sha256.Sum256([]byte(a.rpID))

	// Flags: UserPresent (0x01) | UserVerified (0x04)
	flags := byte(0x01 | 0x04)

	a.counter++
	counter := make([]byte, 4)
	binary.BigEndian.PutUint32(counter, a.counter)

	authData := make([]byte, 0, 37)
	authData = append(authData, rpIDHash[:]...)
	authData = append(authData, flags)
	authData = append(authData, counter...)

	return authData
}

// cosePublicKey returns the CBOR-encoded COSE_Key for the authenticator's
// ECDSA P-256 public key using ES256 (alg -7).
func (a *SoftAuthenticator) cosePublicKey() ([]byte, error) {
	// Marshal the public key to get the uncompressed point (0x04 || X || Y).
	pubKeyBytes := elliptic.Marshal(elliptic.P256(), a.priv.PublicKey.X, a.priv.PublicKey.Y)
	if len(pubKeyBytes) != 65 {
		return nil, fmt.Errorf("soft authenticator: unexpected public key length %d", len(pubKeyBytes))
	}
	x := pubKeyBytes[1:33]
	y := pubKeyBytes[33:65]

	// COSE_Key for EC2 P-256 / ES256:
	// {1: 2 (kty=EC2), 3: -7 (alg=ES256), -1: 1 (crv=P-256), -2: x, -3: y}
	coseKey := map[int64]any{
		1:  int64(2),  // kty: EC2
		3:  int64(-7), // alg: ES256
		-1: int64(1),  // crv: P-256
		-2: x,         // x-coordinate
		-3: y,         // y-coordinate
	}

	encoded, err := webauthncbor.Marshal(coseKey)
	if err != nil {
		return nil, fmt.Errorf("soft authenticator: marshal COSE key: %w", err)
	}
	return encoded, nil
}

// PublicKeyPEM returns the authenticator's public key in PEM format (for
// debugging or optional trusted-signer registration).
func (a *SoftAuthenticator) PublicKeyPEM() (string, error) {
	der, err := x509.MarshalPKIXPublicKey(&a.priv.PublicKey)
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(der), nil
}

// CredentialID returns the authenticator's credential ID as base64url.
func (a *SoftAuthenticator) CredentialID() string {
	return base64.RawURLEncoding.EncodeToString(a.credID)
}

// Sign returns an L3 proof using the WebAuthn assertion, matching the
// Principal.Sign interface for drop-in replacement.
func (a *SoftAuthenticator) Sign(txHash string) *commonv1.L3Metadata {
	return a.SignL3(txHash)
}

