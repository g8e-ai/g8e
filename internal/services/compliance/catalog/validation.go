// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package catalog

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"path"
	"strings"
	"time"

	"google.golang.org/protobuf/proto"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	compliancev1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/compliance/v1"
)

var responsibilities = []string{"platform", "customer", "shared", "inherited", "assessor"}
var supportStatuses = []string{"mapped", "unsupported"}
var mappingTypes = []string{"full", "partial", "supporting", "not_applicable"}
var assessmentStatuses = []string{"satisfied", "not_satisfied", "not_applicable", "unverifiable", "customer_attestation_required"}
var evidenceLevels = []string{"L0", "L1", "L2", "L3", "L4", "L5"}
var validationCycles = []string{"7d", "90d"}
var missingEvidencePolicies = []string{"unverifiable", "customer_attestation_required", "not_applicable"}
var verificationStatuses = []string{"verified", "invalid", "unverifiable", "unsupported"}
var freshnessStatuses = []string{"fresh", "stale", "incomplete", "not_applicable"}
var signatureAlgorithms = []string{"ed25519"}
var supportedGraders = []string{"protocol_chain@1.0.0", "policy_outcome@1.0.0", "receipt_integrity@1.0.0", "receipt_persistence@1.0.0", "commitment_chain@1.0.0", "independent_state@1.0.0", "secret_detection_precision_recall@1.0.0", "model_boundary_raw_secret_rate@1.0.0", "exact_local_rehydration@1.0.0", "authenticated_operation@1.0.0", "fips_mode@1.0.0"}
var supportedVerifiers = []string{"receipt_integrity@1.0.0", "receipt_persistence@1.0.0", "deterministic_stage_chain@1.0.0", "commitment_chain@1.0.0", "state_observation@1.0.0", "eval_metric@1.0.0", "identity_attestation@1.0.0", "notary_proof@1.0.0", "build_provenance@1.0.0", "runtime_fips@1.0.0", "compliance_bundle@1.0.0"}

func ValidateAssertionCatalog(catalog *compliancev1.ControlAssertionCatalog) error {
	if catalog == nil || catalog.CatalogId == "" || catalog.CatalogVersion == "" || len(catalog.Assertions) == 0 {
		return fmt.Errorf("%w: assertion catalog requires identity, version, and assertions", constants.ErrInvalidEvidenceGraph)
	}
	if err := validateSHA256(catalog.Sha256); err != nil {
		return err
	}
	seen := make(map[string]struct{}, len(catalog.Assertions))
	for _, assertion := range catalog.Assertions {
		if assertion == nil || assertion.AssertionId == "" || assertion.AssertionVersion == "" || assertion.Title == "" || assertion.Statement == "" || assertion.Category == "" {
			return fmt.Errorf("%w: assertion requires identity, version, title, statement, and category", constants.ErrInvalidEvidenceGraph)
		}
		key := versionedKey(assertion.AssertionId, assertion.AssertionVersion)
		if _, exists := seen[key]; exists {
			return fmt.Errorf("%w: duplicate assertion %s", constants.ErrInvalidEvidenceGraph, key)
		}
		seen[key] = struct{}{}
		if !contains(responsibilities, assertion.Responsibility) || !contains(evidenceLevels, assertion.MinimumEvidenceLevel) || !contains(validationCycles, assertion.ValidationCycle) || !contains(missingEvidencePolicies, assertion.MissingEvidencePolicy) {
			return fmt.Errorf("%w: assertion %s has invalid semantics", constants.ErrInvalidEvidenceGraph, key)
		}
		if len(assertion.ComponentScope) == 0 || len(assertion.ApplicableActionClasses) == 0 || len(assertion.ApplicableArms) == 0 || len(assertion.RequiredEvidenceTypes) == 0 || len(assertion.RequiredGraderRefs) == 0 || len(assertion.RequiredVerifierRefs) == 0 || assertion.PassingRule == "" {
			return fmt.Errorf("%w: assertion %s has incomplete evaluation requirements", constants.ErrInvalidEvidenceGraph, key)
		}
		stringLists := []struct {
			label  string
			values []string
		}{
			{label: "component scope", values: assertion.ComponentScope},
			{label: "action classes", values: assertion.ApplicableActionClasses},
			{label: "applicable arms", values: assertion.ApplicableArms},
			{label: "evidence types", values: assertion.RequiredEvidenceTypes},
			{label: "exclusions", values: assertion.Exclusions},
		}
		for _, list := range stringLists {
			if err := validateUniqueStrings(list.values); err != nil {
				return fmt.Errorf("%w: assertion %s %s: %v", constants.ErrInvalidEvidenceGraph, key, list.label, err)
			}
		}
		if err := validateVersionedReferences(assertion.RequiredGraderRefs); err != nil {
			return fmt.Errorf("%w: assertion %s grader references: %v", constants.ErrInvalidEvidenceGraph, key, err)
		}
		if err := validateVersionedReferences(assertion.RequiredVerifierRefs); err != nil {
			return fmt.Errorf("%w: assertion %s verifier references: %v", constants.ErrInvalidEvidenceGraph, key, err)
		}
		for _, reference := range assertion.RequiredGraderRefs {
			if !contains(supportedGraders, referenceKey(reference)) {
				return fmt.Errorf("%w: %s", constants.ErrUnsupportedGrader, referenceKey(reference))
			}
		}
		for _, reference := range assertion.RequiredVerifierRefs {
			if !contains(supportedVerifiers, referenceKey(reference)) {
				return fmt.Errorf("%w: %s", constants.ErrUnsupportedVerifier, referenceKey(reference))
			}
		}
	}
	return nil
}

