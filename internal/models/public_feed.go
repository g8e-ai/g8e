// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 0.

package models

import (
	"time"

	"github.com/g8e-ai/g8e/v2/internal/constants"
)

// Public feed record type constants. These mirror the closed vocabulary in
// protocol/models/public_feed.json.
type PublicFeedRecordType string

const (
	PublicFeedRecordTypeProjection    PublicFeedRecordType = "projection"
	PublicFeedRecordTypeEvent         PublicFeedRecordType = "event"
	PublicFeedRecordTypeProofManifest PublicFeedRecordType = "proof_manifest"
	PublicFeedRecordTypeKeyRevocation PublicFeedRecordType = "key_revocation"
)

// PublicFeedOutboxStatus is the lifecycle state of an outbox entry.
type PublicFeedOutboxStatus string

const (
	PublicFeedOutboxStatusPending      PublicFeedOutboxStatus = "pending"
	PublicFeedOutboxStatusSent         PublicFeedOutboxStatus = "sent"
	PublicFeedOutboxStatusAcknowledged PublicFeedOutboxStatus = "acknowledged"
	PublicFeedOutboxStatusFailed       PublicFeedOutboxStatus = "failed"
)

// PublicFeedIngestRejectionReason is the typed rejection reason from the
// mirror when a batch is rejected.
type PublicFeedIngestRejectionReason string

const (
	PublicFeedIngestRejectionSignatureInvalid   PublicFeedIngestRejectionReason = "signature_invalid"
	PublicFeedIngestRejectionSequenceOutOfOrder PublicFeedIngestRejectionReason = "sequence_out_of_order"
	PublicFeedIngestRejectionHashChainMismatch  PublicFeedIngestRejectionReason = "hash_chain_mismatch"
	PublicFeedIngestRejectionDuplicateSequence  PublicFeedIngestRejectionReason = "duplicate_sequence"
	PublicFeedIngestRejectionOversizedBatch     PublicFeedIngestRejectionReason = "oversized_batch"
	PublicFeedIngestRejectionRevokedKey         PublicFeedIngestRejectionReason = "revoked_key"
	PublicFeedIngestRejectionUnknownKey         PublicFeedIngestRejectionReason = "unknown_key"
)

// PublicFeedProofClassification is the disclosure classification for a proof
// artifact. Only public_safe artifacts appear in the public proof catalog.
type PublicFeedProofClassification string

const (
	PublicFeedProofClassificationPublicSafe PublicFeedProofClassification = "public_safe"
)

// PublicFeedRecord is one record in a signed batch. A record is a serialized
// projection or event payload with its SHA-256 content hash.
type PublicFeedRecord struct {
	Sequence    int64                `json:"sequence"`
	RecordType  PublicFeedRecordType `json:"record_type"`
	RecordHash  string               `json:"record_hash"`
	RecordBytes string               `json:"record_bytes"`
}

// PublicFeedBatch is a signed append-only batch binding source, protocol
// version, schema version, monotonic sequence range, prior-batch hash,
// record hashes, generated time, content hash, signing key ID, and Ed25519
// signature.
type PublicFeedBatch struct {
	ProtocolVersion   string             `json:"protocol_version"`
	SchemaVersion     string             `json:"schema_version"`
	SourceID          string             `json:"source_id"`
	FirstSequence     int64              `json:"first_sequence"`
	LastSequence      int64              `json:"last_sequence"`
	PreviousBatchHash string             `json:"previous_batch_hash"`
	RecordHashes      []string           `json:"record_hashes"`
	GeneratedAt       time.Time          `json:"generated_at"`
	ContentHash       string             `json:"content_hash"`
	SigningKeyID      string             `json:"signing_key_id"`
	Signature         string             `json:"signature"`
	Records           []PublicFeedRecord `json:"records"`
}

// PublicFeedSnapshot binds the high-water sequence and the feed-chain hash.
type PublicFeedSnapshot struct {
	ProtocolVersion   string            `json:"protocol_version"`
	SourceID          string            `json:"source_id"`
	HighWaterSequence int64             `json:"high_water_sequence"`
	FeedChainHash     string            `json:"feed_chain_hash"`
	BatchCount        int               `json:"batch_count"`
	GeneratedAt       time.Time         `json:"generated_at"`
	Freshness         CampaignFreshness `json:"freshness"`
}

// PublicProofCatalogSummary is the summary of the proof catalog for bootstrap.
type PublicProofCatalogSummary struct {
	ArtifactCount   int        `json:"artifact_count"`
	TotalByteSize   int64      `json:"total_byte_size"`
	LastGeneratedAt *time.Time `json:"last_generated_at,omitempty"`
}

// PublicFeedBootstrap is one bounded initial snapshot for public first paint.
type PublicFeedBootstrap struct {
	ProtocolVersion     string                    `json:"protocol_version"`
	Snapshot            PublicFeedSnapshot        `json:"snapshot"`
	SourceFreshness     CampaignFreshness         `json:"source_freshness"`
	RecentProjections   []map[string]any          `json:"recent_projections"`
	ProofCatalogSummary PublicProofCatalogSummary `json:"proof_catalog_summary"`
	GeneratedAt         time.Time                 `json:"generated_at"`
}

// PublicFeedCursorPage is a cursor-paginated page of public records.
type PublicFeedCursorPage struct {
	ProtocolVersion string           `json:"protocol_version"`
	Items           []map[string]any `json:"items"`
	Cursor          string           `json:"cursor,omitempty"`
	HasMore         bool             `json:"has_more"`
	Limit           int              `json:"limit"`
}

