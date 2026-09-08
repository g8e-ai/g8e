// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package report

import (
	"context"
	"crypto/ed25519"
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

type bundleArtifactReaderStub struct {
	bodies map[string][]byte
	err    error
}

func (r *bundleArtifactReaderStub) ReadFile(_ context.Context, bundlePath string) ([]byte, error) {
	if r.err != nil {
		return nil, r.err
	}
	body, ok := r.bodies[bundlePath]
	if !ok {
		return nil, constants.ErrNotFound
	}
	return append([]byte(nil), body...), nil
}

func signedBundleVerificationFixture(t *testing.T) (*compliancev1.ComplianceReportBundle, *bundleArtifactReaderStub, *compliancev1.ComplianceReportTrustPolicy, time.Time) {
	t.Helper()
	request, _ := bundleAssemblyFixture(t)
	request.Analysis = rendererTestAnalysis()
	request.Profiles[0].AnalysisRef = request.Analysis.GetAnalysisId()
	analysisBody, err := compliancev1.MarshalCanonical(request.Analysis)
	require.NoError(t, err)
	request.RenderedFormats[0].Body = analysisBody
	result, err := AssembleBundle(request)
	require.NoError(t, err)
	identity := bundleSigningIdentityFixture(t)
	require.NoError(t, SignBundle(result, identity))
	bodies := make(map[string][]byte, len(result.ArtifactBodies))
	for _, artifact := range result.ArtifactBodies {
		bodies[artifact.BundlePath] = append([]byte(nil), artifact.Body...)
	}
	reader := &bundleArtifactReaderStub{bodies: bodies}
	publicKey := identity.privateKey.Public().(ed25519.PublicKey)
	policy := &compliancev1.ComplianceReportTrustPolicy{
		PolicyId:      "policy-1",
		PolicyVersion: "1.0.0",
		TrustedKeys: []*compliancev1.ComplianceReportTrustedKey{{
			Metadata:         proto.Clone(identity.metadata).(*compliancev1.ComplianceReportSigningKeyMetadata),
			PublicKey:        hex.EncodeToString(publicKey),
			AssessmentId:     "assessment-1",
			AssessorIdentity: "assessor-1",
			AssessedAt:       timestamppb.New(request.GeneratedAt),
			AllowedScopeRefs: []string{request.ScopeRef},
		}},
	}
	return result.Bundle, reader, policy, request.GeneratedAt.Add(time.Hour)
}

func TestVerifyComplianceReportBundle_AcceptsCompleteSignedBundleOffline(t *testing.T) {
	bundle, reader, policy, verifiedAt := signedBundleVerificationFixture(t)

	report, err := VerifyComplianceReportBundle(context.Background(), BundleVerificationRequest{
		Bundle:      bundle,
		Reader:      reader,
		TrustPolicy: policy,
		VerifiedAt:  verifiedAt,
	})

	require.NoError(t, err)
	require.NotNil(t, report)
	assert.True(t, report.GetValid())
	assert.Empty(t, report.GetFailures())
	assert.Equal(t, bundle.GetManifest().GetReportId(), report.GetReportId())
	assert.Equal(t, constants.ComplianceBundleVerifierID, report.GetVerifierId())
	assert.Equal(t, constants.ComplianceBundleVerifierVersion, report.GetVerifierVersion())
	assert.Equal(t, bundle.GetChecksumRoot(), report.GetReproducedChecksumRoot())
}

func TestVerifyComplianceReportBundle_ReportsArtifactAndSignatureMutations(t *testing.T) {
	tests := []struct {
		name        string
		mutate      func(*compliancev1.ComplianceReportBundle, *bundleArtifactReaderStub, *compliancev1.ComplianceReportTrustPolicy)
		failureCode error
	}{
		{
			name: "artifact body digest mismatch",
			mutate: func(_ *compliancev1.ComplianceReportBundle, reader *bundleArtifactReaderStub, _ *compliancev1.ComplianceReportTrustPolicy) {
				reader.bodies[constants.ComplianceBundleAnalysisPath] = []byte(`{"tampered":true}`)
			},
			failureCode: constants.ErrChecksumMismatch,
		},
		{
			name: "missing artifact body",
			mutate: func(_ *compliancev1.ComplianceReportBundle, reader *bundleArtifactReaderStub, _ *compliancev1.ComplianceReportTrustPolicy) {
				delete(reader.bodies, constants.ComplianceBundleAnalysisPath)
			},
			failureCode: constants.ErrBundleArtifactMissing,
		},
		{
			name: "descriptor digest mutation",
			mutate: func(bundle *compliancev1.ComplianceReportBundle, _ *bundleArtifactReaderStub, _ *compliancev1.ComplianceReportTrustPolicy) {
				bundle.Artifacts[0].Sha256 = strings.Repeat("0", 64)
			},
			failureCode: constants.ErrChecksumMismatch,
		},
		{
			name: "checksum root mutation",
			mutate: func(bundle *compliancev1.ComplianceReportBundle, _ *bundleArtifactReaderStub, _ *compliancev1.ComplianceReportTrustPolicy) {
				bundle.ChecksumRoot = strings.Repeat("0", 64)
			},
			failureCode: constants.ErrChecksumMismatch,
		},
		{
			name: "manifest content mutation",
			mutate: func(bundle *compliancev1.ComplianceReportBundle, _ *bundleArtifactReaderStub, _ *compliancev1.ComplianceReportTrustPolicy) {
				bundle.Manifest.ReportId = "tampered-report"
			},
			failureCode: constants.ErrChecksumMismatch,
		},
		{
			name: "checksum signature mutation",
			mutate: func(bundle *compliancev1.ComplianceReportBundle, _ *bundleArtifactReaderStub, _ *compliancev1.ComplianceReportTrustPolicy) {
				bundle.ChecksumRootSignature.Signature = strings.Repeat("0", ed25519.SignatureSize*2)
			},
			failureCode: constants.ErrReportSignatureFailed,
		},
		{
			name: "unassessed packaged signer",
			mutate: func(_ *compliancev1.ComplianceReportBundle, _ *bundleArtifactReaderStub, policy *compliancev1.ComplianceReportTrustPolicy) {
				policy.TrustedKeys = nil
			},
			failureCode: constants.ErrEvidenceTrustNotAssessed,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			bundle, reader, policy, verifiedAt := signedBundleVerificationFixture(t)
			test.mutate(bundle, reader, policy)

			report, err := VerifyComplianceReportBundle(context.Background(), BundleVerificationRequest{
				Bundle:      bundle,
				Reader:      reader,
				TrustPolicy: policy,
				VerifiedAt:  verifiedAt,
			})

			require.NoError(t, err)
			require.NotNil(t, report)
			assert.False(t, report.GetValid())
			assert.NotEmpty(t, report.GetFailures())
			assert.Contains(t, failureCodes(report), test.failureCode.Error())
		})
	}
}

