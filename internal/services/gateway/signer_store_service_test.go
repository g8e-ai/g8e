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

package gateway

import (
	"crypto/ed25519"
	"encoding/hex"
	"testing"
	"time"

	"github.com/g8e-ai/g8e/internal/models"
	"github.com/g8e-ai/g8e/internal/services/sqliteutil"
	"github.com/g8e-ai/g8e/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupSignerStore(t *testing.T) *SignerStoreService {
	t.Helper()
	logger := testutil.NewTestLogger()
	cfg := sqliteutil.DefaultDBConfig(":memory:")
	db, err := sqliteutil.OpenDB(cfg, logger)
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })

	// Initialize schema
	_, err = db.Exec(GatewaySchema())
	require.NoError(t, err)

	return NewSignerStoreService(db, logger)
}

func generateTestPublicKey(t *testing.T) ed25519.PublicKey {
	t.Helper()
	pub, _, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)
	return pub
}

func TestSignerStoreService_AddTrustedSigner(t *testing.T) {
	t.Run("AddTrustedSigner with valid signer", func(t *testing.T) {
		svc := setupSignerStore(t)
		pubKey := generateTestPublicKey(t)
		signer := models.TrustedSigner{
			ID:        "signer-1",
			PublicKey: hex.EncodeToString(pubKey),
			Enabled:   true,
		}

		err := svc.AddTrustedSigner(signer)
		assert.NoError(t, err)

		// Verify it was added
		retrieved, err := svc.GetTrustedSigner("signer-1")
		assert.NoError(t, err)
		assert.Equal(t, pubKey, retrieved)
	})

	t.Run("AddTrustedSigner with empty ID returns error", func(t *testing.T) {
		svc := setupSignerStore(t)
		pubKey := generateTestPublicKey(t)
		signer := models.TrustedSigner{
			ID:        "",
			PublicKey: hex.EncodeToString(pubKey),
			Enabled:   true,
		}

		err := svc.AddTrustedSigner(signer)
		assert.Error(t, err)
	})

	t.Run("AddTrustedSigner with empty public key returns error", func(t *testing.T) {
		svc := setupSignerStore(t)
		signer := models.TrustedSigner{
			ID:        "signer-2",
			PublicKey: "",
			Enabled:   true,
		}

		err := svc.AddTrustedSigner(signer)
		assert.Error(t, err)
	})

	t.Run("AddTrustedSigner auto-sets AddedAt when zero", func(t *testing.T) {
		svc := setupSignerStore(t)
		pubKey := generateTestPublicKey(t)
		signer := models.TrustedSigner{
			ID:        "signer-3",
			PublicKey: hex.EncodeToString(pubKey),
			Enabled:   true,
			AddedAt:   time.Time{}, // Zero time
		}

		err := svc.AddTrustedSigner(signer)
		assert.NoError(t, err)

		// Retrieve and verify AddedAt was set
		list, err := svc.ListTrustedSigners()
		assert.NoError(t, err)
		assert.Len(t, list, 1)
		assert.False(t, list[0].AddedAt.IsZero())
	})

	t.Run("AddTrustedSigner preserves non-zero AddedAt on insert", func(t *testing.T) {
		svc := setupSignerStore(t)
		pubKey := generateTestPublicKey(t)
		expectedTime := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
		signer := models.TrustedSigner{
			ID:        "signer-4",
			PublicKey: hex.EncodeToString(pubKey),
			Enabled:   true,
			AddedAt:   expectedTime,
		}

		err := svc.AddTrustedSigner(signer)
		assert.NoError(t, err)

		// Retrieve and verify AddedAt was preserved
		list, err := svc.ListTrustedSigners()
		assert.NoError(t, err)
		assert.Len(t, list, 1)
		assert.Equal(t, expectedTime, list[0].AddedAt)
	})

	t.Run("AddTrustedSigner upserts existing signer", func(t *testing.T) {
		svc := setupSignerStore(t)
		pubKey1 := generateTestPublicKey(t)
		pubKey2 := generateTestPublicKey(t)

		// Add initial signer
		signer1 := models.TrustedSigner{
			ID:        "signer-5",
			PublicKey: hex.EncodeToString(pubKey1),
			Enabled:   true,
		}
		err := svc.AddTrustedSigner(signer1)
		require.NoError(t, err)

		// Update with new public key (keep enabled so GetTrustedSigner returns it)
		signer2 := models.TrustedSigner{
			ID:        "signer-5",
			PublicKey: hex.EncodeToString(pubKey2),
			Enabled:   true,
		}
		err = svc.AddTrustedSigner(signer2)
		assert.NoError(t, err)

		// Verify update
		retrieved, err := svc.GetTrustedSigner("signer-5")
		assert.NoError(t, err)
		assert.Equal(t, pubKey2, retrieved)
	})
}

