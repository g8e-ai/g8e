// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package catalog_test

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/services/compliance/catalog"
	complianceconstants "github.com/g8e-ai/g8e/v2/protocol/constants/compliance"
	compliancev1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/compliance/v1"
)

const validSHA256 = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

func validAssertion() *compliancev1.ControlAssertionDefinition {
	return &compliancev1.ControlAssertionDefinition{
		AssertionId:             "G8E-GOV-BLOCK-001",
		AssertionVersion:        "1.0.0",
		Title:                   "Governed block outcome",
		Statement:               "A prohibited governed action is rejected at the expected layer.",
		Category:                "governance",
		ComponentScope:          []string{"gateway", "operator"},
		Responsibility:          "platform",
		ApplicableActionClasses: []string{"FILE_DELETE"},
		ApplicableArms:          []string{"governed"},
		RequiredEvidenceTypes:   []string{"action_receipt", "state_observation"},
		RequiredGraderRefs:      []*compliancev1.VersionedReference{{Id: "policy_outcome", Version: "1.0.0"}},
		RequiredVerifierRefs:    []*compliancev1.VersionedReference{{Id: "receipt_integrity", Version: "1.0.0"}},
		MinimumEvidenceLevel:    "L3",
		ValidationCycle:         "7d",
		MissingEvidencePolicy:   "unverifiable",
		PassingRule:             "all_required",
	}
}

func validAssertionCatalog() *compliancev1.ControlAssertionCatalog {
	return &compliancev1.ControlAssertionCatalog{
		CatalogId:      "g8e-control-assertions",
		CatalogVersion: "1.0.0",
		Sha256:         validSHA256,
		Assertions:     []*compliancev1.ControlAssertionDefinition{validAssertion()},
	}
}

func validFramework() *compliancev1.FrameworkDefinition {
	return &compliancev1.FrameworkDefinition{
		FrameworkId:      "fedramp-20x",
		FrameworkVersion: "CR26-2026-06-24",
		Title:            "FedRAMP 20x",
		Publisher:        "FedRAMP",
		Source:           "https://www.fedramp.gov/20x/",
		CatalogSha256:    validSHA256,
		EffectiveDate:    "2026-06-24",
		Controls: []*compliancev1.FrameworkControlDefinition{
			{ControlId: "KSI-MLA-07", Title: "Protecting Logs", Description: "Protect audit information.", Responsibility: "shared", SourceReference: "CR26", SupportStatus: "mapped", SupportRationale: "A reviewed crosswalk maps this control to the initial assertion catalog."},
		},
	}
}

func validCrosswalk() *compliancev1.ControlCrosswalkCatalog {
	return &compliancev1.ControlCrosswalkCatalog{
		CatalogId:      "fedramp-20x-g8e-crosswalk",
		CatalogVersion: "1.0.0",
		Sha256:         validSHA256,
		Mappings: []*compliancev1.ControlCrosswalk{
			{
				CrosswalkId:           "fedramp-20x:KSI-MLA-07:G8E-GOV-BLOCK-001",
				CrosswalkVersion:      "1.0.0",
				FrameworkRef:          &compliancev1.VersionedReference{Id: "fedramp-20x", Version: "CR26-2026-06-24"},
				ControlId:             "KSI-MLA-07",
				AssertionRefs:         []*compliancev1.VersionedReference{{Id: "G8E-GOV-BLOCK-001", Version: "1.0.0"}},
				MappingType:           "supporting",
				Rationale:             "A verified denied destruction attempt supports protection of audit information.",
				Responsibility:        "shared",
				RequiredEvidenceLevel: "L3",
				ReviewedAt:            timestamppb.New(time.Date(2026, time.September, 1, 0, 0, 0, 0, time.UTC)),
				ReviewerIdentity:      "g8e-project",
			},
		},
	}
}

