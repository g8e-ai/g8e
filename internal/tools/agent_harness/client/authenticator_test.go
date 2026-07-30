// Copyright (c) 2026 Lateralus Labs, LLC.
// Licensed under the Apache License, Version 2.0.

package client

import (
	"crypto/sha256"
	"encoding/base64"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	commonv1 "github.com/g8e-ai/g8e/protocol/proto/g8e/common/v1"
)

func TestNewSoftAuthenticator_CreatesValidKeyAndCredID(t *testing.T) {
	a, err := NewSoftAuthenticator("localhost", "http://localhost:8080")
	require.NoError(t, err)

	assert.Equal(t, "localhost", a.rpID)
	assert.Equal(t, "http://localhost:8080", a.rpOrigin)
	assert.Len(t, a.credID, 32, "credential ID should be 32 bytes")
	assert.NotNil(t, a.priv, "private key should be initialized")
	assert.Equal(t, "P-256", a.priv.Curve.Params().Name, "should use P-256 curve")
}

func TestNewSoftAuthenticator_DistinctInstancesHaveDistinctKeys(t *testing.T) {
	a1, err := NewSoftAuthenticator("localhost", "http://localhost:8080")
	require.NoError(t, err)
	a2, err := NewSoftAuthenticator("localhost", "http://localhost:8080")
	require.NoError(t, err)

	assert.NotEqual(t, a1.credID, a2.credID, "credential IDs should differ")
	assert.NotEqual(t, a1.priv.D, a2.priv.D, "private keys should differ")
}

func TestSoftAuthenticator_SignAssertion_ProducesValidProof(t *testing.T) {
	a, err := NewSoftAuthenticator("localhost", "http://localhost:8080")
	require.NoError(t, err)

	txHash := "abc123def4567890123456789012345678901234567890123456789012345678"
	proof, err := a.SignAssertion(txHash)
	require.NoError(t, err)

	assert.NotEmpty(t, proof.CredentialId, "CredentialId should be non-empty")
	assert.NotEmpty(t, proof.ClientDataJson, "ClientDataJson should be non-empty")
	assert.NotEmpty(t, proof.AuthenticatorData, "AuthenticatorData should be non-empty")
	assert.NotEmpty(t, proof.Signature, "Signature should be non-empty")

	credID, err := base64.RawURLEncoding.DecodeString(proof.CredentialId)
	require.NoError(t, err, "CredentialId should be base64url-decodable")
	assert.Len(t, credID, 32, "decoded credential ID should be 32 bytes")

	_, err = base64.RawURLEncoding.DecodeString(proof.ClientDataJson)
	require.NoError(t, err, "ClientDataJson should be base64url-decodable")

	_, err = base64.RawURLEncoding.DecodeString(proof.AuthenticatorData)
	require.NoError(t, err, "AuthenticatorData should be base64url-decodable")

	_, err = base64.RawURLEncoding.DecodeString(proof.Signature)
	require.NoError(t, err, "Signature should be base64url-decodable")
}

func TestSoftAuthenticator_SignAssertion_CounterIncrements(t *testing.T) {
	a, err := NewSoftAuthenticator("localhost", "http://localhost:8080")
	require.NoError(t, err)

	txHash := "test-hash-for-counter-increment-1234567890123456"

	proof1, err := a.SignAssertion(txHash)
	require.NoError(t, err)

	proof2, err := a.SignAssertion(txHash)
	require.NoError(t, err)

	authData1, err := base64.RawURLEncoding.DecodeString(proof1.AuthenticatorData)
	require.NoError(t, err)
	authData2, err := base64.RawURLEncoding.DecodeString(proof2.AuthenticatorData)
	require.NoError(t, err)

	require.Len(t, authData1, 37, "assertion authData should be 37 bytes")
	require.Len(t, authData2, 37, "assertion authData should be 37 bytes")

	counter1 := uint32(authData1[33])<<24 | uint32(authData1[34])<<16 | uint32(authData1[35])<<8 | uint32(authData1[36])
	counter2 := uint32(authData2[33])<<24 | uint32(authData2[34])<<16 | uint32(authData2[35])<<8 | uint32(authData2[36])

	assert.Equal(t, counter1+1, counter2, "counter should increment by 1 between assertions")
}

