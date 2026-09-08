// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package report

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"google.golang.org/protobuf/proto"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/services/compliance/catalog"
	compliancev1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/compliance/v1"
)

type ComplianceReportSigningIdentity struct {
	metadata   *compliancev1.ComplianceReportSigningKeyMetadata
	privateKey ed25519.PrivateKey
}

func NewComplianceReportSigningIdentity(metadata *compliancev1.ComplianceReportSigningKeyMetadata, privateKey ed25519.PrivateKey) (*ComplianceReportSigningIdentity, error) {
	if err := validateComplianceReportSigningKeyMetadata(metadata); err != nil {
		return nil, err
	}
	if len(privateKey) != ed25519.PrivateKeySize {
		return nil, fmt.Errorf("%w: Ed25519 private key has invalid length", constants.ErrReportSignatureFailed)
	}
	publicKey, ok := privateKey.Public().(ed25519.PublicKey)
	if !ok {
		return nil, fmt.Errorf("%w: derive Ed25519 public key", constants.ErrReportSignatureFailed)
	}
	publicKeyDigest := sha256.Sum256(publicKey)
	if metadata.PublicKeySha256 != hex.EncodeToString(publicKeyDigest[:]) {
		return nil, fmt.Errorf("%w: signing key metadata does not match private key", constants.ErrReportSignatureFailed)
	}
	return &ComplianceReportSigningIdentity{
		metadata:   proto.Clone(metadata).(*compliancev1.ComplianceReportSigningKeyMetadata),
		privateKey: append(ed25519.PrivateKey(nil), privateKey...),
	}, nil
}

func (i *ComplianceReportSigningIdentity) SignSHA256(digest string) (*compliancev1.ReportSignature, error) {
	if i == nil || i.metadata == nil || len(i.privateKey) != ed25519.PrivateKeySize {
		return nil, fmt.Errorf("%w: signing identity is unavailable", constants.ErrReportSignatureFailed)
	}
	digestBytes, err := decodeLowerSHA256(digest)
	if err != nil {
		return nil, err
	}
	return &compliancev1.ReportSignature{
		KeyId:        i.metadata.KeyId,
		Algorithm:    constants.ComplianceReportSignatureAlgorithm,
		SignedSha256: digest,
		Signature:    hex.EncodeToString(ed25519.Sign(i.privateKey, digestBytes)),
	}, nil
}

func VerifyComplianceReportSignature(signature *compliancev1.ReportSignature, policy *compliancev1.ComplianceReportTrustPolicy, scopeRef string, signedAt time.Time) error {
	if err := catalog.ValidateReportSignature(signature); err != nil {
		return err
	}
	if err := validateComplianceReportTrustPolicy(policy); err != nil {
		return err
	}
	if strings.TrimSpace(scopeRef) == "" || signedAt.IsZero() {
		return fmt.Errorf("%w: report scope and signing time are required", constants.ErrEvidenceTrustNotAssessed)
	}
	trustedKey := findTrustedReportKey(policy, signature.KeyId)
	if trustedKey == nil {
		return fmt.Errorf("%w: report signing key %s is not assessed", constants.ErrEvidenceTrustNotAssessed, signature.KeyId)
	}
	if !containsExact(trustedKey.AllowedScopeRefs, scopeRef) {
		return fmt.Errorf("%w: report signing key %s is not assessed for scope %s", constants.ErrEvidenceTrustNotAssessed, signature.KeyId, scopeRef)
	}
	createdAt := trustedKey.Metadata.CreatedAt.AsTime()
	expiresAt := trustedKey.Metadata.ExpiresAt.AsTime()
	if signedAt.Before(createdAt) || signedAt.After(expiresAt) {
		return fmt.Errorf("%w: report signing key %s is outside its validity interval", constants.ErrEvidenceTrustNotAssessed, signature.KeyId)
	}
	if trustedKey.RevokedAt != nil && !signedAt.Before(trustedKey.RevokedAt.AsTime()) {
		return fmt.Errorf("%w: report signing key %s was revoked", constants.ErrEvidenceTrustNotAssessed, signature.KeyId)
	}
	publicKeyBytes, err := hex.DecodeString(trustedKey.PublicKey)
	if err != nil || len(publicKeyBytes) != ed25519.PublicKeySize {
		return fmt.Errorf("%w: report signing key %s is malformed", constants.ErrEvidenceTrustNotAssessed, signature.KeyId)
	}
	publicKeyDigest := sha256.Sum256(publicKeyBytes)
	if trustedKey.Metadata.PublicKeySha256 != hex.EncodeToString(publicKeyDigest[:]) {
		return fmt.Errorf("%w: report signing key %s does not match assessed metadata", constants.ErrEvidenceTrustNotAssessed, signature.KeyId)
	}
	digestBytes, err := decodeLowerSHA256(signature.SignedSha256)
	if err != nil {
		return err
	}
	signatureBytes, err := hex.DecodeString(signature.Signature)
	if err != nil || len(signatureBytes) != ed25519.SignatureSize || !ed25519.Verify(ed25519.PublicKey(publicKeyBytes), digestBytes, signatureBytes) {
		return fmt.Errorf("%w: Ed25519 verification failed", constants.ErrReportSignatureFailed)
	}
	return nil
}

