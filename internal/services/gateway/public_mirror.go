// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package gateway

import (
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/models"
)

// PublicMirrorServer is the hermetic reference mirror HTTP server. It accepts
// signed append-only batches from the Gateway outbound publisher at the
// authenticated ingest endpoint and serves anonymous read-only endpoints for
// public browsers: bootstrap, snapshot, cursor-paginated history, replayable
// SSE, and content-addressed proof downloads.
//
// The mirror verifies every batch signature, hash chain, and sequence ordering
// before accepting. It tracks a high-water sequence and feed-chain hash per
// source. Key rotation is supported via revocation records. The mirror exposes
// no mutation, eval-launch, approval, producer, audit, filesystem, pub/sub,
// MCP, A2A, or tool route.
//
// This implementation is the reference for the OpenDevOps.ai conformance
// suite. A production mirror (Cloudflare Workers, Supabase, or equivalent)
// must pass the same conformance tests.
type PublicMirrorServer struct {
	mu     sync.RWMutex
	logger *slog.Logger

	// sourceStates maps source_id -> *mirrorSourceState.
	sources map[string]*mirrorSourceState

	// keyRegistry maps "source_id:key_id" -> ed25519 public key bytes.
	keyRegistry map[string]ed25519.PublicKey

	// revokedKeys maps "source_id:key_id" -> revocation time.
	revokedKeys map[string]time.Time

	// proofArtifacts maps artifact_id -> []byte content.
	proofArtifacts map[string][]byte

	// proofCatalogs maps source_id -> PublicProofCatalog.
	proofCatalogs map[string]models.PublicProofCatalog

	// proofManifests maps source_id -> PublicProofManifest.
	proofManifests map[string]models.PublicProofManifest

	// sseSubscribers tracks active SSE connections for broadcast.
	sseSubscribers map[*mirrorSSESubscriber]struct{}

	// ingestAuthToken is the shared secret for ingest authentication. If
	// empty, ingest authentication is disabled (test mode).
	ingestAuthToken string

	// maxBootstrapProjections is the bounded number of recent projections
	// included in the bootstrap response.
	maxBootstrapProjections int

	// maxSSEQueueSize is the bounded SSE queue size per subscriber.
	maxSSEQueueSize int

	// defaultPageSize is the default cursor page size.
	defaultPageSize int

	// maxPageSize is the maximum cursor page size.
	maxPageSize int
}

// mirrorSourceState tracks the accepted state for one source deployment.
type mirrorSourceState struct {
	highWaterSeq  int64
	feedChainHash string
	batchCount    int
	records       []models.PublicFeedRecord
	batches       []models.PublicFeedBatch
	freshness     models.CampaignFreshness
	lastUpdated   time.Time
}

// mirrorSSESubscriber represents one active SSE connection.
type mirrorSSESubscriber struct {
	sourceID string
	ch       chan models.PublicFeedRecord
	closed   bool
	mu       sync.Mutex
}

// NewPublicMirrorServer creates a new reference mirror server with default
// limits. The caller registers source public keys via RegisterSourceKey
// before starting ingest.
func NewPublicMirrorServer(logger *slog.Logger) *PublicMirrorServer {
	return &PublicMirrorServer{
		logger:                  logger,
		sources:                 make(map[string]*mirrorSourceState),
		keyRegistry:             make(map[string]ed25519.PublicKey),
		revokedKeys:             make(map[string]time.Time),
		proofArtifacts:          make(map[string][]byte),
		proofCatalogs:           make(map[string]models.PublicProofCatalog),
		proofManifests:          make(map[string]models.PublicProofManifest),
		sseSubscribers:          make(map[*mirrorSSESubscriber]struct{}),
		maxBootstrapProjections: 50,
		maxSSEQueueSize:         100,
		defaultPageSize:         20,
		maxPageSize:             100,
	}
}

// SetIngestAuthToken sets the shared secret for ingest authentication. If
// set, ingest requests must carry an Authorization: Bearer <token> header.
func (m *PublicMirrorServer) SetIngestAuthToken(token string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.ingestAuthToken = token
}

