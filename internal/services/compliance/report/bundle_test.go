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
	"path"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/services/compliance/catalog"
	compliancev1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/compliance/v1"
)

func bundleAssemblyFixture(t *testing.T) (BundleAssemblyRequest, *compliancev1.FrameworkCatalog) {
	t.Helper()
	analysis := &compliancev1.ComplianceAnalysis{
		AnalysisId:            "analysis:sha256:" + strings.Repeat("a", 64),
		AnalysisSchemaVersion: constants.AnalysisSchemaVersion,
		ScopeRef:              "scope-1",
		GeneratedAt:           timestamppb.Now(),
		GeneratorIdentity:     constants.AnalysisBuilderID,
		GeneratorVersion:      constants.AnalysisBuilderVersion,
		EvidenceGraphValid:    true,
	}
	analysisBytes, err := compliancev1.MarshalCanonical(analysis)
	require.NoError(t, err)
	profile := &compliancev1.FrameworkProfile{
		ProfileId:      "profile:sha256:" + strings.Repeat("b", 64),
		FrameworkRef:   &compliancev1.VersionedReference{Id: "fedramp-20x", Version: "CR26-2026-06-24"},
		ProfileVersion: constants.FrameworkProfileVersion,
		GeneratedAt:    timestamppb.Now(),
		AnalysisRef:    analysis.GetAnalysisId(),
	}
	rendered := RenderedFormat{
		Format:     FormatJSON,
		MediaType:  constants.MediaTypeJSON,
		BundlePath: constants.ComplianceBundleJSONPath,
		Body:       analysisBytes,
	}
	frameworks := &compliancev1.FrameworkCatalog{
		CatalogId:      "frameworks",
		CatalogVersion: "1.0.0",
		Sha256:         strings.Repeat("0", 64),
		Frameworks: []*compliancev1.FrameworkDefinition{{
			FrameworkId:      "fedramp-20x",
			FrameworkVersion: "CR26-2026-06-24",
		}},
	}
	request := BundleAssemblyRequest{
		Profile:         ProfilePublic,
		Analysis:        analysis,
		Profiles:        []*compliancev1.FrameworkProfile{profile},
		RenderedFormats: []RenderedFormat{rendered},
		ScopeRef:        "scope-1",
		ReportID:        "report-1",
		GeneratedAt:     time.Unix(1_700_000_000, 0).UTC(),
		FrameworkRefs: []*compliancev1.VersionedReference{
			{Id: "fedramp-20x", Version: "CR26-2026-06-24"},
		},
		AssertionCatalogRef: path.Join(constants.ComplianceBundleAssertionsDirname, constants.ComplianceBundleAssertionCatalogFilename),
		CrosswalkRefs:       []string{path.Join(constants.ComplianceBundleCrosswalksDirname, constants.ComplianceBundleCrosswalkFilename)},
		AssessmentRefs:      []string{path.Join(constants.ComplianceBundleAssessmentsDirname, constants.ComplianceBundleAssertionAssessmentsFilename)},
		EvidenceIndexRef:    path.Join(constants.ComplianceBundleEvidenceDirname, constants.ComplianceBundleEvidenceIndexFilename),
	}
	return request, frameworks
}

func bundleSigningIdentityFixture(t *testing.T) *ComplianceReportSigningIdentity {
	t.Helper()
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	publicKeyDigest := sha256.Sum256(publicKey)
	createdAt := time.Unix(1_700_000_000, 0).UTC()
	metadata := &compliancev1.ComplianceReportSigningKeyMetadata{
		KeyId:           "report-key-1",
		Algorithm:       constants.ComplianceReportSignatureAlgorithm,
		Purpose:         constants.ComplianceReportSigningPurpose,
		PublicKeySha256: hex.EncodeToString(publicKeyDigest[:]),
		CreatedAt:       timestamppb.New(createdAt),
		ExpiresAt:       timestamppb.New(createdAt.Add(24 * time.Hour)),
	}
	identity, err := NewComplianceReportSigningIdentity(metadata, privateKey)
	require.NoError(t, err)
	return identity
}

func TestParseBundleProfile_AcceptsPublicAndRestricted(t *testing.T) {
	pub, err := ParseBundleProfile(constants.ComplianceBundleProfilePublic)
	require.NoError(t, err)
	assert.Equal(t, ProfilePublic, pub)

	restricted, err := ParseBundleProfile(constants.ComplianceBundleProfileRestricted)
	require.NoError(t, err)
	assert.Equal(t, ProfileRestricted, restricted)
}

