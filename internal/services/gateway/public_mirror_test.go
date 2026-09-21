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
	"github.com/g8e-ai/g8e/v2/internal/services/fs"
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
	fileSvc  fs.RuntimeFileService
}

func newMirrorTestEnv(t *testing.T) *mirrorTestEnv {
	t.Helper()
	fileSvc := newProducerFileSvc(t)
	mirror, err := NewPublicMirrorServer(testutil.NewTestLogger(), NewRuntimePublicMirrorStore(fileSvc))
	require.NoError(t, err)
	pub, priv, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)
	keyID := "test-key-1"
	sourceID := "test-source-1"
	require.NoError(t, mirror.RegisterSourceKey(context.Background(), sourceID, keyID, pub))

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
		fileSvc:  fileSvc,
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

func (e *mirrorTestEnv) makeProofIngestRequest(filename string, content []byte) models.PublicProofIngestRequest {
	e.t.Helper()
	hash := sha256.Sum256(content)
	artifactID := hex.EncodeToString(hash[:])
	generatedAt := time.Now().UTC()
	entry := models.PublicProofCatalogEntry{
		ArtifactID:          artifactID,
		Filename:            filename,
		MediaType:           "application/json",
		ByteSize:            int64(len(content)),
		SHA256:              artifactID,
		Classification:      models.PublicFeedProofClassificationPublicSafe,
		CampaignID:          "c1",
		GeneratedAt:         generatedAt,
		VerificationCommand: "sha256sum " + filename,
		ImmutableURL:        "/proofs/" + artifactID,
	}
	manifest := models.PublicProofManifest{
		SchemaVersion:               constants.PublicProofManifestSchemaVersion,
		CampaignID:                  "c1",
		CampaignRevision:            "rev1",
		VerifiedIndexGenerationHash: artifactID,
		VerificationOK:              true,
		ArtifactCount:               1,
		Artifacts:                   []models.PublicProofCatalogEntry{entry},
		VerifierInstructions:        "verify hashes and signature",
		GeneratedAt:                 generatedAt,
		SigningKeyID:                e.keyID,
	}
	manifest.ProofRootSHA256 = (&PublicPublisherService{}).computeProofRootHash(manifest)
	rootBytes, err := hex.DecodeString(manifest.ProofRootSHA256)
	require.NoError(e.t, err)
	manifest.Signature = hex.EncodeToString(ed25519.Sign(e.priv, rootBytes))
	return models.PublicProofIngestRequest{
		SourceID:  e.sourceID,
		Manifest:  manifest,
		Catalog:   models.PublicProofCatalog{SchemaVersion: constants.PublicProofCatalogSchemaVersion, Entries: []models.PublicProofCatalogEntry{entry}, GeneratedAt: generatedAt},
		Artifacts: []models.PublicProofIngestArtifact{{ArtifactID: artifactID, Content: content}},
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

func TestPublicMirror_PublicHandlerExposesOnlyAnonymousReadRoutes(t *testing.T) {
	env := newMirrorTestEnv(t)
	server := httptest.NewServer(env.mirror.PublicHandler())
	t.Cleanup(server.Close)

	response, err := server.Client().Get(server.URL + "/bootstrap")
	require.NoError(t, err)
	require.NoError(t, response.Body.Close())
	assert.Equal(t, http.StatusOK, response.StatusCode)
	assert.Equal(t, "*", response.Header.Get("Access-Control-Allow-Origin"))

	for _, path := range []string{"/ingest", "/keys/register", "/proof-ingest"} {
		t.Run(path, func(t *testing.T) {
			request, requestErr := http.NewRequest(http.MethodPost, server.URL+path, strings.NewReader("{}"))
			require.NoError(t, requestErr)
			mutationResponse, requestErr := server.Client().Do(request)
			require.NoError(t, requestErr)
			require.NoError(t, mutationResponse.Body.Close())
			assert.Equal(t, http.StatusNotFound, mutationResponse.StatusCode)
			assert.Empty(t, mutationResponse.Header.Get("Access-Control-Allow-Origin"))
		})
	}
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

func TestPublicMirrorClientIDUsesCloudflareAddressFromLoopbackConnector(t *testing.T) {
	request := httptest.NewRequest(http.MethodGet, "/snapshot", nil)
	request.RemoteAddr = "127.0.0.1:32000"
	request.Header.Set("CF-Connecting-IP", "192.0.2.20")

	assert.Equal(t, "192.0.2.20", publicMirrorClientID(request))

	request.RemoteAddr = "198.51.100.4:32000"
	assert.Equal(t, "198.51.100.4", publicMirrorClientID(request))

	request.RemoteAddr = "127.0.0.1:32000"
	request.Header.Set("CF-Connecting-IP", "not-an-ip")
	assert.Equal(t, "127.0.0.1", publicMirrorClientID(request))
}

func TestMirror_AnonymousReadRateLimitRejectsAndResetsPerClientWindow(t *testing.T) {
	env := newMirrorTestEnv(t)
	require.NoError(t, env.mirror.SetAnonymousReadRateLimit(2, time.Minute))
	now := time.Now().UTC()
	env.mirror.now = func() time.Time { return now }

	for range 2 {
		response, err := env.client.Get(env.server.URL + "/snapshot?source=" + env.sourceID)
		require.NoError(t, err)
		assert.Equal(t, http.StatusOK, response.StatusCode)
		require.NoError(t, response.Body.Close())
	}
	response, err := env.client.Get(env.server.URL + "/snapshot?source=" + env.sourceID)
	require.NoError(t, err)
	assert.Equal(t, http.StatusTooManyRequests, response.StatusCode)
	assert.Equal(t, "60", response.Header.Get("Retry-After"))
	require.NoError(t, response.Body.Close())

	now = now.Add(time.Minute)
	response, err = env.client.Get(env.server.URL + "/snapshot?source=" + env.sourceID)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, response.StatusCode)
	require.NoError(t, response.Body.Close())
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
	require.NoError(t, env.mirror.RevokeSourceKey(context.Background(), env.sourceID, env.keyID))

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

func TestMirror_RestartRecoversAcceptedStateAndContinuesHashChain(t *testing.T) {
	env := newMirrorTestEnv(t)
	records1 := []models.PublicFeedRecord{env.makeRecord(1, map[string]any{"campaign_id": "c1"})}
	batch1 := env.buildBatch(records1, constants.PublicFeedZeroHashHex)
	_, resp1 := env.sendIngest(batch1)
	require.True(t, resp1.Accepted)

	mirror2, err := NewPublicMirrorServer(testutil.NewTestLogger(), NewRuntimePublicMirrorStore(env.fileSvc))
	require.NoError(t, err)
	server2 := httptest.NewServer(mirror2.Handler())
	t.Cleanup(server2.Close)

	var snapshot models.PublicFeedSnapshot
	response, err := server2.Client().Get(server2.URL + "/snapshot?source=" + env.sourceID)
	require.NoError(t, err)
	require.NoError(t, json.NewDecoder(response.Body).Decode(&snapshot))
	require.NoError(t, response.Body.Close())
	assert.Equal(t, int64(1), snapshot.HighWaterSequence)
	assert.Equal(t, batch1.ContentHash, snapshot.FeedChainHash)

	requestBody, err := json.Marshal(models.PublicIngestRequest{Batch: batch1})
	require.NoError(t, err)
	response, err = server2.Client().Post(server2.URL+"/ingest", "application/json", bytes.NewReader(requestBody))
	require.NoError(t, err)
	var replayResponse models.PublicIngestResponse
	require.NoError(t, json.NewDecoder(response.Body).Decode(&replayResponse))
	require.NoError(t, response.Body.Close())
	assert.True(t, replayResponse.Accepted)
	_, _, batchCount, recordCount := mirror2.GetSourceState(env.sourceID)
	assert.Equal(t, 1, batchCount)
	assert.Equal(t, 1, recordCount)

	records2 := []models.PublicFeedRecord{env.makeRecord(2, map[string]any{"campaign_id": "c1"})}
	batch2 := env.buildBatch(records2, batch1.ContentHash)
	requestBody, err = json.Marshal(models.PublicIngestRequest{Batch: batch2})
	require.NoError(t, err)
	response, err = server2.Client().Post(server2.URL+"/ingest", "application/json", bytes.NewReader(requestBody))
	require.NoError(t, err)
	var resp2 models.PublicIngestResponse
	require.NoError(t, json.NewDecoder(response.Body).Decode(&resp2))
	require.NoError(t, response.Body.Close())
	assert.True(t, resp2.Accepted)
	assert.Equal(t, int64(2), resp2.HighWaterSequence)
}

func TestMirror_RetentionPrunesOldestHistoryAndRecoversChain(t *testing.T) {
	env := newMirrorTestEnv(t)
	require.NoError(t, env.mirror.SetMaxRetainedBatches(2))
	previousHash := constants.PublicFeedZeroHashHex
	for sequence := int64(1); sequence <= 3; sequence++ {
		batch := env.buildBatch([]models.PublicFeedRecord{env.makeRecord(sequence, map[string]any{"sequence": sequence})}, previousHash)
		_, response := env.sendIngest(batch)
		require.True(t, response.Accepted)
		previousHash = batch.ContentHash
	}

	env.mirror.mu.RLock()
	state := env.mirror.state.Sources[env.sourceID]
	assert.Equal(t, int64(2), state.RetainedFromSequence)
	assert.Equal(t, int64(3), state.HighWaterSequence)
	assert.Equal(t, 3, state.BatchCount)
	require.Len(t, state.Batches, 2)
	require.Len(t, state.Records, 2)
	assert.Equal(t, int64(2), state.Records[0].Sequence)
	env.mirror.mu.RUnlock()

	mirror2, err := NewPublicMirrorServer(testutil.NewTestLogger(), NewRuntimePublicMirrorStore(env.fileSvc))
	require.NoError(t, err)
	require.NoError(t, mirror2.SetMaxRetainedBatches(2))
	batch4 := env.buildBatch([]models.PublicFeedRecord{env.makeRecord(4, map[string]any{"sequence": 4})}, previousHash)
	_, err = mirror2.validateBatch(batch4)
	require.NoError(t, err)
	require.NoError(t, mirror2.acceptBatch(context.Background(), batch4))

	mirror2.mu.RLock()
	defer mirror2.mu.RUnlock()
	state = mirror2.state.Sources[env.sourceID]
	assert.Equal(t, int64(3), state.RetainedFromSequence)
	assert.Equal(t, 4, state.BatchCount)
	require.Len(t, state.Records, 2)
	assert.Equal(t, int64(3), state.Records[0].Sequence)
	assert.Equal(t, int64(4), state.Records[1].Sequence)
}

func TestMirror_RestartRejectsNullDurableSourceState(t *testing.T) {
	env := newMirrorTestEnv(t)
	ctx := context.Background()
	state, err := NewRuntimePublicMirrorStore(env.fileSvc).Load(ctx)
	require.NoError(t, err)
	state.Sources[env.sourceID] = nil
	stateBytes, err := json.Marshal(state)
	require.NoError(t, err)
	require.NoError(t, env.fileSvc.WriteFile(ctx, constants.PublicMirrorStatePath, stateBytes, constants.PermFilePrivate))

	_, err = NewPublicMirrorServer(testutil.NewTestLogger(), NewRuntimePublicMirrorStore(env.fileSvc))

	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrPublicFeedMirrorStoreCorrupt)
}

func TestMirror_RestartRejectsCorruptDurableState(t *testing.T) {
	env := newMirrorTestEnv(t)
	batch := env.buildBatch([]models.PublicFeedRecord{env.makeRecord(1, map[string]any{"campaign_id": "c1"})}, constants.PublicFeedZeroHashHex)
	_, response := env.sendIngest(batch)
	require.True(t, response.Accepted)
	require.NoError(t, env.fileSvc.WriteFile(context.Background(), constants.PublicMirrorStatePath, []byte("{invalid\n"), constants.PermFilePrivate))

	_, err := NewPublicMirrorServer(testutil.NewTestLogger(), NewRuntimePublicMirrorStore(env.fileSvc))
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrPublicFeedMirrorStoreCorrupt)
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

// TestMirror_Idempotency_DuplicateBatchAccepted verifies that re-sending an
// already-accepted identical batch returns the durable high-water state.
func TestMirror_Idempotency_DuplicateBatchAccepted(t *testing.T) {
	env := newMirrorTestEnv(t)
	records := []models.PublicFeedRecord{env.makeRecord(1, map[string]any{"v": "1"})}
	batch := env.buildBatch(records, constants.PublicFeedZeroHashHex)

	_, resp1 := env.sendIngest(batch)
	require.True(t, resp1.Accepted)

	// Re-send the same batch.
	_, resp2 := env.sendIngest(batch)
	assert.True(t, resp2.Accepted)
	assert.Equal(t, int64(1), resp2.HighWaterSequence)
	highWater, feedHash, batchCount, recordCount := env.mirror.GetSourceState(env.sourceID)
	assert.Equal(t, int64(1), highWater)
	assert.Equal(t, batch.ContentHash, feedHash)
	assert.Equal(t, 1, batchCount)
	assert.Equal(t, 1, recordCount)
}

func TestMirror_Idempotency_DifferentBatchForAcceptedRangeRejectsEquivocation(t *testing.T) {
	env := newMirrorTestEnv(t)
	batch1 := env.buildBatch([]models.PublicFeedRecord{env.makeRecord(1, map[string]any{"v": "1"})}, constants.PublicFeedZeroHashHex)
	_, response := env.sendIngest(batch1)
	require.True(t, response.Accepted)

	batch2 := env.buildBatch([]models.PublicFeedRecord{env.makeRecord(1, map[string]any{"v": "different"})}, constants.PublicFeedZeroHashHex)
	_, response = env.sendIngest(batch2)
	assert.False(t, response.Accepted)
	assert.Equal(t, models.PublicFeedIngestRejectionEquivocation, response.RejectionReason)
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

func TestMirror_History_FiltersByRecordKindBeforePagination(t *testing.T) {
	env := newMirrorTestEnv(t)
	records := []models.PublicFeedRecord{
		env.makeRecord(1, map[string]any{"kind": "assignment_result", "dataset_id": "eval-run-1"}),
		env.makeRecord(2, map[string]any{"kind": "catalog_snapshot", "dataset_id": "eval-run-1"}),
		env.makeRecord(3, map[string]any{"kind": "assignment_result", "dataset_id": "eval-run-2"}),
		env.makeRecord(4, map[string]any{"kind": "catalog_snapshot", "dataset_id": "eval-run-2"}),
	}
	batch := env.buildBatch(records, constants.PublicFeedZeroHashHex)
	_, resp := env.sendIngest(batch)
	require.True(t, resp.Accepted)

	var first models.PublicFeedCursorPage
	status := env.getJSON("/history?source="+env.sourceID+"&kind=catalog_snapshot&limit=1", &first)
	assert.Equal(t, http.StatusOK, status)
	require.Len(t, first.Items, 1)
	assert.Equal(t, "eval-run-1", first.Items[0]["dataset_id"])
	assert.True(t, first.HasMore)
	assert.Equal(t, "2", first.Cursor)

	var second models.PublicFeedCursorPage
	status = env.getJSON("/history?source="+env.sourceID+"&kind=catalog_snapshot&cursor="+first.Cursor+"&limit=1", &second)
	assert.Equal(t, http.StatusOK, status)
	require.Len(t, second.Items, 1)
	assert.Equal(t, "eval-run-2", second.Items[0]["dataset_id"])
	assert.False(t, second.HasMore)
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

func TestMirror_SSE_DeliversFirstBatchToExplicitSourceSubscriber(t *testing.T) {
	env := newMirrorTestEnv(t)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, env.server.URL+"/stream?source="+env.sourceID+"&since_id=0", nil)
	require.NoError(t, err)
	streamResponse, err := env.client.Do(req)
	require.NoError(t, err)
	defer streamResponse.Body.Close()

	batch := env.buildBatch([]models.PublicFeedRecord{env.makeRecord(1, map[string]any{"v": "1"})}, constants.PublicFeedZeroHashHex)
	require.NoError(t, env.mirror.acceptBatch(ctx, batch))

	scanner := bufio.NewScanner(streamResponse.Body)
	receivedRecord := false
	for scanner.Scan() {
		if scanner.Text() == "event: projection" {
			receivedRecord = true
			break
		}
	}
	assert.True(t, receivedRecord)
}

// TestMirror_SSEAllowsConcurrentConnectionsBehindOneProxyUntilGlobalLimit verifies
// that tunnel topology does not collapse independent streams into one client.
func TestMirror_SSEAllowsConcurrentConnectionsBehindOneProxyUntilGlobalLimit(t *testing.T) {
	env := newMirrorTestEnv(t)
	env.mirror.maxSSESubscribers = 2
	first, accepted := env.mirror.addSSESubscriber(env.sourceID)
	require.True(t, accepted)
	t.Cleanup(func() { env.mirror.removeSSESubscriber(first) })

	second, accepted := env.mirror.addSSESubscriber(env.sourceID)
	require.True(t, accepted)
	t.Cleanup(func() { env.mirror.removeSSESubscriber(second) })

	_, accepted = env.mirror.addSSESubscriber(env.sourceID)
	assert.False(t, accepted)
}

func TestMirrorSSESubscriber_DropsOldestAndSignalsTruncation(t *testing.T) {
	subscriber := &mirrorSSESubscriber{ch: make(chan models.PublicFeedRecord, 2)}
	subscriber.send(models.PublicFeedRecord{Sequence: 1})
	subscriber.send(models.PublicFeedRecord{Sequence: 2})
	subscriber.send(models.PublicFeedRecord{Sequence: 3})

	assert.Equal(t, int64(2), (<-subscriber.ch).Sequence)
	assert.Equal(t, int64(3), (<-subscriber.ch).Sequence)
	assert.True(t, subscriber.consumeTruncated())
	assert.False(t, subscriber.consumeTruncated())
}

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

func TestMirror_SSE_ReplayTruncatesAtLimit(t *testing.T) {
	env := newMirrorTestEnv(t)
	env.mirror.SetMaxSSEReplayRecords(2)

	records := make([]models.PublicFeedRecord, 4)
	for i := range records {
		records[i] = env.makeRecord(int64(i+1), map[string]any{"v": i + 1})
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
	recordEvents := 0
	truncated := false
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "event: projection") {
			recordEvents++
		}
		if line == "event: truncated" {
			truncated = true
			break
		}
	}
	assert.Equal(t, 2, recordEvents)
	assert.True(t, truncated)
}

// ---------------------------------------------------------------------------
// Stale source tests
// ---------------------------------------------------------------------------

// TestMirror_StaleSource_ReportsStaleFreshness verifies that elapsed accepted-batch time produces stale freshness.
func TestMirror_StaleSource_ReportsStaleFreshness(t *testing.T) {
	env := newMirrorTestEnv(t)

	// Ingest a batch first.
	records := []models.PublicFeedRecord{env.makeRecord(1, map[string]any{"v": "1"})}
	batch := env.buildBatch(records, constants.PublicFeedZeroHashHex)
	_, resp := env.sendIngest(batch)
	require.True(t, resp.Accepted)

	require.NoError(t, env.mirror.SetFreshnessWindows(time.Second, 2*time.Second, 3*time.Second))
	acceptedAt := env.mirror.state.Sources[env.sourceID].LastAcceptedAt
	env.mirror.now = func() time.Time { return acceptedAt.Add(2 * time.Second) }

	var snap models.PublicFeedSnapshot
	status := env.getJSON("/snapshot?source="+env.sourceID, &snap)
	assert.Equal(t, http.StatusOK, status)
	assert.Equal(t, models.CampaignFreshnessStale, snap.Freshness)
}

// TestMirror_FreshnessDerivesFromLastAcceptedBatchTime verifies every elapsed-time freshness boundary.
func TestMirror_FreshnessDerivesFromLastAcceptedBatchTime(t *testing.T) {
	env := newMirrorTestEnv(t)
	batch := env.buildBatch([]models.PublicFeedRecord{env.makeRecord(1, map[string]any{"v": "1"})}, constants.PublicFeedZeroHashHex)
	_, response := env.sendIngest(batch)
	require.True(t, response.Accepted)
	require.NoError(t, env.mirror.SetFreshnessWindows(time.Second, 2*time.Second, 3*time.Second))
	acceptedAt := env.mirror.state.Sources[env.sourceID].LastAcceptedAt

	testCases := []struct {
		name     string
		age      time.Duration
		expected models.CampaignFreshness
	}{
		{name: "active before delayed window", age: 500 * time.Millisecond, expected: models.CampaignFreshnessActive},
		{name: "delayed after delayed window", age: time.Second, expected: models.CampaignFreshnessDelayed},
		{name: "stale after stale window", age: 2 * time.Second, expected: models.CampaignFreshnessStale},
		{name: "offline after liveness window", age: 3 * time.Second, expected: models.CampaignFreshnessSourceOffline},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			env.mirror.now = func() time.Time { return acceptedAt.Add(testCase.age) }
			var snapshot models.PublicFeedSnapshot
			status := env.getJSON("/snapshot?source="+env.sourceID, &snapshot)
			assert.Equal(t, http.StatusOK, status)
			assert.Equal(t, testCase.expected, snapshot.Freshness)
		})
	}
}

