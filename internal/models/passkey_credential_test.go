// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package models

import (
	"testing"

	"github.com/fxamacker/cbor/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func validCOSEKey() []byte {
	coseKey := map[int]any{
		1:  2,                                                                                                                                                                                                      // kty: EC2
		3:  -7,                                                                                                                                                                                                     // alg: ES256
		-1: 1,                                                                                                                                                                                                      // crv: P-256
		-2: []byte{0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08, 0x09, 0x0a, 0x0b, 0x0c, 0x0d, 0x0e, 0x0f, 0x10, 0x11, 0x12, 0x13, 0x14, 0x15, 0x16, 0x17, 0x18, 0x19, 0x1a, 0x1b, 0x1c, 0x1d, 0x1e, 0x1f, 0x20}, // x
		-3: []byte{0x21, 0x22, 0x23, 0x24, 0x25, 0x26, 0x27, 0x28, 0x29, 0x2a, 0x2b, 0x2c, 0x2d, 0x2e, 0x2f, 0x30, 0x31, 0x32, 0x33, 0x34, 0x35, 0x36, 0x37, 0x38, 0x39, 0x3a, 0x3b, 0x3c, 0x3d, 0x3e, 0x3f, 0x40}, // y
	}
	data, _ := cbor.Marshal(coseKey)
	return data
}

func TestPasskeyCredentialValidateValid(t *testing.T) {
	t.Parallel()

	c := PasskeyCredential{
		ID:              []byte{0x01, 0x02, 0x03},
		PublicKey:       validCOSEKey(),
		AttestationType: "none",
		CreatedAtUnixMs: 1719400000000,
	}
	assert.NoError(t, c.Validate())
}

func TestPasskeyCredentialValidateEmptyID(t *testing.T) {
	t.Parallel()

	c := PasskeyCredential{
		ID:              []byte{},
		PublicKey:       validCOSEKey(),
		AttestationType: "none",
		CreatedAtUnixMs: 1719400000000,
	}
	err := c.Validate()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "empty")
}

func TestPasskeyCredentialValidateIDTooLong(t *testing.T) {
	t.Parallel()

	c := PasskeyCredential{
		ID:              make([]byte, 1025),
		PublicKey:       validCOSEKey(),
		AttestationType: "none",
		CreatedAtUnixMs: 1719400000000,
	}
	err := c.Validate()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "1024")
}

func TestPasskeyCredentialValidateEmptyPublicKey(t *testing.T) {
	t.Parallel()

	c := PasskeyCredential{
		ID:              []byte{0x01},
		PublicKey:       []byte{},
		AttestationType: "none",
		CreatedAtUnixMs: 1719400000000,
	}
	err := c.Validate()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "COSE")
}

func TestPasskeyCredentialValidateInvalidCBORPublicKey(t *testing.T) {
	t.Parallel()

	c := PasskeyCredential{
		ID:              []byte{0x01},
		PublicKey:       []byte{0xff, 0xff, 0xff, 0xff},
		AttestationType: "none",
		CreatedAtUnixMs: 1719400000000,
	}
	err := c.Validate()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "COSE")
}

func TestPasskeyCredentialValidateUnknownAttestationType(t *testing.T) {
	t.Parallel()

	c := PasskeyCredential{
		ID:              []byte{0x01},
		PublicKey:       validCOSEKey(),
		AttestationType: "self",
		CreatedAtUnixMs: 1719400000000,
	}
	err := c.Validate()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "attestation")
}

func TestPasskeyCredentialValidateZeroTimestamp(t *testing.T) {
	t.Parallel()

	c := PasskeyCredential{
		ID:              []byte{0x01},
		PublicKey:       validCOSEKey(),
		AttestationType: "none",
		CreatedAtUnixMs: 0,
	}
	err := c.Validate()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "zero")
}

func TestPasskeyCredentialValidateAllAttestationTypes(t *testing.T) {
	t.Parallel()

	for _, att := range []string{"none", "indirect", "direct", "enterprise"} {
		att := att
		t.Run(att, func(t *testing.T) {
			t.Parallel()
			c := PasskeyCredential{
				ID:              []byte{0x01},
				PublicKey:       validCOSEKey(),
				AttestationType: att,
				CreatedAtUnixMs: 1719400000000,
			}
			require.NoError(t, c.Validate())
		})
	}
}
