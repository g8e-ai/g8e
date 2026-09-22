// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

//go:build integration

package gateway

import (
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/marshaler"
	"github.com/g8e-ai/g8e/v2/internal/models"
	"github.com/g8e-ai/g8e/v2/internal/services/sqliteutil"
	"github.com/g8e-ai/g8e/v2/internal/testutil"
)

// newPublicPublisherTestEnv builds a real in-memory SQLite-backed
// PublicPublisherService with a temp-rooted RuntimeFileService, a generated
// Ed25519 signing key, and a test export config. The mirror is a test HTTP
// server that the caller can configure.
func newPublicPublisherTestEnv(t *testing.T) (*PublicPublisherService, *DocumentStoreService, ed25519.PrivateKey, string) {
	t.Helper()
	logger := testutil.NewTestLogger()
	cfg := sqliteutil.DefaultDBConfig(":memory:")
	db, err := sqliteutil.OpenDB(cfg, logger)
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	_, err = db.Exec(gatewaySchema)
	require.NoError(t, err)

	docStore := NewDocumentStoreService(db, logger)
	fileSvc := newProducerFileSvc(t)

	pub, priv, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)
	keyID := "test-key-1"

	exportCfg := models.PublicExportConfig{
		Enabled:                 true,
		MirrorOrigin:            "http://test-mirror.invalid",
		SourceID:                "test-source-1",
		SigningKeyID:            keyID,
		BatchMaxRecords:         constants.PublicFeedBatchMaxRecords,
		BatchMaxBytes:           constants.PublicFeedBatchMaxBytes,
		RetryMaxAttempts:        constants.PublicFeedRetryMaxAttempts,
		RetryInitialBackoffSecs: 0,
		RetryMaxBackoffSecs:     1,
		AckWindowSecs:           constants.PublicFeedAckWindowSeconds,
	}

	publisher := NewPublicPublisherService(docStore, fileSvc, logger, exportCfg, priv, keyID)
	return publisher, docStore, priv, hex.EncodeToString(pub)
}

// makeProjectionRecord creates a valid public feed record from a projection
// dict.
func makeProjectionRecord(t *testing.T, seq int64, proj map[string]any) models.PublicFeedRecord {
	t.Helper()
	recordBytes, err := json.Marshal(proj)
	require.NoError(t, err)
	recordHash := sha256.Sum256(recordBytes)
	return models.PublicFeedRecord{
		Sequence:    seq,
		RecordType:  models.PublicFeedRecordTypeProjection,
		RecordHash:  hex.EncodeToString(recordHash[:]),
		RecordBytes: string(recordBytes),
	}
}

// TestExportBatch_SignsAndWritesOutbox verifies that ExportBatch builds a
// signed batch from records, writes it to the durable outbox, and sends it
// to the mirror. The batch must carry a valid Ed25519 signature over the
// content hash.
func TestExportBatch_SendsConfiguredIngestToken(t *testing.T) {
	publisher, _, _, _ := newPublicPublisherTestEnv(t)
	publisher.SetIngestAuthToken("mirror-token")
	mirror := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "Bearer mirror-token", r.Header.Get("Authorization"))
		require.NoError(t, json.NewEncoder(w).Encode(models.PublicIngestResponse{Accepted: true}))
	}))
	t.Cleanup(mirror.Close)
	publisher.SetMirrorOrigin(mirror.URL)

	require.NoError(t, publisher.ExportBatch(context.Background(), []models.PublicFeedRecord{
		makeProjectionRecord(t, 1, map[string]any{"campaign_id": "campaign-1"}),
	}))
}

func TestExportBatch_SignsAndWritesOutbox(t *testing.T) {
	publisher, _, _, pubKeyHex := newPublicPublisherTestEnv(t)

	// Set up a test mirror that accepts the batch.
	var receivedBatch models.PublicFeedBatch
	mirror := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req models.PublicIngestRequest
		require.NoError(t, json.NewDecoder(r.Body).Decode(&req))
		receivedBatch = req.Batch
		resp := models.PublicIngestResponse{Accepted: true, HighWaterSequence: req.Batch.LastSequence, FeedChainHash: req.Batch.ContentHash}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	t.Cleanup(mirror.Close)
	publisher.SetMirrorOrigin(mirror.URL)

	records := []models.PublicFeedRecord{
		makeProjectionRecord(t, 1, map[string]any{"campaign_id": "c1", "variant_id": "v1"}),
		makeProjectionRecord(t, 2, map[string]any{"campaign_id": "c1", "variant_id": "v2"}),
	}

	err := publisher.ExportBatch(context.Background(), records)
	require.NoError(t, err)

	// The mirror received a valid batch.
	assert.Equal(t, int64(1), receivedBatch.FirstSequence)
	assert.Equal(t, int64(2), receivedBatch.LastSequence)
	assert.Equal(t, constants.PublicFeedZeroHashHex, receivedBatch.PreviousBatchHash)
	assert.Len(t, receivedBatch.RecordHashes, 2)
	assert.NotEmpty(t, receivedBatch.ContentHash)
	assert.NotEmpty(t, receivedBatch.Signature)

	// Verify the signature is valid.
	contentHashBytes, err := hex.DecodeString(receivedBatch.ContentHash)
	require.NoError(t, err)
	sigBytes, err := hex.DecodeString(receivedBatch.Signature)
	require.NoError(t, err)
	pubKeyBytes, err := hex.DecodeString(pubKeyHex)
	require.NoError(t, err)
	assert.True(t, ed25519.Verify(pubKeyBytes, contentHashBytes, sigBytes), "signature must be valid")

	// The outbox has the entry persisted.
	entries, err := publisher.outbox.List(context.Background())
	require.NoError(t, err)
	require.Len(t, entries, 1, "outbox must have one entry")
}

