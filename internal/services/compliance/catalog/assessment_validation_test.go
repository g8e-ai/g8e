// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package catalog_test

import (
	"path"
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
		ComponentInventory:    []*compliancev1.ComponentInventoryEntry{{ComponentId: "gateway", ComponentType: "service", Version: "2.1.3", Digest: validSHA256}},
		ConfigurationHashes:   []*compliancev1.NamedDigest{{Name: "gateway", Sha256: validSHA256}},
		DoctrineBundleHashes:  []*compliancev1.NamedDigest{{Name: "fedramp", Sha256: validSHA256}},
		ConsensusPolicyHashes: []*compliancev1.NamedDigest{{Name: "default", Sha256: validSHA256}},
		TrustAnchorIds:        []string{"root-1"},
	}
}

func TestValidateAssessmentScopeRejectsMalformedAndDuplicateBindings(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*compliancev1.AssessmentScope)
	}{
		{name: "malformed topology hash", mutate: func(s *compliancev1.AssessmentScope) { s.NetworkTopologyHash = "bad" }},
		{name: "reversed assessment window", mutate: func(s *compliancev1.AssessmentScope) { s.AssessmentWindowEnd = s.AssessmentWindowStart }},
		{name: "invalid assessment timestamp", mutate: func(s *compliancev1.AssessmentScope) {
			s.AssessmentWindowStart = &timestamppb.Timestamp{Seconds: 253402300800}
		}},
		{name: "duplicate image identity", mutate: func(s *compliancev1.AssessmentScope) { s.ImageDigests = append(s.ImageDigests, s.ImageDigests[0]) }},
		{name: "incomplete component", mutate: func(s *compliancev1.AssessmentScope) { s.ComponentInventory[0].ComponentType = "" }},
		{name: "malformed component digest", mutate: func(s *compliancev1.AssessmentScope) { s.ComponentInventory[0].Digest = "bad" }},
		{name: "duplicate component identity", mutate: func(s *compliancev1.AssessmentScope) {
			s.ComponentInventory = append(s.ComponentInventory, s.ComponentInventory[0])
		}},
		{name: "duplicate trust anchor", mutate: func(s *compliancev1.AssessmentScope) {
			s.TrustAnchorIds = append(s.TrustAnchorIds, s.TrustAnchorIds[0])
		}},
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
		{name: "cross scope assessment", mutate: func(a *compliancev1.ControlAssertionAssessment) { a.ScopeId = "scope-2" }, want: constants.ErrEvidenceScopeMismatch},
		{name: "satisfied result with stale evidence", mutate: func(a *compliancev1.ControlAssertionAssessment) { a.FreshnessStatus = "stale" }, want: constants.ErrInvalidEvidenceGraph},
		{name: "satisfied result below minimum evidence level", mutate: func(a *compliancev1.ControlAssertionAssessment) { a.EvidenceLevel = "L2" }, want: constants.ErrInvalidEvidenceGraph},
		{name: "duplicate evidence reference", mutate: func(a *compliancev1.ControlAssertionAssessment) {
			a.EvidenceRefs = append(a.EvidenceRefs, a.EvidenceRefs[0])
		}, want: constants.ErrInvalidEvidenceGraph},
		{name: "empty metric reference", mutate: func(a *compliancev1.ControlAssertionAssessment) { a.MetricRefs = []string{""} }, want: constants.ErrInvalidEvidenceGraph},
		{name: "unverifiable result without reason", mutate: func(a *compliancev1.ControlAssertionAssessment) { a.Status = "unverifiable" }, want: constants.ErrInvalidEvidenceGraph},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			candidate := valid()
			tt.mutate(candidate)
			err := catalog.ValidateAssertionAssessment(candidate, "scope-1", validAssertionCatalog())
			require.Error(t, err)
			assert.ErrorIs(t, err, tt.want)
		})
	}
}

