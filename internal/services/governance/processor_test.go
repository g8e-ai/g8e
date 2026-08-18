// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package governance

import (
	"context"
	"testing"

	operatorv1 "github.com/g8e-ai/g8e/protocol/proto/g8e/operator/v1"
	"github.com/stretchr/testify/require"
)

// mockProcessor implements EnvelopeProcessor for testing interface compliance.
type mockProcessor struct {
	receipt *operatorv1.ActionReceipt
	err     error
}

func (m *mockProcessor) ProcessEnvelope(ctx context.Context, payload []byte) (*operatorv1.ActionReceipt, error) {
	return m.receipt, m.err
}

func TestEnvelopeProcessorInterface(t *testing.T) {
	t.Parallel()
	// Verify that mockProcessor implements EnvelopeProcessor
	var _ EnvelopeProcessor = (*mockProcessor)(nil)

	proc := &mockProcessor{
		receipt: &operatorv1.ActionReceipt{TransactionId: "test-tx"},
		err:     nil,
	}

	receipt, err := proc.ProcessEnvelope(context.Background(), []byte("{}"))
	require.NoError(t, err)
	require.Equal(t, "test-tx", receipt.TransactionId)
}
