// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 0.

package gateway

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/marshaler"
	"github.com/g8e-ai/g8e/v2/internal/models"
	"github.com/g8e-ai/g8e/v2/internal/services/fs"
)

// prohibitedRecordFields are field names that must never appear in a public
// feed record's payload. Their presence causes the record to be rejected
// before signing.
var prohibitedRecordFields = []string{
	"raw_prompt",
	"raw_output",
	"prompt",
	"output",
	"chain_of_thought",
	"credentials",
	"credential",
	"api_key",
	"secret",
	"password",
	"token",
	"private_key",
	"private_endpoint",
	"evidence_key",
	"evidence_key_metadata",
	"machine_path",
	"filesystem_path",
	"local_path",
}

// ProofArtifactInput is the input type for the proof package builder. Each
// artifact is public_safe by construction — the builder does not accept
// restricted artifacts.
type ProofArtifactInput struct {
	Filename    string
	MediaType   string
	Content     []byte
	CampaignID  string
	SourceRunID string
}

// PublicPublisherService is the Gateway outbound publisher. It consumes
// persisted O0-allowlisted projections, builds signed append-only batches,
// durably writes an ordered outbox, and transmits to a hosted mirror over
// outbound HTTPS with idempotent retry. Export never mutates authoritative
// eval reports.
type PublicPublisherService struct {
	docStore *DocumentStoreService
	fileSvc  fs.RuntimeFileService
	logger   *slog.Logger
	cfg      models.PublicExportConfig

	signingPrivKey ed25519.PrivateKey
	signingPubKey  ed25519.PublicKey
	signingKeyID   string

	mu             sync.Mutex
	mirrorOrigin   string
	batchMaxRecords int

	// highWaterSeq is the last acknowledged sequence number.
	highWaterSeq   int64
	feedChainHash  string
	batchCount     int
}

// NewPublicPublisherService creates a new PublicPublisherService backed by
// the given document store, file service, logger, export config, Ed25519
// private key, and signing key ID.
func NewPublicPublisherService(docStore *DocumentStoreService, fileSvc fs.RuntimeFileService, logger *slog.Logger, cfg models.PublicExportConfig, priv ed25519.PrivateKey, keyID string) *PublicPublisherService {
	pub := priv.Public().(ed25519.PublicKey)
	return &PublicPublisherService{
		docStore:        docStore,
		fileSvc:         fileSvc,
		logger:          logger,
		cfg:             cfg,
		signingPrivKey:  priv,
		signingPubKey:   pub,
		signingKeyID:    keyID,
		mirrorOrigin:    cfg.MirrorOrigin,
		batchMaxRecords: cfg.BatchMaxRecords,
		feedChainHash:   constants.PublicFeedZeroHashHex,
	}
}

// SetMirrorOrigin updates the mirror origin URL.
func (s *PublicPublisherService) SetMirrorOrigin(origin string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.mirrorOrigin = origin
}

// SetBatchMaxRecords updates the maximum records per batch limit.
func (s *PublicPublisherService) SetBatchMaxRecords(max int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.batchMaxRecords = max
}

// validRecordType returns true if the record type is in the closed vocabulary.
func validRecordType(rt models.PublicFeedRecordType) bool {
	switch rt {
	case models.PublicFeedRecordTypeProjection,
		models.PublicFeedRecordTypeEvent,
		models.PublicFeedRecordTypeProofManifest,
		models.PublicFeedRecordTypeKeyRevocation:
		return true
	default:
		return false
	}
}

// checkProhibitedFields scans the record bytes for prohibited field names
// in the JSON payload. Returns an error if any prohibited field is found.
func checkProhibitedFields(recordBytes string) error {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal([]byte(recordBytes), &fields); err != nil {
		return fmt.Errorf("public-feed: parse record for prohibited field check: %w", err)
	}
	for _, prohibited := range prohibitedRecordFields {
		if _, ok := fields[prohibited]; ok {
			return fmt.Errorf("public-feed: prohibited field '%s' in record", prohibited)
		}
	}
	return nil
}