func TestValidateAssertionAssessmentAcceptsEveryStatus(t *testing.T) {
	tests := []struct {
		name      string
		status    string
		freshness string
		failure   string
		evidence  []string
	}{
		{name: "satisfied", status: "satisfied", freshness: "fresh", evidence: []string{"artifact-1"}},
		{name: "not satisfied", status: "not_satisfied", freshness: "fresh", failure: "grader threshold failed"},
		{name: "not applicable", status: "not_applicable", freshness: "not_applicable"},
		{name: "unverifiable", status: "unverifiable", freshness: "incomplete", failure: "required evidence is missing"},
		{name: "customer attestation required", status: "customer_attestation_required", freshness: "incomplete", failure: "customer evidence is missing"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assessment := &compliancev1.ControlAssertionAssessment{
				AssessmentId:    "assessment-1",
				ScopeId:         "scope-1",
				AssertionRef:    &compliancev1.VersionedReference{Id: "G8E-GOV-BLOCK-001", Version: "1.0.0"},
				Status:          tt.status,
				EvidenceLevel:   "L3",
				EvaluatedAt:     timestamppb.Now(),
				VerifierRef:     &compliancev1.VersionedReference{Id: "receipt_integrity", Version: "1.0.0"},
				EvidenceRefs:    tt.evidence,
				FreshnessStatus: tt.freshness,
				FailureReason:   tt.failure,
			}
			assert.NoError(t, catalog.ValidateAssertionAssessment(assessment, "scope-1", validAssertionCatalog()))
		})
	}
}

func TestValidateAssertionAssessmentEnforcesOrderedMinimumEvidenceLevels(t *testing.T) {
	for _, level := range []string{"L0", "L1", "L2", "L3", "L4", "L5"} {
		t.Run(level, func(t *testing.T) {
			assessment := &compliancev1.ControlAssertionAssessment{
				AssessmentId:    "assessment-1",
				ScopeId:         "scope-1",
				AssertionRef:    &compliancev1.VersionedReference{Id: "G8E-GOV-BLOCK-001", Version: "1.0.0"},
				Status:          "satisfied",
				EvidenceLevel:   level,
				EvaluatedAt:     timestamppb.Now(),
				VerifierRef:     &compliancev1.VersionedReference{Id: "receipt_integrity", Version: "1.0.0"},
				EvidenceRefs:    []string{"artifact-1"},
				FreshnessStatus: "fresh",
			}
			err := catalog.ValidateAssertionAssessment(assessment, "scope-1", validAssertionCatalog())
			if level == "L3" || level == "L4" || level == "L5" {
				assert.NoError(t, err)
				return
			}
			require.Error(t, err)
			assert.ErrorIs(t, err, constants.ErrInvalidEvidenceGraph)
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
		{name: "cross scope assessment", mutate: func(a *compliancev1.FrameworkControlAssessment) { a.ScopeId = "scope-2" }, want: constants.ErrEvidenceScopeMismatch},
		{name: "unknown mapping", mutate: func(a *compliancev1.FrameworkControlAssessment) { a.MappingRefs[0] = "unknown" }, want: constants.ErrUnresolvedReference},
		{name: "mismatched control responsibility", mutate: func(a *compliancev1.FrameworkControlAssessment) { a.Responsibility = "platform" }, want: constants.ErrInvalidEvidenceGraph},
		{name: "satisfied result below mapping evidence level", mutate: func(a *compliancev1.FrameworkControlAssessment) { a.EvidenceLevel = "L2" }, want: constants.ErrInvalidEvidenceGraph},
		{name: "duplicate assertion assessment reference", mutate: func(a *compliancev1.FrameworkControlAssessment) {
			a.AssertionAssessmentRefs = append(a.AssertionAssessmentRefs, a.AssertionAssessmentRefs[0])
		}, want: constants.ErrInvalidEvidenceGraph},
		{name: "empty customer attestation reference", mutate: func(a *compliancev1.FrameworkControlAssessment) { a.CustomerAttestationRefs = []string{""} }, want: constants.ErrInvalidEvidenceGraph},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			candidate := valid()
			tt.mutate(candidate)
			err := catalog.ValidateControlAssessment(candidate, "scope-1", frameworks, crosswalks)
			require.Error(t, err)
			assert.ErrorIs(t, err, tt.want)
		})
	}
}