// SetMaxBootstrapProjections sets the bounded number of recent projections
// in the bootstrap response.
func (m *PublicMirrorServer) SetMaxBootstrapProjections(n int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.maxBootstrapProjections = n
}

// SetMaxSSEQueueSize sets the bounded SSE queue size per subscriber.
func (m *PublicMirrorServer) SetMaxSSEQueueSize(n int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.maxSSEQueueSize = n
}

// RegisterSourceKey registers a public key for a source deployment. The
// mirror accepts batches signed by this key until it is revoked.
func (m *PublicMirrorServer) RegisterSourceKey(sourceID, keyID string, pubKey ed25519.PublicKey) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.keyRegistry[m.keyRegistryKey(sourceID, keyID)] = pubKey
}

// RevokeSourceKey revokes a signing key for a source deployment. Subsequent
// batches signed by the revoked key are rejected with revoked_key.
func (m *PublicMirrorServer) RevokeSourceKey(sourceID, keyID string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.revokedKeys[m.keyRegistryKey(sourceID, keyID)] = time.Now().UTC()
}

// StoreProofArtifact stores a proof artifact's content indexed by its
// content-addressed artifact ID (SHA-256 hex).
func (m *PublicMirrorServer) StoreProofArtifact(artifactID string, content []byte) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.proofArtifacts[artifactID] = content
}

// StoreProofCatalog stores the proof catalog for a source deployment.
func (m *PublicMirrorServer) StoreProofCatalog(sourceID string, catalog models.PublicProofCatalog) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.proofCatalogs[sourceID] = catalog
}

// StoreProofManifest stores the proof manifest for a source deployment.
func (m *PublicMirrorServer) StoreProofManifest(sourceID string, manifest models.PublicProofManifest) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.proofManifests[sourceID] = manifest
}

// SetSourceFreshness sets the freshness label for a source deployment. Used
// by tests to simulate stale, stopped, or offline sources.
func (m *PublicMirrorServer) SetSourceFreshness(sourceID string, freshness models.CampaignFreshness) {
	m.mu.Lock()
	defer m.mu.Unlock()
	state := m.getOrCreateSource(sourceID)
	state.freshness = freshness
}

// keyRegistryKey computes the composite registry key.
func (m *PublicMirrorServer) keyRegistryKey(sourceID, keyID string) string {
	return sourceID + ":" + keyID
}

// getOrCreateSource returns the source state, creating it if absent. Caller
// must hold the write lock.
func (m *PublicMirrorServer) getOrCreateSource(sourceID string) *mirrorSourceState {
	state, ok := m.sources[sourceID]
	if !ok {
		state = &mirrorSourceState{
			feedChainHash: constants.PublicFeedZeroHashHex,
			freshness:     models.CampaignFreshnessActive,
		}
		m.sources[sourceID] = state
	}
	return state
}

// computeBatchContentHash recomputes the SHA-256 content hash over the
// canonical batch fields (excluding signature). This must match the
// publisher's computation exactly.
func computeBatchContentHash(batch models.PublicFeedBatch) string {
	h := sha256.New()
	h.Write([]byte(batch.ProtocolVersion))
	h.Write([]byte(batch.SchemaVersion))
	h.Write([]byte(batch.SourceID))
	_, _ = fmt.Fprintf(h, "%d", batch.FirstSequence)
	_, _ = fmt.Fprintf(h, "%d", batch.LastSequence)
	h.Write([]byte(batch.PreviousBatchHash))
	for _, rh := range batch.RecordHashes {
		h.Write([]byte(rh))
	}
	h.Write([]byte(batch.GeneratedAt.UTC().Format(time.RFC3339Nano)))
	return hex.EncodeToString(h.Sum(nil))
}

// verifyBatchSignature verifies the Ed25519 signature over the content hash.
func verifyBatchSignature(batch models.PublicFeedBatch, pubKey ed25519.PublicKey) error {
	computed := computeBatchContentHash(batch)
	if computed != batch.ContentHash {
		return constants.ErrPublicFeedContentHashMismatch
	}
	contentHashBytes, err := hex.DecodeString(batch.ContentHash)
	if err != nil {
		return fmt.Errorf("mirror: decode content hash: %w", err)
	}
	sigBytes, err := hex.DecodeString(batch.Signature)
	if err != nil {
		return fmt.Errorf("mirror: decode signature: %w", err)
	}
	if !ed25519.Verify(pubKey, contentHashBytes, sigBytes) {
		return constants.ErrPublicFeedSignatureInvalid
	}
	return nil
}