// TestExportBatch_HashChainLinksBatches verifies that the second batch
// carries the content hash of the first as its previous_batch_hash.
func TestExportBatch_HashChainLinksBatches(t *testing.T) {
	publisher, _, _, _ := newPublicPublisherTestEnv(t)

	var firstBatch, secondBatch models.PublicFeedBatch
	callCount := atomic.Int32{}
	mirror := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req models.PublicIngestRequest
		require.NoError(t, json.NewDecoder(r.Body).Decode(&req))
		if callCount.Add(1) == 1 {
			firstBatch = req.Batch
		} else {
			secondBatch = req.Batch
		}
		resp := models.PublicIngestResponse{Accepted: true, HighWaterSequence: req.Batch.LastSequence, FeedChainHash: req.Batch.ContentHash}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	t.Cleanup(mirror.Close)
	publisher.SetMirrorOrigin(mirror.URL)

	records1 := []models.PublicFeedRecord{makeProjectionRecord(t, 1, map[string]any{"campaign_id": "c1"})}
	err := publisher.ExportBatch(context.Background(), records1)
	require.NoError(t, err)

	records2 := []models.PublicFeedRecord{makeProjectionRecord(t, 2, map[string]any{"campaign_id": "c1"})}
	err = publisher.ExportBatch(context.Background(), records2)
	require.NoError(t, err)

	// The second batch's previous_batch_hash must equal the first batch's content_hash.
	assert.Equal(t, firstBatch.ContentHash, secondBatch.PreviousBatchHash)
	assert.Equal(t, int64(2), secondBatch.FirstSequence)
	assert.Equal(t, int64(2), secondBatch.LastSequence)
}

// TestExportBatch_RejectsEmptyBatch verifies that an empty batch is rejected.
func TestExportBatch_RejectsEmptyBatch(t *testing.T) {
	publisher, _, _, _ := newPublicPublisherTestEnv(t)

	err := publisher.ExportBatch(context.Background(), nil)
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrPublicFeedBatchEmpty)
}

// TestExportBatch_RetriesOnMirrorFailure verifies that the publisher retries
// when the mirror is unreachable and eventually succeeds when the mirror
// comes back.
func TestExportBatch_RetriesOnMirrorFailure(t *testing.T) {
	publisher, _, _, _ := newPublicPublisherTestEnv(t)

	var accepted atomic.Int32
	mirror := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if accepted.Add(1) <= 1 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		var req models.PublicIngestRequest
		require.NoError(t, json.NewDecoder(r.Body).Decode(&req))
		resp := models.PublicIngestResponse{Accepted: true, HighWaterSequence: req.Batch.LastSequence, FeedChainHash: req.Batch.ContentHash}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	t.Cleanup(mirror.Close)
	publisher.SetMirrorOrigin(mirror.URL)

	records := []models.PublicFeedRecord{makeProjectionRecord(t, 1, map[string]any{"campaign_id": "c1"})}
	err := publisher.ExportBatch(context.Background(), records)
	require.NoError(t, err)
	assert.GreaterOrEqual(t, accepted.Load(), int32(2), "must have retried at least once")
}

// TestExportBatch_RejectsMirrorTipThatConflictsWithPublisherSnapshot verifies
// that a different accepted hash at the publisher's next sequence fails closed.
func TestExportBatch_RejectsMirrorTipThatConflictsWithPublisherSnapshot(t *testing.T) {
	publisher, _, _, _ := newPublicPublisherTestEnv(t)
	remoteTipHash := strings.Repeat("a", sha256.Size*2)
	var firstBatch models.PublicFeedBatch
	var receivedSequences []int64
	mirror := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req models.PublicIngestRequest
		require.NoError(t, json.NewDecoder(r.Body).Decode(&req))
		receivedSequences = append(receivedSequences, req.Batch.FirstSequence)
		if len(receivedSequences) == 1 {
			firstBatch = req.Batch
			require.NoError(t, json.NewEncoder(w).Encode(models.PublicIngestResponse{Accepted: true, HighWaterSequence: 1, FeedChainHash: req.Batch.ContentHash}))
			return
		}
		require.Equal(t, int64(2), req.Batch.FirstSequence)
		require.NoError(t, json.NewEncoder(w).Encode(models.PublicIngestResponse{
			Accepted:          false,
			SourceID:          req.Batch.SourceID,
			HighWaterSequence: 2,
			FeedChainHash:     remoteTipHash,
			BatchCount:        2,
			RejectionReason:   models.PublicFeedIngestRejectionDuplicateSequence,
		}))
	}))
	t.Cleanup(mirror.Close)
	publisher.SetMirrorOrigin(mirror.URL)

	require.NoError(t, publisher.ExportBatch(context.Background(), []models.PublicFeedRecord{
		makeProjectionRecord(t, 1, map[string]any{"campaign_id": "campaign-1"}),
	}))
	err := publisher.ExportBatch(context.Background(), []models.PublicFeedRecord{
		makeProjectionRecord(t, 2, map[string]any{"campaign_id": "campaign-2"}),
	})
	require.ErrorIs(t, err, constants.ErrPublicFeedMirrorRejected)
	assert.Equal(t, []int64{1, 2}, receivedSequences)
	snapshot, snapshotErr := publisher.GetSnapshot(context.Background())
	require.NoError(t, snapshotErr)
	assert.Equal(t, int64(1), snapshot.HighWaterSequence)
	assert.Equal(t, firstBatch.ContentHash, snapshot.FeedChainHash)
}

