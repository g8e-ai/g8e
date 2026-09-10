// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package models

import "time"

// EvidenceEncryptionWire mirrors the Python EvidenceEncryption model from
// ensemble/evals/g8e_evals/schema.py for cross-language wire compatibility.
// Restricted artifacts carry encryption metadata; public artifacts do not.
type EvidenceEncryptionWire struct {
	Algorithm            string `json:"algorithm"`
	KeyID                string `json:"key_id"`
	AADSHA256            string `json:"aad_sha256"`
	CiphertextSHA256     string `json:"ciphertext_sha256"`
	CiphertextByteLength int64  `json:"ciphertext_byte_length"`
}

// BundleArtifactEntryWire mirrors the Python BundleArtifactEntry model from
// ensemble/evals/g8e_evals/bundle/manifest.py. Each entry enumerates a single
// file beneath the bundle root with its media type, privacy class, canonical
// SHA-256 hash, byte length, semantic record identity, record count, and
// optional encryption metadata for restricted artifacts.
type BundleArtifactEntryWire struct {
	Path         string                  `json:"path"`
	MediaType    string                  `json:"media_type"`
	PrivacyClass string                  `json:"privacy_class"`
	SHA256       string                  `json:"sha256"`
	ByteLength   int64                   `json:"byte_length"`
	ArtifactType string                  `json:"artifact_type"`
	RecordCount  int                     `json:"record_count"`
	Encryption   *EvidenceEncryptionWire `json:"encryption,omitempty"`
}

// ExternalReferenceWire mirrors the Python ExternalReference model. It is a
// content-addressed reference to an artifact outside the bundle.
type ExternalReferenceWire struct {
	ReferenceID   string `json:"reference_id"`
	ContentSHA256 string `json:"content_sha256"`
	ByteLength    int64  `json:"byte_length"`
	MediaType     string `json:"media_type"`
	Description   string `json:"description"`
	SourceURI     string `json:"source_uri"`
}

// BundleManifestWire mirrors the Python BundleManifest model from
// ensemble/evals/g8e_evals/bundle/manifest.py. It is the immutable, versioned
// bundle manifest enumerating every included artifact. The publication path
// reads this as a typed input from the verified bundle; it does not recompute
// canonical analysis.
type BundleManifestWire struct {
	SchemaVersion         string                    `json:"schema_version"`
	BundleID              string                    `json:"bundle_id"`
	RunID                 string                    `json:"run_id"`
	ReleaseVersion        string                    `json:"release_version"`
	CreatedAt             time.Time                 `json:"created_at"`
	Artifacts             []BundleArtifactEntryWire `json:"artifacts"`
	ExternalReferences    []ExternalReferenceWire   `json:"external_references"`
	ManifestContentSHA256 string                    `json:"manifest_content_sha256"`
	ChecksumRootSHA256    string                    `json:"checksum_root_sha256"`
}

// LayerResultWire mirrors the Python LayerResult model from
// ensemble/evals/g8e_evals/bundle/verify.py. It is the result of one
// verification layer.
type LayerResultWire struct {
	Layer        int  `json:"layer"`
	Passed       bool `json:"passed"`
	FailureCount int  `json:"failure_count"`
}

// VerificationFailureWire mirrors the Python VerificationFailure model. It is
// one typed verification failure with stable code, layer, and record identity.
type VerificationFailureWire struct {
	Layer    int    `json:"layer"`
	Code     string `json:"code"`
	RecordID string `json:"record_id"`
	Message  string `json:"message"`
}

// VerificationReportWire mirrors the Python VerificationReport model from
// ensemble/evals/g8e_evals/bundle/verify.py. It is the typed deterministic
// verification report. OK is true only when every layer passed with zero
// failures. The publication path requires OK=true to publish with the
// "verified" status; a receipt-only or partial verification cannot produce it.
type VerificationReportWire struct {
	SchemaVersion  string                    `json:"schema_version"`
	BundleID       string                    `json:"bundle_id"`
	RunID          string                    `json:"run_id"`
	ReleaseVersion string                    `json:"release_version"`
	VerifiedAt     time.Time                 `json:"verified_at"`
	OK             bool                      `json:"ok"`
	Layers         []LayerResultWire         `json:"layers"`
	Failures       []VerificationFailureWire `json:"failures"`
}

// ObserveProducerDownloadArtifactInput is one entry in the publication
// request's download catalog. It carries the artifact metadata and base64
// content for a single public-safe artifact. Restricted artifacts are never
// transmitted; only public_safe artifacts appear in the download catalog.
type ObserveProducerDownloadArtifactInput struct {
	ArtifactID            string                        `json:"artifact_id"`
	Filename              string                        `json:"filename"`
	MediaType             string                        `json:"media_type"`
	ByteSize              int64                         `json:"byte_size"`
	SHA256                string                        `json:"sha256"`
	PrivacyClassification DownloadPrivacyClassification `json:"privacy_classification"`
	SourceRunID           string                        `json:"source_run_id"`
	Content               string                        `json:"content"`
}

// ObserveProducerEvalPublicationRequest is the typed request body for the mTLS
// publication endpoint POST /api/v1/observe/producer/eval-publication. It
// carries the verified bundle manifest, the verification report, the
// campaign-aware eval projection fields extracted from the canonical analysis
// and campaign records by the publisher, and the download catalog with base64
// content for public-safe artifacts. The gateway derives user_id from the
// mTLS peer certificate, never from the request body. The request carries no
// user_id field; unknown identity fields are rejected. Campaign dimensions
// (CampaignID, ArmIDs, ModelCohortIDs, AssignmentCount) replace the former
// single ArmID and single ModelID/ModelProvider scalars so a multi-arm,
// multi-cohort campaign publishes as one projection.
type ObserveProducerEvalPublicationRequest struct {
	SchemaVersion      string                                 `json:"schema_version"`
	BundleID           string                                 `json:"bundle_id"`
	RunID              string                                 `json:"run_id"`
	ReleaseVersion     string                                 `json:"release_version"`
	SuiteID            string                                 `json:"suite_id"`
	SuiteVersion       string                                 `json:"suite_version"`
	CampaignID         string                                 `json:"campaign_id"`
	ArmIDs             []string                               `json:"arm_ids"`
	ModelCohortIDs     []string                               `json:"model_cohort_ids"`
	AssignmentCount    int                                    `json:"assignment_count"`
	ReceiptCount       int                                    `json:"receipt_count"`
	AssignedTasks      int                                    `json:"assigned_tasks"`
	TerminalAttempts   int                                    `json:"terminal_attempts"`
	Metrics            []EvalMetricSummary                    `json:"metrics"`
	VerificationReport VerificationReportWire                 `json:"verification_report"`
	BundleManifest     BundleManifestWire                     `json:"bundle_manifest"`
	Downloads          []ObserveProducerDownloadArtifactInput `json:"downloads"`
	WebSessionID       string                                 `json:"web_session_id,omitempty"`
	CLISessionID       string                                 `json:"cli_session_id,omitempty"`
}