// validateBatch performs all ingest validation layers: signature, content
// hash, hash chain, sequence ordering, duplicate sequence, oversized batch,
// revoked key, unknown key, record hash verification, and prohibited field
// scan. Returns a typed rejection reason on failure.
func (m *PublicMirrorServer) validateBatch(batch models.PublicFeedBatch) (models.PublicFeedIngestRejectionReason, error) {
	if len(batch.Records) == 0 {
		return models.PublicFeedIngestRejectionOversizedBatch, constants.ErrPublicFeedBatchEmpty
	}
	if len(batch.Records) > constants.PublicFeedBatchMaxRecords {
		return models.PublicFeedIngestRejectionOversizedBatch, constants.ErrPublicFeedBatchOversized
	}
	var totalBytes int
	for _, r := range batch.Records {
		totalBytes += len(r.RecordBytes)
	}
	if totalBytes > constants.PublicFeedBatchMaxBytes {
		return models.PublicFeedIngestRejectionOversizedBatch, constants.ErrPublicFeedBatchOversized
	}

	m.mu.RLock()
	pubKey, keyKnown := m.keyRegistry[m.keyRegistryKey(batch.SourceID, batch.SigningKeyID)]
	_, isRevoked := m.revokedKeys[m.keyRegistryKey(batch.SourceID, batch.SigningKeyID)]
	state, sourceExists := m.sources[batch.SourceID]
	m.mu.RUnlock()

	if isRevoked {
		return models.PublicFeedIngestRejectionRevokedKey, constants.ErrPublicFeedRevokedKey
	}
	if !keyKnown {
		return models.PublicFeedIngestRejectionUnknownKey, constants.ErrPublicFeedUnknownKey
	}

	if err := verifyBatchSignature(batch, pubKey); err != nil {
		if err == constants.ErrPublicFeedContentHashMismatch {
			return models.PublicFeedIngestRejectionHashChainMismatch, err
		}
		return models.PublicFeedIngestRejectionSignatureInvalid, err
	}

	// Verify each record hash.
	for _, r := range batch.Records {
		computed := sha256.Sum256([]byte(r.RecordBytes))
		if hex.EncodeToString(computed[:]) != r.RecordHash {
			return models.PublicFeedIngestRejectionSignatureInvalid, constants.ErrPublicFeedRecordHashMismatch
		}
		if err := checkProhibitedFields(r.RecordBytes); err != nil {
			return models.PublicFeedIngestRejectionSignatureInvalid, err
		}
	}

	// Sequence and hash chain validation.
	var highWater int64
	var feedChainHash string
	if sourceExists {
		highWater = state.highWaterSeq
		feedChainHash = state.feedChainHash
	}

	if batch.FirstSequence <= highWater {
		return models.PublicFeedIngestRejectionDuplicateSequence, constants.ErrPublicFeedDuplicateSequence
	}
	if sourceExists && batch.PreviousBatchHash != feedChainHash {
		return models.PublicFeedIngestRejectionHashChainMismatch, constants.ErrPublicFeedHashChainMismatch
	}
	if !sourceExists && batch.PreviousBatchHash != constants.PublicFeedZeroHashHex {
		return models.PublicFeedIngestRejectionHashChainMismatch, constants.ErrPublicFeedHashChainMismatch
	}

	return "", nil
}

// acceptBatch stores an accepted batch and its records, updates the source
// state, and broadcasts records to SSE subscribers. Caller must hold the
// write lock for source state updates.
func (m *PublicMirrorServer) acceptBatch(batch models.PublicFeedBatch) {
	m.mu.Lock()
	state := m.getOrCreateSource(batch.SourceID)
	state.records = append(state.records, batch.Records...)
	state.batches = append(state.batches, batch)
	state.highWaterSeq = batch.LastSequence
	state.feedChainHash = batch.ContentHash
	state.batchCount++
	state.lastUpdated = time.Now().UTC()
	subscribers := make([]*mirrorSSESubscriber, 0, len(m.sseSubscribers))
	for sub := range m.sseSubscribers {
		if sub.sourceID == batch.SourceID {
			subscribers = append(subscribers, sub)
		}
	}
	m.mu.Unlock()

	for _, sub := range subscribers {
		for _, record := range batch.Records {
			sub.send(record)
		}
	}
}