// BuildBatch constructs a signed batch from records. It validates record
// hashes, computes the content hash, and signs with Ed25519. The batch
// carries the previous batch hash for chain linking.
func (s *PublicPublisherService) BuildBatch(records []models.PublicFeedRecord) (models.PublicFeedBatch, error) {
	if len(records) == 0 {
		return models.PublicFeedBatch{}, constants.ErrPublicFeedBatchEmpty
	}
	if len(records) > s.batchMaxRecords {
		return models.PublicFeedBatch{}, constants.ErrPublicFeedBatchOversized
	}

	// Validate each record.
	for _, r := range records {
		if !validRecordType(r.RecordType) {
			return models.PublicFeedBatch{}, fmt.Errorf("public-feed: build batch: %w: %s", constants.ErrPublicFeedRecordTypeInvalid, r.RecordType)
		}
		computed := sha256.Sum256([]byte(r.RecordBytes))
		if hex.EncodeToString(computed[:]) != r.RecordHash {
			return models.PublicFeedBatch{}, constants.ErrPublicFeedRecordHashMismatch
		}
		if err := checkProhibitedFields(r.RecordBytes); err != nil {
			return models.PublicFeedBatch{}, fmt.Errorf("public-feed: build batch: %w", err)
		}
	}

	// Compute batch size in bytes.
	var totalBytes int
	for _, r := range records {
		totalBytes += len(r.RecordBytes)
	}
	if totalBytes > s.cfg.BatchMaxBytes {
		return models.PublicFeedBatch{}, constants.ErrPublicFeedBatchOversized
	}

	recordHashes := make([]string, len(records))
	for i, r := range records {
		recordHashes[i] = r.RecordHash
	}

	s.mu.Lock()
	prevHash := s.feedChainHash
	s.mu.Unlock()

	batch := models.PublicFeedBatch{
		ProtocolVersion:   constants.PublicFeedProtocolVersion,
		SchemaVersion:     constants.PublicFeedSchemaVersion,
		SourceID:          s.cfg.SourceID,
		FirstSequence:     records[0].Sequence,
		LastSequence:      records[len(records)-1].Sequence,
		PreviousBatchHash: prevHash,
		RecordHashes:      recordHashes,
		GeneratedAt:       time.Now().UTC(),
		SigningKeyID:      s.signingKeyID,
		Records:           records,
	}

	// Compute content hash over the canonical batch fields (excluding signature).
	contentHash := s.computeBatchContentHash(batch)
	batch.ContentHash = contentHash

	// Sign the decoded content hash bytes with Ed25519.
	contentHashBytes, err := hex.DecodeString(contentHash)
	if err != nil {
		return models.PublicFeedBatch{}, fmt.Errorf("public-feed: build batch: decode content hash: %w", err)
	}
	sig := ed25519.Sign(s.signingPrivKey, contentHashBytes)
	batch.Signature = hex.EncodeToString(sig)

	return batch, nil
}

// computeBatchContentHash computes the SHA-256 content hash over the
// canonical batch fields (protocol_version, schema_version, source_id,
// first_sequence, last_sequence, previous_batch_hash, record_hashes,
// generated_at). The signature field is excluded.
func (s *PublicPublisherService) computeBatchContentHash(batch models.PublicFeedBatch) string {
	h := sha256.New()
	h.Write([]byte(batch.ProtocolVersion))
	h.Write([]byte(batch.SchemaVersion))
	h.Write([]byte(batch.SourceID))
	h.Write([]byte(fmt.Sprintf("%d", batch.FirstSequence)))
	h.Write([]byte(fmt.Sprintf("%d", batch.LastSequence)))
	h.Write([]byte(batch.PreviousBatchHash))
	for _, rh := range batch.RecordHashes {
		h.Write([]byte(rh))
	}
	h.Write([]byte(batch.GeneratedAt.UTC().Format(time.RFC3339Nano)))
	return hex.EncodeToString(h.Sum(nil))
}