func TestMirror_IntentionallyStopped_BootstrapReportsTerminalFreshness(t *testing.T) {
	env := newMirrorTestEnv(t)

	records := []models.PublicFeedRecord{env.makeRecord(1, map[string]any{"v": "1"})}
	batch := env.buildBatch(records, constants.PublicFeedZeroHashHex)
	_, resp := env.sendIngest(batch)
	require.True(t, resp.Accepted)

	require.NoError(t, env.mirror.SetSourceFreshness(context.Background(), env.sourceID, models.CampaignFreshnessIntentionallyStopped))

	var bootstrap models.PublicFeedBootstrap
	status := env.getJSON("/bootstrap?source="+env.sourceID, &bootstrap)
	assert.Equal(t, http.StatusOK, status)
	assert.Equal(t, models.CampaignFreshnessIntentionallyStopped, bootstrap.SourceFreshness)
	assert.Equal(t, models.CampaignFreshnessIntentionallyStopped, bootstrap.Snapshot.Freshness)
}

// ---------------------------------------------------------------------------
// Key rotation tests
// ---------------------------------------------------------------------------

func TestMirror_KeyRegistration_RequiresAuthenticationStrictPayloadAndCurrentKeySignature(t *testing.T) {
	tests := []struct {
		name           string
		authenticate   bool
		mutate         func(*models.PublicKeyRegistrationRequest)
		appendTrailing bool
		expectedStatus int
		expectedKeys   int
	}{
		{name: "missing authentication", expectedStatus: http.StatusUnauthorized, expectedKeys: 1},
		{name: "invalid current key signature", authenticate: true, mutate: func(request *models.PublicKeyRegistrationRequest) {
			request.Signature = strings.Repeat("00", ed25519.SignatureSize)
		}, expectedStatus: http.StatusBadRequest, expectedKeys: 1},
		{name: "mismatched key identifier", authenticate: true, mutate: func(request *models.PublicKeyRegistrationRequest) { request.NewKeyID = "wrong-key" }, expectedStatus: http.StatusBadRequest, expectedKeys: 1},
		{name: "trailing JSON", authenticate: true, appendTrailing: true, expectedStatus: http.StatusBadRequest, expectedKeys: 1},
		{name: "valid signed registration", authenticate: true, expectedStatus: http.StatusOK, expectedKeys: 2},
	}
	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			env := newMirrorTestEnv(t)
			env.mirror.SetIngestAuthToken("registration-token")
			newPublicKey, _, err := ed25519.GenerateKey(nil)
			require.NoError(t, err)
			digest := sha256.Sum256(newPublicKey)
			request := models.PublicKeyRegistrationRequest{
				SourceID:     env.sourceID,
				CurrentKeyID: env.keyID,
				NewKeyID:     hex.EncodeToString(digest[:]),
				PublicKey:    hex.EncodeToString(newPublicKey),
			}
			signingBytes, err := publicKeyRegistrationSigningBytes(request)
			require.NoError(t, err)
			request.Signature = hex.EncodeToString(ed25519.Sign(env.priv, signingBytes))
			if testCase.mutate != nil {
				testCase.mutate(&request)
			}
			body, err := json.Marshal(request)
			require.NoError(t, err)
			if testCase.appendTrailing {
				body = append(body, []byte(`{}`)...)
			}
			httpRequest, err := http.NewRequest(http.MethodPost, env.server.URL+"/keys/register", bytes.NewReader(body))
			require.NoError(t, err)
			if testCase.authenticate {
				httpRequest.Header.Set("Authorization", "Bearer registration-token")
			}
			response, err := env.client.Do(httpRequest)
			require.NoError(t, err)
			require.NoError(t, response.Body.Close())
			assert.Equal(t, testCase.expectedStatus, response.StatusCode)
			state, err := env.mirror.store.Load(context.Background())
			require.NoError(t, err)
			assert.Len(t, state.KeyRegistry, testCase.expectedKeys)
		})
	}
}

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
	require.NoError(t, env.mirror.RegisterSourceKey(context.Background(), env.sourceID, newKeyID, newPub))

	// Revoke old key.
	require.NoError(t, env.mirror.RevokeSourceKey(context.Background(), env.sourceID, env.keyID))

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
	require.NoError(t, env.mirror.RegisterSourceKey(context.Background(), env.sourceID, "new-key", newPub))
	require.NoError(t, env.mirror.RevokeSourceKey(context.Background(), env.sourceID, env.keyID))

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
	require.NoError(t, env.mirror.RegisterSourceKey(context.Background(), source2ID, key2ID, pub2))

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