func ValidateFrameworkCatalog(catalog *compliancev1.FrameworkCatalog) error {
	if catalog == nil || catalog.CatalogId == "" || catalog.CatalogVersion == "" || len(catalog.Frameworks) == 0 {
		return fmt.Errorf("%w: framework catalog requires identity, version, and frameworks", constants.ErrInvalidEvidenceGraph)
	}
	if err := validateSHA256(catalog.Sha256); err != nil {
		return err
	}
	seen := make(map[string]struct{}, len(catalog.Frameworks))
	for _, framework := range catalog.Frameworks {
		if err := ValidateFrameworkDefinition(framework); err != nil {
			return err
		}
		key := versionedKey(framework.FrameworkId, framework.FrameworkVersion)
		if _, exists := seen[key]; exists {
			return fmt.Errorf("%w: duplicate framework %s", constants.ErrInvalidEvidenceGraph, key)
		}
		seen[key] = struct{}{}
	}
	return nil
}

func ValidateFrameworkDefinition(framework *compliancev1.FrameworkDefinition) error {
	if framework == nil || framework.FrameworkId == "" || framework.FrameworkVersion == "" || framework.Title == "" || framework.Publisher == "" || framework.Source == "" || framework.EffectiveDate == "" || len(framework.Controls) == 0 {
		return fmt.Errorf("%w: framework requires metadata and controls", constants.ErrInvalidEvidenceGraph)
	}
	if err := validateSHA256(framework.CatalogSha256); err != nil {
		return err
	}
	if _, err := time.Parse(time.DateOnly, framework.EffectiveDate); err != nil {
		return fmt.Errorf("%w: framework %s has invalid effective date", constants.ErrInvalidEvidenceGraph, framework.FrameworkId)
	}
	seen := make(map[string]struct{}, len(framework.Controls))
	for _, control := range framework.Controls {
		if control == nil || control.ControlId == "" || control.Title == "" || control.Description == "" || control.SourceReference == "" || control.SupportRationale == "" || !contains(responsibilities, control.Responsibility) || !contains(supportStatuses, control.SupportStatus) {
			return fmt.Errorf("%w: framework %s has an invalid control", constants.ErrInvalidEvidenceGraph, framework.FrameworkId)
		}
		if _, exists := seen[control.ControlId]; exists {
			return fmt.Errorf("%w: duplicate control %s", constants.ErrInvalidEvidenceGraph, control.ControlId)
		}
		seen[control.ControlId] = struct{}{}
	}
	return nil
}

func ValidateCrosswalkCatalog(crosswalks *compliancev1.ControlCrosswalkCatalog, assertions *compliancev1.ControlAssertionCatalog, framework *compliancev1.FrameworkDefinition) error {
	frameworks := &compliancev1.FrameworkCatalog{CatalogId: "validation", CatalogVersion: "1", Sha256: strings.Repeat("0", 64), Frameworks: []*compliancev1.FrameworkDefinition{framework}}
	return ValidateCatalogSet(assertions, frameworks, crosswalks)
}

