// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package evidence

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"google.golang.org/protobuf/proto"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/services/governance"
	compliancev1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/compliance/v1"
	operatorv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/operator/v1"
)

// maxArtifactBytes is the default size limit for a single evidence artifact
// read by an importer. Importers may override this with a larger or smaller
// limit.
const maxArtifactBytes = 16 << 20

// ReadResult holds the bytes and digest of a read artifact.
type ReadResult struct {
	Bytes  []byte
	SHA256 string
}

// ReadAndDigest reads a file from the given reader, enforces the size limit,
// and computes the SHA-256 digest of the bytes.
func ReadAndDigest(reader ArtifactReader, ctx context.Context, path string, maxBytes int) (ReadResult, error) {
	if maxBytes <= 0 {
		maxBytes = maxArtifactBytes
	}
	body, err := reader.ReadFile(ctx, path)
	if err != nil {
		return ReadResult{}, err
	}
	if len(body) > maxBytes {
		return ReadResult{}, constants.ErrEvidenceArtifactTooLarge
	}
	digest := sha256.Sum256(body)
	return ReadResult{Bytes: body, SHA256: hex.EncodeToString(digest[:])}, nil
}

// UnmarshalCanonicalProto unmarshals canonical JSON bytes into a proto
// message using the compliance canonical decoder.
func UnmarshalCanonicalProto(body []byte, msg proto.Message) error {
	return compliancev1.UnmarshalCanonical(body, msg)
}

// MarshalCanonicalProto marshals a proto message into canonical JSON bytes.
func MarshalCanonicalProto(msg proto.Message) ([]byte, error) {
	return compliancev1.MarshalCanonical(msg)
}

// VerifyDigest compares the SHA-256 of the given bytes to the expected
// hex-encoded digest.
func VerifyDigest(body []byte, expected string) bool {
	digest := sha256.Sum256(body)
	return hex.EncodeToString(digest[:]) == expected
}

// ContentReferenceForBody computes the content-addressed reference for the
// given prefix and body bytes: "<prefix>:sha256:<hex-digest>".
func ContentReferenceForBody(prefix string, body []byte) string {
	digest := sha256.Sum256(body)
	return prefix + ":sha256:" + hex.EncodeToString(digest[:])
}

// ParseContentReference splits a content-addressed reference into its prefix,
// digest, and validity flag. The format is "<prefix>:sha256:<64-hex-chars>".
func ParseContentReference(reference string) (string, string, bool) {
	parts := strings.Split(reference, ":")
	if len(parts) != 3 || parts[0] == "" || parts[1] != "sha256" || len(parts[2]) != sha256.Size*2 || strings.ToLower(parts[2]) != parts[2] {
		return "", "", false
	}
	decoded, err := hex.DecodeString(parts[2])
	return parts[0], parts[2], err == nil && len(decoded) == sha256.Size
}

// ParseExpectedContentReference parses a content reference and verifies the
// prefix matches the expected value.
func ParseExpectedContentReference(reference, expectedPrefix string) (string, string, bool) {
	prefix, digest, ok := ParseContentReference(reference)
	return prefix, digest, ok && prefix == expectedPrefix
}

// ValidateCanonicalJSON verifies that the bytes are valid JSON with no
// trailing data and that the encoding is canonical compact (no extra
// whitespace).
func ValidateCanonicalJSON(body []byte) error {
	decoder := json.NewDecoder(bytes.NewReader(body))
	var raw json.RawMessage
	if err := decoder.Decode(&raw); err != nil {
		return err
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return fmt.Errorf("JSON contains trailing data")
	}
	var compact bytes.Buffer
	if err := json.Compact(&compact, raw); err != nil {
		return err
	}
	if !bytes.Equal(body, compact.Bytes()) {
		return fmt.Errorf("JSON is not canonical compact encoding")
	}
	return nil
}

// ValidPathElement returns true if the value is a single safe path component
// (no separators, no traversal, not empty).
func ValidPathElement(value string) bool {
	return value != "" && value != "." && value != ".." && filepath.Base(value) == value && !filepath.IsAbs(value)
}

// ValidRelativePath returns true if the value is a safe relative path that
// does not escape the root via traversal.
func ValidRelativePath(value string) bool {
	clean := filepath.Clean(value)
	return clean != "." && clean != ".." && !filepath.IsAbs(clean) && !strings.HasPrefix(clean, ".."+string(os.PathSeparator))
}

// SignerPublicKey decodes a hex-encoded Ed25519 public key.
func SignerPublicKey(keyID string) (ed25519.PublicKey, error) {
	return governance.SignerPublicKey(keyID)
}

// ClassifyReadError maps a read error to the appropriate evidence error
// constant.
func ClassifyReadError(err error) error {
	if errors.Is(err, constants.ErrNotFound) {
		return constants.ErrUnresolvedReference
	}
	if errors.Is(err, constants.ErrEvidenceArtifactTooLarge) {
		return constants.ErrEvidenceArtifactTooLarge
	}
	return constants.ErrInvalidEvidenceGraph
}

// Contains returns true if the target string appears in the values slice.
func Contains(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

// EqualStringSets returns true if both slices contain the same set of
// strings (order-independent).
func EqualStringSets(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	left = append([]string(nil), left...)
	right = append([]string(nil), right...)
	for i, v := range left {
		if !Contains(right, v) {
			return false
		}
		_ = i
	}
	return true
}

// VersionedKey returns "<id>@<version>".
func VersionedKey(id, version string) string {
	return id + "@" + version
}

// VerifyReceiptSignature verifies an ActionReceipt signature against the
// given Ed25519 public key. Returns nil if the signature is valid.
func VerifyReceiptSignature(receipt *operatorv1.ActionReceipt, publicKey ed25519.PublicKey) error {
	return governance.VerifyActionReceiptSignature(receipt, publicKey)
}

// VerifyReceiptPersistence verifies the final-persistence attestation on an
// ActionReceipt. Returns nil if the attestation is valid.
func VerifyReceiptPersistence(receipt *operatorv1.ActionReceipt, publicKey ed25519.PublicKey) error {
	return governance.VerifyReceiptPersistenceAttestation(receipt, publicKey)
}

// ReceiptInvestigationBound returns true if any deterministic stage evidence
// in the receipt carries one of the given investigation IDs.
func ReceiptInvestigationBound(receipt *operatorv1.ActionReceipt, investigationIDs []string) bool {
	for _, stage := range receipt.GetDeterministicStageEvidence() {
		if Contains(investigationIDs, stage.GetInvestigationId()) {
			return true
		}
	}
	return false
}

// ReceiptAttestationError maps a persistence attestation error to the
// appropriate typed error constant.
func ReceiptAttestationError(err error) error {
	for _, target := range []error{constants.ErrReceiptPersistenceAttestationMissing, constants.ErrReceiptPersistenceSignatureMismatch, constants.ErrReceiptPersistenceAttestationInvalid} {
		if errors.Is(err, target) {
			return target
		}
	}
	return constants.ErrReceiptPersistenceAttestationInvalid
}

// DemoScope returns the canonical scope ID for a demo organization ID.
func DemoScope(demoID string) string {
	switch demoID {
	case constants.DemosOrgFedRAMP:
		return constants.DemoScopeFedRAMP
	case constants.DemosOrgDHS:
		return constants.DemoScopeDHS
	case constants.DemosOrgFinance:
		return constants.DemoScopeFinance
	case constants.DemosOrgHealthcare:
		return constants.DemoScopeHealthcare
	default:
		return ""
	}
}
