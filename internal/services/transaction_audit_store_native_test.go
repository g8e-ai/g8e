// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package services

import (
	"testing"

	"github.com/g8e-ai/g8e/v2/internal/services/gateway"
	"github.com/g8e-ai/g8e/v2/internal/services/governance"
	"github.com/g8e-ai/g8e/v2/internal/services/storage"
	"github.com/stretchr/testify/assert"
)

// TestSQLAuditStore_SatisfiesTransactionAuditStoreNatively asserts that
// *storage.SQLAuditStore satisfies governance.TransactionAuditStore without
// an adapter. Outbound mode wires SQLAuditStore directly into GovernanceDeps;
// the auditStoreTransactionStore adapter existed only because SQLAuditStore
// lacked a DocSet method. This compile-time assertion fails pre-fix.
func TestSQLAuditStore_SatisfiesTransactionAuditStoreNatively(t *testing.T) {
	t.Parallel()
	var _ governance.TransactionAuditStore = (*storage.SQLAuditStore)(nil)
	assert.True(t, true)
}

// TestDocumentStoreService_SatisfiesTransactionAuditStoreNatively asserts
// that *gateway.DocumentStoreService satisfies governance.TransactionAuditStore
// natively (gateway mode wires it directly). This already passes; it is
// co-located with the SQLAuditStore assertion to document the contract that
// both mode-specific audit stores satisfy the interface without adapters.
func TestDocumentStoreService_SatisfiesTransactionAuditStoreNatively(t *testing.T) {
	t.Parallel()
	var _ governance.TransactionAuditStore = (*gateway.DocumentStoreService)(nil)
	assert.True(t, true)
}