func TestMirror_ProofIngest_AuthenticatesValidatesAndAtomicallyStoresPackage(t *testing.T) {
	env := newMirrorTestEnv(t)
	env.mirror.SetIngestAuthToken("proof-token")
	content := []byte(`{"campaign_id":"c1","verification_status":"verified"}`)
	request := env.makeProofIngestRequest("proof.json", content)
	artifactID := request.Artifacts[0].ArtifactID
	requestBody, err := json.Marshal(request)
	require.NoError(t, err)
	req, err := http.NewRequest(http.MethodPost, env.server.URL+"/proof-ingest", bytes.NewReader(requestBody))
	require.NoError(t, err)
	req.Header.Set("Authorization", "Bearer proof-token")
	req.Header.Set("Content-Type", "application/json")
	response, err := env.client.Do(req)
	require.NoError(t, err)
	var ingestResponse models.PublicProofIngestResponse
	require.NoError(t, json.NewDecoder(response.Body).Decode(&ingestResponse))
	require.NoError(t, response.Body.Close())
	assert.Equal(t, http.StatusOK, response.StatusCode)
	assert.True(t, ingestResponse.Accepted)
	assert.Equal(t, 1, ingestResponse.ArtifactCount)

	response, err = env.client.Get(env.server.URL + "/proofs/" + artifactID)
	require.NoError(t, err)
	stored, err := io.ReadAll(response.Body)
	require.NoError(t, err)
	require.NoError(t, response.Body.Close())
	assert.Equal(t, content, stored)
}

