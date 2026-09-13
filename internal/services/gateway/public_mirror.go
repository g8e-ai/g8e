// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package gateway

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"path"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/models"
	"github.com/g8e-ai/g8e/v2/internal/services/fs"
)

type PublicMirrorSourceState struct {
	HighWaterSequence         int64                     `json:"high_water_sequence"`
	FeedChainHash             string                    `json:"feed_chain_hash"`
	BatchCount                int                       `json:"batch_count"`
	RetainedFromSequence      int64                     `json:"retained_from_sequence"`
	RetainedPreviousBatchHash string                    `json:"retained_previous_batch_hash"`
	Records                   []models.PublicFeedRecord `json:"records"`
	Batches                   []models.PublicFeedBatch  `json:"batches"`
	Freshness                 models.CampaignFreshness  `json:"freshness"`
	LastAcceptedAt            time.Time                 `json:"last_accepted_at"`
}

type PublicMirrorStoreState struct {
	Sources        map[string]*PublicMirrorSourceState   `json:"sources"`
	KeyRegistry    map[string]ed25519.PublicKey          `json:"key_registry"`
	RevokedKeys    map[string]time.Time                  `json:"revoked_keys"`
	ProofArtifacts map[string][]byte                     `json:"proof_artifacts"`
	ProofCatalogs  map[string]models.PublicProofCatalog  `json:"proof_catalogs"`
	ProofManifests map[string]models.PublicProofManifest `json:"proof_manifests"`
}

type PublicMirrorStore interface {
	Load(context.Context) (PublicMirrorStoreState, error)
	Save(context.Context, PublicMirrorStoreState) error
}

type runtimePublicMirrorStore struct {
	fileSvc fs.RuntimeFileService
	mu      sync.Mutex
}

func NewRuntimePublicMirrorStore(fileSvc fs.RuntimeFileService) PublicMirrorStore {
	return &runtimePublicMirrorStore{fileSvc: fileSvc}
}

func (s *runtimePublicMirrorStore) Load(ctx context.Context) (PublicMirrorStoreState, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	exists, err := s.fileSvc.FileExists(ctx, constants.PublicMirrorStatePath)
	if err != nil {
		return PublicMirrorStoreState{}, fmt.Errorf("public mirror store: stat: %w", err)
	}
	if !exists {
		return newPublicMirrorStoreState(), nil
	}
	data, err := s.fileSvc.ReadFile(ctx, constants.PublicMirrorStatePath)
	if err != nil {
		return PublicMirrorStoreState{}, fmt.Errorf("public mirror store: read: %w", err)
	}
	var state PublicMirrorStoreState
	if err := json.Unmarshal(data, &state); err != nil {
		return PublicMirrorStoreState{}, fmt.Errorf("%w: decode state: %v", constants.ErrPublicFeedMirrorStoreCorrupt, err)
	}
	normalizePublicMirrorStoreState(&state)
	if err := validatePublicMirrorStoreState(state); err != nil {
		return PublicMirrorStoreState{}, err
	}
	return state, nil
}

func (s *runtimePublicMirrorStore) Save(ctx context.Context, state PublicMirrorStoreState) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := validatePublicMirrorStoreState(state); err != nil {
		return err
	}
	data, err := json.Marshal(state)
	if err != nil {
		return fmt.Errorf("%w: encode state: %v", constants.ErrPublicFeedMirrorStoreWrite, err)
	}
	if err := s.fileSvc.WriteFile(ctx, constants.PublicMirrorStatePath, data, constants.PermFilePrivate); err != nil {
		return fmt.Errorf("%w: %v", constants.ErrPublicFeedMirrorStoreWrite, err)
	}
	return nil
}

func newPublicMirrorStoreState() PublicMirrorStoreState {
	return PublicMirrorStoreState{
		Sources:        make(map[string]*PublicMirrorSourceState),
		KeyRegistry:    make(map[string]ed25519.PublicKey),
		RevokedKeys:    make(map[string]time.Time),
		ProofArtifacts: make(map[string][]byte),
		ProofCatalogs:  make(map[string]models.PublicProofCatalog),
		ProofManifests: make(map[string]models.PublicProofManifest),
	}
}