func TestSignerStoreService_GetTrustedSigner(t *testing.T) {
	t.Run("GetTrustedSigner returns nil for non-existent signer", func(t *testing.T) {
		svc := setupSignerStore(t)
		pubKey, err := svc.GetTrustedSigner("nonexistent")
		assert.NoError(t, err)
		assert.Nil(t, pubKey)
	})

	t.Run("GetTrustedSigner returns enabled signer", func(t *testing.T) {
		svc := setupSignerStore(t)
		pubKey := generateTestPublicKey(t)
		signer := models.TrustedSigner{
			ID:        "signer-enabled",
			PublicKey: hex.EncodeToString(pubKey),
			Enabled:   true,
		}
		err := svc.AddTrustedSigner(signer)
		require.NoError(t, err)

		retrieved, err := svc.GetTrustedSigner("signer-enabled")
		assert.NoError(t, err)
		assert.Equal(t, pubKey, retrieved)
	})

	t.Run("GetTrustedSigner returns nil for disabled signer", func(t *testing.T) {
		svc := setupSignerStore(t)
		pubKey := generateTestPublicKey(t)
		signer := models.TrustedSigner{
			ID:        "signer-disabled",
			PublicKey: hex.EncodeToString(pubKey),
			Enabled:   false,
		}
		err := svc.AddTrustedSigner(signer)
		require.NoError(t, err)

		retrieved, err := svc.GetTrustedSigner("signer-disabled")
		assert.NoError(t, err)
		assert.Nil(t, retrieved)
	})

	t.Run("GetTrustedSigner handles invalid hex public key", func(t *testing.T) {
		// This test would require direct DB manipulation to insert invalid hex data
		// Since we can't insert invalid data through AddTrustedSigner,
		// we'll skip this test as it requires direct DB manipulation
		t.Skip("Requires direct DB manipulation to insert invalid data")
	})

	t.Run("GetTrustedSigner handles wrong size public key", func(t *testing.T) {
		// Similar to above, requires direct DB manipulation
		t.Skip("Requires direct DB manipulation to insert invalid data")
	})
}

func TestSignerStoreService_ListTrustedSigners(t *testing.T) {
	t.Run("ListTrustedSigners returns empty list when no signers", func(t *testing.T) {
		svc := setupSignerStore(t)
		list, err := svc.ListTrustedSigners()
		assert.NoError(t, err)
		assert.Empty(t, list)
	})

	t.Run("ListTrustedSigners returns all signers", func(t *testing.T) {
		svc := setupSignerStore(t)
		pubKey1 := generateTestPublicKey(t)
		pubKey2 := generateTestPublicKey(t)
		pubKey3 := generateTestPublicKey(t)

		signers := []models.TrustedSigner{
			{ID: "signer-1", PublicKey: hex.EncodeToString(pubKey1), Enabled: true},
			{ID: "signer-2", PublicKey: hex.EncodeToString(pubKey2), Enabled: false},
			{ID: "signer-3", PublicKey: hex.EncodeToString(pubKey3), Enabled: true},
		}

		for _, s := range signers {
			err := svc.AddTrustedSigner(s)
			require.NoError(t, err)
		}

		list, err := svc.ListTrustedSigners()
		assert.NoError(t, err)
		assert.Len(t, list, 3)

		// Verify IDs are preserved
		ids := make([]string, len(list))
		for i, s := range list {
			ids[i] = s.ID
		}
		assert.ElementsMatch(t, []string{"signer-1", "signer-2", "signer-3"}, ids)
	})

	t.Run("ListTrustedSigners includes both enabled and disabled", func(t *testing.T) {
		svc := setupSignerStore(t)
		pubKey1 := generateTestPublicKey(t)
		pubKey2 := generateTestPublicKey(t)

		signer1 := models.TrustedSigner{
			ID:        "enabled-signer",
			PublicKey: hex.EncodeToString(pubKey1),
			Enabled:   true,
		}
		signer2 := models.TrustedSigner{
			ID:        "disabled-signer",
			PublicKey: hex.EncodeToString(pubKey2),
			Enabled:   false,
		}

		err := svc.AddTrustedSigner(signer1)
		require.NoError(t, err)
		err = svc.AddTrustedSigner(signer2)
		require.NoError(t, err)

		list, err := svc.ListTrustedSigners()
		assert.NoError(t, err)
		assert.Len(t, list, 2)
	})

	t.Run("ListTrustedSigners handles corrupt data gracefully", func(t *testing.T) {
		// This would require direct DB manipulation to insert corrupt data
		// The service has continue statements to skip corrupt entries
		t.Skip("Requires direct DB manipulation to insert corrupt data")
	})
}

