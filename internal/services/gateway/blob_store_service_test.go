// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

//go:build integration

package gateway

import (
	"testing"
	"time"

	"github.com/g8e-ai/g8e/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupBlobStoreTest(t *testing.T) (*BlobStoreService, *CanonicalDBService) {
	t.Helper()
	logger := testutil.NewTestLogger()
	fileSvc := newTestFileSvc(t)
	dbDir := testutil.TempDir(t)

	db, stores, err := openTestDB(t, dbDir, fileSvc, logger)
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })

	return stores.BlobStore, db
}

func TestBlobStoreService_BlobPut(t *testing.T) {

	blobStore, _ := setupBlobStoreTest(t)

	t.Run("BlobPut with no expiration (ttlSeconds=0)", func(t *testing.T) {
		data := []byte("test data without expiration")
		err := blobStore.BlobPut("test-ns", "blob-1", data, "text/plain", 0)
		require.NoError(t, err)

		retrievedData, contentType, found := blobStore.BlobGet("test-ns", "blob-1")
		assert.True(t, found)
		assert.Equal(t, "text/plain", contentType)
		assert.Equal(t, data, retrievedData)
	})

	t.Run("BlobPut with positive TTL", func(t *testing.T) {
		data := []byte("test data with TTL")
		err := blobStore.BlobPut("test-ns", "blob-2", data, "application/json", 60)
		require.NoError(t, err)

		retrievedData, contentType, found := blobStore.BlobGet("test-ns", "blob-2")
		assert.True(t, found)
		assert.Equal(t, "application/json", contentType)
		assert.Equal(t, data, retrievedData)
	})

	t.Run("BlobPut with negative TTL (immediately expired)", func(t *testing.T) {
		data := []byte("test data that expires immediately")
		err := blobStore.BlobPut("test-ns", "blob-3", data, "text/plain", -1)
		require.NoError(t, err)

		// Should not be retrievable since it's immediately expired
		retrievedData, contentType, found := blobStore.BlobGet("test-ns", "blob-3")
		assert.False(t, found)
		assert.Empty(t, contentType)
		assert.Nil(t, retrievedData)
	})

	t.Run("BlobPut upsert - replaces existing blob", func(t *testing.T) {
		namespace := "upsert-ns"
		id := "upsert-blob"

		// Insert initial blob
		initialData := []byte("initial data")
		err := blobStore.BlobPut(namespace, id, initialData, "text/plain", 0)
		require.NoError(t, err)

		// Verify initial data
		retrievedData, _, found := blobStore.BlobGet(namespace, id)
		require.True(t, found)
		assert.Equal(t, initialData, retrievedData)

		// Replace with new data
		newData := []byte("updated data")
		err = blobStore.BlobPut(namespace, id, newData, "application/json", 0)
		require.NoError(t, err)

		// Verify new data replaced old data
		retrievedData, contentType, found := blobStore.BlobGet(namespace, id)
		require.True(t, found)
		assert.Equal(t, newData, retrievedData)
		assert.Equal(t, "application/json", contentType)
	})

	t.Run("BlobPut with empty data", func(t *testing.T) {
		data := []byte{}
		err := blobStore.BlobPut("test-ns", "empty-blob", data, "text/plain", 0)
		require.NoError(t, err)

		retrievedData, _, found := blobStore.BlobGet("test-ns", "empty-blob")
		assert.True(t, found)
		// SQLite may return nil for empty byte slices, both are equivalent
		if len(retrievedData) == 0 {
			assert.Equal(t, 0, len(data))
		} else {
			assert.Equal(t, data, retrievedData)
		}
	})

	t.Run("BlobPut with large data", func(t *testing.T) {
		largeData := make([]byte, 1024*1024) // 1MB
		for i := range largeData {
			largeData[i] = byte(i % 256)
		}
		err := blobStore.BlobPut("test-ns", "large-blob", largeData, "application/octet-stream", 0)
		require.NoError(t, err)

		retrievedData, _, found := blobStore.BlobGet("test-ns", "large-blob")
		assert.True(t, found)
		assert.Equal(t, largeData, retrievedData)
	})
}