// Handler returns the http.Handler for the mirror server. Mount at the
// mirror root path.
func (m *PublicMirrorServer) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/ingest", m.handleIngest)
	mux.HandleFunc("/bootstrap", m.handleBootstrap)
	mux.HandleFunc("/snapshot", m.handleSnapshot)
	mux.HandleFunc("/history", m.handleHistory)
	mux.HandleFunc("/stream", m.handleStream)
	mux.HandleFunc("/proof-catalog", m.handleProofCatalog)
	mux.HandleFunc("/proof-manifest", m.handleProofManifest)
	mux.HandleFunc("/proofs/", m.handleProofDownload)
	return m.withCORS(mux)
}

// withCORS wraps the handler with CORS headers for anonymous read endpoints.
// Ingest does not need CORS (it is server-to-server).
func (m *PublicMirrorServer) withCORS(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/ingest" {
			w.Header().Set("Access-Control-Allow-Origin", "*")
			w.Header().Set("Access-Control-Allow-Methods", "GET, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Last-Event-ID")
			w.Header().Set("Access-Control-Max-Age", "86400")
		}
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		h.ServeHTTP(w, r)
	})
}

// handleIngest handles POST /ingest — the authenticated ingest endpoint.
func (m *PublicMirrorServer) handleIngest(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Authenticate.
	m.mu.RLock()
	token := m.ingestAuthToken
	m.mu.RUnlock()
	if token != "" {
		auth := r.Header.Get("Authorization")
		if auth != "Bearer "+token {
			resp := models.PublicIngestResponse{Accepted: false, RejectionReason: models.PublicFeedIngestRejectionSignatureInvalid}
			m.writeJSON(w, http.StatusUnauthorized, resp)
			return
		}
	}

	var req models.PublicIngestRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		resp := models.PublicIngestResponse{Accepted: false, RejectionReason: models.PublicFeedIngestRejectionSignatureInvalid}
		m.writeJSON(w, http.StatusBadRequest, resp)
		return
	}

	reason, err := m.validateBatch(req.Batch)
	if err != nil {
		m.logger.Warn("mirror: ingest rejected", "source_id", req.Batch.SourceID, "reason", reason, "error", err)
		resp := models.PublicIngestResponse{Accepted: false, RejectionReason: reason}
		m.writeJSON(w, http.StatusOK, resp)
		return
	}

	m.acceptBatch(req.Batch)

	m.mu.RLock()
	state := m.sources[req.Batch.SourceID]
	hw := state.highWaterSeq
	fch := state.feedChainHash
	m.mu.RUnlock()

	resp := models.PublicIngestResponse{
		Accepted:          true,
		HighWaterSequence: hw,
		FeedChainHash:     fch,
	}
	m.writeJSON(w, http.StatusOK, resp)
}

