// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

//go:build integration

package gateway

import (
	"bufio"
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/models"
	"github.com/g8e-ai/g8e/v2/internal/testutil"
)

// mirrorTestEnv builds a reference mirror server with a registered source
// key, a test HTTP server, and helper functions for building and sending
// signed batches.
type mirrorTestEnv struct {
	t        *testing.T
	mirror   *PublicMirrorServer
	server   *httptest.Server
	pub      ed25519.PublicKey
	priv     ed25519.PrivateKey
	keyID    string
	sourceID string
	client   *http.Client
}

func newMirrorTestEnv(t *testing.T) *mirrorTestEnv {
	t.Helper()
	mirror := NewPublicMirrorServer(testutil.NewTestLogger())
	pub, priv, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)
	keyID := "test-key-1"
	sourceID := "test-source-1"
	mirror.RegisterSourceKey(sourceID, keyID, pub)

	server := httptest.NewServer(mirror.Handler())
	t.Cleanup(server.Close)

	return &mirrorTestEnv{
		t:        t,
		mirror:   mirror,
		server:   server,
		pub:      pub,
		priv:     priv,
		keyID:    keyID,
		sourceID: sourceID,
		client:   server.Client(),
	}
}

// buildBatch builds a signed batch with the given records, previous batch
// hash, and sequence range. The batch is signed with the test private key.
func (e *mirrorTestEnv) buildBatch(records []models.PublicFeedRecord, prevHash string) models.PublicFeedBatch {
	e.t.Helper()
	recordHashes := make([]string, len(records))
	for i, r := range records {
		recordHashes[i] = r.RecordHash
	}
	batch := models.PublicFeedBatch{
		ProtocolVersion:   constants.PublicFeedProtocolVersion,
		SchemaVersion:     constants.PublicFeedSchemaVersion,
		SourceID:          e.sourceID,
		FirstSequence:     records[0].Sequence,
		LastSequence:      records[len(records)-1].Sequence,
		PreviousBatchHash: prevHash,
		RecordHashes:      recordHashes,
		GeneratedAt:       time.Now().UTC(),
		SigningKeyID:      e.keyID,
		Records:           records,
	}
	contentHash := computeBatchContentHash(batch)
	batch.ContentHash = contentHash
	contentHashBytes, _ := hex.DecodeString(contentHash)
	sig := ed25519.Sign(e.priv, contentHashBytes)
	batch.Signature = hex.EncodeToString(sig)
	return batch
}

// makeRecord creates a valid public feed record from a projection dict.
func (e *mirrorTestEnv) makeRecord(seq int64, proj map[string]any) models.PublicFeedRecord {
	e.t.Helper()
	recordBytes, err := json.Marshal(proj)
	require.NoError(e.t, err)
	recordHash := sha256.Sum256(recordBytes)
	return models.PublicFeedRecord{
		Sequence:    seq,
		RecordType:  models.PublicFeedRecordTypeProjection,
		RecordHash:  hex.EncodeToString(recordHash[:]),
		RecordBytes: string(recordBytes),
	}
}

// sendIngest sends a batch to the mirror's ingest endpoint and returns the
// response.
func (e *mirrorTestEnv) sendIngest(batch models.PublicFeedBatch) (int, models.PublicIngestResponse) {
	e.t.Helper()
	reqBody := models.PublicIngestRequest{Batch: batch}
	bodyBytes, err := json.Marshal(reqBody)
	require.NoError(e.t, err)
	resp, err := e.client.Post(e.server.URL+"/ingest", "application/json", bytes.NewReader(bodyBytes))
	require.NoError(e.t, err)
	defer resp.Body.Close()
	var ingestResp models.PublicIngestResponse
	require.NoError(e.t, json.NewDecoder(resp.Body).Decode(&ingestResp))
	return resp.StatusCode, ingestResp
}

// getJSON performs a GET request and decodes the JSON response.
func (e *mirrorTestEnv) getJSON(path string, target any) int {
	e.t.Helper()
	resp, err := e.client.Get(e.server.URL + path)
	require.NoError(e.t, err)
	defer resp.Body.Close()
	if target != nil {
		require.NoError(e.t, json.NewDecoder(resp.Body).Decode(target))
	}
	return resp.StatusCode
}

// ---------------------------------------------------------------------------
// Ingest authentication tests
// ---------------------------------------------------------------------------

// TestMirror_IngestAuth_RejectsMissingToken verifies that the mirror rejects
// ingest requests without a valid bearer token when authentication is
// enabled.
func TestMirror_IngestAuth_RejectsMissingToken(t *testing.T) {
	env := newMirrorTestEnv(t)
	env.mirror.SetIngestAuthToken("secret-token-123")

	records := []models.PublicFeedRecord{env.makeRecord(1, map[string]any{"campaign_id": "c1"})}
	batch := env.buildBatch(records, constants.PublicFeedZeroHashHex)

	reqBody := models.PublicIngestRequest{Batch: batch}
	bodyBytes, err := json.Marshal(reqBody)
	require.NoError(t, err)
	resp, err := env.client.Post(env.server.URL+"/ingest", "application/json", bytes.NewReader(bodyBytes))
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
}

// TestMirror_IngestAuth_AcceptsWithValidToken verifies that the mirror
// accepts ingest requests with a valid bearer token.
func TestMirror_IngestAuth_AcceptsWithValidToken(t *testing.T) {
	env := newMirrorTestEnv(t)
	env.mirror.SetIngestAuthToken("secret-token-123")

	records := []models.PublicFeedRecord{env.makeRecord(1, map[string]any{"campaign_id": "c1"})}
	batch := env.buildBatch(records, constants.PublicFeedZeroHashHex)

	reqBody := models.PublicIngestRequest{Batch: batch}
	bodyBytes, err := json.Marshal(reqBody)
	require.NoError(t, err)
	req, err := http.NewRequest(http.MethodPost, env.server.URL+"/ingest", bytes.NewReader(bodyBytes))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer secret-token-123")
	resp, err := env.client.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	var ingestResp models.PublicIngestResponse
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&ingestResp))
	assert.True(t, ingestResp.Accepted)
	assert.Equal(t, int64(1), ingestResp.HighWaterSequence)
}

// ---------------------------------------------------------------------------
// Signature validation tests
// ---------------------------------------------------------------------------

