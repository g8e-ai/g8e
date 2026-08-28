// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package protocol_test

import (
	"crypto/ed25519"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/encoding/protojson"

	"github.com/g8e-ai/g8e/v2/internal/services/governance"
	operatorv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/operator/v1"
)

type actionReceiptCanonicalizationVector struct {
	Receipt       json.RawMessage `json:"receipt"`
	CanonicalUTF8 string          `json:"canonical_utf8"`
	PublicKeyHex  string          `json:"public_key_hex"`
}

//go:embed vectors/action_receipt_canonicalization.json
var actionReceiptCanonicalizationVectorJSON []byte

func TestActionReceiptCanonicalizationMatchesCrossLanguageVector(t *testing.T) {
	var vector actionReceiptCanonicalizationVector
	require.NoError(t, json.Unmarshal(actionReceiptCanonicalizationVectorJSON, &vector))

	receipt := &operatorv1.ActionReceipt{}
	require.NoError(t, protojson.Unmarshal(vector.Receipt, receipt))
	assert.Equal(t, operatorv1.L2Status_L2_STATUS_REQUIRED_VALID, receipt.L2Status)
	assert.Equal(t, operatorv1.L3Status_L3_STATUS_REQUIRED_FAILED, receipt.L3Status)

	canonical, err := governance.CanonicalizeActionReceipt(receipt)
	require.NoError(t, err)
	assert.Equal(t, vector.CanonicalUTF8, string(canonical))

	publicKey, err := hex.DecodeString(vector.PublicKeyHex)
	require.NoError(t, err)
	signature, err := hex.DecodeString(receipt.Signature)
	require.NoError(t, err)
	assert.True(t, ed25519.Verify(publicKey, canonical, signature))
}