func TestSendToMirror_RejectsUnverifiedMirrorState(t *testing.T) {
	publisher, _, _, _ := newPublicPublisherTestEnv(t)
	batch, err := publisher.BuildBatch([]models.PublicFeedRecord{
		makeProjectionRecord(t, 1, map[string]any{"campaign_id": "campaign-1"}),
	})
	require.NoError(t, err)

	tests := []struct {
		name     string
		response models.PublicIngestResponse
	}{
		{
			name: "different source",
			response: models.PublicIngestResponse{
				SourceID:          "other-source",
				HighWaterSequence: batch.LastSequence,
				FeedChainHash:     batch.ContentHash,
				BatchCount:        1,
				RejectionReason:   models.PublicFeedIngestRejectionDuplicateSequence,
			},
		},
		{
			name: "newer mirror tip",
			response: models.PublicIngestResponse{
				SourceID:          batch.SourceID,
				HighWaterSequence: batch.LastSequence + 1,
				FeedChainHash:     strings.Repeat("a", sha256.Size*2),
				BatchCount:        2,
				RejectionReason:   models.PublicFeedIngestRejectionDuplicateSequence,
			},
		},
		{
			name: "equal sequence with different hash",
			response: models.PublicIngestResponse{
				SourceID:          batch.SourceID,
				HighWaterSequence: batch.LastSequence,
				FeedChainHash:     strings.Repeat("b", sha256.Size*2),
				BatchCount:        1,
				RejectionReason:   models.PublicFeedIngestRejectionDuplicateSequence,
			},
		},
		{
			name: "malformed tip hash",
			response: models.PublicIngestResponse{
				SourceID:          batch.SourceID,
				HighWaterSequence: batch.LastSequence + 1,
				FeedChainHash:     "invalid",
				BatchCount:        2,
				RejectionReason:   models.PublicFeedIngestRejectionDuplicateSequence,
			},
		},
		{
			name: "missing batch count",
			response: models.PublicIngestResponse{
				SourceID:          batch.SourceID,
				HighWaterSequence: batch.LastSequence + 1,
				FeedChainHash:     strings.Repeat("c", sha256.Size*2),
				RejectionReason:   models.PublicFeedIngestRejectionDuplicateSequence,
			},
		},
		{
			name: "non-duplicate rejection",
			response: models.PublicIngestResponse{
				SourceID:          batch.SourceID,
				HighWaterSequence: batch.LastSequence,
				FeedChainHash:     batch.ContentHash,
				BatchCount:        1,
				RejectionReason:   models.PublicFeedIngestRejectionEquivocation,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mirror := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				require.NoError(t, json.NewEncoder(w).Encode(tt.response))
			}))
			t.Cleanup(mirror.Close)
			err := publisher.sendToMirror(context.Background(), mirror.URL, "", batch)
			require.ErrorIs(t, err, constants.ErrPublicFeedMirrorRejected)
		})
	}
}

func TestExportBatch_RecoversWhenMirrorAcceptedBatchBeforeResponseFailure(t *testing.T) {
	publisher, _, _, _ := newPublicPublisherTestEnv(t)
	var accepted models.PublicFeedBatch
	attempts := atomic.Int32{}
	mirror := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req models.PublicIngestRequest
		require.NoError(t, json.NewDecoder(r.Body).Decode(&req))
		if attempts.Add(1) == 1 {
			accepted = req.Batch
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		require.Equal(t, accepted.ContentHash, req.Batch.ContentHash)
		require.NoError(t, json.NewEncoder(w).Encode(models.PublicIngestResponse{
			Accepted:          false,
			SourceID:          accepted.SourceID,
			HighWaterSequence: accepted.LastSequence,
			FeedChainHash:     accepted.ContentHash,
			RejectionReason:   models.PublicFeedIngestRejectionDuplicateSequence,
		}))
	}))
	t.Cleanup(mirror.Close)
	publisher.SetMirrorOrigin(mirror.URL)

	err := publisher.ExportBatch(context.Background(), []models.PublicFeedRecord{
		makeProjectionRecord(t, 1, map[string]any{"campaign_id": "campaign-1"}),
	})

	require.NoError(t, err)
	assert.Equal(t, int32(2), attempts.Load())
	snapshot, err := publisher.GetSnapshot(context.Background())
	require.NoError(t, err)
	assert.Equal(t, accepted.LastSequence, snapshot.HighWaterSequence)
	assert.Equal(t, accepted.ContentHash, snapshot.FeedChainHash)
}

func TestRetransmitOutbox_AcknowledgesBatchAlreadyAcceptedByMirror(t *testing.T) {
	publisher, _, _, _ := newPublicPublisherTestEnv(t)
	records := []models.PublicFeedRecord{makeProjectionRecord(t, 1, map[string]any{"campaign_id": "campaign-1"})}
	batch, err := publisher.BuildBatch(records)
	require.NoError(t, err)
	require.NoError(t, publisher.writeOutboxEntry(context.Background(), batch))
	require.NoError(t, publisher.updateOutboxStatus(context.Background(), batch.LastSequence, models.PublicFeedOutboxStatusSent, true))
	mirror := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req models.PublicIngestRequest
		require.NoError(t, json.NewDecoder(r.Body).Decode(&req))
		require.Equal(t, batch.ContentHash, req.Batch.ContentHash)
		require.NoError(t, json.NewEncoder(w).Encode(models.PublicIngestResponse{
			Accepted:          false,
			SourceID:          batch.SourceID,
			HighWaterSequence: batch.LastSequence,
			FeedChainHash:     batch.ContentHash,
			RejectionReason:   models.PublicFeedIngestRejectionDuplicateSequence,
		}))
	}))
	t.Cleanup(mirror.Close)
	publisher.SetMirrorOrigin(mirror.URL)

	require.NoError(t, publisher.RetransmitOutbox(context.Background()))
	snapshot, err := publisher.GetSnapshot(context.Background())
	require.NoError(t, err)
	assert.Equal(t, batch.LastSequence, snapshot.HighWaterSequence)
	assert.Equal(t, batch.ContentHash, snapshot.FeedChainHash)
}

func TestExportBatch_IdempotentRetransmit(t *testing.T) {
	publisher, _, _, _ := newPublicPublisherTestEnv(t)

	var receivedHashes []string
	mirror := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req models.PublicIngestRequest
		require.NoError(t, json.NewDecoder(r.Body).Decode(&req))
		receivedHashes = append(receivedHashes, req.Batch.ContentHash)
		resp := models.PublicIngestResponse{Accepted: true, HighWaterSequence: req.Batch.LastSequence, FeedChainHash: req.Batch.ContentHash}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	t.Cleanup(mirror.Close)
	publisher.SetMirrorOrigin(mirror.URL)

	records := []models.PublicFeedRecord{makeProjectionRecord(t, 1, map[string]any{"campaign_id": "c1"})}
	batch, err := publisher.BuildBatch(records)
	require.NoError(t, err)
	require.NoError(t, publisher.writeOutboxEntry(context.Background(), batch))
	require.NoError(t, publisher.updateOutboxStatus(context.Background(), batch.LastSequence, models.PublicFeedOutboxStatusSent, true))

	err = publisher.RetransmitOutbox(context.Background())
	require.NoError(t, err)

	require.Len(t, receivedHashes, 1)
}

