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
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/models"
	"github.com/g8e-ai/g8e/v2/internal/services/fs"
	"github.com/g8e-ai/g8e/v2/internal/services/publicdisclosure"
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
	"user_id",
	"user_identity",
	"cli_session_id",
	"web_session_id",
	"session_identity",
	"operator_id",
	"operator_session_id",
	"authenticated_identity",
	"endpoint",
	"gateway_url",
	"target_resource",
	"controlled_target",
	"transaction_id",
	"governance_envelope",
	"audit_record",
	"receipt_body",
	"evidence_body",
}

// ProofArtifactInput is the input type for the proof package builder. Each
// artifact is public_safe by construction — the builder does not accept
// restricted artifacts.
type ProofArtifactInput struct {
	Filename            string
	MediaType           string
	Content             []byte
	CampaignID          string
	SourceRunID         string
	VerificationCommand string
}

type PublicOutboxStore interface {
	Append(context.Context, models.PublicOutboxEntry) error
	List(context.Context) ([]models.PublicOutboxEntry, error)
	Update(context.Context, models.PublicOutboxEntry) error
	PruneAcknowledged(context.Context, time.Time) error
	Replace(context.Context, []models.PublicOutboxEntry) error
}

type runtimePublicOutboxStore struct {
	fileSvc fs.RuntimeFileService
	mu      sync.Mutex
}

func newRuntimePublicOutboxStore(fileSvc fs.RuntimeFileService) PublicOutboxStore {
	return &runtimePublicOutboxStore{fileSvc: fileSvc}
}

func (s *runtimePublicOutboxStore) Append(ctx context.Context, entry models.PublicOutboxEntry) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	entries, err := s.read(ctx)
	if err != nil {
		return err
	}
	for _, existing := range entries {
		if existing.Sequence != entry.Sequence {
			continue
		}
		if existing.BatchHash != entry.BatchHash || existing.BatchBytes != entry.BatchBytes {
			return constants.ErrPublicFeedOutboxEquivocation
		}
		return constants.ErrPublicFeedDuplicateSequence
	}
	if err := validatePublicOutboxEntry(entry); err != nil {
		return err
	}
	if len(entries) > 0 {
		last := entries[len(entries)-1]
		var batch models.PublicFeedBatch
		if err := json.Unmarshal([]byte(entry.BatchBytes), &batch); err != nil {
			return fmt.Errorf("%w: decode appended batch: %v", constants.ErrPublicFeedOutboxCorrupt, err)
		}
		if entry.Sequence <= last.Sequence {
			return constants.ErrPublicFeedSequenceOutOfOrder
		}
		if batch.FirstSequence != last.Sequence+1 || batch.PreviousBatchHash != last.BatchHash {
			return constants.ErrPublicFeedHashChainMismatch
		}
	}
	entries = append(entries, entry)
	return s.write(ctx, entries)
}

func (s *runtimePublicOutboxStore) List(ctx context.Context) ([]models.PublicOutboxEntry, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.read(ctx)
}

func (s *runtimePublicOutboxStore) Update(ctx context.Context, entry models.PublicOutboxEntry) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	entries, err := s.read(ctx)
	if err != nil {
		return err
	}
	found := false
	for i := range entries {
		if entries[i].Sequence != entry.Sequence {
			continue
		}
		if entries[i].BatchHash != entry.BatchHash || entries[i].BatchBytes != entry.BatchBytes {
			return constants.ErrPublicFeedOutboxEquivocation
		}
		entries[i] = entry
		found = true
		break
	}
	if !found {
		return constants.ErrPublicFeedOutboxCorrupt
	}
	return s.write(ctx, entries)
}

func (s *runtimePublicOutboxStore) Replace(ctx context.Context, entries []models.PublicOutboxEntry) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.write(ctx, entries)
}

func (s *runtimePublicOutboxStore) readRaw(ctx context.Context) ([]models.PublicOutboxEntry, error) {
	exists, err := s.fileSvc.FileExists(ctx, constants.PublicFeedOutboxPath)
	if err != nil {
		return nil, fmt.Errorf("public-feed: outbox stat: %w", err)
	}
	if !exists {
		return []models.PublicOutboxEntry{}, nil
	}
	data, err := s.fileSvc.ReadFile(ctx, constants.PublicFeedOutboxPath)
	if err != nil {
		return nil, fmt.Errorf("public-feed: outbox read: %w", err)
	}
	lines := bytes.Split(data, []byte{'\n'})
	entries := make([]models.PublicOutboxEntry, 0, len(lines))
	for index, line := range lines {
		if len(bytes.TrimSpace(line)) == 0 {
			continue
		}
		var entry models.PublicOutboxEntry
		if err := json.Unmarshal(line, &entry); err != nil {
			return nil, fmt.Errorf("%w: line %d: %v", constants.ErrPublicFeedOutboxCorrupt, index+1, err)
		}
		if err := validatePublicOutboxEntry(entry); err != nil {
			return nil, fmt.Errorf("%w: line %d: %v", constants.ErrPublicFeedOutboxCorrupt, index+1, err)
		}
		entries = append(entries, entry)
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Sequence < entries[j].Sequence })
	return entries, nil
}

func (s *runtimePublicOutboxStore) PruneAcknowledged(ctx context.Context, before time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	entries, err := s.read(ctx)
	if err != nil {
		return err
	}
	pruned := 0
	for len(entries) > 0 {
		head := entries[0]
		if head.Status != models.PublicFeedOutboxStatusAcknowledged || head.AcknowledgedAt == nil || !head.AcknowledgedAt.Before(before) {
			break
		}
		entries = entries[1:]
		pruned++
	}
	if pruned == 0 {
		return nil
	}
	return s.write(ctx, entries)
}

func (s *runtimePublicOutboxStore) read(ctx context.Context) ([]models.PublicOutboxEntry, error) {
	exists, err := s.fileSvc.FileExists(ctx, constants.PublicFeedOutboxPath)
	if err != nil {
		return nil, fmt.Errorf("public-feed: outbox stat: %w", err)
	}
	if !exists {
		return []models.PublicOutboxEntry{}, nil
	}
	data, err := s.fileSvc.ReadFile(ctx, constants.PublicFeedOutboxPath)
	if err != nil {
		return nil, fmt.Errorf("public-feed: outbox read: %w", err)
	}
	lines := bytes.Split(data, []byte{'\n'})
	entries := make([]models.PublicOutboxEntry, 0, len(lines))
	for index, line := range lines {
		if len(bytes.TrimSpace(line)) == 0 {
			continue
		}
		var entry models.PublicOutboxEntry
		if err := json.Unmarshal(line, &entry); err != nil {
			return nil, fmt.Errorf("%w: line %d: %v", constants.ErrPublicFeedOutboxCorrupt, index+1, err)
		}
		if err := validatePublicOutboxEntry(entry); err != nil {
			return nil, fmt.Errorf("%w: line %d: %v", constants.ErrPublicFeedOutboxCorrupt, index+1, err)
		}
		var batch models.PublicFeedBatch
		if err := json.Unmarshal([]byte(entry.BatchBytes), &batch); err != nil {
			return nil, fmt.Errorf("%w: line %d batch: %v", constants.ErrPublicFeedOutboxCorrupt, index+1, err)
		}
		if len(entries) > 0 {
			previous := entries[len(entries)-1]
			if entry.Sequence == previous.Sequence && (entry.BatchHash != previous.BatchHash || entry.BatchBytes != previous.BatchBytes) {
				return nil, constants.ErrPublicFeedOutboxEquivocation
			}
			if entry.Sequence <= previous.Sequence {
				return nil, constants.ErrPublicFeedOutboxCorrupt
			}
			if batch.FirstSequence != previous.Sequence+1 || batch.PreviousBatchHash != previous.BatchHash {
				return nil, constants.ErrPublicFeedHashChainMismatch
			}
		}
		entries = append(entries, entry)
	}
	return entries, nil
}

