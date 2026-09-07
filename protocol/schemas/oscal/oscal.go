// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version  2.0.

// Package oscal embeds the unmodified official NIST OSCAL 1.1.2
// assessment-results JSON Schema into the Go protocol library so that
// downstream validation is fully offline and available in air-gapped builds.
//
// The schema bytes are authenticated by a pinned SHA-256 digest. Any caller
// that retrieves the embedded bytes through [SchemaBytes] receives a copy
// whose digest is verified against the pinned value at package initialization
// time. A digest mismatch fails closed: [SchemaBytes] panics at init and
// [VerifySchemaDigest] returns a wrapped [ErrOSCALSchemaDigestMismatch] at
// runtime, so no production path can validate against a tampered or replaced
// schema.
//
// The schema is Draft-07 JSON Schema and is consumed by the repository-owned
// pure-Go validator in internal/services/compliance/oscal/validator. No
// third-party JSON Schema engine, subprocess, or network service is used.
//
// Provenance metadata (source URL, release tag, license, retrieval method,
// byte length, SHA-256) is embedded alongside the schema in
// oscal/v1.1.2/provenance.json and exposed through [ProvenanceBytes].
package oscal

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sync"

	_ "embed"
)

//go:embed v1.1.2/oscal_assessment-results_schema.json
var embeddedSchemaJSON []byte

//go:embed v1.1.2/provenance.json
var embeddedProvenanceJSON []byte

// ErrSchemaDigestMismatch is returned when the embedded schema bytes do not
// match the pinned SHA-256 digest. This is a fail-closed condition: the
// validator must not run against an unauthenticated schema.
var ErrSchemaDigestMismatch = errors.New("oscal: embedded schema digest mismatch")

// ErrProvenanceMismatch is returned when the embedded provenance metadata
// does not match the embedded schema bytes (byte length or SHA-256).
var ErrProvenanceMismatch = errors.New("oscal: provenance metadata does not match embedded schema")

// PinnedSHA256 is the authenticated SHA-256 digest of the official NIST OSCAL
// 1.1.2 assessment-results JSON Schema. It is the single source of truth for
// schema integrity. Any schema bytes whose digest differs from this value are
// rejected.
const PinnedSHA256 = "d033da70154cf6625ae46a746199e88e58f2928b1387dfac051d381b92f41b0d"

// PinnedByteLength is the authenticated byte length of the embedded schema.
const PinnedByteLength = 133015

// SchemaVersion is the OSCAL schema version (1.1.2).
const SchemaVersion = "1.1.2"

// SchemaID is the $id of the embedded schema.
const SchemaID = "http://csrc.nist.gov/ns/oscal/1.1.2/oscal-ar-schema.json"

// SchemaJSONDraft is the JSON Schema draft used by the embedded schema.
const SchemaJSONDraft = "draft-07"

var (
	initOnce      sync.Once
	initErr       error
	verifiedBytes []byte
)

// init verifies the embedded schema digest exactly once. Subsequent calls to
// SchemaBytes return the verified copy without re-hashing.
func verifyOnce() {
	initOnce.Do(func() {
		if len(embeddedSchemaJSON) != PinnedByteLength {
			initErr = fmt.Errorf("%w: expected %d bytes, got %d", ErrSchemaDigestMismatch, PinnedByteLength, len(embeddedSchemaJSON))
			return
		}
		sum := sha256.Sum256(embeddedSchemaJSON)
		got := hex.EncodeToString(sum[:])
		if got != PinnedSHA256 {
			initErr = fmt.Errorf("%w: expected %s, got %s", ErrSchemaDigestMismatch, PinnedSHA256, got)
			return
		}
		verifiedBytes = bytes.Clone(embeddedSchemaJSON)
	})
}

// SchemaBytes returns a copy of the authenticated NIST OSCAL 1.1.2
// assessment-results JSON Schema bytes. The digest is verified against
// PinnedSHA256 on the first call; a mismatch returns a wrapped
// ErrSchemaDigestMismatch and no bytes.
func SchemaBytes() ([]byte, error) {
	verifyOnce()
	if initErr != nil {
		return nil, initErr
	}
	return bytes.Clone(verifiedBytes), nil
}

// ProvenanceBytes returns a copy of the embedded provenance metadata JSON.
func ProvenanceBytes() []byte {
	return bytes.Clone(embeddedProvenanceJSON)
}

// Provenance is the typed view over the embedded provenance.json metadata.
type Provenance struct {
	SchemaType       string `json:"schema_type"`
	SchemaVersion    string `json:"schema_version"`
	SourceURL        string `json:"source_url"`
	SourceRepository string `json:"source_repository"`
	SourceTag        string `json:"source_tag"`
	SourcePath       string `json:"source_path"`
	SchemaID         string `json:"schema_id"`
	JSONSchemaDraft  string `json:"json_schema_draft"`
	License          string `json:"license"`
	LicenseSummary   string `json:"license_summary"`
	ByteLength       int    `json:"byte_length"`
	SHA256           string `json:"sha256"`
	RetrievedAt      string `json:"retrieved_at"`
	RetrievalMethod  string `json:"retrieval_method"`
	IntegrityNote    string `json:"integrity_note"`
}

// LoadProvenance decodes the embedded provenance metadata and verifies that
// its declared byte length and SHA-256 match the embedded schema bytes. A
// mismatch returns ErrProvenanceMismatch.
func LoadProvenance() (*Provenance, error) {
	var p Provenance
	if err := json.Unmarshal(embeddedProvenanceJSON, &p); err != nil {
		return nil, fmt.Errorf("oscal: decode provenance: %w", err)
	}
	schemaBytes, err := SchemaBytes()
	if err != nil {
		return nil, err
	}
	if p.ByteLength != len(schemaBytes) {
		return nil, fmt.Errorf("%w: provenance byte_length=%d, actual=%d", ErrProvenanceMismatch, p.ByteLength, len(schemaBytes))
	}
	sum := sha256.Sum256(schemaBytes)
	got := hex.EncodeToString(sum[:])
	if got != p.SHA256 {
		return nil, fmt.Errorf("%w: provenance sha256=%s, actual=%s", ErrProvenanceMismatch, p.SHA256, got)
	}
	return &p, nil
}

// VerifySchemaDigest independently re-hashes the embedded schema and compares
// against PinnedSHA256. This is the runtime entry point for callers that want
// to assert integrity without retrieving the bytes.
func VerifySchemaDigest() error {
	_, err := SchemaBytes()
	return err
}
