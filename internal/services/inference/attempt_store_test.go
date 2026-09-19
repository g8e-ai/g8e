// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package inference

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/models"
	"github.com/g8e-ai/g8e/v2/internal/services/storage/storagetest"
	operatorv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/operator/v1"
)

func TestAttemptStore_BeginCompleteAndRejectDuplicate(t *testing.T) {
	t.Parallel()
	store, err := NewAttemptStore(storagetest.NewTestFileSvc(t, t.TempDir()))
	require.NoError(t, err)

	ctx := context.Background()
	record := &operatorv1.InferenceProviderAttemptRecord{
		ProviderAttemptId:   "attempt-1",
		TransactionId:       "tx-1",
		RetryCount:          1,
		RetryClassification: models.ClassifyRetry(1),
	}
	require.NoError(t, store.Begin(ctx, record))
	require.NoError(t, store.Complete(ctx, "attempt-1", "digest-1"))

	stored, err := store.Get(ctx, "attempt-1")
	require.NoError(t, err)
	assert.Equal(t, operatorv1.InferenceProviderAttemptStatus_INFERENCE_PROVIDER_ATTEMPT_STATUS_COMPLETED, stored.GetStatus())
	assert.Equal(t, "digest-1", stored.GetResultDigest())

	err = store.Begin(ctx, record)
	assert.ErrorIs(t, err, constants.ErrInferenceProviderAttemptConflict)
}

func TestAttemptStore_FailMarksTerminalState(t *testing.T) {
	t.Parallel()
	store, err := NewAttemptStore(storagetest.NewTestFileSvc(t, t.TempDir()))
	require.NoError(t, err)

	ctx := context.Background()
	require.NoError(t, store.Begin(ctx, &operatorv1.InferenceProviderAttemptRecord{
		ProviderAttemptId: "attempt-2",
		TransactionId:     "tx-2",
	}))
	require.NoError(t, store.Fail(ctx, "attempt-2", "provider unavailable"))

	stored, err := store.Get(ctx, "attempt-2")
	require.NoError(t, err)
	assert.Equal(t, operatorv1.InferenceProviderAttemptStatus_INFERENCE_PROVIDER_ATTEMPT_STATUS_FAILED, stored.GetStatus())
	assert.Equal(t, "provider unavailable", stored.GetFailureSummary())
}