// TestLoadSnapshotFromOutbox_IgnoresUnacknowledgedTail verifies that only
// mirror-acknowledged outbox entries advance the publisher high-water mark.
func TestLoadSnapshotFromOutbox_IgnoresUnacknowledgedTail(t *testing.T) {
	publisher, _, _, _ := newPublicPublisherTestEnv(t)

	mirror := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req models.PublicIngestRequest
		require.NoError(t, json.NewDecoder(r.Body).Decode(&req))
		resp := models.PublicIngestResponse{Accepted: true, HighWaterSequence: req.Batch.LastSequence, FeedChainHash: req.Batch.ContentHash}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	t.Cleanup(mirror.Close)
	publisher.SetMirrorOrigin(mirror.URL)

	records1 := []models.PublicFeedRecord{makeProjectionRecord(t, 1, map[string]any{"campaign_id": "c1"})}
	require.NoError(t, publisher.ExportBatch(context.Background(), records1))

	records2 := []models.PublicFeedRecord{makeProjectionRecord(t, 2, map[string]any{"campaign_id": "c2"})}
	batch2, err := publisher.BuildBatch(records2)
	require.NoError(t, err)
	require.NoError(t, publisher.writeOutboxEntry(context.Background(), batch2))
	require.NoError(t, publisher.updateOutboxStatus(context.Background(), batch2.LastSequence, models.PublicFeedOutboxStatusSent, true))

	publisher.mu.Lock()
	publisher.highWaterSeq = batch2.LastSequence
	publisher.feedChainHash = batch2.ContentHash
	publisher.batchCount = 2
	publisher.mu.Unlock()

	require.NoError(t, publisher.loadSnapshotFromOutbox(context.Background()))

	publisher.mu.Lock()
	highWater := publisher.highWaterSeq
	publisher.mu.Unlock()
	assert.Equal(t, int64(1), highWater)
}

// TestExportBatch_RetriesUnacknowledgedBeforeNewBatch verifies that a sent
// but unacknowledged outbox tail is retransmitted before appending a new batch.
func TestExportBatch_RetriesUnacknowledgedBeforeNewBatch(t *testing.T) {
	publisher, _, _, _ := newPublicPublisherTestEnv(t)

	var receivedSequences []int64
	mirror := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req models.PublicIngestRequest
		require.NoError(t, json.NewDecoder(r.Body).Decode(&req))
		receivedSequences = append(receivedSequences, req.Batch.FirstSequence)
		resp := models.PublicIngestResponse{Accepted: true, HighWaterSequence: req.Batch.LastSequence, FeedChainHash: req.Batch.ContentHash}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	t.Cleanup(mirror.Close)
	publisher.SetMirrorOrigin(mirror.URL)

	records1 := []models.PublicFeedRecord{makeProjectionRecord(t, 1, map[string]any{"campaign_id": "c1"})}
	require.NoError(t, publisher.ExportBatch(context.Background(), records1))

	records2 := []models.PublicFeedRecord{makeProjectionRecord(t, 2, map[string]any{"campaign_id": "c2"})}
	batch2, err := publisher.BuildBatch(records2)
	require.NoError(t, err)
	require.NoError(t, publisher.writeOutboxEntry(context.Background(), batch2))
	require.NoError(t, publisher.updateOutboxStatus(context.Background(), batch2.LastSequence, models.PublicFeedOutboxStatusSent, true))

	records3 := []models.PublicFeedRecord{makeProjectionRecord(t, 3, map[string]any{"campaign_id": "c3"})}
	receivedSequences = nil
	require.NoError(t, publisher.ExportBatch(context.Background(), records3))

	assert.Equal(t, []int64{2, 3}, receivedSequences)

	snap, err := publisher.GetSnapshot(context.Background())
	require.NoError(t, err)
	assert.Equal(t, int64(3), snap.HighWaterSequence)
}

// TestRepairOutboxFromSnapshot_CompactsPrefixGap verifies that a non-contiguous
// outbox prefix is compacted back to the mirror-acknowledged snapshot tip.
func TestRepairOutboxFromSnapshot_CompactsPrefixGap(t *testing.T) {
	publisher, _, _, _ := newPublicPublisherTestEnv(t)

	mirror := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req models.PublicIngestRequest
		require.NoError(t, json.NewDecoder(r.Body).Decode(&req))
		resp := models.PublicIngestResponse{Accepted: true, HighWaterSequence: req.Batch.LastSequence, FeedChainHash: req.Batch.ContentHash}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	t.Cleanup(mirror.Close)
	publisher.SetMirrorOrigin(mirror.URL)

	records1 := []models.PublicFeedRecord{makeProjectionRecord(t, 1, map[string]any{"campaign_id": "c1"})}
	require.NoError(t, publisher.ExportBatch(context.Background(), records1))

	records2 := []models.PublicFeedRecord{makeProjectionRecord(t, 2, map[string]any{"campaign_id": "c2"})}
	require.NoError(t, publisher.ExportBatch(context.Background(), records2))

	entries, err := publisher.outbox.List(context.Background())
	require.NoError(t, err)
	require.Len(t, entries, 2)

	orphanRecord := makeProjectionRecord(t, 1, map[string]any{"campaign_id": "orphan"})
	orphanBatch, err := publisher.BuildBatch([]models.PublicFeedRecord{orphanRecord})
	require.NoError(t, err)
	orphanBytes, err := json.Marshal(orphanBatch)
	require.NoError(t, err)
	orphanEntry := models.PublicOutboxEntry{
		Sequence:   orphanBatch.LastSequence,
		BatchHash:  orphanBatch.ContentHash,
		BatchBytes: string(orphanBytes),
		Status:     models.PublicFeedOutboxStatusFailed,
		CreatedAt:  entries[0].CreatedAt,
	}

	rawStore := publisher.outbox.(*runtimePublicOutboxStore)
	require.NoError(t, rawStore.Replace(context.Background(), []models.PublicOutboxEntry{orphanEntry, entries[1]}))

	_, err = publisher.outbox.List(context.Background())
	require.ErrorIs(t, err, constants.ErrPublicFeedHashChainMismatch)

	require.NoError(t, publisher.RepairOutboxFromSnapshot(context.Background()))

	repaired, err := publisher.outbox.List(context.Background())
	require.NoError(t, err)
	assert.Empty(t, repaired)
}