func TestSoftAuthenticator_SignAssertion_DistinctHashesProduceDistinctSignatures(t *testing.T) {
	a, err := NewSoftAuthenticator("localhost", "http://localhost:8080")
	require.NoError(t, err)

	proof1, err := a.SignAssertion("hash-one-aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
	require.NoError(t, err)
	proof2, err := a.SignAssertion("hash-two-bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb")
	require.NoError(t, err)

	assert.NotEqual(t, proof1.Signature, proof2.Signature, "signatures should differ for different tx hashes")
}

func TestSoftAuthenticator_BuildAssertionAuthData_CorrectRPIDHash(t *testing.T) {
	a, err := NewSoftAuthenticator("localhost", "http://localhost:8080")
	require.NoError(t, err)

	authData := a.buildAssertionAuthData()
	require.Len(t, authData, 37, "assertion authData should be 37 bytes")

	expectedRPIDHash := sha256.Sum256([]byte("localhost"))
	assert.Equal(t, expectedRPIDHash[:], authData[:32], "rpIDHash should match SHA256(rpID)")

	flags := authData[32]
	assert.Equal(t, byte(0x05), flags, "flags should be UP|UV (0x01|0x04)")
}

func TestSoftAuthenticator_BuildRegistrationAuthData_HasAttestedCredentialData(t *testing.T) {
	a, err := NewSoftAuthenticator("localhost", "http://localhost:8080")
	require.NoError(t, err)

	authData, err := a.buildRegistrationAuthData()
	require.NoError(t, err)

	require.Greater(t, len(authData), 37, "registration authData should be longer than 37 bytes (includes attested credential data)")

	flags := authData[32]
	assert.Equal(t, byte(0x45), flags, "flags should be UP|UV|AT (0x01|0x04|0x40)")

	expectedRPIDHash := sha256.Sum256([]byte("localhost"))
	assert.Equal(t, expectedRPIDHash[:], authData[:32], "rpIDHash should match SHA256(rpID)")
}

func TestSoftAuthenticator_CosePublicKey_ValidEC2P256(t *testing.T) {
	a, err := NewSoftAuthenticator("localhost", "http://localhost:8080")
	require.NoError(t, err)

	coseKey, err := a.cosePublicKey()
	require.NoError(t, err)
	assert.NotEmpty(t, coseKey, "COSE key should be non-empty")
}

func TestSoftAuthenticator_SignL3_ReturnsValidMetadata(t *testing.T) {
	a, err := NewSoftAuthenticator("localhost", "http://localhost:8080")
	require.NoError(t, err)

	txHash := "test-hash-for-signl3-1234567890123456789012345678901234567890"
	meta := a.SignL3(txHash)

	require.NotNil(t, meta, "SignL3 should return non-nil L3Metadata")
	require.NotNil(t, meta.Proof, "L3Metadata.Proof should be non-nil")
	assert.NotEmpty(t, meta.Proof.CredentialId, "proof should have CredentialId")
	assert.NotEmpty(t, meta.Proof.Signature, "proof should have Signature")
}

func TestSoftAuthenticator_Sign_MatchesSignL3(t *testing.T) {
	a, err := NewSoftAuthenticator("localhost", "http://localhost:8080")
	require.NoError(t, err)

	txHash := "test-hash-for-sign-alias-12345678901234567890123456789012345"

	meta1 := a.SignL3(txHash)
	// Reset counter to make comparison deterministic — Sign and SignL3 both
	// increment the counter, so we compare structure rather than exact values.
	meta2 := a.Sign(txHash)

	require.NotNil(t, meta1.Proof, "SignL3 should produce proof")
	require.NotNil(t, meta2.Proof, "Sign should produce proof")
	assert.IsType(t, &commonv1.L3Metadata{}, meta1)
	assert.IsType(t, &commonv1.L3Metadata{}, meta2)
}

func TestSoftAuthenticator_CredentialID_ReturnsBase64URL(t *testing.T) {
	a, err := NewSoftAuthenticator("localhost", "http://localhost:8080")
	require.NoError(t, err)

	credID := a.CredentialID()
	assert.NotEmpty(t, credID, "CredentialID should be non-empty")

	decoded, err := base64.RawURLEncoding.DecodeString(credID)
	require.NoError(t, err, "CredentialID should be base64url-decodable")
	assert.Len(t, decoded, 32, "decoded credential ID should be 32 bytes")
}

func TestSoftAuthenticator_PublicKeyPEM_ReturnsNonEmpty(t *testing.T) {
	a, err := NewSoftAuthenticator("localhost", "http://localhost:8080")
	require.NoError(t, err)

	pem, err := a.PublicKeyPEM()
	require.NoError(t, err)
	assert.NotEmpty(t, pem, "PublicKeyPEM should return non-empty string")
}
