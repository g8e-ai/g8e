// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package governance

import (
	"context"

	"github.com/g8e-ai/g8e/internal/constants"
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
