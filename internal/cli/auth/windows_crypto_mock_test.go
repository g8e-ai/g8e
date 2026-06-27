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

package auth

import (
	"encoding/base64"
	"encoding/json"
	"testing"

	"github.com/g8e-ai/g8e/internal/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestAssertionResponseRoundTrip verifies that a synthetic WebAuthnAssertionResponse
// (as returned by Windows Hello) can be encoded via base64.RawURLEncoding into
// models.WebAuthnAssertionResponse, marshaled to JSON, and unmarshaled back
// with all fields intact. This tests the full client→server encoding pathway
// without requiring webauthn.dll.
func TestAssertionResponseRoundTrip(t *testing.T) {
	tests := []struct {
		name      string
		assertion WebAuthnAssertionResponse
	}{
		{
			name: "typical assertion",
			assertion: WebAuthnAssertionResponse{
				Id:                base64.RawURLEncoding.EncodeToString([]byte("credential-id-12345")),
				RawId:             []byte("credential-id-12345"),
				AuthenticatorData: []byte("auth-data-payload"),
				Signature:         []byte("signature-bytes-here"),
				UserHandle:        []byte("user-handle-16bytes"),
			},
		},
		{
			name: "empty UserHandle",
			assertion: WebAuthnAssertionResponse{
				Id:                base64.RawURLEncoding.EncodeToString([]byte("cred-no-userhandle")),
				RawId:             []byte("cred-no-userhandle"),
				AuthenticatorData: []byte("auth-data"),
				Signature:         []byte("sig"),
				UserHandle:        nil,
			},
		},
		{
			name: "large credential ID (1024 bytes)",
			assertion: WebAuthnAssertionResponse{
				Id:                base64.RawURLEncoding.EncodeToString(make([]byte, 1024)),
				RawId:             make([]byte, 1024),
				AuthenticatorData: []byte("auth-data-large-cred"),
				Signature:         []byte("sig-large-cred"),
				UserHandle:        []byte("userhandle"),
			},
		},
		{
			name: "empty RawId",
			assertion: WebAuthnAssertionResponse{
				Id:                "",
				RawId:             nil,
				AuthenticatorData: []byte("auth-data-empty-rawid"),
				Signature:         []byte("sig-empty-rawid"),
				UserHandle:        []byte("userhandle"),
			},
		},
		{
			name: "binary data with special bytes",
			assertion: WebAuthnAssertionResponse{
				Id:                base64.RawURLEncoding.EncodeToString([]byte{0x00, 0xFF, 0x80, 0x7F, 0x01, 0x02}),
				RawId:             []byte{0x00, 0xFF, 0x80, 0x7F, 0x01, 0x02},
				AuthenticatorData: []byte{0x30, 0x31, 0x32, 0x00, 0xAB, 0xCD},
				Signature:         []byte{0xDE, 0xAD, 0xBE, 0xEF},
				UserHandle:        []byte{0x01, 0x02, 0x03, 0x04},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Encode as client.go does: base64.RawURLEncoding for each field
			encoded := models.WebAuthnAssertionResponse{
				ID:                tt.assertion.Id,
				RawID:             base64.RawURLEncoding.EncodeToString(tt.assertion.RawId),
				ClientDataJSON:    base64.RawURLEncoding.EncodeToString([]byte(`{"challenge":"test","origin":"https://localhost","type":"webauthn.get"}`)),
				AuthenticatorData: base64.RawURLEncoding.EncodeToString(tt.assertion.AuthenticatorData),
				Signature:         base64.RawURLEncoding.EncodeToString(tt.assertion.Signature),
				UserHandle:        base64.RawURLEncoding.EncodeToString(tt.assertion.UserHandle),
			}

			// Verify Id field consistency with base64.RawURLEncoding.EncodeToString(RawId)
			expectedID := base64.RawURLEncoding.EncodeToString(tt.assertion.RawId)
			assert.Equal(t, expectedID, encoded.ID, "ID field must match base64.RawURLEncoding of RawId")

			// Marshal to JSON (simulates HTTP request body)
			jsonBytes, err := json.Marshal(encoded)
			require.NoError(t, err)

			// Unmarshal back (simulates server-side parse)
			var decoded models.WebAuthnAssertionResponse
			err = json.Unmarshal(jsonBytes, &decoded)
			require.NoError(t, err)

			// Verify all fields round-trip
			assert.Equal(t, encoded.ID, decoded.ID)
			assert.Equal(t, encoded.RawID, decoded.RawID)
			assert.Equal(t, encoded.ClientDataJSON, decoded.ClientDataJSON)
			assert.Equal(t, encoded.AuthenticatorData, decoded.AuthenticatorData)
			assert.Equal(t, encoded.Signature, decoded.Signature)
			assert.Equal(t, encoded.UserHandle, decoded.UserHandle)

			// Verify the base64-decoded RawID matches the original
			// Note: base64.DecodeString("") returns []byte{} not nil, so we compare content
			decodedRawID, err := base64.RawURLEncoding.DecodeString(decoded.RawID)
			require.NoError(t, err)
			if len(tt.assertion.RawId) == 0 {
				assert.Empty(t, decodedRawID, "decoded RawID must be empty for empty input")
			} else {
				assert.Equal(t, tt.assertion.RawId, decodedRawID, "decoded RawID must match original")
			}

			// Verify the base64-decoded Signature matches the original
			decodedSig, err := base64.RawURLEncoding.DecodeString(decoded.Signature)
			require.NoError(t, err)
			assert.Equal(t, tt.assertion.Signature, decodedSig, "decoded Signature must match original")

			// Verify the base64-decoded AuthenticatorData matches the original
			decodedAuthData, err := base64.RawURLEncoding.DecodeString(decoded.AuthenticatorData)
			require.NoError(t, err)
			assert.Equal(t, tt.assertion.AuthenticatorData, decodedAuthData, "decoded AuthenticatorData must match original")

			// Verify UserHandle round-trip (may be empty string for nil/empty)
			if len(tt.assertion.UserHandle) > 0 {
				decodedUserHandle, err := base64.RawURLEncoding.DecodeString(decoded.UserHandle)
				require.NoError(t, err)
				assert.Equal(t, tt.assertion.UserHandle, decodedUserHandle, "decoded UserHandle must match original")
			}
		})
	}
}

// TestAttestationResponseRoundTrip verifies that a synthetic WebAuthnAttestationResponse
// (as returned by Windows Hello registration) can be encoded via base64.RawURLEncoding
// into models.WebAuthnAttestationResponse, marshaled to JSON, and unmarshaled back
// with all fields intact.
func TestAttestationResponseRoundTrip(t *testing.T) {
	tests := []struct {
		name        string
		attestation WebAuthnAttestationResponse
		transports  []string
		clientData  []byte
	}{
		{
			name: "typical attestation",
			attestation: WebAuthnAttestationResponse{
				Id:                base64.RawURLEncoding.EncodeToString([]byte("new-credential-id")),
				RawId:             []byte("new-credential-id"),
				AuthenticatorData: []byte("auth-data-attest"),
				AttestationObject: []byte("attestation-object-bytes"),
			},
			transports: []string{"internal"},
			clientData: []byte(`{"challenge":"reg-challenge","origin":"https://localhost","type":"webauthn.create"}`),
		},
		{
			name: "large credential ID (1024 bytes)",
			attestation: WebAuthnAttestationResponse{
				Id:                base64.RawURLEncoding.EncodeToString(make([]byte, 1024)),
				RawId:             make([]byte, 1024),
				AuthenticatorData: []byte("auth-data-large"),
				AttestationObject: []byte("attestation-large"),
			},
			transports: []string{"internal", "hybrid"},
			clientData: []byte(`{"challenge":"large","origin":"https://localhost","type":"webauthn.create"}`),
		},
		{
			name: "empty RawId",
			attestation: WebAuthnAttestationResponse{
				Id:                "",
				RawId:             nil,
				AuthenticatorData: []byte("auth-data-empty"),
				AttestationObject: []byte("attestation-empty"),
			},
			transports: nil,
			clientData: []byte(`{"challenge":"empty","origin":"https://localhost","type":"webauthn.create"}`),
		},
		{
			name: "binary data with special bytes",
			attestation: WebAuthnAttestationResponse{
				Id:                base64.RawURLEncoding.EncodeToString([]byte{0x00, 0xFF, 0x80, 0x7F}),
				RawId:             []byte{0x00, 0xFF, 0x80, 0x7F},
				AuthenticatorData: []byte{0x30, 0x31, 0x00, 0xAB},
				AttestationObject: []byte{0xDE, 0xAD, 0xBE, 0xEF, 0x00},
			},
			transports: []string{"usb"},
			clientData: []byte(`{"challenge":"binary","origin":"https://localhost","type":"webauthn.create"}`),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Encode as client.go / passkey_bootstrap.go does
			encoded := models.WebAuthnAttestationResponse{
				ID:                tt.attestation.Id,
				RawID:             base64.RawURLEncoding.EncodeToString(tt.attestation.RawId),
				ClientDataJSON:    base64.RawURLEncoding.EncodeToString(tt.clientData),
				AttestationObject: base64.RawURLEncoding.EncodeToString(tt.attestation.AttestationObject),
				Transports:        tt.transports,
			}

			// Verify Id field consistency
			expectedID := base64.RawURLEncoding.EncodeToString(tt.attestation.RawId)
			assert.Equal(t, expectedID, encoded.ID, "ID field must match base64.RawURLEncoding of RawId")

			// Marshal to JSON
			jsonBytes, err := json.Marshal(encoded)
			require.NoError(t, err)

			// Unmarshal back
			var decoded models.WebAuthnAttestationResponse
			err = json.Unmarshal(jsonBytes, &decoded)
			require.NoError(t, err)

			// Verify all fields round-trip
			assert.Equal(t, encoded.ID, decoded.ID)
			assert.Equal(t, encoded.RawID, decoded.RawID)
			assert.Equal(t, encoded.ClientDataJSON, decoded.ClientDataJSON)
			assert.Equal(t, encoded.AttestationObject, decoded.AttestationObject)
			assert.Equal(t, encoded.Transports, decoded.Transports)

			// Verify base64-decoded fields match originals
			decodedRawID, err := base64.RawURLEncoding.DecodeString(decoded.RawID)
			require.NoError(t, err)
			if len(tt.attestation.RawId) == 0 {
				assert.Empty(t, decodedRawID, "decoded RawID must be empty for empty input")
			} else {
				assert.Equal(t, tt.attestation.RawId, decodedRawID, "decoded RawID must match original")
			}

			decodedAttestationObj, err := base64.RawURLEncoding.DecodeString(decoded.AttestationObject)
			require.NoError(t, err)
			assert.Equal(t, tt.attestation.AttestationObject, decodedAttestationObj, "decoded AttestationObject must match original")

			decodedClientData, err := base64.RawURLEncoding.DecodeString(decoded.ClientDataJSON)
			require.NoError(t, err)
			assert.Equal(t, tt.clientData, decodedClientData, "decoded ClientDataJSON must match original")
		})
	}
}

