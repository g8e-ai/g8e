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

package storage

import (
	"crypto/ed25519"
	"database/sql"
	"path/filepath"
	"testing"

	"github.com/g8e-ai/g8e/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestSQLAuditStore_InitDatabase_CommitmentLedgerLowerColumn verifies that
// initDatabase creates the commitment_ledger table with the lowercase
// actuator_intent_signature_digest column directly from the schema DDL, without
// relying on a post-schema ALTER TABLE ... RENAME COLUMN migration. The
// cosmetic rename migration was removed because SQLite's _renameTableTest path
// (triggered by ALTER TABLE ... RENAME COLUMN with foreign_keys=ON) is expensive
// in modernc/sqlite and caused FIPS-mode integration test timeouts.
func TestSQLAuditStore_InitDatabase_CommitmentLedgerLowerColumn(t *testing.T) {
	tempDir := testutil.TempDir(t)

	fileSvc, _ := newTestFileSvc(t, tempDir)

	_, privKey, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)
	testVault := CreateTestVault(t, filepath.Join(tempDir, "vault"), privKey)

	config := &AuditStoreConfig{
		DBPath:               "schema_init.db",
		MaxDBSizeMB:          100,
		RetentionDays:        7,
		PruneIntervalMinutes: 60,
		EncryptionVault:      testVault,
	}

	ass, err := NewSQLAuditStore(config, testutil.NewTestLogger(), fileSvc)
	require.NoError(t, err)
	t.Cleanup(func() { ass.Close() })

	type columnInfo struct {
		CID  int
		Name string
		Type string
	}

	rows, err := ass.db.Query("PRAGMA table_info(commitment_ledger)")
	require.NoError(t, err)
	t.Cleanup(func() { rows.Close() })

	var columns []columnInfo
	for rows.Next() {
		var ci columnInfo
		var notNull, pk int
		var dflt sql.NullString
		require.NoError(t, rows.Scan(&ci.CID, &ci.Name, &ci.Type, &notNull, &dflt, &pk))
		columns = append(columns, ci)
	}
	require.NoError(t, rows.Err())
	require.NotEmpty(t, columns, "commitment_ledger table should have columns")

	names := make(map[string]bool, len(columns))
	for _, c := range columns {
		names[c.Name] = true
	}

	assert.True(t, names["actuator_intent_signature_digest"],
		"commitment_ledger must have lowercase actuator_intent_signature_digest column; got %v", names)
	assert.False(t, names["Actuator_intent_signature_digest"],
		"commitment_ledger must not have mixed-case Actuator_intent_signature_digest column; got %v", names)
}