// TestMirror_Signature_RejectsTamperedSignature verifies that the mirror
// rejects a batch with a tampered signature.
func TestMirror_Signature_RejectsTamperedSignature(t *testing.T) {
	env := newMirrorTestEnv(t)
	records := []models.PublicFeedRecord{env.makeRecord(1, map[string]any{"campaign_id": "c1"})}
	batch := env.buildBatch(records, constants.PublicFeedZeroHashHex)

	// Tamper with the signature.
	sigBytes, _ := hex.DecodeString(batch.Signature)
	sigBytes[0] ^= 0xFF
	batch.Signature = hex.EncodeToString(sigBytes)

	_, resp := env.sendIngest(batch)
	assert.False(t, resp.Accepted)
	assert.Equal(t, models.PublicFeedIngestRejectionSignatureInvalid, resp.RejectionReason)
}

// TestMirror_Signature_RejectsUnknownKey verifies that the mirror rejects a
// batch signed by an unknown key.
func TestMirror_Signature_RejectsUnknownKey(t *testing.T) {
	env := newMirrorTestEnv(t)
	records := []models.PublicFeedRecord{env.makeRecord(1, map[string]any{"campaign_id": "c1"})}
	batch := env.buildBatch(records, constants.PublicFeedZeroHashHex)
	batch.SigningKeyID = "unknown-key"

	_, resp := env.sendIngest(batch)
	assert.False(t, resp.Accepted)
	assert.Equal(t, models.PublicFeedIngestRejectionUnknownKey, resp.RejectionReason)
}

// TestMirror_Signature_RejectsRevokedKey verifies that the mirror rejects a
// batch signed by a revoked key.
func TestMirror_Signature_RejectsRevokedKey(t *testing.T) {
	env := newMirrorTestEnv(t)
	env.mirror.RevokeSourceKey(env.sourceID, env.keyID)

	records := []models.PublicFeedRecord{env.makeRecord(1, map[string]any{"campaign_id": "c1"})}
	batch := env.buildBatch(records, constants.PublicFeedZeroHashHex)

	_, resp := env.sendIngest(batch)
	assert.False(t, resp.Accepted)
	assert.Equal(t, models.PublicFeedIngestRejectionRevokedKey, resp.RejectionReason)
}

// ---------------------------------------------------------------------------
// Hash chain validation tests
// ---------------------------------------------------------------------------

// TestMirror_HashChain_RejectsWrongPreviousHash verifies that the mirror
// rejects a batch whose previous_batch_hash does not match the feed chain.
func TestMirror_HashChain_RejectsWrongPreviousHash(t *testing.T) {
	env := newMirrorTestEnv(t)

	// First batch accepted.
	records1 := []models.PublicFeedRecord{env.makeRecord(1, map[string]any{"campaign_id": "c1"})}
	batch1 := env.buildBatch(records1, constants.PublicFeedZeroHashHex)
	_, resp1 := env.sendIngest(batch1)
	require.True(t, resp1.Accepted)

	// Second batch with wrong previous hash.
	records2 := []models.PublicFeedRecord{env.makeRecord(2, map[string]any{"campaign_id": "c1"})}
	batch2 := env.buildBatch(records2, "deadbeef00000000000000000000000000000000000000000000000000000000")
	_, resp2 := env.sendIngest(batch2)
	assert.False(t, resp2.Accepted)
	assert.Equal(t, models.PublicFeedIngestRejectionHashChainMismatch, resp2.RejectionReason)
}

// TestMirror_HashChain_AcceptsCorrectChain verifies that the mirror accepts
// a correctly chained sequence of batches.
func TestMirror_HashChain_AcceptsCorrectChain(t *testing.T) {
	env := newMirrorTestEnv(t)

	// First batch.
	records1 := []models.PublicFeedRecord{env.makeRecord(1, map[string]any{"v": "1"})}
	batch1 := env.buildBatch(records1, constants.PublicFeedZeroHashHex)
	_, resp1 := env.sendIngest(batch1)
	require.True(t, resp1.Accepted)

	// Second batch chained to first.
	records2 := []models.PublicFeedRecord{env.makeRecord(2, map[string]any{"v": "2"})}
	batch2 := env.buildBatch(records2, batch1.ContentHash)
	_, resp2 := env.sendIngest(batch2)
	assert.True(t, resp2.Accepted)
	assert.Equal(t, int64(2), resp2.HighWaterSequence)
	assert.Equal(t, batch2.ContentHash, resp2.FeedChainHash)
}

// ---------------------------------------------------------------------------
// Sequence ordering tests
// ---------------------------------------------------------------------------

// TestMirror_Sequence_RejectsOutOfOrder verifies that the mirror rejects a
// batch whose first sequence is less than or equal to the high-water mark.
func TestMirror_Sequence_RejectsOutOfOrder(t *testing.T) {
	env := newMirrorTestEnv(t)

	// First batch with sequences 1-3.
	records1 := []models.PublicFeedRecord{
		env.makeRecord(1, map[string]any{"v": "1"}),
		env.makeRecord(2, map[string]any{"v": "2"}),
		env.makeRecord(3, map[string]any{"v": "3"}),
	}
	batch1 := env.buildBatch(records1, constants.PublicFeedZeroHashHex)
	_, resp1 := env.sendIngest(batch1)
	require.True(t, resp1.Accepted)

	// Second batch starting at sequence 2 (overlap).
	records2 := []models.PublicFeedRecord{env.makeRecord(2, map[string]any{"v": "2b"})}
	batch2 := env.buildBatch(records2, batch1.ContentHash)
	_, resp2 := env.sendIngest(batch2)
	assert.False(t, resp2.Accepted)
	assert.Equal(t, models.PublicFeedIngestRejectionDuplicateSequence, resp2.RejectionReason)
}

// TestMirror_Sequence_RejectsFirstBatchNonZeroPrevHash verifies that the
// first batch for a source must carry the zero hash as previous_batch_hash.
func TestMirror_Sequence_RejectsFirstBatchNonZeroPrevHash(t *testing.T) {
	env := newMirrorTestEnv(t)
	records := []models.PublicFeedRecord{env.makeRecord(1, map[string]any{"v": "1"})}
	batch := env.buildBatch(records, "abcdef0000000000000000000000000000000000000000000000000000000000")
	_, resp := env.sendIngest(batch)
	assert.False(t, resp.Accepted)
	assert.Equal(t, models.PublicFeedIngestRejectionHashChainMismatch, resp.RejectionReason)
}

// ---------------------------------------------------------------------------
// Idempotency tests
// ---------------------------------------------------------------------------