func TestBlobStoreService_BlobGet(t *testing.T) {

	blobStore, _ := setupBlobStoreTest(t)

	t.Run("BlobGet returns not found for non-existent blob", func(t *testing.T) {
		data, contentType, found := blobStore.BlobGet("nonexistent-ns", "nonexistent-id")
		assert.False(t, found)
		assert.Empty(t, contentType)
		assert.Nil(t, data)
	})

	t.Run("BlobGet returns not found for expired blob", func(t *testing.T) {
		// Insert blob with very short TTL
		data := []byte("expiring soon")
		err := blobStore.BlobPut("expire-ns", "expire-blob", data, "text/plain", 1)
		require.NoError(t, err)

		// Wait for expiration
		time.Sleep(2 * time.Second)

		// Should not be retrievable
		retrievedData, contentType, found := blobStore.BlobGet("expire-ns", "expire-blob")
		assert.False(t, found)
		assert.Empty(t, contentType)
		assert.Nil(t, retrievedData)
	})

	t.Run("BlobGet retrieves data and content type correctly", func(t *testing.T) {
		data := []byte("test data")
		contentType := "application/json"
		err := blobStore.BlobPut("get-ns", "get-blob", data, contentType, 0)
		require.NoError(t, err)

		retrievedData, retrievedContentType, found := blobStore.BlobGet("get-ns", "get-blob")
		assert.True(t, found)
		assert.Equal(t, contentType, retrievedContentType)
		assert.Equal(t, data, retrievedData)
	})

	t.Run("BlobGet with different content types", func(t *testing.T) {
		testCases := []struct {
			name        string
			data        []byte
			contentType string
		}{
			{"text/plain", []byte("plain text"), "text/plain"},
			{"application/json", []byte(`{"key":"value"}`), "application/json"},
			{"application/octet-stream", []byte{0x00, 0x01, 0x02, 0x03}, "application/octet-stream"},
			{"image/png", []byte{0x89, 0x50, 0x4E, 0x47}, "image/png"},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				err := blobStore.BlobPut("content-type-ns", tc.name, tc.data, tc.contentType, 0)
				require.NoError(t, err)

				retrievedData, retrievedContentType, found := blobStore.BlobGet("content-type-ns", tc.name)
				assert.True(t, found)
				assert.Equal(t, tc.contentType, retrievedContentType)
				assert.Equal(t, tc.data, retrievedData)
			})
		}
	})
}

func TestBlobStoreService_BlobMeta(t *testing.T) {

	blobStore, _ := setupBlobStoreTest(t)

	t.Run("BlobMeta returns not found for non-existent blob", func(t *testing.T) {
		meta, found := blobStore.BlobMeta("nonexistent-ns", "nonexistent-id")
		assert.False(t, found)
		assert.Nil(t, meta)
	})

	t.Run("BlobMeta returns not found for expired blob", func(t *testing.T) {
		data := []byte("expiring soon")
		err := blobStore.BlobPut("meta-expire-ns", "meta-expire-blob", data, "text/plain", 1)
		require.NoError(t, err)

		// Wait for expiration
		time.Sleep(2 * time.Second)

		meta, found := blobStore.BlobMeta("meta-expire-ns", "meta-expire-blob")
		assert.False(t, found)
		assert.Nil(t, meta)
	})

	t.Run("BlobMeta returns correct metadata", func(t *testing.T) {
		data := []byte("test data for metadata")
		contentType := "application/json"
		namespace := "meta-ns"
		id := "meta-blob"

		err := blobStore.BlobPut(namespace, id, data, contentType, 0)
		require.NoError(t, err)

		meta, found := blobStore.BlobMeta(namespace, id)
		assert.True(t, found)
		assert.NotNil(t, meta)
		assert.Equal(t, id, meta.ID)
		assert.Equal(t, namespace, meta.Namespace)
		assert.Equal(t, int64(len(data)), meta.Size)
		assert.Equal(t, contentType, meta.ContentType)
		assert.False(t, meta.CreatedAt.IsZero())
	})

	t.Run("BlobMeta does not include data", func(t *testing.T) {
		data := []byte("test data")
		err := blobStore.BlobPut("meta-ns", "meta-no-data", data, "text/plain", 0)
		require.NoError(t, err)

		meta, found := blobStore.BlobMeta("meta-ns", "meta-no-data")
		assert.True(t, found)
		assert.NotNil(t, meta)
		// Verify metadata fields are correct
		assert.Equal(t, "meta-no-data", meta.ID)
		assert.Equal(t, "meta-ns", meta.Namespace)
		assert.Equal(t, int64(len(data)), meta.Size)
		assert.Equal(t, "text/plain", meta.ContentType)
		// BlobRecord struct does not have a Data field by design
		// This test verifies that BlobMeta returns metadata without loading the actual data
	})
}