// VerifyBatch verifies the batch signature and content hash against the
// provided public key hex string.
func (s *PublicPublisherService) VerifyBatch(batch models.PublicFeedBatch, pubKeyHex string) error {
	// Recompute content hash.
	computed := s.computeBatchContentHash(batch)
	if computed != batch.ContentHash {
		return constants.ErrPublicFeedContentHashMismatch
	}

	// Verify signature.
	pubKeyBytes, err := hex.DecodeString(pubKeyHex)
	if err != nil {
		return fmt.Errorf("public-feed: verify batch: decode public key: %w", err)
	}
	sigBytes, err := hex.DecodeString(batch.Signature)
	if err != nil {
		return fmt.Errorf("public-feed: verify batch: decode signature: %w", err)
	}
	contentHashBytes, err := hex.DecodeString(batch.ContentHash)
	if err != nil {
		return fmt.Errorf("public-feed: verify batch: decode content hash: %w", err)
	}
	if !ed25519.Verify(pubKeyBytes, contentHashBytes, sigBytes) {
		return constants.ErrPublicFeedSignatureInvalid
	}
	return nil
}

// ExportBatch builds a signed batch from records, durably writes it to the
// outbox, transmits it to the mirror with idempotent retry, and advances
// acknowledgment only after signed mirror acceptance.
func (s *PublicPublisherService) ExportBatch(ctx context.Context, records []models.PublicFeedRecord) error {
	if !s.cfg.Enabled {
		return constants.ErrPublicFeedDisabled
	}
	if len(records) == 0 {
		return constants.ErrPublicFeedBatchEmpty
	}
	if len(records) > s.batchMaxRecords {
		return constants.ErrPublicFeedBatchOversized
	}

	batch, err := s.BuildBatch(records)
	if err != nil {
		return fmt.Errorf("public-feed: export batch: build: %w", err)
	}

	// Persist to outbox before network transmission.
	if err := s.writeOutboxEntry(batch); err != nil {
		return fmt.Errorf("public-feed: export batch: write outbox: %w", err)
	}

	// Transmit to mirror with retry.
	if err := s.transmitBatch(ctx, batch); err != nil {
		return fmt.Errorf("public-feed: export batch: transmit: %w", err)
	}

	// Update snapshot state.
	s.mu.Lock()
	s.highWaterSeq = batch.LastSequence
	s.feedChainHash = batch.ContentHash
	s.batchCount++
	s.mu.Unlock()

	// Persist snapshot.
	if err := s.persistSnapshot(); err != nil {
		s.logger.Warn("public-feed: failed to persist snapshot", "error", err)
	}

	return nil
}

// writeOutboxEntry durably writes a batch to the outbox collection.
func (s *PublicPublisherService) writeOutboxEntry(batch models.PublicFeedBatch) error {
	batchBytes, err := json.Marshal(batch)
	if err != nil {
		return fmt.Errorf("marshal batch: %w", err)
	}
	entry := models.PublicOutboxEntry{
		Sequence:   batch.LastSequence,
		BatchHash:  batch.ContentHash,
		BatchBytes: string(batchBytes),
		Status:     models.PublicFeedOutboxStatusPending,
		CreatedAt:  time.Now().UTC(),
	}
	entryBytes, err := json.Marshal(entry)
	if err != nil {
		return fmt.Errorf("marshal outbox entry: %w", err)
	}
	collection := marshaler.CollectionName(constants.CollectionPublicFeedOutbox)
	docID := fmt.Sprintf("%d", batch.LastSequence)
	return s.docStore.DocSet(collection, docID, entryBytes)
}

