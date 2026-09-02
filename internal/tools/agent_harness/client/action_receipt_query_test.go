// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package client

import (
	"context"
	"crypto/ed25519"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/encoding/protojson"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/tools/agent_harness/config"
	operatorv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/operator/v1"
)

func TestGetActionReceiptQueriesCanonicalReceiptByTransactionID(t *testing.T) {
	expected := &operatorv1.ActionReceipt{
		TransactionId: "transaction/1", TransactionHash: "hash-1", SignerKeyId: "signer-1", Signature: "signature-1",
		FinalPersistenceAttestation: &operatorv1.ReceiptPersistenceAttestation{TransactionId: "transaction/1", AuditRecordId: "transaction/1", SignerKeyId: "signer-1", Signature: "persistence-signature-1"},
	}
	body, err := protojson.Marshal(expected)
	require.NoError(t, err)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, constants.APIPaths.AuditReceipts, r.URL.Path)
		assert.Equal(t, "transaction/1", r.URL.Query().Get("tx_id"))
		w.Header().Set("Content-Type", "application/json")
		_, writeErr := w.Write(body)
		assert.NoError(t, writeErr)
	}))
	defer server.Close()
	client, err := New(config.Config{MTLSBaseURL: server.URL})
	require.NoError(t, err)

	receipt, raw, err := client.GetActionReceipt(context.Background(), "transaction/1")

	require.NoError(t, err)
	assert.Equal(t, body, raw)
	assert.Equal(t, expected.GetTransactionId(), receipt.GetTransactionId())
	assert.Equal(t, expected.GetFinalPersistenceAttestation().GetSignature(), receipt.GetFinalPersistenceAttestation().GetSignature())
}

func TestGetActionReceiptFailsClosedOnMalformedCanonicalReceipt(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, writeErr := w.Write([]byte(`{"unknown_field":true}`))
		assert.NoError(t, writeErr)
	}))
	defer server.Close()
	client, err := New(config.Config{MTLSBaseURL: server.URL})
	require.NoError(t, err)

	receipt, _, err := client.GetActionReceipt(context.Background(), "transaction-1")

	require.Error(t, err)
	assert.Nil(t, receipt)
}

func TestGetActionReceiptReturnsNilForMissingTransaction(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()
	client, err := New(config.Config{MTLSBaseURL: server.URL})
	require.NoError(t, err)

	receipt, _, err := client.GetActionReceipt(context.Background(), "missing-transaction")

	require.NoError(t, err)
	assert.Nil(t, receipt)
}

func TestGetTrustedSignerPublicKeyValidatesGatewayResponse(t *testing.T) {
	publicKeyHex := strings.Repeat("ab", ed25519.PublicKeySize)
	tests := []struct {
		name    string
		status  int
		body    string
		wantErr error
		wantKey string
	}{
		{name: "enabled signer", status: http.StatusOK, body: `{"id":"signer-1","public_key_hex":"` + publicKeyHex + `","enabled":true}`, wantKey: publicKeyHex},
		{name: "missing signer", status: http.StatusNotFound, wantErr: constants.ErrTrustedSignerKeyNotFound},
		{name: "gateway failure", status: http.StatusInternalServerError, body: `{"error":"failed"}`},
		{name: "malformed response", status: http.StatusOK, body: `{`},
		{name: "disabled signer", status: http.StatusOK, body: `{"id":"signer-1","public_key_hex":"` + publicKeyHex + `","enabled":false}`, wantErr: constants.ErrTrustedSignerKeyNotFound},
		{name: "malformed public key", status: http.StatusOK, body: `{"id":"signer-1","public_key_hex":"zz","enabled":true}`},
		{name: "wrong public key size", status: http.StatusOK, body: `{"id":"signer-1","public_key_hex":"ab","enabled":true}`, wantErr: constants.ErrTrustedSignerKeyNotFound},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				assert.Equal(t, constants.APIPaths.GovernanceSignersByID+"signer-1", r.URL.Path)
				w.WriteHeader(tt.status)
				_, err := w.Write([]byte(tt.body))
				assert.NoError(t, err)
			}))
			t.Cleanup(server.Close)
			client, err := New(config.Config{MTLSBaseURL: server.URL})
			require.NoError(t, err)

			key, err := client.GetTrustedSignerPublicKey(context.Background(), "signer-1")

			if tt.wantKey != "" {
				require.NoError(t, err)
				assert.Equal(t, tt.wantKey, hex.EncodeToString(key))
				return
			}
			require.Error(t, err)
			assert.Nil(t, key)
			if tt.wantErr != nil {
				assert.ErrorIs(t, err, tt.wantErr)
			}
		})
	}
}