func ValidateCatalogSet(assertions *compliancev1.ControlAssertionCatalog, frameworks *compliancev1.FrameworkCatalog, crosswalks *compliancev1.ControlCrosswalkCatalog) error {
	if err := ValidateAssertionCatalog(assertions); err != nil {
		return err
	}
	if err := ValidateFrameworkCatalog(frameworks); err != nil {
		return err
	}
	if crosswalks == nil || crosswalks.CatalogId == "" || crosswalks.CatalogVersion == "" || len(crosswalks.Mappings) == 0 {
		return fmt.Errorf("%w: crosswalk catalog requires identity, version, and mappings", constants.ErrInvalidEvidenceGraph)
	}
	if err := validateSHA256(crosswalks.Sha256); err != nil {
		return err
	}
	seen := make(map[string]struct{}, len(crosswalks.Mappings))
	for _, mapping := range crosswalks.Mappings {
		if mapping == nil || mapping.CrosswalkId == "" || mapping.CrosswalkVersion == "" || mapping.FrameworkRef == nil || mapping.ControlId == "" || len(mapping.AssertionRefs) == 0 || mapping.Rationale == "" || mapping.ReviewerIdentity == "" {
			return fmt.Errorf("%w: crosswalk mapping is incomplete", constants.ErrInvalidEvidenceGraph)
		}
		key := versionedKey(mapping.CrosswalkId, mapping.CrosswalkVersion)
		if _, exists := seen[key]; exists {
			return fmt.Errorf("%w: duplicate crosswalk %s", constants.ErrInvalidEvidenceGraph, key)
		}
		seen[key] = struct{}{}
		if !contains(mappingTypes, mapping.MappingType) || !contains(responsibilities, mapping.Responsibility) || !contains(evidenceLevels, mapping.RequiredEvidenceLevel) {
			return fmt.Errorf("%w: crosswalk %s has invalid semantics", constants.ErrInvalidEvidenceGraph, key)
		}
		if mapping.ReviewedAt == nil || mapping.ReviewedAt.CheckValid() != nil {
			return fmt.Errorf("%w: crosswalk %s has invalid review time", constants.ErrInvalidEvidenceGraph, key)
		}
		if err := validateVersionedReferences(mapping.AssertionRefs); err != nil {
			return fmt.Errorf("%w: crosswalk %s assertion references: %v", constants.ErrInvalidEvidenceGraph, key, err)
		}
		framework := FindFramework(frameworks, mapping.FrameworkRef.Id, mapping.FrameworkRef.Version)
		if framework == nil {
			return fmt.Errorf("%w: %s", constants.ErrUnsupportedFramework, versionedKey(mapping.FrameworkRef.Id, mapping.FrameworkRef.Version))
		}
		control := FindFrameworkControl(framework, mapping.ControlId)
		if control == nil {
			return fmt.Errorf("%w: framework control %s", constants.ErrUnresolvedReference, mapping.ControlId)
		}
		if control.SupportStatus != "mapped" {
			return fmt.Errorf("%w: framework control %s is not mapped", constants.ErrInvalidEvidenceGraph, mapping.ControlId)
		}
		if mapping.Responsibility != control.Responsibility {
			return fmt.Errorf("%w: crosswalk %s responsibility does not match framework control", constants.ErrInvalidEvidenceGraph, key)
		}
		for _, ref := range mapping.AssertionRefs {
			if ref == nil {
				return fmt.Errorf("%w: %s", constants.ErrUnsupportedAssertion, referenceKey(ref))
			}
			assertion := FindAssertion(assertions, ref.Id, ref.Version)
			if assertion == nil {
				return fmt.Errorf("%w: %s", constants.ErrUnsupportedAssertion, referenceKey(ref))
			}
			if evidenceLevelIndex(mapping.RequiredEvidenceLevel) < evidenceLevelIndex(assertion.MinimumEvidenceLevel) {
				return fmt.Errorf("%w: crosswalk %s evidence level is below assertion minimum", constants.ErrInvalidEvidenceGraph, key)
			}
		}
	}
	return nil
}