// transmitBatch sends a batch to the mirror with bounded retry. Returns nil
// only after the mirror accepts the batch.
func (s *PublicPublisherService) transmitBatch(ctx context.Context, batch models.PublicFeedBatch) error {
	s.mu.Lock()
	origin := s.mirrorOrigin
	s.mu.Unlock()

	if origin == "" {
		return constants.ErrPublicFeedMirrorOriginRequired
	}

	maxAttempts := s.cfg.RetryMaxAttempts
	if maxAttempts < 1 {
		maxAttempts = 1
	}
	initialBackoff := time.Duration(s.cfg.RetryInitialBackoffSecs) * time.Second
	maxBackoff := time.Duration(s.cfg.RetryMaxBackoffSecs) * time.Second

	var lastErr error
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		err := s.sendToMirror(ctx, origin, batch)
		if err == nil {
			// Update outbox entry to acknowledged.
			s.updateOutboxStatus(batch.LastSequence, models.PublicFeedOutboxStatusAcknowledged)
			return nil
		}
		lastErr = err
		if isMirrorRejection(err) {
			// Rejections are not retried — the batch is fundamentally invalid.
			s.updateOutboxStatus(batch.LastSequence, models.PublicFeedOutboxStatusFailed)
			return err
		}
		// Retryable error — compute backoff.
		if attempt < maxAttempts {
			backoff := initialBackoff * time.Duration(1<<(attempt-1))
			if backoff > maxBackoff {
				backoff = maxBackoff
			}
			if backoff > 0 {
				select {
				case <-time.After(backoff):
				case <-ctx.Done():
					return ctx.Err()
				}
			}
		}
	}
	s.updateOutboxStatus(batch.LastSequence, models.PublicFeedOutboxStatusFailed)
	return fmt.Errorf("%w: %v", constants.ErrPublicFeedMaxRetriesExceeded, lastErr)
}

// isMirrorRejection returns true if the error is a mirror rejection (not a
// transport error).
func isMirrorRejection(err error) bool {
	return errors.Is(err, constants.ErrPublicFeedMirrorRejected)
}