// handleBootstrap handles GET /bootstrap — bounded initial snapshot for
// public first paint.
func (m *PublicMirrorServer) handleBootstrap(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	sourceID := r.URL.Query().Get("source")
	m.mu.RLock()
	defer m.mu.RUnlock()

	if sourceID == "" {
		// Return the first available source.
		for sid := range m.sources {
			sourceID = sid
			break
		}
	}

	state, ok := m.sources[sourceID]
	if !ok {
		m.writeJSON(w, http.StatusOK, models.PublicFeedBootstrap{
			ProtocolVersion: constants.PublicFeedProtocolVersion,
			Snapshot: models.PublicFeedSnapshot{
				ProtocolVersion:   constants.PublicFeedProtocolVersion,
				SourceID:          sourceID,
				HighWaterSequence: 0,
				FeedChainHash:     constants.PublicFeedZeroHashHex,
				BatchCount:        0,
				GeneratedAt:       time.Now().UTC(),
				Freshness:         models.CampaignFreshnessSourceOffline,
			},
			SourceFreshness:     models.CampaignFreshnessSourceOffline,
			RecentProjections:   []map[string]any{},
			ProofCatalogSummary: models.PublicProofCatalogSummary{ArtifactCount: 0, TotalByteSize: 0},
			GeneratedAt:         time.Now().UTC(),
		})
		return
	}

	recent := m.recentProjectionsLocked(state, m.maxBootstrapProjections)
	catalog, hasCatalog := m.proofCatalogs[sourceID]
	summary := models.PublicProofCatalogSummary{ArtifactCount: 0, TotalByteSize: 0}
	if hasCatalog && len(catalog.Entries) > 0 {
		summary.ArtifactCount = len(catalog.Entries)
		var total int64
		for _, e := range catalog.Entries {
			total += e.ByteSize
		}
		summary.TotalByteSize = total
		summary.LastGeneratedAt = &catalog.GeneratedAt
	}

	m.writeJSON(w, http.StatusOK, models.PublicFeedBootstrap{
		ProtocolVersion: constants.PublicFeedProtocolVersion,
		Snapshot: models.PublicFeedSnapshot{
			ProtocolVersion:   constants.PublicFeedProtocolVersion,
			SourceID:          sourceID,
			HighWaterSequence: state.highWaterSeq,
			FeedChainHash:     state.feedChainHash,
			BatchCount:        state.batchCount,
			GeneratedAt:       time.Now().UTC(),
			Freshness:         state.freshness,
		},
		SourceFreshness:     state.freshness,
		RecentProjections:   recent,
		ProofCatalogSummary: summary,
		GeneratedAt:         time.Now().UTC(),
	})
}

// handleSnapshot handles GET /snapshot — current high-water sequence and
// feed-chain hash.
func (m *PublicMirrorServer) handleSnapshot(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	sourceID := r.URL.Query().Get("source")
	m.mu.RLock()
	defer m.mu.RUnlock()

	if sourceID == "" {
		for sid := range m.sources {
			sourceID = sid
			break
		}
	}

	state, ok := m.sources[sourceID]
	if !ok {
		resp := models.PublicFeedSnapshot{
			ProtocolVersion:   constants.PublicFeedProtocolVersion,
			SourceID:          sourceID,
			HighWaterSequence: 0,
			FeedChainHash:     constants.PublicFeedZeroHashHex,
			BatchCount:        0,
			GeneratedAt:       time.Now().UTC(),
			Freshness:         models.CampaignFreshnessSourceOffline,
		}
		m.writeJSON(w, http.StatusOK, resp)
		return
	}

	m.writeJSON(w, http.StatusOK, models.PublicFeedSnapshot{
		ProtocolVersion:   constants.PublicFeedProtocolVersion,
		SourceID:          sourceID,
		HighWaterSequence: state.highWaterSeq,
		FeedChainHash:     state.feedChainHash,
		BatchCount:        state.batchCount,
		GeneratedAt:       time.Now().UTC(),
		Freshness:         state.freshness,
	})
}