func TestRepairOutboxFromSnapshot_DropsDiscontinuousTail(t *testing.T) {
	publisher, _, _, _ := newPublicPublisherTestEnv(t)

	snapshot := models.PublicFeedSnapshot{
		SourceID:          publisher.cfg.SourceID,
		HighWaterSequence: 10,
		FeedChainHash:     "abc123",
		BatchCount:        5,
	}
	snapBytes, err := json.Marshal(snapshot)
	require.NoError(t, err)
	require.NoError(t, publisher.fileSvc.WriteFile(context.Background(), constants.PublicFeedSnapshotPath, snapBytes, constants.PermFilePrivate))

	records := []models.PublicFeedRecord{makeProjectionRecord(t, 15, map[string]any{"campaign_id": "orphan-tail"})}
	batch, err := publisher.BuildBatch(records)
	require.NoError(t, err)
	batchBytes, err := json.Marshal(batch)
	require.NoError(t, err)
	entry := models.PublicOutboxEntry{
		Sequence:   batch.LastSequence,
		BatchHash:  batch.ContentHash,
		BatchBytes: string(batchBytes),
		Status:     models.PublicFeedOutboxStatusPending,
		CreatedAt:  time.Now().UTC(),
	}
	rawStore := publisher.outbox.(*runtimePublicOutboxStore)
	require.NoError(t, rawStore.Replace(context.Background(), []models.PublicOutboxEntry{entry}))

	require.NoError(t, publisher.RepairOutboxFromSnapshot(context.Background()))

	repaired, err := publisher.outbox.List(context.Background())
	require.NoError(t, err)
	assert.Empty(t, repaired)
}

// TestExportBatch_RejectsMirrorRejection verifies that a mirror rejection
// (e.g. signature_invalid) causes the publisher to return an error.
func TestExportBatch_RejectsMirrorRejection(t *testing.T) {
	publisher, _, _, _ := newPublicPublisherTestEnv(t)

	mirror := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := models.PublicIngestResponse{Accepted: false, RejectionReason: models.PublicFeedIngestRejectionSignatureInvalid}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	t.Cleanup(mirror.Close)
	publisher.SetMirrorOrigin(mirror.URL)

	records := []models.PublicFeedRecord{makeProjectionRecord(t, 1, map[string]any{"campaign_id": "c1"})}
	err := publisher.ExportBatch(context.Background(), records)
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrPublicFeedMirrorRejected)
}

// TestGetSnapshot_ReturnsCurrentHighWater verifies that GetSnapshot returns
// the current high-water sequence and feed-chain hash after batches are
// exported.
func TestGetSnapshot_ReturnsCurrentHighWater(t *testing.T) {
	publisher, _, _, _ := newPublicPublisherTestEnv(t)

	mirror := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req models.PublicIngestRequest
		require.NoError(t, json.NewDecoder(r.Body).Decode(&req))
		resp := models.PublicIngestResponse{Accepted: true, HighWaterSequence: req.Batch.LastSequence, FeedChainHash: req.Batch.ContentHash}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	t.Cleanup(mirror.Close)
	publisher.SetMirrorOrigin(mirror.URL)

	records := []models.PublicFeedRecord{makeProjectionRecord(t, 1, map[string]any{"campaign_id": "c1"})}
	err := publisher.ExportBatch(context.Background(), records)
	require.NoError(t, err)

	snap, err := publisher.GetSnapshot(context.Background())
	require.NoError(t, err)
	assert.Equal(t, int64(1), snap.HighWaterSequence)
	assert.NotEmpty(t, snap.FeedChainHash)
	assert.Equal(t, 1, snap.BatchCount)
	assert.Equal(t, models.CampaignFreshnessActive, snap.Freshness)
}

// TestGetSnapshot_NotFoundWhenEmpty verifies that GetSnapshot returns
// ErrPublicFeedSnapshotNotFound when no batches have been exported.
func TestGetSnapshot_NotFoundWhenEmpty(t *testing.T) {
	publisher, _, _, _ := newPublicPublisherTestEnv(t)

	_, err := publisher.GetSnapshot(context.Background())
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrPublicFeedSnapshotNotFound)
}

// TestRecoverOutbox_ResumesAfterRestart verifies that the outbox survives a
// publisher restart and pending entries are retransmitted.
func TestRecoverOutbox_ResumesAfterRestart(t *testing.T) {
	publisher1, docStore, priv, _ := newPublicPublisherTestEnv(t)

	var accepted atomic.Int32
	mirror := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		accepted.Add(1)
		var req models.PublicIngestRequest
		require.NoError(t, json.NewDecoder(r.Body).Decode(&req))
		resp := models.PublicIngestResponse{Accepted: true, HighWaterSequence: req.Batch.LastSequence, FeedChainHash: req.Batch.ContentHash}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	t.Cleanup(mirror.Close)
	publisher1.SetMirrorOrigin(mirror.URL)

	// Export a batch with the first publisher.
	records := []models.PublicFeedRecord{makeProjectionRecord(t, 1, map[string]any{"campaign_id": "c1"})}
	err := publisher1.ExportBatch(context.Background(), records)
	require.NoError(t, err)

	// Create a new publisher with the same docStore and key (simulating restart).
	exportCfg := models.PublicExportConfig{
		Enabled:                 true,
		MirrorOrigin:            mirror.URL,
		SourceID:                "test-source-1",
		SigningKeyID:            "test-key-1",
		BatchMaxRecords:         constants.PublicFeedBatchMaxRecords,
		BatchMaxBytes:           constants.PublicFeedBatchMaxBytes,
		RetryMaxAttempts:        constants.PublicFeedRetryMaxAttempts,
		RetryInitialBackoffSecs: 0,
		RetryMaxBackoffSecs:     1,
		AckWindowSecs:           constants.PublicFeedAckWindowSeconds,
	}
	logger := testutil.NewTestLogger()
	publisher2 := NewPublicPublisherService(docStore, publisher1.fileSvc, logger, exportCfg, priv, "test-key-1")
	publisher2.SetMirrorOrigin(mirror.URL)

	// The snapshot should still be available from the first publisher's state.
	snap, err := publisher2.GetSnapshot(context.Background())
	require.NoError(t, err)
	assert.Equal(t, int64(1), snap.HighWaterSequence)
}