// sendToMirror sends a single ingest request to the mirror.
func (s *PublicPublisherService) sendToMirror(ctx context.Context, origin string, batch models.PublicFeedBatch) error {
	reqBody := models.PublicIngestRequest{Batch: batch}
	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return fmt.Errorf("marshal ingest request: %w", err)
	}

	url := strings.TrimRight(origin, "/") + "/ingest"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(bodyBytes))
	if err != nil {
		return fmt.Errorf("%w: %v", constants.ErrPublicFeedMirrorUnreachable, err)
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("%w: %v", constants.ErrPublicFeedMirrorUnreachable, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode >= 500 {
		return fmt.Errorf("%w: status %d", constants.ErrPublicFeedMirrorUnreachable, resp.StatusCode)
	}

	var ingestResp models.PublicIngestResponse
	if err := json.NewDecoder(resp.Body).Decode(&ingestResp); err != nil {
		return fmt.Errorf("%w: decode response: %v", constants.ErrPublicFeedMirrorUnreachable, err)
	}

	if !ingestResp.Accepted {
		return fmt.Errorf("%w: %s", constants.ErrPublicFeedMirrorRejected, ingestResp.RejectionReason)
	}

	return nil
}

// updateOutboxStatus updates the status of an outbox entry.
func (s *PublicPublisherService) updateOutboxStatus(seq int64, status models.PublicFeedOutboxStatus) {
	collection := marshaler.CollectionName(constants.CollectionPublicFeedOutbox)
	docID := fmt.Sprintf("%d", seq)
	doc, err := s.docStore.DocGet(collection, docID)
	if err != nil || doc == nil {
		return
	}
	var entry models.PublicOutboxEntry
	if err := unmarshalDocData(doc, &entry); err != nil {
		return
	}
	entry.Status = status
	now := time.Now().UTC()
	if status == models.PublicFeedOutboxStatusAcknowledged {
		entry.AcknowledgedAt = &now
	}
	entry.LastAttemptAt = &now
	entryBytes, err := json.Marshal(entry)
	if err != nil {
		return
	}
	_ = s.docStore.DocSet(collection, docID, entryBytes)
}

// RetransmitOutbox retransmits all outbox entries to the mirror. The mirror
// handles idempotent acceptance of duplicate batches. Entries that previously
// failed transmission are retried.
func (s *PublicPublisherService) RetransmitOutbox(ctx context.Context) error {
	collection := marshaler.CollectionName(constants.CollectionPublicFeedOutbox)
	docs, err := s.docStore.DocList(collection)
	if err != nil {
		return fmt.Errorf("public-feed: retransmit: list outbox: %w", err)
	}
	for _, doc := range docs {
		var entry models.PublicOutboxEntry
		if err := unmarshalDocData(doc, &entry); err != nil {
			continue
		}
		var batch models.PublicFeedBatch
		if err := json.Unmarshal([]byte(entry.BatchBytes), &batch); err != nil {
			continue
		}
		if err := s.transmitBatch(ctx, batch); err != nil {
			return fmt.Errorf("public-feed: retransmit: %w", err)
		}
	}
	return nil
}

// GetSnapshot returns the current high-water sequence and feed-chain hash.
func (s *PublicPublisherService) GetSnapshot(ctx context.Context) (models.PublicFeedSnapshot, error) {
	s.mu.Lock()
	hw := s.highWaterSeq
	fch := s.feedChainHash
	bc := s.batchCount
	s.mu.Unlock()

	if hw == 0 {
		// Try recovering from the outbox entries (e.g. after publisher restart).
		if err := s.loadSnapshotFromOutbox(); err != nil {
			s.logger.Debug("public-feed: no snapshot in outbox", "error", err)
		}
		s.mu.Lock()
		hw = s.highWaterSeq
		fch = s.feedChainHash
		bc = s.batchCount
		s.mu.Unlock()
	}

	if hw == 0 {
		return models.PublicFeedSnapshot{}, constants.ErrPublicFeedSnapshotNotFound
	}

	return models.PublicFeedSnapshot{
		ProtocolVersion:   constants.PublicFeedProtocolVersion,
		SourceID:          s.cfg.SourceID,
		HighWaterSequence: hw,
		FeedChainHash:     fch,
		BatchCount:        bc,
		GeneratedAt:       time.Now().UTC(),
		Freshness:         models.CampaignFreshnessActive,
	}, nil
}

// persistSnapshot writes the current snapshot to the runtime tree.
func (s *PublicPublisherService) persistSnapshot() error {
	s.mu.Lock()
	hw := s.highWaterSeq
	fch := s.feedChainHash
	bc := s.batchCount
	s.mu.Unlock()

	if hw == 0 {
		return nil
	}

	snap := models.PublicFeedSnapshot{
		ProtocolVersion:   constants.PublicFeedProtocolVersion,
		SourceID:          s.cfg.SourceID,
		HighWaterSequence: hw,
		FeedChainHash:     fch,
		BatchCount:        bc,
		GeneratedAt:       time.Now().UTC(),
		Freshness:         models.CampaignFreshnessActive,
	}
	snapBytes, err := json.Marshal(snap)
	if err != nil {
		return fmt.Errorf("marshal snapshot: %w", err)
	}
	return s.fileSvc.WriteFile(context.Background(), constants.PublicFeedSnapshotFilename, snapBytes, constants.PermFilePrivate)
}

// loadSnapshotFromOutbox reconstructs the high-water sequence and feed-chain
// hash from the outbox entries in the docStore. Used when a new publisher
// instance needs to recover state from the database (e.g. after process
// restart with a new fileSvc).
func (s *PublicPublisherService) loadSnapshotFromOutbox() error {
	collection := marshaler.CollectionName(constants.CollectionPublicFeedOutbox)
	docs, err := s.docStore.DocList(collection)
	if err != nil {
		return fmt.Errorf("load snapshot from outbox: list: %w", err)
	}

	var maxSeq int64
	var lastHash string
	count := 0
	for _, doc := range docs {
		var entry models.PublicOutboxEntry
		if err := unmarshalDocData(doc, &entry); err != nil {
			continue
		}
		if entry.Sequence > maxSeq {
			maxSeq = entry.Sequence
			lastHash = entry.BatchHash
			count++
		}
	}

	s.mu.Lock()
	if maxSeq > s.highWaterSeq {
		s.highWaterSeq = maxSeq
		s.feedChainHash = lastHash
		s.batchCount = count
	}
	s.mu.Unlock()
	return nil
}

// RotateKey generates a new Ed25519 key pair, emits a key revocation record
// as a signed batch, and switches the signing key. Returns the new key ID
// and new public key hex.
func (s *PublicPublisherService) RotateKey(ctx context.Context) (string, string, error) {
	newPub, newPriv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return "", "", fmt.Errorf("%w: %v", constants.ErrPublicFeedKeyGenFailed, err)
	}
	newKeyID := fmt.Sprintf("key-%d", time.Now().UnixNano())

	// Build and export a key revocation record batch.
	revRecord := models.PublicKeyRevocationRecord{
		RevokedKeyID:        s.signingKeyID,
		RevokedAt:           time.Now().UTC(),
		NewKeyID:            newKeyID,
		RevocationSignature: hex.EncodeToString(ed25519.Sign(s.signingPrivKey, []byte(s.signingKeyID+newKeyID))),
	}
	revBytes, err := json.Marshal(revRecord)
	if err != nil {
		return "", "", fmt.Errorf("marshal revocation record: %w", err)
	}
	revHash := sha256.Sum256(revBytes)

	s.mu.Lock()
	nextSeq := s.highWaterSeq + 1
	s.mu.Unlock()

	record := models.PublicFeedRecord{
		Sequence:    nextSeq,
		RecordType:  models.PublicFeedRecordTypeKeyRevocation,
		RecordHash:  hex.EncodeToString(revHash[:]),
		RecordBytes: string(revBytes),
	}

	// Export the revocation batch with the old key.
	if err := s.ExportBatch(ctx, []models.PublicFeedRecord{record}); err != nil {
		return "", "", fmt.Errorf("public-feed: rotate key: export revocation: %w", err)
	}

	// Switch to the new key.
	s.mu.Lock()
	s.signingPrivKey = newPriv
	s.signingPubKey = newPub
	s.signingKeyID = newKeyID
	s.mu.Unlock()

	return newKeyID, hex.EncodeToString(newPub), nil
}