func TestVerifyComplianceReportBundle_RejectsInvalidVerifierConfiguration(t *testing.T) {
	bundle, reader, policy, verifiedAt := signedBundleVerificationFixture(t)
	tests := []struct {
		name    string
		request BundleVerificationRequest
	}{
		{name: "missing bundle", request: BundleVerificationRequest{Reader: reader, TrustPolicy: policy, VerifiedAt: verifiedAt}},
		{name: "missing reader", request: BundleVerificationRequest{Bundle: bundle, TrustPolicy: policy, VerifiedAt: verifiedAt}},
		{name: "missing trust policy", request: BundleVerificationRequest{Bundle: bundle, Reader: reader, VerifiedAt: verifiedAt}},
		{name: "missing verification time", request: BundleVerificationRequest{Bundle: bundle, Reader: reader, TrustPolicy: policy}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			report, err := VerifyComplianceReportBundle(context.Background(), test.request)
			require.Error(t, err)
			assert.ErrorIs(t, err, constants.ErrReportVerificationFailed)
			assert.Nil(t, report)
		})
	}
}

func TestVerifyComplianceReportBundle_PreservesCancellation(t *testing.T) {
	bundle, reader, policy, verifiedAt := signedBundleVerificationFixture(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	report, err := VerifyComplianceReportBundle(ctx, BundleVerificationRequest{Bundle: bundle, Reader: reader, TrustPolicy: policy, VerifiedAt: verifiedAt})

	assert.ErrorIs(t, err, context.Canceled)
	assert.Nil(t, report)
}

func failureCodes(report *compliancev1.ComplianceVerificationReport) []string {
	codes := make([]string, 0, len(report.GetFailures()))
	for _, failure := range report.GetFailures() {
		codes = append(codes, failure.GetCode())
	}
	return codes
}
