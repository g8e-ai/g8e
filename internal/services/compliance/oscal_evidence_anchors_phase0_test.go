// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package compliance

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestPhase0OSCAL_EvidenceAnchorsAreNonContentAddressedDescriptionStrings
// documents that the current OSCAL assessment-results export produces
// relevant-evidence anchors as synthetic fragment strings of the form
// "#<type>:<reference>" rather than resolvable, content-addressed artifact
// references inside a complete evidence bundle.
//
// The href is a string like "#receipt_id:tx-123". It carries no sha256
// digest, no bundle-relative path, no producer identity, no verification
// status, and no schema reference. No evidence bundle exists today, so the
// anchor cannot resolve to a typed artifact. Phase 1 adds
// ComplianceEvidenceReference with digest, producer, scope binding, verifier,
// and bundle_path; Phase 4 makes OSCAL anchors resolve to bundle resources.
func TestPhase0OSCAL_EvidenceAnchorsAreNonContentAddressedDescriptionStrings(t *testing.T) {
	exporter := NewOSCALExporter(oscalTestCatalog())
	resultSet := oscalTestResultSet()

	doc, err := exporter.GenerateAssessmentResults(resultSet)
	require.NoError(t, err)
	require.NotEmpty(t, doc.Results)
	require.NotEmpty(t, doc.Results[0].Observations)

	// Collect every relevant-evidence href across all observations.
	var hrefs []string
	for _, obs := range doc.Results[0].Observations {
		for _, ev := range obs.RelevantEvidence {
			hrefs = append(hrefs, ev.Href)
		}
	}
	require.NotEmpty(t, hrefs, "test result set must produce at least one evidence anchor")

	for _, href := range hrefs {
		// Current anchors are fragment strings starting with "#".
		assert.True(t, strings.HasPrefix(href, "#"),
			phase0RegressionBeforeFix+
				": evidence anchor %q is a fragment string, not a bundle-relative path", href)

		// No content digest is present in the anchor.
		assert.NotContains(t, href, "sha256:",
			phase0RegressionBeforeFix+
				": evidence anchor %q carries no content digest", href)

		// No bundle-relative path (e.g. "evidence/") is present.
		assert.NotContains(t, href, "evidence/",
			phase0RegressionBeforeFix+
				": evidence anchor %q is not a bundle-relative path", href)
	}
}

// TestPhase0OSCAL_EvidenceAnchorCannotResolveToTypedArtifact documents that
// the current OSCAL evidence anchor is a description string that cannot be
// resolved to any typed artifact because no evidence bundle layout exists.
// The back-matter resources section contains only the KSI catalog reference,
// not individual evidence artifacts. Phase 4 adds resolvable evidence
// resources to the bundle back-matter.
func TestPhase0OSCAL_EvidenceAnchorCannotResolveToTypedArtifact(t *testing.T) {
	exporter := NewOSCALExporter(oscalTestCatalog())
	resultSet := oscalTestResultSet()

	doc, err := exporter.GenerateAssessmentResults(resultSet)
	require.NoError(t, err)

	// The back-matter contains only the KSI catalog resource, not evidence.
	var resourceTitles []string
	for _, res := range doc.Results[0].Observations {
		_ = res
	}
	_ = resourceTitles

	// Check the component definition back-matter for evidence resources.
	compDef, err := exporter.GenerateComponentDefinition()
	require.NoError(t, err)

	var backMatterTitles []string
	for _, r := range compDef.BackMatter.Resources {
		backMatterTitles = append(backMatterTitles, r.Title)
	}

	// The only back-matter resource is the KSI catalog, not evidence artifacts.
	for _, title := range backMatterTitles {
		assert.NotContains(t, strings.ToLower(title), "evidence",
			phase0RegressionBeforeFix+
				": back-matter resource %q is not an evidence artifact; no evidence bundle exists", title)
	}
}