func TestRecoverOutbox_UsesRuntimeFileStateAfterDatabaseReplacement(t *testing.T) {
	publisher1, _, priv, _ := newPublicPublisherTestEnv(t)
	mirror := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req models.PublicIngestRequest
		require.NoError(t, json.NewDecoder(r.Body).Decode(&req))
		resp := models.PublicIngestResponse{Accepted: true, HighWaterSequence: req.Batch.LastSequence, FeedChainHash: req.Batch.ContentHash}
		w.Header().Set("Content-Type", "application/json")
		require.NoError(t, json.NewEncoder(w).Encode(resp))
	}))
	t.Cleanup(mirror.Close)
	publisher1.SetMirrorOrigin(mirror.URL)

	err := publisher1.ExportBatch(context.Background(), []models.PublicFeedRecord{
		makeProjectionRecord(t, 1, map[string]any{"campaign_id": "c1"}),
	})
	require.NoError(t, err)

	logger := testutil.NewTestLogger()
	db, err := sqliteutil.OpenDB(sqliteutil.DefaultDBConfig(":memory:"), logger)
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	_, err = db.Exec(gatewaySchema)
	require.NoError(t, err)

	exportCfg := publisher1.cfg
	exportCfg.MirrorOrigin = mirror.URL
	publisher2 := NewPublicPublisherService(NewDocumentStoreService(db, logger), publisher1.fileSvc, logger, exportCfg, priv, "test-key-1")
	snap, err := publisher2.GetSnapshot(context.Background())
	require.NoError(t, err)
	assert.Equal(t, int64(1), snap.HighWaterSequence)
	assert.Equal(t, 1, snap.BatchCount)
}

func TestRecoverOutbox_RejectsCorruptRuntimeFile(t *testing.T) {
	publisher, _, _, _ := newPublicPublisherTestEnv(t)
	require.NoError(t, publisher.fileSvc.WriteFile(context.Background(), constants.PublicFeedOutboxPath, []byte("{invalid\n"), constants.PermFilePrivate))

	_, err := publisher.GetSnapshot(context.Background())
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrPublicFeedOutboxCorrupt)
}

func TestRecoverOutbox_RejectsSnapshotEquivocationAtHighWater(t *testing.T) {
	publisher1, docStore, priv, _ := newPublicPublisherTestEnv(t)
	mirror := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req models.PublicIngestRequest
		require.NoError(t, json.NewDecoder(r.Body).Decode(&req))
		response := models.PublicIngestResponse{Accepted: true, HighWaterSequence: req.Batch.LastSequence, FeedChainHash: req.Batch.ContentHash}
		w.Header().Set("Content-Type", "application/json")
		require.NoError(t, json.NewEncoder(w).Encode(response))
	}))
	t.Cleanup(mirror.Close)
	publisher1.SetMirrorOrigin(mirror.URL)
	require.NoError(t, publisher1.ExportBatch(context.Background(), []models.PublicFeedRecord{
		makeProjectionRecord(t, 1, map[string]any{"campaign_id": "c1"}),
	}))

	snapshotBytes, err := publisher1.fileSvc.ReadFile(context.Background(), constants.PublicFeedSnapshotPath)
	require.NoError(t, err)
	var snapshot models.PublicFeedSnapshot
	require.NoError(t, json.Unmarshal(snapshotBytes, &snapshot))
	snapshot.FeedChainHash = constants.PublicFeedZeroHashHex
	snapshotBytes, err = json.Marshal(snapshot)
	require.NoError(t, err)
	require.NoError(t, publisher1.fileSvc.WriteFile(context.Background(), constants.PublicFeedSnapshotPath, snapshotBytes, constants.PermFilePrivate))

	publisher2 := NewPublicPublisherService(docStore, publisher1.fileSvc, testutil.NewTestLogger(), publisher1.cfg, priv, "test-key-1")
	_, err = publisher2.GetSnapshot(context.Background())
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrPublicFeedOutboxEquivocation)
}

// TestExportBatch_DisabledReturnsError verifies that exporting when the
// publisher is disabled returns ErrPublicFeedDisabled.
func TestExportBatch_DisabledReturnsError(t *testing.T) {
	logger := testutil.NewTestLogger()
	cfg := sqliteutil.DefaultDBConfig(":memory:")
	db, err := sqliteutil.OpenDB(cfg, logger)
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	_, err = db.Exec(gatewaySchema)
	require.NoError(t, err)
	docStore := NewDocumentStoreService(db, logger)
	fileSvc := newProducerFileSvc(t)
	_, priv, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)

	disabledCfg := models.PublicExportConfig{Enabled: false}
	publisher := NewPublicPublisherService(docStore, fileSvc, logger, disabledCfg, priv, "key-1")

	records := []models.PublicFeedRecord{makeProjectionRecord(t, 1, map[string]any{"campaign_id": "c1"})}
	err = publisher.ExportBatch(context.Background(), records)
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrPublicFeedDisabled)
}

// TestExportBatch_RejectsOversizedBatch verifies that a batch exceeding the
// max records limit is rejected.
func TestExportBatch_RejectsOversizedBatch(t *testing.T) {
	publisher, _, _, _ := newPublicPublisherTestEnv(t)
	publisher.SetBatchMaxRecords(2)

	mirror := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req models.PublicIngestRequest
		require.NoError(t, json.NewDecoder(r.Body).Decode(&req))
		resp := models.PublicIngestResponse{Accepted: true, HighWaterSequence: req.Batch.LastSequence, FeedChainHash: req.Batch.ContentHash}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	t.Cleanup(mirror.Close)
	publisher.SetMirrorOrigin(mirror.URL)

	records := []models.PublicFeedRecord{
		makeProjectionRecord(t, 1, map[string]any{"campaign_id": "c1"}),
		makeProjectionRecord(t, 2, map[string]any{"campaign_id": "c1"}),
		makeProjectionRecord(t, 3, map[string]any{"campaign_id": "c1"}),
	}
	err := publisher.ExportBatch(context.Background(), records)
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrPublicFeedBatchOversized)
}