// TestMirror_Idempotency_DuplicateBatchRejected verifies that re-sending an
// already-accepted batch is rejected as a duplicate sequence.
func TestMirror_Idempotency_DuplicateBatchRejected(t *testing.T) {
	env := newMirrorTestEnv(t)
	records := []models.PublicFeedRecord{env.makeRecord(1, map[string]any{"v": "1"})}
	batch := env.buildBatch(records, constants.PublicFeedZeroHashHex)

	_, resp1 := env.sendIngest(batch)
	require.True(t, resp1.Accepted)

	// Re-send the same batch.
	_, resp2 := env.sendIngest(batch)
	assert.False(t, resp2.Accepted)
	assert.Equal(t, models.PublicFeedIngestRejectionDuplicateSequence, resp2.RejectionReason)
}

// ---------------------------------------------------------------------------
// Oversized batch tests
// ---------------------------------------------------------------------------

// TestMirror_OversizedBatch_RejectsTooManyRecords verifies that the mirror
// rejects a batch with more than the maximum allowed records.
func TestMirror_OversizedBatch_RejectsTooManyRecords(t *testing.T) {
	env := newMirrorTestEnv(t)
	records := make([]models.PublicFeedRecord, constants.PublicFeedBatchMaxRecords+1)
	for i := range records {
		records[i] = env.makeRecord(int64(i+1), map[string]any{"i": i})
	}
	batch := env.buildBatch(records, constants.PublicFeedZeroHashHex)
	_, resp := env.sendIngest(batch)
	assert.False(t, resp.Accepted)
	assert.Equal(t, models.PublicFeedIngestRejectionOversizedBatch, resp.RejectionReason)
}

// ---------------------------------------------------------------------------
// Record hash validation tests
// ---------------------------------------------------------------------------

// TestMirror_RecordHash_RejectsMismatch verifies that the mirror rejects a
// batch where a record hash does not match the computed hash.
func TestMirror_RecordHash_RejectsMismatch(t *testing.T) {
	env := newMirrorTestEnv(t)
	records := []models.PublicFeedRecord{env.makeRecord(1, map[string]any{"v": "1"})}
	batch := env.buildBatch(records, constants.PublicFeedZeroHashHex)

	// Tamper with the record hash.
	batch.Records[0].RecordHash = "deadbeef00000000000000000000000000000000000000000000000000000000"
	batch.RecordHashes[0] = batch.Records[0].RecordHash

	// Recompute content hash and re-sign since we changed record hashes.
	batch.ContentHash = computeBatchContentHash(batch)
	chBytes, _ := hex.DecodeString(batch.ContentHash)
	batch.Signature = hex.EncodeToString(ed25519.Sign(env.priv, chBytes))

	_, resp := env.sendIngest(batch)
	assert.False(t, resp.Accepted)
}

// ---------------------------------------------------------------------------
// Prohibited field disclosure tests
// ---------------------------------------------------------------------------

// TestMirror_Disclosure_RejectsProhibitedFields verifies that the mirror
// rejects a batch containing prohibited fields in a record payload.
func TestMirror_Disclosure_RejectsProhibitedFields(t *testing.T) {
	env := newMirrorTestEnv(t)
	records := []models.PublicFeedRecord{env.makeRecord(1, map[string]any{
		"campaign_id": "c1",
		"api_key":     "should-not-be-here",
	})}
	batch := env.buildBatch(records, constants.PublicFeedZeroHashHex)
	_, resp := env.sendIngest(batch)
	assert.False(t, resp.Accepted)
}

// ---------------------------------------------------------------------------
// Bootstrap endpoint tests
// ---------------------------------------------------------------------------

// TestMirror_Bootstrap_ReturnsSnapshotAndProjections verifies that the
// bootstrap endpoint returns the current snapshot, recent projections, and
// proof catalog summary.
func TestMirror_Bootstrap_ReturnsSnapshotAndProjections(t *testing.T) {
	env := newMirrorTestEnv(t)

	// Ingest a few projection records.
	records := []models.PublicFeedRecord{
		env.makeRecord(1, map[string]any{"campaign_id": "c1", "variant_id": "v1"}),
		env.makeRecord(2, map[string]any{"campaign_id": "c1", "variant_id": "v2"}),
		env.makeRecord(3, map[string]any{"campaign_id": "c1", "variant_id": "v3"}),
	}
	batch := env.buildBatch(records, constants.PublicFeedZeroHashHex)
	_, resp := env.sendIngest(batch)
	require.True(t, resp.Accepted)

	var bootstrap models.PublicFeedBootstrap
	status := env.getJSON("/bootstrap?source="+env.sourceID, &bootstrap)
	assert.Equal(t, http.StatusOK, status)
	assert.Equal(t, constants.PublicFeedProtocolVersion, bootstrap.ProtocolVersion)
	assert.Equal(t, int64(3), bootstrap.Snapshot.HighWaterSequence)
	assert.Equal(t, env.sourceID, bootstrap.Snapshot.SourceID)
	assert.Equal(t, 3, len(bootstrap.RecentProjections))
	assert.Equal(t, models.CampaignFreshnessActive, bootstrap.SourceFreshness)
}

// TestMirror_Bootstrap_EmptySourceReturnsOffline verifies that the bootstrap
// endpoint returns source_offline freshness for an unknown source.
func TestMirror_Bootstrap_EmptySourceReturnsOffline(t *testing.T) {
	env := newMirrorTestEnv(t)
	var bootstrap models.PublicFeedBootstrap
	status := env.getJSON("/bootstrap?source=unknown-source", &bootstrap)
	assert.Equal(t, http.StatusOK, status)
	assert.Equal(t, models.CampaignFreshnessSourceOffline, bootstrap.SourceFreshness)
	assert.Equal(t, 0, len(bootstrap.RecentProjections))
}

// ---------------------------------------------------------------------------
// Snapshot endpoint tests
// ---------------------------------------------------------------------------

// TestMirror_Snapshot_ReturnsHighWater verifies that the snapshot endpoint
// returns the current high-water sequence and feed-chain hash.
func TestMirror_Snapshot_ReturnsHighWater(t *testing.T) {
	env := newMirrorTestEnv(t)
	records := []models.PublicFeedRecord{env.makeRecord(1, map[string]any{"v": "1"})}
	batch := env.buildBatch(records, constants.PublicFeedZeroHashHex)
	_, ingestResp := env.sendIngest(batch)
	require.True(t, ingestResp.Accepted)

	var snap models.PublicFeedSnapshot
	status := env.getJSON("/snapshot?source="+env.sourceID, &snap)
	assert.Equal(t, http.StatusOK, status)
	assert.Equal(t, int64(1), snap.HighWaterSequence)
	assert.Equal(t, batch.ContentHash, snap.FeedChainHash)
	assert.Equal(t, 1, snap.BatchCount)
}

