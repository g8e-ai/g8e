// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

//go:build integration

package storage

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/services/sqliteutil"
	"github.com/g8e-ai/g8e/v2/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCommitmentLedger_ConcurrentLedgerInstancesBuildAgainstUniqueHeads(t *testing.T) {
	dbPath := filepath.Join(testutil.TempDir(t), constants.TestCommitmentLedgerDBFilename)
	firstDB, err := sqliteutil.OpenDB(sqliteutil.DefaultDBConfig(dbPath), testutil.NewTestLogger())
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, firstDB.Close()) })
	_, err = firstDB.Exec(auditStoreSchema)
	require.NoError(t, err)
	secondDB, err := sqliteutil.OpenDB(sqliteutil.DefaultDBConfig(dbPath), testutil.NewTestLogger())
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, secondDB.Close()) })
	ledgers := []*CommitmentLedger{
		NewCommitmentLedger(firstDB, testutil.NewTestLogger()),
		NewCommitmentLedger(secondDB, testutil.NewTestLogger()),
	}

	const appendCount = 20
	start := make(chan struct{})
	errs := make(chan error, appendCount)
	var wg sync.WaitGroup
	for i := 0; i < appendCount; i++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			<-start
			err := ledgers[index%len(ledgers)].AppendCommitment(func(priorHash string) ([]byte, string, error) {
				hashBytes := sha256.Sum256([]byte(fmt.Sprintf("%s|%d", priorHash, index)))
				hash := hex.EncodeToString(hashBytes[:])
				payload, marshalErr := json.Marshal(commitmentFields{
					TransactionID:     fmt.Sprintf("tx-%d", index),
					TransactionHash:   fmt.Sprintf("transaction-hash-%d", index),
					CommittedAtUnixMs: time.Now().UnixMilli(),
				})
				return payload, hash, marshalErr
			})
			errs <- err
		}(i)
	}
	close(start)
	wg.Wait()
	close(errs)
	for err := range errs {
		assert.NoError(t, err)
	}

	rows, err := ledgers[0].ListCommitments()
	require.NoError(t, err)
	require.Len(t, rows, appendCount)
	assert.Empty(t, rows[0].PriorCommitmentHash)
	for i := 1; i < len(rows); i++ {
		assert.Equal(t, rows[i-1].Hash, rows[i].PriorCommitmentHash)
	}
}