func TestSignerStoreService_DeleteTrustedSigner(t *testing.T) {
	t.Run("DeleteTrustedSigner returns false for non-existent signer", func(t *testing.T) {
		svc := setupSignerStore(t)
		deleted, err := svc.DeleteTrustedSigner("nonexistent")
		assert.NoError(t, err)
		assert.False(t, deleted)
	})

	t.Run("DeleteTrustedSigner deletes existing signer", func(t *testing.T) {
		svc := setupSignerStore(t)
		pubKey := generateTestPublicKey(t)
		signer := models.TrustedSigner{
			ID:        "signer-to-delete",
			PublicKey: hex.EncodeToString(pubKey),
			Enabled:   true,
		}
		err := svc.AddTrustedSigner(signer)
		require.NoError(t, err)

		// Verify it exists
		retrieved, err := svc.GetTrustedSigner("signer-to-delete")
		require.NoError(t, err)
		require.NotNil(t, retrieved)

		// Delete it
		deleted, err := svc.DeleteTrustedSigner("signer-to-delete")
		assert.NoError(t, err)
		assert.True(t, deleted)

		// Verify it's gone
		retrieved, err = svc.GetTrustedSigner("signer-to-delete")
		assert.NoError(t, err)
		assert.Nil(t, retrieved)
	})

	t.Run("DeleteTrustedSigner can be called multiple times", func(t *testing.T) {
		svc := setupSignerStore(t)
		pubKey := generateTestPublicKey(t)
		signer := models.TrustedSigner{
			ID:        "signer-multi-delete",
			PublicKey: hex.EncodeToString(pubKey),
			Enabled:   true,
		}
		err := svc.AddTrustedSigner(signer)
		require.NoError(t, err)

		// First delete
		deleted, err := svc.DeleteTrustedSigner("signer-multi-delete")
		assert.NoError(t, err)
		assert.True(t, deleted)

		// Second delete
		deleted, err = svc.DeleteTrustedSigner("signer-multi-delete")
		assert.NoError(t, err)
		assert.False(t, deleted)
	})
}

func TestSignerStoreService_HasTrustedSigners(t *testing.T) {
	t.Run("HasTrustedSigners returns false when no signers", func(t *testing.T) {
		svc := setupSignerStore(t)
		has, err := svc.HasTrustedSigners()
		assert.NoError(t, err)
		assert.False(t, has)
	})

	t.Run("HasTrustedSigners returns true when enabled signer exists", func(t *testing.T) {
		svc := setupSignerStore(t)
		pubKey := generateTestPublicKey(t)
		signer := models.TrustedSigner{
			ID:        "signer-enabled",
			PublicKey: hex.EncodeToString(pubKey),
			Enabled:   true,
		}
		err := svc.AddTrustedSigner(signer)
		require.NoError(t, err)

		has, err := svc.HasTrustedSigners()
		assert.NoError(t, err)
		assert.True(t, has)
	})

	t.Run("HasTrustedSigners returns false when only disabled signers exist", func(t *testing.T) {
		svc := setupSignerStore(t)
		pubKey := generateTestPublicKey(t)
		signer := models.TrustedSigner{
			ID:        "signer-disabled",
			PublicKey: hex.EncodeToString(pubKey),
			Enabled:   false,
		}
		err := svc.AddTrustedSigner(signer)
		require.NoError(t, err)

		has, err := svc.HasTrustedSigners()
		assert.NoError(t, err)
		assert.False(t, has)
	})

	t.Run("HasTrustedSigners returns true when mixed signers exist", func(t *testing.T) {
		svc := setupSignerStore(t)
		pubKey1 := generateTestPublicKey(t)
		pubKey2 := generateTestPublicKey(t)

		signer1 := models.TrustedSigner{
			ID:        "signer-enabled-2",
			PublicKey: hex.EncodeToString(pubKey1),
			Enabled:   true,
		}
		signer2 := models.TrustedSigner{
			ID:        "signer-disabled-2",
			PublicKey: hex.EncodeToString(pubKey2),
			Enabled:   false,
		}

		err := svc.AddTrustedSigner(signer1)
		require.NoError(t, err)
		err = svc.AddTrustedSigner(signer2)
		require.NoError(t, err)

		has, err := svc.HasTrustedSigners()
		assert.NoError(t, err)
		assert.True(t, has)
	})
}

