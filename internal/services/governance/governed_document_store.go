// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package governance

import "encoding/json"

// GovernedDocumentStore defines the interface for mutating governed documents
// (cases, investigations, memories). Only the gateway-mode DocumentStoreService
// implements this; outbound mode has no governed document store and fails
// closed if a document action reaches it. This is intentionally separate from
// TransactionAuditStore, which persists signed ActionReceipt records and has
// different semantics (receipts are keyed by transaction_id, never merged).
type GovernedDocumentStore interface {
	// DocReplace creates or replaces a document. data must be valid JSON.
	// Used when DocumentUpdateRequested.merge is false.
	DocReplace(collection, id string, data json.RawMessage) error
	// DocMerge merges fields into an existing document. fields must be valid
	// JSON. Returns an error if the document does not exist. Used when
	// DocumentUpdateRequested.merge is true. Null values in fields remove
	// the corresponding key from the document.
	DocMerge(collection, id string, fields json.RawMessage) error
	// DocDelete removes a document. A not-found result is not an error.
	DocDelete(collection, id string) error
}
