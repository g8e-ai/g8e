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

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/services/storage"
	compliancev1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/compliance/v1"
	operatorv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/operator/v1"
)

var (
	releaseWindowStart = time.Date(2026, 9, 28, 2, 58, 19, 0, time.UTC)
	releaseWindowEnd   = time.Date(2026, 9, 28, 2, 58, 52, 999_000_000, time.UTC)
)

func releaseDigest(seed string) string {
	digest := sha256.Sum256([]byte(seed))
	return hex.EncodeToString(digest[:])
}

func validReleaseScopeRequest() ReleaseScopeRequest {
	return ReleaseScopeRequest{
		ScopeID:            "gateway-operational-20260928",
		OrganizationID:     "Example Org",
		DeploymentID:       "deployment-20260928",
		ProductVersion:     "v2.2.2",
		BuildIdentity:      releaseDigest("build"),
		SourceRevision:     strings.Repeat("a", 40),
		GatewayImageDigest: releaseDigest("image"),
		GatewayVersion:     "v2.2.2",
		WindowStart:        releaseWindowStart,
		WindowEnd:          releaseWindowEnd,
	}
}

func TestBuildReleaseAssessmentScope_BindsVersionWindowAndGatewayComponent(t *testing.T) {
	scope, err := BuildReleaseAssessmentScope(validReleaseScopeRequest())
	require.NoError(t, err)

	assert.Equal(t, "2.2.2", scope.GetProductVersion(), "the conventional v prefix is not part of the protected product version")
	assert.Equal(t, releaseWindowStart, scope.GetAssessmentWindowStart().AsTime())
	assert.Equal(t, releaseWindowEnd, scope.GetAssessmentWindowEnd().AsTime())
	assert.Equal(t, releaseWindowEnd, scope.GetAssessmentAsOf().AsTime(), "assessment-as-of is the window end")
	require.Len(t, scope.GetComponentInventory(), 1)
	assert.Equal(t, releaseDigest("image"), scope.GetComponentInventory()[0].GetDigest())
	assert.Equal(t, "2.2.2", scope.GetComponentInventory()[0].GetVersion())
	require.Len(t, scope.GetSourceAdmissions(), 1)
	admission := scope.GetSourceAdmissions()[0]
	assert.Equal(t, scope.GetScopeId(), admission.GetSourceScopeId())
	assert.Equal(t, scope.GetScopeId(), admission.GetRunId())
	assert.Equal(t, constants.ComplianceBundleProfilePublic, admission.GetDisclosureClassification())
	assert.Len(t, scope.GetUnavailableContext(), 4, "uncaptured context is declared unavailable, never omitted")
}

func TestBuildReleaseAssessmentScope_RejectsIncompleteOrInvalidRequests(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*ReleaseScopeRequest)
	}{
		{name: "missing scope id", mutate: func(r *ReleaseScopeRequest) { r.ScopeID = "" }},
		{name: "missing source revision", mutate: func(r *ReleaseScopeRequest) { r.SourceRevision = "" }},
		{name: "missing gateway image digest", mutate: func(r *ReleaseScopeRequest) { r.GatewayImageDigest = "" }},
		{name: "window ends before it starts", mutate: func(r *ReleaseScopeRequest) { r.WindowStart, r.WindowEnd = r.WindowEnd, r.WindowStart }},
		{name: "zero window start", mutate: func(r *ReleaseScopeRequest) { r.WindowStart = time.Time{} }},
		{name: "component digest is not sha256", mutate: func(r *ReleaseScopeRequest) { r.GatewayImageDigest = "not-a-digest" }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request := validReleaseScopeRequest()
			test.mutate(&request)
			_, err := BuildReleaseAssessmentScope(request)
			require.Error(t, err)
		})
	}
}

func releaseKeyID(t *testing.T) string {
	t.Helper()
	publicKey, _, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	return hex.EncodeToString(publicKey)
}

func TestCollectOperationalSignerKeys_ReturnsDistinctSortedKeysFromEverySignedSurface(t *testing.T) {
	receiptSigner, persistenceSigner, stageSigner, auditor := "cc", "aa", "dd", "bb"
	receipt, err := compliancev1.MarshalCanonical(&operatorv1.ActionReceipt{
		TransactionId: "tx-1",
		SignerKeyId:   receiptSigner,
		FinalPersistenceAttestation: &operatorv1.ReceiptPersistenceAttestation{
			SignerKeyId: persistenceSigner,
		},
		DeterministicStageEvidence: []*operatorv1.DeterministicStageEvidence{{SignerKeyId: stageSigner}},
	})
	require.NoError(t, err)
	duplicateReceipt, err := compliancev1.MarshalCanonical(&operatorv1.ActionReceipt{TransactionId: "tx-2", SignerKeyId: receiptSigner})
	require.NoError(t, err)
	commitment, err := compliancev1.MarshalCanonical(&operatorv1.CommitmentAttestation{AuditorKeyId: auditor})
	require.NoError(t, err)

	keys, err := CollectOperationalSignerKeys(&storage.OperationalEvidenceSnapshot{
		Receipts: []storage.OperationalReceiptSource{
			{TransactionID: "tx-1", Body: receipt},
			{TransactionID: "tx-2", Body: duplicateReceipt},
			{TransactionID: "tx-3"},
		},
		Commitments: []storage.OperationalCommitmentSource{{TransactionID: "tx-1", Body: commitment}, {TransactionID: "tx-4"}},
	})
	require.NoError(t, err)
	assert.Equal(t, []string{"aa", "bb", "cc", "dd"}, keys)
}

