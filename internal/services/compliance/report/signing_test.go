// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package report

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	compliancev1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/compliance/v1"
)

func reportSigningFixture(t *testing.T) (ed25519.PrivateKey, *compliancev1.ComplianceReportSigningKeyMetadata, *compliancev1.ComplianceReportTrustPolicy, time.Time) {
	t.Helper()
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	createdAt := time.Unix(1_700_000_000, 0).UTC()
	expiresAt := createdAt.Add(24 * time.Hour)
	publicKeyDigest := sha256.Sum256(publicKey)
	metadata := &compliancev1.ComplianceReportSigningKeyMetadata{
		KeyId:           "report-key-1",
		Algorithm:       constants.ComplianceReportSignatureAlgorithm,
		Purpose:         constants.ComplianceReportSigningPurpose,
		PublicKeySha256: hex.EncodeToString(publicKeyDigest[:]),
		CreatedAt:       timestamppb.New(createdAt),
		ExpiresAt:       timestamppb.New(expiresAt),
	}
	policy := &compliancev1.ComplianceReportTrustPolicy{
		PolicyId:      "report-trust-policy-1",
		PolicyVersion: "1.0.0",
		TrustedKeys: []*compliancev1.ComplianceReportTrustedKey{{
			Metadata:         proto.Clone(metadata).(*compliancev1.ComplianceReportSigningKeyMetadata),
			PublicKey:        hex.EncodeToString(publicKey),
			AssessmentId:     "assessment-1",
			AssessorIdentity: "assessor-1",
			AssessedAt:       timestamppb.New(createdAt),
			AllowedScopeRefs: []string{"scope-1"},
		}},
	}
	return privateKey, metadata, policy, createdAt.Add(time.Hour)
}

func TestComplianceReportTrustPolicy_CanonicalRoundTrip(t *testing.T) {
	_, _, policy, _ := reportSigningFixture(t)
	body, err := compliancev1.MarshalCanonical(policy)
	require.NoError(t, err)
	decoded := &compliancev1.ComplianceReportTrustPolicy{}
	require.NoError(t, compliancev1.UnmarshalCanonical(body, decoded))
	assert.True(t, proto.Equal(policy, decoded))
}

func TestNewComplianceReportSigningIdentity_RejectsKeyMetadataMismatch(t *testing.T) {
	privateKey, metadata, _, _ := reportSigningFixture(t)
	tests := []struct {
		name   string
		mutate func(*compliancev1.ComplianceReportSigningKeyMetadata)
	}{
		{name: "missing key identifier", mutate: func(value *compliancev1.ComplianceReportSigningKeyMetadata) { value.KeyId = "" }},
		{name: "wrong algorithm", mutate: func(value *compliancev1.ComplianceReportSigningKeyMetadata) { value.Algorithm = "receipt-ed25519" }},
		{name: "wrong purpose", mutate: func(value *compliancev1.ComplianceReportSigningKeyMetadata) { value.Purpose = "action-receipt" }},
		{name: "wrong public key digest", mutate: func(value *compliancev1.ComplianceReportSigningKeyMetadata) {
			value.PublicKeySha256 = strings.Repeat("0", 64)
		}},
		{name: "missing creation time", mutate: func(value *compliancev1.ComplianceReportSigningKeyMetadata) { value.CreatedAt = nil }},
		{name: "expiration before creation", mutate: func(value *compliancev1.ComplianceReportSigningKeyMetadata) {
			value.ExpiresAt = timestamppb.New(value.CreatedAt.AsTime().Add(-time.Second))
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			candidate := proto.Clone(metadata).(*compliancev1.ComplianceReportSigningKeyMetadata)
			tt.mutate(candidate)
			identity, err := NewComplianceReportSigningIdentity(candidate, privateKey)
			assert.Nil(t, identity)
			assert.ErrorIs(t, err, constants.ErrReportSignatureFailed)
		})
	}
}

func TestNewComplianceReportSigningIdentity_RejectsInvalidPrivateKey(t *testing.T) {
	_, metadata, _, _ := reportSigningFixture(t)
	identity, err := NewComplianceReportSigningIdentity(metadata, ed25519.PrivateKey("not-an-ed25519-private-key"))
	assert.Nil(t, identity)
	assert.ErrorIs(t, err, constants.ErrReportSignatureFailed)
}

func TestComplianceReportSigningIdentity_SignsDigestWithDedicatedIdentity(t *testing.T) {
	privateKey, metadata, _, _ := reportSigningFixture(t)
	identity, err := NewComplianceReportSigningIdentity(metadata, privateKey)
	require.NoError(t, err)
	digest := sha256.Sum256([]byte("canonical manifest root and checksum root"))
	signature, err := identity.SignSHA256(hex.EncodeToString(digest[:]))
	require.NoError(t, err)
	assert.Equal(t, metadata.KeyId, signature.KeyId)
	assert.Equal(t, constants.ComplianceReportSignatureAlgorithm, signature.Algorithm)
	assert.Equal(t, hex.EncodeToString(digest[:]), signature.SignedSha256)
	decoded, err := hex.DecodeString(signature.Signature)
	require.NoError(t, err)
	assert.True(t, ed25519.Verify(privateKey.Public().(ed25519.PublicKey), digest[:], decoded))
}

