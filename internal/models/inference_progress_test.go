// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package models

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	operatorv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/operator/v1"
)

func TestReconcileInferenceProgress_ConcatenatedDeltasMatchOutputHash(t *testing.T) {
	t.Parallel()
	parts := []*operatorv1.InferenceResponsePart{
		{Part: &operatorv1.InferenceResponsePart_Text{Text: "hel"}},
		{Part: &operatorv1.InferenceResponsePart_Text{Text: "lo"}},
	}
	outputHash, err := ComputeInferenceOutputHash(parts, "stop")
	require.NoError(t, err)
	result := &operatorv1.InferenceResult{
		Parts:             parts,
		FinishReason:      "stop",
		OutputHash:        outputHash,
		ProviderAttemptId: "attempt-1",
	}

	err = ReconcileInferenceProgress([]*operatorv1.InferenceProgressEvent{
		{
			ProviderAttemptId: "attempt-1",
			Sequence:          1,
			Parts:             []*operatorv1.InferenceResponsePart{parts[0]},
		},
		{
			ProviderAttemptId: "attempt-1",
			Sequence:          2,
			Parts:             []*operatorv1.InferenceResponsePart{parts[1]},
		},
	}, result)
	assert.NoError(t, err)
}

func TestReconcileInferenceProgress_SequenceGapFailsClosed(t *testing.T) {
	t.Parallel()
	parts := []*operatorv1.InferenceResponsePart{
		{Part: &operatorv1.InferenceResponsePart_Text{Text: "hel"}},
		{Part: &operatorv1.InferenceResponsePart_Text{Text: "lo"}},
	}
	outputHash, err := ComputeInferenceOutputHash(parts, "stop")
	require.NoError(t, err)
	result := &operatorv1.InferenceResult{
		Parts:             parts,
		FinishReason:      "stop",
		OutputHash:        outputHash,
		ProviderAttemptId: "attempt-1",
	}

	err = ReconcileInferenceProgress([]*operatorv1.InferenceProgressEvent{
		{
			ProviderAttemptId: "attempt-1",
			Sequence:          2,
			Parts:             []*operatorv1.InferenceResponsePart{parts[1]},
		},
	}, result)
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrInferenceProgressHashMismatch)
}

func TestReconcileInferenceProgress_MissingEventsWithTerminalPartsFailsClosed(t *testing.T) {
	t.Parallel()
	parts := []*operatorv1.InferenceResponsePart{
		{Part: &operatorv1.InferenceResponsePart_Text{Text: "hello"}},
	}
	outputHash, err := ComputeInferenceOutputHash(parts, "stop")
	require.NoError(t, err)
	result := &operatorv1.InferenceResult{
		Parts:             parts,
		FinishReason:      "stop",
		OutputHash:        outputHash,
		ProviderAttemptId: "attempt-1",
	}

	err = ReconcileInferenceProgress(nil, result)
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrInferenceProgressHashMismatch)
}

func TestReconcileInferenceProgress_MismatchedDeltaHashFailsClosed(t *testing.T) {
	t.Parallel()
	terminal := []*operatorv1.InferenceResponsePart{
		{Part: &operatorv1.InferenceResponsePart_Text{Text: "hello"}},
	}
	outputHash, err := ComputeInferenceOutputHash(terminal, "stop")
	require.NoError(t, err)
	result := &operatorv1.InferenceResult{
		Parts:             terminal,
		FinishReason:      "stop",
		OutputHash:        outputHash,
		ProviderAttemptId: "attempt-1",
	}

	err = ReconcileInferenceProgress([]*operatorv1.InferenceProgressEvent{{
		ProviderAttemptId: "attempt-1",
		Sequence:          1,
		Parts: []*operatorv1.InferenceResponsePart{
			{Part: &operatorv1.InferenceResponsePart_Text{Text: "other"}},
		},
	}}, result)
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrInferenceProgressHashMismatch)
}

func TestReconcileInferenceProgress_ProviderAttemptMismatchFailsClosed(t *testing.T) {
	t.Parallel()
	parts := []*operatorv1.InferenceResponsePart{
		{Part: &operatorv1.InferenceResponsePart_Text{Text: "hello"}},
	}
	outputHash, err := ComputeInferenceOutputHash(parts, "stop")
	require.NoError(t, err)
	result := &operatorv1.InferenceResult{
		Parts:             parts,
		FinishReason:      "stop",
		OutputHash:        outputHash,
		ProviderAttemptId: "attempt-1",
	}

	err = ReconcileInferenceProgress([]*operatorv1.InferenceProgressEvent{{
		ProviderAttemptId: "attempt-other",
		Sequence:          1,
		Parts:             parts,
	}}, result)
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrInferenceProgressHashMismatch)
}