func TestBlobStoreService_BlobDelete(t *testing.T) {

	blobStore, _ := setupBlobStoreTest(t)

	t.Run("BlobDelete returns false for non-existent blob", func(t *testing.T) {
		deleted, err := blobStore.BlobDelete("nonexistent-ns", "nonexistent-id")
		assert.NoError(t, err)
		assert.False(t, deleted)
	})

	t.Run("BlobDelete deletes existing blob", func(t *testing.T) {
		data := []byte("to be deleted")
		err := blobStore.BlobPut("delete-ns", "delete-blob", data, "text/plain", 0)
		require.NoError(t, err)

		// Verify blob exists
		_, _, found := blobStore.BlobGet("delete-ns", "delete-blob")
		assert.True(t, found)

		// Delete blob
		deleted, err := blobStore.BlobDelete("delete-ns", "delete-blob")
		assert.NoError(t, err)
		assert.True(t, deleted)

		// Verify blob is gone
		_, _, found = blobStore.BlobGet("delete-ns", "delete-blob")
		assert.False(t, found)
	})

	t.Run("BlobDelete can be called multiple times on same blob", func(t *testing.T) {
		data := []byte("delete multiple times")
		err := blobStore.BlobPut("delete-ns", "multi-delete", data, "text/plain", 0)
		require.NoError(t, err)

		// First delete
		deleted, err := blobStore.BlobDelete("delete-ns", "multi-delete")
		assert.NoError(t, err)
		assert.True(t, deleted)

		// Second delete (should return false)
		deleted, err = blobStore.BlobDelete("delete-ns", "multi-delete")
		assert.NoError(t, err)
		assert.False(t, deleted)
	})
}

func TestBlobStoreService_BlobDeleteNamespace(t *testing.T) {

	blobStore, _ := setupBlobStoreTest(t)

	t.Run("BlobDeleteNamespace deletes all blobs in namespace", func(t *testing.T) {
		namespace := "ns-to-delete"

		// Insert multiple blobs
		for i := 0; i < 5; i++ {
			data := []byte{byte(i)}
			err := blobStore.BlobPut(namespace, string(rune('a'+i)), data, "text/plain", 0)
			require.NoError(t, err)
		}

		// Verify blobs exist
		for i := 0; i < 5; i++ {
			_, _, found := blobStore.BlobGet(namespace, string(rune('a'+i)))
			assert.True(t, found)
		}

		// Delete namespace
		count, err := blobStore.BlobDeleteNamespace(namespace)
		assert.NoError(t, err)
		assert.Equal(t, int64(5), count)

		// Verify all blobs are gone
		for i := 0; i < 5; i++ {
			_, _, found := blobStore.BlobGet(namespace, string(rune('a'+i)))
			assert.False(t, found)
		}
	})

	t.Run("BlobDeleteNamespace returns 0 for empty namespace", func(t *testing.T) {
		count, err := blobStore.BlobDeleteNamespace("empty-ns")
		assert.NoError(t, err)
		assert.Equal(t, int64(0), count)
	})

	t.Run("BlobDeleteNamespace does not affect other namespaces", func(t *testing.T) {
		ns1 := "ns-1"
		ns2 := "ns-2"

		// Insert blobs in both namespaces
		err := blobStore.BlobPut(ns1, "blob-1", []byte("data1"), "text/plain", 0)
		require.NoError(t, err)
		err = blobStore.BlobPut(ns2, "blob-2", []byte("data2"), "text/plain", 0)
		require.NoError(t, err)

		// Delete ns1
		count, err := blobStore.BlobDeleteNamespace(ns1)
		assert.NoError(t, err)
		assert.Equal(t, int64(1), count)

		// Verify ns1 is empty
		_, _, found := blobStore.BlobGet(ns1, "blob-1")
		assert.False(t, found)

		// Verify ns2 still has its blob
		_, _, found = blobStore.BlobGet(ns2, "blob-2")
		assert.True(t, found)
	})
}