func TestValidateControlAssessmentAcceptsEveryStatusAndResponsibility(t *testing.T) {
	statuses := []string{"satisfied", "not_satisfied", "not_applicable", "unverifiable", "customer_attestation_required"}
	responsibilities := []string{"platform", "customer", "shared", "inherited", "assessor"}
	for _, responsibility := range responsibilities {
		for _, status := range statuses {
			t.Run(responsibility+" "+status, func(t *testing.T) {
				framework := validFramework()
				framework.Controls[0].Responsibility = responsibility
				frameworks := &compliancev1.FrameworkCatalog{CatalogId: "frameworks", CatalogVersion: "1.0.0", Sha256: validSHA256, Frameworks: []*compliancev1.FrameworkDefinition{framework}}
				crosswalks := validCrosswalk()
				crosswalks.Mappings[0].Responsibility = responsibility
				assessment := &compliancev1.FrameworkControlAssessment{
					AssessmentId:            "control-assessment-1",
					ScopeId:                 "scope-1",
					FrameworkRef:            &compliancev1.VersionedReference{Id: "fedramp-20x", Version: "CR26-2026-06-24"},
					ControlId:               "KSI-MLA-07",
					Status:                  status,
					Responsibility:          responsibility,
					MappingRefs:             []string{"fedramp-20x:KSI-MLA-07:G8E-GOV-BLOCK-001"},
					AssertionAssessmentRefs: []string{"assessment-1"},
					EvidenceLevel:           "L3",
				}
				if status == "satisfied" && responsibility == "customer" {
					assessment.CustomerAttestationRefs = []string{"attestation-1"}
				}
				assert.NoError(t, catalog.ValidateControlAssessment(assessment, "scope-1", frameworks, crosswalks))
			})
		}
	}
}

func TestValidateControlAssessmentRejectsSatisfiedCustomerControlWithoutAttestation(t *testing.T) {
	framework := validFramework()
	framework.Controls[0].Responsibility = "customer"
	frameworks := &compliancev1.FrameworkCatalog{CatalogId: "frameworks", CatalogVersion: "1.0.0", Sha256: validSHA256, Frameworks: []*compliancev1.FrameworkDefinition{framework}}
	crosswalks := validCrosswalk()
	crosswalks.Mappings[0].Responsibility = "customer"
	assessment := &compliancev1.FrameworkControlAssessment{
		AssessmentId:            "control-assessment-1",
		ScopeId:                 "scope-1",
		FrameworkRef:            &compliancev1.VersionedReference{Id: "fedramp-20x", Version: "CR26-2026-06-24"},
		ControlId:               "KSI-MLA-07",
		Status:                  "satisfied",
		Responsibility:          "customer",
		MappingRefs:             []string{"fedramp-20x:KSI-MLA-07:G8E-GOV-BLOCK-001"},
		AssertionAssessmentRefs: []string{"assessment-1"},
		EvidenceLevel:           "L3",
	}

	err := catalog.ValidateControlAssessment(assessment, "scope-1", frameworks, crosswalks)
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrInvalidEvidenceGraph)
}

func TestValidateReportManifestRejectsInvalidReferences(t *testing.T) {
	frameworks := &compliancev1.FrameworkCatalog{CatalogId: "frameworks", CatalogVersion: "1.0.0", Sha256: validSHA256, Frameworks: []*compliancev1.FrameworkDefinition{validFramework()}}
	valid := func() *compliancev1.ComplianceReportManifest {
		return &compliancev1.ComplianceReportManifest{
			ReportId:            "report-1",
			ReportSchemaVersion: "1.0.0",
			GeneratedAt:         timestamppb.Now(),
			GeneratorIdentity:   "g8e-cli",
			GeneratorVersion:    "2.1.3",
			ScopeRef:            constants.ComplianceBundleScopeFilename,
			FrameworkRefs:       []*compliancev1.VersionedReference{{Id: "fedramp-20x", Version: "CR26-2026-06-24"}},
			AssertionCatalogRef: path.Join(constants.ComplianceBundleAssertionsDirname, constants.ComplianceBundleAssertionCatalogFilename),
			CrosswalkRefs:       []string{path.Join(constants.ComplianceBundleCrosswalksDirname, constants.ComplianceBundleCrosswalkFilename)},
			AssessmentRefs:      []string{path.Join(constants.ComplianceBundleAssessmentsDirname, constants.ComplianceBundleAssertionAssessmentsFilename)},
			EvidenceIndexRef:    path.Join(constants.ComplianceBundleEvidenceDirname, constants.ComplianceBundleEvidenceIndexFilename),
			ChecksumRoot:        validSHA256,
			Signature:           &compliancev1.ReportSignature{KeyId: "key-1", Algorithm: "ed25519", SignedSha256: validSHA256, Signature: "signature"},
		}
	}
	tests := []struct {
		name   string
		mutate func(*compliancev1.ComplianceReportManifest)
		want   error
	}{
		{name: "unknown framework version", mutate: func(m *compliancev1.ComplianceReportManifest) { m.FrameworkRefs[0].Version = "unknown" }, want: constants.ErrUnsupportedFramework},
		{name: "duplicate framework reference", mutate: func(m *compliancev1.ComplianceReportManifest) {
			m.FrameworkRefs = append(m.FrameworkRefs, m.FrameworkRefs[0])
		}, want: constants.ErrInvalidEvidenceGraph},
		{name: "duplicate crosswalk reference", mutate: func(m *compliancev1.ComplianceReportManifest) {
			m.CrosswalkRefs = append(m.CrosswalkRefs, m.CrosswalkRefs[0])
		}, want: constants.ErrInvalidEvidenceGraph},
		{name: "noncanonical assessment path", mutate: func(m *compliancev1.ComplianceReportManifest) {
			m.AssessmentRefs[0] = "assessments/./assertion-assessments.jsonl"
		}, want: constants.ErrInvalidEvidenceGraph},
		{name: "invalid generation timestamp", mutate: func(m *compliancev1.ComplianceReportManifest) {
			m.GeneratedAt = &timestamppb.Timestamp{Seconds: 253402300800}
		}, want: constants.ErrInvalidEvidenceGraph},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			candidate := valid()
			tt.mutate(candidate)
			err := catalog.ValidateReportManifest(candidate, frameworks)
			require.Error(t, err)
			assert.ErrorIs(t, err, tt.want)
		})
	}
}