// ---------------------------------------------------------------------------
// Cursor pagination tests
// ---------------------------------------------------------------------------

// TestMirror_History_CursorPagination verifies that the history endpoint
// paginates records with cursor-based pagination.
func TestMirror_History_CursorPagination(t *testing.T) {
	env := newMirrorTestEnv(t)

	// Ingest 5 records.
	records := make([]models.PublicFeedRecord, 5)
	for i := range records {
		records[i] = env.makeRecord(int64(i+1), map[string]any{"i": i})
	}
	batch := env.buildBatch(records, constants.PublicFeedZeroHashHex)
	_, resp := env.sendIngest(batch)
	require.True(t, resp.Accepted)

	// Page 1 with limit 2.
	var page1 models.PublicFeedCursorPage
	status := env.getJSON("/history?source="+env.sourceID+"&limit=2", &page1)
	assert.Equal(t, http.StatusOK, status)
	assert.Equal(t, 2, len(page1.Items))
	assert.True(t, page1.HasMore)
	assert.Equal(t, "2", page1.Cursor)

	// Page 2 with cursor.
	var page2 models.PublicFeedCursorPage
	status = env.getJSON("/history?source="+env.sourceID+"&cursor=2&limit=2", &page2)
	assert.Equal(t, http.StatusOK, status)
	assert.Equal(t, 2, len(page2.Items))
	assert.True(t, page2.HasMore)

	// Page 3 with cursor.
	var page3 models.PublicFeedCursorPage
	status = env.getJSON("/history?source="+env.sourceID+"&cursor=4&limit=2", &page3)
	assert.Equal(t, http.StatusOK, status)
	assert.Equal(t, 1, len(page3.Items))
	assert.False(t, page3.HasMore)
}

// TestMirror_History_BoundedPageSize verifies that the page size is bounded
// to the maximum.
func TestMirror_History_BoundedPageSize(t *testing.T) {
	env := newMirrorTestEnv(t)

	// Ingest 10 records.
	records := make([]models.PublicFeedRecord, 10)
	for i := range records {
		records[i] = env.makeRecord(int64(i+1), map[string]any{"i": i})
	}
	batch := env.buildBatch(records, constants.PublicFeedZeroHashHex)
	_, resp := env.sendIngest(batch)
	require.True(t, resp.Accepted)

	// Request limit=1000 (should be capped).
	var page models.PublicFeedCursorPage
	status := env.getJSON("/history?source="+env.sourceID+"&limit=1000", &page)
	assert.Equal(t, http.StatusOK, status)
	assert.LessOrEqual(t, page.Limit, 100)
}

// ---------------------------------------------------------------------------
// CORS tests
// ---------------------------------------------------------------------------

// TestMirror_CORS_AllowsAnonymousReads verifies that anonymous read endpoints
// include CORS headers.
func TestMirror_CORS_AllowsAnonymousReads(t *testing.T) {
	env := newMirrorTestEnv(t)
	resp, err := env.client.Get(env.server.URL + "/snapshot?source=" + env.sourceID)
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, "*", resp.Header.Get("Access-Control-Allow-Origin"))
	assert.Contains(t, resp.Header.Get("Access-Control-Allow-Methods"), "GET")
}

// TestMirror_CORS_OptionsReturnsNoContent verifies that OPTIONS preflight
// requests return 204 No Content.
func TestMirror_CORS_OptionsReturnsNoContent(t *testing.T) {
	env := newMirrorTestEnv(t)
	req, err := http.NewRequest(http.MethodOptions, env.server.URL+"/snapshot", nil)
	require.NoError(t, err)
	resp, err := env.client.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusNoContent, resp.StatusCode)
}

// TestMirror_CORS_IngestHasNoCORS verifies that the ingest endpoint does not
// include CORS headers (server-to-server only).
func TestMirror_CORS_IngestHasNoCORS(t *testing.T) {
	env := newMirrorTestEnv(t)
	records := []models.PublicFeedRecord{env.makeRecord(1, map[string]any{"v": "1"})}
	batch := env.buildBatch(records, constants.PublicFeedZeroHashHex)
	reqBody := models.PublicIngestRequest{Batch: batch}
	bodyBytes, _ := json.Marshal(reqBody)
	resp, err := env.client.Post(env.server.URL+"/ingest", "application/json", bytes.NewReader(bodyBytes))
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Empty(t, resp.Header.Get("Access-Control-Allow-Origin"))
}

// ---------------------------------------------------------------------------
// SSE replay tests
// ---------------------------------------------------------------------------

// TestMirror_SSE_ReplaysExistingRecords verifies that the SSE stream sends
// existing records on connect.
func TestMirror_SSE_ReplaysExistingRecords(t *testing.T) {
	env := newMirrorTestEnv(t)

	records := []models.PublicFeedRecord{
		env.makeRecord(1, map[string]any{"v": "1"}),
		env.makeRecord(2, map[string]any{"v": "2"}),
	}
	batch := env.buildBatch(records, constants.PublicFeedZeroHashHex)
	_, resp := env.sendIngest(batch)
	require.True(t, resp.Accepted)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, env.server.URL+"/stream?source="+env.sourceID, nil)
	require.NoError(t, err)
	resp2, err := env.client.Do(req)
	require.NoError(t, err)
	defer resp2.Body.Close()

	scanner := bufio.NewScanner(resp2.Body)
	eventCount := 0
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "event: ") {
			eventCount++
		}
		// We expect at least a snapshot event + 2 record events.
		if eventCount >= 3 {
			break
		}
	}
	assert.GreaterOrEqual(t, eventCount, 3)
}

// TestMirror_SSE_ResumesFromSinceID verifies that the SSE stream resumes from
// a given since_id, sending only records after that sequence.
func TestMirror_SSE_ResumesFromSinceID(t *testing.T) {
	env := newMirrorTestEnv(t)

	records := []models.PublicFeedRecord{
		env.makeRecord(1, map[string]any{"v": "1"}),
		env.makeRecord(2, map[string]any{"v": "2"}),
		env.makeRecord(3, map[string]any{"v": "3"}),
	}
	batch := env.buildBatch(records, constants.PublicFeedZeroHashHex)
	_, resp := env.sendIngest(batch)
	require.True(t, resp.Accepted)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	// Resume from sequence 1 (should get records 2 and 3).
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, env.server.URL+"/stream?source="+env.sourceID+"&since_id=1", nil)
	require.NoError(t, err)
	resp2, err := env.client.Do(req)
	require.NoError(t, err)
	defer resp2.Body.Close()

	scanner := bufio.NewScanner(resp2.Body)
	dataLines := 0
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "data: ") {
			dataLines++
		}
		if dataLines >= 3 {
			break
		}
	}
	// Expect at least: snapshot event + 2 record events = 3 data lines.
	assert.GreaterOrEqual(t, dataLines, 3)
}