func ValidateAssessmentScope(scope *compliancev1.AssessmentScope) error {
	if scope == nil || scope.ScopeId == "" || scope.OrganizationId == "" || scope.DeploymentId == "" || scope.ProductVersion == "" || scope.BuildIdentity == "" || scope.SourceRevision == "" || scope.NetworkTopologyHash == "" || scope.CryptographicMode == "" || len(scope.ImageDigests) == 0 || len(scope.ComponentInventory) == 0 || len(scope.ConfigurationHashes) == 0 || len(scope.DoctrineBundleHashes) == 0 || len(scope.ConsensusPolicyHashes) == 0 || len(scope.TrustAnchorIds) == 0 {
		return fmt.Errorf("%w: assessment scope is incomplete", constants.ErrInvalidEvidenceGraph)
	}
	if err := validateSHA256(scope.NetworkTopologyHash); err != nil {
		return err
	}
	if scope.AssessmentWindowStart == nil || scope.AssessmentWindowEnd == nil || scope.AssessmentWindowStart.CheckValid() != nil || scope.AssessmentWindowEnd.CheckValid() != nil || !scope.AssessmentWindowStart.AsTime().Before(scope.AssessmentWindowEnd.AsTime()) {
		return fmt.Errorf("%w: assessment window is invalid", constants.ErrInvalidEvidenceGraph)
	}
	for _, digests := range [][]*compliancev1.NamedDigest{scope.ImageDigests, scope.ConfigurationHashes, scope.DoctrineBundleHashes, scope.ConsensusPolicyHashes} {
		if err := validateNamedDigests(digests); err != nil {
			return err
		}
	}
	seenComponents := make(map[string]struct{}, len(scope.ComponentInventory))
	for _, component := range scope.ComponentInventory {
		if component == nil || component.ComponentId == "" || component.ComponentType == "" || component.Version == "" {
			return fmt.Errorf("%w: component inventory entry is incomplete", constants.ErrInvalidEvidenceGraph)
		}
		if _, exists := seenComponents[component.ComponentId]; exists {
			return fmt.Errorf("%w: duplicate component %s", constants.ErrInvalidEvidenceGraph, component.ComponentId)
		}
		seenComponents[component.ComponentId] = struct{}{}
		if err := validateSHA256(component.Digest); err != nil {
			return err
		}
	}
	if err := validateUniqueStrings(scope.TrustAnchorIds); err != nil {
		return fmt.Errorf("%w: trust anchors: %v", constants.ErrInvalidEvidenceGraph, err)
	}
	return nil
}

func ValidateEvidenceReference(reference *compliancev1.ComplianceEvidenceReference, scopeID string) error {
	if reference == nil || reference.ArtifactId == "" || reference.ArtifactType == "" || reference.MediaType == "" || reference.SchemaRef == "" || reference.ProducerIdentity == "" || reference.RunId == "" || reference.VerifierId == "" || reference.VerifierVersion == "" {
		return fmt.Errorf("%w: evidence reference is incomplete", constants.ErrInvalidEvidenceGraph)
	}
	if reference.ScopeId != scopeID {
		return fmt.Errorf("%w: artifact %s belongs to scope %s", constants.ErrEvidenceScopeMismatch, reference.ArtifactId, reference.ScopeId)
	}
	if err := validateSHA256(reference.Sha256); err != nil {
		return err
	}
	if reference.ProducedAt == nil || reference.VerifiedAt == nil || reference.ProducedAt.CheckValid() != nil || reference.VerifiedAt.CheckValid() != nil || reference.VerifiedAt.AsTime().Before(reference.ProducedAt.AsTime()) {
		return fmt.Errorf("%w: artifact %s has invalid timestamps", constants.ErrInvalidEvidenceGraph, reference.ArtifactId)
	}
	if !contains(verificationStatuses, reference.VerificationStatus) {
		return fmt.Errorf("%w: artifact %s has invalid verification status", constants.ErrInvalidEvidenceGraph, reference.ArtifactId)
	}
	if !contains(supportedVerifiers, versionedKey(reference.VerifierId, reference.VerifierVersion)) {
		return fmt.Errorf("%w: %s", constants.ErrUnsupportedVerifier, versionedKey(reference.VerifierId, reference.VerifierVersion))
	}
	if err := validateBundlePath(reference.BundlePath); err != nil {
		return err
	}
	if reference.Encryption != nil {
		if reference.Encryption.Algorithm == "" || reference.Encryption.KeyId == "" || reference.Encryption.AuthorizationScope == "" {
			return fmt.Errorf("%w: artifact %s encryption metadata is incomplete", constants.ErrInvalidEvidenceGraph, reference.ArtifactId)
		}
		if err := validateSHA256(reference.Encryption.PlaintextSha256); err != nil {
			return err
		}
		if err := validateSHA256(reference.Encryption.AuthenticatedMetadataSha256); err != nil {
			return err
		}
	}
	return nil
}