func TestVerifyComplianceReportSignature_RequiresAssessedTrust(t *testing.T) {
	privateKey, metadata, policy, signedAt := reportSigningFixture(t)
	identity, err := NewComplianceReportSigningIdentity(metadata, privateKey)
	require.NoError(t, err)
	digest := sha256.Sum256([]byte("protected report roots"))
	signature, err := identity.SignSHA256(hex.EncodeToString(digest[:]))
	require.NoError(t, err)
	assert.NoError(t, VerifyComplianceReportSignature(signature, policy, "scope-1", signedAt))

	tests := []struct {
		name   string
		mutate func(*compliancev1.ReportSignature, *compliancev1.ComplianceReportTrustPolicy) (string, time.Time)
		want   error
	}{
		{name: "missing trust policy", mutate: func(*compliancev1.ReportSignature, *compliancev1.ComplianceReportTrustPolicy) (string, time.Time) {
			return "scope-1", signedAt
		}, want: constants.ErrEvidenceTrustNotAssessed},
		{name: "missing policy identity", mutate: func(_ *compliancev1.ReportSignature, policy *compliancev1.ComplianceReportTrustPolicy) (string, time.Time) {
			policy.PolicyId = ""
			return "scope-1", signedAt
		}, want: constants.ErrEvidenceTrustNotAssessed},
		{name: "key absent from policy", mutate: func(value *compliancev1.ReportSignature, _ *compliancev1.ComplianceReportTrustPolicy) (string, time.Time) {
			value.KeyId = "untrusted-key"
			return "scope-1", signedAt
		}, want: constants.ErrEvidenceTrustNotAssessed},
		{name: "duplicate assessed key", mutate: func(_ *compliancev1.ReportSignature, policy *compliancev1.ComplianceReportTrustPolicy) (string, time.Time) {
			policy.TrustedKeys = append(policy.TrustedKeys, proto.Clone(policy.TrustedKeys[0]).(*compliancev1.ComplianceReportTrustedKey))
			return "scope-1", signedAt
		}, want: constants.ErrEvidenceTrustNotAssessed},
		{name: "malformed unrelated assessed key", mutate: func(_ *compliancev1.ReportSignature, policy *compliancev1.ComplianceReportTrustPolicy) (string, time.Time) {
			unrelated := proto.Clone(policy.TrustedKeys[0]).(*compliancev1.ComplianceReportTrustedKey)
			unrelated.Metadata.KeyId = "other-key"
			unrelated.PublicKey = "00"
			policy.TrustedKeys = append(policy.TrustedKeys, unrelated)
			return "scope-1", signedAt
		}, want: constants.ErrEvidenceTrustNotAssessed},
		{name: "scope not assessed", mutate: func(*compliancev1.ReportSignature, *compliancev1.ComplianceReportTrustPolicy) (string, time.Time) {
			return "scope-2", signedAt
		}, want: constants.ErrEvidenceTrustNotAssessed},
		{name: "signature predates key", mutate: func(*compliancev1.ReportSignature, *compliancev1.ComplianceReportTrustPolicy) (string, time.Time) {
			return "scope-1", metadata.CreatedAt.AsTime().Add(-time.Second)
		}, want: constants.ErrEvidenceTrustNotAssessed},
		{name: "signature follows expiry", mutate: func(*compliancev1.ReportSignature, *compliancev1.ComplianceReportTrustPolicy) (string, time.Time) {
			return "scope-1", metadata.ExpiresAt.AsTime().Add(time.Second)
		}, want: constants.ErrEvidenceTrustNotAssessed},
		{name: "revoked assessed key", mutate: func(_ *compliancev1.ReportSignature, policy *compliancev1.ComplianceReportTrustPolicy) (string, time.Time) {
			policy.TrustedKeys[0].RevokedAt = timestamppb.New(signedAt.Add(-time.Second))
			return "scope-1", signedAt
		}, want: constants.ErrEvidenceTrustNotAssessed},
		{name: "packaged key digest mismatch", mutate: func(_ *compliancev1.ReportSignature, policy *compliancev1.ComplianceReportTrustPolicy) (string, time.Time) {
			policy.TrustedKeys[0].Metadata.PublicKeySha256 = strings.Repeat("0", 64)
			return "scope-1", signedAt
		}, want: constants.ErrEvidenceTrustNotAssessed},
		{name: "signed digest mutation", mutate: func(value *compliancev1.ReportSignature, _ *compliancev1.ComplianceReportTrustPolicy) (string, time.Time) {
			value.SignedSha256 = strings.Repeat("0", 64)
			return "scope-1", signedAt
		}, want: constants.ErrReportSignatureFailed},
		{name: "signature mutation", mutate: func(value *compliancev1.ReportSignature, _ *compliancev1.ComplianceReportTrustPolicy) (string, time.Time) {
			value.Signature = strings.Repeat("0", ed25519.SignatureSize*2)
			return "scope-1", signedAt
		}, want: constants.ErrReportSignatureFailed},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			candidateSignature := proto.Clone(signature).(*compliancev1.ReportSignature)
			candidatePolicy := proto.Clone(policy).(*compliancev1.ComplianceReportTrustPolicy)
			scope, at := tt.mutate(candidateSignature, candidatePolicy)
			if tt.name == "missing trust policy" {
				candidatePolicy = nil
			}
			err := VerifyComplianceReportSignature(candidateSignature, candidatePolicy, scope, at)
			assert.ErrorIs(t, err, tt.want)
		})
	}
}