func TestMirror_ProofIngest_RejectsInvalidPackageWithoutPartialPersistence(t *testing.T) {
	testCases := []struct {
		name     string
		filename string
		content  []byte
		auth     bool
		mutate   func(*models.PublicProofIngestRequest)
	}{
		{name: "missing authentication", filename: "proof.json", content: []byte(`{"campaign_id":"c1"}`), auth: false},
		{name: "path traversal filename", filename: "../proof.json", content: []byte(`{"campaign_id":"c1"}`), auth: true},
		{name: "nested restricted field", filename: "proof.json", content: []byte(`{"metadata":{"api_key":"restricted"}}`), auth: true},
		{name: "invalid manifest signature", filename: "proof.json", content: []byte(`{"campaign_id":"c1"}`), auth: true, mutate: func(request *models.PublicProofIngestRequest) {
			request.Manifest.Signature = constants.PublicFeedZeroHashHex
		}},
		{name: "artifact hash substitution", filename: "proof.json", content: []byte(`{"campaign_id":"c1"}`), auth: true, mutate: func(request *models.PublicProofIngestRequest) {
			request.Artifacts[0].Content = []byte(`{"campaign_id":"substituted"}`)
		}},
		{name: "catalog manifest mismatch", filename: "proof.json", content: []byte(`{"campaign_id":"c1"}`), auth: true, mutate: func(request *models.PublicProofIngestRequest) {
			request.Catalog.Entries[0].Filename = "different.json"
		}},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			env := newMirrorTestEnv(t)
			env.mirror.SetIngestAuthToken("proof-token")
			request := env.makeProofIngestRequest(testCase.filename, testCase.content)
			if testCase.mutate != nil {
				testCase.mutate(&request)
			}
			requestBody, err := json.Marshal(request)
			require.NoError(t, err)
			req, err := http.NewRequest(http.MethodPost, env.server.URL+"/proof-ingest", bytes.NewReader(requestBody))
			require.NoError(t, err)
			if testCase.auth {
				req.Header.Set("Authorization", "Bearer proof-token")
			}
			response, err := env.client.Do(req)
			require.NoError(t, err)
			require.NoError(t, response.Body.Close())
			if testCase.auth {
				assert.Equal(t, http.StatusBadRequest, response.StatusCode)
			} else {
				assert.Equal(t, http.StatusUnauthorized, response.StatusCode)
			}
			assert.Empty(t, env.mirror.state.ProofArtifacts)
			assert.Empty(t, env.mirror.state.ProofCatalogs)
			assert.Empty(t, env.mirror.state.ProofManifests)
		})
	}
}