// BuildProofPackage creates a complete public proof package from a passing
// verification report. It writes artifacts to disk under the public-proofs
// directory, builds a signed root manifest, and updates the proof catalog.
func (s *PublicPublisherService) BuildProofPackage(ctx context.Context, campaignID, campaignRevision, verifiedIndexGenHash string, verificationOK bool, artifacts []ProofArtifactInput) (models.PublicProofManifest, error) {
	if !verificationOK {
		return models.PublicProofManifest{}, constants.ErrPublicFeedProofNotVerified
	}
	if len(artifacts) == 0 {
		return models.PublicProofManifest{}, constants.ErrPublicFeedBatchEmpty
	}

	// Check for symlinks in the proofs directory before writing.
	proofsDir := s.fileSvc.Resolve(constants.PublicProofsDirname)
	if err := checkNoSymlinks(proofsDir); err != nil {
		return models.PublicProofManifest{}, err
	}

	// Ensure proofs directory exists.
	if err := s.fileSvc.MkdirAll(ctx, constants.PublicProofsDirname, constants.PermDirPrivate); err != nil {
		return models.PublicProofManifest{}, fmt.Errorf("public-feed: build proof: mkdir: %w", err)
	}

	entries := make([]models.PublicProofCatalogEntry, 0, len(artifacts))
	for _, art := range artifacts {
		if int64(len(art.Content)) > int64(constants.PublicFeedMaxArtifactBytes) {
			return models.PublicProofManifest{}, constants.ErrPublicFeedProofOversized
		}

		h := sha256.Sum256(art.Content)
		artifactID := hex.EncodeToString(h[:])

		entry := models.PublicProofCatalogEntry{
			ArtifactID:          artifactID,
			Filename:            art.Filename,
			MediaType:           art.MediaType,
			ByteSize:            int64(len(art.Content)),
			SHA256:              artifactID,
			Classification:      models.PublicFeedProofClassificationPublicSafe,
			CampaignID:          campaignID,
			SourceRunID:         art.SourceRunID,
			GeneratedAt:         time.Now().UTC(),
			VerificationCommand: fmt.Sprintf("sha256sum %s", art.Filename),
			ImmutableURL:        fmt.Sprintf("/proofs/%s", artifactID),
		}

		// Write artifact to disk.
		relPath := filepath.Join(constants.PublicProofsDirname, artifactID)
		if err := s.fileSvc.WriteFile(ctx, relPath, art.Content, constants.PermFilePrivate); err != nil {
			return models.PublicProofManifest{}, fmt.Errorf("public-feed: build proof: write artifact: %w", err)
		}

		entries = append(entries, entry)
	}

	// Compute proof root hash.
	manifest := models.PublicProofManifest{
		SchemaVersion:                constants.PublicProofManifestSchemaVersion,
		CampaignID:                   campaignID,
		CampaignRevision:             campaignRevision,
		VerifiedIndexGenerationHash:  verifiedIndexGenHash,
		VerificationOK:               verificationOK,
		ArtifactCount:                len(entries),
		Artifacts:                    entries,
		VerifierInstructions:         "Verify each artifact SHA-256 matches the catalog entry. Recompute the proof root hash from artifact hashes and manifest metadata. Verify the Ed25519 signature over the proof root hash.",
		GeneratedAt:                  time.Now().UTC(),
		SigningKeyID:                 s.signingKeyID,
	}

	rootHash := s.computeProofRootHash(manifest)
	manifest.ProofRootSHA256 = rootHash

	// Sign the decoded proof root hash bytes with Ed25519.
	rootHashBytes, err := hex.DecodeString(rootHash)
	if err != nil {
		return models.PublicProofManifest{}, fmt.Errorf("public-feed: build proof: decode root hash: %w", err)
	}
	sig := ed25519.Sign(s.signingPrivKey, rootHashBytes)
	manifest.Signature = hex.EncodeToString(sig)

	// Write manifest to disk.
	manifestBytes, err := json.Marshal(manifest)
	if err != nil {
		return models.PublicProofManifest{}, fmt.Errorf("public-feed: build proof: marshal manifest: %w", err)
	}
	if err := s.fileSvc.WriteFile(ctx, constants.PublicProofManifestFilename, manifestBytes, constants.PermFilePrivate); err != nil {
		return models.PublicProofManifest{}, fmt.Errorf("public-feed: build proof: write manifest: %w", err)
	}

	// Update the proof catalog.
	if err := s.updateProofCatalog(ctx, entries); err != nil {
		s.logger.Warn("public-feed: failed to update proof catalog", "error", err)
	}

	return manifest, nil
}