func (s *runtimePublicOutboxStore) write(ctx context.Context, entries []models.PublicOutboxEntry) error {
	var data bytes.Buffer
	for _, entry := range entries {
		encoded, err := json.Marshal(entry)
		if err != nil {
			return fmt.Errorf("public-feed: outbox marshal: %w", err)
		}
		data.Write(encoded)
		data.WriteByte('\n')
	}
	if err := s.fileSvc.WriteFile(ctx, constants.PublicFeedOutboxPath, data.Bytes(), constants.PermFilePrivate); err != nil {
		return fmt.Errorf("public-feed: outbox write: %w", err)
	}
	return nil
}

func validatePublicOutboxEntry(entry models.PublicOutboxEntry) error {
	var batch models.PublicFeedBatch
	if err := json.Unmarshal([]byte(entry.BatchBytes), &batch); err != nil {
		return fmt.Errorf("%w: decode batch: %v", constants.ErrPublicFeedOutboxCorrupt, err)
	}
	if entry.Sequence <= 0 || batch.LastSequence != entry.Sequence || batch.ContentHash != entry.BatchHash {
		return constants.ErrPublicFeedOutboxCorrupt
	}
	if computeBatchContentHash(batch) != batch.ContentHash {
		return constants.ErrPublicFeedContentHashMismatch
	}
	return nil
}

// PublicPublisherService is the Gateway outbound publisher. It consumes
// persisted O0-allowlisted projections, builds signed append-only batches,
// durably writes an ordered outbox, and transmits to a hosted mirror over
// outbound HTTPS with idempotent retry. Export never mutates authoritative
// eval reports.
type PublicPublisherService struct {
	docStore *DocumentStoreService
	fileSvc  fs.RuntimeFileService
	outbox   PublicOutboxStore
	logger   *slog.Logger
	cfg      models.PublicExportConfig

	signingPrivKey ed25519.PrivateKey
	signingPubKey  ed25519.PublicKey
	signingKeyID   string

	mu              sync.Mutex
	mirrorOrigin    string
	ingestAuthToken string
	batchMaxRecords int

	// highWaterSeq is the last acknowledged sequence number.
	highWaterSeq  int64
	feedChainHash string
	batchCount    int
}