func ValidateAssertionAssessment(assessment *compliancev1.ControlAssertionAssessment, scopeID string, assertions *compliancev1.ControlAssertionCatalog) error {
	if assessment == nil || assessment.AssessmentId == "" || assessment.ScopeId == "" || assessment.AssertionRef == nil || assessment.VerifierRef == nil || assessment.EvaluatedAt == nil || assessment.EvaluatedAt.CheckValid() != nil {
		return fmt.Errorf("%w: assertion assessment is incomplete", constants.ErrInvalidEvidenceGraph)
	}
	if assessment.ScopeId != scopeID {
		return fmt.Errorf("%w: assertion assessment %s belongs to scope %s", constants.ErrEvidenceScopeMismatch, assessment.AssessmentId, assessment.ScopeId)
	}
	assertion := FindAssertion(assertions, assessment.AssertionRef.Id, assessment.AssertionRef.Version)
	if assertion == nil {
		return fmt.Errorf("%w: %s", constants.ErrUnsupportedAssertion, referenceKey(assessment.AssertionRef))
	}
	if !contains(supportedVerifiers, referenceKey(assessment.VerifierRef)) {
		return fmt.Errorf("%w: %s", constants.ErrUnsupportedVerifier, referenceKey(assessment.VerifierRef))
	}
	if !contains(assessmentStatuses, assessment.Status) || !contains(evidenceLevels, assessment.EvidenceLevel) || !contains(freshnessStatuses, assessment.FreshnessStatus) {
		return fmt.Errorf("%w: assertion assessment has invalid semantics", constants.ErrInvalidEvidenceGraph)
	}
	if assessment.Status == "satisfied" && evidenceLevelIndex(assessment.EvidenceLevel) < evidenceLevelIndex(assertion.MinimumEvidenceLevel) {
		return fmt.Errorf("%w: satisfied assertion assessment evidence level is below assertion minimum", constants.ErrInvalidEvidenceGraph)
	}
	if err := validateUniqueStrings(assessment.EvidenceRefs); err != nil {
		return fmt.Errorf("%w: assertion assessment evidence references: %v", constants.ErrInvalidEvidenceGraph, err)
	}
	if err := validateUniqueStrings(assessment.MetricRefs); err != nil {
		return fmt.Errorf("%w: assertion assessment metric references: %v", constants.ErrInvalidEvidenceGraph, err)
	}
	if assessment.Status == "satisfied" && (len(assessment.EvidenceRefs) == 0 || assessment.FailureReason != "" || assessment.FreshnessStatus != "fresh") {
		return fmt.Errorf("%w: satisfied assertion assessment requires fresh evidence and no failure", constants.ErrInvalidEvidenceGraph)
	}
	if assessment.Status == "unverifiable" && assessment.FailureReason == "" {
		return fmt.Errorf("%w: unverifiable assertion assessment requires a failure reason", constants.ErrInvalidEvidenceGraph)
	}
	return nil
}

