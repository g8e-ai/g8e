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

package governance

import (
	"context"
	"testing"

	"github.com/g8e-ai/g8e/protocol/proto/g8e/operator/v1"
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