func normalizePublicMirrorStoreState(state *PublicMirrorStoreState) {
	if state.Sources == nil {
		state.Sources = make(map[string]*PublicMirrorSourceState)
	}
	if state.KeyRegistry == nil {
		state.KeyRegistry = make(map[string]ed25519.PublicKey)
	}
	if state.RevokedKeys == nil {
		state.RevokedKeys = make(map[string]time.Time)
	}
	if state.ProofArtifacts == nil {
		state.ProofArtifacts = make(map[string][]byte)
	}
	if state.ProofCatalogs == nil {
		state.ProofCatalogs = make(map[string]models.PublicProofCatalog)
	}
	if state.ProofManifests == nil {
		state.ProofManifests = make(map[string]models.PublicProofManifest)
	}
	for _, source := range state.Sources {
		if source.RetainedFromSequence == 0 {
			source.RetainedFromSequence = 1
		}
		if source.RetainedPreviousBatchHash == "" {
			source.RetainedPreviousBatchHash = constants.PublicFeedZeroHashHex
		}
	}
}

func validatePublicMirrorStoreState(state PublicMirrorStoreState) error {
	for keyID, key := range state.KeyRegistry {
		if keyID == "" || len(key) != ed25519.PublicKeySize {
			return fmt.Errorf("%w: invalid key registry entry", constants.ErrPublicFeedMirrorStoreCorrupt)
		}
	}
	for sourceID, source := range state.Sources {
		if sourceID == "" || source == nil || source.BatchCount < len(source.Batches) || source.RetainedFromSequence <= 0 || source.RetainedPreviousBatchHash == "" {
			return fmt.Errorf("%w: invalid source state", constants.ErrPublicFeedMirrorStoreCorrupt)
		}
		previousHash := source.RetainedPreviousBatchHash
		previousSequence := source.RetainedFromSequence - 1
		var recordCount int
		for _, batch := range source.Batches {
			if batch.SourceID != sourceID || batch.FirstSequence != previousSequence+1 || batch.PreviousBatchHash != previousHash || computeBatchContentHash(batch) != batch.ContentHash {
				return fmt.Errorf("%w: invalid source batch chain", constants.ErrPublicFeedMirrorStoreCorrupt)
			}
			key, ok := state.KeyRegistry[sourceID+":"+batch.SigningKeyID]
			if !ok || verifyBatchSignature(batch, key) != nil {
				return fmt.Errorf("%w: invalid source batch signature", constants.ErrPublicFeedMirrorStoreCorrupt)
			}
			if len(batch.RecordHashes) != len(batch.Records) || batch.LastSequence-batch.FirstSequence+1 != int64(len(batch.Records)) {
				return fmt.Errorf("%w: invalid source batch records", constants.ErrPublicFeedMirrorStoreCorrupt)
			}
			if recordCount+len(batch.Records) > len(source.Records) {
				return fmt.Errorf("%w: missing source records", constants.ErrPublicFeedMirrorStoreCorrupt)
			}
			for index, record := range batch.Records {
				hash := sha256.Sum256([]byte(record.RecordBytes))
				if record.Sequence != batch.FirstSequence+int64(index) || record.RecordHash != batch.RecordHashes[index] || record.RecordHash != hex.EncodeToString(hash[:]) || record != source.Records[recordCount+index] {
					return fmt.Errorf("%w: invalid source record", constants.ErrPublicFeedMirrorStoreCorrupt)
				}
			}
			previousHash = batch.ContentHash
			previousSequence = batch.LastSequence
			recordCount += len(batch.Records)
		}
		if len(source.Batches) > 0 {
			if source.HighWaterSequence != previousSequence || source.FeedChainHash != previousHash || len(source.Records) != recordCount || source.LastAcceptedAt.IsZero() {
				return fmt.Errorf("%w: source summary mismatch", constants.ErrPublicFeedMirrorStoreCorrupt)
			}
		} else if source.HighWaterSequence != 0 || source.BatchCount != 0 || len(source.Records) != 0 {
			return fmt.Errorf("%w: empty source summary mismatch", constants.ErrPublicFeedMirrorStoreCorrupt)
		}
	}
	for artifactID, content := range state.ProofArtifacts {
		hash := sha256.Sum256(content)
		if artifactID != hex.EncodeToString(hash[:]) || len(content) > constants.PublicFeedMaxArtifactBytes {
			return fmt.Errorf("%w: invalid proof artifact", constants.ErrPublicFeedMirrorStoreCorrupt)
		}
	}
	if len(state.ProofCatalogs) != len(state.ProofManifests) {
		return fmt.Errorf("%w: incomplete proof metadata", constants.ErrPublicFeedMirrorStoreCorrupt)
	}
	for sourceID, catalog := range state.ProofCatalogs {
		manifest, ok := state.ProofManifests[sourceID]
		if !ok {
			return fmt.Errorf("%w: missing proof manifest", constants.ErrPublicFeedMirrorStoreCorrupt)
		}
		artifacts := make([]models.PublicProofIngestArtifact, 0, len(catalog.Entries))
		for _, entry := range catalog.Entries {
			content, ok := state.ProofArtifacts[entry.ArtifactID]
			if !ok {
				return fmt.Errorf("%w: missing proof artifact", constants.ErrPublicFeedMirrorStoreCorrupt)
			}
			artifacts = append(artifacts, models.PublicProofIngestArtifact{ArtifactID: entry.ArtifactID, Content: content})
		}
		request := models.PublicProofIngestRequest{SourceID: sourceID, Manifest: manifest, Catalog: catalog, Artifacts: artifacts}
		if err := validatePublicProofPackage(request, state, false); err != nil {
			return fmt.Errorf("%w: proof metadata for %s: %w", constants.ErrPublicFeedMirrorStoreCorrupt, sourceID, err)
		}
	}
	return nil
}