// computeProofRootHash computes the SHA-256 proof root hash over the
// artifact hashes and manifest metadata (excluding the signature).
func (s *PublicPublisherService) computeProofRootHash(manifest models.PublicProofManifest) string {
	h := sha256.New()
	h.Write([]byte(manifest.SchemaVersion))
	h.Write([]byte(manifest.CampaignID))
	h.Write([]byte(manifest.CampaignRevision))
	h.Write([]byte(manifest.VerifiedIndexGenerationHash))
	h.Write([]byte(fmt.Sprintf("%t", manifest.VerificationOK)))
	h.Write([]byte(fmt.Sprintf("%d", manifest.ArtifactCount)))
	for _, entry := range manifest.Artifacts {
		h.Write([]byte(entry.ArtifactID))
		h.Write([]byte(entry.SHA256))
		h.Write([]byte(entry.Filename))
		h.Write([]byte(entry.MediaType))
		h.Write([]byte(fmt.Sprintf("%d", entry.ByteSize)))
	}
	h.Write([]byte(manifest.GeneratedAt.UTC().Format(time.RFC3339Nano)))
	h.Write([]byte(manifest.SigningKeyID))
	return hex.EncodeToString(h.Sum(nil))
}

// ComputeProofRootHash is the public API for recomputing the proof root hash
// from a manifest. This allows offline verification.
func (s *PublicPublisherService) ComputeProofRootHash(manifest models.PublicProofManifest) (string, error) {
	return s.computeProofRootHash(manifest), nil
}

// updateProofCatalog reads the existing catalog, appends new entries, and
// writes it back.
func (s *PublicPublisherService) updateProofCatalog(ctx context.Context, newEntries []models.PublicProofCatalogEntry) error {
	catalog, err := s.GetProofCatalog(ctx)
	if err != nil {
		return err
	}

	// Deduplicate by artifact ID.
	existing := make(map[string]bool)
	for _, e := range catalog.Entries {
		existing[e.ArtifactID] = true
	}
	for _, e := range newEntries {
		if !existing[e.ArtifactID] {
			catalog.Entries = append(catalog.Entries, e)
			existing[e.ArtifactID] = true
		}
	}

	catalog.GeneratedAt = time.Now().UTC()
	catalogBytes, err := json.Marshal(catalog)
	if err != nil {
		return fmt.Errorf("marshal catalog: %w", err)
	}
	return s.fileSvc.WriteFile(ctx, constants.PublicProofCatalogFilename, catalogBytes, constants.PermFilePrivate)
}