func ValidateControlAssessment(assessment *compliancev1.FrameworkControlAssessment, scopeID string, frameworks *compliancev1.FrameworkCatalog, crosswalks *compliancev1.ControlCrosswalkCatalog) error {
	if assessment == nil || assessment.AssessmentId == "" || assessment.ScopeId == "" || assessment.FrameworkRef == nil || assessment.ControlId == "" || len(assessment.MappingRefs) == 0 {
		return fmt.Errorf("%w: control assessment is incomplete", constants.ErrInvalidEvidenceGraph)
	}
	if assessment.ScopeId != scopeID {
		return fmt.Errorf("%w: control assessment %s belongs to scope %s", constants.ErrEvidenceScopeMismatch, assessment.AssessmentId, assessment.ScopeId)
	}
	framework := FindFramework(frameworks, assessment.FrameworkRef.Id, assessment.FrameworkRef.Version)
	if framework == nil {
		return fmt.Errorf("%w: %s", constants.ErrUnsupportedFramework, referenceKey(assessment.FrameworkRef))
	}
	control := FindFrameworkControl(framework, assessment.ControlId)
	if control == nil {
		return fmt.Errorf("%w: framework control %s", constants.ErrUnresolvedReference, assessment.ControlId)
	}
	if assessment.Responsibility != control.Responsibility {
		return fmt.Errorf("%w: control assessment responsibility does not match framework control", constants.ErrInvalidEvidenceGraph)
	}
	if err := validateUniqueStrings(assessment.MappingRefs); err != nil {
		return fmt.Errorf("%w: control assessment mappings: %v", constants.ErrInvalidEvidenceGraph, err)
	}
	for _, reference := range assessment.MappingRefs {
		mapping := FindCrosswalk(crosswalks, reference)
		if mapping == nil {
			return fmt.Errorf("%w: crosswalk %s", constants.ErrUnresolvedReference, reference)
		}
		if mapping.FrameworkRef == nil || mapping.FrameworkRef.Id != assessment.FrameworkRef.Id || mapping.FrameworkRef.Version != assessment.FrameworkRef.Version || mapping.ControlId != assessment.ControlId {
			return fmt.Errorf("%w: crosswalk %s does not bind assessed control", constants.ErrUnresolvedReference, reference)
		}
		if mapping.Responsibility != assessment.Responsibility {
			return fmt.Errorf("%w: crosswalk %s responsibility does not match control assessment", constants.ErrInvalidEvidenceGraph, reference)
		}
		if assessment.Status == "satisfied" && evidenceLevelIndex(assessment.EvidenceLevel) < evidenceLevelIndex(mapping.RequiredEvidenceLevel) {
			return fmt.Errorf("%w: satisfied control assessment evidence level is below crosswalk requirement", constants.ErrInvalidEvidenceGraph)
		}
	}
	if !contains(assessmentStatuses, assessment.Status) || !contains(responsibilities, assessment.Responsibility) || !contains(evidenceLevels, assessment.EvidenceLevel) {
		return fmt.Errorf("%w: control assessment has invalid semantics", constants.ErrInvalidEvidenceGraph)
	}
	if err := validateUniqueStrings(assessment.AssertionAssessmentRefs); err != nil {
		return fmt.Errorf("%w: control assessment assertion references: %v", constants.ErrInvalidEvidenceGraph, err)
	}
	if err := validateUniqueStrings(assessment.CustomerAttestationRefs); err != nil {
		return fmt.Errorf("%w: control assessment attestation references: %v", constants.ErrInvalidEvidenceGraph, err)
	}
	if assessment.Status == "satisfied" && len(assessment.AssertionAssessmentRefs) == 0 {
		return fmt.Errorf("%w: satisfied control assessment requires assertion assessments", constants.ErrInvalidEvidenceGraph)
	}
	if assessment.Status == "satisfied" && assessment.Responsibility == "customer" && len(assessment.CustomerAttestationRefs) == 0 {
		return fmt.Errorf("%w: satisfied customer control requires attestation evidence", constants.ErrInvalidEvidenceGraph)
	}
	return nil
}

func ValidateReportManifest(manifest *compliancev1.ComplianceReportManifest, frameworks *compliancev1.FrameworkCatalog) error {
	if manifest == nil || manifest.ReportId == "" || manifest.ReportSchemaVersion == "" || manifest.GeneratedAt == nil || manifest.GeneratedAt.CheckValid() != nil || manifest.GeneratorIdentity == "" || manifest.GeneratorVersion == "" || manifest.ScopeRef == "" || len(manifest.FrameworkRefs) == 0 || manifest.AssertionCatalogRef == "" || len(manifest.CrosswalkRefs) == 0 || len(manifest.AssessmentRefs) == 0 || manifest.EvidenceIndexRef == "" || manifest.Signature == nil {
		return fmt.Errorf("%w: report manifest is incomplete", constants.ErrInvalidEvidenceGraph)
	}
	if err := validateVersionedReferences(manifest.FrameworkRefs); err != nil {
		return fmt.Errorf("%w: report framework references: %v", constants.ErrInvalidEvidenceGraph, err)
	}
	for _, reference := range manifest.FrameworkRefs {
		if FindFramework(frameworks, reference.Id, reference.Version) == nil {
			return fmt.Errorf("%w: %s", constants.ErrUnsupportedFramework, referenceKey(reference))
		}
	}
	for _, references := range [][]string{manifest.CrosswalkRefs, manifest.AssessmentRefs} {
		if err := validateUniqueStrings(references); err != nil {
			return fmt.Errorf("%w: report bundle references: %v", constants.ErrInvalidEvidenceGraph, err)
		}
	}
	for _, bundlePath := range append(append([]string{manifest.ScopeRef, manifest.AssertionCatalogRef, manifest.EvidenceIndexRef}, manifest.CrosswalkRefs...), manifest.AssessmentRefs...) {
		if err := validateBundlePath(bundlePath); err != nil {
			return err
		}
	}
	if err := validateSHA256(manifest.ChecksumRoot); err != nil {
		return err
	}
	return ValidateReportSignature(manifest.Signature)
}