// NewPublicPublisherService creates a new PublicPublisherService backed by
// the given document store, file service, logger, export config, Ed25519
// private key, and signing key ID.
func NewPublicPublisherService(docStore *DocumentStoreService, fileSvc fs.RuntimeFileService, logger *slog.Logger, cfg models.PublicExportConfig, priv ed25519.PrivateKey, keyID string) *PublicPublisherService {
	pub := priv.Public().(ed25519.PublicKey)
	return &PublicPublisherService{
		docStore:        docStore,
		fileSvc:         fileSvc,
		outbox:          newRuntimePublicOutboxStore(fileSvc),
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

func (s *PublicPublisherService) SetIngestAuthToken(token string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ingestAuthToken = token
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
	var value json.RawMessage
	if err := json.Unmarshal([]byte(recordBytes), &value); err != nil {
		return fmt.Errorf("public-feed: parse record for prohibited field check: %w", err)
	}
	return checkProhibitedJSONValue(value)
}

func checkProhibitedJSONValue(value json.RawMessage) error {
	trimmed := bytes.TrimSpace(value)
	if len(trimmed) == 0 {
		return nil
	}
	switch trimmed[0] {
	case '{':
		var fields map[string]json.RawMessage
		if err := json.Unmarshal(trimmed, &fields); err != nil {
			return fmt.Errorf("public-feed: parse record object: %w", err)
		}
		for name, child := range fields {
			for _, prohibited := range prohibitedRecordFields {
				if name == prohibited {
					return fmt.Errorf("%w: %s", constants.ErrPublicFeedRestrictedField, prohibited)
				}
			}
			if err := checkProhibitedJSONValue(child); err != nil {
				return err
			}
		}
	case '[':
		var values []json.RawMessage
		if err := json.Unmarshal(trimmed, &values); err != nil {
			return fmt.Errorf("public-feed: parse record array: %w", err)
		}
		for _, child := range values {
			if err := checkProhibitedJSONValue(child); err != nil {
				return err
			}
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
		if err := publicdisclosure.ValidatePublicFeedRecord(r.RecordType, []byte(r.RecordBytes)); err != nil {
			return models.PublicFeedBatch{}, fmt.Errorf("public-feed: build record: %w", err)
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
	_, _ = fmt.Fprintf(h, "%d", batch.FirstSequence)
	_, _ = fmt.Fprintf(h, "%d", batch.LastSequence)
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

func isRepairableOutboxError(err error) bool {
	return errors.Is(err, constants.ErrPublicFeedOutboxCorrupt) ||
		errors.Is(err, constants.ErrPublicFeedOutboxEquivocation) ||
		errors.Is(err, constants.ErrPublicFeedHashChainMismatch)
}

func resequencePublicFeedRecords(records []models.PublicFeedRecord, startSequence int64) {
	for index := range records {
		records[index].Sequence = startSequence + int64(index)
	}
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

	err := s.exportBatchOnce(ctx, records)
	if err == nil {
		return nil
	}
	if !isRepairableOutboxError(err) {
		return err
	}
	if repairErr := s.RepairOutboxFromSnapshot(ctx); repairErr != nil {
		return err
	}
	if reloadErr := s.loadSnapshotFromOutbox(ctx); reloadErr != nil {
		return fmt.Errorf("public-feed: export batch: reload after repair: %w", reloadErr)
	}
	s.mu.Lock()
	nextSequence := s.highWaterSeq + 1
	s.mu.Unlock()
	resequencePublicFeedRecords(records, nextSequence)
	return s.exportBatchOnce(ctx, records)
}

func (s *PublicPublisherService) exportBatchOnce(ctx context.Context, records []models.PublicFeedRecord) error {
	if err := s.loadSnapshotFromOutbox(ctx); err != nil {
		return fmt.Errorf("public-feed: export batch: recover outbox: %w", err)
	}
	if err := s.retransmitUnacknowledgedOutbox(ctx); err != nil {
		return fmt.Errorf("public-feed: export batch: retransmit unacknowledged: %w", err)
	}
	s.mu.Lock()
	nextSequence := s.highWaterSeq + 1
	s.mu.Unlock()
	for i, record := range records {
		if record.Sequence != nextSequence+int64(i) {
			return constants.ErrPublicFeedSequenceOutOfOrder
		}
	}

	batch, err := s.BuildBatch(records)
	if err != nil {
		return fmt.Errorf("public-feed: export batch: build: %w", err)
	}

	// Persist to outbox before network transmission.
	if err := s.writeOutboxEntry(ctx, batch); err != nil {
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

	if err := s.persistSnapshot(ctx); err != nil {
		return fmt.Errorf("public-feed: export batch: persist snapshot: %w", err)
	}
	ackWindow := time.Duration(s.cfg.AckWindowSecs) * time.Second
	if err := s.outbox.PruneAcknowledged(ctx, time.Now().UTC().Add(-ackWindow)); err != nil {
		return fmt.Errorf("public-feed: export batch: prune outbox: %w", err)
	}

	return nil
}

// writeOutboxEntry durably writes a batch to the outbox collection.
func (s *PublicPublisherService) writeOutboxEntry(ctx context.Context, batch models.PublicFeedBatch) error {
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
	return s.outbox.Append(ctx, entry)
}

// transmitBatch sends a batch to the mirror with bounded retry. Returns nil
// only after the mirror accepts the batch.
func (s *PublicPublisherService) transmitBatch(ctx context.Context, batch models.PublicFeedBatch) error {
	s.mu.Lock()
	origin := s.mirrorOrigin
	ingestAuthToken := s.ingestAuthToken
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
		if err := s.updateOutboxStatus(ctx, batch.LastSequence, models.PublicFeedOutboxStatusSent, true); err != nil {
			return fmt.Errorf("public-feed: update outbox attempt: %w", err)
		}
		err := s.sendToMirror(ctx, origin, ingestAuthToken, batch)
		if err == nil {
			if err := s.updateOutboxStatus(ctx, batch.LastSequence, models.PublicFeedOutboxStatusAcknowledged, false); err != nil {
				return fmt.Errorf("public-feed: acknowledge outbox: %w", err)
			}
			return nil
		}
		lastErr = err
		if isMirrorRejection(err) {
			if statusErr := s.updateOutboxStatus(ctx, batch.LastSequence, models.PublicFeedOutboxStatusFailed, false); statusErr != nil {
				return fmt.Errorf("public-feed: reject outbox: %w", statusErr)
			}
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
	if err := s.updateOutboxStatus(ctx, batch.LastSequence, models.PublicFeedOutboxStatusFailed, false); err != nil {
		return fmt.Errorf("public-feed: exhaust outbox retries: %w", err)
	}
	return fmt.Errorf("%w: %v", constants.ErrPublicFeedMaxRetriesExceeded, lastErr)
}

// isMirrorRejection returns true if the error is a mirror rejection (not a
// transport error).
func isMirrorRejection(err error) bool {
	return errors.Is(err, constants.ErrPublicFeedMirrorRejected)
}

// sendToMirror sends a single ingest request to the mirror.
func publicKeyRegistrationSigningBytes(request models.PublicKeyRegistrationRequest) ([]byte, error) {
	request.Signature = ""
	return json.Marshal(request)
}

func publicKeyRevocationSigningBytes(record models.PublicKeyRevocationRecord) ([]byte, error) {
	record.RevocationSignature = ""
	return json.Marshal(record)
}

func (s *PublicPublisherService) sendToMirror(ctx context.Context, origin, ingestAuthToken string, batch models.PublicFeedBatch) error {
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
	if ingestAuthToken != "" {
		req.Header.Set("Authorization", "Bearer "+ingestAuthToken)
	}

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
		if ingestResp.RejectionReason == models.PublicFeedIngestRejectionDuplicateSequence &&
			ingestResp.SourceID == batch.SourceID &&
			ingestResp.HighWaterSequence == batch.LastSequence &&
			ingestResp.FeedChainHash == batch.ContentHash {
			return nil
		}
		return fmt.Errorf("%w: %s", constants.ErrPublicFeedMirrorRejected, ingestResp.RejectionReason)
	}

	return nil
}

// updateOutboxStatus updates the status of an outbox entry.
func (s *PublicPublisherService) updateOutboxStatus(ctx context.Context, seq int64, status models.PublicFeedOutboxStatus, incrementAttempt bool) error {
	entries, err := s.outbox.List(ctx)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if entry.Sequence != seq {
			continue
		}
		entry.Status = status
		now := time.Now().UTC()
		if incrementAttempt {
			entry.Attempts++
			entry.LastAttemptAt = &now
		}
		if status == models.PublicFeedOutboxStatusAcknowledged {
			entry.AcknowledgedAt = &now
		}
		return s.outbox.Update(ctx, entry)
	}
	return constants.ErrPublicFeedOutboxCorrupt
}

// retransmitUnacknowledgedOutbox transmits any outbox tail entries that have
// not yet been mirror-acknowledged. The publisher high-water sequence must
// remain aligned with mirror acceptance, so unacknowledged entries are
// retried before new batches are appended.
func (s *PublicPublisherService) retransmitUnacknowledgedOutbox(ctx context.Context) error {
	entries, err := s.outbox.List(ctx)
	if err != nil {
		return fmt.Errorf("public-feed: retransmit unacknowledged: list outbox: %w", err)
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Sequence < entries[j].Sequence })
	s.mu.Lock()
	tipSequence := s.highWaterSeq
	tipHash := s.feedChainHash
	if tipSequence == 0 && tipHash == "" {
		tipHash = constants.PublicFeedZeroHashHex
	}
	batchCount := s.batchCount
	s.mu.Unlock()
	for _, entry := range entries {
		if entry.Sequence <= tipSequence {
			continue
		}
		if entry.Status == models.PublicFeedOutboxStatusAcknowledged {
			tipSequence = entry.Sequence
			tipHash = entry.BatchHash
			batchCount++
			continue
		}
		expectedSequence := tipSequence + 1
		if entry.Sequence != expectedSequence {
			return fmt.Errorf("%w: sequence %d does not continue acknowledged tip %d", constants.ErrPublicFeedOutboxCorrupt, entry.Sequence, tipSequence)
		}
		var batch models.PublicFeedBatch
		if err := json.Unmarshal([]byte(entry.BatchBytes), &batch); err != nil {
			return fmt.Errorf("%w: decode unacknowledged batch: %v", constants.ErrPublicFeedOutboxCorrupt, err)
		}
		if batch.FirstSequence != expectedSequence || batch.PreviousBatchHash != tipHash {
			return fmt.Errorf("%w: sequence %d hash chain does not match mirror tip", constants.ErrPublicFeedOutboxCorrupt, entry.Sequence)
		}
		if err := s.transmitBatch(ctx, batch); err != nil {
			return err
		}
		tipSequence = batch.LastSequence
		tipHash = batch.ContentHash
		batchCount++
		s.mu.Lock()
		s.highWaterSeq = tipSequence
		s.feedChainHash = tipHash
		s.batchCount = batchCount
		s.mu.Unlock()
		if err := s.persistSnapshot(ctx); err != nil {
			return fmt.Errorf("public-feed: retransmit unacknowledged: persist snapshot: %w", err)
		}
	}
	return nil
}

// RetransmitOutbox retransmits mirror-unacknowledged outbox tail entries.
// Historical batches are not resent because the mirror retains only a bounded
// batch window and rejects duplicate_sequence for pruned prefixes.
func (s *PublicPublisherService) RetransmitOutbox(ctx context.Context) error {
	if err := s.loadSnapshotFromOutbox(ctx); err != nil {
		return fmt.Errorf("public-feed: retransmit: recover: %w", err)
	}
	if err := s.retransmitUnacknowledgedOutbox(ctx); err != nil {
		return fmt.Errorf("public-feed: retransmit: %w", err)
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
		if err := s.loadSnapshotFromOutbox(ctx); err != nil {
			return models.PublicFeedSnapshot{}, fmt.Errorf("public-feed: get snapshot: recover: %w", err)
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
func (s *PublicPublisherService) persistSnapshot(ctx context.Context) error {
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
	return s.fileSvc.WriteFile(ctx, constants.PublicFeedSnapshotPath, snapBytes, constants.PermFilePrivate)
}

// RepairOutboxFromSnapshot compacts a prefix-pruned outbox back to the
// mirror-acknowledged snapshot tip so chain validation can resume.
func (s *PublicPublisherService) RepairOutboxFromSnapshot(ctx context.Context) error {
	var snapshot models.PublicFeedSnapshot
	exists, err := s.fileSvc.FileExists(ctx, constants.PublicFeedSnapshotPath)
	if err != nil {
		return fmt.Errorf("public-feed: repair outbox: stat snapshot: %w", err)
	}
	if !exists {
		return constants.ErrPublicFeedSnapshotNotFound
	}
	data, err := s.fileSvc.ReadFile(ctx, constants.PublicFeedSnapshotPath)
	if err != nil {
		return fmt.Errorf("public-feed: repair outbox: read snapshot: %w", err)
	}
	if err := json.Unmarshal(data, &snapshot); err != nil {
		return fmt.Errorf("%w: decode snapshot: %v", constants.ErrPublicFeedOutboxCorrupt, err)
	}
	if snapshot.SourceID != s.cfg.SourceID || snapshot.HighWaterSequence <= 0 || snapshot.FeedChainHash == "" {
		return fmt.Errorf("%w: invalid snapshot", constants.ErrPublicFeedOutboxCorrupt)
	}

	rawStore, ok := s.outbox.(*runtimePublicOutboxStore)
	if !ok {
		return constants.ErrPublicFeedOutboxCorrupt
	}
	entries, err := rawStore.readRaw(ctx)
	if err != nil {
		return fmt.Errorf("public-feed: repair outbox: %w", err)
	}
	retained := make([]models.PublicOutboxEntry, 0, len(entries))
	tipSequence := snapshot.HighWaterSequence
	tipHash := snapshot.FeedChainHash
	for _, entry := range entries {
		if entry.Sequence <= tipSequence {
			continue
		}
		if entry.Sequence != tipSequence+1 {
			break
		}
		var batch models.PublicFeedBatch
		if err := json.Unmarshal([]byte(entry.BatchBytes), &batch); err != nil {
			break
		}
		if batch.FirstSequence != tipSequence+1 || batch.PreviousBatchHash != tipHash {
			break
		}
		retained = append(retained, entry)
		tipSequence = entry.Sequence
		tipHash = entry.BatchHash
	}
	if len(retained) == len(entries) {
		return nil
	}
	if err := s.outbox.Replace(ctx, retained); err != nil {
		return fmt.Errorf("public-feed: repair outbox: %w", err)
	}
	return nil
}

// loadSnapshotFromOutbox reconstructs the high-water sequence and feed-chain
// hash from the runtime snapshot and durable file outbox after process restart.
func (s *PublicPublisherService) loadSnapshotFromOutbox(ctx context.Context) error {
	var snapshot models.PublicFeedSnapshot
	exists, err := s.fileSvc.FileExists(ctx, constants.PublicFeedSnapshotPath)
	if err != nil {
		return fmt.Errorf("load snapshot from outbox: stat snapshot: %w", err)
	}
	if exists {
		data, err := s.fileSvc.ReadFile(ctx, constants.PublicFeedSnapshotPath)
		if err != nil {
			return fmt.Errorf("load snapshot from outbox: read snapshot: %w", err)
		}
		if err := json.Unmarshal(data, &snapshot); err != nil {
			return fmt.Errorf("%w: decode snapshot: %v", constants.ErrPublicFeedOutboxCorrupt, err)
		}
		if snapshot.SourceID != s.cfg.SourceID || snapshot.HighWaterSequence <= 0 || snapshot.FeedChainHash == "" {
			return fmt.Errorf("%w: invalid snapshot", constants.ErrPublicFeedOutboxCorrupt)
		}
	}

	entries, err := s.outbox.List(ctx)
	if err != nil {
		if errors.Is(err, constants.ErrPublicFeedHashChainMismatch) {
			if repairErr := s.RepairOutboxFromSnapshot(ctx); repairErr != nil {
				return fmt.Errorf("load snapshot from outbox: list: %w", err)
			}
			entries, err = s.outbox.List(ctx)
		}
		if err != nil {
			return fmt.Errorf("load snapshot from outbox: list: %w", err)
		}
	}
	maxSeq := snapshot.HighWaterSequence
	lastHash := snapshot.FeedChainHash
	if maxSeq == 0 && lastHash == "" {
		lastHash = constants.PublicFeedZeroHashHex
	}
	count := snapshot.BatchCount
	if maxSeq == 0 && len(entries) > 0 {
		var firstBatch models.PublicFeedBatch
		if err := json.Unmarshal([]byte(entries[0].BatchBytes), &firstBatch); err != nil {
			return fmt.Errorf("%w: decode first batch: %v", constants.ErrPublicFeedOutboxCorrupt, err)
		}
		if firstBatch.FirstSequence != 1 || firstBatch.PreviousBatchHash != constants.PublicFeedZeroHashHex {
			return constants.ErrPublicFeedOutboxCorrupt
		}
	}
	for _, entry := range entries {
		if entry.Sequence < maxSeq {
			continue
		}
		if entry.Sequence == maxSeq {
			if entry.BatchHash != lastHash {
				return constants.ErrPublicFeedOutboxEquivocation
			}
			continue
		}
		if entry.Status != models.PublicFeedOutboxStatusAcknowledged {
			continue
		}
		var batch models.PublicFeedBatch
		if err := json.Unmarshal([]byte(entry.BatchBytes), &batch); err != nil {
			return fmt.Errorf("%w: decode recovery batch: %v", constants.ErrPublicFeedOutboxCorrupt, err)
		}
		if batch.FirstSequence != maxSeq+1 || batch.PreviousBatchHash != lastHash {
			return constants.ErrPublicFeedOutboxCorrupt
		}
		maxSeq = entry.Sequence
		lastHash = entry.BatchHash
		count++
	}

	s.mu.Lock()
	s.highWaterSeq = maxSeq
	s.feedChainHash = lastHash
	s.batchCount = count
	s.mu.Unlock()
	return nil
}

func (s *PublicPublisherService) registerRotatedKey(ctx context.Context, newKeyID string, newPublicKey ed25519.PublicKey) error {
	s.mu.Lock()
	origin := s.mirrorOrigin
	token := s.ingestAuthToken
	currentKeyID := s.signingKeyID
	currentPrivateKey := append(ed25519.PrivateKey(nil), s.signingPrivKey...)
	s.mu.Unlock()
	if origin == "" {
		return constants.ErrPublicFeedMirrorOriginRequired
	}
	request := models.PublicKeyRegistrationRequest{
		SourceID:     s.cfg.SourceID,
		CurrentKeyID: currentKeyID,
		NewKeyID:     newKeyID,
		PublicKey:    hex.EncodeToString(newPublicKey),
	}
	signingBytes, err := publicKeyRegistrationSigningBytes(request)
	if err != nil {
		return fmt.Errorf("%w: encode request: %v", constants.ErrPublicFeedKeyRegistration, err)
	}
	request.Signature = hex.EncodeToString(ed25519.Sign(currentPrivateKey, signingBytes))
	body, err := json.Marshal(request)
	if err != nil {
		return fmt.Errorf("%w: encode request: %v", constants.ErrPublicFeedKeyRegistration, err)
	}
	httpRequest, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(origin, "/")+"/keys/register", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("%w: %v", constants.ErrPublicFeedMirrorUnreachable, err)
	}
	httpRequest.Header.Set("Content-Type", "application/json")
	if token != "" {
		httpRequest.Header.Set("Authorization", "Bearer "+token)
	}
	response, err := (&http.Client{Timeout: 30 * time.Second}).Do(httpRequest)
	if err != nil {
		return fmt.Errorf("%w: %v", constants.ErrPublicFeedMirrorUnreachable, err)
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode >= 500 {
		return fmt.Errorf("%w: status %d", constants.ErrPublicFeedMirrorUnreachable, response.StatusCode)
	}
	var registrationResponse models.PublicKeyRegistrationResponse
	if err := json.NewDecoder(response.Body).Decode(&registrationResponse); err != nil {
		return fmt.Errorf("%w: decode response: %v", constants.ErrPublicFeedKeyRegistration, err)
	}
	if !registrationResponse.Accepted {
		return constants.ErrPublicFeedKeyRegistration
	}
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
	keyDigest := sha256.Sum256(newPub)
	newKeyID := hex.EncodeToString(keyDigest[:])
	if err := s.RotateKeyTo(ctx, newPriv, newKeyID); err != nil {
		return "", "", err
	}
	return newKeyID, hex.EncodeToString(newPub), nil
}

func (s *PublicPublisherService) RotateKeyTo(ctx context.Context, newPriv ed25519.PrivateKey, newKeyID string) error {
	if len(newPriv) != ed25519.PrivateKeySize {
		return constants.ErrPublicFeedSigningKeyRequired
	}
	if newKeyID == "" {
		return constants.ErrPublicFeedSigningKeyIDRequired
	}
	if err := s.loadSnapshotFromOutbox(ctx); err != nil {
		return fmt.Errorf("public-feed: rotate key: recover outbox: %w", err)
	}
	newPub := newPriv.Public().(ed25519.PublicKey)
	if err := s.registerRotatedKey(ctx, newKeyID, newPub); err != nil {
		return fmt.Errorf("public-feed: rotate key: register new key: %w", err)
	}
	revRecord := models.PublicKeyRevocationRecord{
		SourceID:     s.cfg.SourceID,
		RevokedKeyID: s.signingKeyID,
		RevokedAt:    time.Now().UTC(),
		NewKeyID:     newKeyID,
	}
	revocationBytes, err := publicKeyRevocationSigningBytes(revRecord)
	if err != nil {
		return fmt.Errorf("public-feed: rotate key: encode revocation signature payload: %w", err)
	}
	revRecord.RevocationSignature = hex.EncodeToString(ed25519.Sign(s.signingPrivKey, revocationBytes))
	revBytes, err := json.Marshal(revRecord)
	if err != nil {
		return fmt.Errorf("marshal revocation record: %w", err)
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
	if err := s.ExportBatch(ctx, []models.PublicFeedRecord{record}); err != nil {
		return fmt.Errorf("public-feed: rotate key: export revocation: %w", err)
	}

	s.mu.Lock()
	s.signingPrivKey = newPriv
	s.signingPubKey = newPub
	s.signingKeyID = newKeyID
	s.mu.Unlock()
	return nil
}

func (s *PublicPublisherService) PushProofPackage(ctx context.Context) error {
	manifest, catalog, err := s.loadProofPackage(ctx)
	if err != nil {
		return err
	}
	if manifest == nil {
		return nil
	}
	syncState, err := s.loadProofMirrorSyncState(ctx)
	if err != nil {
		return err
	}
	if syncState != nil && syncState.ProofRootSHA256 == manifest.ProofRootSHA256 {
		return nil
	}
	synced := proofMirrorSyncedArtifactSet(syncState)
	pendingIDs := make([]string, 0, len(catalog.Entries))
	for _, entry := range catalog.Entries {
		if _, ok := synced[entry.ArtifactID]; !ok {
			pendingIDs = append(pendingIDs, entry.ArtifactID)
		}
	}
	if len(pendingIDs) == 0 {
		pendingIDs = catalogArtifactIDs(catalog)
	}
	if err := s.postProofIngest(ctx, manifest, catalog, pendingIDs); err != nil {
		if len(pendingIDs) != len(catalog.Entries) {
			if err := s.clearProofMirrorSyncState(ctx); err != nil {
				return err
			}
			return s.postProofIngest(ctx, manifest, catalog, catalogArtifactIDs(catalog))
		}
		return err
	}
	return s.saveProofMirrorSyncState(ctx, models.PublicProofMirrorSyncState{
		SchemaVersion:     constants.PublicProofMirrorSyncSchemaVersion,
		SourceID:          s.cfg.SourceID,
		ProofRootSHA256:   manifest.ProofRootSHA256,
		SyncedArtifactIDs: catalogArtifactIDs(catalog),
		SyncedAt:          time.Now().UTC(),
	})
}

func (s *PublicPublisherService) loadProofPackage(ctx context.Context) (*models.PublicProofManifest, models.PublicProofCatalog, error) {
	manifestExists, err := s.fileSvc.FileExists(ctx, constants.PublicProofManifestFilename)
	if err != nil {
		return nil, models.PublicProofCatalog{}, fmt.Errorf("public-feed: inspect proof manifest: %w", err)
	}
	catalogExists, err := s.fileSvc.FileExists(ctx, constants.PublicProofCatalogFilename)
	if err != nil {
		return nil, models.PublicProofCatalog{}, fmt.Errorf("public-feed: inspect proof catalog: %w", err)
	}
	if !manifestExists && !catalogExists {
		return nil, models.PublicProofCatalog{}, nil
	}
	if !manifestExists || !catalogExists {
		return nil, models.PublicProofCatalog{}, constants.ErrPublicFeedProofManifestInvalid
	}
	manifestBytes, err := s.fileSvc.ReadFile(ctx, constants.PublicProofManifestFilename)
	if err != nil {
		return nil, models.PublicProofCatalog{}, fmt.Errorf("public-feed: read proof manifest: %w", err)
	}
	manifestDecoder := json.NewDecoder(bytes.NewReader(manifestBytes))
	manifestDecoder.DisallowUnknownFields()
	var manifest models.PublicProofManifest
	if err := manifestDecoder.Decode(&manifest); err != nil {
		return nil, models.PublicProofCatalog{}, fmt.Errorf("%w: decode manifest: %v", constants.ErrPublicFeedProofManifestInvalid, err)
	}
	var trailing json.RawMessage
	if err := manifestDecoder.Decode(&trailing); err != io.EOF {
		return nil, models.PublicProofCatalog{}, fmt.Errorf("%w: trailing manifest JSON", constants.ErrPublicFeedProofManifestInvalid)
	}
	catalogBytes, err := s.fileSvc.ReadFile(ctx, constants.PublicProofCatalogFilename)
	if err != nil {
		return nil, models.PublicProofCatalog{}, fmt.Errorf("public-feed: read proof catalog: %w", err)
	}
	catalogDecoder := json.NewDecoder(bytes.NewReader(catalogBytes))
	catalogDecoder.DisallowUnknownFields()
	var catalog models.PublicProofCatalog
	if err := catalogDecoder.Decode(&catalog); err != nil {
		return nil, models.PublicProofCatalog{}, fmt.Errorf("%w: decode catalog: %v", constants.ErrPublicFeedProofCatalogMismatch, err)
	}
	if err := catalogDecoder.Decode(&trailing); err != io.EOF {
		return nil, models.PublicProofCatalog{}, fmt.Errorf("%w: trailing catalog JSON", constants.ErrPublicFeedProofCatalogMismatch)
	}
	return &manifest, catalog, nil
}

func (s *PublicPublisherService) postProofIngest(ctx context.Context, manifest *models.PublicProofManifest, catalog models.PublicProofCatalog, artifactIDs []string) error {
	artifacts, err := s.readProofArtifacts(ctx, artifactIDs)
	if err != nil {
		return err
	}
	request := models.PublicProofIngestRequest{SourceID: s.cfg.SourceID, Manifest: *manifest, Catalog: catalog, Artifacts: artifacts}
	body, err := json.Marshal(request)
	if err != nil {
		return fmt.Errorf("public-feed: encode proof ingest: %w", err)
	}
	s.mu.Lock()
	origin := s.mirrorOrigin
	token := s.ingestAuthToken
	s.mu.Unlock()
	if origin == "" {
		return constants.ErrPublicFeedMirrorOriginRequired
	}
	s.logger.Info("public-feed: pushing proof package to mirror", "artifact_count", len(artifacts), "catalog_count", len(catalog.Entries))
	httpRequest, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(origin, "/")+"/proof-ingest", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("%w: %v", constants.ErrPublicFeedMirrorUnreachable, err)
	}
	httpRequest.Header.Set("Content-Type", "application/json")
	if token != "" {
		httpRequest.Header.Set("Authorization", "Bearer "+token)
	}
	response, err := (&http.Client{Timeout: 10 * time.Minute}).Do(httpRequest)
	if err != nil {
		return fmt.Errorf("%w: %v", constants.ErrPublicFeedMirrorUnreachable, err)
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode >= 500 {
		return fmt.Errorf("%w: status %d", constants.ErrPublicFeedMirrorUnreachable, response.StatusCode)
	}
	var ingestResponse models.PublicProofIngestResponse
	if err := json.NewDecoder(response.Body).Decode(&ingestResponse); err != nil {
		return fmt.Errorf("%w: decode response: %v", constants.ErrPublicFeedProofIngestRejected, err)
	}
	if !ingestResponse.Accepted {
		return constants.ErrPublicFeedProofIngestRejected
	}
	return nil
}

func (s *PublicPublisherService) readProofArtifacts(ctx context.Context, artifactIDs []string) ([]models.PublicProofIngestArtifact, error) {
	artifacts := make([]models.PublicProofIngestArtifact, 0, len(artifactIDs))
	for _, artifactID := range artifactIDs {
		if artifactID == "" {
			return nil, constants.ErrPublicFeedProofPathTraversal
		}
		relPath := filepath.Join(constants.PublicProofsDirname, artifactID)
		info, err := s.fileSvc.Lstat(ctx, relPath)
		if err != nil {
			return nil, fmt.Errorf("public-feed: inspect proof artifact: %w", err)
		}
		if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
			return nil, constants.ErrPublicFeedProofSymlinkRejected
		}
		content, err := s.fileSvc.ReadFile(ctx, relPath)
		if err != nil {
			return nil, fmt.Errorf("public-feed: read proof artifact: %w", err)
		}
		artifacts = append(artifacts, models.PublicProofIngestArtifact{ArtifactID: artifactID, Content: content})
	}
	return artifacts, nil
}

func (s *PublicPublisherService) loadProofMirrorSyncState(ctx context.Context) (*models.PublicProofMirrorSyncState, error) {
	exists, err := s.fileSvc.FileExists(ctx, constants.PublicProofMirrorSyncFilename)
	if err != nil {
		return nil, fmt.Errorf("public-feed: inspect proof mirror sync state: %w", err)
	}
	if !exists {
		return nil, nil
	}
	data, err := s.fileSvc.ReadFile(ctx, constants.PublicProofMirrorSyncFilename)
	if err != nil {
		return nil, fmt.Errorf("public-feed: read proof mirror sync state: %w", err)
	}
	var state models.PublicProofMirrorSyncState
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, fmt.Errorf("public-feed: decode proof mirror sync state: %w", err)
	}
	if state.SourceID != "" && state.SourceID != s.cfg.SourceID {
		return nil, nil
	}
	return &state, nil
}

func (s *PublicPublisherService) saveProofMirrorSyncState(ctx context.Context, state models.PublicProofMirrorSyncState) error {
	data, err := json.Marshal(state)
	if err != nil {
		return fmt.Errorf("public-feed: marshal proof mirror sync state: %w", err)
	}
	return s.fileSvc.WriteFile(ctx, constants.PublicProofMirrorSyncFilename, data, constants.PermFilePrivate)
}

func (s *PublicPublisherService) clearProofMirrorSyncState(ctx context.Context) error {
	exists, err := s.fileSvc.FileExists(ctx, constants.PublicProofMirrorSyncFilename)
	if err != nil {
		return fmt.Errorf("public-feed: inspect proof mirror sync state: %w", err)
	}
	if !exists {
		return nil
	}
	return s.fileSvc.Remove(ctx, constants.PublicProofMirrorSyncFilename)
}

func catalogArtifactIDs(catalog models.PublicProofCatalog) []string {
	ids := make([]string, 0, len(catalog.Entries))
	for _, entry := range catalog.Entries {
		ids = append(ids, entry.ArtifactID)
	}
	return ids
}

func proofMirrorSyncedArtifactSet(state *models.PublicProofMirrorSyncState) map[string]struct{} {
	synced := make(map[string]struct{})
	if state == nil {
		return synced
	}
	for _, artifactID := range state.SyncedArtifactIDs {
		if artifactID != "" {
			synced[artifactID] = struct{}{}
		}
	}
	return synced
}

func (s *PublicPublisherService) writeProofArtifacts(ctx context.Context, campaignID string, artifacts []ProofArtifactInput) ([]models.PublicProofCatalogEntry, error) {
	if len(artifacts) == 0 {
		return nil, constants.ErrPublicFeedBatchEmpty
	}
	if err := checkNoSymlinks(ctx, s.fileSvc, constants.PublicProofsDirname); err != nil {
		return nil, err
	}
	if err := s.fileSvc.MkdirAll(ctx, constants.PublicProofsDirname, constants.PermDirPrivate); err != nil {
		return nil, fmt.Errorf("public-feed: build proof: mkdir: %w", err)
	}
	entries := make([]models.PublicProofCatalogEntry, 0, len(artifacts))
	for _, art := range artifacts {
		if !safePublicProofFilename(art.Filename) {
			return nil, constants.ErrPublicFeedProofPathTraversal
		}
		if int64(len(art.Content)) > int64(constants.PublicFeedMaxArtifactBytes) {
			return nil, constants.ErrPublicFeedProofOversized
		}
		if strings.Contains(art.MediaType, "json") {
			if err := checkProhibitedFields(string(art.Content)); err != nil {
				return nil, fmt.Errorf("%w: %w", constants.ErrPublicFeedProofRestricted, err)
			}
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
			VerificationCommand: art.VerificationCommand,
			ImmutableURL:        fmt.Sprintf("/proofs/%s", artifactID),
		}
		if entry.VerificationCommand == "" {
			entry.VerificationCommand = fmt.Sprintf("sha256sum %s", art.Filename)
		}
		relPath := filepath.Join(constants.PublicProofsDirname, artifactID)
		if err := s.fileSvc.WriteFile(ctx, relPath, art.Content, constants.PermFilePrivate); err != nil {
			return nil, fmt.Errorf("public-feed: build proof: write artifact: %w", err)
		}
		entries = append(entries, entry)
	}
	return entries, nil
}

func (s *PublicPublisherService) finalizeProofManifest(ctx context.Context, campaignID, campaignRevision, verifiedIndexGenHash string, verificationOK bool, newEntries []models.PublicProofCatalogEntry) (models.PublicProofManifest, error) {
	if !verificationOK {
		return models.PublicProofManifest{}, constants.ErrPublicFeedProofNotVerified
	}
	catalog, err := s.GetProofCatalog(ctx)
	if err != nil {
		return models.PublicProofManifest{}, err
	}
	manifestEntries := append([]models.PublicProofCatalogEntry(nil), catalog.Entries...)
	existingArtifactIDs := make(map[string]struct{}, len(manifestEntries))
	for _, entry := range manifestEntries {
		existingArtifactIDs[entry.ArtifactID] = struct{}{}
	}
	for _, entry := range newEntries {
		if _, exists := existingArtifactIDs[entry.ArtifactID]; exists {
			continue
		}
		manifestEntries = append(manifestEntries, entry)
		existingArtifactIDs[entry.ArtifactID] = struct{}{}
	}
	if len(manifestEntries) > constants.PublicFeedProofMaxArtifacts {
		return models.PublicProofManifest{}, constants.ErrPublicFeedProofCatalogMismatch
	}
	manifest := models.PublicProofManifest{
		SchemaVersion:               constants.PublicProofManifestSchemaVersion,
		CampaignID:                  campaignID,
		CampaignRevision:            campaignRevision,
		VerifiedIndexGenerationHash: verifiedIndexGenHash,
		VerificationOK:              verificationOK,
		ArtifactCount:               len(manifestEntries),
		Artifacts:                   manifestEntries,
		VerifierInstructions:        "Verify each artifact SHA-256 matches the catalog entry. Recompute the proof root hash from artifact hashes and manifest metadata. Verify the Ed25519 signature over the proof root hash.",
		GeneratedAt:                 time.Now().UTC(),
		SigningKeyID:                s.signingKeyID,
	}
	rootHash := s.computeProofRootHash(manifest)
	manifest.ProofRootSHA256 = rootHash
	rootHashBytes, err := hex.DecodeString(rootHash)
	if err != nil {
		return models.PublicProofManifest{}, fmt.Errorf("public-feed: build proof: decode root hash: %w", err)
	}
	sig := ed25519.Sign(s.signingPrivKey, rootHashBytes)
	manifest.Signature = hex.EncodeToString(sig)
	manifestBytes, err := json.Marshal(manifest)
	if err != nil {
		return models.PublicProofManifest{}, fmt.Errorf("public-feed: build proof: marshal manifest: %w", err)
	}
	if err := s.fileSvc.WriteFile(ctx, constants.PublicProofManifestFilename, manifestBytes, constants.PermFilePrivate); err != nil {
		return models.PublicProofManifest{}, fmt.Errorf("public-feed: build proof: write manifest: %w", err)
	}
	if err := s.updateProofCatalog(ctx, newEntries); err != nil {
		s.logger.Warn("public-feed: failed to update proof catalog", "error", err)
	}
	return manifest, nil
}

// BuildProofPackage creates a complete public proof package from a passing
// verification report. It writes artifacts to disk under the public-proofs
// directory, builds a signed root manifest, and updates the proof catalog.
func (s *PublicPublisherService) BuildProofPackage(ctx context.Context, campaignID, campaignRevision, verifiedIndexGenHash string, verificationOK bool, artifacts []ProofArtifactInput) (models.PublicProofManifest, error) {
	if !verificationOK {
		return models.PublicProofManifest{}, constants.ErrPublicFeedProofNotVerified
	}
	entries, err := s.writeProofArtifacts(ctx, campaignID, artifacts)
	if err != nil {
		return models.PublicProofManifest{}, err
	}
	return s.finalizeProofManifest(ctx, campaignID, campaignRevision, verifiedIndexGenHash, verificationOK, entries)
}

func assignmentAuditProofArtifacts(request models.PublicAssignmentAuditProofPublishRequest) ([]ProofArtifactInput, string, string, string, error) {
	if request.CampaignID == "" || request.RunID == "" || request.AssignmentID == "" || len(request.Database) == 0 || len(request.VaultKey) == 0 {
		return nil, "", "", "", fmt.Errorf("public-feed: publish assignment audit proof: %w", constants.ErrMissingRequiredField)
	}
	revision := request.CampaignRevision
	if revision == "" {
		revision = request.CampaignID
	}
	indexDigest := request.IndexDigest
	if indexDigest == "" {
		indexDigest = request.AssignmentID
	}
	dbFilename := fmt.Sprintf("assignment-%s.db", request.AssignmentID)
	keyFilename := fmt.Sprintf("assignment-%s.vault.key", request.AssignmentID)
	verifyCommand := fmt.Sprintf("g8e public verify-assignment --db %s --vault-key %s", dbFilename, keyFilename)
	return []ProofArtifactInput{
		{
			Filename:            dbFilename,
			MediaType:           "application/vnd.sqlite3",
			Content:             request.Database,
			CampaignID:          request.CampaignID,
			SourceRunID:         request.RunID,
			VerificationCommand: verifyCommand,
		},
		{
			Filename:            keyFilename,
			MediaType:           "text/plain",
			Content:             request.VaultKey,
			CampaignID:          request.CampaignID,
			SourceRunID:         request.RunID,
			VerificationCommand: verifyCommand,
		},
	}, request.CampaignID, revision, indexDigest, nil
}

// PublishAssignmentAuditProof ingests one assignment audit export package into
// the public proof catalog and mirror.
func (s *PublicPublisherService) PublishAssignmentAuditProof(ctx context.Context, request models.PublicAssignmentAuditProofPublishRequest) (models.PublicProofManifest, error) {
	return s.PublishAssignmentAuditProofBatch(ctx, models.PublicAssignmentAuditProofBatchPublishRequest{
		Proofs:          []models.PublicAssignmentAuditProofPublishRequest{request},
		DeferMirrorPush: request.DeferMirrorPush,
	})
}

// PublishAssignmentAuditProofBatch ingests many assignment audit export
// packages in one catalog update and one signed manifest rebuild.
func (s *PublicPublisherService) PublishAssignmentAuditProofBatch(ctx context.Context, request models.PublicAssignmentAuditProofBatchPublishRequest) (models.PublicProofManifest, error) {
	if len(request.Proofs) == 0 {
		return models.PublicProofManifest{}, nil
	}
	artifacts := make([]ProofArtifactInput, 0, len(request.Proofs)*2)
	campaignID := ""
	campaignRevision := ""
	verifiedIndexGenHash := ""
	for _, proof := range request.Proofs {
		proofArtifacts, proofCampaignID, revision, indexDigest, err := assignmentAuditProofArtifacts(proof)
		if err != nil {
			return models.PublicProofManifest{}, err
		}
		if campaignID == "" {
			campaignID = proofCampaignID
			campaignRevision = revision
		}
		verifiedIndexGenHash = indexDigest
		artifacts = append(artifacts, proofArtifacts...)
	}
	entries, err := s.writeProofArtifacts(ctx, campaignID, artifacts)
	if err != nil {
		return models.PublicProofManifest{}, err
	}
	manifest, err := s.finalizeProofManifest(ctx, campaignID, campaignRevision, verifiedIndexGenHash, true, entries)
	if err != nil {
		return models.PublicProofManifest{}, err
	}
	if !request.DeferMirrorPush {
		if err := s.PushProofPackage(ctx); err != nil {
			return models.PublicProofManifest{}, err
		}
	}
	return manifest, nil
}

// PruneProofCatalogForRun removes prior proof artifacts for one run so force
// restore can republish without growing the catalog or mirror payload.
func (s *PublicPublisherService) PruneProofCatalogForRun(ctx context.Context, runID string) (int, error) {
	if runID == "" {
		return 0, constants.ErrMissingRequiredField
	}
	catalog, err := s.GetProofCatalog(ctx)
	if err != nil {
		return 0, err
	}
	if len(catalog.Entries) == 0 {
		return 0, nil
	}
	kept := make([]models.PublicProofCatalogEntry, 0, len(catalog.Entries))
	removedIDs := make([]string, 0)
	for _, entry := range catalog.Entries {
		if entry.SourceRunID == runID {
			removedIDs = append(removedIDs, entry.ArtifactID)
			continue
		}
		kept = append(kept, entry)
	}
	if len(removedIDs) == 0 {
		return 0, nil
	}
	for _, artifactID := range removedIDs {
		if err := s.fileSvc.Remove(ctx, filepath.Join(constants.PublicProofsDirname, artifactID)); err != nil {
			return 0, fmt.Errorf("public-feed: prune proof artifact: %w", err)
		}
	}
	catalog.Entries = kept
	catalog.GeneratedAt = time.Now().UTC()
	catalogBytes, err := json.Marshal(catalog)
	if err != nil {
		return 0, fmt.Errorf("public-feed: prune proof catalog: %w", err)
	}
	if err := s.fileSvc.WriteFile(ctx, constants.PublicProofCatalogFilename, catalogBytes, constants.PermFilePrivate); err != nil {
		return 0, fmt.Errorf("public-feed: prune proof catalog: %w", err)
	}
	if len(kept) == 0 {
		_ = s.fileSvc.Remove(ctx, constants.PublicProofManifestFilename)
	} else {
		campaignID := kept[0].CampaignID
		revision := campaignID
		indexDigest := kept[0].SourceRunID
		if manifest, _, err := s.loadProofPackage(ctx); err == nil && manifest != nil {
			if manifest.CampaignID != "" {
				campaignID = manifest.CampaignID
			}
			if manifest.CampaignRevision != "" {
				revision = manifest.CampaignRevision
			}
			if manifest.VerifiedIndexGenerationHash != "" {
				indexDigest = manifest.VerifiedIndexGenerationHash
			}
		}
		if _, err := s.finalizeProofManifest(ctx, campaignID, revision, indexDigest, true, nil); err != nil {
			return 0, err
		}
	}
	if err := s.removeProofMirrorSyncArtifacts(ctx, removedIDs); err != nil {
		return 0, err
	}
	s.logger.Info("public-feed: pruned proof catalog for run", "run_id", runID, "removed_count", len(removedIDs), "remaining_count", len(kept))
	return len(removedIDs), nil
}

func (s *PublicPublisherService) removeProofMirrorSyncArtifacts(ctx context.Context, artifactIDs []string) error {
	if len(artifactIDs) == 0 {
		return nil
	}
	syncState, err := s.loadProofMirrorSyncState(ctx)
	if err != nil || syncState == nil {
		return err
	}
	remove := make(map[string]struct{}, len(artifactIDs))
	for _, artifactID := range artifactIDs {
		remove[artifactID] = struct{}{}
	}
	remaining := make([]string, 0, len(syncState.SyncedArtifactIDs))
	for _, artifactID := range syncState.SyncedArtifactIDs {
		if _, drop := remove[artifactID]; !drop {
			remaining = append(remaining, artifactID)
		}
	}
	syncState.SyncedArtifactIDs = remaining
	syncState.SyncedAt = time.Now().UTC()
	syncState.ProofRootSHA256 = ""
	return s.saveProofMirrorSyncState(ctx, *syncState)
}

// FlushProofPackage pushes the current proof catalog and manifest to the mirror.
func (s *PublicPublisherService) FlushProofPackage(ctx context.Context) (models.PublicProofManifest, error) {
	manifestExists, err := s.fileSvc.FileExists(ctx, constants.PublicProofManifestFilename)
	if err != nil {
		return models.PublicProofManifest{}, fmt.Errorf("public-feed: inspect proof manifest: %w", err)
	}
	if !manifestExists {
		return models.PublicProofManifest{}, nil
	}
	if err := s.PushProofPackage(ctx); err != nil {
		return models.PublicProofManifest{}, err
	}
	manifestBytes, err := s.fileSvc.ReadFile(ctx, constants.PublicProofManifestFilename)
	if err != nil {
		return models.PublicProofManifest{}, fmt.Errorf("public-feed: read proof manifest: %w", err)
	}
	var manifest models.PublicProofManifest
	if err := json.Unmarshal(manifestBytes, &manifest); err != nil {
		return models.PublicProofManifest{}, fmt.Errorf("public-feed: read proof manifest: %w", err)
	}
	return manifest, nil
}

// computeProofRootHash computes the SHA-256 proof root hash over the
// artifact hashes and manifest metadata (excluding the signature).
func (s *PublicPublisherService) computeProofRootHash(manifest models.PublicProofManifest) string {
	return computePublicProofRootHash(manifest)
}

func computePublicProofRootHash(manifest models.PublicProofManifest) string {
	h := sha256.New()
	h.Write([]byte(manifest.SchemaVersion))
	h.Write([]byte(manifest.CampaignID))
	h.Write([]byte(manifest.CampaignRevision))
	h.Write([]byte(manifest.VerifiedIndexGenerationHash))
	_, _ = fmt.Fprintf(h, "%t", manifest.VerificationOK)
	_, _ = fmt.Fprintf(h, "%d", manifest.ArtifactCount)
	for _, entry := range manifest.Artifacts {
		h.Write([]byte(entry.ArtifactID))
		h.Write([]byte(entry.SHA256))
		h.Write([]byte(entry.Filename))
		h.Write([]byte(entry.MediaType))
		_, _ = fmt.Fprintf(h, "%d", entry.ByteSize)
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

	// Check if the file is a symlink.
	fi, err := s.fileSvc.Lstat(ctx, relPath)
	if err != nil {
		return constants.ErrPublicFeedProofNotFound
	}
	if fi.Mode()&os.ModeSymlink != 0 {
		return constants.ErrPublicFeedProofSymlinkRejected
	}

	// Read the file.
	data, err := s.fileSvc.ReadFile(ctx, relPath)
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
func checkNoSymlinks(ctx context.Context, fileSvc fs.RuntimeFileService, relDir string) error {
	info, err := fileSvc.Lstat(ctx, relDir)
	if err != nil {
		if errors.Is(err, constants.ErrNotFound) {
			return nil
		}
		return fmt.Errorf("public-feed: inspect proof path: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return constants.ErrPublicFeedProofSymlinkRejected
	}
	if !info.IsDir() {
		return nil
	}
	entries, err := fileSvc.ReadDir(ctx, relDir)
	if err != nil {
		return fmt.Errorf("public-feed: read proof directory: %w", err)
	}
	for _, entry := range entries {
		childPath := filepath.Join(relDir, entry.Name())
		if err := checkNoSymlinks(ctx, fileSvc, childPath); err != nil {
			return err
		}
	}
	return nil
}