// TestMirror_ProofDownload_ServesContentAddressedArtifact verifies that the
// mirror serves proof artifacts under content-addressed URLs with safe
// headers.
func TestMirror_ProofDownload_ServesContentAddressedArtifact(t *testing.T) {
	env := newMirrorTestEnv(t)

	content := []byte(`{"proof": "test"}`)
	proofRequest := env.makeProofIngestRequest("proof.json", content)
	require.NoError(t, env.mirror.storeProofPackage(context.Background(), proofRequest))
	artifactID := proofRequest.Artifacts[0].ArtifactID

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
func TestMirror_RestartRecoversProofCatalogArtifactAndKeyRevocation(t *testing.T) {
	env := newMirrorTestEnv(t)
	content := []byte(`{"proof":"durable"}`)
	proofRequest := env.makeProofIngestRequest("proof.json", content)
	require.NoError(t, env.mirror.storeProofPackage(context.Background(), proofRequest))
	artifactID := proofRequest.Artifacts[0].ArtifactID
	require.NoError(t, env.mirror.RevokeSourceKey(context.Background(), env.sourceID, env.keyID))

	mirror2, err := NewPublicMirrorServer(testutil.NewTestLogger(), NewRuntimePublicMirrorStore(env.fileSvc))
	require.NoError(t, err)
	server2 := httptest.NewServer(mirror2.Handler())
	t.Cleanup(server2.Close)
	response, err := server2.Client().Get(server2.URL + "/proofs/" + artifactID)
	require.NoError(t, err)
	body, err := io.ReadAll(response.Body)
	require.NoError(t, err)
	require.NoError(t, response.Body.Close())
	assert.Equal(t, http.StatusOK, response.StatusCode)
	assert.Equal(t, content, body)

	batch := env.buildBatch([]models.PublicFeedRecord{env.makeRecord(1, map[string]any{"v": "1"})}, constants.PublicFeedZeroHashHex)
	requestBody, err := json.Marshal(models.PublicIngestRequest{Batch: batch})
	require.NoError(t, err)
	response, err = server2.Client().Post(server2.URL+"/ingest", "application/json", bytes.NewReader(requestBody))
	require.NoError(t, err)
	var ingestResponse models.PublicIngestResponse
	require.NoError(t, json.NewDecoder(response.Body).Decode(&ingestResponse))
	require.NoError(t, response.Body.Close())
	assert.False(t, ingestResponse.Accepted)
	assert.Equal(t, models.PublicFeedIngestRejectionRevokedKey, ingestResponse.RejectionReason)
}

func TestMirror_RestartRejectsCorruptDurableProofMetadata(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(string, *PublicMirrorStoreState)
	}{
		{
			name: "catalog path traversal",
			mutate: func(sourceID string, state *PublicMirrorStoreState) {
				catalog := state.ProofCatalogs[sourceID]
				catalog.Entries[0].Filename = "../private.json"
				state.ProofCatalogs[sourceID] = catalog
			},
		},
		{
			name: "manifest root mismatch",
			mutate: func(sourceID string, state *PublicMirrorStoreState) {
				manifest := state.ProofManifests[sourceID]
				manifest.ProofRootSHA256 = constants.PublicFeedZeroHashHex
				state.ProofManifests[sourceID] = manifest
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			env := newMirrorTestEnv(t)
			ctx := context.Background()
			request := env.makeProofIngestRequest("proof.json", []byte(`{"campaign_id":"c1","verification_ok":true}`))
			require.NoError(t, env.mirror.storeProofPackage(ctx, request))
			state, err := NewRuntimePublicMirrorStore(env.fileSvc).Load(ctx)
			require.NoError(t, err)
			tt.mutate(env.sourceID, &state)
			stateBytes, err := json.Marshal(state)
			require.NoError(t, err)
			require.NoError(t, env.fileSvc.WriteFile(ctx, constants.PublicMirrorStatePath, stateBytes, constants.PermFilePrivate))

			_, err = NewPublicMirrorServer(testutil.NewTestLogger(), NewRuntimePublicMirrorStore(env.fileSvc))

			require.Error(t, err)
			assert.ErrorIs(t, err, constants.ErrPublicFeedMirrorStoreCorrupt)
		})
	}
}

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
	proofRequest := env.makeProofIngestRequest("proof.json", content)
	require.NoError(t, env.mirror.storeProofPackage(context.Background(), proofRequest))
	artifactID := proofRequest.Artifacts[0].ArtifactID

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
	proofRequest := env.makeProofIngestRequest("proof.json", []byte(`{"proof":"test"}`))
	require.NoError(t, env.mirror.storeProofPackage(context.Background(), proofRequest))

	var result models.PublicProofManifest
	status := env.getJSON("/proof-manifest?source="+env.sourceID, &result)
	assert.Equal(t, http.StatusOK, status)
	assert.Equal(t, "c1", result.CampaignID)
	assert.Equal(t, proofRequest.Manifest.ProofRootSHA256, result.ProofRootSHA256)
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
	newPublicKey, _, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)
	newKeyDigest := sha256.Sum256(newPublicKey)
	newKeyID := hex.EncodeToString(newKeyDigest[:])
	require.NoError(t, env.mirror.RegisterSourceKey(context.Background(), env.sourceID, newKeyID, newPublicKey))
	revocation := models.PublicKeyRevocationRecord{
		SourceID:     env.sourceID,
		RevokedKeyID: env.keyID,
		RevokedAt:    time.Now().UTC(),
		NewKeyID:     newKeyID,
	}
	signingBytes, err := publicKeyRevocationSigningBytes(revocation)
	require.NoError(t, err)
	revocation.RevocationSignature = hex.EncodeToString(ed25519.Sign(env.priv, signingBytes))
	revBytes, err := json.Marshal(revocation)
	require.NoError(t, err)
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
	state, err := env.mirror.store.Load(context.Background())
	require.NoError(t, err)
	assert.Equal(t, revocation.RevokedAt, state.RevokedKeys[env.sourceID+":"+env.keyID])
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
	proofRequest := env.makeProofIngestRequest("proof.json", content)
	require.NoError(t, env.mirror.storeProofPackage(context.Background(), proofRequest))

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