func TestCollectOperationalSignerKeys_RejectsMissingSnapshotAndMalformedBodies(t *testing.T) {
	_, err := CollectOperationalSignerKeys(nil)
	require.ErrorIs(t, err, constants.ErrValidationFailed)

	_, err = CollectOperationalSignerKeys(&storage.OperationalEvidenceSnapshot{
		Receipts: []storage.OperationalReceiptSource{{TransactionID: "tx-1", Body: []byte("not canonical json")}},
	})
	require.Error(t, err)
}

func TestGenerateReportSigningKey_ProducesIdentityThatSignsAndVerifiesUnderReleaseTrust(t *testing.T) {
	metadata, privateKeyHex, err := GenerateReportSigningKey(rand.Reader, "v2.2.2-engineering-report-key", releaseWindowStart, constants.ComplianceReleaseNewKeyLifetime)
	require.NoError(t, err)
	privateKey, err := hex.DecodeString(privateKeyHex)
	require.NoError(t, err)
	identity, err := NewComplianceReportSigningIdentity(metadata, ed25519.PrivateKey(privateKey))
	require.NoError(t, err)

	policy, err := BuildReleaseReportTrustPolicy(ReleaseTrustRequest{
		PolicyID:         "v2.2.2-engineering-report-trust-gateway",
		AssessmentID:     "v2.2.2-engineering-acceptance-gateway",
		AssessorIdentity: constants.ComplianceReleaseDefaultAssessor,
		ScopeID:          "gateway-operational-20260928",
		AssessedAt:       releaseWindowEnd,
	}, identity)
	require.NoError(t, err)

	signature, err := identity.SignSHA256(releaseDigest("bundle"))
	require.NoError(t, err)
	signedAt := releaseWindowEnd.Add(time.Hour)
	require.NoError(t, VerifyComplianceReportSignature(signature, policy, "gateway-operational-20260928", signedAt))
	require.ErrorIs(t, VerifyComplianceReportSignature(signature, policy, "another-scope", signedAt), constants.ErrEvidenceTrustNotAssessed, "trust is bound to the release scope only")
	require.ErrorIs(t, VerifyComplianceReportSignature(signature, policy, "gateway-operational-20260928", releaseWindowStart.Add(-time.Hour)), constants.ErrEvidenceTrustNotAssessed, "a signature before the key existed is not trusted")
}

func TestGenerateReportSigningKey_RejectsIncompleteRequest(t *testing.T) {
	_, _, err := GenerateReportSigningKey(rand.Reader, "", releaseWindowStart, time.Hour)
	require.ErrorIs(t, err, constants.ErrValidationFailed)
	_, _, err = GenerateReportSigningKey(rand.Reader, "key", releaseWindowStart, 0)
	require.ErrorIs(t, err, constants.ErrValidationFailed)
}

func TestBuildReleaseEvidenceTrustPolicy_BindsEachSourceSignerToTheReleaseScope(t *testing.T) {
	first, second := releaseKeyID(t), releaseKeyID(t)
	request := ReleaseTrustRequest{
		PolicyID:         "v2.2.2-gateway-evidence-trust",
		AssessmentID:     "v2.2.2-engineering-acceptance-gateway",
		AssessorIdentity: constants.ComplianceReleaseDefaultAssessor,
		ScopeID:          "gateway-operational-20260928",
		AssessedAt:       releaseWindowEnd,
	}

	policy, err := BuildReleaseEvidenceTrustPolicy(request, []string{first, second})
	require.NoError(t, err)
	require.Len(t, policy.GetTrustedKeys(), 2)
	for _, key := range policy.GetTrustedKeys() {
		publicKey, decodeErr := hex.DecodeString(key.GetPublicKey())
		require.NoError(t, decodeErr)
		digest := sha256.Sum256(publicKey)
		assert.Equal(t, key.GetKeyId(), key.GetPublicKey(), "the Gateway signs with its public key as the key id")
		assert.Equal(t, hex.EncodeToString(digest[:]), key.GetPublicKeySha256())
		assert.Equal(t, []string{"gateway-operational-20260928"}, key.GetAllowedScopeRefs())
		assert.True(t, key.GetValidFrom().AsTime().Before(releaseWindowStart) && key.GetValidUntil().AsTime().After(releaseWindowEnd), "validity must cover the assessment window")
	}
}

func TestBuildReleaseEvidenceTrustPolicy_RejectsUnusableInputs(t *testing.T) {
	request := ReleaseTrustRequest{PolicyID: "p", AssessmentID: "a", AssessorIdentity: "i", ScopeID: "s", AssessedAt: releaseWindowEnd}
	_, err := BuildReleaseEvidenceTrustPolicy(request, nil)
	require.ErrorIs(t, err, constants.ErrEvidenceTrustNotAssessed, "a window with no signed evidence cannot yield trust")
	_, err = BuildReleaseEvidenceTrustPolicy(request, []string{"not-hex"})
	require.ErrorIs(t, err, constants.ErrEvidenceTrustNotAssessed)
	_, err = BuildReleaseEvidenceTrustPolicy(request, []string{strings.ToUpper(releaseKeyID(t))})
	require.ErrorIs(t, err, constants.ErrEvidenceTrustNotAssessed, "key ids must be lowercase hex")
	_, err = BuildReleaseEvidenceTrustPolicy(ReleaseTrustRequest{}, []string{releaseKeyID(t)})
	require.ErrorIs(t, err, constants.ErrValidationFailed)
}