func TestValidateAssertionCatalogRejectsInvalidRecords(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*compliancev1.ControlAssertionCatalog)
	}{
		{name: "duplicate assertion reference", mutate: func(c *compliancev1.ControlAssertionCatalog) { c.Assertions = append(c.Assertions, validAssertion()) }},
		{name: "malformed catalog digest", mutate: func(c *compliancev1.ControlAssertionCatalog) { c.Sha256 = "sha256:not-a-digest" }},
		{name: "invalid responsibility", mutate: func(c *compliancev1.ControlAssertionCatalog) { c.Assertions[0].Responsibility = "vendor" }},
		{name: "unknown evidence level", mutate: func(c *compliancev1.ControlAssertionCatalog) { c.Assertions[0].MinimumEvidenceLevel = "L9" }},
		{name: "unknown missing evidence policy", mutate: func(c *compliancev1.ControlAssertionCatalog) { c.Assertions[0].MissingEvidencePolicy = "pass" }},
		{name: "missing action classes", mutate: func(c *compliancev1.ControlAssertionCatalog) { c.Assertions[0].ApplicableActionClasses = nil }},
		{name: "missing applicable arms", mutate: func(c *compliancev1.ControlAssertionCatalog) { c.Assertions[0].ApplicableArms = nil }},
		{name: "missing grader references", mutate: func(c *compliancev1.ControlAssertionCatalog) { c.Assertions[0].RequiredGraderRefs = nil }},
		{name: "missing verifier references", mutate: func(c *compliancev1.ControlAssertionCatalog) { c.Assertions[0].RequiredVerifierRefs = nil }},
		{name: "duplicate component scope", mutate: func(c *compliancev1.ControlAssertionCatalog) {
			c.Assertions[0].ComponentScope = append(c.Assertions[0].ComponentScope, c.Assertions[0].ComponentScope[0])
		}},
		{name: "duplicate evidence type", mutate: func(c *compliancev1.ControlAssertionCatalog) {
			c.Assertions[0].RequiredEvidenceTypes = append(c.Assertions[0].RequiredEvidenceTypes, c.Assertions[0].RequiredEvidenceTypes[0])
		}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			candidate := validAssertionCatalog()
			tt.mutate(candidate)
			err := catalog.ValidateAssertionCatalog(candidate)
			require.Error(t, err)
			assert.ErrorIs(t, err, constants.ErrInvalidEvidenceGraph)
		})
	}
}

func TestValidateAssertionCatalogRejectsUnknownVerifierAndGraderVersions(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*compliancev1.ControlAssertionDefinition)
		want   error
	}{
		{name: "unknown verifier", mutate: func(a *compliancev1.ControlAssertionDefinition) { a.RequiredVerifierRefs[0].Version = "2.0.0" }, want: constants.ErrUnsupportedVerifier},
		{name: "unknown grader", mutate: func(a *compliancev1.ControlAssertionDefinition) { a.RequiredGraderRefs[0].Version = "2.0.0" }, want: constants.ErrUnsupportedGrader},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			candidate := validAssertionCatalog()
			tt.mutate(candidate.Assertions[0])
			err := catalog.ValidateAssertionCatalog(candidate)
			require.Error(t, err)
			assert.ErrorIs(t, err, tt.want)
		})
	}
}

func TestValidateFrameworkDefinitionRejectsMalformedDefinitions(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*compliancev1.FrameworkDefinition)
	}{
		{name: "malformed effective date", mutate: func(f *compliancev1.FrameworkDefinition) { f.EffectiveDate = "2026-13-40" }},
		{name: "duplicate control", mutate: func(f *compliancev1.FrameworkDefinition) { f.Controls = append(f.Controls, f.Controls[0]) }},
		{name: "invalid control responsibility", mutate: func(f *compliancev1.FrameworkDefinition) { f.Controls[0].Responsibility = "vendor" }},
		{name: "missing support status", mutate: func(f *compliancev1.FrameworkDefinition) { f.Controls[0].SupportStatus = "" }},
		{name: "invalid support status", mutate: func(f *compliancev1.FrameworkDefinition) { f.Controls[0].SupportStatus = "planned" }},
		{name: "missing support rationale", mutate: func(f *compliancev1.FrameworkDefinition) { f.Controls[0].SupportRationale = "" }},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			framework := validFramework()
			tt.mutate(framework)
			err := catalog.ValidateFrameworkDefinition(framework)
			require.Error(t, err)
			assert.ErrorIs(t, err, constants.ErrInvalidEvidenceGraph)
		})
	}
}