// TestExportBatch_RecordHashMismatch verifies that a record with a wrong
// hash is rejected before signing.
func TestBuildBatch_RejectsNestedProhibitedField(t *testing.T) {
	publisher, _, _, _ := newPublicPublisherTestEnv(t)
	record := makeProjectionRecord(t, 1, map[string]any{
		"campaign_id": "c1",
		"metadata": map[string]any{
			"api_key": "restricted",
		},
	})

	_, err := publisher.BuildBatch([]models.PublicFeedRecord{record})
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrPublicFeedRestrictedField)
}

func TestBuildBatch_RejectsPrivateIdentityAndRuntimeFields(t *testing.T) {
	publisher, _, _, _ := newPublicPublisherTestEnv(t)
	fields := []string{"user_id", "cli_session_id", "operator_id", "operator_session_id", "authenticated_identity", "endpoint", "filesystem_path", "target_resource"}
	for _, field := range fields {
		t.Run(field, func(t *testing.T) {
			body := []byte(`{"kind":"evaluation_summary","native_result":{"` + field + `":"restricted"}}`)
			digest := sha256.Sum256(body)
			_, err := publisher.BuildBatch([]models.PublicFeedRecord{{Sequence: 1, RecordType: models.PublicFeedRecordTypeProjection, RecordHash: hex.EncodeToString(digest[:]), RecordBytes: string(body)}})
			require.Error(t, err)
			assert.ErrorIs(t, err, constants.ErrPublicFeedRestrictedField)
		})
	}
}

func TestExportBatch_RecordHashMismatch(t *testing.T) {
	publisher, _, _, _ := newPublicPublisherTestEnv(t)

	mirror := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := models.PublicIngestResponse{Accepted: true}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	t.Cleanup(mirror.Close)
	publisher.SetMirrorOrigin(mirror.URL)

	records := []models.PublicFeedRecord{
		{
			Sequence:    1,
			RecordType:  models.PublicFeedRecordTypeProjection,
			RecordHash:  "0000000000000000000000000000000000000000000000000000000000000000",
			RecordBytes: `{"campaign_id":"c1"}`,
		},
	}
	err := publisher.ExportBatch(context.Background(), records)
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrPublicFeedRecordHashMismatch)
}

// TestExportBatch_RejectsInvalidRecordType verifies that an invalid record
// type is rejected.
func TestExportBatch_RejectsInvalidRecordType(t *testing.T) {
	publisher, _, _, _ := newPublicPublisherTestEnv(t)

	mirror := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := models.PublicIngestResponse{Accepted: true}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	t.Cleanup(mirror.Close)
	publisher.SetMirrorOrigin(mirror.URL)

	recordBytes := `{"campaign_id":"c1"}`
	recordHash := sha256.Sum256([]byte(recordBytes))
	records := []models.PublicFeedRecord{
		{
			Sequence:    1,
			RecordType:  PublicFeedRecordTypeInvalid,
			RecordHash:  hex.EncodeToString(recordHash[:]),
			RecordBytes: recordBytes,
		},
	}
	err := publisher.ExportBatch(context.Background(), records)
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrPublicFeedRecordTypeInvalid)
}

// TestVerifyBatch_SignatureValidation verifies that VerifyBatch accepts a
// valid signature and rejects a tampered signature.
func TestVerifyBatch_SignatureValidation(t *testing.T) {
	publisher, _, _, pubKeyHex := newPublicPublisherTestEnv(t)

	recordBytes := `{"campaign_id":"c1"}`
	recordHash := sha256.Sum256([]byte(recordBytes))
	records := []models.PublicFeedRecord{
		{
			Sequence:    1,
			RecordType:  models.PublicFeedRecordTypeProjection,
			RecordHash:  hex.EncodeToString(recordHash[:]),
			RecordBytes: recordBytes,
		},
	}

	batch, err := publisher.BuildBatch(records)
	require.NoError(t, err)

	// Valid signature.
	err = publisher.VerifyBatch(batch, pubKeyHex)
	require.NoError(t, err)

	// Tampered signature.
	tampered := batch
	tamperedSig, err := hex.DecodeString(batch.Signature)
	require.NoError(t, err)
	tamperedSig[0] ^= 0xFF
	tampered.Signature = hex.EncodeToString(tamperedSig)
	err = publisher.VerifyBatch(tampered, pubKeyHex)
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrPublicFeedSignatureInvalid)
}

// TestVerifyBatch_HashChainValidation verifies that VerifyBatch rejects a
// batch whose content hash does not match the computed hash.
func TestVerifyBatch_HashChainValidation(t *testing.T) {
	publisher, _, _, _ := newPublicPublisherTestEnv(t)

	recordBytes := `{"campaign_id":"c1"}`
	recordHash := sha256.Sum256([]byte(recordBytes))
	records := []models.PublicFeedRecord{
		{
			Sequence:    1,
			RecordType:  models.PublicFeedRecordTypeProjection,
			RecordHash:  hex.EncodeToString(recordHash[:]),
			RecordBytes: recordBytes,
		},
	}

	batch, err := publisher.BuildBatch(records)
	require.NoError(t, err)

	// Tampered content hash.
	tampered := batch
	tampered.ContentHash = constants.PublicFeedZeroHashHex
	err = publisher.VerifyBatch(tampered, hex.EncodeToString(publisher.signingPubKey))
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrPublicFeedContentHashMismatch)
}

