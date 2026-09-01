// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package compliance_test

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	legacycompliance "github.com/g8e-ai/g8e/v2/internal/services/compliance"
	"github.com/g8e-ai/g8e/v2/internal/services/compliance/catalog"
)

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