func TestParseBundleProfile_RejectsUnsupportedProfile(t *testing.T) {
	_, err := ParseBundleProfile("confidential")
	assert.ErrorIs(t, err, constants.ErrBundleProfileUnsupported)
}

func TestAssembleBundle_PublicProfile_ProducesChecksummedArtifactsAndManifest(t *testing.T) {
	request, frameworks := bundleAssemblyFixture(t)
	result, err := AssembleBundle(request)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.NotNil(t, result.Bundle)

	manifest := result.Bundle.GetManifest()
	assert.Equal(t, request.ReportID, manifest.GetReportId())
	assert.Equal(t, constants.ComplianceBundleSchemaVersion, manifest.GetReportSchemaVersion())
	assert.Equal(t, constants.ComplianceBundleAssemblerID, manifest.GetGeneratorIdentity())
	assert.Equal(t, constants.ComplianceBundleAssemblerVersion, manifest.GetGeneratorVersion())
	assert.Equal(t, constants.ComplianceBundleProfilePublic, manifest.GetBundleProfile())
	assert.NotEmpty(t, manifest.GetChecksumRoot())
	assert.NotEmpty(t, manifest.GetManifestSha256())
	assert.Nil(t, manifest.GetSignature())

	assert.NotEmpty(t, result.Bundle.GetChecksumRoot())
	assert.Nil(t, result.Bundle.GetChecksumRootSignature())
	assert.Equal(t, manifest.GetChecksumRoot(), result.Bundle.GetChecksumRoot())

	assert.NotEmpty(t, result.Bundle.GetArtifacts())
	for _, artifact := range result.Bundle.GetArtifacts() {
		assert.Equal(t, constants.ComplianceBundleProfilePublic, artifact.GetProfile())
		assert.Nil(t, artifact.GetEncryption())
		assert.Greater(t, artifact.GetByteLength(), int64(0))
		assert.NotEmpty(t, artifact.GetSha256())
	}

	assert.Len(t, result.Bundle.GetRenderedFormats(), 1)
	assert.Equal(t, string(FormatJSON), result.Bundle.GetRenderedFormats()[0].GetFormat())

	require.NoError(t, SignBundle(result, bundleSigningIdentityFixture(t)))
	assert.NoError(t, catalog.ValidateComplianceReportBundle(result.Bundle, frameworks))
}

func TestAssembleBundle_RestrictedProfile_IncludesRestrictedArtifactsWithEncryption(t *testing.T) {
	request, _ := bundleAssemblyFixture(t)
	request.Profile = ProfileRestricted
	encryption := &compliancev1.EvidenceEncryptionMetadata{
		Algorithm:                   "aes-256-gcm",
		KeyId:                       "key-1",
		AuthorizationScope:          "restricted-evidence",
		PlaintextSha256:             strings.Repeat("a", 64),
		AuthenticatedMetadataSha256: strings.Repeat("b", 64),
	}
	request.RestrictedArtifacts = []RestrictedArtifact{{
		BundlePath: "restricted/evidence.json",
		Body:       []byte(`{"encrypted":"ciphertext"}`),
		MediaType:  constants.MediaTypeJSON,
		Encryption: encryption,
	}}
	result, err := AssembleBundle(request)
	require.NoError(t, err)

	var restrictedFound bool
	for _, artifact := range result.Bundle.GetArtifacts() {
		if artifact.GetProfile() == constants.ComplianceBundleProfileRestricted {
			restrictedFound = true
			assert.NotNil(t, artifact.GetEncryption())
			assert.Equal(t, encryption.GetAlgorithm(), artifact.GetEncryption().GetAlgorithm())
			assert.Equal(t, encryption.GetKeyId(), artifact.GetEncryption().GetKeyId())
		}
	}
	assert.True(t, restrictedFound, "restricted profile bundle must include restricted artifacts")
}

func TestAssembleBundle_RestrictedProfile_RejectsRestrictedArtifactWithoutEncryption(t *testing.T) {
	request, _ := bundleAssemblyFixture(t)
	request.Profile = ProfileRestricted
	request.RestrictedArtifacts = []RestrictedArtifact{{
		BundlePath: "restricted/evidence.json",
		Body:       []byte(`{"data":"value"}`),
		MediaType:  constants.MediaTypeJSON,
	}}
	_, err := AssembleBundle(request)
	assert.ErrorIs(t, err, constants.ErrBundleAssemblyFailed)
}