func ValidateChecksumEntry(checksum *compliancev1.ChecksumEntry) error {
	if checksum == nil {
		return fmt.Errorf("%w: checksum entry is missing", constants.ErrInvalidEvidenceGraph)
	}
	if err := validateBundlePath(checksum.BundlePath); err != nil {
		return err
	}
	return validateSHA256(checksum.Sha256)
}

func ValidateReportSignature(signature *compliancev1.ReportSignature) error {
	if signature == nil || signature.KeyId == "" || signature.Algorithm == "" || signature.Signature == "" {
		return fmt.Errorf("%w: report signature is incomplete", constants.ErrReportSignatureFailed)
	}
	if !contains(signatureAlgorithms, signature.Algorithm) {
		return fmt.Errorf("%w: unsupported signature algorithm %s", constants.ErrReportSignatureFailed, signature.Algorithm)
	}
	if err := validateSHA256(signature.SignedSha256); err != nil {
		return fmt.Errorf("%w: %v", constants.ErrReportSignatureFailed, err)
	}
	return nil
}

func ValidateVerificationReport(report *compliancev1.ComplianceVerificationReport) error {
	if report == nil || report.ReportId == "" || report.VerifiedAt == nil || report.VerifiedAt.CheckValid() != nil || report.VerifierId == "" || report.VerifierVersion == "" {
		return fmt.Errorf("%w: verification report is incomplete", constants.ErrReportVerificationFailed)
	}
	if !contains(supportedVerifiers, versionedKey(report.VerifierId, report.VerifierVersion)) {
		return fmt.Errorf("%w: %s", constants.ErrUnsupportedVerifier, versionedKey(report.VerifierId, report.VerifierVersion))
	}
	if err := validateSHA256(report.ReproducedChecksumRoot); err != nil {
		return fmt.Errorf("%w: %v", constants.ErrReportVerificationFailed, err)
	}
	if report.Valid && len(report.Failures) != 0 || !report.Valid && len(report.Failures) == 0 {
		return fmt.Errorf("%w: verification result and failures disagree", constants.ErrReportVerificationFailed)
	}
	for _, failure := range report.Failures {
		if failure == nil || failure.Code == "" || failure.SubjectRef == "" || failure.Reason == "" {
			return fmt.Errorf("%w: verification failure is incomplete", constants.ErrReportVerificationFailed)
		}
	}
	return nil
}

func FindAssertion(catalog *compliancev1.ControlAssertionCatalog, id, version string) *compliancev1.ControlAssertionDefinition {
	if catalog == nil {
		return nil
	}
	for _, assertion := range catalog.Assertions {
		if assertion.AssertionId == id && assertion.AssertionVersion == version {
			return assertion
		}
	}
	return nil
}

func FindFramework(catalog *compliancev1.FrameworkCatalog, id, version string) *compliancev1.FrameworkDefinition {
	if catalog == nil {
		return nil
	}
	for _, framework := range catalog.Frameworks {
		if framework.FrameworkId == id && framework.FrameworkVersion == version {
			return framework
		}
	}
	return nil
}

func FindFrameworkControl(framework *compliancev1.FrameworkDefinition, id string) *compliancev1.FrameworkControlDefinition {
	if framework == nil {
		return nil
	}
	for _, control := range framework.Controls {
		if control.ControlId == id {
			return control
		}
	}
	return nil
}