// GetProofCatalog returns the full public proof catalog.
func (s *PublicPublisherService) GetProofCatalog(ctx context.Context) (models.PublicProofCatalog, error) {
	exists, err := s.fileSvc.FileExists(ctx, constants.PublicProofCatalogFilename)
	if err != nil {
		return models.PublicProofCatalog{}, fmt.Errorf("public-feed: get catalog: stat: %w", err)
	}
	if !exists {
		return models.PublicProofCatalog{
			SchemaVersion: constants.PublicProofCatalogSchemaVersion,
			Entries:       []models.PublicProofCatalogEntry{},
			GeneratedAt:   time.Now().UTC(),
		}, nil
	}

	data, err := s.fileSvc.ReadFile(ctx, constants.PublicProofCatalogFilename)
	if err != nil {
		return models.PublicProofCatalog{}, fmt.Errorf("public-feed: get catalog: read: %w", err)
	}

	var catalog models.PublicProofCatalog
	if err := json.Unmarshal(data, &catalog); err != nil {
		return models.PublicProofCatalog{}, fmt.Errorf("public-feed: get catalog: unmarshal: %w", err)
	}
	if catalog.Entries == nil {
		catalog.Entries = []models.PublicProofCatalogEntry{}
	}
	return catalog, nil
}

// StreamProof serves a proof artifact by its content-addressed ID. It
// verifies the on-disk file hash against the catalog hash before serving,
// rejects symlinks, and sets fixed safe headers (Cache-Control: immutable,
// Content-Type, Content-Disposition).
func (s *PublicPublisherService) StreamProof(ctx context.Context, artifactID string, w http.ResponseWriter) error {
	if artifactID == "" {
		return constants.ErrPublicFeedProofIDRequired
	}

	// Find the artifact in the catalog.
	catalog, err := s.GetProofCatalog(ctx)
	if err != nil {
		return fmt.Errorf("public-feed: stream proof: get catalog: %w", err)
	}

	var entry *models.PublicProofCatalogEntry
	for i := range catalog.Entries {
		if catalog.Entries[i].ArtifactID == artifactID {
			entry = &catalog.Entries[i]
			break
		}
	}
	if entry == nil {
		return constants.ErrPublicFeedProofNotFound
	}

	// Resolve the file path and check for symlinks.
	relPath := filepath.Join(constants.PublicProofsDirname, artifactID)
	absPath := s.fileSvc.Resolve(relPath)

	// Check if the file is a symlink.
	fi, err := os.Lstat(absPath)
	if err != nil {
		return constants.ErrPublicFeedProofNotFound
	}
	if fi.Mode()&os.ModeSymlink != 0 {
		return constants.ErrPublicFeedProofSymlinkRejected
	}

	// Read the file.
	data, err := os.ReadFile(absPath)
	if err != nil {
		return constants.ErrPublicFeedProofNotFound
	}

	// Verify the hash.
	h := sha256.Sum256(data)
	computedHash := hex.EncodeToString(h[:])
	if computedHash != entry.SHA256 {
		return constants.ErrPublicFeedProofHashMismatch
	}

	// Verify the size.
	if int64(len(data)) != entry.ByteSize {
		return constants.ErrPublicFeedProofSizeMismatch
	}

	// Set safe headers.
	w.Header().Set("Content-Type", entry.MediaType)
	w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s\"", entry.Filename))
	w.Header().Set("Content-Length", fmt.Sprintf("%d", len(data)))

	// Write the body.
	if _, err := w.Write(data); err != nil {
		return fmt.Errorf("public-feed: stream proof: write: %w", err)
	}

	return nil
}

// checkNoSymlinks walks the given directory and returns an error if any
// symlink is found.
func checkNoSymlinks(dir string) error {
	info, err := os.Stat(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	if !info.IsDir() {
		return nil
	}
	return filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return constants.ErrPublicFeedProofSymlinkRejected
		}
		return nil
	})
}