func TestBlobStoreService_RunMaintenance_Comprehensive(t *testing.T) {

	blobStore, _ := setupBlobStoreTest(t)

	t.Run("RunMaintenance removes expired blobs", func(t *testing.T) {
		// Insert blob with short TTL
		data := []byte("will expire")
		err := blobStore.BlobPut("maint-ns", "expire-blob", data, "text/plain", 1)
		require.NoError(t, err)

		// Insert blob without expiration
		err = blobStore.BlobPut("maint-ns", "no-expire-blob", []byte("will not expire"), "text/plain", 0)
		require.NoError(t, err)

		// Wait for expiration
		time.Sleep(2 * time.Second)

		// Run maintenance
		err = blobStore.RunMaintenance()
		assert.NoError(t, err)

		// Verify expired blob is gone
		_, _, found := blobStore.BlobGet("maint-ns", "expire-blob")
		assert.False(t, found)

		// Verify non-expired blob still exists
		_, _, found = blobStore.BlobGet("maint-ns", "no-expire-blob")
		assert.True(t, found)
	})

	t.Run("RunMaintenance handles no expired blobs gracefully", func(t *testing.T) {
		// Insert blobs without expiration
		for i := 0; i < 3; i++ {
			err := blobStore.BlobPut("maint-ns-2", string(rune('a'+i)), []byte{byte(i)}, "text/plain", 0)
			require.NoError(t, err)
		}

		// Run maintenance (should be no-op)
		err := blobStore.RunMaintenance()
		assert.NoError(t, err)

		// Verify all blobs still exist
		for i := 0; i < 3; i++ {
			_, _, found := blobStore.BlobGet("maint-ns-2", string(rune('a'+i)))
			assert.True(t, found)
		}
	})

	t.Run("RunMaintenance handles empty database gracefully", func(t *testing.T) {
		err := blobStore.RunMaintenance()
		assert.NoError(t, err)
	})

	t.Run("RunMaintenance removes immediately expired blobs (negative TTL)", func(t *testing.T) {
		// Insert blob with negative TTL (immediately expired)
		data := []byte("immediately expired")
		err := blobStore.BlobPut("maint-ns-3", "immediate-expire", data, "text/plain", -1)
		require.NoError(t, err)

		// Run maintenance
		err = blobStore.RunMaintenance()
		assert.NoError(t, err)

		// Verify blob is gone
		_, _, found := blobStore.BlobGet("maint-ns-3", "immediate-expire")
		assert.False(t, found)
	})
}

func TestBlobStoreService_NamespaceIsolation(t *testing.T) {

	blobStore, _ := setupBlobStoreTest(t)

	t.Run("Blobs in different namespaces are isolated", func(t *testing.T) {
		ns1 := "isolated-ns-1"
		ns2 := "isolated-ns-2"
		id := "same-id"

		// Insert same ID in different namespaces
		err := blobStore.BlobPut(ns1, id, []byte("data from ns1"), "text/plain", 0)
		require.NoError(t, err)
		err = blobStore.BlobPut(ns2, id, []byte("data from ns2"), "text/plain", 0)
		require.NoError(t, err)

		// Verify each namespace has its own data
		data1, _, found := blobStore.BlobGet(ns1, id)
		assert.True(t, found)
		assert.Equal(t, []byte("data from ns1"), data1)

		data2, _, found := blobStore.BlobGet(ns2, id)
		assert.True(t, found)
		assert.Equal(t, []byte("data from ns2"), data2)
	})
}

func TestBlobStoreService_ConcurrentOperations(t *testing.T) {

	blobStore, _ := setupBlobStoreTest(t)

	t.Run("Concurrent BlobPut operations", func(t *testing.T) {
		namespace := "concurrent-put"
		done := make(chan bool, 10)

		for i := 0; i < 10; i++ {
			go func(idx int) {
				data := []byte{byte(idx)}
				err := blobStore.BlobPut(namespace, string(rune('a'+idx)), data, "text/plain", 0)
				assert.NoError(t, err)
				done <- true
			}(i)
		}

		// Wait for all goroutines
		for i := 0; i < 10; i++ {
			<-done
		}

		// Verify all blobs were inserted
		for i := 0; i < 10; i++ {
			_, _, found := blobStore.BlobGet(namespace, string(rune('a'+i)))
			assert.True(t, found)
		}
	})

	t.Run("Concurrent BlobGet operations", func(t *testing.T) {
		namespace := "concurrent-get"
		id := "shared-blob"

		// Insert a blob
		err := blobStore.BlobPut(namespace, id, []byte("shared data"), "text/plain", 0)
		require.NoError(t, err)

		done := make(chan bool, 10)

		for i := 0; i < 10; i++ {
			go func() {
				data, _, found := blobStore.BlobGet(namespace, id)
				assert.True(t, found)
				assert.Equal(t, []byte("shared data"), data)
				done <- true
			}()
		}

		// Wait for all goroutines
		for i := 0; i < 10; i++ {
			<-done
		}
	})
}