// TestRotateKey_SwitchesSigningKey verifies that RotateKey generates a new
// key pair, emits a key revocation record, and subsequent batches are signed
// with the new key.
func TestRotateKey_SwitchesSigningKey(t *testing.T) {
	publisher, _, _, oldPubKeyHex := newPublicPublisherTestEnv(t)

	var receivedKeyIDs []string
	mirror := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/keys/register" {
			require.NoError(t, json.NewEncoder(w).Encode(models.PublicKeyRegistrationResponse{Accepted: true}))
			return
		}
		var req models.PublicIngestRequest
		require.NoError(t, json.NewDecoder(r.Body).Decode(&req))
		receivedKeyIDs = append(receivedKeyIDs, req.Batch.SigningKeyID)
		resp := models.PublicIngestResponse{Accepted: true, HighWaterSequence: req.Batch.LastSequence, FeedChainHash: req.Batch.ContentHash}
		require.NoError(t, json.NewEncoder(w).Encode(resp))
	}))
	t.Cleanup(mirror.Close)
	publisher.SetMirrorOrigin(mirror.URL)

	// Export with the old key.
	records1 := []models.PublicFeedRecord{makeProjectionRecord(t, 1, map[string]any{"campaign_id": "c1"})}
	err := publisher.ExportBatch(context.Background(), records1)
	require.NoError(t, err)

	// Rotate the key.
	newKeyID, newPubKeyHex, err := publisher.RotateKey(context.Background())
	require.NoError(t, err)
	assert.NotEqual(t, "test-key-1", newKeyID)
	assert.NotEqual(t, oldPubKeyHex, newPubKeyHex)

	// Export with the new key.
	records2 := []models.PublicFeedRecord{makeProjectionRecord(t, 3, map[string]any{"campaign_id": "c1"})}
	err = publisher.ExportBatch(context.Background(), records2)
	require.NoError(t, err)

	// The first batch used the old key, the second used the new key.
	require.Len(t, receivedKeyIDs, 3) // batch1 + revocation + batch2
	assert.Equal(t, "test-key-1", receivedKeyIDs[0])
	assert.Equal(t, "test-key-1", receivedKeyIDs[1]) // revocation record signed with old key
	assert.Equal(t, newKeyID, receivedKeyIDs[2])
}

// TestExportBatch_MirrorOutageDoesNotLoseSequence verifies that when the
// mirror is unreachable, the batch remains in the outbox and can be
// retransmitted later without losing sequence.
func TestExportBatch_MirrorOutageDoesNotLoseSequence(t *testing.T) {
	publisher, _, _, _ := newPublicPublisherTestEnv(t)

	// Point to an invalid URL so the mirror is unreachable.
	publisher.SetMirrorOrigin("http://127.0.0.1:1")

	records := []models.PublicFeedRecord{makeProjectionRecord(t, 1, map[string]any{"campaign_id": "c1"})}
	err := publisher.ExportBatch(context.Background(), records)
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrPublicFeedMaxRetriesExceeded)

	// The outbox has the pending entry.
	entries, err := publisher.outbox.List(context.Background())
	require.NoError(t, err)
	require.Len(t, entries, 1, "outbox must retain the pending entry")
	assert.Equal(t, models.PublicFeedOutboxStatusFailed, entries[0].Status)
	assert.Equal(t, constants.PublicFeedRetryMaxAttempts, entries[0].Attempts)

	// Now set up a working mirror and retransmit.
	var accepted atomic.Int32
	mirror := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		accepted.Add(1)
		var req models.PublicIngestRequest
		require.NoError(t, json.NewDecoder(r.Body).Decode(&req))
		resp := models.PublicIngestResponse{Accepted: true, HighWaterSequence: req.Batch.LastSequence, FeedChainHash: req.Batch.ContentHash}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	t.Cleanup(mirror.Close)
	publisher.SetMirrorOrigin(mirror.URL)

	err = publisher.RetransmitOutbox(context.Background())
	require.NoError(t, err)
	assert.GreaterOrEqual(t, accepted.Load(), int32(1))
}

// TestExportBatch_NeverMutatesAuthoritativeReports verifies that exporting
// a batch does not modify any existing eval projections or download
// artifacts. The export only reads persisted safe projections.
func TestExportBatch_NeverMutatesAuthoritativeReports(t *testing.T) {
	publisher, docStore, _, _ := newPublicPublisherTestEnv(t)

	// Persist an eval projection.
	evalCollection := marshaler.CollectionName(constants.CollectionObserveEvals)
	originalProj := evalProjection{
		UserID: "user-1",
		EvalDetail: models.EvalDetail{
			SchemaVersion: constants.ObserveAPIReadModelSchemaVersion,
			RunID:         "run-1",
			SuiteID:       "suite-1",
			SuiteVersion:  "1.0.0",
			CampaignID:    "campaign-1",
			Status:        models.RunLifecycleStatusCompleted,
		},
	}
	projBytes, err := json.Marshal(originalProj)
	require.NoError(t, err)
	err = docStore.DocSet(evalCollection, "run-1", projBytes)
	require.NoError(t, err)

	mirror := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := models.PublicIngestResponse{Accepted: true}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	t.Cleanup(mirror.Close)
	publisher.SetMirrorOrigin(mirror.URL)

	records := []models.PublicFeedRecord{makeProjectionRecord(t, 1, map[string]any{"campaign_id": "c1"})}
	err = publisher.ExportBatch(context.Background(), records)
	require.NoError(t, err)

	// The eval projection is unchanged.
	doc, err := docStore.DocGet(evalCollection, "run-1")
	require.NoError(t, err)
	require.NotNil(t, doc)
	var afterProj evalProjection
	require.NoError(t, unmarshalDocData(doc, &afterProj))
	assert.Equal(t, originalProj.RunID, afterProj.RunID)
	assert.Equal(t, originalProj.Status, afterProj.Status)
}

// TestExportBatch_RejectsProhibitedFields verifies that a record containing
// prohibited fields (raw_prompt, credentials, etc.) is rejected before
// signing.
func TestExportBatch_RejectsProhibitedFields(t *testing.T) {
	publisher, _, _, _ := newPublicPublisherTestEnv(t)

	mirror := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := models.PublicIngestResponse{Accepted: true}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	t.Cleanup(mirror.Close)
	publisher.SetMirrorOrigin(mirror.URL)

	// A record with a prohibited field.
	badRecord := map[string]any{
		"campaign_id": "c1",
		"raw_prompt":  "some private prompt",
	}
	recordBytes, err := json.Marshal(badRecord)
	require.NoError(t, err)
	recordHash := sha256.Sum256(recordBytes)
	records := []models.PublicFeedRecord{
		{
			Sequence:    1,
			RecordType:  models.PublicFeedRecordTypeProjection,
			RecordHash:  hex.EncodeToString(recordHash[:]),
			RecordBytes: string(recordBytes),
		},
	}
	err = publisher.ExportBatch(context.Background(), records)
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrPublicFeedRestrictedField)
}