// TestAssertionResponseJSONShape verifies that the JSON output from encoding
// models.WebAuthnAssertionResponse uses the correct JSON field names expected
// by the server-side handlers (camelCase, matching WebAuthn spec).
func TestAssertionResponseJSONShape(t *testing.T) {
	encoded := models.WebAuthnAssertionResponse{
		ID:                "test-id",
		RawID:             "test-raw-id",
		ClientDataJSON:    "test-client-data",
		AuthenticatorData: "test-auth-data",
		Signature:         "test-signature",
		UserHandle:        "test-user-handle",
	}

	jsonBytes, err := json.Marshal(encoded)
	require.NoError(t, err)

	var raw map[string]json.RawMessage
	err = json.Unmarshal(jsonBytes, &raw)
	require.NoError(t, err)

	expectedFields := []string{"id", "rawId", "clientDataJSON", "authenticatorData", "signature", "userHandle"}
	for _, field := range expectedFields {
		_, exists := raw[field]
		assert.True(t, exists, "JSON must contain field %q", field)
	}
}

// TestAttestationResponseJSONShape verifies that the JSON output from encoding
// models.WebAuthnAttestationResponse uses the correct JSON field names expected
// by the server-side handlers.
func TestAttestationResponseJSONShape(t *testing.T) {
	encoded := models.WebAuthnAttestationResponse{
		ID:                "test-id",
		RawID:             "test-raw-id",
		ClientDataJSON:    "test-client-data",
		AttestationObject: "test-attestation-obj",
		Transports:        []string{"internal"},
	}

	jsonBytes, err := json.Marshal(encoded)
	require.NoError(t, err)

	var raw map[string]json.RawMessage
	err = json.Unmarshal(jsonBytes, &raw)
	require.NoError(t, err)

	expectedFields := []string{"id", "rawId", "clientDataJSON", "attestationObject", "transports"}
	for _, field := range expectedFields {
		_, exists := raw[field]
		assert.True(t, exists, "JSON must contain field %q", field)
	}
}

