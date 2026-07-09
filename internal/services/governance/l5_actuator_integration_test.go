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

package governance

import (
	"context"
	"crypto/ed25519"
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	"github.com/g8e-ai/g8e/internal/constants"
	govtypes "github.com/g8e-ai/g8e/internal/governance"
	"github.com/g8e-ai/g8e/internal/services/storage"
	"github.com/g8e-ai/g8e/internal/services/vault"
	"github.com/g8e-ai/g8e/internal/uuid"
	"github.com/stretchr/testify/require"

	operatorv1 "github.com/g8e-ai/g8e/protocol/proto/g8e/operator/v1"
)

func TestL5ActuatorRecordActionReceiptCalled(t *testing.T) {
	actuator, _ := newTestActuator(t)

	// Create a real AuditVault with test database
	tempDir := t.TempDir()

	// Create vault for encryption
	_, privKey, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)
	vaultDir := filepath.Join(tempDir, constants.VaultDirname)
	require.NoError(t, os.MkdirAll(vaultDir, 0700))
	vHeader, _, err := vault.NewVaultHeader(privKey)
	require.NoError(t, err)
	require.NoError(t, vHeader.Save(vaultDir))
	testVault, err := vault.NewVault(&vault.VaultConfig{DataDir: vaultDir, Logger: slog.Default()})
	require.NoError(t, err)
	require.NoError(t, testVault.Unlock(privKey))
	defer testVault.Close()

	auditConfig := &storage.AuditStoreConfig{
		DataDir:         tempDir,
		DBPath:          "test.db",
		MaxDBSizeMB:     100,
		RetentionDays:   1,
		EncryptionVault: testVault,
	}

	auditStore, err := storage.NewSQLAuditStore(auditConfig, slog.Default())
	require.NoError(t, err)
	defer auditStore.Close()

	actuator.SQLAuditStore = auditStore

	// Create the Operator session first (required for fail-closed audit)
	err = auditStore.CreateSession("test-operator-session", "operator", "Test Session", "test-user")
	require.NoError(t, err)

	envelope := &govtypes.GovernanceEnvelope{
		Id:                uuid.NewString(),
		TransactionHash:   "test-hash-1234567890abcdef",
		OperatorId:        "test-operator",
		OperatorSessionId: "test-operator-session",
		ActionType:        string(constants.ActionTypeExecuteBash),
		TargetResource:    "localhost",
	}

	vt := &VerifiedTransaction{
		Envelope:   envelope,
		ActionType: constants.ActionTypeExecuteBash,
	}

	// Execute
	receipt, err := actuator.Execute(context.Background(), vt, nil)
	require.NoError(t, err)
	require.NotNil(t, receipt)

	// Verify RecordActionReceipt was called by querying the audit store
	persistedReceipt, err := auditStore.GetActionReceipt(envelope.Id)
	require.NoError(t, err)
	require.NotNil(t, persistedReceipt, "Receipt should be persisted in audit store")

	// Verify persisted receipt matches the returned receipt
	require.Equal(t, envelope.Id, persistedReceipt.TransactionID)
	require.Equal(t, envelope.TransactionHash, persistedReceipt.TransactionHash)
	require.Equal(t, operatorv1.ExecutionStatus_EXECUTION_STATUS_COMPLETED, persistedReceipt.Status)
	require.Equal(t, "completed", persistedReceipt.ResultSummary)
	require.NotEmpty(t, persistedReceipt.Signature, "Persisted receipt should have signature")
}
