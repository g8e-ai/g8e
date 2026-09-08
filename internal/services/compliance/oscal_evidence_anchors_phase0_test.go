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

// TestPhase0OSCAL_EvidenceAnchorsResolveToContentAddressedResources verifies the Phase 4 replacement for synthetic evidence fragments.
func TestPhase0OSCAL_EvidenceAnchorsResolveToContentAddressedResources(t *testing.T) {
	doc, err := NewOSCALExporter(nil).GenerateAssessmentResults(oscalTestAnalysis())
	require.NoError(t, err)
	resources := make(map[string]OSCALResource, len(doc.AssessmentResults.BackMatter.Resources))
	for _, resource := range doc.AssessmentResults.BackMatter.Resources {
		resources[resource.UUID] = resource
	}
	require.NotEmpty(t, resources)
	for _, observation := range doc.AssessmentResults.Results[0].Observations {
		for _, relevantEvidence := range observation.RelevantEvidence {
			resource, exists := resources[strings.TrimPrefix(relevantEvidence.Href, "#")]
			assert.True(t, exists, phase0RegressionAfterFix+": evidence anchor must resolve to an OSCAL back-matter resource")
			assert.Contains(t, oscalPropValue(resource.Props, "artifact-id"), "sha256:", phase0RegressionAfterFix+": resolved resource must carry a content address")
		}
	}
}

// TestPhase0OSCAL_EvidenceResourcesPreserveTypedArtifactMetadata verifies that analysis evidence metadata survives OSCAL projection.
func TestPhase0OSCAL_EvidenceResourcesPreserveTypedArtifactMetadata(t *testing.T) {
	analysis := oscalTestAnalysis()
	doc, err := NewOSCALExporter(nil).GenerateAssessmentResults(analysis)
	require.NoError(t, err)
	require.Len(t, doc.AssessmentResults.BackMatter.Resources, len(analysis.GetEvidenceResources()))
	for _, resource := range doc.AssessmentResults.BackMatter.Resources {
		assert.NotEmpty(t, oscalPropValue(resource.Props, "artifact-type"), phase0RegressionAfterFix)
		assert.Len(t, oscalPropValue(resource.Props, "sha256"), 64, phase0RegressionAfterFix)
		assert.NotEmpty(t, oscalPropValue(resource.Props, "producer-identity"), phase0RegressionAfterFix)
		assert.Equal(t, "verified", oscalPropValue(resource.Props, "verification-status"), phase0RegressionAfterFix)
		assert.NotEmpty(t, oscalPropValue(resource.Props, "bundle-path"), phase0RegressionAfterFix)
	}
}
