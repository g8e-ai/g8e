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