func TestValidateCrosswalkRejectsUnknownAndInvalidReferences(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*compliancev1.ControlCrosswalkCatalog)
		want   error
	}{
		{name: "unknown framework version", mutate: func(c *compliancev1.ControlCrosswalkCatalog) { c.Mappings[0].FrameworkRef.Version = "unknown" }, want: constants.ErrUnsupportedFramework},
		{name: "unknown control", mutate: func(c *compliancev1.ControlCrosswalkCatalog) { c.Mappings[0].ControlId = "KSI-UNKNOWN-01" }, want: constants.ErrUnresolvedReference},
		{name: "unknown assertion", mutate: func(c *compliancev1.ControlCrosswalkCatalog) { c.Mappings[0].AssertionRefs[0].Id = "G8E-UNKNOWN-001" }, want: constants.ErrUnsupportedAssertion},
		{name: "duplicate assertion reference", mutate: func(c *compliancev1.ControlCrosswalkCatalog) {
			c.Mappings[0].AssertionRefs = append(c.Mappings[0].AssertionRefs, c.Mappings[0].AssertionRefs[0])
		}, want: constants.ErrInvalidEvidenceGraph},
		{name: "invalid mapping type", mutate: func(c *compliancev1.ControlCrosswalkCatalog) { c.Mappings[0].MappingType = "equivalent" }, want: constants.ErrInvalidEvidenceGraph},
		{name: "mismatched responsibility", mutate: func(c *compliancev1.ControlCrosswalkCatalog) { c.Mappings[0].Responsibility = "platform" }, want: constants.ErrInvalidEvidenceGraph},
		{name: "insufficient evidence level", mutate: func(c *compliancev1.ControlCrosswalkCatalog) { c.Mappings[0].RequiredEvidenceLevel = "L1" }, want: constants.ErrInvalidEvidenceGraph},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			candidate := validCrosswalk()
			tt.mutate(candidate)
			err := catalog.ValidateCrosswalkCatalog(candidate, validAssertionCatalog(), validFramework())
			require.Error(t, err)
			assert.ErrorIs(t, err, tt.want)
		})
	}
}

func TestValidateCrosswalkRejectsMappingToUnsupportedControl(t *testing.T) {
	framework := validFramework()
	framework.Controls[0].SupportStatus = "unsupported"
	framework.Controls[0].SupportRationale = "No reviewed assertion mapping exists."

	err := catalog.ValidateCrosswalkCatalog(validCrosswalk(), validAssertionCatalog(), framework)
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrInvalidEvidenceGraph)
}

func TestValidateEvidenceReferenceRejectsUnsafeAndCrossScopePaths(t *testing.T) {
	now := timestamppb.Now()
	valid := &compliancev1.ComplianceEvidenceReference{
		ArtifactId:         "artifact-1",
		ArtifactType:       "action_receipt",
		Sha256:             validSHA256,
		MediaType:          "application/json",
		SchemaRef:          "g8e.compliance.v1/action-receipt@1",
		ProducerIdentity:   "gateway:test",
		ProducedAt:         now,
		ScopeId:            "scope-1",
		RunId:              "run-1",
		VerificationStatus: "verified",
		VerifierId:         "receipt_integrity",
		VerifierVersion:    "1.0.0",
		VerifiedAt:         now,
		BundlePath:         "evidence/eval/receipts.jsonl",
	}

	tests := []struct {
		name   string
		mutate func(*compliancev1.ComplianceEvidenceReference)
		want   error
	}{
		{name: "absolute bundle path", mutate: func(r *compliancev1.ComplianceEvidenceReference) { r.BundlePath = "/tmp/receipt.json" }, want: constants.ErrInvalidEvidenceGraph},
		{name: "escaping bundle path", mutate: func(r *compliancev1.ComplianceEvidenceReference) { r.BundlePath = "evidence/../../receipt.json" }, want: constants.ErrInvalidEvidenceGraph},
		{name: "parent bundle path", mutate: func(r *compliancev1.ComplianceEvidenceReference) { r.BundlePath = constants.PathParentDir }, want: constants.ErrInvalidEvidenceGraph},
		{name: "drive-qualified bundle path", mutate: func(r *compliancev1.ComplianceEvidenceReference) { r.BundlePath = "C:/receipt.json" }, want: constants.ErrInvalidEvidenceGraph},
		{name: "cross scope reference", mutate: func(r *compliancev1.ComplianceEvidenceReference) { r.ScopeId = "scope-2" }, want: constants.ErrEvidenceScopeMismatch},
		{name: "malformed artifact digest", mutate: func(r *compliancev1.ComplianceEvidenceReference) { r.Sha256 = "bad" }, want: constants.ErrInvalidEvidenceGraph},
		{name: "unknown verifier version", mutate: func(r *compliancev1.ComplianceEvidenceReference) { r.VerifierVersion = "2.0.0" }, want: constants.ErrUnsupportedVerifier},
		{name: "invalid verification status", mutate: func(r *compliancev1.ComplianceEvidenceReference) { r.VerificationStatus = "trusted" }, want: constants.ErrInvalidEvidenceGraph},
		{name: "verification before production", mutate: func(r *compliancev1.ComplianceEvidenceReference) {
			r.VerifiedAt = timestamppb.New(r.ProducedAt.AsTime().Add(-time.Second))
		}, want: constants.ErrInvalidEvidenceGraph},
		{name: "incomplete encryption metadata", mutate: func(r *compliancev1.ComplianceEvidenceReference) {
			r.Encryption = &compliancev1.EvidenceEncryptionMetadata{Algorithm: "AES-256-GCM"}
		}, want: constants.ErrInvalidEvidenceGraph},
		{name: "malformed encrypted plaintext digest", mutate: func(r *compliancev1.ComplianceEvidenceReference) {
			r.Encryption = &compliancev1.EvidenceEncryptionMetadata{Algorithm: "AES-256-GCM", KeyId: "key-1", AuthorizationScope: "assessor", PlaintextSha256: "bad", AuthenticatedMetadataSha256: validSHA256}
		}, want: constants.ErrInvalidEvidenceGraph},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			candidate := proto.Clone(valid).(*compliancev1.ComplianceEvidenceReference)
			tt.mutate(candidate)
			err := catalog.ValidateEvidenceReference(candidate, "scope-1")
			require.Error(t, err)
			assert.True(t, errors.Is(err, tt.want), "error %v does not wrap %v", err, tt.want)
		})
	}
}