func TestAssembleBundle_PublicProfile_OmitsRestrictedArtifacts(t *testing.T) {
	request, _ := bundleAssemblyFixture(t)
	request.Profile = ProfilePublic
	request.RestrictedArtifacts = []RestrictedArtifact{{
		BundlePath: "restricted/evidence.json",
		Body:       []byte(`{"encrypted":"ciphertext"}`),
		MediaType:  constants.MediaTypeJSON,
		Encryption: &compliancev1.EvidenceEncryptionMetadata{
			Algorithm:                   "aes-256-gcm",
			KeyId:                       "key-1",
			AuthorizationScope:          "restricted",
			PlaintextSha256:             strings.Repeat("a", 64),
			AuthenticatedMetadataSha256: strings.Repeat("b", 64),
		},
	}}
	result, err := AssembleBundle(request)
	require.NoError(t, err)
	for _, artifact := range result.Bundle.GetArtifacts() {
		assert.Equal(t, constants.ComplianceBundleProfilePublic, artifact.GetProfile(),
			"public profile bundle must not include restricted artifacts")
	}
}

func TestAssembleBundle_ChecksumRootIsDeterministic(t *testing.T) {
	request, _ := bundleAssemblyFixture(t)
	first, err := AssembleBundle(request)
	require.NoError(t, err)
	second, err := AssembleBundle(request)
	require.NoError(t, err)
	assert.Equal(t, first.Bundle.GetChecksumRoot(), second.Bundle.GetChecksumRoot())
	assert.Equal(t, first.Bundle.GetManifest().GetManifestSha256(), second.Bundle.GetManifest().GetManifestSha256())
}

func TestAssembleBundle_ChecksumRootChangesOnArtifactMutation(t *testing.T) {
	request, _ := bundleAssemblyFixture(t)
	first, err := AssembleBundle(request)
	require.NoError(t, err)
	mutated := request
	mutated.Analysis = proto.Clone(request.Analysis).(*compliancev1.ComplianceAnalysis)
	mutated.Analysis.GeneratorVersion = "mutated"
	mutated.RenderedFormats = append([]RenderedFormat(nil), request.RenderedFormats...)
	mutated.RenderedFormats[0].Body, err = compliancev1.MarshalCanonical(mutated.Analysis)
	require.NoError(t, err)
	second, err := AssembleBundle(mutated)
	require.NoError(t, err)
	assert.NotEqual(t, first.Bundle.GetChecksumRoot(), second.Bundle.GetChecksumRoot())
}

func TestAssembleBundle_RejectsInvalidRequests(t *testing.T) {
	request, _ := bundleAssemblyFixture(t)
	tests := []struct {
		name    string
		mutate  func(BundleAssemblyRequest) BundleAssemblyRequest
		wantErr error
	}{
		{name: "unsupported profile", mutate: func(r BundleAssemblyRequest) BundleAssemblyRequest { r.Profile = "confidential"; return r }, wantErr: constants.ErrBundleProfileUnsupported},
		{name: "missing analysis", mutate: func(r BundleAssemblyRequest) BundleAssemblyRequest { r.Analysis = nil; return r }, wantErr: constants.ErrBundleAssemblyFailed},
		{name: "missing scope", mutate: func(r BundleAssemblyRequest) BundleAssemblyRequest { r.ScopeRef = ""; return r }, wantErr: constants.ErrBundleAssemblyFailed},
		{name: "missing report ID", mutate: func(r BundleAssemblyRequest) BundleAssemblyRequest { r.ReportID = ""; return r }, wantErr: constants.ErrBundleAssemblyFailed},
		{name: "zero generation time", mutate: func(r BundleAssemblyRequest) BundleAssemblyRequest { r.GeneratedAt = time.Time{}; return r }, wantErr: constants.ErrBundleAssemblyFailed},
		{name: "missing framework refs", mutate: func(r BundleAssemblyRequest) BundleAssemblyRequest { r.FrameworkRefs = nil; return r }, wantErr: constants.ErrBundleAssemblyFailed},
		{name: "incomplete framework ref", mutate: func(r BundleAssemblyRequest) BundleAssemblyRequest {
			r.FrameworkRefs = []*compliancev1.VersionedReference{nil}
			return r
		}, wantErr: constants.ErrBundleAssemblyFailed},
		{name: "duplicate framework refs", mutate: func(r BundleAssemblyRequest) BundleAssemblyRequest {
			r.FrameworkRefs = append(r.FrameworkRefs, proto.Clone(r.FrameworkRefs[0]).(*compliancev1.VersionedReference))
			return r
		}, wantErr: constants.ErrBundleAssemblyFailed},
		{name: "unsafe scope ref", mutate: func(r BundleAssemblyRequest) BundleAssemblyRequest { r.ScopeRef = constants.PathParentDir; return r }, wantErr: constants.ErrBundleAssemblyFailed},
		{name: "duplicate crosswalk refs", mutate: func(r BundleAssemblyRequest) BundleAssemblyRequest {
			r.CrosswalkRefs = append(r.CrosswalkRefs, r.CrosswalkRefs[0])
			return r
		}, wantErr: constants.ErrBundleAssemblyFailed},
		{name: "missing rendered formats", mutate: func(r BundleAssemblyRequest) BundleAssemblyRequest { r.RenderedFormats = nil; return r }, wantErr: constants.ErrBundleAssemblyFailed},
		{name: "missing profiles", mutate: func(r BundleAssemblyRequest) BundleAssemblyRequest { r.Profiles = nil; return r }, wantErr: constants.ErrBundleAssemblyFailed},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := AssembleBundle(tt.mutate(request))
			assert.ErrorIs(t, err, tt.wantErr)
		})
	}
}