// ---------------------------------------------------------------------------
// Stale source tests
// ---------------------------------------------------------------------------

// TestMirror_StaleSource_ReportsStaleFreshness verifies that the mirror
// reports the source freshness label set via SetSourceFreshness.
func TestMirror_StaleSource_ReportsStaleFreshness(t *testing.T) {
	env := newMirrorTestEnv(t)

	// Ingest a batch first.
	records := []models.PublicFeedRecord{env.makeRecord(1, map[string]any{"v": "1"})}
	batch := env.buildBatch(records, constants.PublicFeedZeroHashHex)
	_, resp := env.sendIngest(batch)
	require.True(t, resp.Accepted)

	// Set freshness to stale.
	env.mirror.SetSourceFreshness(env.sourceID, models.CampaignFreshnessStale)

	var snap models.PublicFeedSnapshot
	status := env.getJSON("/snapshot?source="+env.sourceID, &snap)
	assert.Equal(t, http.StatusOK, status)
	assert.Equal(t, models.CampaignFreshnessStale, snap.Freshness)
}

// TestMirror_StaleSource_BootstrapReportsStale verifies that the bootstrap
// endpoint reports stale freshness.
func TestMirror_StaleSource_BootstrapReportsStale(t *testing.T) {
	env := newMirrorTestEnv(t)

	records := []models.PublicFeedRecord{env.makeRecord(1, map[string]any{"v": "1"})}
	batch := env.buildBatch(records, constants.PublicFeedZeroHashHex)
	_, resp := env.sendIngest(batch)
	require.True(t, resp.Accepted)

	env.mirror.SetSourceFreshness(env.sourceID, models.CampaignFreshnessIntentionallyStopped)

	var bootstrap models.PublicFeedBootstrap
	status := env.getJSON("/bootstrap?source="+env.sourceID, &bootstrap)
	assert.Equal(t, http.StatusOK, status)
	assert.Equal(t, models.CampaignFreshnessIntentionallyStopped, bootstrap.SourceFreshness)
	assert.Equal(t, models.CampaignFreshnessIntentionallyStopped, bootstrap.Snapshot.Freshness)
}

// ---------------------------------------------------------------------------
// Key rotation tests
// ---------------------------------------------------------------------------

// TestMirror_KeyRotation_AcceptsNewKey verifies that the mirror accepts
// batches signed by a newly registered key after the old key is revoked.
func TestMirror_KeyRotation_AcceptsNewKey(t *testing.T) {
	env := newMirrorTestEnv(t)

	// First batch with old key.
	records1 := []models.PublicFeedRecord{env.makeRecord(1, map[string]any{"v": "1"})}
	batch1 := env.buildBatch(records1, constants.PublicFeedZeroHashHex)
	_, resp1 := env.sendIngest(batch1)
	require.True(t, resp1.Accepted)

	// Generate and register a new key.
	newPub, newPriv, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)
	newKeyID := "test-key-2"
	env.mirror.RegisterSourceKey(env.sourceID, newKeyID, newPub)

	// Revoke old key.
	env.mirror.RevokeSourceKey(env.sourceID, env.keyID)

	// Second batch with new key.
	records2 := []models.PublicFeedRecord{env.makeRecord(2, map[string]any{"v": "2"})}
	batch2 := models.PublicFeedBatch{
		ProtocolVersion:   constants.PublicFeedProtocolVersion,
		SchemaVersion:     constants.PublicFeedSchemaVersion,
		SourceID:          env.sourceID,
		FirstSequence:     2,
		LastSequence:      2,
		PreviousBatchHash: batch1.ContentHash,
		RecordHashes:      []string{records2[0].RecordHash},
		GeneratedAt:       time.Now().UTC(),
		SigningKeyID:      newKeyID,
		Records:           records2,
	}
	batch2.ContentHash = computeBatchContentHash(batch2)
	chBytes, _ := hex.DecodeString(batch2.ContentHash)
	batch2.Signature = hex.EncodeToString(ed25519.Sign(newPriv, chBytes))

	_, resp2 := env.sendIngest(batch2)
	assert.True(t, resp2.Accepted)
	assert.Equal(t, int64(2), resp2.HighWaterSequence)
}

// TestMirror_KeyRotation_OldKeyRejectedAfterRevocation verifies that batches
// signed by the old key are rejected after revocation.
func TestMirror_KeyRotation_OldKeyRejectedAfterRevocation(t *testing.T) {
	env := newMirrorTestEnv(t)

	// First batch with old key.
	records1 := []models.PublicFeedRecord{env.makeRecord(1, map[string]any{"v": "1"})}
	batch1 := env.buildBatch(records1, constants.PublicFeedZeroHashHex)
	_, resp1 := env.sendIngest(batch1)
	require.True(t, resp1.Accepted)

	// Register new key and revoke old.
	newPub, _, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)
	env.mirror.RegisterSourceKey(env.sourceID, "new-key", newPub)
	env.mirror.RevokeSourceKey(env.sourceID, env.keyID)

	// Try to send with old key.
	records2 := []models.PublicFeedRecord{env.makeRecord(2, map[string]any{"v": "2"})}
	batch2 := env.buildBatch(records2, batch1.ContentHash)
	_, resp2 := env.sendIngest(batch2)
	assert.False(t, resp2.Accepted)
	assert.Equal(t, models.PublicFeedIngestRejectionRevokedKey, resp2.RejectionReason)
}

// ---------------------------------------------------------------------------
// Multi-source isolation tests
// ---------------------------------------------------------------------------

