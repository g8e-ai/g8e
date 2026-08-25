// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package models

import (
	"encoding/json"
	"testing"

	"github.com/fxamacker/cbor/v2"
	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/go-webauthn/webauthn/protocol"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// OperatorDocumentGo.MarshalJSON
// ---------------------------------------------------------------------------

func TestOperatorDocumentGoMarshalJSON_DefaultsOperatorTypeToSystem(t *testing.T) {
	t.Parallel()

	doc := OperatorDocumentGo{
		ID:        "op-1",
		UserID:    "user-1",
		Component: constants.ComponentNameG8EO,
		Status:    constants.OperatorStatusActive,
	}

	data, err := json.Marshal(&doc)
	require.NoError(t, err)

	var decoded map[string]any
	require.NoError(t, json.Unmarshal(data, &decoded))

	assert.Equal(t, string(constants.OperatorTypeSystem), decoded["operator_type"])
}

func TestOperatorDocumentGoMarshalJSON_PreservesExplicitOperatorType(t *testing.T) {
	t.Parallel()

	doc := OperatorDocumentGo{
		ID:           "op-2",
		UserID:       "user-2",
		Component:    constants.ComponentNameG8EO,
		Status:       constants.OperatorStatusActive,
		OperatorType: constants.OperatorTypeCloud,
	}

	data, err := json.Marshal(&doc)
	require.NoError(t, err)

	var decoded map[string]any
	require.NoError(t, json.Unmarshal(data, &decoded))

	assert.Equal(t, string(constants.OperatorTypeCloud), decoded["operator_type"])
}

func TestOperatorDocumentGoMarshalJSON_PreservesCloudSubtype(t *testing.T) {
	t.Parallel()

	doc := OperatorDocumentGo{
		ID:           "op-3",
		UserID:       "user-3",
		Component:    constants.ComponentNameG8EO,
		Status:       constants.OperatorStatusActive,
		CloudSubtype: constants.CloudSubtypeAWS,
	}

	data, err := json.Marshal(&doc)
	require.NoError(t, err)

	var decoded map[string]any
	require.NoError(t, json.Unmarshal(data, &decoded))

	assert.Equal(t, string(constants.CloudSubtypeAWS), decoded["cloud_subtype"])
}

func TestOperatorDocumentGoMarshalJSON_RoundTrip(t *testing.T) {
	t.Parallel()

	doc := OperatorDocumentGo{
		ID:           "op-4",
		UserID:       "user-4",
		Component:    constants.ComponentNameG8EO,
		Name:         "test-operator",
		Status:       constants.OperatorStatusActive,
		OperatorType: constants.OperatorTypeCloud,
		CloudSubtype: constants.CloudSubtypeAzure,
		IsSlot:       true,
		Claimed:      false,
	}

	data, err := json.Marshal(&doc)
	require.NoError(t, err)

	var decoded OperatorDocumentGo
	require.NoError(t, json.Unmarshal(data, &decoded))

	assert.Equal(t, doc.ID, decoded.ID)
	assert.Equal(t, doc.UserID, decoded.UserID)
	assert.Equal(t, doc.Component, decoded.Component)
	assert.Equal(t, doc.Name, decoded.Name)
	assert.Equal(t, doc.Status, decoded.Status)
	assert.Equal(t, doc.OperatorType, decoded.OperatorType)
	assert.Equal(t, doc.CloudSubtype, decoded.CloudSubtype)
	assert.Equal(t, doc.IsSlot, decoded.IsSlot)
	assert.Equal(t, doc.Claimed, decoded.Claimed)
}

// ---------------------------------------------------------------------------
// User.WebAuthnID
// ---------------------------------------------------------------------------

func TestUserWebAuthnID_ValidUUIDReturnsBytes(t *testing.T) {
	t.Parallel()

	u := &User{
		ID:             "user-1",
		WebAuthnUserID: "550e8400-e29b-41d4-a716-446655440000",
	}

	got := u.WebAuthnID()
	require.Len(t, got, 16)

	// Verify the bytes match the UUID
	expected := []byte{0x55, 0x0e, 0x84, 0x00, 0xe2, 0x9b, 0x41, 0xd4, 0xa7, 0x16, 0x44, 0x66, 0x55, 0x44, 0x00, 0x00}
	assert.Equal(t, expected, got)
}

func TestUserWebAuthnID_InvalidUUIDReturnsNil(t *testing.T) {
	t.Parallel()

	u := &User{
		ID:             "user-1",
		WebAuthnUserID: "not-a-uuid",
	}

	assert.Nil(t, u.WebAuthnID())
}

func TestUserWebAuthnID_EmptyUUIDReturnsNil(t *testing.T) {
	t.Parallel()

	u := &User{
		ID:             "user-1",
		WebAuthnUserID: "",
	}

	assert.Nil(t, u.WebAuthnID())
}

// ---------------------------------------------------------------------------
// User.WebAuthnName
// ---------------------------------------------------------------------------

func TestUserWebAuthnName_WithLocalOSUserReturnsUsername(t *testing.T) {
	t.Parallel()

	u := &User{
		ID: "user-1",
		LocalOSUser: &LocalOSUser{
			Username: "admin",
			Domain:   "CORP",
		},
	}

	assert.Equal(t, "admin", u.WebAuthnName())
}

func TestUserWebAuthnName_WithoutLocalOSUserReturnsUserID(t *testing.T) {
	t.Parallel()

	u := &User{
		ID: "user-1",
	}

	assert.Equal(t, "user-1", u.WebAuthnName())
}

func TestUserWebAuthnName_WithEmptyLocalOSUserUsernameReturnsUserID(t *testing.T) {
	t.Parallel()

	u := &User{
		ID: "user-1",
		LocalOSUser: &LocalOSUser{
			Domain: "CORP",
		},
	}

	assert.Equal(t, "user-1", u.WebAuthnName())
}

// ---------------------------------------------------------------------------
// User.WebAuthnDisplayName
// ---------------------------------------------------------------------------

func TestUserWebAuthnDisplayName_WithLocalOSUserReturnsUsername(t *testing.T) {
	t.Parallel()

	u := &User{
		ID: "user-1",
		LocalOSUser: &LocalOSUser{
			Username: "admin",
		},
	}

	assert.Equal(t, "admin", u.WebAuthnDisplayName())
}

func TestUserWebAuthnDisplayName_WithoutLocalOSUserReturnsUserID(t *testing.T) {
	t.Parallel()

	u := &User{
		ID: "user-1",
	}

	assert.Equal(t, "user-1", u.WebAuthnDisplayName())
}

// ---------------------------------------------------------------------------
// User.WebAuthnIcon
// ---------------------------------------------------------------------------

func TestUserWebAuthnIcon_AlwaysReturnsEmpty(t *testing.T) {
	t.Parallel()

	u := &User{ID: "user-1"}
	assert.Equal(t, "", u.WebAuthnIcon())
}

// ---------------------------------------------------------------------------
// User.WebAuthnCredentials
// ---------------------------------------------------------------------------

func TestUserWebAuthnCredentials_EmptyCredentialsReturnsEmptySlice(t *testing.T) {
	t.Parallel()

	u := &User{ID: "user-1"}
	creds := u.WebAuthnCredentials()

	assert.Len(t, creds, 0)
}

func TestUserWebAuthnCredentials_ConvertsPasskeyCredentialsToWebAuthnFormat(t *testing.T) {
	t.Parallel()

	coseKey, err := cbor.Marshal(map[int]any{
		1:  2,
		3:  -7,
		-1: 1,
		-2: []byte{0x01, 0x02, 0x03, 0x04},
		-3: []byte{0x05, 0x06, 0x07, 0x08},
	})
	require.NoError(t, err)

	u := &User{
		ID: "user-1",
		PasskeyCredentials: []PasskeyCredential{
			{
				ID:              []byte{0xAA, 0xBB},
				PublicKey:       coseKey,
				AttestationType: "none",
				Transport:       []protocol.AuthenticatorTransport{protocol.Internal},
				Authenticator: Authenticator{
					AAGUID:       []byte{0x01, 0x02},
					SignCount:    42,
					CloneWarning: false,
				},
				CreatedAtUnixMs: 1719400000000,
			},
		},
	}

	creds := u.WebAuthnCredentials()
	require.Len(t, creds, 1)

	assert.Equal(t, []byte{0xAA, 0xBB}, creds[0].ID)
	assert.Equal(t, coseKey, creds[0].PublicKey)
	assert.Equal(t, "none", creds[0].AttestationType)
	assert.Equal(t, []protocol.AuthenticatorTransport{protocol.Internal}, creds[0].Transport)
	assert.Equal(t, []byte{0x01, 0x02}, creds[0].Authenticator.AAGUID)
	assert.Equal(t, uint32(42), creds[0].Authenticator.SignCount)
	assert.False(t, creds[0].Authenticator.CloneWarning)
}

func TestUserWebAuthnCredentials_PreservesCredentialOrder(t *testing.T) {
	t.Parallel()

	u := &User{
		ID: "user-1",
		PasskeyCredentials: []PasskeyCredential{
			{ID: []byte{0x01}, AttestationType: "none"},
			{ID: []byte{0x02}, AttestationType: "direct"},
			{ID: []byte{0x03}, AttestationType: "enterprise"},
		},
	}

	creds := u.WebAuthnCredentials()
	require.Len(t, creds, 3)

	assert.Equal(t, []byte{0x01}, creds[0].ID)
	assert.Equal(t, []byte{0x02}, creds[1].ID)
	assert.Equal(t, []byte{0x03}, creds[2].ID)
}

// ---------------------------------------------------------------------------
// User.IsActive
// ---------------------------------------------------------------------------

func TestUserIsActive_NilReceiverReturnsFalse(t *testing.T) {
	t.Parallel()

	var u *User
	assert.False(t, u.IsActive())
}

func TestUserIsActive_ActiveStatusReturnsTrue(t *testing.T) {
	t.Parallel()

	u := &User{
		ID:     "user-1",
		Status: constants.UserStatusActive,
	}

	assert.True(t, u.IsActive())
}

func TestUserIsActive_DisabledStatusReturnsFalse(t *testing.T) {
	t.Parallel()

	u := &User{
		ID:     "user-1",
		Status: constants.UserStatusDisabled,
	}

	assert.False(t, u.IsActive())
}

func TestUserIsActive_EmptyStatusReturnsFalse(t *testing.T) {
	t.Parallel()

	u := &User{
		ID: "user-1",
	}

	assert.False(t, u.IsActive())
}