func TestAssembleBundle_RejectsEmptyArtifactBody(t *testing.T) {
	request, _ := bundleAssemblyFixture(t)
	request.RenderedFormats[0].Body = nil
	_, err := AssembleBundle(request)
	assert.ErrorIs(t, err, constants.ErrBundleArtifactMissing)
}

func TestAssembleBundle_RejectsRenderedJSONThatDiffersFromCanonicalAnalysis(t *testing.T) {
	request, _ := bundleAssemblyFixture(t)
	request.RenderedFormats[0].Body = []byte(`{"analysis_id":"mutated"}`)
	_, err := AssembleBundle(request)
	assert.ErrorIs(t, err, constants.ErrBundleAssemblyFailed)
}

func TestAssembleBundle_RejectsEmptyBundlePath(t *testing.T) {
	request, _ := bundleAssemblyFixture(t)
	request.RenderedFormats[0].BundlePath = ""
	_, err := AssembleBundle(request)
	assert.ErrorIs(t, err, constants.ErrBundleAssemblyFailed)
}

func TestAssembleBundle_RejectsDuplicateBundlePaths(t *testing.T) {
	request, _ := bundleAssemblyFixture(t)
	request.RenderedFormats = append(request.RenderedFormats, RenderedFormat{
		Format:     FormatMarkdown,
		MediaType:  constants.MediaTypeMarkdown,
		BundlePath: request.RenderedFormats[0].BundlePath,
		Body:       []byte("# duplicate path"),
	})
	_, err := AssembleBundle(request)
	assert.ErrorIs(t, err, constants.ErrBundleChecksumRootFailed)
}

func TestSignBundle_SignsChecksumRootAndManifestRoot(t *testing.T) {
	request, _ := bundleAssemblyFixture(t)
	result, err := AssembleBundle(request)
	require.NoError(t, err)
	identity := bundleSigningIdentityFixture(t)

	err = SignBundle(result, identity)
	require.NoError(t, err)

	manifest := result.Bundle.GetManifest()
	require.NotNil(t, manifest.GetSignature())
	assert.Equal(t, identity.metadata.GetKeyId(), manifest.GetSignature().GetKeyId())
	assert.Equal(t, manifest.GetManifestSha256(), manifest.GetSignature().GetSignedSha256())

	require.NotNil(t, result.Bundle.GetChecksumRootSignature())
	assert.Equal(t, identity.metadata.GetKeyId(), result.Bundle.GetChecksumRootSignature().GetKeyId())
	assert.Equal(t, result.Bundle.GetChecksumRoot(), result.Bundle.GetChecksumRootSignature().GetSignedSha256())
}

func TestSignBundle_RejectsNilIdentity(t *testing.T) {
	request, _ := bundleAssemblyFixture(t)
	result, err := AssembleBundle(request)
	require.NoError(t, err)
	err = SignBundle(result, nil)
	assert.ErrorIs(t, err, constants.ErrReportSignatureFailed)
}