// TestMirror_MultiSource_IsolatesSources verifies that the mirror isolates
// state (high-water sequence, feed-chain hash) between multiple sources.
func TestMirror_MultiSource_IsolatesSources(t *testing.T) {
	env := newMirrorTestEnv(t)

	// Register a second source.
	pub2, priv2, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)
	source2ID := "test-source-2"
	key2ID := "test-key-2"
	env.mirror.RegisterSourceKey(source2ID, key2ID, pub2)

	// Ingest to source 1.
	records1 := []models.PublicFeedRecord{env.makeRecord(1, map[string]any{"s": "1"})}
	batch1 := env.buildBatch(records1, constants.PublicFeedZeroHashHex)
	_, resp1 := env.sendIngest(batch1)
	require.True(t, resp1.Accepted)

	// Ingest to source 2 (independent sequence space).
	records2Bytes, _ := json.Marshal(map[string]any{"s": "2"})
	records2Hash := sha256.Sum256(records2Bytes)
	records2 := []models.PublicFeedRecord{{
		Sequence:    1,
		RecordType:  models.PublicFeedRecordTypeProjection,
		RecordHash:  hex.EncodeToString(records2Hash[:]),
		RecordBytes: string(records2Bytes),
	}}
	batch2 := models.PublicFeedBatch{
		ProtocolVersion:   constants.PublicFeedProtocolVersion,
		SchemaVersion:     constants.PublicFeedSchemaVersion,
		SourceID:          source2ID,
		FirstSequence:     1,
		LastSequence:      1,
		PreviousBatchHash: constants.PublicFeedZeroHashHex,
		RecordHashes:      []string{records2[0].RecordHash},
		GeneratedAt:       time.Now().UTC(),
		SigningKeyID:      key2ID,
		Records:           records2,
	}
	batch2.ContentHash = computeBatchContentHash(batch2)
	chBytes, _ := hex.DecodeString(batch2.ContentHash)
	batch2.Signature = hex.EncodeToString(ed25519.Sign(priv2, chBytes))

	_, resp2 := env.sendIngest(batch2)
	require.True(t, resp2.Accepted)

	// Source 1 snapshot.
	var snap1 models.PublicFeedSnapshot
	env.getJSON("/snapshot?source="+env.sourceID, &snap1)
	assert.Equal(t, int64(1), snap1.HighWaterSequence)
	assert.Equal(t, batch1.ContentHash, snap1.FeedChainHash)

	// Source 2 snapshot (independent).
	var snap2 models.PublicFeedSnapshot
	env.getJSON("/snapshot?source="+source2ID, &snap2)
	assert.Equal(t, int64(1), snap2.HighWaterSequence)
	assert.Equal(t, batch2.ContentHash, snap2.FeedChainHash)
	assert.NotEqual(t, snap1.FeedChainHash, snap2.FeedChainHash)
}

// ---------------------------------------------------------------------------
// Proof download tests
// ---------------------------------------------------------------------------

// TestMirror_ProofDownload_ServesContentAddressedArtifact verifies that the
// mirror serves proof artifacts under content-addressed URLs with safe
// headers.
func TestMirror_ProofDownload_ServesContentAddressedArtifact(t *testing.T) {
	env := newMirrorTestEnv(t)

	content := []byte(`{"proof": "test"}`)
	h := sha256.Sum256(content)
	artifactID := hex.EncodeToString(h[:])
	env.mirror.StoreProofArtifact(artifactID, content)

	// Store a catalog entry for safe headers.
	env.mirror.StoreProofCatalog(env.sourceID, models.PublicProofCatalog{
		SchemaVersion: constants.PublicProofCatalogSchemaVersion,
		Entries: []models.PublicProofCatalogEntry{{
			ArtifactID:     artifactID,
			Filename:       "proof.json",
			MediaType:      "application/json",
			ByteSize:       int64(len(content)),
			SHA256:         artifactID,
			Classification: models.PublicFeedProofClassificationPublicSafe,
			CampaignID:     "c1",
			GeneratedAt:    time.Now().UTC(),
			ImmutableURL:   "/proofs/" + artifactID,
		}},
		GeneratedAt: time.Now().UTC(),
	})

	resp, err := env.client.Get(env.server.URL + "/proofs/" + artifactID)
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, "application/json", resp.Header.Get("Content-Type"))
	assert.Contains(t, resp.Header.Get("Cache-Control"), "immutable")
	assert.Contains(t, resp.Header.Get("Content-Disposition"), "proof.json")

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	assert.Equal(t, content, body)
}

// TestMirror_ProofDownload_Returns404ForUnknown verifies that the mirror
// returns 404 for an unknown proof artifact.
func TestMirror_ProofDownload_Returns404ForUnknown(t *testing.T) {
	env := newMirrorTestEnv(t)
	resp, err := env.client.Get(env.server.URL + "/proofs/unknown-artifact-id")
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
}

// TestMirror_ProofCatalog_ReturnsEntries verifies that the proof catalog
// endpoint returns the stored catalog entries.
func TestMirror_ProofCatalog_ReturnsEntries(t *testing.T) {
	env := newMirrorTestEnv(t)

	content := []byte(`{"proof": "test"}`)
	h := sha256.Sum256(content)
	artifactID := hex.EncodeToString(h[:])
	env.mirror.StoreProofArtifact(artifactID, content)
	env.mirror.StoreProofCatalog(env.sourceID, models.PublicProofCatalog{
		SchemaVersion: constants.PublicProofCatalogSchemaVersion,
		Entries: []models.PublicProofCatalogEntry{{
			ArtifactID:     artifactID,
			Filename:       "proof.json",
			MediaType:      "application/json",
			ByteSize:       int64(len(content)),
			SHA256:         artifactID,
			Classification: models.PublicFeedProofClassificationPublicSafe,
			CampaignID:     "c1",
			GeneratedAt:    time.Now().UTC(),
			ImmutableURL:   "/proofs/" + artifactID,
		}},
		GeneratedAt: time.Now().UTC(),
	})

	var catalog models.PublicProofCatalog
	status := env.getJSON("/proof-catalog?source="+env.sourceID, &catalog)
	assert.Equal(t, http.StatusOK, status)
	assert.Equal(t, 1, len(catalog.Entries))
	assert.Equal(t, artifactID, catalog.Entries[0].ArtifactID)
}

// TestMirror_ProofCatalog_EmptyReturnsEmptyArray verifies that the proof
// catalog endpoint returns an empty array when no proofs exist.
func TestMirror_ProofCatalog_EmptyReturnsEmptyArray(t *testing.T) {
	env := newMirrorTestEnv(t)
	var catalog models.PublicProofCatalog
	status := env.getJSON("/proof-catalog?source="+env.sourceID, &catalog)
	assert.Equal(t, http.StatusOK, status)
	assert.Equal(t, 0, len(catalog.Entries))
}

