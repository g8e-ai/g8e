// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package compliance_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	legacycompliance "github.com/g8e-ai/g8e/v2/internal/services/compliance"
	"github.com/g8e-ai/g8e/v2/internal/services/compliance/catalog"
	compliancev1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/compliance/v1"
)

type doctrineReferenceCatalog struct {
	Doctrines []doctrineReference `json:"doctrines"`
}

type doctrineReference struct {
	ID            string                             `json:"id"`
	KSIIDs        []string                           `json:"ksi_ids"`
	ControlIDs    []string                           `json:"control_ids"`
	AssertionRefs []*compliancev1.VersionedReference `json:"assertion_refs"`
}

func TestCanonicalComplianceCatalogMatchesReviewedPhase1Scope(t *testing.T) {
	assertions, frameworks, crosswalks, err := catalog.LoadCanonicalCatalogs()
	require.NoError(t, err)

	mappedControls := 0
	unsupportedControls := 0
	for _, framework := range frameworks.Frameworks {
		for _, control := range framework.Controls {
			switch control.SupportStatus {
			case "mapped":
				mappedControls++
			case "unsupported":
				unsupportedControls++
			}
		}
	}
	assert.Len(t, assertions.Assertions, 13)
	assert.Len(t, crosswalks.Mappings, 57)
	assert.Equal(t, 33, mappedControls)
	assert.Equal(t, 98, unsupportedControls)
}

func TestCanonicalCrosswalkMapsEveryInitialAssertion(t *testing.T) {
	assertions, _, crosswalks, err := catalog.LoadCanonicalCatalogs()
	require.NoError(t, err)

	mapped := make(map[string]struct{})
	for _, mapping := range crosswalks.Mappings {
		for _, reference := range mapping.AssertionRefs {
			mapped[reference.Id+"@"+reference.Version] = struct{}{}
		}
	}
	for _, assertion := range assertions.Assertions {
		_, exists := mapped[assertion.AssertionId+"@"+assertion.AssertionVersion]
		assert.True(t, exists, assertion.AssertionId)
	}
}

func TestCanonicalFrameworkCatalogSupportDispositionsMatchCrosswalkCoverage(t *testing.T) {
	_, frameworks, crosswalks, err := catalog.LoadCanonicalCatalogs()
	require.NoError(t, err)

	mapped := make(map[string]struct{}, len(crosswalks.Mappings))
	for _, mapping := range crosswalks.Mappings {
		mapped[mapping.FrameworkRef.Id+"@"+mapping.FrameworkRef.Version+":"+mapping.ControlId] = struct{}{}
	}
	for _, framework := range frameworks.Frameworks {
		for _, control := range framework.Controls {
			key := framework.FrameworkId + "@" + framework.FrameworkVersion + ":" + control.ControlId
			_, hasMapping := mapped[key]
			assert.Equal(t, control.SupportStatus == "mapped", hasMapping, key)
		}
	}
}

func TestFedRAMPDoctrineReferencesResolveCanonicalCatalogs(t *testing.T) {
	assertions, frameworks, _, err := catalog.LoadCanonicalCatalogs()
	require.NoError(t, err)
	fedRAMP := catalog.FindFramework(frameworks, "fedramp-20x", "CR26-2026-06-24")
	require.NotNil(t, fedRAMP)
	nist := catalog.FindFramework(frameworks, "nist-sp-800-53", "rev5")
	require.NotNil(t, nist)

	doctrinePath := filepath.Join(constants.TestPathRepoRootFromCompliancePackage, constants.DemosDirname, constants.DemosOrgFedRAMP, constants.DemosDoctrineDir, constants.DemosFedRAMPDoctrineFile)
	doctrineJSON, err := os.ReadFile(doctrinePath)
	require.NoError(t, err)
	var doctrineCatalog doctrineReferenceCatalog
	require.NoError(t, json.Unmarshal(doctrineJSON, &doctrineCatalog))
	require.NotEmpty(t, doctrineCatalog.Doctrines)
	for _, doctrine := range doctrineCatalog.Doctrines {
		require.NotEmpty(t, doctrine.AssertionRefs, doctrine.ID)
		for _, ksiID := range doctrine.KSIIDs {
			assert.NotNil(t, catalog.FindFrameworkControl(fedRAMP, ksiID), "%s references %s", doctrine.ID, ksiID)
		}
		for _, controlID := range doctrine.ControlIDs {
			assert.NotNil(t, catalog.FindFrameworkControl(nist, controlID), "%s references %s", doctrine.ID, controlID)
		}
		for _, assertionRef := range doctrine.AssertionRefs {
			assert.NotNil(t, catalog.FindAssertion(assertions, assertionRef.Id, assertionRef.Version), "%s references %s@%s", doctrine.ID, assertionRef.Id, assertionRef.Version)
		}
	}
}

func TestCanonicalFrameworkCatalogResolvesKSIAndOverlayControlReferences(t *testing.T) {
	_, frameworks, _, err := catalog.LoadCanonicalCatalogs()
	require.NoError(t, err)
	fedRAMP := catalog.FindFramework(frameworks, "fedramp-20x", "CR26-2026-06-24")
	require.NotNil(t, fedRAMP)
	nist := catalog.FindFramework(frameworks, "nist-sp-800-53", "rev5")
	require.NotNil(t, nist)

	ksiPath := filepath.Join(constants.TestPathRepoRootFromCompliancePackage, constants.DefaultKSICatalogPath)
	ksiCatalog, err := legacycompliance.LoadKSICatalog(ksiPath)
	require.NoError(t, err)
	for _, ksi := range ksiCatalog.KSIs {
		assert.NotNil(t, catalog.FindFrameworkControl(fedRAMP, ksi.ID), ksi.ID)
		for _, controlRef := range ksi.ControlRefs {
			assert.NotNil(t, catalog.FindFrameworkControl(nist, controlRef), "%s references %s", ksi.ID, controlRef)
		}
	}

	overlayPath := filepath.Join(constants.TestPathRepoRootFromCompliancePackage, constants.DefaultOverlayDirPath, constants.COSAiSOverlaysFilename)
	overlayCatalog, err := legacycompliance.LoadOverlayCatalog(overlayPath)
	require.NoError(t, err)
	for _, overlay := range overlayCatalog.Overlays {
		for _, controlRef := range overlay.ControlRefs {
			assert.NotNil(t, catalog.FindFrameworkControl(nist, controlRef), "%s references %s", overlay.ID, controlRef)
		}
	}
}