func TestSignBundle_RejectsNilResult(t *testing.T) {
	identity := bundleSigningIdentityFixture(t)
	err := SignBundle(nil, identity)
	assert.ErrorIs(t, err, constants.ErrBundleAssemblyFailed)
}

func TestSignBundle_ManifestSignatureBindsManifestContent(t *testing.T) {
	request, _ := bundleAssemblyFixture(t)
	result, err := AssembleBundle(request)
	require.NoError(t, err)
	identity := bundleSigningIdentityFixture(t)
	require.NoError(t, SignBundle(result, identity))

	manifestBytes, err := canonicalManifestBytes(result.Bundle.GetManifest())
	require.NoError(t, err)
	digest := sha256.Sum256(manifestBytes)
	assert.Equal(t, hex.EncodeToString(digest[:]), result.Bundle.GetManifest().GetManifestSha256())
	assert.Equal(t, hex.EncodeToString(digest[:]), result.Bundle.GetManifest().GetSignature().GetSignedSha256())
}

func TestSignBundle_SignaturesVerifyAgainstAssessedTrustPolicy(t *testing.T) {
	request, _ := bundleAssemblyFixture(t)
	result, err := AssembleBundle(request)
	require.NoError(t, err)
	identity := bundleSigningIdentityFixture(t)
	require.NoError(t, SignBundle(result, identity))

	policy := &compliancev1.ComplianceReportTrustPolicy{
		PolicyId:      "policy-1",
		PolicyVersion: "1.0.0",
		TrustedKeys: []*compliancev1.ComplianceReportTrustedKey{{
			Metadata:         proto.Clone(identity.metadata).(*compliancev1.ComplianceReportSigningKeyMetadata),
			PublicKey:        hex.EncodeToString(identity.privateKey.Public().(ed25519.PublicKey)),
			AssessmentId:     "assessment-1",
			AssessorIdentity: "assessor-1",
			AssessedAt:       timestamppb.New(time.Unix(1_700_000_000, 0).UTC()),
			AllowedScopeRefs: []string{request.ScopeRef},
		}},
	}
	signedAt := request.GeneratedAt
	assert.NoError(t, VerifyComplianceReportSignature(result.Bundle.GetManifest().GetSignature(), policy, request.ScopeRef, signedAt))
	assert.NoError(t, VerifyComplianceReportSignature(result.Bundle.GetChecksumRootSignature(), policy, request.ScopeRef, signedAt))
}

func TestSignBundle_ManifestSignatureFailsOnContentMutation(t *testing.T) {
	request, _ := bundleAssemblyFixture(t)
	result, err := AssembleBundle(request)
	require.NoError(t, err)
	identity := bundleSigningIdentityFixture(t)
	require.NoError(t, SignBundle(result, identity))

	mutated := proto.Clone(result.Bundle.GetManifest()).(*compliancev1.ComplianceReportManifest)
	mutated.ReportId = "tampered-report-id"
	mutatedBytes, err := canonicalManifestBytes(mutated)
	require.NoError(t, err)
	mutatedDigest := sha256.Sum256(mutatedBytes)
	assert.NotEqual(t, hex.EncodeToString(mutatedDigest[:]), result.Bundle.GetManifest().GetManifestSha256(),
		"manifest content mutation must change the manifest SHA-256")
}

func TestValidateComplianceReportBundle_RejectsUnsignedBundle(t *testing.T) {
	request, frameworks := bundleAssemblyFixture(t)
	result, err := AssembleBundle(request)
	require.NoError(t, err)
	err = catalog.ValidateComplianceReportBundle(result.Bundle, frameworks)
	assert.ErrorIs(t, err, constants.ErrReportSignatureFailed)
}

func TestValidateComplianceReportBundle_RejectsChecksumRootSignatureMismatch(t *testing.T) {
	request, frameworks := bundleAssemblyFixture(t)
	result, err := AssembleBundle(request)
	require.NoError(t, err)
	identity := bundleSigningIdentityFixture(t)
	require.NoError(t, SignBundle(result, identity))

	tampered := proto.Clone(result.Bundle).(*compliancev1.ComplianceReportBundle)
	tampered.ChecksumRootSignature.SignedSha256 = strings.Repeat("0", 64)
	err = catalog.ValidateComplianceReportBundle(tampered, frameworks)
	assert.ErrorIs(t, err, constants.ErrReportSignatureFailed)
}