// TestMirror_ProofManifest_ReturnsManifest verifies that the proof manifest
// endpoint returns the stored manifest.
func TestMirror_ProofManifest_ReturnsManifest(t *testing.T) {
	env := newMirrorTestEnv(t)
	manifest := models.PublicProofManifest{
		SchemaVersion:               constants.PublicProofManifestSchemaVersion,
		ProofRootSHA256:             "abc123",
		CampaignID:                  "c1",
		CampaignRevision:            "rev1",
		VerifiedIndexGenerationHash: "idx1",
		VerificationOK:              true,
		ArtifactCount:               1,
		Artifacts:                   []models.PublicProofCatalogEntry{},
		VerifierInstructions:        "verify",
		GeneratedAt:                 time.Now().UTC(),
		SigningKeyID:                env.keyID,
		Signature:                   "sig",
	}
	env.mirror.StoreProofManifest(env.sourceID, manifest)

	var result models.PublicProofManifest
	status := env.getJSON("/proof-manifest?source="+env.sourceID, &result)
	assert.Equal(t, http.StatusOK, status)
	assert.Equal(t, "c1", result.CampaignID)
	assert.Equal(t, "abc123", result.ProofRootSHA256)
}

// TestMirror_ProofManifest_Returns404ForUnknown verifies that the proof
// manifest endpoint returns 404 for an unknown source.
func TestMirror_ProofManifest_Returns404ForUnknown(t *testing.T) {
	env := newMirrorTestEnv(t)
	resp, err := env.client.Get(env.server.URL + "/proof-manifest?source=unknown")
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
}

// ---------------------------------------------------------------------------
// Full ingest + read cycle test
// ---------------------------------------------------------------------------

// TestMirror_FullCycle_IngestThenRead verifies the complete cycle: ingest a
// batch, then read it back through bootstrap, snapshot, and history.
func TestMirror_FullCycle_IngestThenRead(t *testing.T) {
	env := newMirrorTestEnv(t)

	// Ingest two batches.
	records1 := []models.PublicFeedRecord{
		env.makeRecord(1, map[string]any{"campaign_id": "c1", "variant_id": "v1"}),
		env.makeRecord(2, map[string]any{"campaign_id": "c1", "variant_id": "v2"}),
	}
	batch1 := env.buildBatch(records1, constants.PublicFeedZeroHashHex)
	_, resp1 := env.sendIngest(batch1)
	require.True(t, resp1.Accepted)

	records2 := []models.PublicFeedRecord{
		env.makeRecord(3, map[string]any{"campaign_id": "c1", "variant_id": "v3"}),
	}
	batch2 := env.buildBatch(records2, batch1.ContentHash)
	_, resp2 := env.sendIngest(batch2)
	require.True(t, resp2.Accepted)

	// Snapshot.
	var snap models.PublicFeedSnapshot
	env.getJSON("/snapshot?source="+env.sourceID, &snap)
	assert.Equal(t, int64(3), snap.HighWaterSequence)
	assert.Equal(t, batch2.ContentHash, snap.FeedChainHash)
	assert.Equal(t, 2, snap.BatchCount)

	// Bootstrap.
	var bootstrap models.PublicFeedBootstrap
	env.getJSON("/bootstrap?source="+env.sourceID, &bootstrap)
	assert.Equal(t, int64(3), bootstrap.Snapshot.HighWaterSequence)
	assert.Equal(t, 3, len(bootstrap.RecentProjections))

	// History (all records).
	var page models.PublicFeedCursorPage
	env.getJSON("/history?source="+env.sourceID+"&limit=10", &page)
	assert.Equal(t, 3, len(page.Items))
	assert.False(t, page.HasMore)
}

// ---------------------------------------------------------------------------
// Record type validation tests
// ---------------------------------------------------------------------------

// TestMirror_RecordType_AcceptsEventRecords verifies that the mirror accepts
// event-type records in addition to projections.
func TestMirror_RecordType_AcceptsEventRecords(t *testing.T) {
	env := newMirrorTestEnv(t)

	eventBytes, _ := json.Marshal(map[string]any{"event_type": "eval.cycle.started", "cycle_id": "cyc-1"})
	eventHash := sha256.Sum256(eventBytes)
	records := []models.PublicFeedRecord{{
		Sequence:    1,
		RecordType:  models.PublicFeedRecordTypeEvent,
		RecordHash:  hex.EncodeToString(eventHash[:]),
		RecordBytes: string(eventBytes),
	}}
	batch := env.buildBatch(records, constants.PublicFeedZeroHashHex)
	_, resp := env.sendIngest(batch)
	assert.True(t, resp.Accepted)
}

// TestMirror_RecordType_AcceptsProofManifestRecords verifies that the mirror
// accepts proof_manifest-type records.
func TestMirror_RecordType_AcceptsProofManifestRecords(t *testing.T) {
	env := newMirrorTestEnv(t)

	manifestBytes, _ := json.Marshal(map[string]any{"proof_root": "abc123", "campaign_id": "c1"})
	manifestHash := sha256.Sum256(manifestBytes)
	records := []models.PublicFeedRecord{{
		Sequence:    1,
		RecordType:  models.PublicFeedRecordTypeProofManifest,
		RecordHash:  hex.EncodeToString(manifestHash[:]),
		RecordBytes: string(manifestBytes),
	}}
	batch := env.buildBatch(records, constants.PublicFeedZeroHashHex)
	_, resp := env.sendIngest(batch)
	assert.True(t, resp.Accepted)
}

// TestMirror_RecordType_AcceptsKeyRevocationRecords verifies that the mirror
// accepts key_revocation-type records.
func TestMirror_RecordType_AcceptsKeyRevocationRecords(t *testing.T) {
	env := newMirrorTestEnv(t)

	revBytes, _ := json.Marshal(map[string]any{
		"revoked_key_id":       env.keyID,
		"revoked_at":           time.Now().UTC().Format(time.RFC3339Nano),
		"new_key_id":           "new-key",
		"revocation_signature": "sig",
	})
	revHash := sha256.Sum256(revBytes)
	records := []models.PublicFeedRecord{{
		Sequence:    1,
		RecordType:  models.PublicFeedRecordTypeKeyRevocation,
		RecordHash:  hex.EncodeToString(revHash[:]),
		RecordBytes: string(revBytes),
	}}
	batch := env.buildBatch(records, constants.PublicFeedZeroHashHex)
	_, resp := env.sendIngest(batch)
	assert.True(t, resp.Accepted)
}

// ---------------------------------------------------------------------------
// Method not allowed tests
// ---------------------------------------------------------------------------

// TestMirror_MethodNotAllowed_RejectsPOSTOnReadEndpoints verifies that read
// endpoints reject POST requests.
func TestMirror_MethodNotAllowed_RejectsPOSTOnReadEndpoints(t *testing.T) {
	env := newMirrorTestEnv(t)
	for _, path := range []string{"/bootstrap", "/snapshot", "/history", "/proof-catalog"} {
		resp, err := env.client.Post(env.server.URL+path, "application/json", strings.NewReader("{}"))
		require.NoError(t, err)
		assert.Equal(t, http.StatusMethodNotAllowed, resp.StatusCode, "path: %s", path)
		resp.Body.Close()
	}
}

