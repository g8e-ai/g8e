// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package storage

// This file previously tested gitGetCurrentHash, which was an internal method
// on the test monolith TestSQLAuditStore. GitLedgerService does not expose
// getCurrentHash as a public method - git functionality is tested indirectly
// through public methods like GetFileHistory, GetFileAtCommit, etc. in ledger_test.go.