func TestValidateComplianceReportBundle_RejectsManifestSignatureMismatch(t *testing.T) {
	request, frameworks := bundleAssemblyFixture(t)
	result, err := AssembleBundle(request)
	require.NoError(t, err)
	identity := bundleSigningIdentityFixture(t)
	require.NoError(t, SignBundle(result, identity))

	tampered := proto.Clone(result.Bundle).(*compliancev1.ComplianceReportBundle)
	tampered.Manifest.Signature.SignedSha256 = strings.Repeat("0", 64)
	err = catalog.ValidateComplianceReportBundle(tampered, frameworks)
	assert.ErrorIs(t, err, constants.ErrReportSignatureFailed)
}

func TestValidateComplianceReportBundle_RejectsPublicArtifactWithEncryption(t *testing.T) {
	request, frameworks := bundleAssemblyFixture(t)
	result, err := AssembleBundle(request)
	require.NoError(t, err)
	identity := bundleSigningIdentityFixture(t)
	require.NoError(t, SignBundle(result, identity))

	tampered := proto.Clone(result.Bundle).(*compliancev1.ComplianceReportBundle)
	for _, artifact := range tampered.Artifacts {
		if artifact.GetProfile() == constants.ComplianceBundleProfilePublic {
			artifact.Encryption = &compliancev1.EvidenceEncryptionMetadata{
				Algorithm:                   "aes-256-gcm",
				KeyId:                       "key-1",
				AuthorizationScope:          "scope-1",
				PlaintextSha256:             strings.Repeat("a", 64),
				AuthenticatedMetadataSha256: strings.Repeat("b", 64),
			}
			break
		}
	}
	err = catalog.ValidateComplianceReportBundle(tampered, frameworks)
	assert.ErrorIs(t, err, constants.ErrEvidenceEncryptionInvalid)
}

func TestValidateComplianceReportBundle_RejectsDuplicateArtifactPaths(t *testing.T) {
	request, frameworks := bundleAssemblyFixture(t)
	result, err := AssembleBundle(request)
	require.NoError(t, err)
	identity := bundleSigningIdentityFixture(t)
	require.NoError(t, SignBundle(result, identity))

	tampered := proto.Clone(result.Bundle).(*compliancev1.ComplianceReportBundle)
	tampered.Artifacts = append(tampered.Artifacts, proto.Clone(tampered.Artifacts[0]).(*compliancev1.BundleArtifact))
	err = catalog.ValidateComplianceReportBundle(tampered, frameworks)
	assert.ErrorIs(t, err, constants.ErrInvalidEvidenceGraph)
}

func TestValidateComplianceReportBundle_RejectsRenderedFormatWithoutArtifact(t *testing.T) {
	request, frameworks := bundleAssemblyFixture(t)
	result, err := AssembleBundle(request)
	require.NoError(t, err)
	identity := bundleSigningIdentityFixture(t)
	require.NoError(t, SignBundle(result, identity))

	tampered := proto.Clone(result.Bundle).(*compliancev1.ComplianceReportBundle)
	tampered.RenderedFormats = append(tampered.RenderedFormats, &compliancev1.RenderedFormatEntry{
		Format:     "oscal",
		MediaType:  constants.MediaTypeOSCALJSON,
		BundlePath: "nonexistent.json",
	})
	err = catalog.ValidateComplianceReportBundle(tampered, frameworks)
	assert.ErrorIs(t, err, constants.ErrUnresolvedReference)
}

func TestValidateBundleArtifact_RejectsRestrictedWithoutEncryption(t *testing.T) {
	artifact := &compliancev1.BundleArtifact{
		BundlePath: "restricted/evidence.json",
		Sha256:     strings.Repeat("a", 64),
		MediaType:  constants.MediaTypeJSON,
		Profile:    constants.ComplianceBundleProfileRestricted,
		ByteLength: 100,
	}
	err := catalog.ValidateBundleArtifact(artifact)
	assert.ErrorIs(t, err, constants.ErrEvidenceEncryptionInvalid)
}

func TestValidateBundleArtifact_RejectsUnsupportedProfile(t *testing.T) {
	artifact := &compliancev1.BundleArtifact{
		BundlePath: "analysis.json",
		Sha256:     strings.Repeat("a", 64),
		MediaType:  constants.MediaTypeJSON,
		Profile:    "confidential",
		ByteLength: 100,
	}
	err := catalog.ValidateBundleArtifact(artifact)
	assert.ErrorIs(t, err, constants.ErrBundleProfileUnsupported)
}