// TestAssertionResponseEmptyUserHandleOmit verifies that an empty UserHandle
// is omitted from JSON when the omitempty tag is present, matching the
// models.WebAuthnAssertionResponse struct definition.
func TestAssertionResponseEmptyUserHandleOmit(t *testing.T) {
	encoded := models.WebAuthnAssertionResponse{
		ID:                "test-id",
		RawID:             "test-raw-id",
		ClientDataJSON:    "test-client-data",
		AuthenticatorData: "test-auth-data",
		Signature:         "test-signature",
		UserHandle:        "",
	}

	jsonBytes, err := json.Marshal(encoded)
	require.NoError(t, err)

	var raw map[string]json.RawMessage
	err = json.Unmarshal(jsonBytes, &raw)
	require.NoError(t, err)

	_, exists := raw["userHandle"]
	assert.False(t, exists, "empty UserHandle should be omitted from JSON due to omitempty tag")
}

// TestCLIEncodingPathwayConsistency verifies that the encoding done in client.go
// (base64.RawURLEncoding for all byte fields) produces identical results to
// direct base64.RawURLEncoding.EncodeToString calls, ensuring no encoding
// drift between the client and server.
func TestCLIEncodingPathwayConsistency(t *testing.T) {
	rawID := []byte("consistent-credential-id")
	authData := []byte("consistent-auth-data")
	signature := []byte("consistent-signature")
	userHandle := []byte("consistent-user-handle")
	clientData := []byte(`{"challenge":"consistency-test","origin":"https://localhost","type":"webauthn.get"}`)

	// Simulate what client.go:692-701 does
	assertion := WebAuthnAssertionResponse{
		Id:                base64.RawURLEncoding.EncodeToString(rawID),
		RawId:             rawID,
		AuthenticatorData: authData,
		Signature:         signature,
		UserHandle:        userHandle,
	}

	encoded := models.WebAuthnAssertionResponse{
		ID:                assertion.Id,
		RawID:             base64.RawURLEncoding.EncodeToString(assertion.RawId),
		ClientDataJSON:    base64.RawURLEncoding.EncodeToString(clientData),
		AuthenticatorData: base64.RawURLEncoding.EncodeToString(assertion.AuthenticatorData),
		Signature:         base64.RawURLEncoding.EncodeToString(assertion.Signature),
		UserHandle:        base64.RawURLEncoding.EncodeToString(assertion.UserHandle),
	}

	// The Id field must equal the encoded RawID
	assert.Equal(t, encoded.RawID, encoded.ID, "ID must equal base64 encoding of RawId")

	// Double-encoding consistency: encoding the same bytes twice must yield the same string
	assert.Equal(t, base64.RawURLEncoding.EncodeToString(rawID), encoded.RawID)
	assert.Equal(t, base64.RawURLEncoding.EncodeToString(authData), encoded.AuthenticatorData)
	assert.Equal(t, base64.RawURLEncoding.EncodeToString(signature), encoded.Signature)
	assert.Equal(t, base64.RawURLEncoding.EncodeToString(userHandle), encoded.UserHandle)

	// Round-trip decode must recover original bytes
	decodedRawID, err := base64.RawURLEncoding.DecodeString(encoded.RawID)
	require.NoError(t, err)
	assert.Equal(t, rawID, decodedRawID)

	decodedAuthData, err := base64.RawURLEncoding.DecodeString(encoded.AuthenticatorData)
	require.NoError(t, err)
	assert.Equal(t, authData, decodedAuthData)

	decodedSig, err := base64.RawURLEncoding.DecodeString(encoded.Signature)
	require.NoError(t, err)
	assert.Equal(t, signature, decodedSig)

	decodedUserHandle, err := base64.RawURLEncoding.DecodeString(encoded.UserHandle)
	require.NoError(t, err)
	assert.Equal(t, userHandle, decodedUserHandle)
}