// PublicIngestRequest is the authenticated ingest request from the Gateway
// outbound publisher to the hosted mirror.
type PublicIngestRequest struct {
	Batch PublicFeedBatch `json:"batch"`
}

// PublicIngestResponse is the authenticated ingest response from the mirror.
type PublicIngestResponse struct {
	Accepted          bool                            `json:"accepted"`
	HighWaterSequence int64                           `json:"high_water_sequence,omitempty"`
	FeedChainHash     string                          `json:"feed_chain_hash,omitempty"`
	RejectionReason   PublicFeedIngestRejectionReason `json:"rejection_reason,omitempty"`
}

// PublicKeyRevocationRecord is a key revocation record published as a signed
// batch record before the old key is deactivated.
type PublicKeyRevocationRecord struct {
	RevokedKeyID        string    `json:"revoked_key_id"`
	RevokedAt           time.Time `json:"revoked_at"`
	NewKeyID            string    `json:"new_key_id"`
	RevocationSignature string    `json:"revocation_signature"`
}

// PublicProofCatalogEntry is one entry in the public proof catalog. Each
// artifact is served under an immutable content-addressed identity.
type PublicProofCatalogEntry struct {
	ArtifactID          string                        `json:"artifact_id"`
	Filename            string                        `json:"filename"`
	MediaType           string                        `json:"media_type"`
	ByteSize            int64                         `json:"byte_size"`
	SHA256              string                        `json:"sha256"`
	Classification      PublicFeedProofClassification `json:"classification"`
	CampaignID          string                        `json:"campaign_id"`
	SourceRunID         string                        `json:"source_run_id,omitempty"`
	GeneratedAt         time.Time                     `json:"generated_at"`
	VerificationCommand string                        `json:"verification_command"`
	ImmutableURL        string                        `json:"immutable_url"`
}

// PublicProofManifest is the root manifest for a public proof package.
type PublicProofManifest struct {
	SchemaVersion               string                    `json:"schema_version"`
	ProofRootSHA256             string                    `json:"proof_root_sha256"`
	CampaignID                  string                    `json:"campaign_id"`
	CampaignRevision            string                    `json:"campaign_revision"`
	VerifiedIndexGenerationHash string                    `json:"verified_index_generation_hash"`
	VerificationOK              bool                      `json:"verification_ok"`
	ArtifactCount               int                       `json:"artifact_count"`
	Artifacts                   []PublicProofCatalogEntry `json:"artifacts"`
	VerifierInstructions        string                    `json:"verifier_instructions"`
	GeneratedAt                 time.Time                 `json:"generated_at"`
	SigningKeyID                string                    `json:"signing_key_id"`
	Signature                   string                    `json:"signature"`
}

// PublicProofCatalog is the full public proof catalog.
type PublicProofCatalog struct {
	SchemaVersion string                    `json:"schema_version"`
	Entries       []PublicProofCatalogEntry `json:"entries"`
	GeneratedAt   time.Time                 `json:"generated_at"`
}

// PublicExportConfig is the owner-only export configuration for the outbound
// publisher. The private key is stored in the Gateway runtime tree, not in
// this configuration.
type PublicExportConfig struct {
	Enabled                 bool   `json:"enabled"`
	MirrorOrigin            string `json:"mirror_origin"`
	SourceID                string `json:"source_id"`
	SigningKeyID            string `json:"signing_key_id"`
	BatchMaxRecords         int    `json:"batch_max_records"`
	BatchMaxBytes           int    `json:"batch_max_bytes"`
	RetryMaxAttempts        int    `json:"retry_max_attempts"`
	RetryInitialBackoffSecs int    `json:"retry_initial_backoff_seconds"`
	RetryMaxBackoffSecs     int    `json:"retry_max_backoff_seconds"`
	AckWindowSecs           int    `json:"ack_confirmation_window_seconds"`
}

// DefaultPublicExportConfig returns the default export configuration with
// sensible defaults from the constants package. The caller overrides
// Enabled, MirrorOrigin, SourceID, and SigningKeyID.
func DefaultPublicExportConfig() PublicExportConfig {
	return PublicExportConfig{
		Enabled:                 false,
		BatchMaxRecords:         constants.PublicFeedBatchMaxRecords,
		BatchMaxBytes:           constants.PublicFeedBatchMaxBytes,
		RetryMaxAttempts:        constants.PublicFeedRetryMaxAttempts,
		RetryInitialBackoffSecs: constants.PublicFeedRetryInitialBackoff,
		RetryMaxBackoffSecs:     constants.PublicFeedRetryMaxBackoff,
		AckWindowSecs:           constants.PublicFeedAckWindowSeconds,
	}
}

// PublicOutboxEntry is one entry in the durable ordered outbox.
type PublicOutboxEntry struct {
	Sequence       int64                  `json:"sequence"`
	BatchHash      string                 `json:"batch_hash"`
	BatchBytes     string                 `json:"batch_bytes"`
	Status         PublicFeedOutboxStatus `json:"status"`
	Attempts       int                    `json:"attempts"`
	LastAttemptAt  *time.Time             `json:"last_attempt_at,omitempty"`
	AcknowledgedAt *time.Time             `json:"acknowledged_at,omitempty"`
	CreatedAt      time.Time              `json:"created_at"`
}