func TestValidateChecksumEntryRejectsUnsafePathsAndMalformedDigests(t *testing.T) {
	tests := []struct {
		name string
		path string
		hash string
	}{
		{name: "empty path", hash: validSHA256},
		{name: "absolute path", path: "/manifest.json", hash: validSHA256},
		{name: "parent path", path: "..", hash: validSHA256},
		{name: "escaping path", path: "../manifest.json", hash: validSHA256},
		{name: "nested escaping path", path: "evidence/../../manifest.json", hash: validSHA256},
		{name: "drive-qualified path", path: "C:/manifest.json", hash: validSHA256},
		{name: "backslash path", path: `evidence\manifest.json`, hash: validSHA256},
		{name: "current-directory path", path: "./manifest.json", hash: validSHA256},
		{name: "embedded current directory", path: "evidence/./manifest.json", hash: validSHA256},
		{name: "duplicate separator", path: "evidence//manifest.json", hash: validSHA256},
		{name: "trailing separator", path: "evidence/", hash: validSHA256},
		{name: "malformed digest", path: "manifest.json", hash: "bad"},
		{name: "uppercase digest", path: "manifest.json", hash: "0123456789ABCDEF0123456789ABCDEF0123456789ABCDEF0123456789ABCDEF"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := catalog.ValidateChecksumEntry(&compliancev1.ChecksumEntry{BundlePath: tt.path, Sha256: tt.hash})
			require.Error(t, err)
			assert.ErrorIs(t, err, constants.ErrInvalidEvidenceGraph)
		})
	}
	assert.NoError(t, catalog.ValidateChecksumEntry(&compliancev1.ChecksumEntry{BundlePath: "evidence/manifest.json", Sha256: validSHA256}))
}

func TestValidateReportSignatureEnforcesRequiredFieldsAlgorithmAndDigest(t *testing.T) {
	valid := func() *compliancev1.ReportSignature {
		return &compliancev1.ReportSignature{KeyId: "key-1", Algorithm: "ed25519", SignedSha256: validSHA256, Signature: "signature"}
	}
	tests := []struct {
		name   string
		mutate func(*compliancev1.ReportSignature) *compliancev1.ReportSignature
	}{
		{name: "missing signature", mutate: func(*compliancev1.ReportSignature) *compliancev1.ReportSignature { return nil }},
		{name: "missing key ID", mutate: func(s *compliancev1.ReportSignature) *compliancev1.ReportSignature { s.KeyId = ""; return s }},
		{name: "missing algorithm", mutate: func(s *compliancev1.ReportSignature) *compliancev1.ReportSignature { s.Algorithm = ""; return s }},
		{name: "missing signature bytes", mutate: func(s *compliancev1.ReportSignature) *compliancev1.ReportSignature { s.Signature = ""; return s }},
		{name: "unsupported algorithm", mutate: func(s *compliancev1.ReportSignature) *compliancev1.ReportSignature { s.Algorithm = "rsa"; return s }},
		{name: "malformed digest", mutate: func(s *compliancev1.ReportSignature) *compliancev1.ReportSignature { s.SignedSha256 = "bad"; return s }},
		{name: "uppercase digest", mutate: func(s *compliancev1.ReportSignature) *compliancev1.ReportSignature {
			s.SignedSha256 = "0123456789ABCDEF0123456789ABCDEF0123456789ABCDEF0123456789ABCDEF"
			return s
		}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := catalog.ValidateReportSignature(tt.mutate(valid()))
			require.Error(t, err)
			assert.ErrorIs(t, err, constants.ErrReportSignatureFailed)
		})
	}
	assert.NoError(t, catalog.ValidateReportSignature(valid()))
}