// handleHistory handles GET /history — cursor-paginated page of public
// records. The cursor is the last sequence number seen; the response
// returns records after the cursor up to the page limit.
func (m *PublicMirrorServer) handleHistory(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	sourceID := r.URL.Query().Get("source")
	cursorStr := r.URL.Query().Get("cursor")
	limitStr := r.URL.Query().Get("limit")

	limit := m.defaultPageSize
	if limitStr != "" {
		if n, err := strconv.Atoi(limitStr); err == nil && n > 0 {
			limit = n
			if limit > m.maxPageSize {
				limit = m.maxPageSize
			}
		}
	}
	cursor := int64(0)
	if cursorStr != "" {
		if n, err := strconv.ParseInt(cursorStr, 10, 64); err == nil {
			cursor = n
		}
	}

	m.mu.RLock()
	defer m.mu.RUnlock()

	if sourceID == "" {
		for sid := range m.sources {
			sourceID = sid
			break
		}
	}

	state, ok := m.sources[sourceID]
	if !ok {
		m.writeJSON(w, http.StatusOK, models.PublicFeedCursorPage{
			ProtocolVersion: constants.PublicFeedProtocolVersion,
			Items:           []map[string]any{},
			HasMore:         false,
			Limit:           limit,
		})
		return
	}

	var items []map[string]any
	var lastSeq int64
	count := 0
	for _, rec := range state.records {
		if rec.Sequence <= cursor {
			continue
		}
		if count >= limit {
			break
		}
		var item map[string]any
		if err := json.Unmarshal([]byte(rec.RecordBytes), &item); err != nil {
			continue
		}
		item["sequence"] = rec.Sequence
		item["record_type"] = rec.RecordType
		items = append(items, item)
		lastSeq = rec.Sequence
		count++
	}

	hasMore := false
	nextCursor := ""
	if count == limit {
		for _, rec := range state.records {
			if rec.Sequence > lastSeq {
				hasMore = true
				nextCursor = strconv.FormatInt(lastSeq, 10)
				break
			}
		}
	}

	if items == nil {
		items = []map[string]any{}
	}

	m.writeJSON(w, http.StatusOK, models.PublicFeedCursorPage{
		ProtocolVersion: constants.PublicFeedProtocolVersion,
		Items:           items,
		Cursor:          nextCursor,
		HasMore:         hasMore,
		Limit:           limit,
	})
}

// handleStream handles GET /stream — replayable SSE. The client connects with
// an optional since_id query parameter (or Last-Event-ID header) to resume
// from a specific sequence. The server sends existing records after the
// cursor, then live records as they arrive. A bounded queue prevents
// unbounded memory growth; overflow sends a truncation sentinel.
func (m *PublicMirrorServer) handleStream(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	sourceID := r.URL.Query().Get("source")
	sinceIDStr := r.URL.Query().Get("since_id")
	if sinceIDStr == "" {
		sinceIDStr = r.Header.Get("Last-Event-ID")
	}
	sinceID := int64(0)
	if sinceIDStr != "" {
		if n, err := strconv.ParseInt(sinceIDStr, 10, 64); err == nil {
			sinceID = n
		}
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
		return
	}

	m.mu.RLock()
	if sourceID == "" {
		for sid := range m.sources {
			sourceID = sid
			break
		}
	}
	state, stateExists := m.sources[sourceID]
	var replayRecords []models.PublicFeedRecord
	if stateExists {
		for _, rec := range state.records {
			if rec.Sequence > sinceID {
				replayRecords = append(replayRecords, rec)
			}
		}
	}
	m.mu.RUnlock()

	// Send snapshot event first.
	snapshot := m.getSnapshotForSSE(sourceID)
	sseWriteEvent(w, flusher, "snapshot", snapshot)

	// Send replay records.
	for _, rec := range replayRecords {
		sseWriteRecord(w, flusher, rec)
	}

	if !stateExists {
		sseWriteSentinel(w, flusher, "error", "source not found")
		flusher.Flush()
		return
	}

	// Subscribe to live updates.
	sub := &mirrorSSESubscriber{
		sourceID: sourceID,
		ch:       make(chan models.PublicFeedRecord, m.maxSSEQueueSize),
	}
	m.mu.Lock()
	m.sseSubscribers[sub] = struct{}{}
	m.mu.Unlock()
	defer func() {
		m.mu.Lock()
		delete(m.sseSubscribers, sub)
		m.mu.Unlock()
		sub.close()
	}()

	ctx := r.Context()
	for {
		select {
		case <-ctx.Done():
			return
		case rec, open := <-sub.ch:
			if !open {
				sseWriteSentinel(w, flusher, "error", "stream closed")
				flusher.Flush()
				return
			}
			if rec.Sequence <= sinceID {
				continue
			}
			sseWriteRecord(w, flusher, rec)
		default:
			// Check for queue overflow.
			if len(sub.ch) >= m.maxSSEQueueSize {
				sseWriteSentinel(w, flusher, "truncated", "queue overflow")
				flusher.Flush()
				return
			}
			// Brief wait to avoid busy loop.
			select {
			case <-ctx.Done():
				return
			case rec, open := <-sub.ch:
				if !open {
					sseWriteSentinel(w, flusher, "error", "stream closed")
					flusher.Flush()
					return
				}
				if rec.Sequence <= sinceID {
					continue
				}
				sseWriteRecord(w, flusher, rec)
			case <-time.After(50 * time.Millisecond):
				flusher.Flush()
			}
		}
	}
}