func clonePublicMirrorStoreState(state PublicMirrorStoreState) (PublicMirrorStoreState, error) {
	data, err := json.Marshal(state)
	if err != nil {
		return PublicMirrorStoreState{}, fmt.Errorf("public mirror store: clone encode: %w", err)
	}
	var cloned PublicMirrorStoreState
	if err := json.Unmarshal(data, &cloned); err != nil {
		return PublicMirrorStoreState{}, fmt.Errorf("public mirror store: clone decode: %w", err)
	}
	normalizePublicMirrorStoreState(&cloned)
	return cloned, nil
}

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
	store  PublicMirrorStore
	state  PublicMirrorStoreState

	// sseSubscribers tracks active SSE connections for broadcast.
	sseSubscribers map[*mirrorSSESubscriber]struct{}

	// ingestAuthToken is the shared secret for ingest authentication. If
	// empty, ingest authentication is disabled (test mode).
	ingestAuthToken string

	// maxBootstrapProjections is the bounded number of recent projections
	// included in the bootstrap response.
	maxBootstrapProjections int

	// maxSSEQueueSize is the bounded SSE queue size per subscriber.
	maxSSEQueueSize    int
	maxRetainedBatches int

	// defaultPageSize is the default cursor page size.
	defaultPageSize int

	// maxPageSize is the maximum cursor page size.
	maxPageSize int

	now                  func() time.Time
	freshnessDelayed     time.Duration
	freshnessStale       time.Duration
	freshnessOffline     time.Duration
	anonymousRateMax     int
	anonymousRateWindow  time.Duration
	anonymousRateClients map[string]publicMirrorRateWindow
}

// mirrorSSESubscriber represents one active SSE connection.
type mirrorSSESubscriber struct {
	sourceID  string
	clientID  string
	ch        chan models.PublicFeedRecord
	closed    bool
	truncated bool
	mu        sync.Mutex
}

type publicMirrorRateWindow struct {
	StartedAt time.Time
	Count     int
}