func validateComplianceReportSigningKeyMetadata(metadata *compliancev1.ComplianceReportSigningKeyMetadata) error {
	if metadata == nil || metadata.KeyId == "" || metadata.KeyId != strings.TrimSpace(metadata.KeyId) || metadata.Algorithm != constants.ComplianceReportSignatureAlgorithm || metadata.Purpose != constants.ComplianceReportSigningPurpose || metadata.CreatedAt == nil || metadata.ExpiresAt == nil {
		return fmt.Errorf("%w: signing key metadata is incomplete or unsupported", constants.ErrReportSignatureFailed)
	}
	if metadata.CreatedAt.CheckValid() != nil || metadata.ExpiresAt.CheckValid() != nil || !metadata.CreatedAt.AsTime().Before(metadata.ExpiresAt.AsTime()) {
		return fmt.Errorf("%w: signing key validity interval is invalid", constants.ErrReportSignatureFailed)
	}
	if _, err := decodeLowerSHA256(metadata.PublicKeySha256); err != nil {
		return fmt.Errorf("%w: signing key public digest is invalid", constants.ErrReportSignatureFailed)
	}
	return nil
}

func validateComplianceReportTrustPolicy(policy *compliancev1.ComplianceReportTrustPolicy) error {
	if policy == nil || policy.PolicyId == "" || policy.PolicyId != strings.TrimSpace(policy.PolicyId) || policy.PolicyVersion == "" || policy.PolicyVersion != strings.TrimSpace(policy.PolicyVersion) || len(policy.TrustedKeys) == 0 {
		return fmt.Errorf("%w: compliance report trust policy is incomplete", constants.ErrEvidenceTrustNotAssessed)
	}
	seen := make(map[string]struct{}, len(policy.TrustedKeys))
	for _, trustedKey := range policy.TrustedKeys {
		if trustedKey == nil || trustedKey.Metadata == nil || trustedKey.PublicKey == "" || trustedKey.PublicKey != strings.TrimSpace(trustedKey.PublicKey) || trustedKey.AssessmentId == "" || trustedKey.AssessmentId != strings.TrimSpace(trustedKey.AssessmentId) || trustedKey.AssessorIdentity == "" || trustedKey.AssessorIdentity != strings.TrimSpace(trustedKey.AssessorIdentity) || trustedKey.AssessedAt == nil || trustedKey.AssessedAt.CheckValid() != nil || len(trustedKey.AllowedScopeRefs) == 0 {
			return fmt.Errorf("%w: assessed report signing key is incomplete", constants.ErrEvidenceTrustNotAssessed)
		}
		if err := validateComplianceReportSigningKeyMetadata(trustedKey.Metadata); err != nil {
			return fmt.Errorf("%w: assessed report signing key metadata is invalid", constants.ErrEvidenceTrustNotAssessed)
		}
		publicKey, err := hex.DecodeString(trustedKey.PublicKey)
		if err != nil || len(publicKey) != ed25519.PublicKeySize || trustedKey.PublicKey != strings.ToLower(trustedKey.PublicKey) {
			return fmt.Errorf("%w: assessed report signing key %s is malformed", constants.ErrEvidenceTrustNotAssessed, trustedKey.Metadata.KeyId)
		}
		publicKeyDigest := sha256.Sum256(publicKey)
		if trustedKey.Metadata.PublicKeySha256 != hex.EncodeToString(publicKeyDigest[:]) {
			return fmt.Errorf("%w: assessed report signing key %s does not match metadata", constants.ErrEvidenceTrustNotAssessed, trustedKey.Metadata.KeyId)
		}
		if _, exists := seen[trustedKey.Metadata.KeyId]; exists {
			return fmt.Errorf("%w: report signing key %s is duplicated", constants.ErrEvidenceTrustNotAssessed, trustedKey.Metadata.KeyId)
		}
		seen[trustedKey.Metadata.KeyId] = struct{}{}
		if !uniqueNonEmptyStrings(trustedKey.AllowedScopeRefs) {
			return fmt.Errorf("%w: assessed scopes for report signing key %s are invalid", constants.ErrEvidenceTrustNotAssessed, trustedKey.Metadata.KeyId)
		}
		if trustedKey.RevokedAt != nil && (trustedKey.RevokedAt.CheckValid() != nil || trustedKey.RevokedAt.AsTime().Before(trustedKey.Metadata.CreatedAt.AsTime())) {
			return fmt.Errorf("%w: revocation for report signing key %s is invalid", constants.ErrEvidenceTrustNotAssessed, trustedKey.Metadata.KeyId)
		}
	}
	return nil
}

func decodeLowerSHA256(value string) ([]byte, error) {
	decoded, err := hex.DecodeString(value)
	if err != nil || len(decoded) != sha256.Size || value != strings.ToLower(value) {
		return nil, fmt.Errorf("%w: SHA-256 digest is invalid", constants.ErrReportSignatureFailed)
	}
	return decoded, nil
}

func findTrustedReportKey(policy *compliancev1.ComplianceReportTrustPolicy, keyID string) *compliancev1.ComplianceReportTrustedKey {
	for _, trustedKey := range policy.TrustedKeys {
		if trustedKey.Metadata.KeyId == keyID {
			return trustedKey
		}
	}
	return nil
}

func containsExact(values []string, expected string) bool {
	for _, value := range values {
		if value == expected {
			return true
		}
	}
	return false
}

func uniqueNonEmptyStrings(values []string) bool {
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		if value == "" || value != strings.TrimSpace(value) {
			return false
		}
		if _, exists := seen[value]; exists {
			return false
		}
		seen[value] = struct{}{}
	}
	return true
}