// handleProofCatalog handles GET /proof-catalog — the full public proof
// catalog.
func (m *PublicMirrorServer) handleProofCatalog(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	sourceID := r.URL.Query().Get("source")
	m.mu.RLock()
	defer m.mu.RUnlock()

	if sourceID == "" {
		for sid := range m.proofCatalogs {
			sourceID = sid
			break
		}
	}

	catalog, ok := m.proofCatalogs[sourceID]
	if !ok {
		m.writeJSON(w, http.StatusOK, models.PublicProofCatalog{
			SchemaVersion: constants.PublicProofCatalogSchemaVersion,
			Entries:       []models.PublicProofCatalogEntry{},
			GeneratedAt:   time.Now().UTC(),
		})
		return
	}
	if catalog.Entries == nil {
		catalog.Entries = []models.PublicProofCatalogEntry{}
	}
	m.writeJSON(w, http.StatusOK, catalog)
}

// handleProofManifest handles GET /proof-manifest — the proof root manifest.
func (m *PublicMirrorServer) handleProofManifest(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	sourceID := r.URL.Query().Get("source")
	m.mu.RLock()
	defer m.mu.RUnlock()

	if sourceID == "" {
		for sid := range m.proofManifests {
			sourceID = sid
			break
		}
	}

	manifest, ok := m.proofManifests[sourceID]
	if !ok {
		http.Error(w, "proof manifest not found", http.StatusNotFound)
		return
	}
	m.writeJSON(w, http.StatusOK, manifest)
}

// handleProofDownload handles GET /proofs/:artifactID — content-addressed
// proof artifact download with safe headers.
func (m *PublicMirrorServer) handleProofDownload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	artifactID := strings.TrimPrefix(r.URL.Path, "/proofs/")
	if artifactID == "" {
		http.Error(w, "artifact id required", http.StatusBadRequest)
		return
	}

	m.mu.RLock()
	content, ok := m.proofArtifacts[artifactID]
	catalogs := m.proofCatalogs
	m.mu.RUnlock()

	if !ok {
		http.Error(w, "proof not found", http.StatusNotFound)
		return
	}

	// Find the catalog entry for safe headers.
	var entry *models.PublicProofCatalogEntry
	for _, cat := range catalogs {
		for i := range cat.Entries {
			if cat.Entries[i].ArtifactID == artifactID {
				entry = &cat.Entries[i]
				break
			}
		}
		if entry != nil {
			break
		}
	}

	// Verify hash before serving.
	h := sha256.Sum256(content)
	computedHash := hex.EncodeToString(h[:])
	if computedHash != artifactID {
		http.Error(w, "proof hash mismatch", http.StatusInternalServerError)
		return
	}

	if entry != nil {
		w.Header().Set("Content-Type", entry.MediaType)
		w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s\"", entry.Filename))
	} else {
		w.Header().Set("Content-Type", "application/octet-stream")
	}
	w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	w.Header().Set("Content-Length", strconv.Itoa(len(content)))
	if _, err := w.Write(content); err != nil {
		m.logger.Error("mirror: write proof response", "error", err)
	}
}

// recentProjectionsLocked returns the most recent projection records as
// deserialized JSON objects. Caller must hold the read lock.
func (m *PublicMirrorServer) recentProjectionsLocked(state *mirrorSourceState, max int) []map[string]any {
	var projections []map[string]any
	for i := len(state.records) - 1; i >= 0 && len(projections) < max; i-- {
		rec := state.records[i]
		if rec.RecordType != models.PublicFeedRecordTypeProjection {
			continue
		}
		var item map[string]any
		if err := json.Unmarshal([]byte(rec.RecordBytes), &item); err != nil {
			continue
		}
		item["sequence"] = rec.Sequence
		projections = append(projections, item)
	}
	if projections == nil {
		return []map[string]any{}
	}
	return projections
}

