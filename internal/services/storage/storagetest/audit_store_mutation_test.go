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

//go:build integration

package storagetest

import (
	"crypto/ed25519"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/g8e-ai/g8e/internal/services/storage"
	"github.com/g8e-ai/g8e/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSQLAuditStore_FileMutation(t *testing.T) {
	t.Parallel()
	tempDir := t.TempDir()
	vaultDir := filepath.Join(tempDir, "vault")

	// Create test vault
	_, privKey, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)
	testVault := createTestVault(t, vaultDir, privKey)

	config := &TestSQLAuditStoreConfig{
		DataDir:                   tempDir,
		DBPath:                    "test.db",
		LedgerDir:                 "ledger",
		MaxDBSizeMB:               100,
		RetentionDays:             7,
		PruneIntervalMinutes:      60,
		Enabled:                   true,
		OutputTruncationThreshold: 102400,
		HeadTailSize:              51200,
		EncryptionVault:           testVault,
	}

	avs, err := NewTestSQLAuditStore(config, testutil.NewTestLogger())
	require.NoError(t, err)
	defer avs.Close()

	// Create a session
	operatorSessionID := "test-session-mutation"
	err = avs.CreateSession(operatorSessionID, "operator", "Mutation Test", "user@example.com")
	require.NoError(t, err)

	// Record a file mutation event
	exitCode := 0
	event := &storage.Event{
		OperatorSessionID:   operatorSessionID,
		Timestamp:           time.Now().UTC(),
		Type:                constants.Event.Operator.FileEdit.Completed,
		ContentText:         "Write config file",
		CommandRaw:          "file_write /etc/nginx/nginx.conf",
		CommandExitCode:     &exitCode,
		ExecutionDurationMs: 25,
	}

	eventID, err := avs.RecordEvent(event)
	require.NoError(t, err)

	// Record file mutation log
	mutation := &storage.FileMutationLog{
		EventID:          eventID,
		Filepath:         "/etc/nginx/nginx.conf",
		Operation:        storage.FileMutationWrite,
		LedgerHashBefore: "abc123",
		LedgerHashAfter:  "def456",
		DiffStat:         "+5 lines, -2 lines",
	}

	err = avs.RecordFileMutation(mutation)
	require.NoError(t, err)

	// Retrieve file mutations
	mutations, err := avs.GetFileMutations(eventID)
	require.NoError(t, err)
	require.Len(t, mutations, 1)

	retrievedMutation := mutations[0]
	assert.Equal(t, "/etc/nginx/nginx.conf", retrievedMutation.Filepath)
	assert.Equal(t, storage.FileMutationWrite, retrievedMutation.Operation)
	assert.Equal(t, "abc123", retrievedMutation.LedgerHashBefore)
	assert.Equal(t, "def456", retrievedMutation.LedgerHashAfter)
}

func TestSQLAuditStore_MultipleFileMutationsPerEvent(t *testing.T) {
	t.Parallel()
	tempDir := t.TempDir()
	vaultDir := filepath.Join(tempDir, "vault")

	// Create test vault
	_, privKey, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)
	testVault := createTestVault(t, vaultDir, privKey)

	config := &TestSQLAuditStoreConfig{
		DataDir:                   tempDir,
		DBPath:                    "test.db",
		LedgerDir:                 "ledger",
		MaxDBSizeMB:               100,
		RetentionDays:             7,
		PruneIntervalMinutes:      60,
		Enabled:                   true,
		OutputTruncationThreshold: 102400,
		HeadTailSize:              51200,
		EncryptionVault:           testVault,
	}

	avs, err := NewTestSQLAuditStore(config, testutil.NewTestLogger())
	require.NoError(t, err)
	defer avs.Close()

	operatorSessionID := "test-multi-mutation-session"
	err = avs.CreateSession(operatorSessionID, "operator", "Multi-Mutation Test", "user@test.com")
	require.NoError(t, err)

	// Create an event
	exitCode := 0
	event := &storage.Event{
		OperatorSessionID: operatorSessionID,
		Timestamp:         time.Now().UTC(),
		Type:              constants.Event.Operator.FileEdit.Completed,
		ContentText:       "Batch file operation",
		CommandExitCode:   &exitCode,
	}
	eventID, err := avs.RecordEvent(event)
	require.NoError(t, err)

	// Record multiple file mutations for the same event
	files := []string{"/etc/nginx/nginx.conf", "/etc/hosts", "/var/log/app.log"}
	for i, file := range files {
		mutation := &storage.FileMutationLog{
			EventID:          eventID,
			Filepath:         file,
			Operation:        storage.FileMutationWrite,
			LedgerHashBefore: fmt.Sprintf("before_%d", i),
			LedgerHashAfter:  fmt.Sprintf("after_%d", i),
			DiffStat:         fmt.Sprintf("+%d lines", i+1),
		}
		err = avs.RecordFileMutation(mutation)
		require.NoError(t, err)
	}

	// Retrieve all mutations for the event
	mutations, err := avs.GetFileMutations(eventID)
	require.NoError(t, err)
	assert.Len(t, mutations, 3)

	// Verify each mutation
	for i, m := range mutations {
		assert.Equal(t, files[i], m.Filepath)
		assert.Equal(t, storage.FileMutationWrite, m.Operation)
	}
}

