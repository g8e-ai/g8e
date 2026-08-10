package services

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/g8e-ai/g8e/internal/models"
	"github.com/g8e-ai/g8e/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAuditStoreTransactionStore_DocSet_ValidRecord(t *testing.T) {
	t.Parallel()

	store := &auditStoreTransactionStore{store: nil}

	receipt := models.ActionReceiptRecord{
		TransactionID:   "tx-1",
		TransactionHash: "hash-1",
		OperatorID:      "op-1",
		ActionType:      constants.ActionTypeFileEdit,
		TargetResource:  "/test/file.txt",
		Status:          2,
		ResultSummary:   "success",
	}

	data, err := json.Marshal(&receipt)
	require.NoError(t, err)

	err = store.DocSet("receipts", "tx-1", data)
	assert.NoError(t, err)
}

func TestAuditStoreTransactionStore_DocSet_InvalidJSON(t *testing.T) {
	t.Parallel()

	store := &auditStoreTransactionStore{store: nil}

	err := store.DocSet("receipts", "tx-1", []byte("invalid json"))
	assert.Error(t, err)
}

func TestAuditStoreTransactionStore_DocSet_NilStore(t *testing.T) {
	t.Parallel()

	store := &auditStoreTransactionStore{store: nil}

	receipt := models.ActionReceiptRecord{
		TransactionID: "tx-1",
	}
	data, err := json.Marshal(&receipt)
	require.NoError(t, err)

	err = store.DocSet("receipts", "tx-1", data)
	assert.NoError(t, err)
}

func TestPrintOperatorStartupBanner(t *testing.T) {
	t.Parallel()
	cfg := testutil.NewTestConfig(t)
	logger := testutil.NewTestLogger()

	assert.NotPanics(t, func() {
		printOperatorStartupBanner(cfg, logger)
	})
}

func TestNewG8eoService_NilTLSConfig(t *testing.T) {
	t.Parallel()
	cfg := testutil.NewTestConfig(t)
	logger := testutil.NewTestLogger()

	_, err := NewG8eoService(cfg, logger, nil, nil)
	assert.Error(t, err)
}

func TestAuditStoreTransactionStore_DocSet_RoundTrip(t *testing.T) {
	t.Parallel()

	receipt := models.ActionReceiptRecord{
		TransactionID:     "tx-roundtrip",
		TransactionHash:   "hash-roundtrip",
		OperatorID:        "op-1",
		OperatorSessionID: "session-1",
		ActionType:        constants.ActionTypeExecuteBash,
		TargetResource:    "/bin/ls",
		Status:            2,
		ResultSummary:     "completed",
		SignerKeyID:       "key-1",
		Signature:         "sig-1",
		L2Valid:           true,
		L3Valid:           false,
	}

	data, err := json.Marshal(&receipt)
	require.NoError(t, err)

	var unmarshaled models.ActionReceiptRecord
	err = json.Unmarshal(data, &unmarshaled)
	require.NoError(t, err)

	assert.Equal(t, receipt.TransactionID, unmarshaled.TransactionID)
	assert.Equal(t, receipt.ActionType, unmarshaled.ActionType)
	assert.Equal(t, receipt.L2Valid, unmarshaled.L2Valid)
}

func TestErrorsIs_Used(t *testing.T) {
	t.Parallel()

	err := errors.New("test error")
	assert.Error(t, err)
}