// NewPublicMirrorServer creates a new reference mirror server with default
// limits. The caller registers source public keys via RegisterSourceKey
// before starting ingest.
func NewPublicMirrorServer(logger *slog.Logger, store PublicMirrorStore) (*PublicMirrorServer, error) {
	state, err := store.Load(context.Background())
	if err != nil {
		return nil, fmt.Errorf("public mirror: load store: %w", err)
	}
	return &PublicMirrorServer{
		logger:                  logger,
		store:                   store,
		state:                   state,
		sseSubscribers:          make(map[*mirrorSSESubscriber]struct{}),
		maxBootstrapProjections: 50,
		maxSSEQueueSize:         100,
		maxRetainedBatches:      constants.PublicFeedMirrorRetainedBatches,
		defaultPageSize:         20,
		maxPageSize:             100,
		now:                     time.Now,
		freshnessDelayed:        time.Duration(constants.PublicFeedFreshnessDelayedSeconds) * time.Second,
		freshnessStale:          time.Duration(constants.PublicFeedFreshnessStaleSeconds) * time.Second,
		freshnessOffline:        time.Duration(constants.PublicFeedFreshnessOfflineSeconds) * time.Second,
		anonymousRateMax:        constants.PublicFeedAnonymousRatePerWindow,
		anonymousRateWindow:     time.Duration(constants.PublicFeedAnonymousRateWindowSecs) * time.Second,
		anonymousRateClients:    make(map[string]publicMirrorRateWindow),
	}, nil
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

func (m *PublicMirrorServer) SetMaxRetainedBatches(maxBatches int) error {
	if maxBatches <= 0 {
		return constants.ErrPublicFeedRetentionConfig
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.maxRetainedBatches = maxBatches
	return nil
}

func (m *PublicMirrorServer) SetFreshnessWindows(delayed, stale, offline time.Duration) error {
	if delayed <= 0 || stale <= delayed || offline <= stale {
		return constants.ErrPublicFeedFreshnessConfig
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.freshnessDelayed = delayed
	m.freshnessStale = stale
	m.freshnessOffline = offline
	return nil
}

func (m *PublicMirrorServer) SetAnonymousReadRateLimit(maxRequests int, window time.Duration) error {
	if maxRequests <= 0 || window <= 0 {
		return constants.ErrPublicFeedRateLimitConfig
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.anonymousRateMax = maxRequests
	m.anonymousRateWindow = window
	m.anonymousRateClients = make(map[string]publicMirrorRateWindow)
	return nil
}

func (m *PublicMirrorServer) addSSESubscriber(sourceID, clientID string) (*mirrorSSESubscriber, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for subscriber := range m.sseSubscribers {
		if subscriber.clientID == clientID {
			return nil, false
		}
	}
	subscriber := &mirrorSSESubscriber{
		sourceID: sourceID,
		clientID: clientID,
		ch:       make(chan models.PublicFeedRecord, m.maxSSEQueueSize),
	}
	m.sseSubscribers[subscriber] = struct{}{}
	return subscriber, true
}

func (m *PublicMirrorServer) removeSSESubscriber(subscriber *mirrorSSESubscriber) {
	m.mu.Lock()
	delete(m.sseSubscribers, subscriber)
	m.mu.Unlock()
	subscriber.close()
}

// RegisterSourceKey registers a public key for a source deployment. The
// mirror accepts batches signed by this key until it is revoked.
func (m *PublicMirrorServer) RegisterSourceKey(ctx context.Context, sourceID, keyID string, pubKey ed25519.PublicKey) error {
	return m.mutateState(ctx, func(state *PublicMirrorStoreState) error {
		state.KeyRegistry[m.keyRegistryKey(sourceID, keyID)] = append(ed25519.PublicKey(nil), pubKey...)
		return nil
	})
}

// RevokeSourceKey revokes a signing key for a source deployment. Subsequent
// batches signed by the revoked key are rejected with revoked_key.
func (m *PublicMirrorServer) RevokeSourceKey(ctx context.Context, sourceID, keyID string) error {
	return m.mutateState(ctx, func(state *PublicMirrorStoreState) error {
		state.RevokedKeys[m.keyRegistryKey(sourceID, keyID)] = time.Now().UTC()
		return nil
	})
}

// SetSourceFreshness sets the freshness label for a source deployment. Used
// by tests to simulate stale, stopped, or offline sources.
func (m *PublicMirrorServer) SetSourceFreshness(ctx context.Context, sourceID string, freshness models.CampaignFreshness) error {
	return m.mutateState(ctx, func(storeState *PublicMirrorStoreState) error {
		state := getOrCreatePublicMirrorSource(storeState, sourceID)
		state.Freshness = freshness
		return nil
	})
}

// keyRegistryKey computes the composite registry key.
func (m *PublicMirrorServer) keyRegistryKey(sourceID, keyID string) string {
	return sourceID + ":" + keyID
}

// getOrCreateSource returns the source state, creating it if absent. Caller
// must hold the write lock.
func getOrCreatePublicMirrorSource(state *PublicMirrorStoreState, sourceID string) *PublicMirrorSourceState {
	source, ok := state.Sources[sourceID]
	if !ok {
		source = &PublicMirrorSourceState{
			FeedChainHash:             constants.PublicFeedZeroHashHex,
			RetainedFromSequence:      1,
			RetainedPreviousBatchHash: constants.PublicFeedZeroHashHex,
			Freshness:                 models.CampaignFreshnessActive,
		}
		state.Sources[sourceID] = source
	}
	return source
}

func (m *PublicMirrorServer) mutateState(ctx context.Context, mutate func(*PublicMirrorStoreState) error) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	next, err := clonePublicMirrorStoreState(m.state)
	if err != nil {
		return err
	}
	if err := mutate(&next); err != nil {
		return err
	}
	if err := m.store.Save(ctx, next); err != nil {
		return err
	}
	m.state = next
	return nil
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
	if batch.FirstSequence <= 0 || batch.LastSequence < batch.FirstSequence || batch.LastSequence-batch.FirstSequence+1 != int64(len(batch.Records)) || len(batch.RecordHashes) != len(batch.Records) {
		return models.PublicFeedIngestRejectionSequenceOutOfOrder, constants.ErrPublicFeedSequenceOutOfOrder
	}
	var totalBytes int
	for _, r := range batch.Records {
		totalBytes += len(r.RecordBytes)
	}
	if totalBytes > constants.PublicFeedBatchMaxBytes {
		return models.PublicFeedIngestRejectionOversizedBatch, constants.ErrPublicFeedBatchOversized
	}

	m.mu.RLock()
	pubKey, keyKnown := m.state.KeyRegistry[m.keyRegistryKey(batch.SourceID, batch.SigningKeyID)]
	_, isRevoked := m.state.RevokedKeys[m.keyRegistryKey(batch.SourceID, batch.SigningKeyID)]
	state, sourceExists := m.state.Sources[batch.SourceID]
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
	for index, r := range batch.Records {
		if r.Sequence != batch.FirstSequence+int64(index) || r.RecordHash != batch.RecordHashes[index] {
			return models.PublicFeedIngestRejectionSequenceOutOfOrder, constants.ErrPublicFeedSequenceOutOfOrder
		}
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
		highWater = state.HighWaterSequence
		feedChainHash = state.FeedChainHash
	}

	if batch.FirstSequence <= highWater {
		for _, accepted := range state.Batches {
			if accepted.FirstSequence != batch.FirstSequence || accepted.LastSequence != batch.LastSequence {
				continue
			}
			if accepted.ContentHash == batch.ContentHash {
				return "", nil
			}
			return models.PublicFeedIngestRejectionEquivocation, constants.ErrPublicFeedEquivocation
		}
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

func (m *PublicMirrorServer) sourceFreshness(state *PublicMirrorSourceState) models.CampaignFreshness {
	if state.Freshness == models.CampaignFreshnessIntentionallyStopped || state.Freshness == models.CampaignFreshnessSafetyStopped {
		return state.Freshness
	}
	if state.LastAcceptedAt.IsZero() {
		return models.CampaignFreshnessSourceOffline
	}
	age := m.now().UTC().Sub(state.LastAcceptedAt)
	switch {
	case age >= m.freshnessOffline:
		return models.CampaignFreshnessSourceOffline
	case age >= m.freshnessStale:
		return models.CampaignFreshnessStale
	case age >= m.freshnessDelayed:
		return models.CampaignFreshnessDelayed
	default:
		return models.CampaignFreshnessActive
	}
}

func (m *PublicMirrorServer) batchAlreadyAccepted(batch models.PublicFeedBatch) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	state, ok := m.state.Sources[batch.SourceID]
	if !ok {
		return false
	}
	for _, accepted := range state.Batches {
		if accepted.FirstSequence == batch.FirstSequence && accepted.LastSequence == batch.LastSequence && accepted.ContentHash == batch.ContentHash {
			return true
		}
	}
	return false
}

// acceptBatch atomically persists an accepted batch before broadcasting its
// records to in-memory SSE subscribers.
func (m *PublicMirrorServer) acceptBatch(ctx context.Context, batch models.PublicFeedBatch) error {
	if err := m.mutateState(ctx, func(storeState *PublicMirrorStoreState) error {
		state := getOrCreatePublicMirrorSource(storeState, batch.SourceID)
		if batch.FirstSequence != state.HighWaterSequence+1 || batch.PreviousBatchHash != state.FeedChainHash {
			return constants.ErrPublicFeedHashChainMismatch
		}
		state.Records = append(state.Records, batch.Records...)
		state.Batches = append(state.Batches, batch)
		state.HighWaterSequence = batch.LastSequence
		state.FeedChainHash = batch.ContentHash
		state.BatchCount++
		if excess := len(state.Batches) - m.maxRetainedBatches; excess > 0 {
			lastRemoved := state.Batches[excess-1]
			state.RetainedFromSequence = lastRemoved.LastSequence + 1
			state.RetainedPreviousBatchHash = lastRemoved.ContentHash
			state.Batches = append([]models.PublicFeedBatch(nil), state.Batches[excess:]...)
			firstRetainedRecord := 0
			for firstRetainedRecord < len(state.Records) && state.Records[firstRetainedRecord].Sequence < state.RetainedFromSequence {
				firstRetainedRecord++
			}
			state.Records = append([]models.PublicFeedRecord(nil), state.Records[firstRetainedRecord:]...)
		}
		state.LastAcceptedAt = m.now().UTC()
		state.Freshness = models.CampaignFreshnessActive
		return nil
	}); err != nil {
		return err
	}

	m.mu.RLock()
	subscribers := make([]*mirrorSSESubscriber, 0, len(m.sseSubscribers))
	for sub := range m.sseSubscribers {
		if sub.sourceID == batch.SourceID {
			subscribers = append(subscribers, sub)
		}
	}
	m.mu.RUnlock()
	for _, sub := range subscribers {
		for _, record := range batch.Records {
			sub.send(record)
		}
	}
	return nil
}

// Handler returns the http.Handler for the mirror server. Mount at the
// mirror root path.
func (m *PublicMirrorServer) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/ingest", m.handleIngest)
	mux.HandleFunc("/proof-ingest", m.handleProofIngest)
	mux.HandleFunc("/bootstrap", m.handleBootstrap)
	mux.HandleFunc("/snapshot", m.handleSnapshot)
	mux.HandleFunc("/history", m.handleHistory)
	mux.HandleFunc("/stream", m.handleStream)
	mux.HandleFunc("/proof-catalog", m.handleProofCatalog)
	mux.HandleFunc("/proof-manifest", m.handleProofManifest)
	mux.HandleFunc("/proofs/", m.handleProofDownload)
	return m.withCORS(mux)
}

func publicMirrorAnonymousReadPath(requestPath string) bool {
	switch requestPath {
	case "/bootstrap", "/snapshot", "/history", "/stream", "/proof-catalog", "/proof-manifest":
		return true
	default:
		return strings.HasPrefix(requestPath, "/proofs/")
	}
}

func publicMirrorClientID(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

func (m *PublicMirrorServer) allowAnonymousRead(r *http.Request) bool {
	host := publicMirrorClientID(r)
	now := m.now().UTC()
	m.mu.Lock()
	defer m.mu.Unlock()
	for client, window := range m.anonymousRateClients {
		if now.Sub(window.StartedAt) >= m.anonymousRateWindow {
			delete(m.anonymousRateClients, client)
		}
	}
	window, ok := m.anonymousRateClients[host]
	if !ok {
		if len(m.anonymousRateClients) >= constants.PublicFeedAnonymousRateMaxClients {
			return false
		}
		m.anonymousRateClients[host] = publicMirrorRateWindow{StartedAt: now, Count: 1}
		return true
	}
	if window.Count >= m.anonymousRateMax {
		return false
	}
	window.Count++
	m.anonymousRateClients[host] = window
	return true
}

// withCORS wraps the handler with CORS headers for anonymous read endpoints.
// Ingest does not need CORS (it is server-to-server).
func (m *PublicMirrorServer) withCORS(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/ingest" && r.URL.Path != "/proof-ingest" {
			w.Header().Set("Access-Control-Allow-Origin", "*")
			w.Header().Set("Access-Control-Allow-Methods", "GET, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Last-Event-ID")
			w.Header().Set("Access-Control-Max-Age", "86400")
		}
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		if r.Method == http.MethodGet && publicMirrorAnonymousReadPath(r.URL.Path) && !m.allowAnonymousRead(r) {
			w.Header().Set("Retry-After", strconv.Itoa(int(m.anonymousRateWindow.Seconds())))
			http.Error(w, constants.ErrPublicFeedRateLimited.Error(), http.StatusTooManyRequests)
			return
		}
		h.ServeHTTP(w, r)
	})
}

func (m *PublicMirrorServer) ingestAuthorized(r *http.Request) bool {
	m.mu.RLock()
	token := m.ingestAuthToken
	m.mu.RUnlock()
	if token == "" {
		return true
	}
	provided := r.Header.Get("Authorization")
	expected := "Bearer " + token
	return subtle.ConstantTimeCompare([]byte(provided), []byte(expected)) == 1
}

// handleIngest handles POST /ingest — the authenticated ingest endpoint.
func (m *PublicMirrorServer) handleIngest(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Authenticate.
	if !m.ingestAuthorized(r) {
		resp := models.PublicIngestResponse{Accepted: false, RejectionReason: models.PublicFeedIngestRejectionSignatureInvalid}
		m.writeJSON(w, http.StatusUnauthorized, resp)
		return
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

	if !m.batchAlreadyAccepted(req.Batch) {
		if err := m.acceptBatch(r.Context(), req.Batch); err != nil {
			m.logger.Error("mirror: persist accepted batch", "source_id", req.Batch.SourceID, "error", err)
			resp := models.PublicIngestResponse{Accepted: false}
			m.writeJSON(w, http.StatusInternalServerError, resp)
			return
		}
	}

	m.mu.RLock()
	state := m.state.Sources[req.Batch.SourceID]
	hw := state.HighWaterSequence
	fch := state.FeedChainHash
	m.mu.RUnlock()

	resp := models.PublicIngestResponse{
		Accepted:          true,
		HighWaterSequence: hw,
		FeedChainHash:     fch,
	}
	m.writeJSON(w, http.StatusOK, resp)
}

func (m *PublicMirrorServer) handleProofIngest(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !m.ingestAuthorized(r) {
		m.writeJSON(w, http.StatusUnauthorized, models.PublicProofIngestResponse{Accepted: false})
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, constants.PublicFeedProofIngestMaxBytes)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	var request models.PublicProofIngestRequest
	if err := decoder.Decode(&request); err != nil {
		m.writeJSON(w, http.StatusBadRequest, models.PublicProofIngestResponse{Accepted: false})
		return
	}
	var trailing json.RawMessage
	if err := decoder.Decode(&trailing); err != io.EOF {
		m.writeJSON(w, http.StatusBadRequest, models.PublicProofIngestResponse{Accepted: false})
		return
	}
	if err := m.storeProofPackage(r.Context(), request); err != nil {
		m.logger.Warn("mirror: proof ingest rejected", "source_id", request.SourceID, "error", err)
		m.writeJSON(w, http.StatusBadRequest, models.PublicProofIngestResponse{Accepted: false})
		return
	}
	m.writeJSON(w, http.StatusOK, models.PublicProofIngestResponse{
		Accepted:      true,
		ArtifactCount: len(request.Artifacts),
		ProofRoot:     request.Manifest.ProofRootSHA256,
	})
}

func (m *PublicMirrorServer) storeProofPackage(ctx context.Context, request models.PublicProofIngestRequest) error {
	return m.mutateState(ctx, func(state *PublicMirrorStoreState) error {
		if err := validatePublicProofIngest(request, *state); err != nil {
			return err
		}
		for _, artifact := range request.Artifacts {
			if existing, ok := state.ProofArtifacts[artifact.ArtifactID]; ok && !bytes.Equal(existing, artifact.Content) {
				return constants.ErrPublicFeedEquivocation
			}
			state.ProofArtifacts[artifact.ArtifactID] = append([]byte(nil), artifact.Content...)
		}
		state.ProofCatalogs[request.SourceID] = request.Catalog
		state.ProofManifests[request.SourceID] = request.Manifest
		return nil
	})
}

func validatePublicProofIngest(request models.PublicProofIngestRequest, state PublicMirrorStoreState) error {
	return validatePublicProofPackage(request, state, true)
}

func validatePublicProofPackage(request models.PublicProofIngestRequest, state PublicMirrorStoreState, rejectRevoked bool) error {
	manifest := request.Manifest
	if request.SourceID == "" || !manifest.VerificationOK || manifest.SchemaVersion != constants.PublicProofManifestSchemaVersion || request.Catalog.SchemaVersion != constants.PublicProofCatalogSchemaVersion {
		return constants.ErrPublicFeedProofManifestInvalid
	}
	if manifest.ArtifactCount <= 0 || manifest.ArtifactCount > constants.PublicFeedProofMaxArtifacts || manifest.ArtifactCount != len(manifest.Artifacts) || manifest.ArtifactCount != len(request.Catalog.Entries) || manifest.ArtifactCount != len(request.Artifacts) {
		return constants.ErrPublicFeedProofCatalogMismatch
	}
	keyRegistryID := request.SourceID + ":" + manifest.SigningKeyID
	if _, revoked := state.RevokedKeys[keyRegistryID]; rejectRevoked && revoked {
		return constants.ErrPublicFeedRevokedKey
	}
	key, ok := state.KeyRegistry[keyRegistryID]
	if !ok {
		return constants.ErrPublicFeedUnknownKey
	}
	computedRoot := computePublicProofRootHash(manifest)
	if computedRoot != manifest.ProofRootSHA256 {
		return constants.ErrPublicFeedProofManifestInvalid
	}
	rootBytes, err := hex.DecodeString(manifest.ProofRootSHA256)
	if err != nil || len(rootBytes) != sha256.Size {
		return constants.ErrPublicFeedProofManifestInvalid
	}
	signature, err := hex.DecodeString(manifest.Signature)
	if err != nil || !ed25519.Verify(key, rootBytes, signature) {
		return constants.ErrPublicFeedSignatureInvalid
	}

	artifacts := make(map[string][]byte, len(request.Artifacts))
	for _, artifact := range request.Artifacts {
		if artifact.ArtifactID == "" {
			return constants.ErrPublicFeedProofHashMismatch
		}
		if _, duplicate := artifacts[artifact.ArtifactID]; duplicate {
			return constants.ErrPublicFeedProofCatalogMismatch
		}
		artifacts[artifact.ArtifactID] = artifact.Content
	}
	for index, entry := range request.Catalog.Entries {
		if entry != manifest.Artifacts[index] || entry.Classification != models.PublicFeedProofClassificationPublicSafe || entry.ArtifactID != entry.SHA256 {
			return constants.ErrPublicFeedProofCatalogMismatch
		}
		if !safePublicProofFilename(entry.Filename) || entry.ImmutableURL != "/proofs/"+entry.ArtifactID {
			return constants.ErrPublicFeedProofPathTraversal
		}
		content, ok := artifacts[entry.ArtifactID]
		if !ok {
			return constants.ErrPublicFeedProofCatalogMismatch
		}
		if len(content) > constants.PublicFeedMaxArtifactBytes || int64(len(content)) != entry.ByteSize {
			return constants.ErrPublicFeedProofSizeMismatch
		}
		hash := sha256.Sum256(content)
		if entry.SHA256 != hex.EncodeToString(hash[:]) {
			return constants.ErrPublicFeedProofHashMismatch
		}
		if strings.Contains(entry.MediaType, "json") {
			if err := checkProhibitedFields(string(content)); err != nil {
				return fmt.Errorf("%w: %v", constants.ErrPublicFeedProofRestricted, err)
			}
		}
	}
	return nil
}

func safePublicProofFilename(filename string) bool {
	if filename == "" || path.IsAbs(filename) || strings.Contains(filename, "\\") {
		return false
	}
	cleaned := path.Clean(filename)
	return cleaned == filename && cleaned != "." && cleaned != ".." && path.Base(cleaned) == cleaned
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
		for sid := range m.state.Sources {
			sourceID = sid
			break
		}
	}

	state, ok := m.state.Sources[sourceID]
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
	catalog, hasCatalog := m.state.ProofCatalogs[sourceID]
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
			HighWaterSequence: state.HighWaterSequence,
			FeedChainHash:     state.FeedChainHash,
			BatchCount:        state.BatchCount,
			GeneratedAt:       time.Now().UTC(),
			Freshness:         m.sourceFreshness(state),
		},
		SourceFreshness:     m.sourceFreshness(state),
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
		for sid := range m.state.Sources {
			sourceID = sid
			break
		}
	}

	state, ok := m.state.Sources[sourceID]
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
		HighWaterSequence: state.HighWaterSequence,
		FeedChainHash:     state.FeedChainHash,
		BatchCount:        state.BatchCount,
		GeneratedAt:       time.Now().UTC(),
		Freshness:         m.sourceFreshness(state),
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
		for sid := range m.state.Sources {
			sourceID = sid
			break
		}
	}

	state, ok := m.state.Sources[sourceID]
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
	for _, rec := range state.Records {
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
		for _, rec := range state.Records {
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
		for sid := range m.state.Sources {
			sourceID = sid
			break
		}
	}
	state, stateExists := m.state.Sources[sourceID]
	var replayRecords []models.PublicFeedRecord
	if stateExists {
		for _, rec := range state.Records {
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
	sub, accepted := m.addSSESubscriber(sourceID, publicMirrorClientID(r))
	if !accepted {
		sseWriteSentinel(w, flusher, "error", "client stream already connected")
		flusher.Flush()
		return
	}
	defer m.removeSSESubscriber(sub)

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
			if sub.consumeTruncated() {
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
		for sid := range m.state.ProofCatalogs {
			sourceID = sid
			break
		}
	}

	catalog, ok := m.state.ProofCatalogs[sourceID]
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
		for sid := range m.state.ProofManifests {
			sourceID = sid
			break
		}
	}

	manifest, ok := m.state.ProofManifests[sourceID]
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
	content, ok := m.state.ProofArtifacts[artifactID]
	catalogs := m.state.ProofCatalogs
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
func (m *PublicMirrorServer) recentProjectionsLocked(state *PublicMirrorSourceState, max int) []map[string]any {
	var projections []map[string]any
	for i := len(state.Records) - 1; i >= 0 && len(projections) < max; i-- {
		rec := state.Records[i]
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
	state, ok := m.state.Sources[sourceID]
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
		HighWaterSequence: state.HighWaterSequence,
		FeedChainHash:     state.FeedChainHash,
		BatchCount:        state.BatchCount,
		GeneratedAt:       time.Now().UTC(),
		Freshness:         m.sourceFreshness(state),
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
		select {
		case <-s.ch:
		default:
		}
		select {
		case s.ch <- record:
		default:
		}
		s.truncated = true
	}
}

func (s *mirrorSSESubscriber) consumeTruncated() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	truncated := s.truncated
	s.truncated = false
	return truncated
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
	state, ok := m.state.Sources[sourceID]
	if !ok {
		return 0, "", 0, 0
	}
	return state.HighWaterSequence, state.FeedChainHash, state.BatchCount, len(state.Records)
}

// InjectBatch directly stores a batch without validation. Used by tests that
// need to set up mirror state without going through the ingest endpoint.
func (m *PublicMirrorServer) InjectBatch(ctx context.Context, batch models.PublicFeedBatch) error {
	return m.acceptBatch(ctx, batch)
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