// TestMirror_MethodNotAllowed_RejectsGETOnIngest verifies that the ingest
// endpoint rejects GET requests.
func TestMirror_MethodNotAllowed_RejectsGETOnIngest(t *testing.T) {
	env := newMirrorTestEnv(t)
	resp, err := env.client.Get(env.server.URL + "/ingest")
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusMethodNotAllowed, resp.StatusCode)
}

// ---------------------------------------------------------------------------
// Empty source tests
// ---------------------------------------------------------------------------

// TestMirror_EmptySource_HistoryReturnsEmpty verifies that the history
// endpoint returns an empty page for a source with no records.
func TestMirror_EmptySource_HistoryReturnsEmpty(t *testing.T) {
	env := newMirrorTestEnv(t)
	var page models.PublicFeedCursorPage
	status := env.getJSON("/history?source="+env.sourceID, &page)
	assert.Equal(t, http.StatusOK, status)
	assert.Equal(t, 0, len(page.Items))
	assert.False(t, page.HasMore)
}

// TestMirror_EmptySource_SnapshotReturnsZero verifies that the snapshot
// endpoint returns zero high-water for a source with no batches.
func TestMirror_EmptySource_SnapshotReturnsZero(t *testing.T) {
	env := newMirrorTestEnv(t)
	var snap models.PublicFeedSnapshot
	status := env.getJSON("/snapshot?source="+env.sourceID, &snap)
	assert.Equal(t, http.StatusOK, status)
	assert.Equal(t, int64(0), snap.HighWaterSequence)
	assert.Equal(t, constants.PublicFeedZeroHashHex, snap.FeedChainHash)
}

// ---------------------------------------------------------------------------
// Content hash mismatch test
// ---------------------------------------------------------------------------

// TestMirror_ContentHashMismatch_RejectsTamperedContentHash verifies that
// the mirror rejects a batch whose content hash does not match the computed
// hash.
func TestMirror_ContentHashMismatch_RejectsTamperedContentHash(t *testing.T) {
	env := newMirrorTestEnv(t)
	records := []models.PublicFeedRecord{env.makeRecord(1, map[string]any{"v": "1"})}
	batch := env.buildBatch(records, constants.PublicFeedZeroHashHex)

	// Tamper with the content hash.
	batch.ContentHash = "deadbeef00000000000000000000000000000000000000000000000000000000"

	_, resp := env.sendIngest(batch)
	assert.False(t, resp.Accepted)
	assert.Equal(t, models.PublicFeedIngestRejectionHashChainMismatch, resp.RejectionReason)
}

// ---------------------------------------------------------------------------
// Bootstrap with proof catalog summary test
// ---------------------------------------------------------------------------

// TestMirror_Bootstrap_IncludesProofCatalogSummary verifies that the
// bootstrap response includes the proof catalog summary when proofs exist.
func TestMirror_Bootstrap_IncludesProofCatalogSummary(t *testing.T) {
	env := newMirrorTestEnv(t)

	// Ingest a batch.
	records := []models.PublicFeedRecord{env.makeRecord(1, map[string]any{"v": "1"})}
	batch := env.buildBatch(records, constants.PublicFeedZeroHashHex)
	_, resp := env.sendIngest(batch)
	require.True(t, resp.Accepted)

	// Store a proof artifact and catalog.
	content := []byte(`{"proof": "test"}`)
	h := sha256.Sum256(content)
	artifactID := hex.EncodeToString(h[:])
	env.mirror.StoreProofArtifact(artifactID, content)
	env.mirror.StoreProofCatalog(env.sourceID, models.PublicProofCatalog{
		SchemaVersion: constants.PublicProofCatalogSchemaVersion,
		Entries: []models.PublicProofCatalogEntry{{
			ArtifactID:     artifactID,
			Filename:       "proof.json",
			MediaType:      "application/json",
			ByteSize:       int64(len(content)),
			SHA256:         artifactID,
			Classification: models.PublicFeedProofClassificationPublicSafe,
			CampaignID:     "c1",
			GeneratedAt:    time.Now().UTC(),
			ImmutableURL:   "/proofs/" + artifactID,
		}},
		GeneratedAt: time.Now().UTC(),
	})

	var bootstrap models.PublicFeedBootstrap
	env.getJSON("/bootstrap?source="+env.sourceID, &bootstrap)
	assert.Equal(t, 1, bootstrap.ProofCatalogSummary.ArtifactCount)
	assert.Equal(t, int64(len(content)), bootstrap.ProofCatalogSummary.TotalByteSize)
	assert.NotNil(t, bootstrap.ProofCatalogSummary.LastGeneratedAt)
}

// ---------------------------------------------------------------------------
// History cursor format test
// ---------------------------------------------------------------------------

// TestMirror_History_CursorIsOpaqueSequence verifies that the cursor is the
// last sequence number and can be used to fetch the next page.
func TestMirror_History_CursorIsOpaqueSequence(t *testing.T) {
	env := newMirrorTestEnv(t)

	// Ingest 10 records.
	records := make([]models.PublicFeedRecord, 10)
	for i := range records {
		records[i] = env.makeRecord(int64(i+1), map[string]any{"i": i})
	}
	batch := env.buildBatch(records, constants.PublicFeedZeroHashHex)
	_, resp := env.sendIngest(batch)
	require.True(t, resp.Accepted)

	// Page 1 with limit 3.
	var page1 models.PublicFeedCursorPage
	env.getJSON("/history?source="+env.sourceID+"&limit=3", &page1)
	assert.Equal(t, 3, len(page1.Items))
	assert.True(t, page1.HasMore)
	cursor := page1.Cursor
	assert.NotEmpty(t, cursor)

	// Page 2 using the cursor.
	var page2 models.PublicFeedCursorPage
	env.getJSON(fmt.Sprintf("/history?source=%s&cursor=%s&limit=3", env.sourceID, cursor), &page2)
	assert.Equal(t, 3, len(page2.Items))

	// Verify no overlap: page2 items should start after page1's last item.
	page1LastSeq, _ := strconv.ParseInt(page1.Cursor, 10, 64)
	page2FirstItem := page2.Items[0]["sequence"].(float64)
	assert.Greater(t, int64(page2FirstItem), page1LastSeq)
}
