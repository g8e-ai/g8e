// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package governance

import "encoding/json"

//go:generate mockery --name TransactionAuditStore --output ./mocks --dir .

// TransactionAuditStore defines the interface for persisting audit documents
// to a document store. Implemented natively by DocumentStoreService (gateway
// mode, persists JSON documents) and storage.SQLAuditStore (outbound mode,
// decodes the payload as an ActionReceiptRecord and records it in the receipts
// table). storagetest.TestSQLAuditStore provides a no-op implementation for
// tests. No adapter is required in either production mode.
type TransactionAuditStore interface {
	DocSet(collection, id string, data json.RawMessage) error
}