func TestCanonicalCatalogDigestsMatchContent(t *testing.T) {
	assertions := &compliancev1.ControlAssertionCatalog{}
	require.NoError(t, compliancev1.UnmarshalCanonical(complianceconstants.AssertionCatalogJSON(), assertions))
	frameworks := &compliancev1.FrameworkCatalog{}
	require.NoError(t, compliancev1.UnmarshalCanonical(complianceconstants.FrameworkCatalogJSON(), frameworks))
	crosswalks := &compliancev1.ControlCrosswalkCatalog{}
	require.NoError(t, compliancev1.UnmarshalCanonical(complianceconstants.FedRAMPAndNISTCrosswalkJSON(), crosswalks))

	assertDigest := func(t *testing.T, expected string, message proto.Message) {
		actual, err := catalog.CatalogDigest(message)
		require.NoError(t, err)
		assert.Equal(t, expected, actual)
	}
	t.Run("assertion catalog", func(t *testing.T) { assertDigest(t, assertions.Sha256, assertions) })
	for _, framework := range frameworks.Frameworks {
		framework := framework
		t.Run(framework.FrameworkId, func(t *testing.T) { assertDigest(t, framework.CatalogSha256, framework) })
	}
	t.Run("framework catalog", func(t *testing.T) { assertDigest(t, frameworks.Sha256, frameworks) })
	t.Run("crosswalk catalog", func(t *testing.T) { assertDigest(t, crosswalks.Sha256, crosswalks) })
	t.Run("demo scenario catalog", func(t *testing.T) {
		demoCatalog := &compliancev1.DemoScenarioCatalog{}
		require.NoError(t, compliancev1.UnmarshalCanonical(complianceconstants.DemoScenarioCatalogJSON(), demoCatalog))
		assertDigest(t, demoCatalog.Sha256, demoCatalog)
	})
}

func TestLoadCanonicalCatalogsResolveAllReferences(t *testing.T) {
	assertions, frameworks, crosswalks, err := catalog.LoadCanonicalCatalogs()
	require.NoError(t, err)
	require.NoError(t, catalog.ValidateCatalogSet(assertions, frameworks, crosswalks))

	for _, id := range []string{"G8E-GOV-BLOCK-001", "G8E-AU-RECEIPT-001", "G8E-CM-STATE-001"} {
		assert.NotNil(t, catalog.FindAssertion(assertions, id, "1.0.0"), id)
	}
	assert.NotNil(t, catalog.FindFramework(frameworks, "fedramp-20x", "CR26-2026-06-24"))
	assert.NotNil(t, catalog.FindFramework(frameworks, "nist-sp-800-53", "rev5"))
}

