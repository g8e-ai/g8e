// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package storagetest

// This file previously contained all audit store tests.
// Tests have been split into logical groups:
// - audit_store_config_test.go: Configuration, initialization, lifecycle
// - audit_store_session_test.go: Session management
// - audit_store_event_test.go: Event recording, retrieval, pagination, truncation
// - audit_store_mutation_test.go: File mutation operations
// - audit_store_encryption_test.go: Encryption integration tests
// - audit_store_receipt_test.go: ActionReceipt operations
// - audit_store_e2e_test.go: End-to-end audit trail flows
//
// All test files use the //go:build integration tag (Tier 2 tests)
// as they use real on-disk SQLite databases and local PKI generation.