func TestSignerStoreService_Integration(t *testing.T) {
	t.Run("Full lifecycle: add, get, list, delete", func(t *testing.T) {
		svc := setupSignerStore(t)
		pubKey := generateTestPublicKey(t)
		signer := models.TrustedSigner{
			ID:        "lifecycle-signer",
			PublicKey: hex.EncodeToString(pubKey),
			Enabled:   true,
		}

		// Add
		err := svc.AddTrustedSigner(signer)
		require.NoError(t, err)

		// Get
		retrieved, err := svc.GetTrustedSigner("lifecycle-signer")
		require.NoError(t, err)
		assert.Equal(t, pubKey, retrieved)

		// List
		list, err := svc.ListTrustedSigners()
		require.NoError(t, err)
		assert.Len(t, list, 1)
		assert.Equal(t, "lifecycle-signer", list[0].ID)

		// Has
		has, err := svc.HasTrustedSigners()
		require.NoError(t, err)
		assert.True(t, has)

		// Delete
		deleted, err := svc.DeleteTrustedSigner("lifecycle-signer")
		require.NoError(t, err)
		assert.True(t, deleted)

		// Verify gone
		retrieved, err = svc.GetTrustedSigner("lifecycle-signer")
		assert.NoError(t, err)
		assert.Nil(t, retrieved)

		has, err = svc.HasTrustedSigners()
		assert.NoError(t, err)
		assert.False(t, has)
	})

	t.Run("Multiple signers with different states", func(t *testing.T) {
		svc := setupSignerStore(t)
		pubKey1 := generateTestPublicKey(t)
		pubKey2 := generateTestPublicKey(t)
		pubKey3 := generateTestPublicKey(t)

		signers := []models.TrustedSigner{
			{ID: "signer-a", PublicKey: hex.EncodeToString(pubKey1), Enabled: true},
			{ID: "signer-b", PublicKey: hex.EncodeToString(pubKey2), Enabled: false},
			{ID: "signer-c", PublicKey: hex.EncodeToString(pubKey3), Enabled: true},
		}

		for _, s := range signers {
			err := svc.AddTrustedSigner(s)
			require.NoError(t, err)
		}

		// List should return all 3
		list, err := svc.ListTrustedSigners()
		require.NoError(t, err)
		assert.Len(t, list, 3)

		// HasTrustedSigners should return true (2 enabled)
		has, err := svc.HasTrustedSigners()
		require.NoError(t, err)
		assert.True(t, has)

		// Get should return only enabled ones
		retrievedA, err := svc.GetTrustedSigner("signer-a")
		require.NoError(t, err)
		assert.NotNil(t, retrievedA)

		retrievedB, err := svc.GetTrustedSigner("signer-b")
		require.NoError(t, err)
		assert.Nil(t, retrievedB) // disabled

		retrievedC, err := svc.GetTrustedSigner("signer-c")
		require.NoError(t, err)
		assert.NotNil(t, retrievedC)

		// Delete one enabled signer
		deleted, err := svc.DeleteTrustedSigner("signer-a")
		require.NoError(t, err)
		assert.True(t, deleted)

		// HasTrustedSigners should still return true (1 enabled left)
		has, err = svc.HasTrustedSigners()
		require.NoError(t, err)
		assert.True(t, has)

		// Delete the other enabled signer
		deleted, err = svc.DeleteTrustedSigner("signer-c")
		require.NoError(t, err)
		assert.True(t, deleted)

		// HasTrustedSigners should now return false (only disabled left)
		has, err = svc.HasTrustedSigners()
		require.NoError(t, err)
		assert.False(t, has)
	})
}
