// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package governance

import (
	"context"
	"sync"

	"github.com/g8e-ai/g8e/internal/constants"
	govtypes "github.com/g8e-ai/g8e/internal/governance"
	operatorv1 "github.com/g8e-ai/g8e/protocol/proto/g8e/operator/v1"
)

// mockExecutionHandler is a test-only implementation of ExecutionHandler.
type mockExecutionHandler struct {
	executed                       bool
	err                            error
	ExecuteVerifiedTransactionFunc func(ctx context.Context, eventType constants.EventType, cmdMsg CommandMessage) (string, error)
}

func (m *mockExecutionHandler) ExecuteVerifiedTransaction(ctx context.Context, eventType constants.EventType, cmdMsg CommandMessage) (string, error) {
	m.executed = true
	if m.ExecuteVerifiedTransactionFunc != nil {
		return m.ExecuteVerifiedTransactionFunc(ctx, eventType, cmdMsg)
	}
	return "", m.err
}

// mockReceiptPublisher is a test-only implementation of ReceiptPublisher.
// It records the envelope and receipt passed to PublishActionReceipt so
// tests can assert that Execute calls the publisher after successful
// execution and does not call it when signAndLogFinalReceipt fails.
type mockReceiptPublisher struct {
	mu           sync.Mutex
	calls        int
	envelope     *govtypes.GovernanceEnvelope
	receipt      *operatorv1.ActionReceipt
	publishError error
}

func (m *mockReceiptPublisher) PublishActionReceipt(_ context.Context, env *govtypes.GovernanceEnvelope, receipt *operatorv1.ActionReceipt) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.calls++
	m.envelope = env
	m.receipt = receipt
	return m.publishError
}

func (m *mockReceiptPublisher) callCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.calls
}