func TestSQLAuditStore_GetFileMutationsNoMutations(t *testing.T) {
	t.Parallel()
	tempDir := t.TempDir()
	vaultDir := filepath.Join(tempDir, "vault")

	// Create test vault
	_, privKey, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)
	testVault := createTestVault(t, vaultDir, privKey)

	config := &TestSQLAuditStoreConfig{
		DataDir:                   tempDir,
		DBPath:                    "test.db",
		LedgerDir:                 "ledger",
		MaxDBSizeMB:               100,
		RetentionDays:             7,
		PruneIntervalMinutes:      60,
		Enabled:                   true,
		OutputTruncationThreshold: 102400,
		HeadTailSize:              51200,
		EncryptionVault:           testVault,
	}

	avs, err := NewTestSQLAuditStore(config, testutil.NewTestLogger())
	require.NoError(t, err)
	defer avs.Close()

	// Non-existent event ID
	mutations, err := avs.GetFileMutations(99999)
	require.NoError(t, err)
	assert.Len(t, mutations, 0)
}

func TestSQLAuditStore_FileMutationOperationTypes(t *testing.T) {
	t.Parallel()
	tempDir := t.TempDir()
	vaultDir := filepath.Join(tempDir, "vault")

	// Create test vault
	_, privKey, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)
	testVault := createTestVault(t, vaultDir, privKey)

	config := &TestSQLAuditStoreConfig{
		DataDir:                   tempDir,
		DBPath:                    "test.db",
		LedgerDir:                 "ledger",
		MaxDBSizeMB:               100,
		RetentionDays:             7,
		PruneIntervalMinutes:      60,
		Enabled:                   true,
		OutputTruncationThreshold: 102400,
		HeadTailSize:              51200,
		EncryptionVault:           testVault,
	}

	avs, err := NewTestSQLAuditStore(config, testutil.NewTestLogger())
	require.NoError(t, err)
	defer avs.Close()

	operatorSessionID := "test-mutation-types"
	err = avs.CreateSession(operatorSessionID, "operator", "Mutation Types Test", "user@test.com")
	require.NoError(t, err)

	operations := []storage.FileMutationOperation{
		storage.FileMutationWrite,
		storage.FileMutationDelete,
		storage.FileMutationCreate,
	}

	for _, op := range operations {
		event := &storage.Event{
			OperatorSessionID: operatorSessionID,
			Timestamp:         time.Now().UTC(),
			Type:              constants.Event.Operator.FileEdit.Completed,
		}
		eventID, err := avs.RecordEvent(event)
		require.NoError(t, err)

		mutation := &storage.FileMutationLog{
			EventID:   eventID,
			Filepath:  fmt.Sprintf("/test/%s_file.txt", op),
			Operation: op,
		}
		err = avs.RecordFileMutation(mutation)
		require.NoError(t, err)

		// Verify operation type is stored correctly
		mutations, err := avs.GetFileMutations(eventID)
		require.NoError(t, err)
		require.Len(t, mutations, 1)
		assert.Equal(t, op, mutations[0].Operation)
	}
}