func TestValidateVerificationReportEnforcesIntegrityResultConsistency(t *testing.T) {
	valid := func() *compliancev1.ComplianceVerificationReport {
		return &compliancev1.ComplianceVerificationReport{ReportId: "report-1", Valid: true, VerifiedAt: timestamppb.Now(), VerifierId: "compliance_bundle", VerifierVersion: "1.0.0", ReproducedChecksumRoot: validSHA256}
	}
	tests := []struct {
		name   string
		mutate func(*compliancev1.ComplianceVerificationReport) *compliancev1.ComplianceVerificationReport
		want   error
	}{
		{name: "missing report", mutate: func(*compliancev1.ComplianceVerificationReport) *compliancev1.ComplianceVerificationReport {
			return nil
		}, want: constants.ErrReportVerificationFailed},
		{name: "missing report ID", mutate: func(r *compliancev1.ComplianceVerificationReport) *compliancev1.ComplianceVerificationReport {
			r.ReportId = ""
			return r
		}, want: constants.ErrReportVerificationFailed},
		{name: "invalid verification timestamp", mutate: func(r *compliancev1.ComplianceVerificationReport) *compliancev1.ComplianceVerificationReport {
			r.VerifiedAt = &timestamppb.Timestamp{Seconds: 253402300800}
			return r
		}, want: constants.ErrReportVerificationFailed},
		{name: "unsupported verifier", mutate: func(r *compliancev1.ComplianceVerificationReport) *compliancev1.ComplianceVerificationReport {
			r.VerifierVersion = "2.0.0"
			return r
		}, want: constants.ErrUnsupportedVerifier},
		{name: "malformed checksum root", mutate: func(r *compliancev1.ComplianceVerificationReport) *compliancev1.ComplianceVerificationReport {
			r.ReproducedChecksumRoot = "bad"
			return r
		}, want: constants.ErrReportVerificationFailed},
		{name: "valid result with failures", mutate: func(r *compliancev1.ComplianceVerificationReport) *compliancev1.ComplianceVerificationReport {
			r.Failures = []*compliancev1.VerificationFailure{{Code: "changed", SubjectRef: "manifest.json", Reason: "digest mismatch"}}
			return r
		}, want: constants.ErrReportVerificationFailed},
		{name: "invalid result without failures", mutate: func(r *compliancev1.ComplianceVerificationReport) *compliancev1.ComplianceVerificationReport {
			r.Valid = false
			return r
		}, want: constants.ErrReportVerificationFailed},
		{name: "incomplete failure", mutate: func(r *compliancev1.ComplianceVerificationReport) *compliancev1.ComplianceVerificationReport {
			r.Valid = false
			r.Failures = []*compliancev1.VerificationFailure{{Code: "changed", SubjectRef: "manifest.json"}}
			return r
		}, want: constants.ErrReportVerificationFailed},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := catalog.ValidateVerificationReport(tt.mutate(valid()))
			require.Error(t, err)
			assert.ErrorIs(t, err, tt.want)
		})
	}
	assert.NoError(t, catalog.ValidateVerificationReport(valid()))
	invalid := valid()
	invalid.Valid = false
	invalid.Failures = []*compliancev1.VerificationFailure{{Code: "changed", SubjectRef: "manifest.json", Reason: "digest mismatch"}}
	assert.NoError(t, catalog.ValidateVerificationReport(invalid))
}