func FindCrosswalk(catalog *compliancev1.ControlCrosswalkCatalog, id string) *compliancev1.ControlCrosswalk {
	if catalog == nil {
		return nil
	}
	for _, crosswalk := range catalog.Mappings {
		if crosswalk.CrosswalkId == id {
			return crosswalk
		}
	}
	return nil
}

func CatalogDigest(message proto.Message) (string, error) {
	candidate := proto.Clone(message)
	switch typed := candidate.(type) {
	case *compliancev1.ControlAssertionCatalog:
		typed.Sha256 = ""
	case *compliancev1.FrameworkCatalog:
		typed.Sha256 = ""
	case *compliancev1.FrameworkDefinition:
		typed.CatalogSha256 = ""
	case *compliancev1.ControlCrosswalkCatalog:
		typed.Sha256 = ""
	default:
		return "", fmt.Errorf("%w: unsupported catalog message %T", constants.ErrInvalidEvidenceGraph, message)
	}
	encoded, err := compliancev1.MarshalCanonical(candidate)
	if err != nil {
		return "", fmt.Errorf("%w: canonicalize catalog: %v", constants.ErrInvalidEvidenceGraph, err)
	}
	digest := sha256.Sum256(encoded)
	return hex.EncodeToString(digest[:]), nil
}

func validateSHA256(value string) error {
	if len(value) != sha256.Size*2 {
		return fmt.Errorf("%w: malformed sha256 digest", constants.ErrInvalidEvidenceGraph)
	}
	decoded, err := hex.DecodeString(value)
	if err != nil || len(decoded) != sha256.Size || strings.ToLower(value) != value {
		return fmt.Errorf("%w: malformed sha256 digest", constants.ErrInvalidEvidenceGraph)
	}
	return nil
}

func validateBundlePath(value string) error {
	firstSegment := strings.SplitN(value, "/", 2)[0]
	if value == "" || path.IsAbs(value) || strings.Contains(value, "\\") || strings.Contains(firstSegment, ":") || path.Clean(value) != value || value == "." || value == constants.PathParentDir || strings.HasPrefix(value, constants.PathParentDir+"/") {
		return fmt.Errorf("%w: unsafe bundle path %q", constants.ErrInvalidEvidenceGraph, value)
	}
	return nil
}

func validateVersionedReferences(references []*compliancev1.VersionedReference) error {
	seen := make(map[string]struct{}, len(references))
	for _, reference := range references {
		if reference == nil || reference.Id == "" || reference.Version == "" {
			return fmt.Errorf("versioned reference is incomplete")
		}
		key := versionedKey(reference.Id, reference.Version)
		if _, exists := seen[key]; exists {
			return fmt.Errorf("duplicate versioned reference %s", key)
		}
		seen[key] = struct{}{}
	}
	return nil
}

func validateNamedDigests(digests []*compliancev1.NamedDigest) error {
	seen := make(map[string]struct{}, len(digests))
	for _, digest := range digests {
		if digest == nil || digest.Name == "" {
			return fmt.Errorf("%w: named digest is incomplete", constants.ErrInvalidEvidenceGraph)
		}
		if _, exists := seen[digest.Name]; exists {
			return fmt.Errorf("%w: duplicate named digest %s", constants.ErrInvalidEvidenceGraph, digest.Name)
		}
		seen[digest.Name] = struct{}{}
		if err := validateSHA256(digest.Sha256); err != nil {
			return err
		}
	}
	return nil
}

func validateUniqueStrings(values []string) error {
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		if value == "" {
			return fmt.Errorf("reference is empty")
		}
		if _, exists := seen[value]; exists {
			return fmt.Errorf("duplicate reference %s", value)
		}
		seen[value] = struct{}{}
	}
	return nil
}

func contains(values []string, candidate string) bool {
	for _, value := range values {
		if value == candidate {
			return true
		}
	}
	return false
}

func evidenceLevelIndex(level string) int {
	for index, candidate := range evidenceLevels {
		if candidate == level {
			return index
		}
	}
	return -1
}

func versionedKey(id, version string) string {
	return id + "@" + version
}

func referenceKey(reference *compliancev1.VersionedReference) string {
	if reference == nil {
		return "<nil>"
	}
	return versionedKey(reference.Id, reference.Version)
}
