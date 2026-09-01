// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package catalog_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/services/compliance/catalog"
	compliancev1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/compliance/v1"
)

func validScope() *compliancev1.AssessmentScope {
	return &compliancev1.AssessmentScope{
		ScopeId:               "scope-1",
		OrganizationId:        "org-1",
		DeploymentId:          "deployment-1",
		ProductVersion:        "2.1.3",
		BuildIdentity:         "build-1",
		SourceRevision:        "revision-1",
		NetworkTopologyHash:   validSHA256,
		CryptographicMode:     "standard",
		AssessmentWindowStart: timestamppb.New(time.Date(2026, time.August, 1, 0, 0, 0, 0, time.UTC)),
		AssessmentWindowEnd:   timestamppb.New(time.Date(2026, time.September, 1, 0, 0, 0, 0, time.UTC)),
		ImageDigests:          []*compliancev1.NamedDigest{{Name: "gateway", Sha256: validSHA256}},
		ConfigurationHashes:   []*compliancev1.NamedDigest{{Name: "gateway", Sha256: validSHA256}},
	}
}

func TestValidateAssessmentScopeRejectsMalformedAndDuplicateBindings(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*compliancev1.AssessmentScope)
	}{
		{name: "malformed topology hash", mutate: func(s *compliancev1.AssessmentScope) { s.NetworkTopologyHash = "bad" }},
		{name: "reversed assessment window", mutate: func(s *compliancev1.AssessmentScope) { s.AssessmentWindowEnd = s.AssessmentWindowStart }},
		{name: "duplicate image identity", mutate: func(s *compliancev1.AssessmentScope) { s.ImageDigests = append(s.ImageDigests, s.ImageDigests[0]) }},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			candidate := validScope()
			tt.mutate(candidate)
			err := catalog.ValidateAssessmentScope(candidate)
			require.Error(t, err)
			assert.ErrorIs(t, err, constants.ErrInvalidEvidenceGraph)
		})
	}
}

func TestValidateAssertionAssessmentEnforcesReferencesStatusAndFreshness(t *testing.T) {
	valid := func() *compliancev1.ControlAssertionAssessment {
		return &compliancev1.ControlAssertionAssessment{
			AssessmentId:    "assessment-1",
			ScopeId:         "scope-1",
			AssertionRef:    &compliancev1.VersionedReference{Id: "G8E-GOV-BLOCK-001", Version: "1.0.0"},
			Status:          "satisfied",
			EvidenceLevel:   "L3",
			EvaluatedAt:     timestamppb.Now(),
			VerifierRef:     &compliancev1.VersionedReference{Id: "receipt_integrity", Version: "1.0.0"},
			EvidenceRefs:    []string{"artifact-1"},
			FreshnessStatus: "fresh",
		}
	}
	tests := []struct {
		name   string
		mutate func(*compliancev1.ControlAssertionAssessment)
		want   error
	}{
		{name: "unknown assertion version", mutate: func(a *compliancev1.ControlAssertionAssessment) { a.AssertionRef.Version = "2.0.0" }, want: constants.ErrUnsupportedAssertion},
		{name: "unknown verifier version", mutate: func(a *compliancev1.ControlAssertionAssessment) { a.VerifierRef.Version = "2.0.0" }, want: constants.ErrUnsupportedVerifier},
		{name: "satisfied result with stale evidence", mutate: func(a *compliancev1.ControlAssertionAssessment) { a.FreshnessStatus = "stale" }, want: constants.ErrInvalidEvidenceGraph},
		{name: "unverifiable result without reason", mutate: func(a *compliancev1.ControlAssertionAssessment) { a.Status = "unverifiable" }, want: constants.ErrInvalidEvidenceGraph},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			candidate := valid()
			tt.mutate(candidate)
			err := catalog.ValidateAssertionAssessment(candidate, validAssertionCatalog())
			require.Error(t, err)
			assert.ErrorIs(t, err, tt.want)
		})
	}
}

func TestValidateControlAssessmentRejectsUnresolvedMappingsAndUnsupportedFrameworks(t *testing.T) {
	frameworks := &compliancev1.FrameworkCatalog{CatalogId: "frameworks", CatalogVersion: "1.0.0", Sha256: validSHA256, Frameworks: []*compliancev1.FrameworkDefinition{validFramework()}}
	crosswalks := validCrosswalk()
	valid := func() *compliancev1.FrameworkControlAssessment {
		return &compliancev1.FrameworkControlAssessment{
			AssessmentId:            "control-assessment-1",
			ScopeId:                 "scope-1",
			FrameworkRef:            &compliancev1.VersionedReference{Id: "fedramp-20x", Version: "CR26-2026-06-24"},
			ControlId:               "KSI-MLA-07",
			Status:                  "satisfied",
			Responsibility:          "shared",
			MappingRefs:             []string{"fedramp-20x:KSI-MLA-07:G8E-GOV-BLOCK-001"},
			AssertionAssessmentRefs: []string{"assessment-1"},
			EvidenceLevel:           "L3",
		}
	}
	tests := []struct {
		name   string
		mutate func(*compliancev1.FrameworkControlAssessment)
		want   error
	}{
		{name: "unknown framework", mutate: func(a *compliancev1.FrameworkControlAssessment) { a.FrameworkRef.Version = "unknown" }, want: constants.ErrUnsupportedFramework},
		{name: "unknown mapping", mutate: func(a *compliancev1.FrameworkControlAssessment) { a.MappingRefs[0] = "unknown" }, want: constants.ErrUnresolvedReference},
		{name: "customer satisfaction without attestation", mutate: func(a *compliancev1.FrameworkControlAssessment) { a.Responsibility = "customer" }, want: constants.ErrInvalidEvidenceGraph},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			candidate := valid()
			tt.mutate(candidate)
			err := catalog.ValidateControlAssessment(candidate, frameworks, crosswalks)
			require.Error(t, err)
			assert.ErrorIs(t, err, tt.want)
		})
	}
}

func TestValidateReportRecordsRejectIntegrityContradictions(t *testing.T) {
	tests := []struct {
		name string
		run  func() error
		want error
	}{
		{name: "escaping checksum path", run: func() error {
			return catalog.ValidateChecksumEntry(&compliancev1.ChecksumEntry{BundlePath: "../manifest.json", Sha256: validSHA256})
		}, want: constants.ErrInvalidEvidenceGraph},
		{name: "malformed signature digest", run: func() error {
			return catalog.ValidateReportSignature(&compliancev1.ReportSignature{KeyId: "key-1", Algorithm: "ed25519", SignedSha256: "bad", Signature: "signature"})
		}, want: constants.ErrReportSignatureFailed},
		{name: "valid verification report with failures", run: func() error {
			return catalog.ValidateVerificationReport(&compliancev1.ComplianceVerificationReport{ReportId: "report-1", Valid: true, VerifiedAt: timestamppb.Now(), VerifierId: "compliance_bundle", VerifierVersion: "1.0.0", Failures: []*compliancev1.VerificationFailure{{Code: "changed", SubjectRef: "manifest.json", Reason: "digest mismatch"}}, ReproducedChecksumRoot: validSHA256})
		}, want: constants.ErrReportVerificationFailed},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.run()
			require.Error(t, err)
			assert.ErrorIs(t, err, tt.want)
		})
	}
}