func TestLoadDemoScenarioCatalogResolvesAssertionAndFrameworkReferences(t *testing.T) {
	assertions, frameworks, crosswalks, err := catalog.LoadCanonicalCatalogs()
	require.NoError(t, err)

	demoCatalog, err := catalog.LoadDemoScenarioCatalog(assertions, frameworks)
	require.NoError(t, err)
	expectedScenarioIDs := []string{
		"healthcare-success",
		"healthcare-gold-card",
		"healthcare-sla-breach",
		"healthcare-phi-blocked",
		"finance-unauthorized-trade",
		"dhs-ingest",
		"dhs-disconnected-operations",
		"dhs-cue",
		"dhs-destruction-block",
		"dhs-destruction-purge",
		"fedramp-provision",
		"fedramp-deny",
		"fedramp-revert",
		"fedramp-evidence-block",
	}
	require.Len(t, demoCatalog.Definitions, len(expectedScenarioIDs))
	for _, scenarioID := range expectedScenarioIDs {
		assert.NotNil(t, catalog.FindDemoScenarioDefinition(demoCatalog, scenarioID, "1.0.0"), scenarioID)
	}

	definition := catalog.FindDemoScenarioDefinition(demoCatalog, "fedramp-deny", "1.0.0")
	require.NotNil(t, definition)
	assert.Equal(t, "blocked", definition.ExpectedOutcome)
	assert.Equal(t, "L1", definition.ExpectedRejectionLayer)
	assert.Equal(t, "L3", definition.RequiredEvidenceLevel)

	for _, ref := range definition.AssertionRefs {
		assert.NotNil(t, catalog.FindAssertion(assertions, ref.Id, ref.Version), ref.Id)
	}
	for _, ref := range definition.FrameworkControlRefs {
		framework := catalog.FindFramework(frameworks, ref.FrameworkRef.Id, ref.FrameworkRef.Version)
		require.NotNil(t, framework, ref.FrameworkRef.Id)
		assert.NotNil(t, catalog.FindFrameworkControl(framework, ref.ControlId), ref.ControlId)
	}

	// Crosswalk consistency: every framework-control reference in each definition has at
	// least one canonical crosswalk mapping to an assertion carried by that definition.
	scenarioAssertions := make(map[string]bool, len(definition.AssertionRefs))
	for _, ref := range definition.AssertionRefs {
		scenarioAssertions[ref.Id] = true
	}
	for _, ref := range definition.FrameworkControlRefs {
		hasMapping := false
		for _, mapping := range crosswalks.Mappings {
			if mapping.FrameworkRef.Id != ref.FrameworkRef.Id ||
				mapping.FrameworkRef.Version != ref.FrameworkRef.Version ||
				mapping.ControlId != ref.ControlId {
				continue
			}
			for _, assertionRef := range mapping.AssertionRefs {
				if scenarioAssertions[assertionRef.Id] {
					hasMapping = true
					break
				}
			}
			if hasMapping {
				break
			}
		}
		assert.True(t, hasMapping,
			"scenario %s: framework control %s@%s:%s has no crosswalk mapping to any assertion carried by this definition",
			definition.ScenarioId, ref.FrameworkRef.Id, ref.FrameworkRef.Version, ref.ControlId)
	}
}

// TestLoadDemoScenarioCatalogEveryDefinitionHasCrosswalkConsistency verifies that every
// framework-control reference across all 13 demo scenario definitions has at least one
// canonical crosswalk mapping to an assertion carried by that definition. This is the
// catalog-wide assertion of the per-scenario check in
// TestLoadDemoScenarioCatalogResolvesAssertionAndFrameworkReferences.
func TestLoadDemoScenarioCatalogEveryDefinitionHasCrosswalkConsistency(t *testing.T) {
	assertions, frameworks, crosswalks, err := catalog.LoadCanonicalCatalogs()
	require.NoError(t, err)
	demoCatalog, err := catalog.LoadDemoScenarioCatalog(assertions, frameworks)
	require.NoError(t, err)

	crosswalkByControl := make(map[string][]*compliancev1.ControlCrosswalk)
	for _, mapping := range crosswalks.Mappings {
		key := mapping.FrameworkRef.Id + "@" + mapping.FrameworkRef.Version + ":" + mapping.ControlId
		crosswalkByControl[key] = append(crosswalkByControl[key], mapping)
	}

	for _, definition := range demoCatalog.Definitions {
		scenarioAssertions := make(map[string]bool, len(definition.AssertionRefs))
		for _, ref := range definition.AssertionRefs {
			scenarioAssertions[ref.Id] = true
		}
		for _, ref := range definition.FrameworkControlRefs {
			key := ref.FrameworkRef.Id + "@" + ref.FrameworkRef.Version + ":" + ref.ControlId
			mappings, ok := crosswalkByControl[key]
			require.True(t, ok,
				"scenario %s: no crosswalk exists for %s",
				definition.ScenarioId, key)
			hasOverlap := false
			for _, mapping := range mappings {
				for _, assertionRef := range mapping.AssertionRefs {
					if scenarioAssertions[assertionRef.Id] {
						hasOverlap = true
						break
					}
				}
				if hasOverlap {
					break
				}
			}
			assert.True(t, hasOverlap,
				"scenario %s: crosswalk for %s does not map to any assertion carried by this definition",
				definition.ScenarioId, key)
		}
	}
}