// getSnapshotForSSE returns the current snapshot for a source.
func (m *PublicMirrorServer) getSnapshotForSSE(sourceID string) models.PublicFeedSnapshot {
	m.mu.RLock()
	defer m.mu.RUnlock()
	state, ok := m.sources[sourceID]
	if !ok {
		return models.PublicFeedSnapshot{
			ProtocolVersion:   constants.PublicFeedProtocolVersion,
			SourceID:          sourceID,
			HighWaterSequence: 0,
			FeedChainHash:     constants.PublicFeedZeroHashHex,
			BatchCount:        0,
			GeneratedAt:       time.Now().UTC(),
			Freshness:         models.CampaignFreshnessSourceOffline,
		}
	}
	return models.PublicFeedSnapshot{
		ProtocolVersion:   constants.PublicFeedProtocolVersion,
		SourceID:          sourceID,
		HighWaterSequence: state.highWaterSeq,
		FeedChainHash:     state.feedChainHash,
		BatchCount:        state.batchCount,
		GeneratedAt:       time.Now().UTC(),
		Freshness:         state.freshness,
	}
}

// send delivers a record to the subscriber's channel. If the channel is
// full, the record is dropped (the subscriber will see a truncation
// sentinel on its next read).
func (s *mirrorSSESubscriber) send(record models.PublicFeedRecord) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return
	}
	select {
	case s.ch <- record:
	default:
		// Channel full — drop. The subscriber detects overflow via
		// queue length check.
	}
}

// close closes the subscriber's channel.
func (s *mirrorSSESubscriber) close() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return
	}
	s.closed = true
	close(s.ch)
}

// writeJSON writes a JSON response with the given status code.
func (m *PublicMirrorServer) writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		m.logger.Error("mirror: encode JSON response", "error", err)
	}
}

// sseWriteEvent writes a named SSE event with a JSON payload.
func sseWriteEvent(w http.ResponseWriter, flusher http.Flusher, eventType string, payload any) {
	data, _ := json.Marshal(payload)
	fmt.Fprintf(w, "event: %s\ndata: %s\n\n", eventType, string(data))
	flusher.Flush()
}

// sseWriteRecord writes a public feed record as an SSE data event with the
// record sequence as the event ID.
func sseWriteRecord(w http.ResponseWriter, flusher http.Flusher, rec models.PublicFeedRecord) {
	fmt.Fprintf(w, "id: %d\nevent: %s\ndata: %s\n\n", rec.Sequence, rec.RecordType, rec.RecordBytes)
	flusher.Flush()
}

// sseWriteSentinel writes a sentinel SSE event (truncated or error).
func sseWriteSentinel(w http.ResponseWriter, flusher http.Flusher, sentinelType, reason string) {
	payload, _ := json.Marshal(map[string]string{"reason": reason})
	fmt.Fprintf(w, "event: %s\ndata: %s\n\n", sentinelType, string(payload))
	flusher.Flush()
}

// GetSourceState returns a copy of the source state for testing. Returns nil
// if the source has no accepted batches.
func (m *PublicMirrorServer) GetSourceState(sourceID string) (highWater int64, feedChainHash string, batchCount int, recordCount int) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	state, ok := m.sources[sourceID]
	if !ok {
		return 0, "", 0, 0
	}
	return state.highWaterSeq, state.feedChainHash, state.batchCount, len(state.records)
}

// InjectBatch directly stores a batch without validation. Used by tests that
// need to set up mirror state without going through the ingest endpoint.
func (m *PublicMirrorServer) InjectBatch(batch models.PublicFeedBatch) {
	m.acceptBatch(batch)
}

// CloseAllSubscribers closes all SSE subscribers. Used for clean shutdown.
func (m *PublicMirrorServer) CloseAllSubscribers() {
	m.mu.Lock()
	defer m.mu.Unlock()
	for sub := range m.sseSubscribers {
		sub.close()
		delete(m.sseSubscribers, sub)
	}
}

// FlushForTests waits for pending operations to settle. Used in tests after
// ingest to ensure records are visible before reading.
func (m *PublicMirrorServer) FlushForTests() {
	// The acceptBatch method updates state synchronously under the lock,
	// so no flush is needed. This method exists for future async paths.
	_ = context.Background()
}
