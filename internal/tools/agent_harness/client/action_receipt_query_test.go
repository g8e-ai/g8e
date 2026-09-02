// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package client

import (
	"context"
	"net/http"
	"net/http/httptest"
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
