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

package services

import (
	"testing"

	"github.com/g8e-ai/g8e/internal/services/gateway"
	"github.com/g8e-ai/g8e/internal/services/governance"
	"github.com/g8e-ai/g8e/internal/services/storage"
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
